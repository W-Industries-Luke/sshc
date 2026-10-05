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
)

const (
	prog    = "sshc"
	version = "0.3.1"
)

const usageText = `Usage: sshc [ssh] [ssh options] destination [command ...]
       sshc scp  [scp options] source ... target
       sshc sftp [sftp options] destination
       sshc rsync [rsync options] source ... target
       sshc ssh-copy-id [ssh-copy-id options] destination

Runs the tool and answers its prompt for a login password, or for the
passphrase of an SSH key, from what you have stored. Everything after the
optional tool name is handed to that tool unchanged.

  sshc set [-s] [-p] [value]    store a password, or with -p a key passphrase:
                                in your system's credential store, or with -s
                                for this terminal only ("sshc set -h")
  sshc unset [-s] [-p]          remove one ("sshc unset -h")
  sshc list                     show what is stored and where, not the values
  sshc check [tool] args...     show where the password would come from
  sshc init                     create a config file template
  sshc install [directory]      copy sshc to a per-user directory, put it on
                                your PATH and set up the shell hook
  sshc shell-init [shell]       print the shell hook that "set -s" needs
  sshc help | version           also -h and -v, when given on their own

Options of set and unset. Letters combine: "sshc set -sp" stores a key
passphrase for this terminal only.
  -s, --session         this terminal only, as an environment variable
  -p, --passphrase      a key passphrase rather than a login password
  -H, --host NAME       the login password of one host
  -k, --key NAME        the passphrase of one key
  -P, --profile NAME    a profile other than the active one
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
system's credential store, or in the file itself where there is none.

Config file: $SSHC_CONFIG, else sshc.conf next to the sshc executable, else
sshc/sshc.conf in your user config directory. On Linux and macOS it must be
private to you (chmod 600).
`

func warnf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, prog+": "+format+"\n", a...)
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
	case "ssh", "scp", "sftp", "rsync", "ssh-copy-id":
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
	}
	tool := "ssh"
	if isTool(args[0]) {
		tool, args = args[0], args[1:]
	}
	return runTool(tool, args)
}
