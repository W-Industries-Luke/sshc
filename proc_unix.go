//go:build !windows

package main

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

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
