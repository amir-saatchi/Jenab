package project

import (
	"errors"
	"syscall"
)

// errCrossDevice is ERROR_NOT_SAME_DEVICE: a rename to another drive.
var errCrossDevice error = syscall.Errno(17)

// crossDevice tells whether a rename failed only because src and dst are on
// different drives, so a copy can do it.
func crossDevice(err error) bool { return errors.Is(err, errCrossDevice) }
