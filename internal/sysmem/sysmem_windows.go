package sysmem

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var globalMemoryStatusEx = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

// memoryStatusEx is MEMORYSTATUSEX.
type memoryStatusEx struct {
	length               uint32
	memoryLoad           uint32
	totalPhys            uint64
	availPhys            uint64
	totalPageFile        uint64
	availPageFile        uint64 // the commit headroom: what can still be committed
	totalVirtual         uint64
	availVirtual         uint64
	availExtendedVirtual uint64
}

func read() (Reading, error) {
	m := memoryStatusEx{length: uint32(unsafe.Sizeof(memoryStatusEx{}))}
	if ok, _, err := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m))); ok == 0 {
		return Reading{}, fmt.Errorf("sysmem: GlobalMemoryStatusEx: %w", err)
	}
	return Reading{Free: m.availPageFile, Total: m.totalPhys, Level: levelOf(m.availPageFile)}, nil
}
