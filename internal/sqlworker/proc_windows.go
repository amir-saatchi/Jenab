package sqlworker

import (
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// On Windows each worker runs in a Job Object of its own (SPIKE-016): the
// job caps the worker's committed memory, and kills it when the job's last
// handle closes, which the app holds, so workers end with the app however
// it ends. The job reports a worker that reached the cap to a completion
// port; the pool then terminates the job at once, because a Go process
// that has run out of memory inside a job can hang instead of exiting.

func configure(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}

// watchMemory is for other systems; here the job caps the memory.
func watchMemory(uint64) {}

// limits is a worker's job and the port it reports to.
type limits struct {
	mu        sync.Mutex // guards the handles against use after release
	job, port windows.Handle
	released  bool
	memHit    atomic.Bool
	drained   chan struct{} // closed when the port has said the job is empty
}

// Job messages (winnt.h).
const (
	jobMsgActiveProcessZero  = 4
	jobMsgProcessMemoryLimit = 9
)

type jobAssociateCompletionPort struct {
	CompletionKey  uintptr
	CompletionPort windows.Handle
}

// limit puts the started worker in a new job capped at memory bytes. The
// job's messages wait in the port until watch reads them.
func limit(cmd *exec.Cmd, memory uint64) (*limits, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	l := &limits{job: job, drained: make(chan struct{})}
	fail := func(err error) (*limits, error) {
		l.close()
		return nil, err
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_PROCESS_MEMORY |
		windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_DIE_ON_UNHANDLED_EXCEPTION
	info.ProcessMemoryLimit = uintptr(memory)
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		return fail(err)
	}
	if l.port, err = windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, 1); err != nil {
		return fail(err)
	}
	acp := jobAssociateCompletionPort{CompletionKey: 1, CompletionPort: l.port}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectAssociateCompletionPortInformation,
		uintptr(unsafe.Pointer(&acp)), uint32(unsafe.Sizeof(acp))); err != nil {
		return fail(err)
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		return fail(err)
	}
	err = windows.AssignProcessToJobObject(job, h)
	windows.CloseHandle(h)
	if err != nil {
		return fail(err)
	}
	return l, nil
}

var getQueuedCompletionStatus = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetQueuedCompletionStatus")

// watch reads the job's messages until the job is empty; onMemory runs
// when the worker reaches the cap. For job messages
// the OVERLAPPED pointer holds a process ID, so it is read into a
// uintptr: a pointer variable holding it could crash the garbage
// collector.
func (l *limits) watch(onMemory func()) {
	defer close(l.drained)
	for {
		var msg uint32
		var key, ov uintptr
		r, _, _ := getQueuedCompletionStatus.Call(uintptr(l.port), uintptr(unsafe.Pointer(&msg)),
			uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&ov)), windows.INFINITE)
		if r == 0 {
			return // the port was closed
		}
		switch msg {
		case jobMsgProcessMemoryLimit:
			l.memHit.Store(true)
			onMemory()
		case jobMsgActiveProcessZero:
			return
		}
	}
}

// kill terminates the job, and so the worker.
func (l *limits) kill() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.released {
		windows.TerminateJobObject(l.job, 1)
	}
}

// release closes the handles once the worker has exited, after the port
// has given every message, so memoryHit is final.
func (l *limits) release() {
	select {
	case <-l.drained:
	case <-time.After(time.Second):
	}
	l.close()
}

func (l *limits) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.released {
		return
	}
	l.released = true
	if l.port != 0 {
		windows.CloseHandle(l.port)
	}
	windows.CloseHandle(l.job)
}

// memoryHit reports that the worker reached the cap.
func (l *limits) memoryHit() bool { return l.memHit.Load() }

// peak is the most memory the worker has committed, for tests.
func (l *limits) peak() uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.released {
		return 0
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	if windows.QueryInformationJobObject(l.job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil) != nil {
		return 0
	}
	return uint64(info.PeakProcessMemoryUsed)
}
