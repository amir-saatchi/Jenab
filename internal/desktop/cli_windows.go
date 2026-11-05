package desktop

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func configure(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}

// openPinned opens the program shared for reading only, so it can't be
// written, renamed or deleted while it is open, and checks its hash.
func openPinned(path, want string) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProgram, err)
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrProgram, path, err)
	}
	f := os.NewFile(uintptr(h), path)
	if err := checkHash(f, want); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// prepareHome makes the state folder on first use, with a DACL that
// grants only this user and SYSTEM and inherits nothing, and refuses one
// that grants anyone else or inherits access (8.10). agent-desktop makes
// the same check itself.
func prepareHome(dir string) error {
	if dir == "" {
		return fmt.Errorf("%w: no folder is set", ErrHome)
	}
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
		return nil, fmt.Errorf("desktop: reading the user's SID: %w", err)
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
	if err := windows.CreateDirectory(p, sa); err != nil {
		return fmt.Errorf("desktop: making the state folder: %w", err)
	}
	return nil
}

// checkPrivate: the DACL is protected, and every entry is for the user,
// SYSTEM or the Administrators group.
func checkPrivate(dir string, user *windows.SID) error {
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("%w: reading its ACL: %v", ErrHome, err)
	}
	ctl, _, err := sd.Control()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrHome, err)
	}
	if ctl&windows.SE_DACL_PROTECTED == 0 {
		return fmt.Errorf("%w: %s inherits access from its parent", ErrHome, dir)
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		return fmt.Errorf("%w: %s has no ACL", ErrHome, dir)
	}
	for i := range uint32(dacl.AceCount) {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			return fmt.Errorf("%w: %v", ErrHome, err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			continue // a deny entry takes access away
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !sid.Equals(user) && !sid.IsWellKnown(windows.WinLocalSystemSid) && !sid.IsWellKnown(windows.WinBuiltinAdministratorsSid) {
			return fmt.Errorf("%w: %s grants access to %s", ErrHome, dir, sid.String())
		}
	}
	return nil
}

// WindowClass is the class name of the window agent-desktop calls id
// ("w-<handle>"), or "" if it can't be read.
func WindowClass(id string) string {
	h, err := strconv.ParseUint(strings.TrimPrefix(id, "w-"), 10, 64)
	if err != nil || !strings.HasPrefix(id, "w-") {
		return ""
	}
	buf := make([]uint16, 256)
	n, err := windows.GetClassName(windows.HWND(h), &buf[0], int32(len(buf)))
	if err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}
