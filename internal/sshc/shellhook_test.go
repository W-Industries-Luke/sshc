package sshc

import (
	"testing"
)

func TestEmitAssignment(t *testing.T) {
	tests := []struct{ kind, value, want string }{
		{"posix", "plain", `export V='plain'`},
		{"posix", `it's $HOME "x" \ ;`, `export V='it'\''s $HOME "x" \ ;'`},
		{"fish", `it's \ $x`, `set -gx V 'it\'s \\ $x'`},
		{"powershell", `it's $x "q" ` + "\u2019", `$env:V = 'it''s $x "q" ` + "\u2019\u2019'"},
	}
	for _, tt := range tests {
		if got, ok := emitAssignment(tt.kind, "V", tt.value); !ok || got != tt.want {
			t.Errorf("emitAssignment(%s, %q) = %q; want %q", tt.kind, tt.value, got, tt.want)
		}
	}
	for _, kind := range []string{"", "cmd", "bash"} {
		if _, ok := emitAssignment(kind, "V", "x"); ok {
			t.Errorf("emitAssignment(%q) should not be supported", kind)
		}
	}
	for name, want := range map[string]string{"/bin/bash": "posix", "zsh": "posix", "/usr/bin/fish": "fish", "pwsh.exe": "powershell", "PowerShell": "powershell", "cmd": ""} {
		if got := shellKind(name); got != want {
			t.Errorf("shellKind(%q) = %q; want %q", name, got, want)
		}
	}
}
