package main

import (
	"os"
	"os/exec"
	"os/signal"
)

// passthrough runs the tool as a child; Windows cannot replace a process.
func passthrough(tool string, args []string) int {
	return spawn(tool, args, os.Environ())
}

// relaySignals keeps sshc alive through Ctrl-C. The console already delivers
// it to the child, so there is nothing to forward.
func relaySignals(*os.Process) (stop func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	return func() { signal.Stop(ch) }
}

func exitStatus(ee *exec.ExitError) int {
	return ee.ExitCode()
}
