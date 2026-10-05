package sshc

import (
	"strings"
	"testing"
)

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
