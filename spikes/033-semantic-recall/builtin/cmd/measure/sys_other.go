//go:build !windows

package main

import (
	"runtime"
	"time"
)

// Outside Windows the spike reports the Go runtime's view only.
func workingSet() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Sys
}

func privateBytes() uint64 { return workingSet() }

func peakPrivate() uint64 { return workingSet() }

func cpuTime() time.Duration { return 0 }

func cpuName() string { return "unknown CPU" }
