//go:build !windows && !darwin && !linux

package sshc

import "errors"

func platformStore() (secretStore, error) {
	return nil, errors.New("sshc has no credential store support on this system")
}
