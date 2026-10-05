package sshc

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

const eachUsage = `Usage: sshc each [-j N] [ssh options] host [host ...] -- command [arguments ...]

Runs one command on several hosts at the same time and shows each host's
output under its name.

  -j, --jobs N    at most N hosts at once (default: all of them)

Each host is reached with "ssh [ssh options] host command", so names, users
and ports come from your ssh config; ssh options given here apply to every
host. Nothing is asked on the terminal while it runs: a host that needs a
secret sshc has not stored, or whose host key is not known yet, fails and is
reported at the end.
`

// eachResult is how one host's command ended.
type eachResult struct {
	host   string
	status int
}

func cmdEach(args []string) int {
	jobs := 0
own:
	for len(args) > 0 {
		name, val, attached := strings.Cut(args[0], "=")
		switch {
		case args[0] == "-h" || args[0] == "--help":
			fmt.Print(eachUsage)
			return 0
		case name == "-j" || name == "--jobs":
			args = args[1:]
			if !attached {
				if len(args) == 0 {
					warnf("%s needs a number", name)
					return 1
				}
				val, args = args[0], args[1:]
			}
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 {
				warnf("%q is not a usable number of jobs", val)
				return 1
			}
			jobs = n
		default:
			// Anything else is an ssh option or the first host.
			break own
		}
	}
	split := -1
	for i, a := range args {
		if a == "--" {
			split = i
			break
		}
	}
	if split < 1 || split == len(args)-1 {
		fmt.Fprint(os.Stderr, eachUsage)
		return 1
	}
	// Leading ssh options are passed to every ssh; the rest are the hosts.
	_, hosts := splitArgs(args[:split], optsWithArg["ssh"])
	sshOpts := args[:split-len(hosts)]
	command := args[split+1:]
	if len(hosts) == 0 {
		fmt.Fprint(os.Stderr, eachUsage)
		return 1
	}
	if jobs == 0 || jobs > len(hosts) {
		jobs = len(hosts)
	}

	cfgPath, ok, plain := prepare("ssh")
	if lockedNow {
		warnf("\"sshc each\" cannot ask for passwords, and sshc is locked")
		return 1
	}
	if !ok && !plain {
		return 1
	}
	env := os.Environ()
	if ok {
		// Resolve every host first, so each may be offered the active password.
		dests := make([]dest, len(hosts))
		found := make([]bool, len(hosts))
		var wg sync.WaitGroup
		for i, h := range hosts {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if d, ok := queryDest(append(append([]string{}, sshOpts...), "--", h)); ok {
					d.alias = h
					dests[i], found[i] = d, true
				}
			}()
		}
		wg.Wait()
		var known []dest
		for i := range dests {
			if found[i] {
				known = append(known, dests[i])
			}
		}
		self, err := os.Executable()
		if err != nil {
			warnf("could not locate my own executable: %v", err)
			return 1
		}
		state, err := os.MkdirTemp(os.Getenv("XDG_RUNTIME_DIR"), "sshc-")
		if err != nil {
			state, err = os.MkdirTemp("", "sshc-")
		}
		if err != nil {
			warnf("could not create a private temporary directory: %v", err)
			return 1
		}
		defer os.RemoveAll(state)
		env = append(childEnv(self, state, cfgPath, known), envTool+"=each", envNoTTY+"=1")
	}

	width := 0
	for _, h := range hosts {
		if len(h) > width {
			width = len(h)
		}
	}
	colour := newUI(os.Stdout).color
	var out sync.Mutex
	emit := func(host, line string) {
		label := fmt.Sprintf("%-*s", width, host)
		if colour {
			label = sgrCyan + label + sgrReset
		}
		out.Lock()
		fmt.Printf("%s | %s\n", label, line)
		out.Unlock()
	}
	stream := func(host string, r io.Reader, wg *sync.WaitGroup) {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			emit(host, sc.Text())
		}
	}

	results := make([]eachResult, len(hosts))
	slots := make(chan struct{}, jobs)
	var all sync.WaitGroup
	for i, h := range hosts {
		all.Add(1)
		go func() {
			defer all.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			// -n: no host may read the terminal's input.
			argv := append([]string{"-n"}, sshOpts...)
			argv = append(append(argv, "--", h), command...)
			cmd := exec.Command("ssh", argv...)
			cmd.Env = env
			stdout, _ := cmd.StdoutPipe()
			stderr, _ := cmd.StderrPipe()
			status := 0
			if err := cmd.Start(); err != nil {
				emit(h, err.Error())
				status = 127
			} else {
				var pipes sync.WaitGroup
				pipes.Add(2)
				go stream(h, stdout, &pipes)
				go stream(h, stderr, &pipes)
				pipes.Wait()
				if err := cmd.Wait(); err != nil {
					status = 1
					if ee, ok := err.(*exec.ExitError); ok {
						status = exitStatus(ee)
					}
				}
			}
			results[i] = eachResult{h, status}
		}()
	}
	all.Wait()
	lastBlank = false

	var failed []string
	for _, r := range results {
		if r.status != 0 {
			failed = append(failed, fmt.Sprintf("  %s (exit status %d)", r.host, r.status))
		}
	}
	u := newUI(os.Stdout)
	if len(failed) == 0 {
		u.say("Done: all %d hosts succeeded.", len(hosts))
	} else {
		u.say("Warning: %d of %d hosts failed:\n%s", len(failed), len(hosts), strings.Join(failed, "\n"))
	}
	u.flush()
	if len(failed) > 0 {
		return 1
	}
	return 0
}
