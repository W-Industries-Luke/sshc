package sshc

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ownCommands are the first words that select one of sshc's own commands or
// a wrapped tool.
var ownCommands = []string{
	"set", "unset", "list", "check", "hosts", "pick", "each", "run",
	"install", "init", "shell-init", "help", "version",
	"ssh", "scp", "sftp", "rsync", "ssh-copy-id", "ssh-add",
}

var setOptions = []string{
	"--session", "--passphrase", "--host", "--key", "--profile", "--plain", "--command", "--no-clear", "--help",
}

// privateKeys lists the key files in ~/.ssh by name.
func privateKeys() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	pubs, _ := filepath.Glob(filepath.Join(home, ".ssh", "*.pub"))
	var keys []string
	for _, p := range pubs {
		if key := strings.TrimSuffix(p, ".pub"); exists(key) {
			keys = append(keys, filepath.Base(key))
		}
	}
	return keys
}

// profileNames lists the profiles of the config file.
func profileNames() []string {
	cfgPath, _ := findConfig()
	cfg, _ := loadConfig(cfgPath)
	if cfg == nil {
		return nil
	}
	seen := map[string]bool{}
	var names []string
	for k := range cfg.entries {
		section, _, _ := strings.Cut(k, "\n")
		if name, ok := strings.CutPrefix(section, "profile "); ok && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

// completions returns the candidates for the word at index cword of words,
// where words[0] is "sshc". An empty result leaves the shell to complete file
// names.
func completions(words []string, cword int) []string {
	cur := ""
	if cword < len(words) {
		cur = words[cword]
	}
	prev := ""
	if cword >= 1 && cword-1 < len(words) {
		prev = words[cword-1]
	}
	command := ""
	if cword >= 2 {
		command = words[1]
	}
	hosts := sshConfigHosts()

	var candidates []string
	switch {
	case cword <= 1:
		candidates = append(append([]string{}, ownCommands...), hosts...)
	case command == "set" || command == "unset":
		switch prev {
		case "-H", "--host":
			candidates = hosts
		case "-k", "--key":
			candidates = privateKeys()
		case "-P", "--profile":
			candidates = profileNames()
		case "-c", "--command":
		default:
			if strings.HasPrefix(cur, "-") {
				candidates = setOptions
			}
		}
	case command == "scp" || command == "rsync":
		// A remote operand is "host:path"; anything path-like is local.
		if !strings.HasPrefix(cur, "-") && !strings.ContainsAny(cur, "/\\.~:") {
			for _, h := range hosts {
				candidates = append(candidates, h+":")
			}
		}
	case command == "list" || command == "hosts" || command == "pick" || command == "init" ||
		command == "install" || command == "help" || command == "version" || command == "ssh-add":
	case command == "shell-init":
		candidates = []string{"bash", "zsh", "fish", "powershell"}
	case command == "run":
		if prev == "-d" || prev == "--dest" {
			candidates = hosts
		}
	default:
		// ssh, sftp, ssh-copy-id, check, each, or a host given directly.
		if !strings.HasPrefix(cur, "-") {
			candidates = hosts
		}
	}

	// "user@" in front of a host is kept while the host is completed.
	prefix := ""
	if i := strings.LastIndex(cur, "@"); i >= 0 && !strings.HasPrefix(cur, "-") {
		prefix, cur = cur[:i+1], cur[i+1:]
	}
	var out []string
	for _, c := range candidates {
		if strings.HasPrefix(strings.ToLower(c), strings.ToLower(cur)) {
			out = append(out, prefix+c)
		}
	}
	sort.Strings(out)
	return compactStrings(out)
}

// cmdComplete is called by the shell hook: "sshc --complete CWORD WORDS...".
func cmdComplete(args []string) int {
	if len(args) < 1 {
		return 1
	}
	cword, err := strconv.Atoi(args[0])
	if err != nil {
		return 1
	}
	for _, c := range completions(args[1:], cword) {
		fmt.Println(c)
	}
	return 0
}
