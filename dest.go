package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Option letters that take an argument, per tool, from each tool's getopt
// string. They are needed only to tell options from operands.
var optsWithArg = map[string]string{
	"ssh":  "BbcDEeFIiJLlmOoPpQRSWw",
	"scp":  "cDFiJloPSX",
	"sftp": "BbcDFiJloPRSsX",

	"ssh-copy-id": "iopFt",
}

// rsync options that take a value. Unlike the OpenSSH tools, rsync accepts
// options anywhere on the line and has long options, whose value may be
// attached with "=" or given as the next argument.
const rsyncShortWithArg = "BefMT@"

var rsyncLongWithArg = map[string]bool{
	"info": true, "debug": true, "stderr": true, "backup-dir": true, "suffix": true,
	"chmod": true, "checksum-choice": true, "cc": true, "block-size": true, "rsh": true,
	"rsync-path": true, "max-delete": true, "max-size": true, "min-size": true,
	"max-alloc": true, "partial-dir": true, "usermap": true, "groupmap": true,
	"chown": true, "timeout": true, "contimeout": true, "modify-window": true,
	"temp-dir": true, "compare-dest": true, "copy-dest": true, "link-dest": true,
	"compress-choice": true, "zc": true, "compress-level": true, "zl": true,
	"skip-compress": true, "filter": true, "exclude": true, "exclude-from": true,
	"include": true, "include-from": true, "files-from": true, "copy-as": true,
	"address": true, "port": true, "sockopts": true, "outbuf": true,
	"remote-option": true, "out-format": true, "log-file": true,
	"log-file-format": true, "password-file": true, "early-input": true,
	"bwlimit": true, "stop-after": true, "stop-at": true, "write-batch": true,
	"only-write-batch": true, "read-batch": true, "protocol": true, "iconv": true,
	"checksum-seed": true,
}

// rsyncOperands separates an rsync command line into its operands and the
// remote shell command given with -e/--rsh, if any.
func rsyncOperands(args []string) (rsh string, operands []string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			return rsh, append(operands, args[i+1:]...)
		case strings.HasPrefix(arg, "--"):
			name, val, attached := strings.Cut(arg[2:], "=")
			if !attached && rsyncLongWithArg[name] && i+1 < len(args) {
				i++
				val = args[i]
			}
			if name == "rsh" {
				rsh = val
			}
		case len(arg) > 1 && arg[0] == '-':
			for j := 1; j < len(arg); j++ {
				if !strings.ContainsRune(rsyncShortWithArg, rune(arg[j])) {
					continue
				}
				val := arg[j+1:]
				if val == "" && i+1 < len(args) {
					i++
					val = args[i]
				}
				if arg[j] == 'e' {
					rsh = val
				}
				break
			}
		default:
			operands = append(operands, arg)
		}
	}
	return rsh, operands
}

// rshConfigOpts picks the -F and -o options out of an rsync remote shell
// command such as "ssh -p 2222 -F cfg", so hosts resolve the way they will
// when rsync runs it.
func rshConfigOpts(rsh string) []string {
	words := strings.Fields(rsh)
	if len(words) == 0 {
		return nil
	}
	if name := strings.TrimSuffix(filepath.Base(words[0]), ".exe"); name != "ssh" {
		return nil
	}
	cfgOpts, _ := splitArgs(words[1:], optsWithArg["ssh"])
	return cfgOpts
}

// splitArgs walks a getopt-style command line up to the first operand. It
// returns the -F and -o options, which are the ones that change how a host
// name resolves, and everything from the first operand on.
func splitArgs(args []string, withArg string) (cfgOpts, operands []string) {
	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			i++
			break
		}
		if len(arg) < 2 || arg[0] != '-' {
			break
		}
		i++
		for j := 1; j < len(arg); j++ {
			c := arg[j]
			if !strings.ContainsRune(withArg, rune(c)) {
				continue
			}
			val := arg[j+1:]
			if val == "" {
				if i >= len(args) {
					return cfgOpts, nil
				}
				val = args[i]
				i++
			}
			if c == 'F' || c == 'o' {
				cfgOpts = append(cfgOpts, "-"+string(c), val)
			}
			break
		}
	}
	return cfgOpts, args[i:]
}

// scpColon mirrors colon() in OpenSSH's scp.c: the index of the ":" that
// separates host from path, or -1 if the operand is a local file.
func scpColon(s string) int {
	if s == "" || s[0] == ':' {
		return -1
	}
	if runtime.GOOS == "windows" && len(s) >= 2 && s[1] == ':' && isLetter(s[0]) {
		return -1 // drive letter
	}
	bracket := s[0] == '['
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '@' && i+1 < len(s) && s[i+1] == '[':
			bracket = true
		case s[i] == ']' && i+1 < len(s) && s[i+1] == ':' && bracket:
			return i + 1
		case s[i] == ':' && !bracket:
			return i
		case s[i] == '/', s[i] == '\\' && runtime.GOOS == "windows":
			return -1
		}
	}
	return -1
}

func isLetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// bareHost strips "user@", "[...]" brackets and ":port" from a host spec.
func bareHost(s string, hasPort bool) string {
	if i := strings.LastIndex(s, "@"); i >= 0 {
		s = s[i+1:]
	}
	if strings.HasPrefix(s, "[") {
		if i := strings.Index(s, "]"); i >= 0 {
			return s[1:i]
		}
	}
	if hasPort {
		s, _, _ = strings.Cut(s, ":")
	}
	return s
}

// operandHost extracts the host from one operand of the given tool, or ""
// when the operand is a local path.
func operandHost(tool, s string) string {
	if tool == "rsync" {
		// host::module and rsync:// talk to an rsync daemon, not to ssh.
		i := scpColon(s)
		if i < 0 || strings.HasPrefix(s, "rsync://") || strings.HasPrefix(s[i+1:], ":") {
			return ""
		}
		return bareHost(s[:i], false)
	}
	if _, rest, ok := strings.Cut(s, "://"); ok && strings.HasPrefix(s, tool+"://") {
		authority, _, _ := strings.Cut(rest, "/")
		return bareHost(authority, true)
	}
	switch tool {
	case "ssh", "ssh-copy-id":
		return bareHost(s, false)
	case "scp":
		i := scpColon(s)
		if i < 0 {
			return ""
		}
		return bareHost(s[:i], false)
	default: // sftp: [user@]host[:path]
		if i := scpColon(s); i >= 0 {
			s = s[:i]
		}
		return bareHost(s, false)
	}
}

// queryDest asks ssh itself how it would resolve a destination: "ssh -G"
// prints the effective configuration without connecting.
func queryDest(args []string) (d dest, ok bool) {
	cmd := exec.Command("ssh", append([]string{"-G"}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return d, false
	}
	for _, line := range strings.Split(string(out), "\n") {
		k, v, _ := strings.Cut(strings.TrimSpace(line), " ")
		switch k {
		case "host":
			d.alias = v
		case "hostname":
			d.hostname = v
		case "user":
			d.user = v
		case "identityfile":
			d.keys = append(d.keys, v)
		}
	}
	return d, d.hostname != ""
}

// findDests works out which remote hosts a command line names. An empty
// result means the tool is not going to connect anywhere (ssh -V, a usage
// error, a purely local scp).
func findDests(tool string, args []string) []dest {
	var cfgOpts, operands []string
	if tool == "rsync" {
		rsh, ops := rsyncOperands(args)
		if rsh == "" {
			rsh = os.Getenv("RSYNC_RSH")
		}
		cfgOpts, operands = rshConfigOpts(rsh), ops
	} else {
		cfgOpts, operands = splitArgs(args, optsWithArg[tool])
	}
	if tool == "ssh" {
		d, ok := queryDest(args)
		if !ok {
			return nil
		}
		if d.alias == "" && len(operands) > 0 {
			d.alias = operandHost(tool, operands[0])
		}
		if d.alias == "" {
			d.alias = d.hostname
		}
		return []dest{d}
	}
	if (tool == "sftp" || tool == "ssh-copy-id") && len(operands) > 1 {
		operands = operands[:1]
	}
	var dests []dest
	seen := map[string]bool{}
	for _, op := range operands {
		host := operandHost(tool, op)
		if host == "" || seen[host] {
			continue
		}
		seen[host] = true
		q := append(append([]string{}, cfgOpts...), "--", host)
		if d, ok := queryDest(q); ok {
			d.alias = host
			dests = append(dests, d)
		}
	}
	return dests
}
