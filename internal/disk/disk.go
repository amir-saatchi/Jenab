// Package disk tells the free space on the volume that holds a path, for
// the checks before an export or a long migration (SPEC 2.1, 7.5).
package disk

import (
	"os"
	"path/filepath"
)

// existingDir is p, or its nearest parent that exists.
func existingDir(p string) string {
	for {
		if _, err := os.Stat(p); err == nil {
			return p
		}
		up := filepath.Dir(p)
		if up == p {
			return p
		}
		p = up
	}
}
