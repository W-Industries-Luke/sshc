package sshc

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"golang.org/x/term"
)

// A host can have a directory to start in and a command to run on entry:
//
//	[host w.go-2]
//	directory = /var/www
//	entry = source ~/venv/bin/activate
//
// They apply to an interactive login only - plain "sshc host" at a terminal.
// sshc then asks ssh for a terminal and passes one remote command that
// changes directory, runs the entry command and hands over to the user's
// login shell. Every other kind of connection is left exactly as typed, which
// is what ssh's own RemoteCommand setting cannot do.

// envNoEntry skips the login settings, like the --no-entry option.
const envNoEntry = "SSHC_NO_ENTRY"

// quoteRemote quotes s for the POSIX-style shell on the far side.
func quoteRemote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// entryCommand builds the remote command for a start directory and an entry
// command; either may be empty.
func entryCommand(dir, entry string) string {
	var cd string
	switch {
	case dir == "":
	case dir == "~":
		cd = "cd ~"
	case strings.HasPrefix(dir, "~/"):
		// The tilde has to stay outside the quotes to be expanded.
		cd = "cd ~/" + quoteRemote(dir[2:])
	default:
		cd = "cd " + quoteRemote(dir)
	}
	// If the directory is missing, cd says so and the entry command is
	// skipped; the shell is started either way.
	steps := cd
	if entry != "" {
		if steps != "" {
			steps += " && "
		}
		steps += entry
	}
	return steps + `; exec "$SHELL" -l`
}

// interactiveLogin reports whether an ssh command line is a plain login:
// options, one destination and nothing else. Anything that carries its own
// command, asks for no shell, or picks its own remote command is not.
func interactiveLogin(args []string) bool {
	cfgOpts, operands := splitArgs(args, optsWithArg["ssh"])
	if len(operands) != 1 {
		return false
	}
	for i := 1; i < len(cfgOpts); i += 2 {
		name, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(cfgOpts[i])), "=")
		switch strings.TrimSpace(strings.Fields(name + " x")[0]) {
		case "remotecommand", "requesttty", "sessiontype", "forkafterauthentication", "stdinnull":
			return false
		}
	}
	// -N no command, -W forward only, -T no terminal, -s subsystem, -f
	// background, -n no input, -O control an existing connection.
	withArg := optsWithArg["ssh"]
	for _, arg := range args[:len(args)-1] {
		if len(arg) < 2 || arg[0] != '-' || arg == "--" {
			continue
		}
		for j := 1; j < len(arg); j++ {
			if strings.ContainsRune("NWTsfnO", rune(arg[j])) {
				return false
			}
			if strings.ContainsRune(withArg, rune(arg[j])) {
				break // the rest of this word is the option's value
			}
		}
	}
	return true
}

// hasLoginSettings reports whether any host has a directory or entry command,
// so that hosts without one cost nothing extra.
func hasLoginSettings(cfg *config) bool {
	if cfg == nil {
		return false
	}
	for k, v := range cfg.entries {
		if _, key, _ := strings.Cut(k, "\n"); v != "" && (key == "directory" || key == "entry") {
			return true
		}
	}
	return false
}

// loginSettings finds the start directory and entry command for user@host.
func (r *resolver) loginSettings(user, host string) (dir, entry string) {
	keys, _ := r.hostKeys(user, host)
	for _, k := range keys {
		if dir == "" {
			dir, _ = r.cfg.get("host "+k, "directory")
		}
		if entry == "" {
			entry, _ = r.cfg.get("host "+k, "entry")
		}
	}
	return dir, entry
}

// withEntry returns the ssh arguments to use for this login: unchanged, or
// with a terminal request and the remote command added.
func withEntry(args []string, cfgPath string) []string {
	if os.Getenv(envNoEntry) != "" || !term.IsTerminal(int(os.Stdin.Fd())) || !interactiveLogin(args) {
		return args
	}
	cfg, err := loadConfig(cfgPath)
	if err != nil || !hasLoginSettings(cfg) {
		return args
	}
	d, ok := queryDest(args)
	if !ok {
		return args
	}
	// ssh refuses a command next to a RemoteCommand from its own config.
	if d.remoteCommand != "" && !strings.EqualFold(d.remoteCommand, "none") {
		debugf("the ssh config sets RemoteCommand for this host; leaving the login alone")
		return args
	}
	if d.alias == "" {
		d.alias = d.hostname
	}
	dir, entry := newResolver(cfg, []dest{d}).loginSettings(d.user, d.hostname)
	if dir == "" && entry == "" {
		return args
	}
	command := entryCommand(dir, entry)
	debugf("interactive login with a directory or entry command: %q", command)
	out := append([]string{"-t"}, args...)
	return append(out, command)
}

// setLogin stores or removes the login settings of one host.
func setLogin(t target, remove bool, out *os.File) int {
	cfgFile, err := findConfig()
	if err == nil && cfgFile == "" && !remove {
		if cfgFile, err = defaultConfigTarget(); err == nil {
			err = createConfig(cfgFile)
		}
	}
	var cfg *config
	if err == nil {
		cfg, err = loadConfig(cfgFile)
	}
	var lines []string
	if err == nil && cfgFile != "" {
		lines, err = readConfigLines(cfgFile)
	}
	if err != nil {
		warnf("%v", err)
		return 1
	}
	section := "host " + t.host
	u := newUI(out)
	var done []string
	change := func(key, value string, wanted bool) bool {
		if !wanted {
			return true
		}
		if remove {
			if current, _ := cfg.get(section, key); current == "" {
				warnf("[%s] has no %s set", section, key)
				return false
			}
			lines, _ = deleteConfigValue(lines, section, key)
			done = append(done, fmt.Sprintf("%s of [%s]", key, section))
			return true
		}
		lines = setConfigValue(lines, section, key, value)
		done = append(done, fmt.Sprintf("%s of [%s]: %s", key, section, value))
		return true
	}
	if !change("directory", t.dir, t.dir != "" || t.unsetDir) || !change("entry", t.entry, t.entry != "" || t.unsetEntry) {
		return 1
	}
	if err := writeConfig(cfgFile, lines); err != nil {
		warnf("%v", err)
		return 1
	}
	if remove {
		u.say("Removed!")
	} else {
		u.say("Updated!")
	}
	u.say("%s", strings.Join(done, "\n"))
	if !remove {
		u.say("Used when you log in with \"sshc %s\" and nothing after it. Add --no-entry to skip it once.", t.host)
	}
	u.flush()
	return 0
}

// loginRows lists the login settings for "sshc list".
func loginRows(cfg *config) []string {
	var rows []string
	if cfg == nil {
		return nil
	}
	for k, v := range cfg.entries {
		section, key, _ := strings.Cut(k, "\n")
		if v != "" && strings.HasPrefix(section, "host ") && (key == "directory" || key == "entry") {
			rows = append(rows, fmt.Sprintf("  %-28s %-11s %s", "["+section+"]", key, v))
		}
	}
	sort.Strings(rows)
	return rows
}
