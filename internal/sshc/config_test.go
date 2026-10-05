package sshc

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseConfig(t *testing.T) {
	entries, bad := parseConfig(strings.NewReader(`
# comment
; also a comment
profile = Work
[profile  work]
password = p@ss = #1 ; not a comment
[Host w.go-2]
password = "  spaced  "
user = ignored
[host quoted]
password = 'x'
stray line
[host oneq]
password = "
`))
	c := &config{entries: entries}
	want := []struct{ section, key, value string }{
		{"", "profile", "Work"},
		{"profile work", "password", "p@ss = #1 ; not a comment"},
		{"PROFILE WORK", "Password", "p@ss = #1 ; not a comment"},
		{"host W.GO-2", "password", "  spaced  "},
		{"host quoted", "password", "x"},
		{"host oneq", "password", `"`},
	}
	for _, w := range want {
		if got, ok := c.get(w.section, w.key); !ok || got != w.value {
			t.Errorf("get(%q, %q) = %q, %v; want %q", w.section, w.key, got, ok, w.value)
		}
	}
	if _, ok := c.get("host missing", "password"); ok {
		t.Error("missing section should not be found")
	}
	if !reflect.DeepEqual(bad, []int{12}) {
		t.Errorf("bad lines = %v; want [12]", bad)
	}
	var none *config
	if _, ok := none.get("", "profile"); ok {
		t.Error("nil config should be empty")
	}
}

func TestManagedDir(t *testing.T) {
	tests := []struct {
		dir  string
		want bool
	}{
		{"/home/luke/.local/bin", false},
		{`C:\Users\Luke\AppData\Local\Programs\sshc`, false},
		{"/opt/homebrew/Cellar/sshc/0.1.1/bin", true},
		{"/home/linuxbrew/.linuxbrew/Cellar/sshc/0.1.1/bin", true},
		{`C:\Users\Luke\scoop\apps\sshc\current`, true},
		{`C:\Users\Luke\AppData\Local\Microsoft\WinGet\Packages\LukeWeaver.sshc_Microsoft.Winget.Source_8wekyb3d8bbwe`, true},
		{"/nix/store/abc-sshc-0.1.1/bin", true},
	}
	for _, tt := range tests {
		if got := managedDir(tt.dir); got != tt.want {
			t.Errorf("managedDir(%q) = %v; want %v", tt.dir, got, tt.want)
		}
	}
}
