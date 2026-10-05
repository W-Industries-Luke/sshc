package sshc

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// Oldest OpenSSH we can drive safely: 8.4 added SSH_ASKPASS_REQUIRE, and 8.5
// started prefixing keyboard-interactive prompts with "(user@host)", which is
// what stops a server from choosing which host's password it is asking for.
const minSSHMajor, minSSHMinor = 8, 5

var reSSHVersion = regexp.MustCompile(`OpenSSH(?:_for_Windows)?_(\d+)\.(\d+)`)

func checkSSHVersion() error {
	// ssh -V writes to stderr.
	out, err := exec.Command("ssh", "-V").CombinedOutput()
	if err != nil {
		return fmt.Errorf("could not run ssh; is the OpenSSH client installed? (%v)", err)
	}
	m := reSSHVersion.FindSubmatch(out)
	if m == nil {
		return fmt.Errorf("the ssh on your PATH does not look like OpenSSH: %s", strings.TrimSpace(string(out)))
	}
	major, _ := strconv.Atoi(string(m[1]))
	minor, _ := strconv.Atoi(string(m[2]))
	if major < minSSHMajor || major == minSSHMajor && minor < minSSHMinor {
		return fmt.Errorf("OpenSSH %d.%d or newer is required (found %d.%d)", minSSHMajor, minSSHMinor, major, minor)
	}
	return nil
}

// childEnv is our environment with the askpass hook pointed back at us.
func childEnv(self, state, cfgPath string, dests []dest) []string {
	drop := []string{"SSH_ASKPASS", "SSH_ASKPASS_REQUIRE", envState, envDests, envConfig, envTool, envNoTTY}
	var env []string
next:
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		for _, d := range drop {
			if strings.EqualFold(k, d) {
				continue next
			}
		}
		env = append(env, kv)
	}
	return append(env,
		"SSH_ASKPASS="+self,
		"SSH_ASKPASS_REQUIRE=force",
		envState+"="+state,
		envDests+"="+encodeDests(dests),
		envConfig+"="+cfgPath,
	)
}

// spawn runs the tool on our terminal and returns its exit status.
func spawn(tool string, args, env []string) int {
	cmd := exec.Command(tool, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			warnf("%s is not installed, or not on your PATH", tool)
		} else {
			warnf("%v", err)
		}
		return 127
	}
	stop := relaySignals(cmd.Process)
	err := cmd.Wait()
	stop()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &ee):
		return exitStatus(ee)
	default:
		warnf("%v", err)
		return 1
	}
}

// launch runs a program with sshc answering the prompts of any ssh it starts.
// dests are the hosts that may be given the active password; tool names the
// wrapped tool for the askpass side.
func launch(program string, args []string, cfgPath string, dests []dest, tool string) int {
	self, err := os.Executable()
	if err != nil {
		warnf("could not locate my own executable: %v", err)
		return 1
	}
	// Prefer the per-user runtime directory (memory-backed on Linux).
	state, err := os.MkdirTemp(os.Getenv("XDG_RUNTIME_DIR"), "sshc-")
	if err != nil {
		state, err = os.MkdirTemp("", "sshc-")
	}
	if err != nil {
		warnf("could not create a private temporary directory: %v", err)
		return 1
	}
	defer os.RemoveAll(state)
	debugf("askpass program: %q, state: %q", self, state)

	return spawn(program, args, append(childEnv(self, state, cfgPath, dests), envTool+"="+tool))
}

// prepare does what every launch needs first. ok is false when sshc has
// nothing stored (plain is then true: just run the program) or cannot go on.
func prepare(name string) (cfgPath string, ok, plain bool) {
	cfgPath, err := findConfig()
	if err != nil {
		warnf("%v", err)
		return "", false, false
	}
	// Nothing stored anywhere: behave exactly like the plain tool.
	if cfgPath == "" && !newResolver(nil, nil).hasEnvPasswords() {
		debugf("no config file and no SSHC_PASSWORD*/SSHC_PASSPHRASE* variables; running plain %s", name)
		return "", false, true
	}
	debugf("config file: %q", cfgPath)
	if cfgPath != "" {
		if err := checkConfigSecure(cfgPath); err != nil {
			warnf("%v", err)
			return "", false, false
		}
	}
	if err := checkSSHVersion(); err != nil {
		warnf("%v", err)
		return "", false, false
	}
	return cfgPath, true, false
}

func runTool(tool string, args []string) int {
	cfgPath, ok, plain := prepare(tool)
	if plain {
		return passthrough(tool, args)
	}
	if !ok {
		return 1
	}
	var dests []dest
	// ssh-add talks to the local agent, not to a host.
	if tool != "ssh-add" {
		if dests = findDests(tool, args); len(dests) == 0 {
			debugf("no destination found in the arguments; running plain %s", tool)
			return passthrough(tool, args)
		}
		debugf("destinations: %q", encodeDests(dests))
	}
	return launch(tool, args, cfgPath, dests, tool)
}

const runUsage = `Usage: sshc run [-d HOST]... [--] command [arguments ...]

Runs any command with sshc answering the ssh prompts underneath it - for
programs that call ssh themselves, such as git, ansible or sshfs.

  -d, --dest HOST   a host this command will connect to. Stored key
                    passphrases and host-specific passwords are used anyway;
                    the active login password is only offered to hosts named
                    here.
`

func cmdRun(args []string) int {
	var hosts []string
options:
	for len(args) > 0 {
		name, val, attached := strings.Cut(args[0], "=")
		switch {
		case args[0] == "-h" || args[0] == "--help":
			fmt.Print(runUsage)
			return 0
		case args[0] == "--":
			args = args[1:]
			break options
		case name == "-d" || name == "--dest":
			args = args[1:]
			if !attached {
				if len(args) == 0 {
					warnf("%s needs a host", name)
					return 1
				}
				val, args = args[0], args[1:]
			}
			hosts = append(hosts, val)
		case strings.HasPrefix(args[0], "-"):
			fmt.Fprint(os.Stderr, runUsage)
			return 1
		default:
			break options
		}
	}
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, runUsage)
		return 1
	}
	cfgPath, ok, plain := prepare(args[0])
	if plain {
		return passthrough(args[0], args[1:])
	}
	if !ok {
		return 1
	}
	var dests []dest
	for _, h := range hosts {
		if d, ok := queryDest([]string{"--", h}); ok {
			d.alias = h
			dests = append(dests, d)
		}
	}
	return launch(args[0], args[1:], cfgPath, dests, "run")
}

func cmdCheck(args []string) int {
	tool := "ssh"
	if len(args) > 0 && isTool(args[0]) {
		tool, args = args[0], args[1:]
	}
	if len(args) == 0 {
		warnf("--check needs a destination, e.g. %s --check myhost", prog)
		return 1
	}
	cfgPath, err := findConfig()
	if err != nil {
		warnf("%v", err)
		return 1
	}
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		warnf("%v", err)
		return 1
	}
	u := newUI(os.Stdout)
	if cfgPath == "" {
		u.say("config file: none")
	} else {
		u.say("config file: %s", cfgPath)
	}
	if err := checkSSHVersion(); err != nil {
		warnf("%v", err)
		return 1
	}
	dests := findDests(tool, args)
	if len(dests) == 0 {
		warnf("could not work out a destination from: %s %s", tool, strings.Join(args, " "))
		return 1
	}
	r := newResolver(cfg, dests)
	r.dryRun = true // a password manager should not be woken just to look
	for _, d := range dests {
		lines := []string{fmt.Sprintf("%s -> %s (user %s)", d.alias, d.hostname, d.user)}
		if d.alias == d.hostname {
			lines[0] = fmt.Sprintf("%s (user %s)", d.alias, d.user)
		}
		password, from, note := r.lookup(d.user, d.hostname)
		switch {
		case password != "":
			lines = append(lines, "  password from "+from)
		case note != "":
			lines = append(lines, fmt.Sprintf("  no stored password (%s); you would be prompted", note))
		default:
			lines = append(lines, "  no stored password; you would be prompted")
		}
		for _, key := range d.keys {
			file := key
			if file == "~" || strings.HasPrefix(file, "~/") || strings.HasPrefix(file, "~\\") {
				if home, err := os.UserHomeDir(); err == nil {
					file = home + file[1:]
				}
			}
			if !exists(file) {
				continue
			}
			if passphrase, from, note := r.lookupPassphrase(key); passphrase != "" {
				lines = append(lines, fmt.Sprintf("  key %s: passphrase from %s", key, from))
			} else if note != "" {
				lines = append(lines, fmt.Sprintf("  key %s: no usable passphrase (%s)", key, note))
			} else {
				lines = append(lines, fmt.Sprintf("  key %s: no stored passphrase; you would be prompted if it has one", key))
			}
		}
		u.say("%s", strings.Join(lines, "\n"))
	}
	u.flush()
	return 0
}
