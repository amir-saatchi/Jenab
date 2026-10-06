//go:build unix

package project

import (
	"errors"
	"syscall"
)

// errCrossDevice is EXDEV: a rename to another file system.
var errCrossDevice error = syscall.EXDEV

// crossDevice tells whether a rename failed only because src and dst are on
// different file systems, so a copy can do it.
func crossDevice(err error) bool { return errors.Is(err, errCrossDevice) }
