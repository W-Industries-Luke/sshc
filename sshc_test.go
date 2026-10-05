package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestParsePrompt(t *testing.T) {
	tests := []struct {
		prompt     string
		user, host string
		ok         bool
	}{
		{"luke@10.0.0.5's password: ", "luke", "10.0.0.5", true},
		{"luke@w.go-2's password:", "luke", "w.go-2", true},
		{"corp\\luke@ad.example@box's password: ", "corp\\luke@ad.example", "box", true},
		{"(luke@10.0.0.5) Password: ", "luke", "10.0.0.5", true},
		{"(luke@box) Password for luke@box: ", "luke", "box", true},
		{"(luke@box) password:", "luke", "box", true},

		// Server-chosen text must not be able to name another host.
		{"(luke@evil) admin@bank's password: ", "", "", false},
		{"(luke@evil) (admin@bank) Password: ", "", "", false},
		{"Banner\nadmin@bank's password: ", "", "", false},

		// A key passphrase is a different kind of prompt.
		{"Enter passphrase for key '/home/luke/.ssh/id_ed25519': ", "", "", false},

		// Not a login password: left for the human.
		{"Password: ", "", "", false},
		{"(luke@box) Verification code: ", "", "", false},
		{"(luke@box) New password: ", "", "", false},
		{"(luke@box) Retype new password: ", "", "", false},
		{"(luke@box) (current) UNIX password: ", "", "", false},
		{"Enter passphrase for key '/home/luke/.ssh/id_ed25519': ", "", "", false},
		{"", "", "", false},
	}
	for _, tt := range tests {
		user, host, ok := parsePrompt(tt.prompt)
		if user != tt.user || host != tt.host || ok != tt.ok {
			t.Errorf("parsePrompt(%q) = %q, %q, %v; want %q, %q, %v",
				tt.prompt, user, host, ok, tt.user, tt.host, tt.ok)
		}
	}
}

func TestIsAskpassCall(t *testing.T) {
	t.Setenv(envState, "/tmp/x")
	if !isAskpassCall([]string{"luke@box's password: "}) {
		t.Error("a prompt from ssh should be an askpass call")
	}
	for _, args := range [][]string{{"box"}, {"-V"}, {"box", "echo hi"}, {}} {
		if isAskpassCall(args) {
			t.Errorf("%q should not be an askpass call", args)
		}
	}
	t.Setenv(envState, "")
	if isAskpassCall([]string{"luke@box's password: "}) {
		t.Error("without the state variable nothing is an askpass call")
	}
}

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

func TestSetConfigValue(t *testing.T) {
	join := func(l []string) string { return strings.Join(l, "\n") }
	file := strings.Split(`# top comment
profile = work

[profile work]
password = old
other = keep

# about the host
[host box]
password = boxpw`, "\n")

	got := join(setConfigValue(file, "Profile  WORK", "password", "new"))
	if !strings.Contains(got, "[profile work]\npassword = new\nother = keep") || strings.Contains(got, "old") {
		t.Errorf("replace in place failed:\n%s", got)
	}
	got = join(setConfigValue(file, "profile work", "extra", "x"))
	if !strings.Contains(got, "other = keep\nextra = x\n\n# about the host") {
		t.Errorf("insert at end of section failed:\n%s", got)
	}
	got = join(setConfigValue(file, "host new", "password", "p"))
	if !strings.HasSuffix(got, "password = boxpw\n\n[host new]\npassword = p") {
		t.Errorf("append new section failed:\n%s", got)
	}
	got = join(setConfigValue(file, "", "clear_on_set", "no"))
	if !strings.Contains(got, "profile = work\nclear_on_set = no\n\n[profile work]") {
		t.Errorf("insert top-level key failed:\n%s", got)
	}
	got = join(setConfigValue([]string{"# only a comment", "#profile = x"}, "", "profile", "default"))
	if got != "profile = default\n# only a comment\n#profile = x" {
		t.Errorf("top-level key in a file without entries:\n%s", got)
	}

	// Whatever is written must read back exactly.
	for _, pw := range []string{"plain", " lead", "trail ", `"quoted"`, "'q'", `"`, "a = b # c", `x"y`, "[brackets]", "#hash"} {
		lines := setConfigValue(nil, "profile p", "password", pw)
		entries, bad := parseConfig(strings.NewReader(join(lines)))
		if got := entries[configKey("profile p", "password")]; got != pw || len(bad) != 0 {
			t.Errorf("password %q read back as %q (bad lines %v)", pw, got, bad)
		}
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

func TestPassphrasePrompt(t *testing.T) {
	tests := []struct{ prompt, key string }{
		{"Enter passphrase for key '/home/luke/.ssh/id_ed25519': ", "/home/luke/.ssh/id_ed25519"},
		{`Enter passphrase for key 'C:\Users\Luke.Weaver/.ssh/id_ed25519': `, `C:\Users\Luke.Weaver/.ssh/id_ed25519`},
		{"Enter passphrase for key '/home/o'brien/.ssh/id': ", "/home/o'brien/.ssh/id"},

		// A server can only speak behind the "(user@host) " prefix.
		{"(luke@evil) Enter passphrase for key '/home/luke/.ssh/id_ed25519': ", ""},
		{"Banner\nEnter passphrase for key '/home/luke/.ssh/id_ed25519': ", ""},
		{"Enter PIN for ED25519-SK key /home/luke/.ssh/id_sk: ", ""},
		{"Bad passphrase, try again for /home/luke/.ssh/id: ", ""},
	}
	for _, tt := range tests {
		got := ""
		if m := rePassphrase.FindStringSubmatch(tt.prompt); m != nil {
			got = m[1]
		}
		if got != tt.key {
			t.Errorf("passphrase prompt %q: key %q; want %q", tt.prompt, got, tt.key)
		}
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

func TestSplitArgs(t *testing.T) {
	tests := []struct {
		tool     string
		args     string
		cfgOpts  string
		operands string
	}{
		{"ssh", "box", "", "box"},
		{"ssh", "-p 2222 -v box uptime -a", "", "box uptime -a"},
		{"ssh", "-vvp2222 -F cfg -oPort=1 -o User=x box", "-F cfg -o Port=1 -o User=x", "box"},
		{"ssh", "-4AF cfg box", "-F cfg", "box"},
		{"ssh", "-V", "", ""},
		{"ssh", "-i", "", ""},
		{"scp", "-r -P 2222 ./a box:b", "", "./a box:b"},
		{"scp", "-rP2222 -- -odd box:b", "", "-odd box:b"},
		{"sftp", "-b batch -oPort=1 luke@box:dir", "-o Port=1", "luke@box:dir"},
		{"ssh-copy-id", "-i key.pub -p 2222 -F cfg luke@box", "-F cfg", "luke@box"},
	}
	for _, tt := range tests {
		cfgOpts, operands := splitArgs(strings.Fields(tt.args), optsWithArg[tt.tool])
		if strings.Join(cfgOpts, " ") != tt.cfgOpts || strings.Join(operands, " ") != tt.operands {
			t.Errorf("splitArgs(%s %s) = %q, %q; want %q, %q",
				tt.tool, tt.args, cfgOpts, operands, tt.cfgOpts, tt.operands)
		}
	}
}

func TestRsyncOperands(t *testing.T) {
	tests := []struct {
		args     []string
		rsh      string
		operands string
	}{
		{[]string{"-av", "./a", "box:b"}, "", "./a box:b"},
		{[]string{"./a", "-av", "--delete", "box:b"}, "", "./a box:b"},
		{[]string{"-e", "ssh -p 2222", "./a", "box:b"}, "ssh -p 2222", "./a box:b"},
		{[]string{"-ave", "ssh -F cfg", "./a", "box:b"}, "ssh -F cfg", "./a box:b"},
		{[]string{"--rsh=ssh -F cfg", "./a", "box:b"}, "ssh -F cfg", "./a box:b"},
		{[]string{"--rsh", "ssh", "--exclude", "x:y", "--exclude=p:q", "-f", "- a:b", "./a", "box:b"}, "ssh", "./a box:b"},
		{[]string{"--bwlimit=10", "-T", "/tmp", "-B4096", "./a", "box:b"}, "", "./a box:b"},
		{[]string{"-a", "--", "-odd", "box:b"}, "", "-odd box:b"},
	}
	for _, tt := range tests {
		rsh, operands := rsyncOperands(tt.args)
		if rsh != tt.rsh || strings.Join(operands, " ") != tt.operands {
			t.Errorf("rsyncOperands(%q) = %q, %q; want %q, %q", tt.args, rsh, operands, tt.rsh, tt.operands)
		}
	}
}

func TestRshConfigOpts(t *testing.T) {
	tests := []struct{ rsh, want string }{
		{"", ""},
		{"ssh", ""},
		{"ssh -p 2222 -F cfg -oUser=x", "-F cfg -o User=x"},
		{"/usr/bin/ssh -F cfg", "-F cfg"},
		{"rsh -F cfg", ""},
	}
	for _, tt := range tests {
		if got := strings.Join(rshConfigOpts(tt.rsh), " "); got != tt.want {
			t.Errorf("rshConfigOpts(%q) = %q; want %q", tt.rsh, got, tt.want)
		}
	}
}

func TestOperandHost(t *testing.T) {
	tests := []struct{ tool, operand, want string }{
		{"ssh", "box", "box"},
		{"ssh", "luke@box", "box"},
		{"ssh", "a@b@box", "box"},
		{"ssh", "ssh://luke@box:2222", "box"},
		{"ssh", "ssh://[::1]:2222", "::1"},

		{"scp", "box:file", "box"},
		{"scp", "luke@box:dir/a@b:c", "box"},
		{"scp", "box:", "box"},
		{"scp", "[::1]:file", "::1"},
		{"scp", "luke@[fe80::1]:file", "fe80::1"},
		{"scp", "scp://luke@box:2222/dir/file", "box"},
		{"scp", "./copy-path", ""},
		{"scp", "dir/with:colon", ""},
		{"scp", ":leading", ""},
		{"scp", "plainfile", ""},
		{"scp", "/abs/path", ""},

		{"sftp", "box", "box"},
		{"sftp", "luke@box:dir", "box"},
		{"sftp", "sftp://luke@box/dir", "box"},
		{"sftp", "[::1]:dir", "::1"},

		{"ssh-copy-id", "luke@box", "box"},

		{"rsync", "box:dir/", "box"},
		{"rsync", "luke@box:", "box"},
		{"rsync", "[::1]:dir", "::1"},
		{"rsync", "./local/", ""},
		{"rsync", "box::module/dir", ""},
		{"rsync", "rsync://box/module", ""},
		{"rsync", ": /.rsync-filter", ""},
	}
	for _, tt := range tests {
		if got := operandHost(tt.tool, tt.operand); got != tt.want {
			t.Errorf("operandHost(%s, %q) = %q; want %q", tt.tool, tt.operand, got, tt.want)
		}
	}
}
