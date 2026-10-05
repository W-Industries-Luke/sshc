//go:build !windows

package main

import "os"

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
