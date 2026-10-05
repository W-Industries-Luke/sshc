package sshc

import (
	"strings"
	"testing"
)

func TestExpandShort(t *testing.T) {
	tests := []struct{ in, want string }{
		{"-sp", "--session --passphrase"},
		{"-s -p", "--session --passphrase"},
		{"-H box", "--host box"},
		{"-Hbox", "--host box"},
		{"-sk id_ed25519", "--session --key id_ed25519"},
		{"-P work -fn", "--profile work --plain --no-clear"},
		{"-h", "--help"},
		{"--session value", "--session value"},
		{"-s -- -p", "--session -- -p"},
		{"-", "-"},
		{"-k", "--key"},
	}
	for _, tt := range tests {
		got, ok := expandShort(strings.Fields(tt.in))
		if !ok || strings.Join(got, " ") != tt.want {
			t.Errorf("expandShort(%q) = %q, %v; want %q", tt.in, got, ok, tt.want)
		}
	}
	for _, bad := range []string{"-x", "-sx", "-sZ value"} {
		if got, ok := expandShort(strings.Fields(bad)); ok {
			t.Errorf("expandShort(%q) = %q; want a rejection", bad, got)
		}
	}
}

func TestDeleteConfigValue(t *testing.T) {
	file := strings.Split(`profile = work

[profile work]
password = a
passphrase = b

# about the host
[host box]
password = c

[key id]
passphrase = d`, "\n")
	join := func(l []string) string { return strings.Join(l, "\n") }

	got, ok := deleteConfigValue(file, "profile work", "password")
	if !ok || !strings.Contains(join(got), "[profile work]\npassphrase = b") || strings.Contains(join(got), "password = a") {
		t.Errorf("delete one of two keys:\n%s", join(got))
	}
	got, ok = deleteConfigValue(file, "HOST box", "Password")
	if !ok || strings.Contains(join(got), "[host box]") || strings.Contains(join(got), "password = c") || !strings.Contains(join(got), "# about the host") {
		t.Errorf("delete the only key should drop the header:\n%s", join(got))
	}
	got, ok = deleteConfigValue(file, "key id", "passphrase")
	if !ok || strings.Contains(join(got), "[key id]") {
		t.Errorf("delete in the last section:\n%s", join(got))
	}
	if got, ok := deleteConfigValue(file, "host nope", "password"); ok || join(got) != join(file) {
		t.Error("deleting a missing entry should change nothing")
	}
	if got, ok := deleteConfigValue(file, "host box", "passphrase"); ok || join(got) != join(file) {
		t.Error("deleting a missing key should change nothing")
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
