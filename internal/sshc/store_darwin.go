package sshc

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// The macOS login keychain, through the system's own "security" tool. Entries
// are generic passwords with service "sshc".
//
// Both the account name and the secret are stored base64url-encoded. That
// keeps every word free of quoting characters, so that a new secret can be
// handed to "security -i" on its standard input rather than on a command
// line, where other users could see it in the process list.

const (
	securityTool   = "/usr/bin/security"
	keychainSvc    = prog
	keychainPrefix = "b64."
)

type keychain struct{}

func platformStore() (secretStore, error) {
	if _, err := exec.LookPath(securityTool); err != nil {
		return nil, err
	}
	return keychain{}, nil
}

func (keychain) name() string { return "the macOS Keychain" }

func account(key string) string {
	return keychainPrefix + base64.RawURLEncoding.EncodeToString([]byte(key))
}

func (k keychain) set(key, value string) error {
	enc := keychainPrefix + base64.RawURLEncoding.EncodeToString([]byte(value))
	cmd := exec.Command(securityTool, "-i")
	cmd.Stdin = strings.NewReader(fmt.Sprintf("add-generic-password -U -s %s -a %s -w %s\n", keychainSvc, account(key), enc))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, bytes.TrimSpace(out))
	}
	// "security -i" reports success even when the command inside it failed.
	if got, err := k.get(key); err != nil || got != value {
		return fmt.Errorf("the keychain did not accept the entry: %s", bytes.TrimSpace(out))
	}
	return nil
}

func (keychain) get(key string) (string, error) {
	cmd := exec.Command(securityTool, "find-generic-password", "-s", keychainSvc, "-a", account(key), "-w")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 44 {
			return "", errNotFound
		}
		return "", fmt.Errorf("%v: %s", err, bytes.TrimSpace(stderr.Bytes()))
	}
	enc := strings.TrimSpace(string(out))
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(enc, keychainPrefix))
	if err != nil || !strings.HasPrefix(enc, keychainPrefix) {
		return "", errors.New("the keychain entry is not in sshc's format")
	}
	return string(raw), nil
}

func (keychain) del(key string) error {
	out, err := exec.Command(securityTool, "delete-generic-password", "-s", keychainSvc, "-a", account(key)).CombinedOutput()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 44 {
			return errNotFound
		}
		return fmt.Errorf("%v: %s", err, bytes.TrimSpace(out))
	}
	return nil
}
