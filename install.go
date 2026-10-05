package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func exeName() string {
	if runtime.GOOS == "windows" {
		return prog + ".exe"
	}
	return prog
}

func sameFile(a, b string) bool {
	fa, errA := os.Stat(a)
	fb, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(fa, fb)
}

// copyFile writes src to dst through a temporary file, so a failed copy never
// leaves a half-written dst.
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".sshc-install-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = io.Copy(tmp, in)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), mode)
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}

// onPath reports whether dir is already one of the PATH entries.
func onPath(dir string) bool {
	want := filepath.Clean(dir)
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if p == "" {
			continue
		}
		p = filepath.Clean(p)
		if p == want || runtime.GOOS == "windows" && strings.EqualFold(p, want) {
			return true
		}
	}
	return false
}

// cmdInstall copies the running executable to a per-user directory and puts
// that directory on the user's PATH. It needs no administrator rights.
func cmdInstall(args []string) int {
	if len(args) > 1 || len(args) == 1 && strings.HasPrefix(args[0], "-") {
		warnf("usage: %s --install [directory]", prog)
		return 1
	}
	var dir string
	var err error
	if len(args) == 1 {
		dir, err = filepath.Abs(args[0])
	} else {
		dir, err = defaultInstallDir()
	}
	if err != nil {
		warnf("%v", err)
		return 1
	}
	self, err := os.Executable()
	if err != nil {
		warnf("could not locate my own executable: %v", err)
		return 1
	}
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		warnf("%v", err)
		return 1
	}

	target := filepath.Join(dir, exeName())
	if sameFile(self, target) {
		fmt.Printf("%s is already installed at %s\n", prog, target)
	} else {
		if err := copyFile(self, target, 0o755); err != nil {
			warnf("could not install to %s: %v", target, err)
			return 1
		}
		fmt.Printf("Installed %s %s to %s\n", prog, version, target)

		// The config file is looked up next to the executable, so one that
		// sits beside this copy has to come along or it would stop applying.
		oldCfg := filepath.Join(filepath.Dir(self), configName)
		newCfg := filepath.Join(dir, configName)
		switch {
		case !exists(oldCfg):
		case exists(newCfg):
			fmt.Printf("Kept the config file already at %s\n  (the one at %s is not used by the installed copy)\n", newCfg, oldCfg)
		default:
			err := os.Rename(oldCfg, newCfg)
			if err != nil {
				if err = copyFile(oldCfg, newCfg, 0o600); err == nil {
					err = os.Remove(oldCfg)
				}
			}
			if err != nil {
				warnf("could not move %s to %s: %v", oldCfg, newCfg, err)
			} else {
				fmt.Printf("Moved your config file to %s\n", newCfg)
			}
		}
	}

	if onPath(dir) {
		fmt.Printf("%s is already on your PATH.\n", dir)
		return 0
	}
	changed, err := addToUserPath(dir)
	if err != nil {
		warnf("could not add %s to your PATH: %v", dir, err)
		fmt.Printf("Add it yourself, then open a new terminal.\n")
		return 1
	}
	fmt.Printf("Added %s to your PATH (%s).\n", dir, changed)
	fmt.Printf("Open a new terminal, then run: %s --version\n", prog)
	return 0
}
