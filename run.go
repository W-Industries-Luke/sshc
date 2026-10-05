package main

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
	drop := []string{"SSH_ASKPASS", "SSH_ASKPASS_REQUIRE", envState, envDests, envConfig}
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

func runTool(tool string, args []string) int {
	cfgPath, err := findConfig()
	if err != nil {
		warnf("%v", err)
		return 1
	}
	// Nothing stored anywhere: behave exactly like the plain tool.
	if cfgPath == "" && !newResolver(nil, nil).hasEnvPasswords() {
		return passthrough(tool, args)
	}
	if cfgPath != "" {
		if err := checkConfigSecure(cfgPath); err != nil {
			warnf("%v", err)
			return 1
		}
	}
	if err := checkSSHVersion(); err != nil {
		warnf("%v", err)
		return 1
	}
	dests := findDests(tool, args)
	if len(dests) == 0 {
		return passthrough(tool, args)
	}

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

	return spawn(tool, args, childEnv(self, state, cfgPath, dests))
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
	if cfgPath == "" {
		fmt.Println("config file: none")
	} else {
		fmt.Println("config file:", cfgPath)
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
	for _, d := range dests {
		if d.alias == d.hostname {
			fmt.Printf("%s (user %s)\n", d.alias, d.user)
		} else {
			fmt.Printf("%s -> %s (user %s)\n", d.alias, d.hostname, d.user)
		}
		password, from, note := r.lookup(d.user, d.hostname)
		switch {
		case password != "":
			fmt.Printf("  password from %s\n", from)
		case note != "":
			fmt.Printf("  no stored password (%s); you would be prompted\n", note)
		default:
			fmt.Println("  no stored password; you would be prompted")
		}
	}
	return 0
}
