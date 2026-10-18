//go:build !windows && !linux && !darwin

package project

// diskSpace can't tell the free space here, so the export doesn't check it.
func diskSpace(string) (free uint64, volume string, ok bool) { return 0, "", false }
