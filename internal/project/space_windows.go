package project

import (
	"strings"
	"syscall"
	"unsafe"
)

var procGetDiskFreeSpaceEx = kernel32.NewProc("GetDiskFreeSpaceExW")

// diskSpace is the space free to this user on the volume that holds p, and
// a name for the volume; ok is false when it can't be told. p need not
// exist.
func diskSpace(p string) (free uint64, volume string, ok bool) {
	dir := existingDir(p)
	in, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return 0, "", false
	}
	r, _, _ := procGetDiskFreeSpaceEx.Call(uintptr(unsafe.Pointer(in)), uintptr(unsafe.Pointer(&free)), 0, 0)
	if r == 0 {
		return 0, "", false
	}
	root, rok := volumeRoot(dir)
	if !rok {
		root = dir
	}
	return free, strings.ToLower(root), true
}
