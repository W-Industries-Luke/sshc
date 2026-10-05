package sshc

import (
	"errors"
	"os"
	"os/exec"
	"os/user"
)

const verifyMethod = "for your macOS account password"

func canVerifyUser() error {
	if os.Geteuid() == 0 {
		return errors.New("you are root, and the system does not ask root to prove who it is")
	}
	if _, err := exec.LookPath("dscl"); err != nil {
		return errors.New("dscl, the tool that checks your account password, was not found")
	}
	return nil
}

// verifyUser has Directory Services check the account password, which it
// asks for on the terminal. (Touch ID is not used: it cannot be reached from
// a command-line program without extra components.)
func verifyUser() error {
	me, err := user.Current()
	if err != nil {
		return err
	}
	cmd := exec.Command("dscl", ".", "-authonly", me.Username)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	lastBlank = false
	if err := cmd.Run(); err != nil {
		return errors.New("your account password was not accepted")
	}
	return nil
}
