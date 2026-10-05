package sshc

import (
	"errors"
	"os"
	"strings"
)

// storeMarker stands in the config file for a secret that is kept in the
// operating system's credential store rather than in the file itself.
const storeMarker = "@credential-store"

// envNoStore turns the credential store off: "sshc set" then writes plain
// text to the config file, and entries already in the store are not read.
const envNoStore = "SSHC_CREDENTIAL_STORE"

var errNotFound = errors.New("no such entry")

// secretStore is the operating system's place for secrets: Windows Credential
// Manager, the macOS Keychain, or the Secret Service on Linux.
type secretStore interface {
	name() string
	get(key string) (string, error) // errNotFound when there is no such entry
	set(key, value string) error
	del(key string) error
}

// storeKey names a config entry inside the store, e.g. "profile work/password".
func storeKey(section, key string) string {
	return strings.ReplaceAll(configKey(section, key), "\n", "/")
}

// systemStore returns the credential store, or the reason there is none. It
// is a variable so that tests can substitute one.
var systemStore = func() (secretStore, error) {
	if isNo(os.Getenv(envNoStore)) {
		return nil, errors.New("turned off by $" + envNoStore)
	}
	return platformStore()
}
