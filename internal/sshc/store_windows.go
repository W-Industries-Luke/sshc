package sshc

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows Credential Manager, through the Cred* functions in advapi32. Entries
// are generic credentials named "sshc:<key>"; they are encrypted with the
// user's logon credentials and show up under "Windows Credentials" in the
// Credential Manager control panel.

const (
	credPrefix              = "sshc:"
	credTypeGeneric         = 1
	credPersistLocalMachine = 2
	errorNotFound           = syscall.Errno(1168)
)

// credential mirrors the Win32 CREDENTIALW structure.
type credential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

var (
	advapi32       = windows.NewLazySystemDLL("advapi32.dll")
	procCredWrite  = advapi32.NewProc("CredWriteW")
	procCredRead   = advapi32.NewProc("CredReadW")
	procCredDelete = advapi32.NewProc("CredDeleteW")
	procCredFree   = advapi32.NewProc("CredFree")
)

type credManager struct{}

func platformStore() (secretStore, error) { return credManager{}, nil }

func (credManager) name() string { return "Windows Credential Manager" }

func (credManager) set(key, value string) error {
	target, err := windows.UTF16PtrFromString(credPrefix + key)
	if err != nil {
		return err
	}
	user, _ := windows.UTF16PtrFromString(prog)
	blob := []byte(value)
	cred := credential{
		Type:               credTypeGeneric,
		TargetName:         target,
		CredentialBlobSize: uint32(len(blob)),
		CredentialBlob:     &blob[0],
		Persist:            credPersistLocalMachine,
		UserName:           user,
	}
	if r, _, err := procCredWrite.Call(uintptr(unsafe.Pointer(&cred)), 0); r == 0 {
		return err
	}
	return nil
}

func (credManager) get(key string) (string, error) {
	target, err := windows.UTF16PtrFromString(credPrefix + key)
	if err != nil {
		return "", err
	}
	var cred *credential
	r, _, err := procCredRead.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&cred)))
	if r == 0 {
		if err == errorNotFound {
			return "", errNotFound
		}
		return "", err
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(cred)))
	if cred.CredentialBlobSize == 0 {
		return "", nil
	}
	return string(unsafe.Slice(cred.CredentialBlob, cred.CredentialBlobSize)), nil
}

func (credManager) del(key string) error {
	target, err := windows.UTF16PtrFromString(credPrefix + key)
	if err != nil {
		return err
	}
	if r, _, err := procCredDelete.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0); r == 0 {
		if err == errorNotFound {
			return errNotFound
		}
		return err
	}
	return nil
}
