package sshc

import (
	"reflect"
	"strings"
	"testing"
)

func TestLookup(t *testing.T) {
	cfg := &config{path: "sshc.conf", entries: map[string]string{
		configKey("", "profile"):               "work",
		configKey("profile work", "password"):  "work-pw",
		configKey("profile home", "password"):  "home-pw",
		configKey("host jump", "password"):     "jump-pw",
		configKey("host root@box", "password"): "root-pw",
		configKey("host 10.0.0.9", "password"): "nine-pw",
	}}
	dests := []dest{
		{alias: "box", hostname: "10.0.0.5"},
		{alias: "w.go-2", hostname: "10.0.0.6"},
		{alias: "nine", hostname: "10.0.0.9"},
	}
	tests := []struct {
		name       string
		env        []string
		cfg        *config
		user, host string
		want, from string
	}{
		{"profile from file", nil, cfg, "luke", "10.0.0.5", "work-pw", "[profile work]"},
		{"SSHC_PROFILE overrides file", []string{"SSHC_PROFILE=home"}, cfg, "luke", "10.0.0.5", "home-pw", "[profile home]"},
		{"SSHC_PASSWORD beats profile", []string{"SSHC_PASSWORD=env-pw"}, cfg, "luke", "10.0.0.5", "env-pw", "SSHC_PASSWORD"},
		{"env only, no config", []string{"SSHC_PASSWORD=env-pw"}, nil, "luke", "10.0.0.5", "env-pw", "SSHC_PASSWORD"},
		{"host section by user@alias", []string{"SSHC_PASSWORD=env-pw"}, cfg, "root", "10.0.0.5", "root-pw", "[host root@box]"},
		{"host section by hostname", nil, cfg, "luke", "10.0.0.9", "nine-pw", "[host 10.0.0.9]"},
		{"host env by alias", []string{"SSHC_PASSWORD_W_GO_2=two-pw", "SSHC_PASSWORD=env-pw"}, cfg, "luke", "10.0.0.6", "two-pw", "SSHC_PASSWORD_W_GO_2"},
		{"host env any case", []string{"sshc_password_w_go_2=two-pw"}, cfg, "luke", "10.0.0.6", "two-pw", "sshc_password_w_go_2"},
		{"host env beats host section", []string{"SSHC_PASSWORD_ROOT_BOX=e"}, cfg, "root", "10.0.0.5", "e", "SSHC_PASSWORD_ROOT_BOX"},
		{"prompt may show the alias", nil, cfg, "luke", "BOX", "work-pw", "[profile work]"},

		// Hosts not named on the command line only get host-specific entries.
		{"jump host with entry", []string{"SSHC_PASSWORD=env-pw"}, cfg, "luke", "jump", "jump-pw", "[host jump]"},
		{"jump host without entry", []string{"SSHC_PASSWORD=env-pw"}, cfg, "luke", "other", "", ""},
		{"empty env value ignored", []string{"SSHC_PASSWORD=", "SSHC_PASSWORD_BOX="}, nil, "luke", "10.0.0.5", "", ""},
	}
	for _, tt := range tests {
		r := &resolver{cfg: tt.cfg, dests: dests, environ: tt.env}
		got, from, _ := r.lookup(tt.user, tt.host)
		if got != tt.want || !strings.Contains(from, tt.from) {
			t.Errorf("%s: lookup = %q from %q; want %q from %q", tt.name, got, from, tt.want, tt.from)
		}
	}

	r := &resolver{cfg: cfg, dests: dests, environ: []string{"SSHC_PROFILE=nope"}}
	if got, _, note := r.lookup("luke", "10.0.0.5"); got != "" || note == "" {
		t.Errorf("unknown profile: got %q, note %q; want no password and a note", got, note)
	}
}

func TestLookupPassphrase(t *testing.T) {
	cfg := &config{path: "sshc.conf", entries: map[string]string{
		configKey("", "profile"):                                 "work",
		configKey("profile work", "passphrase"):                  "work-pp",
		configKey("profile home", "passphrase"):                  "home-pp",
		configKey("key id_special", "passphrase"):                "special-pp",
		configKey("key /srv/keys/id_special", "passphrase"):      "srv-pp",
		configKey(`key C:\Users\Luke\.ssh\id_win`, "passphrase"): "win-pp",
	}}
	long := "/" + strings.Repeat("d", 120) + "/id_long"
	cfgLong := &config{path: "sshc.conf", entries: map[string]string{configKey("key "+long, "passphrase"): "long-pp"}}
	tests := []struct {
		name       string
		env        []string
		cfg        *config
		key        string
		want, from string
	}{
		{"profile", nil, cfg, "/home/luke/.ssh/id_ed25519", "work-pp", "[profile work]"},
		{"SSHC_PROFILE", []string{"SSHC_PROFILE=home"}, cfg, "/home/luke/.ssh/id_ed25519", "home-pp", "[profile home]"},
		{"SSHC_PASSPHRASE beats profile", []string{"SSHC_PASSPHRASE=env-pp"}, cfg, "/home/luke/.ssh/id_ed25519", "env-pp", "SSHC_PASSPHRASE"},
		{"env only", []string{"SSHC_PASSPHRASE=env-pp"}, nil, `C:\Users\Luke/.ssh/id_ed25519`, "env-pp", "SSHC_PASSPHRASE"},
		{"key section by file name", []string{"SSHC_PASSPHRASE=env-pp"}, cfg, "/home/luke/.ssh/id_special", "special-pp", "[key id_special]"},
		{"key section by full path wins", nil, cfg, "/srv/keys/id_special", "srv-pp", "[key /srv/keys/id_special]"},
		{"windows path, mixed slashes and case", nil, cfg, `c:\users\luke/.ssh/ID_WIN`, "win-pp", "id_win]"},
		{"key env by file name", []string{"SSHC_PASSPHRASE_ID_SPECIAL=e"}, cfg, "/home/luke/.ssh/id_special", "e", "SSHC_PASSPHRASE_ID_SPECIAL"},
		{"path cut off by ssh at 100 chars", nil, cfgLong, long[:100], "long-pp", "[key /ddd"},
		{"password variables are not passphrases", []string{"SSHC_PASSWORD=pw"}, nil, "/home/luke/.ssh/id", "", ""},
	}
	for _, tt := range tests {
		r := &resolver{cfg: tt.cfg, environ: tt.env}
		got, from, _ := r.lookupPassphrase(tt.key)
		if got != tt.want || !strings.Contains(from, tt.from) {
			t.Errorf("%s: lookupPassphrase = %q from %q; want %q from %q", tt.name, got, from, tt.want, tt.from)
		}
	}

	// A profile holding only a passphrase must not draw a "no password" note.
	r := &resolver{cfg: cfg, dests: []dest{{alias: "box", hostname: "10.0.0.5"}}}
	if pw, _, note := r.lookup("luke", "10.0.0.5"); pw != "" || note != "" {
		t.Errorf("passphrase-only profile: password %q, note %q; want neither", pw, note)
	}
}

func TestHasEnvPasswords(t *testing.T) {
	tests := []struct {
		env  []string
		want bool
	}{
		{nil, false},
		{[]string{"SSHC_PROFILE=work", "SSHC_PASSWORD="}, false},
		{[]string{"SSHC_PASSWORD=x"}, true},
		{[]string{"SSHC_PASSWORD_BOX=x"}, true},
		{[]string{"SSHC_PASSPHRASE=x"}, true},
		{[]string{"SSHC_PASSPHRASE_ID_ED25519=x"}, true},
	}
	for _, tt := range tests {
		if got := (&resolver{environ: tt.env}).hasEnvPasswords(); got != tt.want {
			t.Errorf("hasEnvPasswords(%q) = %v", tt.env, got)
		}
	}
}

func TestDestsRoundTrip(t *testing.T) {
	in := []dest{{alias: "box", hostname: "10.0.0.5", user: "luke"}, {alias: "::1", hostname: "::1"}}
	if got := decodeDests(encodeDests(in)); !reflect.DeepEqual(got, in) {
		t.Errorf("round trip = %v; want %v", got, in)
	}
	if got := decodeDests(""); got != nil {
		t.Errorf("decodeDests(\"\") = %v", got)
	}
}
