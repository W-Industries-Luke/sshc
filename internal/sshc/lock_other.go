//go:build !windows && !darwin

package sshc

import (
	"errors"
	"os"
	"os/exec"
	"os/user"
)

const verifyMethod = "for your account password"

func canVerifyUser() error {
	// The system never asks root for a password, so a lock would open for
	// anyone at the keyboard.
	if os.Geteuid() == 0 {
		return errors.New("you are root, and the system does not ask root to prove who it is")
	}
	if _, err := exec.LookPath("su"); err != nil {
		return errors.New("su, the tool that checks your account password, was not found")
	}
	return nil
}

// verifyUser has the system check the account password, by switching to the
// same user with su, which asks for it on the terminal.
func verifyUser() error {
	me, err := user.Current()
	if err != nil {
		return err
	}
	cmd := exec.Command("su", "-c", "true", me.Username)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	lastBlank = false
	if err := cmd.Run(); err != nil {
		return errors.New("your account password was not accepted")
	}
	return nil
}
