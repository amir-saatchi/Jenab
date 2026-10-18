//go:build !windows

package store

import "runtime"

// processMemory is what the Go runtime holds; SQLite's own memory isn't
// seen here (tests run on Windows).
func processMemory() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Sys - m.HeapReleased
}
