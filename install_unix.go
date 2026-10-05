//go:build !windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const profileMarker = "# Added by sshc --install"

func defaultInstallDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "bin"), nil
}

// startupFile is the file the user's shell reads when it starts, and the
// syntax family of that shell.
func startupFile() (file, kind string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	file, kind = ".profile", "posix"
	switch filepath.Base(os.Getenv("SHELL")) {
	case "zsh":
		file = ".zshrc"
	case "bash":
		file = ".bashrc"
		if runtime.GOOS == "darwin" {
			file = ".bash_profile"
		}
	case "fish":
		file, kind = filepath.Join(".config", "fish", "config.fish"), "fish"
	}
	return filepath.Join(home, file), kind, nil
}

// appendLine adds line to file under a marker comment, unless it is there
// already.
func appendLine(file, line string) (added bool, err error) {
	if data, err := os.ReadFile(file); err == nil && strings.Contains(string(data), line) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return false, err
	}
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return false, err
	}
	_, err = f.WriteString("\n" + profileMarker + "\n" + line + "\n")
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err == nil, err
}

// addToUserPath appends a PATH line to the startup file of the user's shell
// and says which file it changed.
func addToUserPath(dir string) (changed string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if strings.ContainsAny(dir, "\"'`$\\\n") {
		return "", errors.New("the directory name has characters that are unsafe in a shell startup file")
	}
	// Written relative to $HOME so the line survives a moved home directory.
	shown := dir
	if rel, err := filepath.Rel(home, dir); err == nil && !strings.HasPrefix(rel, "..") {
		shown = "$HOME/" + rel
	}
	file, kind, err := startupFile()
	if err != nil {
		return "", err
	}
	line := `export PATH="` + shown + `:$PATH"`
	if kind == "fish" {
		line = `fish_add_path "` + shown + `"`
	}
	added, err := appendLine(file, line)
	if err != nil {
		return "", err
	}
	if !added {
		return "already in " + file + ", which this terminal has not loaded", nil
	}
	return "one line appended to " + file, nil
}

// installShellHook makes the shell load the hook that "sshc set --session"
// needs, and reports what it did.
func installShellHook() string {
	file, kind, err := startupFile()
	if err == nil {
		var added bool
		if added, err = appendLine(file, hookLine(kind)); err == nil {
			if !added {
				return "The shell hook for \"sshc set --session\" is already in " + file + "."
			}
			return "Added the shell hook for \"sshc set --session\" (one line appended to " + file + ")."
		}
	}
	return "Could not set up the shell hook for \"sshc set --session\": " + err.Error() +
		"\n  Add this line to your shell's startup file yourself:\n    " + hookLine(kind)
}
