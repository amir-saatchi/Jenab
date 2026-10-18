//go:build !windows

package project

import (
	"os"
	"syscall"
)

// rmdir removes the folder name if it is empty, and never a file.
func rmdir(name string) error {
	if err := syscall.Rmdir(name); err != nil {
		return &os.PathError{Op: "rmdir", Path: name, Err: err}
	}
	return nil
}
