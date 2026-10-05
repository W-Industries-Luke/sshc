package sshc

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// --- sshc use ---------------------------------------------------------------

const useUsage = `Usage: sshc use [NAME]
       sshc use --save NAME
       sshc use --default

Chooses the active profile - the one "sshc set" stores into and whose
password and passphrase sshc offers.

  sshc use            show the active profile and the ones that exist
  sshc use NAME       switch this terminal to NAME (needs the shell hook)
  sshc use --default  switch this terminal back to the saved default
  sshc use --save NAME   make NAME the default for every terminal
`

func cmdUse(args []string) int {
	emit := os.Getenv(envEmit)
	out := os.Stdout
	if emit != "" {
		out = os.Stderr
	}
	save, reset, name := false, false, ""
	for _, a := range args {
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprint(out, useUsage)
			return 0
		case a == "-S" || a == "--save":
			save = true
		case a == "-d" || a == "--default":
			reset = true
		case strings.HasPrefix(a, "-") || name != "":
			fmt.Fprint(os.Stderr, useUsage)
			return 1
		default:
			name = a
		}
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
	names := profileNames()
	sort.Strings(names)
	u := newUI(out)
	defer u.flush()

	if name == "" && !reset {
		if save {
			fmt.Fprint(os.Stderr, useUsage)
			return 1
		}
		active := newResolver(cfg, nil).activeProfile()
		if active == "" {
			active = "(none yet)"
		}
		u.say("active profile: %s", active)
		if len(names) > 0 {
			u.say("Profiles:\n  %s", strings.Join(names, "\n  "))
		}
		return 0
	}
	if name != "" {
		known := false
		for _, n := range names {
			known = known || strings.EqualFold(n, name)
		}
		if !known {
			u.say("Note: there is no profile named %q yet. \"sshc set\" will create it.", name)
		}
	}
	if save {
		if cfgFile == "" {
			if cfgFile, err = defaultConfigTarget(); err == nil {
				err = createConfig(cfgFile)
			}
		}
		var lines []string
		if err == nil {
			lines, err = readConfigLines(cfgFile)
		}
		if err == nil {
			err = writeConfig(cfgFile, setConfigValue(lines, "", "profile", name))
		}
		if err != nil {
			warnf("%v", err)
			return 1
		}
		u.say("Updated!")
		u.say("%s is now the default profile, in %s", name, cfgFile)
		return 0
	}
	var code string
	var ok bool
	if reset {
		code, ok = emitUnset(emit, "SSHC_PROFILE")
	} else {
		code, ok = emitAssignment(emit, "SSHC_PROFILE", name)
	}
	if !ok {
		return hookMissing("switching the profile of this terminal")
	}
	fmt.Println(code)
	u.say("Updated!")
	if reset {
		u.say("This terminal is back on the saved default profile.")
	} else {
		u.say("This terminal now uses the profile %s.", name)
	}
	return 0
}

// --- sshc migrate -----------------------------------------------------------

// cmdMigrate moves every plain-text secret of the config file into the
// credential store.
func cmdMigrate(args []string) int {
	if len(args) > 0 {
		warnf("usage: %s migrate", prog)
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
	u := newUI(os.Stdout)
	defer u.flush()
	type entry struct{ section, key, value string }
	var plain []entry
	if cfg != nil {
		for k, v := range cfg.entries {
			section, key, _ := strings.Cut(k, "\n")
			kind, _, _ := strings.Cut(section, " ")
			if v != "" && v != storeMarker && (key == "password" || key == "passphrase") && (kind == "profile" || kind == "host" || kind == "key") {
				plain = append(plain, entry{section, key, v})
			}
		}
	}
	if len(plain) == 0 {
		u.say("Nothing to move: no secret is stored in plain text.")
		return 0
	}
	st, err := systemStore()
	if err != nil {
		warnf("no credential store is available here (%v); nothing was changed", err)
		return 1
	}
	lines, err := readConfigLines(cfgFile)
	if err != nil {
		warnf("%v", err)
		return 1
	}
	sort.Slice(plain, func(i, j int) bool { return plain[i].section+plain[i].key < plain[j].section+plain[j].key })
	var moved []string
	for _, e := range plain {
		if err := st.set(storeKey(e.section, e.key), e.value); err != nil {
			warnf("could not save the %s of [%s] to %s: %v", e.key, e.section, st.name(), err)
			break
		}
		lines = setConfigValue(lines, e.section, e.key, storeMarker)
		moved = append(moved, fmt.Sprintf("  %s of [%s]", e.key, e.section))
	}
	if len(moved) == 0 {
		return 1
	}
	if err := writeConfig(cfgFile, lines); err != nil {
		warnf("%v", err)
		return 1
	}
	u.say("Updated!")
	u.say("Moved to %s:\n%s", st.name(), strings.Join(moved, "\n"))
	if len(moved) < len(plain) {
		return 1
	}
	return 0
}

// --- sshc update ------------------------------------------------------------

const releaseBase = "https://github.com/W-Industries-Luke/sshc/releases"

// curl fetches a URL with the system's curl, which ships with Windows, macOS
// and practically every Linux. Using it keeps a TLS stack out of sshc itself.
func curl(args ...string) ([]byte, error) {
	cmd := exec.Command("curl", append([]string{"-fsSL", "--retry", "2"}, args...)...)
	cmd.Stderr = os.Stderr
	return cmd.Output()
}

// latestVersion asks GitHub which release is the latest.
func latestVersion() (string, error) {
	out, err := curl("-o", os.DevNull, "-w", "%{url_effective}", releaseBase+"/latest")
	if err != nil {
		return "", errors.New("could not reach GitHub to look for a newer version")
	}
	tag := filepath.Base(strings.TrimSpace(string(out)))
	if !strings.HasPrefix(tag, "v") || strings.Count(tag, ".") != 2 {
		return "", fmt.Errorf("unexpected answer from GitHub: %s", strings.TrimSpace(string(out)))
	}
	return strings.TrimPrefix(tag, "v"), nil
}

// newer reports whether version a is higher than b (both "x.y.z").
func newer(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		var na, nb int
		if i < len(pa) {
			fmt.Sscanf(pa[i], "%d", &na)
		}
		if i < len(pb) {
			fmt.Sscanf(pb[i], "%d", &nb)
		}
		if na != nb {
			return na > nb
		}
	}
	return false
}

// replaceFile puts replacement where target is. A running program cannot be
// overwritten on Windows, but it can be renamed out of the way.
func replaceFile(replacement, target string) error {
	if runtime.GOOS != "windows" {
		return os.Rename(replacement, target)
	}
	old := target + ".old"
	_ = os.Remove(old)
	if err := os.Rename(target, old); err != nil {
		return err
	}
	if err := os.Rename(replacement, target); err != nil {
		_ = os.Rename(old, target)
		return err
	}
	return nil
}

func cmdUpdate(args []string) int {
	checkOnly := false
	for _, a := range args {
		switch a {
		case "-c", "--check":
			checkOnly = true
		default:
			warnf("usage: %s update [--check]", prog)
			return 1
		}
	}
	self, err := os.Executable()
	if err != nil {
		warnf("could not locate my own executable: %v", err)
		return 1
	}
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	// Leftover from an earlier update on Windows.
	_ = os.Remove(self + ".old")

	latest, err := latestVersion()
	if err != nil {
		warnf("%v", err)
		return 1
	}
	u := newUI(os.Stdout)
	defer u.flush()
	if !newer(latest, version) {
		u.say("%s %s is the latest version.", prog, version)
		return 0
	}
	if checkOnly {
		u.say("%s %s is available (this is %s). Run \"sshc update\" to install it.", prog, latest, version)
		return 0
	}
	if managedDir(filepath.Dir(self)) {
		u.say("%s %s is available (this is %s).", prog, latest, version)
		u.say("Note: this copy was installed by a package manager, so update it there:\n  scoop update sshc\n  brew upgrade sshc\n  winget upgrade LukeWeaver.sshc")
		return 0
	}

	file := fmt.Sprintf("sshc-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		file += ".exe"
	}
	base := releaseBase + "/download/v" + latest + "/"
	tmp, err := os.CreateTemp(filepath.Dir(self), ".sshc-update-*")
	if err != nil {
		warnf("cannot write to %s: %v", filepath.Dir(self), err)
		return 1
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	if _, err := curl("-o", tmp.Name(), base+file); err != nil {
		warnf("could not download %s", base+file)
		return 1
	}
	sums, err := curl(base + "SHA256SUMS")
	if err != nil {
		warnf("could not download the checksums; nothing was changed")
		return 1
	}
	want := ""
	for _, line := range strings.Split(string(sums), "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[1] == file {
			want = f[0]
		}
	}
	data, err := os.ReadFile(tmp.Name())
	if err != nil {
		warnf("%v", err)
		return 1
	}
	sum := sha256.Sum256(data)
	if want == "" || !strings.EqualFold(want, hex.EncodeToString(sum[:])) {
		warnf("the download does not match its published checksum; nothing was changed")
		return 1
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		warnf("%v", err)
		return 1
	}
	if err := replaceFile(tmp.Name(), self); err != nil {
		warnf("could not replace %s: %v", self, err)
		return 1
	}
	u.say("Updated!")
	u.say("%s %s -> %s, at %s", prog, version, latest, self)
	u.say("Open a new terminal so that the shell hook of the new version is loaded.")
	return 0
}

// --- audit log --------------------------------------------------------------

// auditLog appends one line saying that sshc answered (or failed to answer) a
// prompt, if the config file asks for a log. It never records a secret.
func auditLog(cfg *config, event, what, from string) {
	target, _ := cfg.get("", "log")
	if target == "" || isNo(target) {
		return
	}
	switch strings.ToLower(target) {
	case "yes", "true", "on", "1":
		target = filepath.Join(filepath.Dir(cfg.path), "sshc.log")
	}
	f, err := os.OpenFile(target, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		debugf("cannot write the log %s: %v", target, err)
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s\t%s\t%s\t%s\t%s\n", time.Now().Format(time.RFC3339), event, what, from, os.Getenv(envTool))
}
