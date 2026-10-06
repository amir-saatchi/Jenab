//go:build !windows && !unix

package project

import "errors"

// errCrossDevice stands for a rename across file systems, which this OS
// doesn't report.
var errCrossDevice = errors.New("cross-device rename")

func crossDevice(err error) bool { return errors.Is(err, errCrossDevice) }
