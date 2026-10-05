package sshc

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/term"
)

// sshConfigHosts returns the host names defined in the user's ssh config:
// every word of a "Host" line that is a plain name rather than a pattern.
// "Include" lines are followed.
func sshConfigHosts() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	dir := filepath.Join(home, ".ssh")
	seen := map[string]bool{}
	var hosts []string
	var read func(file string, depth int)
	read = func(file string, depth int) {
		f, err := os.Open(file)
		if err != nil || depth > 5 {
			return
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			keyword, rest, _ := strings.Cut(strings.Replace(line, "=", " ", 1), " ")
			fields := strings.Fields(strings.ReplaceAll(rest, `"`, ""))
			switch strings.ToLower(keyword) {
			case "host":
				for _, h := range fields {
					if !strings.ContainsAny(h, "*?!") && !seen[h] {
						seen[h] = true
						hosts = append(hosts, h)
					}
				}
			case "include":
				for _, pattern := range fields {
					if strings.HasPrefix(pattern, "~/") {
						pattern = filepath.Join(home, pattern[2:])
					} else if !filepath.IsAbs(pattern) {
						pattern = filepath.Join(dir, pattern)
					}
					matches, _ := filepath.Glob(pattern)
					for _, m := range matches {
						read(m, depth+1)
					}
				}
			}
		}
	}
	read(filepath.Join(dir, "config"), 0)
	sort.Strings(hosts)
	return hosts
}

// hostNote says what is stored specifically for a host, by the name used in
// the ssh config.
func hostNote(r *resolver, host string) string {
	if _, name := r.specificEnv(hostEnvPrefix, host); name != "" {
		return "own password, from " + name
	}
	if r.has("host "+host, "password") {
		return "own stored password"
	}
	return ""
}

// cmdHosts lists the hosts of the ssh config.
func cmdHosts(args []string) int {
	if len(args) > 0 {
		warnf("usage: %s hosts", prog)
		return 1
	}
	hosts := sshConfigHosts()
	u := newUI(os.Stdout)
	defer u.flush()
	if len(hosts) == 0 {
		u.say("No hosts are defined in ~/.ssh/config.")
		return 0
	}
	cfgPath, _ := findConfig()
	cfg, _ := loadConfig(cfgPath)
	r := newResolver(cfg, nil)
	rows := make([]string, len(hosts))
	for i, h := range hosts {
		rows[i] = strings.TrimRight(fmt.Sprintf("  %-28s %s", h, hostNote(r, h)), " ")
	}
	u.say("Hosts in ~/.ssh/config:\n%s", strings.Join(rows, "\n"))
	return 0
}

// matchHosts narrows hosts by what the user typed: a number from the list,
// an exact name, or any part of a name.
func matchHosts(hosts []string, input string) []string {
	var n int
	if _, err := fmt.Sscanf(input, "%d", &n); err == nil && fmt.Sprint(n) == input && n >= 1 && n <= len(hosts) {
		return []string{hosts[n-1]}
	}
	var out []string
	for _, h := range hosts {
		if strings.EqualFold(h, input) {
			return []string{h}
		}
		if strings.Contains(strings.ToLower(h), strings.ToLower(input)) {
			out = append(out, h)
		}
	}
	return out
}

// cmdPick shows the hosts of the ssh config and connects to the chosen one.
func cmdPick(args []string) int {
	if len(args) > 0 {
		warnf("usage: %s pick", prog)
		return 1
	}
	hosts := sshConfigHosts()
	if len(hosts) == 0 {
		warnf("no hosts are defined in ~/.ssh/config; nothing to pick from")
		return 1
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		warnf("pick needs a terminal to ask on")
		return 1
	}
	in := bufio.NewReader(os.Stdin)
	for {
		rows := make([]string, len(hosts))
		for i, h := range hosts {
			rows[i] = fmt.Sprintf("  %3d  %s", i+1, h)
		}
		u := newUI(os.Stdout)
		u.say("%s", strings.Join(rows, "\n"))
		u.flush()
		fmt.Print("Connect to (number or part of a name, Enter to cancel): ")
		lastBlank = false
		line, err := in.ReadString('\n')
		input := strings.TrimSpace(line)
		if err != nil || input == "" {
			return 0
		}
		switch matches := matchHosts(hosts, input); len(matches) {
		case 0:
			warnf("nothing matches %q", input)
		case 1:
			return runTool("ssh", matches)
		default:
			hosts = matches
		}
	}
}
