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

func TestExtractEntryFlags(t *testing.T) {
	tests := []struct {
		in         []string
		rest       string
		dir, entry string
		skip       bool
	}{
		{[]string{"box"}, "box", "", "", false},
		{[]string{"--dir", "/var/www", "box"}, "box", "/var/www", "", false},
		{[]string{"--dir=/var/www", "--entry", "make dev", "box"}, "box", "/var/www", "make dev", false},
		{[]string{"-p", "2222", "--entry=x", "-v", "box", "ls"}, "-p 2222 -v box ls", "", "x", false},
		{[]string{"--no-entry", "box"}, "box", "", "", true},
		{[]string{"-vp2222", "--dir", "~/app", "luke@box"}, "-vp2222 luke@box", "~/app", "", false},
		// After the destination everything is the remote command.
		{[]string{"box", "ls", "--dir", "/tmp"}, "box ls --dir /tmp", "", "", false},
		{[]string{"box", "--no-entry"}, "box --no-entry", "", "", false},
		// The value of an ssh option is not looked at.
		{[]string{"-o", "--dir", "box"}, "-o --dir box", "", "", false},
		{[]string{"-i", "key", "--dir", "/x", "box"}, "-i key box", "/x", "", false},
		{[]string{"--", "--dir"}, "-- --dir", "", "", false},
	}
	for _, tt := range tests {
		rest, in, err := extractEntryFlags(tt.in)
		if err != nil || strings.Join(rest, " ") != tt.rest || in.dir != tt.dir || in.entry != tt.entry || in.skip != tt.skip {
			t.Errorf("extractEntryFlags(%q) = %q, %+v, %v; want %q dir=%q entry=%q skip=%v", tt.in, rest, in, err, tt.rest, tt.dir, tt.entry, tt.skip)
		}
	}
	for _, bad := range [][]string{{"--dir"}, {"--entry="}, {"-v", "--dir"}, {"--dir", " ", "box"}} {
		if rest, _, err := extractEntryFlags(bad); err == nil {
			t.Errorf("extractEntryFlags(%q) = %q; want an error", bad, rest)
		}
	}
}

func TestWithEntryInline(t *testing.T) {
	// No config file and no terminal: only what is given on the command line.
	tests := []struct {
		args string
		in   inlineEntry
		want string
	}{
		{"box", inlineEntry{}, "box"},
		{"box uptime", inlineEntry{}, "box uptime"},
		{"box", inlineEntry{dir: "/var/www"}, `box cd '/var/www'; exec "$SHELL" -l`},
		{"-p 2222 box", inlineEntry{entry: "make dev"}, `-p 2222 box make dev; exec "$SHELL" -l`},
		{"box ls -la", inlineEntry{dir: "/var/www"}, `box cd '/var/www' && ls -la`},
		{"-v box make test", inlineEntry{dir: "~/app", entry: "source venv/bin/activate"}, `-v box cd ~/'app' && source venv/bin/activate && make test`},
		{"box ls", inlineEntry{dir: "/x", skip: true}, "box ls"},
	}
	for _, tt := range tests {
		got, err := withEntry(strings.Fields(tt.args), "", tt.in)
		// The added remote command is one argument; compare with it spelled out.
		if err != nil || strings.Join(got, " ") != tt.want {
			t.Errorf("withEntry(%q, %+v) = %q, %v; want %q", tt.args, tt.in, got, err, tt.want)
		}
	}
	got, _ := withEntry([]string{"box", "ls", "-la"}, "", inlineEntry{dir: "/x"})
	if len(got) != 2 {
		t.Errorf("the remote command should be a single argument: %q", got)
	}
	for _, bad := range []string{"-N box", "-W h:22 box", "box -v ls"} {
		if got, err := withEntry(strings.Fields(bad), "", inlineEntry{dir: "/x"}); err == nil {
			t.Errorf("withEntry(%q) with --dir = %q; want an error", bad, got)
		}
	}
	// SSHC_NO_ENTRY wins over everything.
	t.Setenv(envNoEntry, "1")
	if got, _ := withEntry([]string{"box"}, "", inlineEntry{dir: "/x"}); strings.Join(got, " ") != "box" {
		t.Errorf("SSHC_NO_ENTRY ignored: %q", got)
	}
}
