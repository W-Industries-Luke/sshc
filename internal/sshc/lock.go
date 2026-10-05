package sshc

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// "sshc lock" switches sshc off: while locked it supplies no secret from any
// source, and connections fall back to asking. "sshc unlock" switches it back
// on after the operating system has verified who is at the keyboard - with
// Windows Hello, or the account password elsewhere.
//
// This is a switch that sshc honours, not a vault. It stops someone at an
// unlocked terminal, or a stray script, from using what is stored through
// sshc. It does not stop a program running as the user from reading the
// credential store itself, or from deleting the lock file.

// envStateDir overrides where the lock state is kept. For the tests.
const envStateDir = "SSHC_STATE_DIR"

// lockedNow is set by prepare when it finds sshc locked.
var lockedNow bool

// stateDir is the per-user directory for the lock file. It does not move
// with the config file, so that pointing sshc at another config is not a way
// around the lock.
func stateDir() string {
	if d := os.Getenv(envStateDir); d != "" {
		return d
	}
	p, err := userConfigPath()
	if err != nil {
		return ""
	}
	return filepath.Dir(p)
}

func lockFile() string     { return filepath.Join(stateDir(), "locked") }
func activityFile() string { return filepath.Join(stateDir(), "last-used") }

// parseLockAfter reads a "lock_after" setting such as 30m, 8h or 2d.
func parseLockAfter(s string) (time.Duration, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.ParseFloat(days, 64)
		if err != nil || n <= 0 {
			return 0, false
		}
		return time.Duration(n * 24 * float64(time.Hour)), true
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, false
	}
	return d, true
}

// isLocked reports whether sshc is locked, locking it first if the config
// asks for that after a period without use and the period has passed.
func isLocked(cfg *config) bool {
	if stateDir() == "" {
		return false
	}
	if exists(lockFile()) {
		return true
	}
	setting, _ := cfg.get("", "lock_after")
	limit, ok := parseLockAfter(setting)
	if !ok {
		return false
	}
	fi, err := os.Stat(activityFile())
	if err != nil {
		// Never used yet: the clock starts now.
		touchActivity()
		return false
	}
	if time.Since(fi.ModTime()) > limit {
		_ = writeLock("locked automatically after " + setting + " without use")
		return true
	}
	return false
}

// touchActivity records that sshc was just used or unlocked.
func touchActivity() {
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return
	}
	now := time.Now()
	if os.Chtimes(activityFile(), now, now) != nil {
		_ = os.WriteFile(activityFile(), nil, 0o600)
	}
}

func writeLock(reason string) error {
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return err
	}
	return os.WriteFile(lockFile(), []byte(time.Now().Format(time.RFC3339)+" "+reason+"\n"), 0o600)
}

// lockedNotice is the one line every command shows while sshc is locked.
const lockedNotice = "sshc is locked, so nothing stored is being used. Run \"sshc unlock\" to switch it back on."

func cmdLock(args []string) int {
	if len(args) > 0 {
		warnf("usage: %s lock", prog)
		return 1
	}
	u := newUI(os.Stdout)
	defer u.flush()
	if exists(lockFile()) {
		u.say("sshc is already locked. Run \"sshc unlock\" to switch it back on.")
		return 0
	}
	// Refuse to lock what could not be unlocked again.
	if err := canVerifyUser(); err != nil {
		failf(fmt.Sprintf("not locking: %v", err), "Unlocking needs your device to verify that it is you, and that is not available here.")
		return 1
	}
	if err := writeLock("locked by sshc lock"); err != nil {
		warnf("%v", err)
		return 1
	}
	u.say("Locked!")
	u.say("sshc will not supply any stored password, passphrase or code until you run \"sshc unlock\",\nwhich asks %s. Connections will ask you instead.", verifyMethod)
	return 0
}

func cmdUnlock(args []string) int {
	if len(args) > 0 {
		warnf("usage: %s unlock", prog)
		return 1
	}
	u := newUI(os.Stdout)
	defer u.flush()
	if !exists(lockFile()) {
		u.say("sshc is not locked.")
		return 0
	}
	if err := verifyUser(); err != nil {
		failf(fmt.Sprintf("still locked: %v", err),
			"If your device cannot verify you at all, the lock can be removed by deleting\n    "+lockFile())
		return 1
	}
	if err := os.Remove(lockFile()); err != nil {
		warnf("%v", err)
		return 1
	}
	touchActivity()
	u.say("Unlocked!")
	u.say("sshc is switched on again.")
	return 0
}

// sayIfLocked adds the reminder that sshc is locked to a command's output,
// so that a change made while locked does not look like it had no effect.
func sayIfLocked(u *ui) {
	cfgPath, _ := findConfig()
	if cfg, _ := loadConfig(cfgPath); isLocked(cfg) {
		u.say("Note: " + lockedNotice)
	}
}
