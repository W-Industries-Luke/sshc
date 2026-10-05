package sshc

import (
	"errors"
	"os"
	"os/exec"
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

// powershellValue runs one expression in the given PowerShell and returns
// what it prints.
func powershellValue(exe, expr string) string {
	out, err := exec.Command(exe, "-NoProfile", "-NonInteractive", "-Command", expr).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// installShellHook adds the hook to the PowerShell profile - of Windows
// PowerShell and of PowerShell 7, where each is installed - provided that
// PowerShell will load the profile. Under a restrictive execution policy a
// profile only produces an error in every new window, so in that case the
// step is explained and left to the user.
func installShellHook() string {
	line := hookLine("powershell")
	manual := "To use \"sshc set -s\", \"sshc use\" and Tab completion, load the shell hook from your PowerShell profile:\n" +
		"    if (!(Test-Path $PROFILE)) { New-Item -Force -ItemType File $PROFILE | Out-Null }\n" +
		"    Add-Content $PROFILE '" + line + "'\n" +
		"If new windows then report that running scripts is disabled, allow your own profile with:\n" +
		"    Set-ExecutionPolicy -Scope CurrentUser RemoteSigned"
	var done, skipped []string
	for _, exe := range []string{"powershell", "pwsh"} {
		if _, err := exec.LookPath(exe); err != nil {
			continue
		}
		switch strings.ToLower(powershellValue(exe, "Get-ExecutionPolicy")) {
		case "remotesigned", "unrestricted", "bypass":
		default:
			skipped = append(skipped, exe)
			continue
		}
		profile := powershellValue(exe, "$PROFILE")
		if profile == "" {
			skipped = append(skipped, exe)
			continue
		}
		if data, err := os.ReadFile(profile); err == nil && strings.Contains(string(data), line) {
			done = append(done, "already in "+profile)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(profile), 0o755); err != nil {
			skipped = append(skipped, exe)
			continue
		}
		f, err := os.OpenFile(profile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			skipped = append(skipped, exe)
			continue
		}
		_, err = f.WriteString("\r\n# Added by sshc --install\r\n" + line + "\r\n")
		f.Close()
		if err != nil {
			skipped = append(skipped, exe)
			continue
		}
		done = append(done, "one line appended to "+profile)
	}
	if len(done) == 0 {
		return "Optional: " + manual
	}
	msg := "Added the shell hook (" + strings.Join(done, "; ") + ")."
	if len(skipped) > 0 {
		msg += "\nNote: " + strings.Join(skipped, " and ") + " does not allow profile scripts, so it was left alone. " + manual
	}
	return msg
}
