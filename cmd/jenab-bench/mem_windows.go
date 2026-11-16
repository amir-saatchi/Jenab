package main

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procGetProcessMemoryInfo = windows.NewLazySystemDLL("psapi.dll").NewProc("GetProcessMemoryInfo")

type memCounters struct {
	CB                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
	PrivateUsage               uintptr
}

// treeMemory sums private bytes and working sets over root and every
// process below it, as in SPIKE-022.
func treeMemory(root int) (memory, error) {
	pids, err := tree(uint32(root))
	if err != nil {
		return memory{}, err
	}
	var m memory
	for pid := range pids {
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_VM_READ, false, pid)
		if err != nil {
			continue
		}
		var mc memCounters
		mc.CB = uint32(unsafe.Sizeof(mc))
		r, _, _ := procGetProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&mc)), uintptr(mc.CB))
		windows.CloseHandle(h)
		if r == 0 {
			continue
		}
		priv, ws := float64(mc.PrivateUsage)/mb, float64(mc.WorkingSetSize)/mb
		m.PrivateMB += priv
		m.WorkingSetMB += ws
		m.Processes++
		if pid == uint32(root) {
			m.AppPrivateMB = priv
		}
	}
	return m, nil
}

// tree is root and its descendants, by PID, with their exe names.
func tree(root uint32) (map[uint32]string, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)
	type ent struct {
		parent uint32
		name   string
	}
	all := map[uint32]ent{}
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		all[e.ProcessID] = ent{e.ParentProcessID, windows.UTF16ToString(e.ExeFile[:])}
	}
	out := map[uint32]string{root: all[root].name}
	for changed := true; changed; {
		changed = false
		for pid, en := range all {
			if _, in := out[pid]; !in && pid != en.parent {
				if _, p := out[en.parent]; p {
					out[pid] = en.name
					changed = true
				}
			}
		}
	}
	return out, nil
}

// started is when the process was created.
func started(pid int) (time.Time, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return time.Time{}, err
	}
	defer windows.CloseHandle(h)
	var c, e, k, u windows.Filetime
	if err := windows.GetProcessTimes(h, &c, &e, &k, &u); err != nil {
		return time.Time{}, err
	}
	return time.Unix(0, c.Nanoseconds()), nil
}
