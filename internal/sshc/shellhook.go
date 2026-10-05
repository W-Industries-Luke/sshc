package sshc

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// A program cannot change the environment of the shell that started it, so
// "sshc set --session" works through a small shell function that wraps the
// sshc command. The function runs the real sshc with $SSHC_EMIT naming the
// shell's syntax, and evaluates the one assignment sshc prints on stdout.
const envEmit = "SSHC_EMIT"

const hookPosix = `sshc() {
	if [ "${1-}" = set ]; then
		for __sshc_arg in "$@"; do
			if [ "$__sshc_arg" = --session ]; then
				__sshc_code=$(SSHC_EMIT=posix command sshc "$@") || { unset __sshc_arg __sshc_code; return 1; }
				eval "$__sshc_code"
				unset __sshc_arg __sshc_code
				return 0
			fi
		done
		unset __sshc_arg
	fi
	command sshc "$@"
}
`

const hookFish = `function sshc
    if test (count $argv) -ge 2; and test "$argv[1]" = set; and contains -- --session $argv
        set -l code (SSHC_EMIT=fish command sshc $argv); or return 1
        eval $code
    else
        command sshc $argv
    end
end
`

const hookPowerShell = `function sshc {
    $exe = (Get-Command sshc -CommandType Application -ErrorAction Stop | Select-Object -First 1).Source
    if ($args.Count -ge 2 -and $args[0] -eq 'set' -and $args -contains '--session') {
        $env:SSHC_EMIT = 'powershell'
        try { $code = & $exe @args } finally { Remove-Item Env:SSHC_EMIT -ErrorAction SilentlyContinue }
        if ($LASTEXITCODE -eq 0 -and $code) { Invoke-Expression ($code -join [Environment]::NewLine) }
    } elseif ($MyInvocation.ExpectingInput) {
        $input | & $exe @args
    } else {
        & $exe @args
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
