package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

func defaultInstallDir() (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, "AppData", "Local")
	}
	return filepath.Join(base, "Programs", prog), nil
}

// addToUserPath appends dir to the per-user Path in the registry, which is
// where Windows keeps it, and tells running programs that it changed. The
// existing value is only ever appended to, and is left untouched if it cannot
// be read.
func addToUserPath(dir string) (changed string, err error) {
	if strings.Contains(dir, ";") {
		return "", errors.New("the directory name contains a semicolon")
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, "Environment", registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return "", err
	}
	defer key.Close()

	current, valType, err := key.GetStringValue("Path")
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return "", err
	}
	for _, p := range strings.Split(current, ";") {
		if strings.EqualFold(strings.TrimRight(p, `\`), strings.TrimRight(dir, `\`)) {
			return "already in your user Path, which this terminal has not loaded", nil
		}
	}
	updated := dir
	if current != "" {
		updated = strings.TrimRight(current, ";") + ";" + dir
	}
	// Keep the value's type: an expandable Path may hold %VARIABLES%.
	if valType == registry.SZ {
		err = key.SetStringValue("Path", updated)
	} else {
		err = key.SetExpandStringValue("Path", updated)
	}
	if err != nil {
		return "", err
	}

	// WM_SETTINGCHANGE lets Explorer hand the new Path to terminals opened
	// from now on.
	const hwndBroadcast, wmSettingChange, smtoAbortIfHung = 0xffff, 0x001A, 0x0002
	env, _ := syscall.UTF16PtrFromString("Environment")
	syscall.NewLazyDLL("user32.dll").NewProc("SendMessageTimeoutW").Call(
		hwndBroadcast, wmSettingChange, 0, uintptr(unsafe.Pointer(env)), smtoAbortIfHung, 5000, 0)
	return "your user Path setting", nil
}
