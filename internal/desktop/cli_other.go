//go:build !windows

package desktop

import (
	"os"
	"os/exec"
)

// Desktop control is Windows-only in v1 (8.10): the CLI refuses to start.

func configure(*exec.Cmd) {}

func openPinned(string, string) (*os.File, error) { return nil, ErrUnsupported }

func prepareHome(string) error { return ErrUnsupported }

// WindowClass is for Windows.
func WindowClass(string) string { return "" }
