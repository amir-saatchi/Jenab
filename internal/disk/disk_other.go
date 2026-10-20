//go:build !windows && !linux && !darwin

package disk

// Free can't tell the free space here, so callers don't check it.
func Free(string) (free uint64, volume string, ok bool) { return 0, "", false }
