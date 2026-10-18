//go:build unix

package sqlworker

import (
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// On Linux and macOS the pool kills a worker at the deadline, and the
// worker watches its own peak memory and exits with memExit above the
// cap; one large allocation can go over it between two checks. A worker
// ends with the app: Linux sends it SIGKILL (deathSignal), and on every
// system it exits when its stdin closes, even in the middle of a query.
// This is less strict than the Job Object on Windows, and tested on
// Windows only.

// memExit is the exit code of a worker that went over its cap.
const memExit = 3

func configure(cmd *exec.Cmd) { deathSignal(cmd) }

// watchMemory exits the worker once its peak resident memory is over
// limit bytes, checked every 20 ms.
func watchMemory(limit uint64) {
	if limit == 0 {
		return
	}
	unit := uint64(1024) // Linux gives kilobytes, macOS bytes
	if runtime.GOOS == "darwin" {
		unit = 1
	}
	go func() {
		for range time.Tick(20 * time.Millisecond) {
			var ru unix.Rusage
			if unix.Getrusage(unix.RUSAGE_SELF, &ru) == nil && uint64(ru.Maxrss)*unit > limit {
				os.Exit(memExit)
			}
		}
	}()
}

type limits struct {
	mu       sync.Mutex
	cmd      *exec.Cmd
	released bool
}

func limit(cmd *exec.Cmd, _ uint64) (*limits, error) { return &limits{cmd: cmd}, nil }

// watch has nothing to read: the worker watches its own memory.
func (l *limits) watch(func()) {}

func (l *limits) kill() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.released {
		l.cmd.Process.Kill()
	}
}

func (l *limits) release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.released = true
}

// peak is not measured here.
func (l *limits) peak() uint64 { return 0 }

func (l *limits) memoryHit() bool {
	return l.cmd.ProcessState != nil && l.cmd.ProcessState.ExitCode() == memExit
}
