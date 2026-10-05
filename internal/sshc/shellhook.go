package sshc

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// A program cannot change the environment of the shell that started it, so
// "sshc set --session" (and "unset --session") works through a small shell
// function that wraps the sshc command. For "set" and "unset" the function
// runs the real sshc with $SSHC_EMIT naming the shell's syntax, and evaluates
// what sshc prints on stdout: one assignment in session mode, nothing
// otherwise. sshc sends everything meant for the human to stderr meanwhile.
//
// The same hook sets up tab completion, which asks "sshc --complete" for the
// candidates: host names from the ssh config, sshc's commands and options.
const envEmit = "SSHC_EMIT"

const hookPosix = `export SSHC_HOOK=posix
sshc() {
	case "${1-}" in
	set | unset | use)
		__sshc_code=$(SSHC_EMIT=posix command sshc "$@") || { unset __sshc_code; return 1; }
		eval "$__sshc_code"
		unset __sshc_code
		;;
	*)
		command sshc "$@"
		;;
	esac
}
# Tab completion. Each shell's own syntax is kept inside eval so that the
# others never have to parse it.
if [ -n "${BASH_VERSION-}" ]; then
	eval "$(command sshc --shell-init bash-completion)"
elif [ -n "${ZSH_VERSION-}" ]; then
	eval "$(command sshc --shell-init zsh-completion)"
fi
`

const completeBash = `_sshc_complete() {
	local IFS="
"
	COMPREPLY=($(command sshc --complete "$COMP_CWORD" "${COMP_WORDS[@]}" 2>/dev/null))
	case "${COMPREPLY[0]-}" in *:) compopt -o nospace 2>/dev/null ;; esac
}
complete -o default -F _sshc_complete sshc
`

const completeZsh = `_sshc_complete() {
	local -a found
	found=("${(@f)$(command sshc --complete $((CURRENT - 1)) "${words[@]}" 2>/dev/null)}")
	if [[ -z "${found[1]}" ]]; then
		_files
	elif [[ "${found[1]}" == *: ]]; then
		compadd -S "" -a found
	else
		compadd -a found
	fi
}
if (( $+functions[compdef] )); then compdef _sshc_complete sshc; fi
`

const hookFish = `set -gx SSHC_HOOK fish
function sshc
    if test (count $argv) -ge 1; and contains -- "$argv[1]" set unset use
        set -l code (SSHC_EMIT=fish command sshc $argv); or return 1
        eval $code
    else
        command sshc $argv
    end
end
complete -c sshc -e
complete -c sshc -a "(command sshc --complete (count (commandline -opc)) (commandline -opc) (commandline -ct) 2>/dev/null)"
`

const hookPowerShell = `$env:SSHC_HOOK = 'powershell'
function sshc {
    $exe = (Get-Command sshc -CommandType Application -ErrorAction Stop | Select-Object -First 1).Source
    if ($args.Count -ge 1 -and (@('set', 'unset', 'use') -contains $args[0])) {
        $env:SSHC_EMIT = 'powershell'
        try {
            if ($MyInvocation.ExpectingInput) { $code = $input | & $exe @args } else { $code = & $exe @args }
        } finally { Remove-Item Env:SSHC_EMIT -ErrorAction SilentlyContinue }
        if ($LASTEXITCODE -eq 0 -and $code) { Invoke-Expression ($code -join [Environment]::NewLine) }
    } elseif ($MyInvocation.ExpectingInput) {
        $input | & $exe @args
    } else {
        & $exe @args
    }
}
Register-ArgumentCompleter -Native -CommandName sshc -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)
    $exe = (Get-Command sshc -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1).Source
    if (-not $exe) { return }
    $words = @($commandAst.CommandElements | ForEach-Object { $_.Extent.Text })
    $index = $words.Count
    if ($wordToComplete) { $index = $words.Count - 1 }
    & $exe --complete $index @words 2>$null | ForEach-Object {
        [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
    }
}
`

// shellKind maps a shell name to the syntax family sshc can emit.
func shellKind(name string) string {
	switch strings.ToLower(strings.TrimSuffix(filepath.Base(name), ".exe")) {
	case "powershell", "pwsh":
		return "powershell"
	case "fish":
		return "fish"
	case "posix", "sh", "bash", "zsh", "dash", "ksh", "ash":
		return "posix"
	}
	return ""
}

func defaultShellKind() string {
	if runtime.GOOS == "windows" {
		return "powershell"
	}
	if k := shellKind(os.Getenv("SHELL")); k != "" {
		return k
	}
	return "posix"
}

// hookLine is what a shell startup file needs in order to load the hook.
func hookLine(kind string) string {
	switch kind {
	case "powershell":
		return `sshc --shell-init powershell | Out-String | Invoke-Expression`
	case "fish":
		return `command -q sshc; and sshc --shell-init fish | source`
	}
	return `command -v sshc >/dev/null 2>&1 && eval "$(sshc --shell-init posix)"`
}

func cmdShellInit(args []string) int {
	kind := defaultShellKind()
	if len(args) == 1 {
		kind = shellKind(args[0])
	}
	// The posix hook asks for these two itself, from inside the right shell.
	if len(args) == 1 {
		switch args[0] {
		case "bash-completion":
			fmt.Print(completeBash)
			return 0
		case "zsh-completion":
			fmt.Print(completeZsh)
			return 0
		}
	}
	if len(args) > 1 || kind == "" {
		warnf("usage: %s --shell-init [bash|zsh|fish|powershell]", prog)
		return 1
	}
	switch kind {
	case "powershell":
		fmt.Print(hookPowerShell)
	case "fish":
		fmt.Print(hookFish)
	default:
		fmt.Print(hookPosix)
	}
	return 0
}

// emitAssignment renders "set this variable in the current shell" in the
// given shell's syntax, quoting the value so it is taken literally.
func emitAssignment(kind, name, value string) (string, bool) {
	switch kind {
	case "posix":
		return "export " + name + "='" + strings.ReplaceAll(value, "'", `'\''`) + "'", true
	case "fish":
		v := strings.ReplaceAll(value, `\`, `\\`)
		return "set -gx " + name + " '" + strings.ReplaceAll(v, "'", `\'`) + "'", true
	case "powershell":
		// PowerShell also treats the typographic single quotes as quotes.
		v := value
		for _, q := range []string{"'", "‘", "’", "‚", "‛"} {
			v = strings.ReplaceAll(v, q, q+q)
		}
		return "$env:" + name + " = '" + v + "'", true
	}
	return "", false
}

// emitUnset renders "remove this variable from the current shell".
func emitUnset(kind, name string) (string, bool) {
	switch kind {
	case "posix":
		return "unset " + name, true
	case "fish":
		return "set -e " + name, true
	case "powershell":
		return "Remove-Item Env:" + name + " -ErrorAction SilentlyContinue", true
	}
	return "", false
}
