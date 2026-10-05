// Package sshc implements the sshc command.
//
// sshc starts the real OpenSSH client with itself registered as the askpass
// program (SSH_ASKPASS), and the client calls back into sshc whenever it needs
// something typed. The files are organised by what they do:
//
//	cli.go        usage text and command dispatch
//	run.go        starting ssh/scp/sftp/rsync with the askpass hook in place
//	askpass.go    the callback: recognising a prompt and answering it
//	lookup.go     which stored password or passphrase applies
//	dest.go       working out the destination hosts on a command line
//	config.go     finding, checking and parsing the config file
//	set.go        "sshc set": saving a secret to the file or the session
//	shellhook.go  the shell function behind "sshc set --session"
//	install.go    "sshc --install"
//	*_unix.go, *_windows.go   the parts that differ per operating system
package sshc

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

const prog = "sshc"

// version is a variable so that a test build can pretend to be an old one.
var version = "0.6.1"

const usageText = `Usage: sshc [ssh] [ssh options] destination [command ...]
       sshc scp  [scp options] source ... target
       sshc sftp [sftp options] destination
       sshc rsync [rsync options] source ... target
       sshc ssh-copy-id [ssh-copy-id options] destination
       sshc ssh-add [ssh-add options] [key ...]

Runs the tool and answers its prompt for a login password, or for the
passphrase of an SSH key, from what you have stored. Everything after the
optional tool name is handed to that tool unchanged.

Storing secrets:
  sshc set [-s] [-p] [value]    store a password, or with -p a key passphrase:
                                in your system's credential store, or with -s
                                for this terminal only ("sshc set -h")
  sshc unset [-s] [-p]          remove one ("sshc unset -h")
  sshc list                     show what is stored and where, not the values
  sshc check [tool] args...     show where the password would come from
  sshc use [NAME]               show or switch the active profile
  sshc migrate                  move plain-text entries into the credential
                                store
  sshc lock                     switch sshc off: nothing stored is supplied
  sshc unlock                   switch it back on, after your device has
                                verified you (PIN, or account password)

More ways to connect:
  sshc                          on its own: pick a host from your ssh config
  sshc pick                     the same, explicitly
  sshc hosts                    list the hosts in your ssh config
  sshc each HOST... -- COMMAND  run a command on several hosts at once
  sshc run [-d HOST] COMMAND    run any program that uses ssh underneath
                                (git, ansible, ...) with prompts answered
  sshc ssh-add [KEY]            unlock a key into ssh-agent, so that plain
                                ssh, git and the rest need no prompt either

Setup:
  sshc install [directory]      copy sshc to a per-user directory, put it on
                                your PATH and set up the shell hook
  sshc update [--check]         install the latest release
  sshc doctor                   check the setup and say how to fix problems
  sshc init                     create a config file template
  sshc shell-init [shell]       print the shell hook: "set -s" and tab
                                completion need it
  sshc help [COMMAND]           this text, or the help of one command
  sshc version                  also -v (and -h for help) when given alone

Options of set and unset. Letters combine: "sshc set -sp" stores a key
passphrase for this terminal only.
  -s, --session         this terminal only, as an environment variable
  -p, --passphrase      a key passphrase rather than a login password
  -H, --host NAME       the login password of one host
  -k, --key NAME        the passphrase of one key
  -P, --profile NAME    a profile other than the active one
  -c, --command CMD     do not store it: run CMD (a password manager) each
                        time and use what it prints (set only)
  -o, --otp             the one-time code a host asks for after the password;
                        needs -H and -c
  -f, --plain           keep it in the config file, in plain text (set only)
  -n, --no-clear        do not clear the screen afterwards (set only)
  -h, --help            the full help for set or unset

check, init, install, shell-init, help and version can also be written with a
leading "--". A host that shares a name with one of these words is reachable
as "sshc ssh <name>".

Password sources, first match wins:
  1. $SSHC_PASSWORD_<HOST>   one host, e.g. SSHC_PASSWORD_W_GO_2 for "w.go-2"
  2. [host <name>] entry     saved with "sshc set --host"
  3. $SSHC_PASSWORD          the active password, for the host(s) you name
  4. [profile <name>] entry  the active profile, selected by $SSHC_PROFILE or
                             the config file's "profile =" line

Key passphrases work the same way: $SSHC_PASSPHRASE_<KEYFILE>, a [key <name>]
section, $SSHC_PASSPHRASE, then "passphrase =" in the active profile.

Saved entries are listed in the config file; their values are kept in your
system's credential store, fetched by a command you name, or - where there is
no store - in the file itself.

Other variables: SSHC_PROFILE picks the profile, SSHC_NO_PROMPT=1 makes sshc
fail instead of asking on the terminal (for scripts), SSHC_DEBUG=1 shows what
it decides, NO_COLOR=1 turns colour off.

Config file: $SSHC_CONFIG, else sshc.conf next to the sshc executable, else
sshc/sshc.conf in your user config directory. On Linux and macOS it must be
private to you (chmod 600).
`

// warnf reports a problem on stderr.
func warnf(format string, a ...any) {
	u := newUI(os.Stderr)
	u.say(prog+": "+format, a...)
	u.flush()
}

// failf reports a problem together with what to do about it.
func failf(problem string, advice ...string) {
	u := newUI(os.Stderr)
	u.say("%s: %s", prog, problem)
	for _, a := range advice {
		u.say("%s", a)
	}
	u.flush()
}

// debugf traces what sshc decides when $SSHC_DEBUG is set. It never prints a
// password.
func debugf(format string, a ...any) {
	if os.Getenv("SSHC_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, prog+"[debug]: "+format+"\n", a...)
	}
}

func isTool(s string) bool {
	switch s {
	case "ssh", "scp", "sftp", "rsync", "ssh-copy-id", "ssh-add":
		return true
	}
	return false
}

// Main runs sshc with the given command-line arguments and returns its exit
// status.
func Main(args []string) int {
	if isAskpassCall(args) {
		return askpassMain(args[0])
	}
	if os.Getenv(envState) != "" {
		debugf("started inside an sshc session, but not as an askpass call: %d args %q", len(args), args)
	}
	if len(args) == 0 {
		// With hosts to choose from and someone to ask, offer them.
		if term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd())) && len(sshConfigHosts()) > 0 {
			return cmdPick(nil)
		}
		fmt.Print(usageText)
		return 0
	}
	// On their own, -v and -h cannot mean anything to ssh, which needs a
	// destination, so they are short for --version and --help. Next to a
	// destination they are passed through: "sshc -v host" is still ssh's
	// verbose mode.
	if len(args) == 1 {
		switch args[0] {
		case "-v":
			args = []string{"--version"}
		case "-h":
			args = []string{"--help"}
		}
	}
	// Each action has a plain word as well as its "--" form. Single letters
	// are not used for the ones that take arguments: "-c" and the like are
	// ssh options, and "sshc -c x host" has to stay an ssh command.
	switch args[0] {
	case "--help", "help":
		// "sshc help set" and the like show that command's own help.
		if len(args) == 2 {
			if text, ok := helpTopics[args[1]]; ok {
				fmt.Print(text)
				return 0
			}
		}
		fmt.Print(usageText)
		return 0
	case "--version", "version":
		fmt.Println(prog, version)
		return 0
	case "--init", "init":
		return cmdInit()
	case "--install", "install":
		return cmdInstall(args[1:])
	case "--shell-init", "shell-init":
		return cmdShellInit(args[1:])
	case "--check", "check":
		return cmdCheck(args[1:])
	case "set":
		return cmdSet(args[1:])
	case "unset":
		return cmdUnset(args[1:])
	case "list":
		return cmdList(args[1:])
	case "run":
		return cmdRun(args[1:])
	case "each":
		return cmdEach(args[1:])
	case "hosts":
		return cmdHosts(args[1:])
	case "pick":
		return cmdPick(args[1:])
	case "--complete":
		return cmdComplete(args[1:])
	case "use":
		return cmdUse(args[1:])
	case "migrate":
		return cmdMigrate(args[1:])
	case "update":
		return cmdUpdate(args[1:])
	case "doctor":
		return cmdDoctor(args[1:])
	case "lock":
		return cmdLock(args[1:])
	case "unlock":
		return cmdUnlock(args[1:])
	}
	tool := "ssh"
	if isTool(args[0]) {
		tool, args = args[0], args[1:]
	}
	return runTool(tool, args)
}

// helpTopics are the commands with a help text of their own.
var helpTopics = map[string]string{
	"set":   setUsage,
	"unset": unsetUsage,
	"use":   useUsage,
	"run":   runUsage,
	"each":  eachUsage,
}
