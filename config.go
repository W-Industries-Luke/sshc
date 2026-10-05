package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const configName = "sshc.conf"

const configTemplate = `# sshc configuration.
#
# This file holds passwords in plain text. On Linux and macOS sshc refuses to
# read it unless it is owned by you and private (chmod 600). Never commit it.
#
# Values run to the end of the line and are taken literally - there are no
# escapes and no inline comments, so "#" and "=" are fine inside a password.
# Wrap a value in quotes only if it starts or ends with a space.

# "sshc set" edits this file for you.

# "sshc set <password>" clears the screen afterwards; "no" turns that off.
#clear_on_set = no

# The active profile. $SSHC_PROFILE overrides this per shell.
#profile = work

# A profile can hold a login password, the passphrase of your SSH key, or both.
#[profile work]
#password = change-me
#passphrase = change-me

#[profile home]
#password = change-me

# A [host] section always wins over the active profile, and is the only way to
# answer a jump host. Use the name you type or the real host name, optionally
# narrowed with "user@".
#[host w.go-2]
#password = change-me

# A [key] section is the passphrase of one key, by file name or full path.
#[key id_ed25519]
#passphrase = change-me
`

// config is a parsed config file. A nil *config is valid and empty.
type config struct {
	path    string
	entries map[string]string
}

func configKey(section, key string) string {
	return strings.ToLower(strings.Join(strings.Fields(section), " ")) + "\n" + strings.ToLower(key)
}

// get looks up key in section; both match case-insensitively.
func (c *config) get(section, key string) (string, bool) {
	if c == nil {
		return "", false
	}
	v, ok := c.entries[configKey(section, key)]
	return v, ok
}

// installDir is the directory holding the real sshc executable.
func installDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Dir(exe), nil
}

func userConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, prog, configName), nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// findConfig returns the config file to use, or "" when there is none.
func findConfig() (string, error) {
	if p := os.Getenv("SSHC_CONFIG"); p != "" {
		if !exists(p) {
			return "", fmt.Errorf("SSHC_CONFIG points at %s, which does not exist", p)
		}
		return p, nil
	}
	if dir, err := installDir(); err == nil {
		if p := filepath.Join(dir, configName); exists(p) {
			return p, nil
		}
	}
	if p, err := userConfigPath(); err == nil && exists(p) {
		return p, nil
	}
	return "", nil
}

// parseConfig reads the file as data only; nothing in it is ever evaluated.
// Unrecognised lines are reported by number, never by content, since they may
// well be a mistyped password.
func parseConfig(r io.Reader) (entries map[string]string, badLines []int) {
	entries = map[string]string{}
	section := ""
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "" || line[0] == '#' || line[0] == ';':
		case line[0] == '[' && line[len(line)-1] == ']':
			section = line[1 : len(line)-1]
		case strings.Contains(line, "="):
			k, v, _ := strings.Cut(line, "=")
			v = strings.TrimSpace(v)
			// One pair of surrounding quotes keeps leading/trailing spaces.
			if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
				v = v[1 : len(v)-1]
			}
			entries[configKey(section, strings.TrimSpace(k))] = v
		default:
			badLines = append(badLines, n)
		}
	}
	return entries, badLines
}

// loadConfig checks that path is safe to trust and parses it. An empty path
// yields an empty config.
func loadConfig(path string) (*config, error) {
	if path == "" {
		return nil, nil
	}
	if err := checkConfigSecure(path); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	entries, bad := parseConfig(f)
	for _, n := range bad {
		warnf("%s: ignoring line %d (expected \"key = value\" or \"[section]\")", path, n)
	}
	return &config{path: path, entries: entries}, nil
}

// defaultConfigTarget is where a new config file goes: $SSHC_CONFIG, else
// next to the executable, else the user config directory.
func defaultConfigTarget() (string, error) {
	if p := os.Getenv("SSHC_CONFIG"); p != "" {
		return p, nil
	}
	if dir, err := installDir(); err == nil && dirWritable(dir) {
		return filepath.Join(dir, configName), nil
	}
	return userConfigPath()
}

// createConfig writes the template to target, which must not exist yet.
func createConfig(target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("%s already exists; not overwriting it", target)
	}
	if err != nil {
		return err
	}
	_, err = f.WriteString(configTemplate)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

func cmdInit() int {
	target, err := defaultConfigTarget()
	if err == nil {
		err = createConfig(target)
	}
	if err != nil {
		warnf("%v", err)
		return 1
	}
	fmt.Printf("Created %s\n", target)
	return 0
}

func dirWritable(dir string) bool {
	f, err := os.CreateTemp(dir, ".sshc-probe-*")
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(f.Name())
	return true
}
