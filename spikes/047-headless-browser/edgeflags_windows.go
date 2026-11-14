package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// partEdgeFlags: Edge headless with and without CREATE_NO_WINDOW.
func partEdgeFlags() {
	if *flagJob {
		j, err := windows.CreateJobObject(nil, nil)
		if err == nil {
			info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
			info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
			windows.SetInformationJobObject(j, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
			err = windows.AssignProcessToJobObject(j, windows.CurrentProcess())
		}
		p("in a kill-on-close job: %v", err)
	}
	p("| Creation flags | Exit | Output has the page |")
	p("|---|---|---|")
	for _, f := range []struct {
		name string
		attr *syscall.SysProcAttr
	}{
		{"none", nil},
		{"HideWindow", &syscall.SysProcAttr{HideWindow: true}},
		{"CREATE_NO_WINDOW", &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}},
		{"DETACHED_PROCESS", &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS}},
	} {
		dir := filepath.Join(*flagHome, fmt.Sprintf("edgeflags-%d", time.Now().UnixNano()))
		os.MkdirAll(filepath.Join(dir, "tmp"), 0o700)
		cmd := exec.Command(*flagEdge, "--headless=new", "--no-first-run", "--user-data-dir="+filepath.Join(dir, "p"), "--dump-dom", "data:text/html,<p>flags-ok</p>")
		cmd.Env = append(os.Environ(), "TEMP="+filepath.Join(dir, "tmp"), "TMP="+filepath.Join(dir, "tmp"))
		cmd.SysProcAttr = f.attr
		out, err := cmd.Output()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
		p("| %s | %d | %v |", f.name, code, strings.Contains(string(out), "flags-ok"))
	}
}
