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
	totalPageFile        uint64 // the commit limit: RAM plus the page files
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
	// Free and Total are a pair: the commit headroom out of the commit
	// limit, which is more than the RAM when there is a page file. The
	// headroom is what runs out first when a command can't start.
	return Reading{Free: m.availPageFile, Total: m.totalPageFile, Level: levelOf(m.availPageFile)}, nil
}
