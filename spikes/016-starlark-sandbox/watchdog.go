package main

import (
	"context"
	"runtime/metrics"
	"sync/atomic"
	"time"
)

// watchdog samples the Go heap and cancels the script when it passes the cap.
// It can only stop a script between interpreter steps: one large allocation
// inside a builtin happens before the next sample.
type watchdog struct {
	peak atomic.Uint64
	done chan struct{}
}

const heapMetric = "/memory/classes/heap/objects:bytes"

func heapBytes(s []metrics.Sample) uint64 {
	metrics.Read(s)
	return s[0].Value.Uint64()
}

func startWatchdog(cancel context.CancelCauseFunc, capBytes uint64, every time.Duration) *watchdog {
	w := &watchdog{done: make(chan struct{})}
	go func() {
		s := []metrics.Sample{{Name: heapMetric}}
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-w.done:
				return
			case <-t.C:
				h := heapBytes(s)
				if h > w.peak.Load() {
					w.peak.Store(h)
				}
				if h > capBytes {
					cancel(errWatchdog)
				}
			}
		}
	}()
	return w
}

func (w *watchdog) stop() uint64 {
	close(w.done)
	return w.peak.Load()
}

func metricsSample() []metrics.Sample { return []metrics.Sample{{Name: heapMetric}} }
