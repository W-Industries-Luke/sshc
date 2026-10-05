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
	envTool   = "SSHC_ASKPASS_TOOL"   // the tool sshc was asked to run
	envNoTTY  = "SSHC_ASKPASS_NOTTY"  // set when nobody can be asked on the terminal
	envHook   = "SSHC_HOOK"           // set by the shell hook, to the shell it runs in

	// envNoPrompt is for the user to set: never ask on the terminal, fail
	// instead. Meant for scripts and scheduled jobs.
	envNoPrompt = "SSHC_NO_PROMPT"
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

// ssh-add words its prompt differently from ssh. Like the one above it is
// written by a local program and cannot come from a server.
var reAddPassphrase = regexp.MustCompile(`^Enter passphrase for (.+?)(?: \(will confirm each use\))?: ?$`)

// A keyboard-interactive prompt that is not for the password: possibly a
// one-time code. The text after "(user@host) " is the server's, so a code is
// only supplied to a host that has one configured, and only when the prompt
// reads like a request for one.
var (
	reKbdAny = regexp.MustCompile(`^\((\S+)@([^@)\s]+)\) (.+)$`)
	reOTP    = regexp.MustCompile(`(?i)verification code|one[- ]time|\botp\b|passcode|authenticat|\btoken\b|2fa|two[- ]factor|security code`)
)

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
	// Several connections at once ("sshc each") cannot share the terminal,
	// and a script can ask never to be prompted.
	if os.Getenv(envNoTTY) != "" || os.Getenv(envNoPrompt) != "" {
		return 1
	}
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
	m := rePassphrase.FindStringSubmatch(prompt)
	if m == nil {
		m = reAddPassphrase.FindStringSubmatch(prompt)
	}
	if m != nil {
		what, id = "passphrase for key "+m[1], "key."+m[1]
		find = func(r *resolver) (string, string, string) { return r.lookupPassphrase(m[1]) }
		debugf("prompt is for the passphrase of key %q", m[1])
	} else if user, host, ok := parsePrompt(prompt); ok {
		what, id = "password for "+user+"@"+host, "host."+host
		find = func(r *resolver) (string, string, string) { return r.lookup(user, host) }
		debugf("prompt is for user %q at host %q; destinations: %q", user, host, os.Getenv(envDests))
	} else if k := reKbdAny.FindStringSubmatch(prompt); k != nil && reOTP.MatchString(k[3]) {
		what, id = "one-time code for "+k[1]+"@"+k[2], "otp."+k[2]
		find = func(r *resolver) (string, string, string) { return r.lookupOTP(k[1], k[2]) }
		debugf("prompt asks %q at %q for a one-time code", k[1], k[2])
	} else {
		debugf("not a prompt sshc answers; asking on the terminal")
		// ssh-add's second ask has its own wording.
		if strings.HasPrefix(prompt, "Bad passphrase, try again") {
			warnf("the stored passphrase was not accepted")
		}
		// git asks for an https user name through the same hook; that one
		// should be visible while it is typed.
		return askTTY(prompt, strings.HasPrefix(strings.ToLower(prompt), "username"))
	}

	// A stored secret is offered once per connection. If ssh asks again it
	// was rejected, and repeating it would only burn login attempts.
	mark := filepath.Join(os.Getenv(envState), fmt.Sprintf("%d.%s", os.Getppid(), reUnsafeName.ReplaceAllString(id, "_")))
	if prev, err := os.ReadFile(mark); err == nil {
		// Say so once; later asks are retries of what the human typed.
		if len(prev) > 0 {
			warnf("the stored %s (from %s) was not accepted", what, strings.TrimSpace(string(prev)))
			if cfg, err := loadConfig(os.Getenv(envConfig)); err == nil {
				auditLog(cfg, "rejected", what, strings.TrimSpace(string(prev)))
			}
			_ = os.WriteFile(mark, nil, 0o600)
		}
		return askTTY(prompt, false)
	}

	cfg, err := loadConfig(os.Getenv(envConfig))
	if err != nil {
		warnf("%v", err)
	}
	// Locked after this connection was started, or by the inactivity limit.
	if isLocked(cfg) {
		debugf("sshc is locked; asking on the terminal")
		u := newUI(os.Stderr)
		u.say("Note: " + lockedNotice)
		u.flush()
		return askTTY(prompt, false)
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
	auditLog(cfg, "supplied", what, from)
	touchActivity()
	fmt.Println(secret)
	return 0
}
