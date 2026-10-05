package main

import (
	"fmt"
	"os"
)

// checkConfigSecure only checks the file type on Windows. Access there is
// governed by ACLs rather than mode bits, and files under the user profile
// are private to the user by default.
func checkConfigSecure(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("config file %s is not a regular file", path)
	}
	return nil
}
