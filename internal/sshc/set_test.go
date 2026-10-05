package sshc

import (
	"strings"
	"testing"
)

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
