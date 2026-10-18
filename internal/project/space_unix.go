//go:build linux || darwin

package project

import (
	"fmt"
	"syscall"
)

// diskSpace is the space free to this user on the volume that holds p, and
// a name for the volume; ok is false when it can't be told. p need not
// exist.
func diskSpace(p string) (free uint64, volume string, ok bool) {
	var st syscall.Statfs_t
	if syscall.Statfs(existingDir(p), &st) != nil {
		return 0, "", false
	}
	return st.Bavail * uint64(st.Bsize), fmt.Sprint(st.Fsid), true
}
