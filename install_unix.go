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
	file, line := ".profile", `export PATH="`+shown+`:$PATH"`
	switch filepath.Base(os.Getenv("SHELL")) {
	case "zsh":
		file = ".zshrc"
	case "bash":
		file = ".bashrc"
		if runtime.GOOS == "darwin" {
			file = ".bash_profile"
		}
	case "fish":
		file, line = filepath.Join(".config", "fish", "config.fish"), `fish_add_path "`+shown+`"`
	}
	file = filepath.Join(home, file)

	if data, err := os.ReadFile(file); err == nil && strings.Contains(string(data), line) {
		return "already in " + file + ", which this terminal has not loaded", nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return "", err
	}
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	_, err = f.WriteString("\n" + profileMarker + "\n" + line + "\n")
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return "one line appended to " + file, err
}
