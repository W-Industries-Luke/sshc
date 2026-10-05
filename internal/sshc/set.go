package sshc

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/term"
)

const setUsage = `Usage: sshc set [options] [--] [value]

Stores a login password, or the passphrase of an SSH key. Without a value you
are asked to type it, hidden - which also keeps it out of your shell history.

Every option has a short form, and short forms combine: "sshc set -sp" is
"sshc set --session --passphrase".

Where it is kept:
  (no option)       your system's credential store (Windows Credential
                    Manager, macOS Keychain, the Linux keyring): encrypted,
                    and used in every terminal. Where there is none, the
                    config file, in plain text.
  -s, --session     this terminal only: sets the SSHC_* environment variable,
                    nothing is written to disk and it is gone when the
                    terminal closes. Needs the shell hook - "sshc --install"
                    sets it up, or see "sshc --shell-init".
  -f, --plain       the config file, in plain text, even where a credential
                    store exists
  -c, --command CMD nowhere: sshc runs CMD each time and uses the first line
                    it prints, e.g. a password manager's command-line tool.
                    %h, %u and %k stand for the host, user and key file.

What to store:
  (no option)       the login password of the active profile
  -p, --passphrase  the key passphrase of the active profile, tried for any key
  -H, --host NAME   the login password of one host; NAME as you type it for
                    ssh, optionally with "user@"
  -k, --key NAME    the passphrase of one key; NAME is the key's file name
                    (id_ed25519) or its full path
  -o, --otp         the one-time code a host asks for after the password.
                    Always fetched by a command, for one host:
                    sshc set -o -H NAME -c 'oathtool --totp -b SECRET'

Other options:
  -P, --profile NAME  use that profile instead of the active one
  -n, --no-clear    do not clear the screen after a value given as argument
                    (or put "clear_on_set = no" in the config file)
  -h, --help        show this help

The active profile is created as "default" if there is none yet.
"sshc unset" takes the same options and removes an entry; "sshc list" shows
what is stored, never the values.
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
	lastBlank = false
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
	lastBlank = false
}

// deleteConfigValue removes key from section, and the section header with it
// if nothing else is left there. It reports whether the key was present.
func deleteConfigValue(lines []string, section, key string) ([]string, bool) {
	target := configKey(section, "")
	current := configKey("", "")
	header, found, others := -1, -1, 0
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		switch {
		case line == "" || line[0] == '#' || line[0] == ';':
		case line[0] == '[' && line[len(line)-1] == ']':
			current = configKey(line[1:len(line)-1], "")
			if current == target && found < 0 {
				header, others = i, 0
			}
		case current == target && strings.Contains(line, "="):
			k, _, _ := strings.Cut(line, "=")
			if found < 0 && strings.EqualFold(strings.TrimSpace(k), key) {
				found = i
			} else {
				others++
			}
		}
	}
	if found < 0 {
		return lines, false
	}
	var out []string
	for i, raw := range lines {
		if i == found || i == header && others == 0 {
			continue
		}
		out = append(out, raw)
	}
	return out, true
}

// Short forms of the "set" and "unset" options. The ones in shortWithValue
// take the next word (or the rest of the cluster) as their value.
var (
	shortFlags = map[byte]string{
		's': "--session", 'p': "--passphrase", 'f': "--plain", 'n': "--no-clear", 'h': "--help", 'o': "--otp",
	}
	shortWithValue = map[byte]string{'H': "--host", 'k': "--key", 'P': "--profile", 'c': "--command"}
)

// expandShort rewrites short options to their long forms, so "-sp" becomes
// "--session --passphrase" and "-H box" becomes "--host box". Everything from
// "--" on is left alone, as is a lone "-".
func expandShort(args []string) (out []string, ok bool) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return append(out, args[i:]...), true
		}
		if len(arg) < 2 || arg[0] != '-' || arg[1] == '-' {
			out = append(out, arg)
			continue
		}
		for j := 1; j < len(arg); j++ {
			if long, isFlag := shortFlags[arg[j]]; isFlag {
				out = append(out, long)
				continue
			}
			long, takesValue := shortWithValue[arg[j]]
			if !takesValue {
				return nil, false
			}
			if rest := arg[j+1:]; rest != "" {
				out = append(out, long, rest)
			} else if i+1 < len(args) {
				i++
				out = append(out, long, args[i])
			} else {
				out = append(out, long)
			}
			break
		}
	}
	return out, true
}

// target is what a "set" or "unset" command line points at.
type target struct {
	profile, host, key  string
	command             string // fetch the secret by running this instead of storing it
	passphrase, session bool
	otp                 bool // the one-time code a host asks for after the password
	plain, clear        bool
	value               string
	haveValue           bool
}

// parseTarget reads the options shared by "set" and "unset". done is true
// when the caller should just return rc.
func parseTarget(args []string, usage string, takesValue bool, out *os.File) (t target, rc int, done bool) {
	t.clear = true
	fail := func(format string, a ...any) (target, int, bool) {
		warnf(format, a...)
		return t, 1, true
	}
	args, ok := expandShort(args)
	if !ok {
		fmt.Fprint(os.Stderr, usage)
		return t, 1, true
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, val, attached := strings.Cut(arg, "=")
		switch {
		case arg == "--help":
			fmt.Fprint(out, usage)
			return t, 0, true
		case arg == "--session":
			t.session = true
		case arg == "--plain" && takesValue:
			t.plain = true
		case arg == "--no-clear" && takesValue:
			t.clear = false
		case arg == "--passphrase":
			t.passphrase = true
		case arg == "--otp":
			t.otp = true
		case name == "--command" && takesValue:
			if !attached {
				i++
				if i >= len(args) {
					return fail("--command needs the command to run")
				}
				val = args[i]
			}
			if _, err := splitCommand(val); err != nil || strings.ContainsAny(val, "\r\n") {
				return fail("%q is not a usable command", val)
			}
			t.command = val
		case name == "--profile" || name == "--host" || name == "--key":
			if !attached {
				i++
				if i >= len(args) {
					return fail("%s needs a name", name)
				}
				val = args[i]
			}
			if val == "" || strings.ContainsAny(val, "[]\n") {
				return fail("%q is not a usable name", val)
			}
			switch name {
			case "--profile":
				t.profile = val
			case "--host":
				t.host = val
			default:
				t.key = val
			}
		case arg == "--" && takesValue && i+2 == len(args):
			t.value, t.haveValue = args[i+1], true
			i++
		case strings.HasPrefix(arg, "-") && arg != "-" || t.haveValue || !takesValue:
			fmt.Fprint(os.Stderr, usage)
			return t, 1, true
		default:
			t.value, t.haveValue = arg, true
		}
	}
	chosen := 0
	for _, on := range []bool{t.host != "", t.key != "", t.profile != "" || t.passphrase} {
		if on {
			chosen++
		}
	}
	if chosen > 1 {
		return fail("--host, --key and --profile/--passphrase point at different entries; use one")
	}
	if t.session && t.profile != "" {
		return fail("--session works on a variable in this terminal; it cannot be combined with --profile")
	}
	if t.session && t.plain {
		return fail("--session and --plain are different places to keep it; use one")
	}
	if t.otp && (t.host == "" || t.passphrase || t.session || t.plain || t.haveValue || takesValue && t.command == "") {
		return fail("a one-time code is fetched by a command, for one host: use --otp with --host and --command")
	}
	if t.command != "" && (t.session || t.plain || t.haveValue) {
		return fail("--command replaces the stored value; it cannot be combined with --session, --plain or a value")
	}
	return t, 0, false
}

// what is the kind of secret: "password" or "passphrase".
func (t target) what() string {
	if t.otp {
		return "otp"
	}
	if t.key != "" || t.passphrase {
		return "passphrase"
	}
	return "password"
}

// envName is the environment variable that holds this secret for a session.
func (t target) envName() string {
	switch {
	case t.host != "":
		return hostEnvPrefix + strings.ToUpper(envSuffix(t.host))
	case t.key != "":
		return keyEnvPrefix + strings.ToUpper(envSuffix(path.Base(normKeyPath(t.key))))
	}
	return "SSHC_" + strings.ToUpper(t.what())
}

// section is the config file section this secret belongs to. For a profile it
// also returns the profile's name.
func (t target) section(cfg *config) (section, profile string) {
	switch {
	case t.host != "":
		return "host " + t.host, ""
	case t.key != "":
		return "key " + t.key, ""
	}
	profile = t.profile
	if profile == "" {
		profile = os.Getenv("SSHC_PROFILE")
	}
	if profile == "" {
		profile, _ = cfg.get("", "profile")
	}
	if profile == "" {
		profile = "default"
	}
	return "profile " + profile, profile
}

// hookMissing explains that --session cannot work in this terminal.
func hookMissing(what string) int {
	failf(what+" needs the sshc shell hook, which is not loaded in this terminal.",
		"Add this line to your shell's startup file, then open a new terminal:\n    "+hookLine(defaultShellKind()))
	return 1
}

func readConfigLines(file string) ([]string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimRight(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n"), "\n"), nil
}

func cmdSet(args []string) int {
	// Under the shell hook stdout is evaluated by the shell, so everything
	// meant for the human goes to stderr.
	emit := os.Getenv(envEmit)
	out := os.Stdout
	if emit != "" {
		out = os.Stderr
	}
	t, rc, done := parseTarget(args, setUsage, true, out)
	if done {
		return rc
	}
	what := t.what()

	if t.session {
		if _, ok := emitAssignment(emit, "X", ""); !ok {
			return hookMissing("--session")
		}
		// A shell runs a piped-into function in a subshell, where the
		// variable would be set and immediately lost.
		if !t.haveValue && !term.IsTerminal(int(os.Stdin.Fd())) {
			warnf("with --session, type the %s when asked or give it as an argument; it cannot be piped in", what)
			return 1
		}
	}

	var cfgFile string
	var cfg *config
	var err error
	if t.session {
		// Only consulted for clear_on_set; a missing file is fine.
		cfgFile, _ = findConfig()
		cfg, _ = loadConfig(cfgFile)
	} else {
		cfgFile, err = findConfig()
		if err == nil && cfgFile == "" {
			if cfgFile, err = defaultConfigTarget(); err == nil {
				err = createConfig(cfgFile)
			}
		}
		if err == nil {
			cfg, err = loadConfig(cfgFile)
		}
		if err != nil {
			warnf("%v", err)
			return 1
		}
	}

	secret := t.value
	if t.command == "" {
		if !t.haveValue {
			if secret, err = readNewPassword(what); err != nil {
				warnf("%v", err)
				return 1
			}
		}
		if secret == "" || strings.ContainsAny(secret, "\r\n") {
			warnf("the %s cannot be empty or contain a line break", what)
			return 1
		}
		if secret == storeMarker {
			warnf("%q is reserved by sshc and cannot be used as a %s", storeMarker, what)
			return 1
		}
	}

	var where string
	var notes []string
	if t.session {
		code, _ := emitAssignment(emit, t.envName(), secret)
		fmt.Println(code)
		where = t.envName() + " is set for this terminal session only."
	} else {
		lines, err := readConfigLines(cfgFile)
		if err != nil {
			warnf("%v", err)
			return 1
		}
		section, profile := t.section(cfg)
		// The first profile ever saved becomes the active one.
		if active, _ := cfg.get("", "profile"); profile != "" && active == "" {
			lines = setConfigValue(lines, "", "profile", profile)
		}

		inFile := secret
		st, storeErr := systemStore()
		switch {
		case t.command != "":
			// The command takes over: drop any value kept so far.
			if st != nil {
				_ = st.del(storeKey(section, what))
			}
			lines, _ = deleteConfigValue(lines, section, what)
			where = fmt.Sprintf("%s of [%s], fetched by running: %s", what, section, t.command)
		case t.plain || storeErr != nil:
			if storeErr != nil && !t.plain {
				notes = append(notes, fmt.Sprintf("Note: no credential store is available here (%v).", storeErr))
			}
			// Do not leave an older copy behind in the store.
			if st != nil {
				_ = st.del(storeKey(section, what))
			}
			where = fmt.Sprintf("%s of [%s], in plain text in %s", what, section, cfgFile)
		default:
			if err := st.set(storeKey(section, what), secret); err != nil {
				failf(fmt.Sprintf("could not save to %s: %v", st.name(), err),
					"Nothing was changed. Add --plain to keep it in the config file instead.")
				return 1
			}
			inFile = storeMarker
			where = fmt.Sprintf("%s of [%s], kept in %s", what, section, st.name())
		}
		if t.command != "" {
			lines = setConfigValue(lines, section, what+"_command", t.command)
		} else {
			lines = setConfigValue(lines, section, what, inFile)
			lines, _ = deleteConfigValue(lines, section, what+"_command")
		}
		if err := writeConfig(cfgFile, lines); err != nil {
			warnf("%v", err)
			return 1
		}
		if envName := "SSHC_" + strings.ToUpper(what); t.host == "" && t.key == "" && os.Getenv(envName) != "" {
			notes = append(notes, "Note: "+envName+" is set in this shell and takes precedence here.")
		}
	}

	if v, _ := cfg.get("", "clear_on_set"); t.haveValue && t.clear && !isNo(v) {
		clearScreen(out)
	}
	u := newUI(out)
	u.say("Updated!")
	u.say("%s", where)
	for _, note := range notes {
		u.say("%s", note)
	}
	if t.haveValue {
		u.say("Note: a %s typed as an argument stays in your shell history.\nLeave the value off to type it hidden instead.", what)
	}
	u.flush()
	return 0
}

const unsetUsage = `Usage: sshc unset [-s] [-p | -H NAME | -k NAME] [-P NAME]

Removes a stored login password or key passphrase. The options name the entry
the same way as for "sshc set": with none it is the login password of the
active profile.

  -s, --session       clear the variable in this terminal instead
  -p, --passphrase    the key passphrase of the active profile
  -H, --host NAME     the login password of one host
  -k, --key NAME      the passphrase of one key
  -P, --profile NAME  use that profile instead of the active one
  -h, --help          show this help
`

func cmdUnset(args []string) int {
	emit := os.Getenv(envEmit)
	out := os.Stdout
	if emit != "" {
		out = os.Stderr
	}
	t, rc, done := parseTarget(args, unsetUsage, false, out)
	if done {
		return rc
	}
	what := t.what()

	if t.session {
		code, ok := emitUnset(emit, t.envName())
		if !ok {
			return hookMissing("--session")
		}
		fmt.Println(code)
		u := newUI(out)
		u.say("Removed!")
		u.say("%s is no longer set in this terminal.", t.envName())
		u.flush()
		return 0
	}

	cfgFile, err := findConfig()
	var cfg *config
	if err == nil {
		cfg, err = loadConfig(cfgFile)
	}
	if err != nil {
		warnf("%v", err)
		return 1
	}
	section, _ := t.section(cfg)
	current, _ := cfg.get(section, what)
	if command, _ := cfg.get(section, what+"_command"); current == "" && command == "" {
		warnf("there is no stored %s for [%s]", what, section)
		return 1
	}
	if current == storeMarker {
		st, err := systemStore()
		if err == nil {
			err = st.del(storeKey(section, what))
		}
		if err != nil && !errors.Is(err, errNotFound) {
			warnf("could not remove it from the credential store: %v", err)
			return 1
		}
	}
	lines, err := readConfigLines(cfgFile)
	if err == nil {
		lines, _ = deleteConfigValue(lines, section, what)
		lines, _ = deleteConfigValue(lines, section, what+"_command")
		err = writeConfig(cfgFile, lines)
	}
	if err != nil {
		warnf("%v", err)
		return 1
	}
	u := newUI(out)
	u.say("Removed!")
	u.say("%s of [%s]", what, section)
	u.flush()
	return 0
}

// cmdList shows what is stored and where, never the values.
func cmdList(args []string) int {
	if len(args) > 0 {
		warnf("usage: %s list", prog)
		return 1
	}
	cfgFile, err := findConfig()
	var cfg *config
	if err == nil {
		cfg, err = loadConfig(cfgFile)
	}
	if err != nil {
		warnf("%v", err)
		return 1
	}
	st, storeErr := systemStore()
	u := newUI(os.Stdout)
	head := "config file: none"
	if cfgFile != "" {
		head = "config file: " + cfgFile
	}
	if storeErr != nil {
		head += fmt.Sprintf("\ncredential store: not available (%v)", storeErr)
	} else {
		head += "\ncredential store: " + st.name()
	}
	r := newResolver(cfg, nil)
	if p := r.activeProfile(); p != "" {
		head += "\nactive profile: " + p
	}
	u.say("%s", head)

	var rows []string
	if cfg != nil {
		for k, v := range cfg.entries {
			section, key, _ := strings.Cut(k, "\n")
			kind, _, _ := strings.Cut(section, " ")
			isCommand := strings.HasSuffix(key, "_command")
			key = strings.TrimSuffix(key, "_command")
			if v == "" || key != "password" && key != "passphrase" && key != "otp" || kind != "profile" && kind != "host" && kind != "key" {
				continue
			}
			place := "plain text in the config file"
			if isCommand {
				place = "from the command: " + v
			} else if v == storeMarker {
				place = "credential store"
				if storeErr != nil {
					place += " (not readable here)"
				} else if _, err := st.get(storeKey(section, key)); errors.Is(err, errNotFound) {
					place += " - MISSING, set it again"
				} else if err != nil {
					place += " (could not be read: " + err.Error() + ")"
				}
			}
			rows = append(rows, fmt.Sprintf("  %-28s %-11s %s", "["+section+"]", key, place))
		}
	}
	sort.Strings(rows)
	if len(rows) == 0 {
		u.say("Nothing is stored.")
	} else {
		u.say("Stored:\n%s", strings.Join(rows, "\n"))
	}

	var vars []string
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		up := strings.ToUpper(k)
		if v != "" && (up == "SSHC_PASSWORD" || up == "SSHC_PASSPHRASE" || strings.HasPrefix(up, hostEnvPrefix) || strings.HasPrefix(up, keyEnvPrefix)) {
			vars = append(vars, "  "+k)
		}
	}
	if len(vars) > 0 {
		sort.Strings(vars)
		u.say("Set in this terminal (these take precedence):\n%s", strings.Join(vars, "\n"))
	}
	u.flush()
	return 0
}
