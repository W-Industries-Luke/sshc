package sshc

import (
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
