package sshc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// The freedesktop Secret Service (GNOME Keyring, KWallet, KeePassXC, ...),
// through libsecret's "secret-tool". Entries have the attributes
// service=sshc and account=<key>. A new secret is passed on standard input.
//
// Servers and containers usually have no secret service running; sshc then
// falls back to the config file.

type secretService struct{}

func (secretService) name() string { return "the system keyring (Secret Service)" }

// secretTool runs secret-tool, giving up if no keyring answers.
func secretTool(stdin string, args ...string) (stdout string, stderr string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "secret-tool", args...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err = cmd.Run()
	if ctx.Err() != nil {
		err = errors.New("timed out waiting for the keyring")
	}
	return out.String(), strings.TrimSpace(errb.String()), err
}

func platformStore() (secretStore, error) {
	if _, err := exec.LookPath("secret-tool"); err != nil {
		return nil, errors.New("secret-tool (from libsecret) is not installed")
	}
	// A lookup that merely finds nothing is silent; one that cannot reach a
	// secret service complains.
	if _, stderr, err := secretTool("", "lookup", "service", prog, "account", "sshc-probe"); stderr != "" {
		return nil, fmt.Errorf("no secret service is reachable (%s)", firstLine(stderr))
	} else if err != nil && err.Error() == "timed out waiting for the keyring" {
		return nil, err
	}
	return secretService{}, nil
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

func (secretService) set(key, value string) error {
	_, stderr, err := secretTool(value, "store", "--label", "sshc: "+key, "service", prog, "account", key)
	if err != nil {
		return fmt.Errorf("%v: %s", err, firstLine(stderr))
	}
	return nil
}

func (secretService) get(key string) (string, error) {
	out, stderr, err := secretTool("", "lookup", "service", prog, "account", key)
	if err != nil {
		if stderr == "" {
			return "", errNotFound
		}
		return "", fmt.Errorf("%v: %s", err, firstLine(stderr))
	}
	return strings.TrimSuffix(out, "\n"), nil
}

func (s secretService) del(key string) error {
	if _, err := s.get(key); err != nil {
		return err
	}
	_, stderr, err := secretTool("", "clear", "service", prog, "account", key)
	if err != nil {
		return fmt.Errorf("%v: %s", err, firstLine(stderr))
	}
	return nil
}
