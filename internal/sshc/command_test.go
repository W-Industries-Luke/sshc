package sshc

import (
	"reflect"
	"runtime"
	"testing"
)

func TestSplitCommand(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{`pass show ssh/work`, []string{"pass", "show", "ssh/work"}},
		{`op read "op://Work/my server/password"`, []string{"op", "read", "op://Work/my server/password"}},
		{`bw get password 'item name'`, []string{"bw", "get", "password", "item name"}},
		{`  spaced   out  `, []string{"spaced", "out"}},
		{`say "a \"quoted\" word"`, []string{"say", `a "quoted" word`}},
		{`C:\Tools\pm.exe get "C:\Users\Luke Weaver\db"`, []string{`C:\Tools\pm.exe`, "get", `C:\Users\Luke Weaver\db`}},
		{`a "" b`, []string{"a", "", "b"}},
		{`x'y'"z"`, []string{"xyz"}},
		// No shell: these are just characters.
		{`echo $HOME; rm -rf * | cat`, []string{"echo", "$HOME;", "rm", "-rf", "*", "|", "cat"}},
	}
	for _, tt := range tests {
		got, err := splitCommand(tt.in)
		if err != nil || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("splitCommand(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
	for _, bad := range []string{"", "   ", `unterminated "quote`, `'open`} {
		if got, err := splitCommand(bad); err == nil {
			t.Errorf("splitCommand(%q) = %q; want an error", bad, got)
		}
	}
}

func TestExpandVars(t *testing.T) {
	vars := map[byte]string{'h': "box; rm -rf /", 'u': "luke"}
	tests := []struct{ in, want string }{
		{"ssh/%h", "ssh/box; rm -rf /"},
		{"%u@%h", "luke@box; rm -rf /"},
		{"100%%", "100%"},
		{"%k stays", "%k stays"},
		{"trailing %", "trailing %"},
		{"plain", "plain"},
	}
	for _, tt := range tests {
		if got := expandVars(tt.in, vars); got != tt.want {
			t.Errorf("expandVars(%q) = %q; want %q", tt.in, got, tt.want)
		}
	}
}

func TestSecretFromCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses printf")
	}
	cfg := &config{path: "sshc.conf", entries: map[string]string{
		configKey("", "profile"):                      "work",
		configKey("profile work", "password_command"): `printf "%s\nsecond line" "pw for %u at %h"`,
		configKey("key id_x", "passphrase_command"):   `printf %s "pp for %k"`,
		configKey("host broken", "password_command"):  "no-such-program-anywhere",
		configKey("host silent", "password_command"):  "true",
		configKey("profile both", "password"):         "plain wins",
		configKey("profile both", "password_command"): "false",
	}}
	dests := []dest{{alias: "box", hostname: "10.0.0.5"}, {alias: "broken", hostname: "10.0.0.6"}, {alias: "silent", hostname: "10.0.0.7"}}
	r := &resolver{cfg: cfg, dests: dests}

	if pw, from, note := r.lookup("luke", "10.0.0.5"); pw != "pw for luke at 10.0.0.5" || note != "" || from == "" {
		t.Errorf("password command = %q from %q note %q", pw, from, note)
	}
	if pp, _, _ := r.lookupPassphrase("/home/luke/.ssh/id_x"); pp != "pp for /home/luke/.ssh/id_x" {
		t.Errorf("passphrase command = %q", pp)
	}
	for _, host := range []string{"10.0.0.6", "10.0.0.7"} {
		if pw, _, note := r.lookup("luke", host); pw != "" || note == "" {
			t.Errorf("failing command for %s: password %q, note %q; want a note only", host, pw, note)
		}
	}
	r2 := &resolver{cfg: cfg, dests: dests, environ: []string{"SSHC_PROFILE=both"}}
	if pw, _, _ := r2.lookup("luke", "10.0.0.5"); pw != "plain wins" {
		t.Errorf("a stored value should win over a command, got %q", pw)
	}
	// check must describe the command without running it.
	dry := &resolver{cfg: cfg, dests: dests, dryRun: true}
	if pw, from, note := dry.lookup("luke", "10.0.0.6"); pw != notRun || note != "" || from == "" {
		t.Errorf("dry run = %q from %q note %q", pw, from, note)
	}
}
