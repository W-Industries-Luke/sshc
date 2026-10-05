package main

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

// Only two prompt shapes are answered automatically, and in both the
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
	// Host key confirmations and the like are never answered automatically.
	hint := os.Getenv("SSH_ASKPASS_PROMPT")
	if hint == "confirm" || strings.Contains(prompt, "(yes/no") {
		return askTTY(prompt, true)
	}
	if hint == "none" {
		return 0
	}

	// Key passphrases, one-time codes, password changes: not ours to answer.
	user, host, ok := parsePrompt(prompt)
	if !ok {
		return askTTY(prompt, false)
	}

	// A stored password is offered once per connection. If ssh asks again it
	// was rejected, and repeating it would only burn login attempts.
	safeHost := regexp.MustCompile(`[^A-Za-z0-9._-]`).ReplaceAllString(host, "_")
	mark := filepath.Join(os.Getenv(envState), fmt.Sprintf("%d.%s", os.Getppid(), safeHost))
	if prev, err := os.ReadFile(mark); err == nil {
		// Say so once; later asks are retries of what the human typed.
		if len(prev) > 0 {
			warnf("the stored password for %s@%s (from %s) was not accepted", user, host, strings.TrimSpace(string(prev)))
			_ = os.WriteFile(mark, nil, 0o600)
		}
		return askTTY(prompt, false)
	}

	cfg, err := loadConfig(os.Getenv(envConfig))
	if err != nil {
		warnf("%v", err)
	}
	password, from, note := newResolver(cfg, decodeDests(os.Getenv(envDests))).lookup(user, host)
	if note != "" {
		warnf("%s", note)
	}
	if password == "" {
		return askTTY(prompt, false)
	}
	if err := os.WriteFile(mark, []byte(from+"\n"), 0o600); err != nil {
		// Without the marker we could not tell a retry from a first ask.
		warnf("%v", err)
		return askTTY(prompt, false)
	}
	fmt.Println(password)
	return 0
}
