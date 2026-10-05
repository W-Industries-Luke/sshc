//go:build !windows

package sshc

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// --- terminal ---------------------------------------------------------------

// enableVT is only needed on Windows.
func enableVT(*os.File) {}

// openTTY opens the controlling terminal for reading and writing.
func openTTY() (in, out *os.File, err error) {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, err
	}
	return f, f, nil
}

// --- config file permissions ------------------------------------------------

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

// --- running the tool -------------------------------------------------------

// passthrough replaces sshc with the tool itself.
func passthrough(tool string, args []string) int {
	path, err := exec.LookPath(tool)
	if err != nil {
		warnf("%v", err)
		return 127
	}
	err = syscall.Exec(path, append([]string{tool}, args...), os.Environ())
	warnf("%s: %v", path, err)
	return 126
}

// relaySignals passes termination signals on to the child and keeps sshc
// alive until the child has exited, so the scratch directory is always
// removed.
func relaySignals(child *os.Process) (stop func()) {
	ch := make(chan os.Signal, 4)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	go func() {
		for s := range ch {
			_ = child.Signal(s)
		}
	}()
	return func() {
		signal.Stop(ch)
		close(ch)
	}
}

func exitStatus(ee *exec.ExitError) int {
	if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return ee.ExitCode()
}
