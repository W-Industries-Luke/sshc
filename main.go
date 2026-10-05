// sshc runs ssh, scp and sftp and answers their password prompt from a stored
// password.
//
// It does this through OpenSSH's own SSH_ASKPASS hook: sshc starts the real
// client with itself registered as the askpass program, and the client calls
// back into sshc whenever it needs something typed. There is no sshpass or
// expect involved, the password never appears on a command line, and every
// option is handed to the real tool untouched.
package main

import (
	"fmt"
	"os"
)

const (
	prog    = "sshc"
	version = "0.1.1"
)

const usageText = `Usage: sshc [ssh] [ssh options] destination [command ...]
       sshc scp  [scp options] source ... target
       sshc sftp [sftp options] destination
       sshc rsync [rsync options] source ... target
       sshc ssh-copy-id [ssh-copy-id options] destination

Runs the tool and answers its prompt for a login password, or for the
passphrase of an SSH key, from what you have stored. Everything after the
optional tool name is handed to that tool unchanged.

  sshc set [password]           save a password or passphrase ("sshc set --help")
  sshc --check [tool] args...   show where the password would come from
  sshc --init                   create a config file template
  sshc --install [directory]    copy sshc to a per-user directory and put it
                                on your PATH
  sshc --help | --version

Password sources, first match wins:
  1. $SSHC_PASSWORD_<HOST>   one host, e.g. SSHC_PASSWORD_W_GO_2 for "w.go-2"
  2. [host <name>] section   in the config file
  3. $SSHC_PASSWORD          the active password, for the host(s) you name
  4. [profile <name>]        the active profile in the config file, selected
                             by $SSHC_PROFILE or the file's "profile =" line

Key passphrases work the same way: $SSHC_PASSPHRASE_<KEYFILE>, a [key <name>]
section, $SSHC_PASSPHRASE, then "passphrase =" in the active profile.

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

func run(args []string) int {
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
	switch args[0] {
	case "--help":
		fmt.Print(usageText)
		return 0
	case "--version":
		fmt.Println(prog, version)
		return 0
	case "--init":
		return cmdInit()
	case "--install":
		return cmdInstall(args[1:])
	case "--check":
		return cmdCheck(args[1:])
	case "set":
		return cmdSet(args[1:])
	}
	tool := "ssh"
	if isTool(args[0]) {
		tool, args = args[0], args[1:]
	}
	return runTool(tool, args)
}

func main() {
	os.Exit(run(os.Args[1:]))
}
