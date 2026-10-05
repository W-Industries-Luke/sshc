// Package e2e runs the real sshc binary and the real OpenSSH client of this
// machine against an SSH server that lives inside the test. Nothing outside
// the test process is contacted, and the only credentials involved are the
// made-up ones below.
package e2e

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
)

const (
	testUser     = "luke"
	testPassword = "s3cret pass#1"
)

var sshcBin string

func TestMain(m *testing.M) {
	if _, err := exec.LookPath("ssh"); err != nil {
		fmt.Println("skipping end-to-end tests: no ssh client on PATH")
		return
	}
	dir, err := os.MkdirTemp("", "sshc-e2e-bin-")
	if err != nil {
		panic(err)
	}
	sshcBin = filepath.Join(dir, "sshc")
	if runtime.GOOS == "windows" {
		sshcBin += ".exe"
	}
	build := exec.Command("go", "build", "-o", sshcBin, "../..")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Println("could not build sshc:", err)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// server is a minimal SSH server: it authenticates and runs a few toy
// commands, which is all that is needed to see whether sshc got a login
// through.
type server struct {
	port int

	mu        sync.Mutex
	passwords []string // every password a client offered
}

type serverOptions struct {
	password   string        // accepted by password authentication, if set
	kbdAnswers []string      // accepted answers to keyboard-interactive, if set
	kbdPrompts []string      // the questions asked, one per answer
	key        ssh.PublicKey // accepted by public-key authentication, if set
}

func (s *server) offered() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.passwords...)
}

func startServer(t *testing.T, opts serverOptions) *server {
	t.Helper()
	s := &server{}
	cfg := &ssh.ServerConfig{}
	if opts.password != "" {
		cfg.PasswordCallback = func(c ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			s.mu.Lock()
			s.passwords = append(s.passwords, string(pw))
			s.mu.Unlock()
			if c.User() == testUser && string(pw) == opts.password {
				return nil, nil
			}
			return nil, fmt.Errorf("wrong password")
		}
	}
	if opts.kbdAnswers != nil {
		cfg.KeyboardInteractiveCallback = func(c ssh.ConnMetadata, ask ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
			for i, want := range opts.kbdAnswers {
				got, err := ask("", "", []string{opts.kbdPrompts[i]}, []bool{false})
				if err != nil || len(got) != 1 || got[0] != want {
					return nil, fmt.Errorf("wrong answer")
				}
			}
			return nil, nil
		}
	}
	if opts.key != nil {
		cfg.PublicKeyCallback = func(c ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if bytes.Equal(key.Marshal(), opts.key.Marshal()) {
				return nil, nil
			}
			return nil, fmt.Errorf("unknown key")
		}
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	s.port = ln.Addr().(*net.TCPAddr).Port
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serve(conn, cfg)
		}
	}()
	return s
}

func serve(conn net.Conn, cfg *ssh.ServerConfig) {
	defer conn.Close()
	sc, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	defer sc.Close()
	go ssh.DiscardRequests(reqs)
	for nc := range chans {
		if nc.ChannelType() != "session" {
			nc.Reject(ssh.UnknownChannelType, "sessions only")
			continue
		}
		ch, requests, err := nc.Accept()
		if err != nil {
			return
		}
		go func() {
			defer ch.Close()
			for req := range requests {
				if req.Type != "exec" {
					req.Reply(req.Type == "env" || req.Type == "pty-req", nil)
					continue
				}
				var payload struct{ Command string }
				ssh.Unmarshal(req.Payload, &payload)
				req.Reply(true, nil)
				status := uint32(0)
				switch cmd := payload.Command; {
				case cmd == "whoami":
					fmt.Fprintln(ch, sc.User())
				case strings.HasPrefix(cmd, "echo "):
					fmt.Fprintln(ch, strings.TrimPrefix(cmd, "echo "))
				case strings.HasPrefix(cmd, "exit "):
					n, _ := strconv.Atoi(strings.TrimPrefix(cmd, "exit "))
					status = uint32(n)
				default:
					fmt.Fprintln(ch, "ran:", cmd)
				}
				ch.SendRequest("exit-status", false, binary.BigEndian.AppendUint32(nil, status))
				return
			}
		}()
	}
}

// env is the isolated world one test runs sshc in.
type env struct {
	t    *testing.T
	dir  string
	vars []string
}

// newEnv prepares a scratch directory with an ssh config naming each server.
func newEnv(t *testing.T, hosts map[string]*server) *env {
	t.Helper()
	dir := t.TempDir()
	slash := filepath.ToSlash(dir)
	var cfg strings.Builder
	fmt.Fprintf(&cfg, "Host *\n  StrictHostKeyChecking no\n  UserKnownHostsFile \"%s/known_hosts\"\n  LogLevel ERROR\n"+
		"  IdentityAgent none\n  PubkeyAuthentication no\n  NumberOfPasswordPrompts 3\n  User %s\n  HostName 127.0.0.1\n", slash, testUser)
	for name, s := range hosts {
		fmt.Fprintf(&cfg, "Host %s\n  Port %d\n", name, s.port)
	}
	if err := os.WriteFile(filepath.Join(dir, "ssh_config"), []byte(cfg.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, dir: dir}
	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); !strings.HasPrefix(strings.ToUpper(name), "SSHC_") {
			e.vars = append(e.vars, kv)
		}
	}
	// Keep sshc away from the real config file and the real terminal. The
	// credential store is off unless a test turns it on.
	conf := filepath.Join(dir, "sshc.conf")
	if err := os.WriteFile(conf, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	e.vars = append(e.vars, "SSHC_CONFIG="+conf, "SSHC_NO_PROMPT=1", "SSHC_CREDENTIAL_STORE=off", "NO_COLOR=1",
		"SSHC_STATE_DIR="+filepath.Join(dir, "state"))
	return e
}

func (e *env) sshConfig() string { return filepath.Join(e.dir, "ssh_config") }

// run executes sshc and returns its output and exit status.
func (e *env) run(stdin string, extraEnv []string, args ...string) (stdout, stderr string, status int) {
	e.t.Helper()
	cmd := exec.Command(sshcBin, args...)
	cmd.Env = append(append([]string{}, e.vars...), extraEnv...)
	cmd.Dir = e.dir
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok {
		status = ee.ExitCode()
	} else if err != nil {
		e.t.Fatalf("running sshc %v: %v", args, err)
	}
	return out.String(), errb.String(), status
}

// ssh runs "sshc -F <config> args...".
func (e *env) ssh(extraEnv []string, args ...string) (string, string, int) {
	e.t.Helper()
	return e.run("", extraEnv, append([]string{"-F", e.sshConfig()}, args...)...)
}

func wantOutput(t *testing.T, what, stdout, stderr string, status int, want string) {
	t.Helper()
	if status != 0 || !strings.Contains(stdout, want) {
		t.Errorf("%s: status %d, stdout %q, stderr %q; want %q on stdout", what, status, stdout, stderr, want)
	}
}

func TestPasswordFromEnvironment(t *testing.T) {
	s := startServer(t, serverOptions{password: testPassword})
	e := newEnv(t, map[string]*server{"box": s})
	out, errs, status := e.ssh([]string{"SSHC_PASSWORD=" + testPassword}, "box", "echo", "hello")
	wantOutput(t, "login by alias", out, errs, status, "hello")

	out, errs, status = e.ssh([]string{"SSHC_PASSWORD=" + testPassword}, "-p", strconv.Itoa(s.port), testUser+"@127.0.0.1", "whoami")
	wantOutput(t, "login by full connection string", out, errs, status, testUser)

	if _, _, status := e.ssh([]string{"SSHC_PASSWORD=" + testPassword}, "box", "exit", "7"); status != 7 {
		t.Errorf("remote exit status: got %d, want 7", status)
	}
}

func TestRejectedPasswordIsOfferedOnce(t *testing.T) {
	s := startServer(t, serverOptions{password: testPassword})
	e := newEnv(t, map[string]*server{"box": s})
	_, errs, status := e.ssh([]string{"SSHC_PASSWORD=wrong one"}, "box", "echo", "hello")
	if status == 0 {
		t.Fatal("login with a wrong password succeeded")
	}
	if !strings.Contains(errs, "was not accepted") {
		t.Errorf("no word about the rejected password in: %q", errs)
	}
	sent := 0
	for _, pw := range s.offered() {
		if pw == "wrong one" {
			sent++
		}
	}
	if sent != 1 {
		t.Errorf("the stored password was sent %d times (%q); want exactly once", sent, s.offered())
	}
}

func TestNothingStoredRunsPlainSSH(t *testing.T) {
	s := startServer(t, serverOptions{password: testPassword})
	e := newEnv(t, map[string]*server{"box": s})
	if _, _, status := e.ssh(nil, "-o", "BatchMode=yes", "box", "echo", "hello"); status == 0 {
		t.Error("login succeeded with nothing stored")
	}
	if got := s.offered(); len(got) != 0 {
		t.Errorf("passwords were sent with nothing stored: %q", got)
	}
}

func TestActivePasswordOnlyForNamedHost(t *testing.T) {
	s := startServer(t, serverOptions{password: testPassword})
	e := newEnv(t, map[string]*server{"box": s})
	// "sshc run" is not told which host ssh will reach, so the active
	// password must stay home; with -d it may go.
	_, _, status := e.run("", []string{"SSHC_PASSWORD=" + testPassword}, "run", "ssh", "-F", e.sshConfig(), "box", "echo", "hello")
	if status == 0 || len(s.offered()) != 0 && s.offered()[0] == testPassword {
		t.Errorf("the active password went to an unnamed host: status %d, offered %q", status, s.offered())
	}
	out, errs, status := e.run("", []string{"SSHC_PASSWORD=" + testPassword}, "run", "-d", "127.0.0.1", "--", "ssh", "-F", e.sshConfig(), "box", "echo", "hello")
	wantOutput(t, "run -d", out, errs, status, "hello")
}

func TestKeyboardInteractive(t *testing.T) {
	s := startServer(t, serverOptions{kbdAnswers: []string{testPassword}, kbdPrompts: []string{"Password: "}})
	e := newEnv(t, map[string]*server{"box": s})
	out, errs, status := e.ssh([]string{"SSHC_PASSWORD=" + testPassword}, "box", "echo", "hello")
	wantOutput(t, "keyboard-interactive login", out, errs, status, "hello")
}

func TestStoredInConfigFile(t *testing.T) {
	s := startServer(t, serverOptions{password: testPassword})
	e := newEnv(t, map[string]*server{"box": s})
	out, errs, status := e.run(testPassword+"\n", nil, "set")
	wantOutput(t, "sshc set", out, errs, status, "Updated!")
	out, errs, status = e.ssh(nil, "box", "echo", "hello")
	wantOutput(t, "login from the config file", out, errs, status, "hello")

	if out, _, _ := e.run("", nil, "list"); !strings.Contains(out, "[profile default]") || strings.Contains(out, testPassword) {
		t.Errorf("list should name the entry and not the secret: %q", out)
	}
	out, errs, status = e.run("", nil, "unset")
	wantOutput(t, "sshc unset", out, errs, status, "Removed!")
	if _, _, status := e.ssh(nil, "-o", "BatchMode=yes", "box", "echo", "hello"); status == 0 {
		t.Error("login still works after unset")
	}
}

// sshc's own "version" output stands in for a password manager: it is a
// program that exists on every platform and prints one predictable line.
func managerOutput(t *testing.T, e *env) string {
	t.Helper()
	out, _, _ := e.run("", nil, "version")
	return strings.TrimSpace(out)
}

func TestPasswordFromCommand(t *testing.T) {
	probe := newEnv(t, nil)
	secret := managerOutput(t, probe)
	s := startServer(t, serverOptions{password: secret})
	e := newEnv(t, map[string]*server{"box": s})
	out, errs, status := e.run("", nil, "set", "-c", `"`+sshcBin+`" version`)
	wantOutput(t, "sshc set -c", out, errs, status, "fetched by running")
	out, errs, status = e.ssh(nil, "box", "echo", "hello")
	wantOutput(t, "login through a command", out, errs, status, "hello")
}

func TestOneTimeCode(t *testing.T) {
	probe := newEnv(t, nil)
	code := managerOutput(t, probe)
	s := startServer(t, serverOptions{
		kbdAnswers: []string{testPassword, code},
		kbdPrompts: []string{"Password: ", "Verification code: "},
	})
	e := newEnv(t, map[string]*server{"box": s})
	// Without a configured code the login stops at the second question.
	if _, _, status := e.ssh([]string{"SSHC_PASSWORD=" + testPassword}, "box", "echo", "hello"); status == 0 {
		t.Fatal("login succeeded without the one-time code")
	}
	out, errs, status := e.run("", nil, "set", "-o", "-H", "box", "-c", `"`+sshcBin+`" version`)
	wantOutput(t, "sshc set -o", out, errs, status, "Updated!")
	out, errs, status = e.ssh([]string{"SSHC_PASSWORD=" + testPassword}, "box", "echo", "hello")
	wantOutput(t, "login with password and one-time code", out, errs, status, "hello")
}

func TestKeyPassphrase(t *testing.T) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("no ssh-keygen on PATH")
	}
	dir := t.TempDir()
	key := filepath.Join(dir, "id_test")
	const phrase = "key phrase#2"
	if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", phrase, "-f", key).CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v: %s", err, out)
	}
	pubBytes, err := os.ReadFile(key + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	pub, _, _, _, err := ssh.ParseAuthorizedKey(pubBytes)
	if err != nil {
		t.Fatal(err)
	}
	s := startServer(t, serverOptions{key: pub})
	e := newEnv(t, map[string]*server{"box": s})
	keyArgs := []string{"-i", key, "-o", "PubkeyAuthentication=yes", "-o", "PreferredAuthentications=publickey", "box", "echo", "hello"}

	out, errs, status := e.ssh([]string{"SSHC_PASSPHRASE=" + phrase}, keyArgs...)
	wantOutput(t, "login with a key passphrase", out, errs, status, "hello")

	// A login password must never be tried as a passphrase.
	if _, _, status := e.ssh([]string{"SSHC_PASSWORD=" + phrase}, keyArgs...); status == 0 {
		t.Error("a stored password unlocked the key")
	}
	_, errs, status = e.ssh([]string{"SSHC_PASSPHRASE=wrong"}, keyArgs...)
	if status == 0 || !strings.Contains(errs, "was not accepted") {
		t.Errorf("wrong passphrase: status %d, stderr %q", status, errs)
	}
}

func TestEach(t *testing.T) {
	a := startServer(t, serverOptions{password: testPassword})
	b := startServer(t, serverOptions{password: testPassword})
	e := newEnv(t, map[string]*server{"alpha": a, "beta": b})
	out, errs, status := e.run("", []string{"SSHC_PASSWORD=" + testPassword}, "each", "-F", e.sshConfig(), "alpha", "beta", "--", "echo", "hello")
	if status != 0 || !strings.Contains(out, "alpha | hello") || !strings.Contains(out, "beta  | hello") {
		t.Errorf("each: status %d, stdout %q, stderr %q", status, out, errs)
	}
	out, _, status = e.run("", []string{"SSHC_PASSWORD=wrong"}, "each", "-F", e.sshConfig(), "alpha", "--", "echo", "hello")
	if status == 0 || !strings.Contains(out, "1 of 1 hosts failed") {
		t.Errorf("each with a wrong password: status %d, stdout %q", status, out)
	}
}

// TestCredentialStore uses the real credential store of this machine, under a
// host name nobody else would use, and removes the entry again. It is the one
// test that proves the whole path - store, askpass callback, login - on
// Windows and macOS.
func TestCredentialStore(t *testing.T) {
	s := startServer(t, serverOptions{password: testPassword})
	alias := fmt.Sprintf("sshc-e2e-%d", os.Getpid())
	e := newEnv(t, map[string]*server{alias: s})
	on := []string{"SSHC_CREDENTIAL_STORE=on"}
	if out, _, _ := e.run("", on, "list"); strings.Contains(out, "not available") {
		t.Skipf("no credential store here: %s", strings.TrimSpace(out))
	}
	out, errs, status := e.run(testPassword+"\n", on, "set", "-H", alias)
	if status != 0 || !strings.Contains(out, "kept in") {
		t.Fatalf("set into the store: status %d, stdout %q, stderr %q", status, out, errs)
	}
	t.Cleanup(func() { e.run("", on, "unset", "-H", alias) })
	conf, _ := os.ReadFile(filepath.Join(e.dir, "sshc.conf"))
	if strings.Contains(string(conf), testPassword) || !strings.Contains(string(conf), "@credential-store") {
		t.Errorf("the config file should hold the marker, not the secret:\n%s", conf)
	}
	out, errs, status = e.ssh(on, alias, "echo", "hello")
	wantOutput(t, "login from the credential store", out, errs, status, "hello")

	out, errs, status = e.run("", on, "unset", "-H", alias)
	wantOutput(t, "unset", out, errs, status, "Removed!")
	if _, _, status := e.ssh(on, "-o", "BatchMode=yes", alias, "echo", "hello"); status == 0 {
		t.Error("login still works after the entry was removed")
	}
}

func TestDoctorAndHelp(t *testing.T) {
	e := newEnv(t, nil)
	out, _, _ := e.run("", nil, "doctor")
	if !strings.Contains(out, "OpenSSH client") || !strings.Contains(out, "sshc ") {
		t.Errorf("doctor: %q", out)
	}
	for _, topic := range []string{"set", "unset", "use", "run", "each"} {
		if out, _, status := e.run("", nil, "help", topic); status != 0 || !strings.Contains(out, "Usage: sshc "+topic) {
			t.Errorf("help %s: status %d, %q", topic, status, out)
		}
	}
}

// TestUpdate builds a copy that believes it is ancient and lets it update
// itself from the real latest release, which also exercises replacing a
// running executable. It needs the network, so it only runs when asked to.
func TestUpdate(t *testing.T) {
	if os.Getenv("SSHC_E2E_NETWORK") == "" {
		t.Skip("set SSHC_E2E_NETWORK=1 to run the tests that download from GitHub")
	}
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("no curl on PATH")
	}
	old := filepath.Join(t.TempDir(), filepath.Base(sshcBin))
	build := exec.Command("go", "build", "-ldflags", "-X github.com/W-Industries-Luke/sshc/internal/sshc.version=0.0.1", "-o", old, "../..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the old copy: %v: %s", err, out)
	}
	run := func(args ...string) string {
		cmd := exec.Command(old, args...)
		cmd.Env = append(os.Environ(), "NO_COLOR=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %v: %s", old, args, err, out)
		}
		return string(out)
	}
	if got := run("version"); !strings.Contains(got, "0.0.1") {
		t.Fatalf("the old copy reports %q", got)
	}
	if got := run("update", "--check"); !strings.Contains(got, "is available") {
		t.Fatalf("update --check: %q", got)
	}
	if got := run("update"); !strings.Contains(got, "Updated!") {
		t.Fatalf("update: %q", got)
	}
	if got := run("version"); strings.Contains(got, "0.0.1") {
		t.Errorf("still the old version after update: %q", got)
	}
}

// TestInstall runs "sshc install" for real. It changes the user's PATH and
// shell startup file, so it only runs on a CI runner, which is thrown away.
func TestInstall(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "" {
		t.Skip("only on a CI runner: this changes PATH and the shell profile")
	}
	target := t.TempDir()
	cmd := exec.Command(sshcBin, "install", target)
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	out, err := cmd.CombinedOutput()
	t.Logf("sshc install:\n%s", out)
	if err != nil || !strings.Contains(string(out), "Installed sshc") {
		t.Fatalf("install failed: %v", err)
	}
	installed := filepath.Join(target, filepath.Base(sshcBin))
	if got, err := exec.Command(installed, "version").Output(); err != nil || !strings.Contains(string(got), "sshc ") {
		t.Errorf("the installed copy does not run: %v %q", err, got)
	}
	if !strings.Contains(string(out), "shell hook") {
		t.Errorf("install said nothing about the shell hook")
	}
	// A second run must be harmless.
	if out, err := exec.Command(installed, "install", target).CombinedOutput(); err != nil || !strings.Contains(string(out), "already installed") {
		t.Errorf("second install: %v %q", err, out)
	}
}

func TestLocked(t *testing.T) {
	s := startServer(t, serverOptions{password: testPassword})
	e := newEnv(t, map[string]*server{"box": s})
	stored := []string{"SSHC_PASSWORD=" + testPassword}
	out, errs, status := e.ssh(stored, "box", "echo", "hello")
	wantOutput(t, "login before locking", out, errs, status, "hello")
	before := len(s.offered())

	// Lock by hand: "sshc lock" needs a device that can verify the user,
	// which a CI runner may not have.
	state := filepath.Join(e.dir, "state")
	if err := os.MkdirAll(state, 0o700); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(state, "locked")
	if err := os.WriteFile(lock, []byte("test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, errs, status = e.ssh(stored, "-o", "BatchMode=yes", "box", "echo", "hello")
	if status == 0 || !strings.Contains(errs, "sshc is locked") {
		t.Errorf("locked login: status %d, stderr %q; want a failure that says sshc is locked", status, errs)
	}
	if got := len(s.offered()); got != before {
		t.Errorf("a password was sent while locked: %q", s.offered())
	}
	if out, _, _ := e.run("", nil, "list"); !strings.Contains(out, "LOCKED") {
		t.Errorf("list does not show the lock: %q", out)
	}
	if out, _, status := e.run("", stored, "each", "-F", e.sshConfig(), "box", "--", "echo", "hello"); status == 0 {
		t.Errorf("each ran while locked: %q", out)
	}
	// With no terminal and nobody to verify, unlock must refuse. (Root is
	// never asked to prove who it is, so there the check cannot fail.)
	if os.Geteuid() != 0 {
		if _, errs, status := e.run("", nil, "unlock"); status == 0 || !strings.Contains(errs, "still locked") {
			t.Errorf("unlock without verification: status %d, stderr %q", status, errs)
		}
		if _, err := os.Stat(lock); err != nil {
			t.Fatal("a failed unlock removed the lock")
		}
	}

	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	out, errs, status = e.ssh(stored, "box", "echo", "hello")
	wantOutput(t, "login after the lock is gone", out, errs, status, "hello")
	if out, _, _ := e.run("", nil, "unlock"); !strings.Contains(out, "not locked") {
		t.Errorf("unlock when not locked: %q", out)
	}
}

// TestLockCommand checks that "sshc lock" either locks, or refuses because
// this machine has no way to verify the user for the unlock.
func TestLockCommand(t *testing.T) {
	e := newEnv(t, nil)
	out, errs, status := e.run("", nil, "lock")
	t.Logf("sshc lock: status %d\n%s%s", status, out, errs)
	lock := filepath.Join(e.dir, "state", "locked")
	_, err := os.Stat(lock)
	switch {
	case status == 0 && err == nil && strings.Contains(out, "Locked!"):
	case status != 0 && err != nil && strings.Contains(errs, "not locking"):
	default:
		t.Errorf("lock: status %d, lock file present: %v", status, err == nil)
	}
}
