package project

import (
	"syscall"
	"unsafe"

	"github.com/amir-saatchi/jenab/internal/disk"
)

var (
	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	procGetDriveType  = kernel32.NewProc("GetDriveTypeW")
	procGetVolumeInfo = kernel32.NewProc("GetVolumeInformationW")
)

const driveRemote = 4 // DRIVE_REMOTE

// isNetwork reports a UNC path or a drive letter mapped to a share.
func isNetwork(p string) bool {
	if uncNetwork(p) {
		return true
	}
	root, ok := volumeRoot(p)
	return ok && driveType(root) == driveRemote
}

// driveType is GetDriveType for a volume root: 3 for a fixed disk, 4 for a
// network drive.
func driveType(root string) uintptr {
	r, err := syscall.UTF16PtrFromString(root)
	if err != nil {
		return 0
	}
	t, _, _ := procGetDriveType.Call(uintptr(unsafe.Pointer(r)))
	return t
}

// volumeService reports Google Drive's virtual drive, which has the volume
// label "Google Drive" in stream mode.
func volumeService(p string) string {
	root, ok := volumeRoot(p)
	if !ok {
		return ""
	}
	r, err := syscall.UTF16PtrFromString(root)
	if err != nil {
		return ""
	}
	label := make([]uint16, 261)
	ok2, _, _ := procGetVolumeInfo.Call(uintptr(unsafe.Pointer(r)), uintptr(unsafe.Pointer(&label[0])), uintptr(len(label)), 0, 0, 0, 0, 0)
	if ok2 != 0 && syscall.UTF16ToString(label) == "Google Drive" {
		return "Google Drive"
	}
	return ""
}

// volumeRoot is the root of the volume that holds p, e.g. `C:\`.
var volumeRoot = disk.VolumeRoot
