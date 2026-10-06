package project

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// TestSamePathShortName checks that an 8.3 short name names the same
// folder, where the volume makes short names.
func TestSamePathShortName(t *testing.T) {
	long := filepath.Join(t.TempDir(), "A long folder name")
	if err := os.MkdirAll(long, 0o755); err != nil {
		t.Fatal(err)
	}
	p, err := syscall.UTF16PtrFromString(long)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]uint16, syscall.MAX_PATH)
	n, err := syscall.GetShortPathName(p, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 || int(n) > len(buf) {
		t.Skipf("no short name: %v", err)
	}
	short := syscall.UTF16ToString(buf[:n])
	if filepath.Base(short) == filepath.Base(long) {
		t.Skip("this volume makes no short names")
	}
	if !samePath(long, short) {
		t.Errorf("%s and %s", long, short)
	}
}
