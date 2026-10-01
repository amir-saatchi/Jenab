package main

import (
	"syscall"
	"time"
	"unsafe"
)

// clock and since use the Windows performance counter: time.Now on this machine only moves every ~0.5 ms (SPIKE-010).
type stamp int64

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	qpc      = kernel32.NewProc("QueryPerformanceCounter")
	qpcFreq  = func() int64 {
		var f int64
		kernel32.NewProc("QueryPerformanceFrequency").Call(uintptr(unsafe.Pointer(&f)))
		return f
	}()
)

func clock() stamp {
	var c int64
	qpc.Call(uintptr(unsafe.Pointer(&c)))
	return stamp(c)
}

func since(s stamp) time.Duration {
	return time.Duration(float64(clock()-s) * 1e9 / float64(qpcFreq))
}
