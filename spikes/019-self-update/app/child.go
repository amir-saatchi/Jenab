package main

import (
	"os"
	"os/exec"
	"strconv"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// runChild stands in for Burrow's Starlark sandbox child (SPIKE-016): the same
// exe with a flag, running a long loop. It logs a heartbeat so the driver can
// see whether it survives the update and which binary it is running.
func runChild(args []string) {
	secs := 20
	if len(args) > 0 {
		if n, err := strconv.Atoi(args[0]); err == nil {
			secs = n
		}
	}
	end := time.Now().Add(time.Duration(secs) * time.Second)
	for i := 0; time.Now().Before(end); i++ {
		exe, _ := os.Executable()
		lg.Log("child-beat", map[string]any{"i": i, "exe": exe, "ppidAlive": ppidAlive()})
		time.Sleep(250 * time.Millisecond)
	}
	lg.Log("child-exit", nil)
}

var parentPID = os.Getppid()

func ppidAlive() bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(parentPID))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if windows.GetExitCodeProcess(h, &code) != nil {
		return false
	}
	return code == 259
}

// childJob keeps the Job Object handle alive for the parent's lifetime; with
// KILL_ON_JOB_CLOSE the child dies when the parent exits and the handle closes.
var childJob windows.Handle

func startChild(exe string) {
	secs := cfg.ChildSeconds
	if secs == 0 {
		secs = 20
	}
	cmd := exec.Command(exe, "--child", strconv.Itoa(secs))
	if err := cmd.Start(); err != nil {
		lg.Log("child-start-error", map[string]any{"err": err.Error()})
		return
	}
	info := map[string]any{"childPid": cmd.Process.Pid, "kind": cfg.Child}
	if cfg.Child == "job" {
		job, err := windows.CreateJobObject(nil, nil)
		if err == nil {
			var li windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
			li.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
			_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&li)), uint32(unsafe.Sizeof(li)))
		}
		if err == nil {
			var h windows.Handle
			h, err = windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
			if err == nil {
				err = windows.AssignProcessToJobObject(job, h)
				windows.CloseHandle(h)
			}
		}
		if err != nil {
			info["jobErr"] = err.Error()
		}
		childJob = job
	}
	go func() { _ = cmd.Wait() }()
	lg.Log("child-started", info)
}
