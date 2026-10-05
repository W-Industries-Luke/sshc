package sshc

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/term"
)

// Environment handed from the parent sshc, through ssh, to the askpass call.
const (
	envState  = "SSHC_ASKPASS_STATE"  // private scratch directory; also marks askpass mode
	envDests  = "SSHC_ASKPASS_DESTS"  // destinations named on the command line
	envConfig = "SSHC_ASKPASS_CONFIG" // config file in use, may be empty
)

// Only two password prompt shapes are answered automatically, and in both the
// user@host is written by the local ssh client, not by the server:
//
//	user@host's password:        password authentication
//	(user@host) Password:        keyboard-interactive (OpenSSH >= 8.5)
//
// The user part may not contain whitespace. That matters: the text after
// "(user@host) " in a keyboard-interactive prompt is chosen by the server, and
// without it a hostile server could send "x@otherhost's password:" and be
// handed the password of a different host.
var (
	rePassword = regexp.MustCompile(`(?i)^([^\s(]\S*)@([^@\s]+)'s password: ?$`)
	reKbdInt   = regexp.MustCompile(`(?i)^\((\S+)@([^@)\s]+)\) password(?: for \S+)?: ?$`)
)

// The prompt for an encrypted private key. It is produced entirely by the
// local client and names a local file; a server cannot send it, because
// anything a server sends arrives behind the "(user@host) " prefix.
var rePassphrase = regexp.MustCompile(`^Enter passphrase for key '(.+)': ?$`)

var reUnsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]`)

func parsePrompt(prompt string) (user, host string, ok bool) {
	for _, re := range []*regexp.Regexp{rePassword, reKbdInt} {
		if m := re.FindStringSubmatch(prompt); m != nil {
			return m[1], m[2], true
		}
	}
	return "", "", false
}

// isAskpassCall reports whether ssh is calling us back for an answer. ssh
// passes the prompt as a single argument, and every prompt contains
// whitespace, which no destination or lone option does - so an sshc started
// by hand from inside an ssh session (say in a ProxyCommand) is not mistaken
// for a callback.
func isAskpassCall(args []string) bool {
	return os.Getenv(envState) != "" && len(args) == 1 && strings.ContainsAny(args[0], " \t\n")
}

// askTTY puts the prompt to the human on the terminal and relays the answer
// to ssh. Without a terminal it fails, and ssh treats that as no answer.
func askTTY(prompt string, echo bool) int {
	in, out, err := openTTY()
	if err != nil {
		return 1
	}
	defer in.Close()
	defer out.Close()
	fmt.Fprint(out, prompt)
	var answer string
	if echo {
		line, err := bufio.NewReader(in).ReadString('\n')
		if err != nil && line == "" {
			return 1
		}
		answer = strings.TrimRight(line, "\r\n")
	} else {
		b, err := term.ReadPassword(int(in.Fd()))
		fmt.Fprintln(out)
		if err != nil {
			return 1
		}
		answer = string(b)
	}
	fmt.Println(answer)
	return 0
}

func askpassMain(prompt string) int {
	debugf("askpass called by pid %d with prompt %q", os.Getppid(), prompt)
	// Host key confirmations and the like are never answered automatically.
	hint := os.Getenv("SSH_ASKPASS_PROMPT")
	if hint == "confirm" || strings.Contains(prompt, "(yes/no") {
		return askTTY(prompt, true)
	}
	if hint == "none" {
		return 0
	}

	// Work out what is being asked for. One-time codes, password changes and
	// anything else unrecognised are not ours to answer.
	var what, id string
	var find func(*resolver) (secret, from, note string)
	if m := rePassphrase.FindStringSubmatch(prompt); m != nil {
		what, id = "passphrase for key "+m[1], "key."+m[1]
		find = func(r *resolver) (string, string, string) { return r.lookupPassphrase(m[1]) }
		debugf("prompt is for the passphrase of key %q", m[1])
	} else if user, host, ok := parsePrompt(prompt); ok {
		what, id = "password for "+user+"@"+host, "host."+host
		find = func(r *resolver) (string, string, string) { return r.lookup(user, host) }
		debugf("prompt is for user %q at host %q; destinations: %q", user, host, os.Getenv(envDests))
	} else {
		debugf("not a prompt sshc answers; asking on the terminal")
		return askTTY(prompt, false)
	}

	// A stored secret is offered once per connection. If ssh asks again it
	// was rejected, and repeating it would only burn login attempts.
	mark := filepath.Join(os.Getenv(envState), fmt.Sprintf("%d.%s", os.Getppid(), reUnsafeName.ReplaceAllString(id, "_")))
	if prev, err := os.ReadFile(mark); err == nil {
		// Say so once; later asks are retries of what the human typed.
		if len(prev) > 0 {
			warnf("the stored %s (from %s) was not accepted", what, strings.TrimSpace(string(prev)))
			_ = os.WriteFile(mark, nil, 0o600)
		}
		return askTTY(prompt, false)
	}

	cfg, err := loadConfig(os.Getenv(envConfig))
	if err != nil {
		warnf("%v", err)
	}
	secret, from, note := find(newResolver(cfg, decodeDests(os.Getenv(envDests))))
	if note != "" {
		warnf("%s", note)
	}
	if secret == "" {
		debugf("nothing stored matches; asking on the terminal")
		return askTTY(prompt, false)
	}
	debugf("answering from %s", from)
	if err := os.WriteFile(mark, []byte(from+"\n"), 0o600); err != nil {
		// Without the marker we could not tell a retry from a first ask.
		warnf("%v", err)
		return askTTY(prompt, false)
	}
	fmt.Println(secret)
	return 0
}
