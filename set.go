package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"
)

const setUsage = `Usage: sshc set [--profile NAME | --host NAME] [--no-clear] [--] [password]

Saves a password in the config file. Without a password argument you are asked
to type it, hidden - which also keeps it out of your shell history.

  (no option)       the active profile (created as "default" if there is none)
  --profile NAME    that profile
  --host NAME       that host only; NAME as you type it for ssh, optionally
                    with "user@"
  --no-clear        do not clear the screen after a password given as argument
                    (or put "clear_on_set = no" in the config file)
`

// quoteValue writes a value so that parseConfig reads back exactly v.
func quoteValue(v string) string {
	needs := v != strings.TrimSpace(v)
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		needs = true
	}
	if needs {
		return `"` + v + `"`
	}
	return v
}

// setConfigValue returns lines with key set to value in section, changing
// nothing else: comments and other entries stay where they are.
func setConfigValue(lines []string, section, key, value string) []string {
	entry := key + " = " + quoteValue(value)
	target := configKey(section, "")
	current := configKey("", "")
	insertAt := -1 // just after the last entry of the target section
	if section == "" {
		insertAt = 0
	}
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		switch {
		case line == "" || line[0] == '#' || line[0] == ';':
			continue
		case line[0] == '[' && line[len(line)-1] == ']':
			current = configKey(line[1:len(line)-1], "")
		case current == target && strings.Contains(line, "="):
			k, _, _ := strings.Cut(line, "=")
			if strings.EqualFold(strings.TrimSpace(k), key) {
				out := append([]string{}, lines...)
				out[i] = entry
				return out
			}
		}
		if current == target {
			insertAt = i + 1
		}
	}
	if insertAt < 0 {
		out := append([]string{}, lines...)
		if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
			out = append(out, "")
		}
		return append(out, "["+section+"]", entry)
	}
	out := append([]string{}, lines[:insertAt]...)
	out = append(out, entry)
	return append(out, lines[insertAt:]...)
}

// writeConfig replaces path atomically, keeping it private.
func writeConfig(path string, lines []string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".sshc-conf-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil && !errors.Is(err, errors.ErrUnsupported) {
		tmp.Close()
		return err
	}
	_, err = tmp.WriteString(strings.Join(lines, "\n") + "\n")
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// readNewPassword gets the password without it being shown: typed twice on
// the terminal, or as one line on stdin when that is a pipe.
func readNewPassword() (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", errors.New("no password given")
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	in, out, err := openTTY()
	if err != nil {
		return "", err
	}
	defer in.Close()
	defer out.Close()
	ask := func(label string) (string, error) {
		fmt.Fprint(out, label)
		b, err := term.ReadPassword(int(in.Fd()))
		fmt.Fprintln(out)
		return string(b), err
	}
	first, err := ask("New password: ")
	if err != nil {
		return "", err
	}
	second, err := ask("Again: ")
	if err != nil {
		return "", err
	}
	if first != second {
		return "", errors.New("the two entries do not match; nothing was changed")
	}
	return first, nil
}

func isNo(v string) bool {
	switch strings.ToLower(v) {
	case "no", "false", "off", "0":
		return true
	}
	return false
}

// clearScreen wipes the visible screen and the scrollback, where a password
// typed as an argument would otherwise stay readable.
func clearScreen() {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return
	}
	enableVT(os.Stdout)
	fmt.Print("\x1b[H\x1b[2J\x1b[3J")
}

func cmdSet(args []string) int {
	var profile, host, password string
	havePassword, clear := false, true
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, val, attached := strings.Cut(arg, "=")
		switch {
		case arg == "--help":
			fmt.Print(setUsage)
			return 0
		case arg == "--no-clear":
			clear = false
		case name == "--profile" || name == "--host":
			if !attached {
				i++
				if i >= len(args) {
					warnf("%s needs a name", name)
					return 1
				}
				val = args[i]
			}
			if val == "" || strings.ContainsAny(val, "[]\n") {
				warnf("%q is not a usable name", val)
				return 1
			}
			if name == "--profile" {
				profile = val
			} else {
				host = val
			}
		case arg == "--" && i+2 == len(args):
			password, havePassword = args[i+1], true
			i++
		case strings.HasPrefix(arg, "-") && arg != "-" || havePassword:
			fmt.Fprint(os.Stderr, setUsage)
			return 1
		default:
			password, havePassword = arg, true
		}
	}
	if profile != "" && host != "" {
		warnf("use either --profile or --host, not both")
		return 1
	}
	fromArg := havePassword

	path, err := findConfig()
	if err == nil && path == "" {
		if path, err = defaultConfigTarget(); err == nil {
			err = createConfig(path)
		}
	}
	if err != nil {
		warnf("%v", err)
		return 1
	}
	cfg, err := loadConfig(path)
	if err != nil {
		warnf("%v", err)
		return 1
	}

	if !havePassword {
		if password, err = readNewPassword(); err != nil {
			warnf("%v", err)
			return 1
		}
	}
	if password == "" || strings.ContainsAny(password, "\r\n") {
		warnf("the password cannot be empty or contain a line break")
		return 1
	}

	data, err := os.ReadFile(path)
	if err != nil {
		warnf("%v", err)
		return 1
	}
	lines := strings.Split(strings.TrimRight(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n"), "\n")

	section := "host " + host
	if host == "" {
		active, _ := cfg.get("", "profile")
		if profile == "" {
			profile = os.Getenv("SSHC_PROFILE")
		}
		if profile == "" {
			profile = active
		}
		if profile == "" {
			profile = "default"
		}
		// The first profile ever saved becomes the active one.
		if active == "" {
			lines = setConfigValue(lines, "", "profile", profile)
		}
		section = "profile " + profile
	}
	lines = setConfigValue(lines, section, "password", password)
	if err := writeConfig(path, lines); err != nil {
		warnf("%v", err)
		return 1
	}

	if v, _ := cfg.get("", "clear_on_set"); fromArg && clear && !isNo(v) {
		clearScreen()
	}
	fmt.Println("Updated!")
	fmt.Printf("  [%s] in %s\n", section, path)
	if fromArg {
		fmt.Println("  Note: a password typed as an argument stays in your shell history.")
		fmt.Println("  Run \"sshc set\" with no password to type it hidden instead.")
	}
	if host == "" && os.Getenv("SSHC_PASSWORD") != "" {
		fmt.Println("  Note: SSHC_PASSWORD is set in this shell and takes precedence here.")
	}
	return 0
}
