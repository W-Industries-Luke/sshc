package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/term"
)

const setUsage = `Usage: sshc set [options] [--] [value]

Saves a login password, or the passphrase of an SSH key. Without a value you are asked to type it, hidden - which also keeps it out of
your shell history.

Where to save it:
  (no option)       the config file: kept, and used in every terminal
  --session         this terminal only: sets the SSHC_* environment variable,
                    nothing is written to disk and it is gone when the
                    terminal closes. Needs the shell hook - "sshc --install"
                    sets it up, or see "sshc --shell-init".

What to save:
  (no option)       the login password of the active profile
  --passphrase      the key passphrase of the active profile, tried for any key
  --host NAME       the login password of one host; NAME as you type it for
                    ssh, optionally with "user@"
  --key NAME        the passphrase of one key; NAME is the key's file name
                    (id_ed25519) or its full path

Other options:
  --profile NAME    use that profile instead of the active one
  --no-clear        do not clear the screen after a value given as argument
                    (or put "clear_on_set = no" in the config file)

The active profile is created as "default" if there is none yet.
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
func readNewPassword(what string) (string, error) {
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
	first, err := ask("New " + what + ": ")
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
func clearScreen(w *os.File) {
	if !term.IsTerminal(int(w.Fd())) {
		return
	}
	enableVT(w)
	fmt.Fprint(w, "\x1b[H\x1b[2J\x1b[3J")
}

func cmdSet(args []string) int {
	var profile, host, key, password string
	havePassword, clear, passphrase, session := false, true, false, false
	// Under the shell hook stdout is evaluated by the shell, so everything
	// meant for the human goes to stderr.
	emit := os.Getenv(envEmit)
	out := os.Stdout
	if emit != "" {
		out = os.Stderr
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, val, attached := strings.Cut(arg, "=")
		switch {
		case arg == "--help":
			fmt.Fprint(out, setUsage)
			return 0
		case arg == "--session":
			session = true
		case arg == "--no-clear":
			clear = false
		case arg == "--passphrase":
			passphrase = true
		case name == "--profile" || name == "--host" || name == "--key":
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
			switch name {
			case "--profile":
				profile = val
			case "--host":
				host = val
			default:
				key = val
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
	chosen := 0
	for _, on := range []bool{host != "", key != "", profile != "" || passphrase} {
		if on {
			chosen++
		}
	}
	if chosen > 1 {
		warnf("--host, --key and --profile/--passphrase save to different places; use one")
		return 1
	}
	what := "password"
	if key != "" || passphrase {
		what = "passphrase"
	}
	fromArg := havePassword

	if session {
		if profile != "" {
			warnf("--session sets a variable in this terminal; it cannot be combined with --profile")
			return 1
		}
		name := "SSHC_" + strings.ToUpper(what)
		switch {
		case host != "":
			name = hostEnvPrefix + strings.ToUpper(envSuffix(host))
		case key != "":
			name = keyEnvPrefix + strings.ToUpper(envSuffix(path.Base(normKeyPath(key))))
		}
		if _, ok := emitAssignment(emit, name, ""); !ok {
			warnf("--session needs the sshc shell hook, which is not loaded in this terminal.")
			fmt.Fprintf(os.Stderr, "  Add this line to your shell's startup file, then open a new terminal:\n    %s\n", hookLine(defaultShellKind()))
			return 1
		}
		if !havePassword {
			// A shell runs a piped-into function in a subshell, where the
			// variable would be set and immediately lost.
			if !term.IsTerminal(int(os.Stdin.Fd())) {
				warnf("with --session, type the %s when asked or give it as an argument; it cannot be piped in", what)
				return 1
			}
			var err error
			if password, err = readNewPassword(what); err != nil {
				warnf("%v", err)
				return 1
			}
		}
		if password == "" || strings.ContainsAny(password, "\r\n") {
			warnf("the %s cannot be empty or contain a line break", what)
			return 1
		}
		code, _ := emitAssignment(emit, name, password)
		fmt.Println(code)

		cfgPath, _ := findConfig()
		cfg, _ := loadConfig(cfgPath)
		if v, _ := cfg.get("", "clear_on_set"); fromArg && clear && !isNo(v) {
			clearScreen(out)
		}
		fmt.Fprintln(out, "Updated!")
		fmt.Fprintf(out, "  %s is set for this terminal session only.\n", name)
		if fromArg {
			fmt.Fprintf(out, "  Note: a %s typed as an argument stays in your shell history.\n", what)
			fmt.Fprintln(out, "  Leave the value off to type it hidden instead.")
		}
		return 0
	}

	cfgFile, err := findConfig()
	if err == nil && cfgFile == "" {
		if cfgFile, err = defaultConfigTarget(); err == nil {
			err = createConfig(cfgFile)
		}
	}
	if err != nil {
		warnf("%v", err)
		return 1
	}
	cfg, err := loadConfig(cfgFile)
	if err != nil {
		warnf("%v", err)
		return 1
	}

	if !havePassword {
		if password, err = readNewPassword(what); err != nil {
			warnf("%v", err)
			return 1
		}
	}
	if password == "" || strings.ContainsAny(password, "\r\n") {
		warnf("the %s cannot be empty or contain a line break", what)
		return 1
	}

	data, err := os.ReadFile(cfgFile)
	if err != nil {
		warnf("%v", err)
		return 1
	}
	lines := strings.Split(strings.TrimRight(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n"), "\n")

	section := "host " + host
	if key != "" {
		section = "key " + key
	} else if host == "" {
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
	lines = setConfigValue(lines, section, what, password)
	if err := writeConfig(cfgFile, lines); err != nil {
		warnf("%v", err)
		return 1
	}

	if v, _ := cfg.get("", "clear_on_set"); fromArg && clear && !isNo(v) {
		clearScreen(out)
	}
	fmt.Fprintln(out, "Updated!")
	fmt.Fprintf(out, "  %s of [%s] in %s\n", what, section, cfgFile)
	if fromArg {
		fmt.Fprintf(out, "  Note: a %s typed as an argument stays in your shell history.\n", what)
		fmt.Fprintln(out, "  Leave the value off to type it hidden instead.")
	}
	if envName := "SSHC_" + strings.ToUpper(what); host == "" && key == "" && os.Getenv(envName) != "" {
		fmt.Fprintf(out, "  Note: %s is set in this shell and takes precedence here.\n", envName)
	}
	return 0
}
