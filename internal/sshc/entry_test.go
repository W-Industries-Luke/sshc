package sshc

import (
	"strings"
	"testing"
)

func TestEntryCommand(t *testing.T) {
	tests := []struct{ dir, entry, want string }{
		{"/var/www", "", `cd '/var/www'; exec "$SHELL" -l`},
		{"", "source ~/venv/bin/activate", `source ~/venv/bin/activate; exec "$SHELL" -l`},
		{"/var/www", "make dev", `cd '/var/www' && make dev; exec "$SHELL" -l`},
		{"~/projects/my app", "", `cd ~/'projects/my app'; exec "$SHELL" -l`},
		{"~", "", `cd ~; exec "$SHELL" -l`},
		// A path cannot break out of its quotes.
		{"/tmp/x'; rm -rf ~; echo '", "", `cd '/tmp/x'\''; rm -rf ~; echo '\'''; exec "$SHELL" -l`},
		{"$(reboot)", "", `cd '$(reboot)'; exec "$SHELL" -l`},
	}
	for _, tt := range tests {
		if got := entryCommand(tt.dir, tt.entry); got != tt.want {
			t.Errorf("entryCommand(%q, %q) =\n  %s\nwant\n  %s", tt.dir, tt.entry, got, tt.want)
		}
	}
}

func TestInteractiveLogin(t *testing.T) {
	yes := []string{"box", "luke@box", "-p 2222 box", "-v -A box", "-i key -o User=x box", "-L 8080:localhost:80 box", "-J jump box", "-vp2222 box"}
	no := []string{
		"box uptime", "box -N", "-N box", "-W host:22 box", "-T box", "-fN box", "-n box", "-s box sftp",
		"-O check box", "-o RemoteCommand=x box", "-oRequestTTY=no box", "-o SessionType=none box",
		"", "-V", "-p 2222", "-vN box", "-L 1:h:2 -N box",
	}
	for _, line := range yes {
		if !interactiveLogin(strings.Fields(line)) {
			t.Errorf("%q should count as an interactive login", line)
		}
	}
	for _, line := range no {
		if interactiveLogin(strings.Fields(line)) {
			t.Errorf("%q should not count as an interactive login", line)
		}
	}
	// A value that happens to contain flag letters is not a flag.
	if !interactiveLogin([]string{"-i", "Nfs-key", "box"}) || !interactiveLogin([]string{"-iNfs", "box"}) {
		t.Error("an option value was read as flags")
	}
}

func TestLoginSettings(t *testing.T) {
	cfg := &config{entries: map[string]string{
		configKey("host w.go-2", "directory"):  "/var/www",
		configKey("host root@w.go-2", "entry"): "echo root",
		configKey("host 10.0.0.9", "entry"):    "echo nine",
	}}
	r := &resolver{cfg: cfg, dests: []dest{{alias: "w.go-2", hostname: "10.0.0.6"}, {alias: "nine", hostname: "10.0.0.9"}}}
	if dir, entry := r.loginSettings("luke", "10.0.0.6"); dir != "/var/www" || entry != "" {
		t.Errorf("by alias: %q %q", dir, entry)
	}
	if dir, entry := r.loginSettings("root", "10.0.0.6"); dir != "/var/www" || entry != "echo root" {
		t.Errorf("user-specific entry with host directory: %q %q", dir, entry)
	}
	if dir, entry := r.loginSettings("luke", "10.0.0.9"); dir != "" || entry != "echo nine" {
		t.Errorf("by host name: %q %q", dir, entry)
	}
	if !hasLoginSettings(cfg) || hasLoginSettings(&config{entries: map[string]string{configKey("host x", "password"): "p"}}) || hasLoginSettings(nil) {
		t.Error("hasLoginSettings is wrong")
	}
}
