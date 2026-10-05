package main

import (
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
)

const (
	hostEnvPrefix = "SSHC_PASSWORD_"
	keyEnvPrefix  = "SSHC_PASSPHRASE_"
)

// dest is a destination named on the command line: the name the user typed,
// and the host name and user ssh resolves it to.
type dest struct {
	alias, hostname, user string
	keys                  []string // identity files ssh would try; not passed to askpass
}

func encodeDests(ds []dest) string {
	var b strings.Builder
	for _, d := range ds {
		fmt.Fprintf(&b, "%s\t%s\t%s\n", d.alias, d.hostname, d.user)
	}
	return b.String()
}

func decodeDests(s string) []dest {
	var ds []dest
	for _, line := range strings.Split(s, "\n") {
		f := strings.Split(line, "\t")
		if len(f) == 3 && f[0] != "" {
			ds = append(ds, dest{alias: f[0], hostname: f[1], user: f[2]})
		}
	}
	return ds
}

// resolver finds the stored password for a prompt.
type resolver struct {
	cfg     *config
	dests   []dest
	environ []string // "KEY=value" pairs
}

func newResolver(cfg *config, dests []dest) *resolver {
	return &resolver{cfg: cfg, dests: dests, environ: os.Environ()}
}

func (r *resolver) getenv(name string) string {
	for _, kv := range r.environ {
		if k, v, ok := strings.Cut(kv, "="); ok && k == name {
			return v
		}
	}
	return ""
}

// envSuffix turns a host or key name into the tail of a variable name.
func envSuffix(key string) string {
	return strings.Map(func(c rune) rune {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			return c
		}
		return '_'
	}, key)
}

// specificEnv looks for <prefix><KEY>, where non-alphanumerics in key become
// "_" and case is ignored.
func (r *resolver) specificEnv(prefix, key string) (value, name string) {
	want := prefix + envSuffix(key)
	for _, kv := range r.environ {
		if k, v, ok := strings.Cut(kv, "="); ok && v != "" && strings.EqualFold(k, want) {
			return v, k
		}
	}
	return "", ""
}

// hasEnvPasswords reports whether any password or key passphrase is supplied
// by environment.
func (r *resolver) hasEnvPasswords() bool {
	for _, kv := range r.environ {
		k, v, _ := strings.Cut(kv, "=")
		if v == "" {
			continue
		}
		up := strings.ToUpper(k)
		if k == "SSHC_PASSWORD" || k == "SSHC_PASSPHRASE" ||
			strings.HasPrefix(up, hostEnvPrefix) || strings.HasPrefix(up, keyEnvPrefix) {
			return true
		}
	}
	return false
}

// activeProfile is the profile selected by $SSHC_PROFILE or the config file.
func (r *resolver) activeProfile() string {
	if p := r.getenv("SSHC_PROFILE"); p != "" {
		return p
	}
	p, _ := r.cfg.get("", "profile")
	return p
}

// normKeyPath makes key file paths comparable: "~" expanded, one kind of
// slash, lower case.
func normKeyPath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = strings.ReplaceAll(home, "\\", "/") + p[1:]
		}
	}
	return strings.ToLower(path.Clean(p))
}

// lookupPassphrase finds the passphrase for the private key file that ssh is
// asking about.
//
// Unlike a password, a passphrase is only used by the local ssh client to
// unlock a local file, so offering the wrong one discloses nothing. The
// general passphrase is therefore tried for any key.
func (r *resolver) lookupPassphrase(keyPath string) (passphrase, from, note string) {
	full := normKeyPath(keyPath)
	base := path.Base(full)
	if v, name := r.specificEnv(keyEnvPrefix, base); v != "" {
		return v, "environment variable " + name, ""
	}

	// [key NAME] sections: a full path beats a bare file name. ssh cuts the
	// path in its prompt off at 100 characters, hence the prefix match.
	if r.cfg != nil {
		var names []string
		for k := range r.cfg.entries {
			if section, key, _ := strings.Cut(k, "\n"); key == "passphrase" && strings.HasPrefix(section, "key ") {
				names = append(names, strings.TrimPrefix(section, "key "))
			}
		}
		sort.Strings(names)
		for _, exact := range []bool{true, false} {
			for _, name := range names {
				n := normKeyPath(name)
				var match bool
				if exact {
					match = n == full || len(keyPath) >= 100 && strings.HasPrefix(n, full)
				} else {
					match = !strings.Contains(n, "/") && n == base
				}
				if v, _ := r.cfg.get("key "+name, "passphrase"); match && v != "" {
					return v, fmt.Sprintf("[key %s] in %s", name, r.cfg.path), ""
				}
			}
		}
	}

	if v := r.getenv("SSHC_PASSPHRASE"); v != "" {
		return v, "environment variable SSHC_PASSPHRASE", ""
	}
	if profile := r.activeProfile(); profile != "" {
		if v, ok := r.cfg.get("profile "+profile, "passphrase"); ok && v != "" {
			return v, fmt.Sprintf("[profile %s] in %s", profile, r.cfg.path), ""
		}
	}
	return "", "", ""
}

// lookup finds the password for a prompt from user@host, and describes where
// it came from. note is set when a profile was selected but is unusable.
//
// A host-specific entry matches the name ssh reports or the alias the user
// typed for it. The general "active" password is only ever offered to a
// destination from the command line - never to a jump host or anything else
// that happens to ask.
func (r *resolver) lookup(user, host string) (password, from, note string) {
	names := []string{}
	isDest := false
	for _, d := range r.dests {
		if strings.EqualFold(d.hostname, host) || strings.EqualFold(d.alias, host) {
			isDest = true
			names = append(names, d.alias)
		}
	}
	names = append(names, host)

	var keys []string
	for _, n := range names {
		keys = append(keys, user+"@"+n, n)
	}
	for _, k := range keys {
		if v, name := r.specificEnv(hostEnvPrefix, k); v != "" {
			return v, "environment variable " + name, ""
		}
	}
	for _, k := range keys {
		if v, ok := r.cfg.get("host "+k, "password"); ok && v != "" {
			return v, fmt.Sprintf("[host %s] in %s", k, r.cfg.path), ""
		}
	}

	if !isDest {
		return "", "", ""
	}
	if v := r.getenv("SSHC_PASSWORD"); v != "" {
		return v, "environment variable SSHC_PASSWORD", ""
	}
	profile := r.activeProfile()
	if profile == "" {
		return "", "", ""
	}
	if v, ok := r.cfg.get("profile "+profile, "password"); ok && v != "" {
		return v, fmt.Sprintf("[profile %s] in %s", profile, r.cfg.path), ""
	}
	// A profile that only carries a key passphrase is not a mistake.
	if v, _ := r.cfg.get("profile "+profile, "passphrase"); v != "" {
		return "", "", ""
	}
	return "", "", fmt.Sprintf("active profile %q has no password in the config file", profile)
}
