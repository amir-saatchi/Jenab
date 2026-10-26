package main

import (
	"errors"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// A job is a Windows Job Object that kills every process in it when it
// closes (SPEC 8.7: a server is stopped with its whole process tree).
// Processes are started suspended, put in the job, then resumed, so no
// child can start before the process is in the job.
//
// Kill-on-close is "armed" while the job holds only the processes the
// spike started itself. As soon as it holds anything else (say Viso's
// shim launched the Viso app), or the job can't be read, it is disarmed:
// from then on nothing in the job is killed by closing it, by a crash or
// by the console closing, and only the adopted processes are stopped, by
// the handles kept for them (so a reused PID is never hit).
type job struct {
	mu      sync.Mutex
	h       windows.Handle
	armed   bool
	adopted map[int]windows.Handle
}

func newJob() (*job, error) {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	j := &job{h: h, adopted: map[int]windows.Handle{}}
	if err := j.setKill(true); err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	return j, nil
}

func (j *job) setKill(on bool) error {
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	if on {
		info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	}
	if _, err := windows.SetInformationJobObject(j.h, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		return err
	}
	j.armed = on
	return nil
}

// suspended makes cmd start suspended, without a console window.
func suspended(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED | windows.CREATE_NO_WINDOW
}

var ntResumeProcess = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtResumeProcess")

// adopt puts a suspended process in the job, keeps a handle to it and
// resumes it.
func (j *job) adopt(pid int) error {
	const access = windows.PROCESS_SET_QUOTA | windows.PROCESS_TERMINATE | windows.PROCESS_SUSPEND_RESUME |
		windows.PROCESS_QUERY_LIMITED_INFORMATION | windows.SYNCHRONIZE
	ph, err := windows.OpenProcess(access, false, uint32(pid))
	if err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := windows.AssignProcessToJobObject(j.h, ph); err != nil {
		windows.CloseHandle(ph)
		return err
	}
	if old, ok := j.adopted[pid]; ok {
		windows.CloseHandle(old)
	}
	j.adopted[pid] = ph
	if r, _, _ := ntResumeProcess.Call(uintptr(ph)); r != 0 {
		return fmt.Errorf("NtResumeProcess: status %#x", r)
	}
	return nil
}

// pids lists the processes in the job. Any error, including more
// processes than the buffer holds, is returned: callers must then assume
// the job holds processes they don't know.
func (j *job) pids() ([]int, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.pidsLocked()
}

func (j *job) pidsLocked() ([]int, error) {
	if j.h == 0 {
		return nil, errors.New("job closed")
	}
	type list struct {
		Assigned, InList uint32
		IDs              [1024]uintptr
	}
	var l list
	if err := windows.QueryInformationJobObject(j.h, windows.JobObjectBasicProcessIdList,
		uintptr(unsafe.Pointer(&l)), uint32(unsafe.Sizeof(l)), nil); err != nil {
		return nil, err
	}
	if l.InList < l.Assigned {
		return nil, fmt.Errorf("job holds %d processes, listed %d", l.Assigned, l.InList)
	}
	out := make([]int, 0, l.InList)
	for i := range l.InList {
		out = append(out, int(l.IDs[i]))
	}
	return out, nil
}

// othersLocked lists the job's processes besides keep; ok is false when
// the job can't be read.
func (j *job) othersLocked(keep int) (others []int, ok bool) {
	ps, err := j.pidsLocked()
	if err != nil {
		return nil, false
	}
	for _, p := range ps {
		if p != keep {
			others = append(others, p)
		}
	}
	return others, true
}

// watch disarms the job as soon as it holds anything besides shim, or
// can't be read. It stops when stop is called.
func (j *job) watch(shim int) (stop func()) {
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(50 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
			}
			j.mu.Lock()
			if j.h == 0 || !j.armed {
				j.mu.Unlock()
				return
			}
			others, ok := j.othersLocked(shim)
			if !ok || len(others) > 0 {
				err := j.setKill(false)
				log.Printf("job: holds %v besides the shim %d (readable %v): kill-on-close off (err %v)", others, shim, ok, err)
			}
			j.mu.Unlock()
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { close(done) }) }
}

// closeOnly stops shim and closes the job. If the job holds only shim
// (checked twice, right before), the whole job is terminated. Otherwise,
// or if the job can't be read, kill-on-close is cleared, only shim is
// stopped by its kept handle, and the rest keeps running; if kill-on-close
// can't be cleared, the handle stays open rather than kill them. It
// returns the pids it left running.
func (j *job) closeOnly(shim int) ([]int, error) {
	if j == nil {
		return nil, nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.h == 0 {
		return nil, nil
	}
	others, ok := j.othersLocked(shim)
	if ok && len(others) == 0 && j.armed {
		if others, ok = j.othersLocked(shim); ok && len(others) == 0 {
			_ = windows.TerminateJobObject(j.h, 1)
			return nil, j.closeLocked()
		}
	}
	if j.armed {
		if err := j.setKill(false); err != nil {
			return others, fmt.Errorf("job holds %v (readable %v); kill-on-close not cleared (%v), handle left open", others, ok, err)
		}
	}
	var kerr error
	if ph, has := j.adopted[shim]; has {
		if err := windows.TerminateProcess(ph, 1); err != nil && !exited(ph) {
			kerr = err
		}
	}
	if !ok {
		kerr = errors.Join(kerr, errors.New("job not readable; left it running"))
	}
	return others, errors.Join(kerr, j.closeLocked())
}

// release closes the job without killing anything in it.
func (j *job) release() {
	if j == nil {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.h == 0 {
		return
	}
	if err := j.setKill(false); err != nil {
		log.Printf("job: can't clear kill-on-close (%v); handle left open", err)
		return
	}
	_ = j.closeLocked()
}

// close kills every process in the job. Only for jobs that hold the
// spike's own processes (the fake server).
func (j *job) close() error {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.h == 0 {
		return nil
	}
	_ = windows.TerminateJobObject(j.h, 1)
	return j.closeLocked()
}

func (j *job) closeLocked() error {
	for pid, ph := range j.adopted {
		windows.CloseHandle(ph)
		delete(j.adopted, pid)
	}
	err := windows.CloseHandle(j.h)
	j.h = 0
	return err
}

func exited(ph windows.Handle) bool {
	ev, err := windows.WaitForSingleObject(ph, 0)
	return err == nil && ev == windows.WAIT_OBJECT_0
}

// processes lists the running processes, with their start times.
func processes() (map[int]proc, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)
	out := map[int]proc{}
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		p := proc{PID: int(e.ProcessID), Parent: int(e.ParentProcessID), Exe: windows.UTF16ToString(e.ExeFile[:])}
		p.Created = created(p.PID)
		out[p.PID] = p
	}
	return out, nil
}

// created is a process's start time (0 if it can't be read).
func created(pid int) uint64 {
	ph, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(ph)
	var c, x, k, u windows.Filetime
	if err := windows.GetProcessTimes(ph, &c, &x, &k, &u); err != nil {
		return 0
	}
	return uint64(c.HighDateTime)<<32 | uint64(c.LowDateTime)
}

// tree is pid and its descendants that are running now. A process counts
// as a child only if it started after its parent: Windows keeps a dead
// parent's PID in its children, and the PID may since belong to another
// process.
func tree(pid int) []proc {
	ps, err := processes()
	if err != nil {
		return nil
	}
	var out []proc
	var walk func(proc)
	walk = func(p proc) {
		out = append(out, p)
		for _, c := range ps {
			if c.Parent == p.PID && c.PID != p.PID && p.Created != 0 && c.Created >= p.Created {
				walk(c)
			}
		}
	}
	if p, ok := ps[pid]; ok {
		walk(p)
	}
	return out
}

// still reports which of ps still run: the same PID and start time.
func still(ps []proc) []proc {
	now, err := processes()
	if err != nil {
		return nil
	}
	var out []proc
	for _, p := range ps {
		if q, ok := now[p.PID]; ok && q.Created == p.Created && strings.EqualFold(q.Exe, p.Exe) {
			out = append(out, q)
		}
	}
	return out
}

// killProc stops p if it is still the same process (PID, start time) and
// its exe is named exe. For the spike's own leftovers (fake.exe).
func killProc(p proc, exe string) error {
	if !strings.EqualFold(p.Exe, exe) {
		return fmt.Errorf("pid %d is %s, not %s", p.PID, p.Exe, exe)
	}
	ph, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(p.PID))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(ph)
	var c, x, k, u windows.Filetime
	if err := windows.GetProcessTimes(ph, &c, &x, &k, &u); err != nil {
		return err
	}
	if uint64(c.HighDateTime)<<32|uint64(c.LowDateTime) != p.Created || p.Created == 0 {
		return fmt.Errorf("pid %d is a different process now", p.PID)
	}
	buf := make([]uint16, windows.MAX_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(ph, 0, &buf[0], &n); err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Base(windows.UTF16ToString(buf[:n])), exe) {
		return fmt.Errorf("pid %d is not %s", p.PID, exe)
	}
	return windows.TerminateProcess(ph, 1)
}
