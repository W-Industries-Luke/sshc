package sshc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// A config entry may name a command instead of holding a secret:
//
//	[profile work]
//	password_command = op read "op://Work/server/password"
//
// sshc runs it when the secret is needed and uses the first line it prints,
// so a password manager stays the only place the secret lives.

// commandTimeout leaves room for a password manager to ask for a fingerprint
// or a master password.
const commandTimeout = 2 * time.Minute

// splitCommand cuts a command line into words. Single and double quotes group
// a word; inside double quotes \" is a literal quote. Nothing else is special
// - no variables, no globbing, and a backslash is an ordinary character, so
// Windows paths need no doubling. The command is run directly, not by a shell.
func splitCommand(s string) ([]string, error) {
	var words []string
	var cur strings.Builder
	inWord := false
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			} else if c == '\\' && quote == '"' && i+1 < len(s) && s[i+1] == '"' {
				cur.WriteByte('"')
				i++
			} else {
				cur.WriteByte(c)
			}
		case c == '\'' || c == '"':
			quote, inWord = c, true
		case c == ' ' || c == '\t':
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteByte(c)
			inWord = true
		}
	}
	if quote != 0 {
		return nil, errors.New("unterminated quote")
	}
	if inWord {
		words = append(words, cur.String())
	}
	if len(words) == 0 {
		return nil, errors.New("empty command")
	}
	return words, nil
}

// expandVars replaces %h, %u and %k (host, user, key file) in one word. It is
// applied after the command has been split, so a value can never add words.
func expandVars(word string, vars map[byte]string) string {
	var b strings.Builder
	for i := 0; i < len(word); i++ {
		if word[i] == '%' && i+1 < len(word) {
			if word[i+1] == '%' {
				b.WriteByte('%')
				i++
				continue
			}
			if v, ok := vars[word[i+1]]; ok {
				b.WriteString(v)
				i++
				continue
			}
		}
		b.WriteByte(word[i])
	}
	return b.String()
}

// runSecretCommand runs a configured command and returns the first line of
// its output.
func runSecretCommand(command string, vars map[byte]string) (string, error) {
	argv, err := splitCommand(command)
	if err != nil {
		return "", err
	}
	for i := range argv {
		argv[i] = expandVars(argv[i], vars)
	}
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return "", errors.New("timed out")
	}
	if err != nil {
		return "", err
	}
	line, _, _ := bytes.Cut(out, []byte("\n"))
	secret := strings.TrimRight(string(line), "\r")
	if secret == "" {
		return "", fmt.Errorf("%s printed nothing", argv[0])
	}
	return secret, nil
}
