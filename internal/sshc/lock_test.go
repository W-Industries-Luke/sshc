package sshc

import (
	"os"
	"testing"
	"time"
)

func TestParseLockAfter(t *testing.T) {
	good := map[string]time.Duration{"30m": 30 * time.Minute, "8h": 8 * time.Hour, "2d": 48 * time.Hour, " 1H ": time.Hour, "0.5d": 12 * time.Hour, "90s": 90 * time.Second}
	for in, want := range good {
		if got, ok := parseLockAfter(in); !ok || got != want {
			t.Errorf("parseLockAfter(%q) = %v, %v; want %v", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "yes", "8", "-1h", "0m", "d", "xd"} {
		if got, ok := parseLockAfter(bad); ok {
			t.Errorf("parseLockAfter(%q) = %v; want a rejection", bad, got)
		}
	}
}

func TestIsLocked(t *testing.T) {
	t.Setenv(envStateDir, t.TempDir())
	plain := &config{entries: map[string]string{}}
	timed := &config{entries: map[string]string{configKey("", "lock_after"): "1h"}}

	if isLocked(plain) || isLocked(nil) {
		t.Fatal("locked without a lock file")
	}
	// The inactivity clock starts at first use.
	if isLocked(timed) {
		t.Fatal("locked on first use")
	}
	if _, err := os.Stat(activityFile()); err != nil {
		t.Fatalf("first use should start the clock: %v", err)
	}
	if isLocked(timed) {
		t.Fatal("locked although just used")
	}
	// Two hours without use.
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(activityFile(), old, old); err != nil {
		t.Fatal(err)
	}
	if isLocked(plain) {
		t.Fatal("the limit applied without being configured")
	}
	if !isLocked(timed) {
		t.Fatal("not locked after the limit passed")
	}
	// Once locked it stays locked, whatever the config now says.
	if !isLocked(plain) {
		t.Fatal("the automatic lock did not persist")
	}
	if err := os.Remove(lockFile()); err != nil {
		t.Fatal(err)
	}
	touchActivity()
	if isLocked(timed) {
		t.Fatal("locked right after an unlock")
	}
	if err := writeLock("test"); err != nil || !isLocked(nil) {
		t.Fatalf("explicit lock: %v", err)
	}
}
