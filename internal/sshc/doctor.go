package sshc

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// cmdDoctor checks the things that most often explain "it does not work" and
// says what to do about each.
func cmdDoctor(args []string) int {
	if len(args) > 0 {
		warnf("usage: %s doctor", prog)
		return 1
	}
	var lines []string
	problems := 0
	good := func(format string, a ...any) { lines = append(lines, "  ok       "+fmt.Sprintf(format, a...)) }
	note := func(format string, a ...any) { lines = append(lines, "  note     "+fmt.Sprintf(format, a...)) }
	bad := func(fix, format string, a ...any) {
		problems++
		lines = append(lines, "  PROBLEM  "+fmt.Sprintf(format, a...), "           -> "+fix)
	}

	// The OpenSSH client.
	if out, err := exec.Command("ssh", "-V").CombinedOutput(); err != nil {
		bad("install the OpenSSH client", "ssh was not found on your PATH")
	} else if err := checkSSHVersion(); err != nil {
		bad("upgrade the OpenSSH client", "%v", err)
	} else {
		good("OpenSSH client: %s", strings.TrimSpace(strings.SplitN(string(out), ",", 2)[0]))
	}

	// This copy of sshc, and any others.
	self, _ := os.Executable()
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	good("sshc %s at %s", version, self)
	var copies []string
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		c := filepath.Join(dir, exeName())
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			dup := false
			for _, seen := range copies {
				dup = dup || sameFile(seen, c)
			}
			if !dup {
				copies = append(copies, c)
			}
		}
	}
	switch {
	case len(copies) == 0:
		bad("run \"sshc install\", or add its folder to PATH", "sshc is not on your PATH")
	case len(copies) > 1:
		bad("keep one and remove the others", "%d copies of sshc are on your PATH; the first one runs:\n           %s", len(copies), strings.Join(copies, "\n           "))
	case !sameFile(copies[0], self):
		note("the sshc on your PATH is a different copy: %s", copies[0])
	}

	// The shell hook.
	if kind := os.Getenv(envHook); kind != "" {
		good("shell hook loaded (%s): \"sshc set -s\", \"sshc use\" and Tab completion work", kind)
	} else {
		bad("add this line to your shell's startup file and open a new terminal:\n              "+hookLine(defaultShellKind()),
			"the shell hook is not loaded in this terminal")
	}

	// Config file and what it points at.
	cfgFile, err := findConfig()
	var cfg *config
	if err == nil {
		cfg, err = loadConfig(cfgFile)
	}
	switch {
	case err != nil:
		bad("fix the file, or its permissions as the message says", "%v", err)
	case cfgFile == "":
		note("no config file yet; \"sshc set\" creates one")
	default:
		good("config file: %s", cfgFile)
	}
	st, storeErr := systemStore()
	if storeErr != nil {
		note("no credential store here (%v); \"sshc set\" keeps secrets in plain text", storeErr)
	} else {
		good("credential store: %s", st.name())
	}
	if cfg != nil {
		plain, missing := 0, 0
		for k, v := range cfg.entries {
			section, key, _ := strings.Cut(k, "\n")
			if key != "password" && key != "passphrase" || section == "" {
				continue
			}
			if v != storeMarker {
				plain++
			} else if storeErr == nil {
				if _, err := st.get(storeKey(section, key)); err != nil {
					missing++
				}
			}
		}
		if missing > 0 {
			bad("run \"sshc list\" to see which, and \"sshc set\" them again", "%d saved entries are missing from the credential store", missing)
		}
		if plain > 0 && storeErr == nil {
			note("%d secrets are in plain text in the config file; \"sshc migrate\" moves them to the credential store", plain)
		}
	}
	if isLocked(cfg) {
		note("sshc is locked: nothing stored is supplied until you run \"sshc unlock\"")
	} else if setting, _ := cfg.get("", "lock_after"); setting != "" {
		if _, ok := parseLockAfter(setting); ok {
			good("locks itself after %s without use", setting)
		} else {
			bad("use a value such as 30m, 8h or 2d", "lock_after = %s in the config file is not a length of time", setting)
		}
	}
	if err := canVerifyUser(); err != nil {
		note("\"sshc lock\" is not available here: %v", err)
	}
	if newResolver(cfg, nil).hasEnvPasswords() {
		note("SSHC_* variables are set in this terminal and take precedence over saved entries")
	}

	// ssh-agent, for "sshc ssh-add".
	if err := exec.Command("ssh-add", "-l").Run(); err == nil {
		good("ssh-agent is running and holds at least one key")
	} else if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		good("ssh-agent is running (no keys loaded; \"sshc ssh-add\" loads yours)")
	} else {
		note("ssh-agent is not reachable; only needed for \"sshc ssh-add\"")
	}

	if n := len(sshConfigHosts()); n > 0 {
		good("%d hosts in ~/.ssh/config for the picker and Tab completion", n)
	} else {
		note("no hosts in ~/.ssh/config; the picker and host completion have nothing to offer")
	}

	u := newUI(os.Stdout)
	u.say("%s", strings.Join(lines, "\n"))
	if problems == 0 {
		u.say("Done: no problems found.")
	} else {
		u.say("Warning: %d problem(s) found; each has a suggested fix above.", problems)
	}
	u.flush()
	if problems > 0 {
		return 1
	}
	return 0
}
