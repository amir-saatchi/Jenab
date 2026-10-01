package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Run modes for hostile scripts. Every mode runs in a child process inside a
// Job Object, so a memory bomb can never take more than the job's cap.
const (
	modeLib      = "lib"      // library limits only; job cap 1 GB as a safety net
	modeWatchdog = "watchdog" // library limits + in-process heap watchdog at 256 MB; job cap 1 GB as a safety net
	modeJob      = "job"      // library limits; job cap 256 MB is the memory limit

	safetyCap   = 1 << 30
	jobCap      = 256 << 20
	watchCap    = 256 << 20
	watchEvery  = time.Millisecond
	hardTimeout = 10 * time.Second
)

// childResult is what the child prints as one JSON line.
type childResult struct {
	Stopped  string  `json:"stopped"`
	Result   string  `json:"result"`
	Ms       float64 `json:"ms"`
	WatchMax uint64  `json:"watch_max"`
}

// outcome is what the parent records for one run.
type outcome struct {
	Stopped string
	Result  string
	Time    time.Duration
	PeakMB  float64
	Stderr  string // last part of the child's stderr, for debugging
}

// childMain runs one hostile script: engine, case id, mode.
func childMain(args []string) {
	eng, c, mode := engineByName(args[0]), hostileByID(args[1]), args[2]
	src := c.src[args[0]]
	// Wait until the parent has put this process into the job.
	bufio.NewReader(os.Stdin).ReadString('\n')

	ctx, cancelT := context.WithTimeoutCause(context.Background(), runTimeout, errTimeout)
	defer cancelT()
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	var w *watchdog
	if mode == modeWatchdog {
		w = startWatchdog(cancel, watchCap, watchEvery)
	}
	start := clock()
	v, err := eng.Run(ctx, src, RunOpts{NoSteps: c.noSteps})
	d := since(start)
	r := childResult{Stopped: classify(err, ctx), Ms: float64(d) / 1e6}
	if err == nil {
		r.Result = short(v)
	}
	if w != nil {
		r.WatchMax = w.stop()
	}
	json.NewEncoder(os.Stdout).Encode(r)
}

// runChild starts the child in a new job with the given memory cap. The job
// posts a message to a completion port when the child hits the cap; the parent
// then terminates the job at once, because a Go process that has hit
// "out of memory" inside a job can hang instead of exiting (seen in this spike).
func runChild(engine, caseID, mode string) outcome {
	exe, _ := os.Executable()
	capBytes := uintptr(safetyCap)
	if mode == modeJob {
		capBytes = jobCap
	}
	job, port, err := newJob(capBytes)
	if err != nil {
		return outcome{Stopped: "job error: " + err.Error()}
	}
	defer windows.CloseHandle(job)
	defer windows.CloseHandle(port)

	cmd := exec.Command(exe, "child", engine, caseID, mode)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	in, _ := cmd.StdinPipe()
	if err := cmd.Start(); err != nil {
		return outcome{Stopped: "start error: " + err.Error()}
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_INFORMATION, false, uint32(cmd.Process.Pid))
	if err == nil {
		err = windows.AssignProcessToJobObject(job, h)
		windows.CloseHandle(h)
	}
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return outcome{Stopped: "assign error: " + err.Error()}
	}
	start := clock()
	in.Write([]byte("go\n"))
	in.Close()

	var killed, memHit atomic.Bool
	var memAt atomic.Int64
	done := make(chan struct{})
	go func() { // job messages
		for {
			select {
			case <-done:
				return
			default:
			}
			var msg uint32
			var key uintptr
			var ov *windows.Overlapped
			if windows.GetQueuedCompletionStatus(port, &msg, &key, &ov, 50) != nil {
				continue
			}
			if msg == jobMsgProcessMemoryLimit || msg == jobMsgJobMemoryLimit {
				if !memHit.Swap(true) {
					memAt.Store(int64(since(start)))
					windows.TerminateJobObject(job, 3)
				}
			}
		}
	}()
	t := time.AfterFunc(hardTimeout, func() {
		killed.Store(true)
		windows.TerminateJobObject(job, 1)
	})
	cmd.Wait()
	t.Stop()
	close(done)
	o := outcome{Time: since(start), PeakMB: float64(jobPeak(job)) / (1 << 20)}

	var r childResult
	if line := strings.TrimSpace(stdout.String()); !memHit.Load() && line != "" && json.Unmarshal([]byte(line), &r) == nil {
		o.Stopped, o.Result = r.Stopped, r.Result
		o.Time = time.Duration(r.Ms * 1e6)
		return o
	}
	errText := stderr.String()
	if len(errText) > 600 && os.Getenv("S16_FULL") == "" {
		o.Stderr = errText[:600]
	} else {
		o.Stderr = errText
	}
	switch {
	case memHit.Load():
		o.Stopped = fmt.Sprintf("**job cap %d MB**", capBytes>>20)
		o.Time = time.Duration(memAt.Load())
	case killed.Load():
		o.Stopped = fmt.Sprintf("**hard kill at %s**", hardTimeout)
	case strings.Contains(errText, "stack overflow") || strings.Contains(errText, "stack exceeds"):
		o.Stopped = "**crash: Go stack overflow**"
	default:
		o.Stopped = "**crash:** " + firstLine(strings.TrimSpace(errText))
	}
	return o
}

const (
	jobMsgProcessMemoryLimit = 9  // JOB_OBJECT_MSG_PROCESS_MEMORY_LIMIT
	jobMsgJobMemoryLimit     = 10 // JOB_OBJECT_MSG_JOB_MEMORY_LIMIT
)

// jobAssociateCompletionPort mirrors JOBOBJECT_ASSOCIATE_COMPLETION_PORT.
type jobAssociateCompletionPort struct {
	CompletionKey  uintptr
	CompletionPort windows.Handle
}

func newJob(capBytes uintptr) (windows.Handle, windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, 0, err
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_PROCESS_MEMORY | windows.JOB_OBJECT_LIMIT_JOB_MEMORY | windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_DIE_ON_UNHANDLED_EXCEPTION
	info.ProcessMemoryLimit = capBytes
	info.JobMemoryLimit = capBytes
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return 0, 0, err
	}
	port, err := windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, 1)
	if err != nil {
		windows.CloseHandle(job)
		return 0, 0, err
	}
	acp := jobAssociateCompletionPort{CompletionKey: 1, CompletionPort: port}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectAssociateCompletionPortInformation, uintptr(unsafe.Pointer(&acp)), uint32(unsafe.Sizeof(acp))); err != nil {
		windows.CloseHandle(port)
		windows.CloseHandle(job)
		return 0, 0, err
	}
	return job, port, nil
}

// jobPeak returns the peak committed memory of the job's process.
func jobPeak(job windows.Handle) uintptr {
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	if err := windows.QueryInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil); err != nil {
		return 0
	}
	return info.PeakProcessMemoryUsed
}
