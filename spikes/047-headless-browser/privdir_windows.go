package main

// The private state folder, as Jenab makes it for agent-desktop (8.10,
// internal/desktop/cli_windows.go): access for this user and SYSTEM only,
// inheriting nothing.

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

func prepareHome(dir string) error {
	user, err := currentUser()
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		if err := makePrivateDir(dir, user); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return checkPrivate(dir, user)
}

func currentUser() (*windows.SID, error) {
	tu, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	return tu.User.Sid.Copy()
}

func makePrivateDir(dir string, user *windows.SID) error {
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + user.String() + ")(A;OICI;FA;;;SY)")
	if err != nil {
		return err
	}
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	sa := &windows.SecurityAttributes{SecurityDescriptor: sd}
	sa.Length = uint32(unsafe.Sizeof(*sa))
	return windows.CreateDirectory(p, sa)
}

func checkPrivate(dir string, user *windows.SID) error {
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	ctl, _, err := sd.Control()
	if err != nil {
		return err
	}
	if ctl&windows.SE_DACL_PROTECTED == 0 {
		return fmt.Errorf("%s inherits access from its parent", dir)
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		return fmt.Errorf("%s has no ACL", dir)
	}
	for i := range uint32(dacl.AceCount) {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			return err
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !sid.Equals(user) && !sid.IsWellKnown(windows.WinLocalSystemSid) && !sid.IsWellKnown(windows.WinBuiltinAdministratorsSid) {
			return fmt.Errorf("%s grants access to %s", dir, sid.String())
		}
	}
	return nil
}

// openPinned opens the program shared for reading only, so it can't be
// swapped while it is open, and checks its SHA-256 (TASK-035).
func openPinned(path, want string) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	f := os.NewFile(uintptr(h), path)
	if err := checkHash(f, want); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}
