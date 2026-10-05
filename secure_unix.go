//go:build !windows

package main

import (
	"fmt"
	"os"
	"syscall"
)

// checkConfigSecure refuses a config file that other users could read or that
// someone else owns, the same way ssh treats private keys.
func checkConfigSecure(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("config file %s is not a regular file", path)
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Geteuid() {
		return fmt.Errorf("config file %s is not owned by you; refusing to use it", path)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("config file %s is accessible by other users; run: chmod 600 '%s'", path, path)
	}
	return nil
}
