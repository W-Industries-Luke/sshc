package sshc

import (
	"strings"
	"testing"
)

func TestHighlight(t *testing.T) {
	strip := func(s string) string {
		for _, c := range []string{sgrReset, sgrBold, sgrGreen, sgrYellow, sgrRed, sgrCyan} {
			s = strings.ReplaceAll(s, c, "")
		}
		return s
	}
	texts := []string{
		"Updated!\n\npassphrase of [profile default], kept in Windows Credential Manager\n\nNote: SSHC_PASSPHRASE is set in this shell and takes precedence here.",
		"sshc: there is no stored password for [host box]",
		"config file: none\ncredential store: the system keyring (Secret Service)\n\nStored:\n  [key id_ed25519]   passphrase  credential store - MISSING, set it again",
		"box -> 10.0.0.5 (user luke)\n  no stored password; you would be prompted\n  key ~/.ssh/id: passphrase from environment variable SSHC_PASSPHRASE",
		"Warning: another copy of sshc comes first\nRun \"sshc set -p\" again.",
		"nothing to colour here",
	}
	for _, text := range texts {
		got := highlight(text)
		// Colouring must never change, drop or reorder the words themselves.
		if strip(got) != text {
			t.Errorf("highlight changed the text:\n%q\n%q", text, strip(got))
		}
		if strings.Count(got, "\x1b[") != 2*strings.Count(got, sgrReset) {
			t.Errorf("unbalanced colour codes in %q", got)
		}
	}
	got := highlight(texts[0])
	for _, want := range []string{sgrGreen + "Updated!" + sgrReset, sgrCyan + "[profile default]" + sgrReset, sgrCyan + "SSHC_PASSPHRASE" + sgrReset, sgrYellow + "Note:" + sgrReset, sgrGreen + "Windows Credential Manager" + sgrReset} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	if got := highlight(texts[5]); got != texts[5] {
		t.Errorf("plain text was changed: %q", got)
	}
}
