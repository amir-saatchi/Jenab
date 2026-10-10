package main

import (
	"errors"
	"strings"
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
// process below it: the app and its WebView2 processes, as in SPIKE-022.
func treeMemory(root int) (memory, error) {
	pids, err := tree(uint32(root))
	if err != nil {
		return memory{}, err
	}
	var m memory
	for pid, name := range pids {
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
		} else if strings.EqualFold(name, "msedgewebview2.exe") {
			m.WebviewPrivateMB += priv
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

var (
	user32              = windows.NewLazySystemDLL("user32.dll")
	procPostMessage     = user32.NewProc("PostMessageW")
	procIsWindowVisible = user32.NewProc("IsWindowVisible")
	procGetWindow       = user32.NewProc("GetWindow")
)

// closeApp asks the app to quit as the window's close button does: a
// WM_CLOSE to its visible top-level windows. A WM_CLOSE to WebView2's
// hidden windows too would end it before it saves its profile.
func closeApp(pid int) error {
	const wmClose, gwOwner = 0x0010, 4
	var found []windows.HWND
	cb := windows.NewCallback(func(h windows.HWND, _ uintptr) uintptr {
		var p uint32
		if _, err := windows.GetWindowThreadProcessId(h, &p); err != nil || p != uint32(pid) {
			return 1
		}
		visible, _, _ := procIsWindowVisible.Call(uintptr(h))
		owner, _, _ := procGetWindow.Call(uintptr(h), gwOwner)
		if visible != 0 && owner == 0 {
			found = append(found, h)
		}
		return 1
	})
	if err := windows.EnumWindows(cb, nil); err != nil {
		return err
	}
	if len(found) == 0 {
		return errors.New("the app has no window to close")
	}
	for _, h := range found {
		procPostMessage.Call(uintptr(h), wmClose, 0, 0)
	}
	return nil
}

// treeWaiter opens the app's child processes, WebView2's among them, so
// quit can wait until they have ended too. The next start would otherwise
// join a WebView2 browser that is still shutting down, with its old
// arguments.
func treeWaiter(root int) func(time.Duration) error {
	pids, _ := tree(uint32(root))
	var hs []windows.Handle
	for pid := range pids {
		if pid == uint32(root) {
			continue
		}
		if h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid); err == nil {
			hs = append(hs, h)
		}
	}
	return func(d time.Duration) error {
		deadline := time.Now().Add(d)
		var err error
		for _, h := range hs {
			ms := max(0, time.Until(deadline).Milliseconds())
			if ev, _ := windows.WaitForSingleObject(h, uint32(ms)); ev != windows.WAIT_OBJECT_0 && err == nil {
				err = errors.New("the app's child processes didn't end in time")
			}
			windows.CloseHandle(h)
		}
		return err
	}
}
