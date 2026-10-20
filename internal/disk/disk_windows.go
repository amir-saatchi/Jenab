package disk

import (
	"strings"
	"syscall"
	"unsafe"
)

var (
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procGetDiskFreeSpaceEx = kernel32.NewProc("GetDiskFreeSpaceExW")
	procGetVolumePathName  = kernel32.NewProc("GetVolumePathNameW")
)

// Free is the space free to this user on the volume that holds p, and a
// name for the volume; ok is false when it can't be told. p need not
// exist.
func Free(p string) (free uint64, volume string, ok bool) {
	dir := existingDir(p)
	in, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return 0, "", false
	}
	r, _, _ := procGetDiskFreeSpaceEx.Call(uintptr(unsafe.Pointer(in)), uintptr(unsafe.Pointer(&free)), 0, 0)
	if r == 0 {
		return 0, "", false
	}
	root, rok := VolumeRoot(dir)
	if !rok {
		root = dir
	}
	return free, strings.ToLower(root), true
}

// VolumeRoot is the root of the volume that holds p, e.g. `C:\`. p need
// not exist.
func VolumeRoot(p string) (string, bool) {
	in, err := syscall.UTF16PtrFromString(p)
	if err != nil {
		return "", false
	}
	buf := make([]uint16, 1024)
	ok, _, _ := procGetVolumePathName.Call(uintptr(unsafe.Pointer(in)), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if ok == 0 {
		return "", false
	}
	return syscall.UTF16ToString(buf), true
}
