package main

import (
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

type memCounters struct {
	cb                         uint32
	pageFaultCount             uint32
	peakWorkingSetSize         uintptr
	workingSetSize             uintptr
	quotaPeakPagedPoolUsage    uintptr
	quotaPagedPoolUsage        uintptr
	quotaPeakNonPagedPoolUsage uintptr
	quotaNonPagedPoolUsage     uintptr
	pagefileUsage              uintptr
	peakPagefileUsage          uintptr
	privateUsage               uintptr
}

var procGetProcessMemoryInfo = windows.NewLazySystemDLL("psapi.dll").NewProc("GetProcessMemoryInfo")

func counters() memCounters {
	var c memCounters
	c.cb = uint32(unsafe.Sizeof(c))
	procGetProcessMemoryInfo.Call(uintptr(windows.CurrentProcess()), uintptr(unsafe.Pointer(&c)), uintptr(c.cb))
	return c
}

func workingSet() uint64   { return uint64(counters().workingSetSize) }
func privateBytes() uint64 { return uint64(counters().privateUsage) }

// peakPrivate is the most committed memory the process has had.
func peakPrivate() uint64 { return uint64(counters().peakPagefileUsage) }

// cpuTime is the process's user plus kernel time.
func cpuTime() time.Duration {
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(windows.CurrentProcess(), &created, &exited, &kernel, &user); err != nil {
		return 0
	}
	ft := func(f windows.Filetime) time.Duration {
		return time.Duration((int64(f.HighDateTime)<<32 | int64(f.LowDateTime)) * 100)
	}
	return ft(kernel) + ft(user)
}

func cpuName() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE)
	if err != nil {
		return "unknown CPU"
	}
	defer k.Close()
	s, _, err := k.GetStringValue("ProcessorNameString")
	if err != nil {
		return "unknown CPU"
	}
	return strings.TrimSpace(s)
}
