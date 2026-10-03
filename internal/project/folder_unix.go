//go:build linux || darwin

package project

import (
	"errors"
	"io/fs"
	"path/filepath"
)

// statfsUp runs statfs on p, or on its nearest existing parent when p
// doesn't exist yet.
func statfsUp(p string, statfs func(string) error) error {
	for {
		err := statfs(p)
		if err == nil || !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		parent := filepath.Dir(p)
		if parent == p {
			return err
		}
		p = parent
	}
}
