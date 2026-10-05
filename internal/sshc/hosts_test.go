package sshc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sshHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("SSHC_CONFIG", filepath.Join(home, "none.conf"))
	dir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(filepath.Join(dir, "conf.d"), 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(name, text string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("config", "# comment\nHost w.go-2 web1\n  HostName 10.0.0.1\nhost=web2\nHost *.internal !bad ?x\n  User x\nInclude conf.d/*.conf\nHost \"quoted\"\n")
	write(filepath.Join("conf.d", "db.conf"), "Host db-prod\nHost web1\n")
	write("id_ed25519", "x")
	write("id_ed25519.pub", "x")
	write("orphan.pub", "x")
}

func TestSSHConfigHosts(t *testing.T) {
	sshHome(t)
	if got := strings.Join(sshConfigHosts(), " "); got != "db-prod quoted w.go-2 web1 web2" {
		t.Errorf("hosts = %q", got)
	}
}

func TestCompletions(t *testing.T) {
	sshHome(t)
	tests := []struct{ line, want string }{
		{"sshc w", "w.go-2 web1 web2"},
		{"sshc se", "set"},
		{"sshc W", "w.go-2 web1 web2"},
		{"sshc luke@d", "luke@db-prod"},
		{"sshc -p 2222 d", "db-prod"},
		{"sshc -", ""},
		{"sshc set --pa", "--passphrase"},
		{"sshc unset -H web", "web1 web2"},
		{"sshc set -k ", "id_ed25519"},
		{"sshc set -c ", ""},
		{"sshc set plainvalue", ""},
		{"sshc scp ./file we", "web1: web2:"},
		{"sshc scp ./lo", ""},
		{"sshc rsync -av dir/ d", "db-prod:"},
		{"sshc check q", "quoted"},
		{"sshc each web1 w", "w.go-2 web1 web2"},
		{"sshc run -d d", "db-prod"},
		{"sshc run git pu", ""},
		{"sshc shell-init ", "bash fish powershell zsh"},
		{"sshc list ", ""},
	}
	for _, tt := range tests {
		words := strings.Split(tt.line, " ")
		if got := strings.Join(completions(words, len(words)-1), " "); got != tt.want {
			t.Errorf("completions(%q) = %q; want %q", tt.line, got, tt.want)
		}
	}
	// The first word offers every command and every host.
	all := strings.Join(completions([]string{"sshc", ""}, 1), " ")
	for _, want := range []string{"set", "each", "ssh-add", "db-prod", "w.go-2"} {
		if !strings.Contains(" "+all+" ", " "+want+" ") {
			t.Errorf("first-word completions lack %q: %s", want, all)
		}
	}
	// A shell that drops the empty current word still gets an answer.
	if got := completions([]string{"sshc", "check"}, 2); len(got) != 5 {
		t.Errorf("missing current word: %q", got)
	}
}

func TestMatchHosts(t *testing.T) {
	hosts := []string{"db-prod", "web1", "web10", "web2"}
	tests := []struct{ in, want string }{
		{"2", "web1"},
		{"web1", "web1"},
		{"WEB1", "web1"},
		{"web", "web1 web10 web2"},
		{"prod", "db-prod"},
		{"9", ""},
		{"zzz", ""},
		{"0", "web10"},
	}
	for _, tt := range tests {
		if got := strings.Join(matchHosts(hosts, tt.in), " "); got != tt.want {
			t.Errorf("matchHosts(%q) = %q; want %q", tt.in, got, tt.want)
		}
	}
}
