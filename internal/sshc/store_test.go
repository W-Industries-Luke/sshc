package sshc

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
)

// memStore is a credential store that lives in the test process.
type memStore map[string]string

func (memStore) name() string { return "the test store" }
func (m memStore) get(key string) (string, error) {
	if v, ok := m[key]; ok {
		return v, nil
	}
	return "", errNotFound
}
func (m memStore) set(key, value string) error { m[key] = value; return nil }
func (m memStore) del(key string) error {
	if _, ok := m[key]; !ok {
		return errNotFound
	}
	delete(m, key)
	return nil
}

func TestStoreKey(t *testing.T) {
	if got := storeKey("Profile  Work", "Password"); got != "profile work/password" {
		t.Errorf("storeKey = %q", got)
	}
}

// TestPlatformStore round-trips real entries through this machine's
// credential store. Linux machines without a keyring skip it.
func TestPlatformStore(t *testing.T) {
	st, err := platformStore()
	if err != nil {
		if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
			t.Fatalf("no credential store: %v", err)
		}
		t.Skipf("no credential store here: %v", err)
	}
	key := fmt.Sprintf("test/unit %d/password", os.Getpid())
	t.Cleanup(func() { _ = st.del(key) })

	if _, err := st.get(key); !errors.Is(err, errNotFound) {
		t.Fatalf("get of a missing entry: err = %v; want errNotFound", err)
	}
	for _, value := range []string{"plain", `it's "quoted" $HOME \ ; * -w`, " spaces ", "ünïcödé ✓ 密码", "-starts-with-dash"} {
		if err := st.set(key, value); err != nil {
			t.Fatalf("set(%q): %v", value, err)
		}
		if got, err := st.get(key); err != nil || got != value {
			t.Fatalf("get after set(%q) = %q, %v", value, got, err)
		}
	}
	if err := st.del(key); err != nil {
		t.Fatalf("del: %v", err)
	}
	if _, err := st.get(key); !errors.Is(err, errNotFound) {
		t.Fatalf("get after del: err = %v; want errNotFound", err)
	}
	if err := st.del(key); !errors.Is(err, errNotFound) {
		t.Fatalf("del of a missing entry: err = %v; want errNotFound", err)
	}
}

func TestSecretFromStore(t *testing.T) {
	st := memStore{"profile work/password": "from-store", "key id_x/passphrase": "pp-store"}
	cfg := &config{path: "sshc.conf", entries: map[string]string{
		configKey("", "profile"):                "work",
		configKey("profile work", "password"):   storeMarker,
		configKey("profile work", "passphrase"): "plain-pp",
		configKey("key id_x", "passphrase"):     storeMarker,
		configKey("host gone", "password"):      storeMarker,
	}}
	r := &resolver{cfg: cfg, store: st, dests: []dest{{alias: "box", hostname: "10.0.0.5"}, {alias: "gone", hostname: "10.0.0.6"}}}

	if pw, from, note := r.lookup("luke", "10.0.0.5"); pw != "from-store" || !strings.Contains(from, "kept in the test store") || note != "" {
		t.Errorf("password via store = %q from %q note %q", pw, from, note)
	}
	if pp, from, _ := r.lookupPassphrase("/home/luke/.ssh/id_x"); pp != "pp-store" || !strings.Contains(from, "[key id_x], kept in") {
		t.Errorf("key passphrase via store = %q from %q", pp, from)
	}
	if pp, from, _ := r.lookupPassphrase("/home/luke/.ssh/other"); pp != "plain-pp" || !strings.Contains(from, "[profile work] in sshc.conf") {
		t.Errorf("plain passphrase = %q from %q", pp, from)
	}
	// The file says it is in the store, but the store has lost it.
	if pw, _, note := r.lookup("luke", "10.0.0.6"); pw != "" || !strings.Contains(note, "could not read") {
		t.Errorf("missing store entry: password %q, note %q", pw, note)
	}
	// The marker itself must never be handed out as a password.
	r2 := &resolver{cfg: cfg, dests: r.dests}
	old := systemStore
	systemStore = func() (secretStore, error) { return nil, errors.New("none here") }
	defer func() { systemStore = old }()
	if pw, _, note := r2.lookup("luke", "10.0.0.5"); pw != "" || !strings.Contains(note, "cannot be used here") {
		t.Errorf("no store: password %q, note %q", pw, note)
	}
}
