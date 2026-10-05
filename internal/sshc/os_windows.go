package sshc

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"

	"golang.org/x/sys/windows"
)

// --- terminal ---------------------------------------------------------------

// enableVT makes the console interpret ANSI escape sequences, which older
// console hosts do not do by default.
func enableVT(f *os.File) {
	h := windows.Handle(f.Fd())
	var mode uint32
	if windows.GetConsoleMode(h, &mode) == nil {
		_ = windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
	}
}

// openTTY opens the console for reading and writing.
func openTTY() (in, out *os.File, err error) {
	in, err = os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, err
	}
	out, err = os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err != nil {
		in.Close()
		return nil, nil, err
	}
	return in, out, nil
}

// --- config file permissions ------------------------------------------------

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

// --- running the tool -------------------------------------------------------

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
