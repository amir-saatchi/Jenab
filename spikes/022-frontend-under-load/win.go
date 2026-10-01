// Windows helpers: process-tree memory, CPU sampling, process start time,
// foreground check and key presses. No cgo; plain syscalls.
package main

import (
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	psapi                = windows.NewLazySystemDLL("psapi.dll")
	user32               = windows.NewLazySystemDLL("user32.dll")
	procGetSystemTimes   = kernel32.NewProc("GetSystemTimes")
	procGetProcessMemory = psapi.NewProc("GetProcessMemoryInfo")
	procGetForeground    = user32.NewProc("GetForegroundWindow")
	procSetForeground    = user32.NewProc("SetForegroundWindow")
	procKeybdEvent       = user32.NewProc("keybd_event")
)

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

type ProcMem struct {
	PID       uint32  `json:"pid"`
	Name      string  `json:"name"`
	WorkingMB float64 `json:"workingSetMB"`
	PrivateMB float64 `json:"privateMB"`
	PeakWSMB  float64 `json:"peakWorkingSetMB"`
}

type MemSnapshot struct {
	At            int64     `json:"at"`
	WebviewCount  int       `json:"webviewProcesses"`
	WebviewWSMB   float64   `json:"webviewWorkingSetMB"`
	WebviewPrivMB float64   `json:"webviewPrivateMB"`
	AppWSMB       float64   `json:"appWorkingSetMB"`
	AppPrivMB     float64   `json:"appPrivateMB"`
	TotalWSMB     float64   `json:"totalWorkingSetMB"`
	TotalPrivMB   float64   `json:"totalPrivateMB"`
	Procs         []ProcMem `json:"procs"`
}

// descendants returns pid -> exe name for all processes below root (not root itself).
func descendants(root uint32) map[uint32]string {
	out := map[uint32]string{}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return out
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
	// walk: repeated passes until no new children are found
	in := map[uint32]bool{root: true}
	for changed := true; changed; {
		changed = false
		for pid, en := range all {
			if !in[pid] && in[en.parent] && pid != en.parent {
				in[pid] = true
				out[pid] = en.name
				changed = true
			}
		}
	}
	return out
}

func procMem(pid uint32) (memCounters, bool) {
	var mc memCounters
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_VM_READ, false, pid)
	if err != nil {
		return mc, false
	}
	defer windows.CloseHandle(h)
	mc.CB = uint32(unsafe.Sizeof(mc))
	r, _, _ := procGetProcessMemory.Call(uintptr(h), uintptr(unsafe.Pointer(&mc)), uintptr(mc.CB))
	return mc, r != 0
}

const mb = 1024 * 1024

func memSnapshot() MemSnapshot {
	s := MemSnapshot{At: time.Now().UnixMilli()}
	self := uint32(os.Getpid())
	if mc, ok := procMem(self); ok {
		s.AppWSMB = float64(mc.WorkingSetSize) / mb
		s.AppPrivMB = float64(mc.PrivateUsage) / mb
	}
	for pid, name := range descendants(self) {
		mc, ok := procMem(pid)
		if !ok {
			continue
		}
		p := ProcMem{PID: pid, Name: name, WorkingMB: float64(mc.WorkingSetSize) / mb, PrivateMB: float64(mc.PrivateUsage) / mb, PeakWSMB: float64(mc.PeakWorkingSetSize) / mb}
		s.Procs = append(s.Procs, p)
		if strings.EqualFold(name, "msedgewebview2.exe") {
			s.WebviewCount++
			s.WebviewWSMB += p.WorkingMB
			s.WebviewPrivMB += p.PrivateMB
		}
	}
	sort.Slice(s.Procs, func(i, j int) bool { return s.Procs[i].WorkingMB > s.Procs[j].WorkingMB })
	s.TotalWSMB = s.AppWSMB + s.WebviewWSMB
	s.TotalPrivMB = s.AppPrivMB + s.WebviewPrivMB
	return s
}

func ft(f windows.Filetime) uint64 { return uint64(f.HighDateTime)<<32 | uint64(f.LowDateTime) }

// processStartEpochMs is the creation time of this process.
func processStartEpochMs() float64 {
	var c, e, k, u windows.Filetime
	if err := windows.GetProcessTimes(windows.CurrentProcess(), &c, &e, &k, &u); err != nil {
		return 0
	}
	return float64(c.Nanoseconds()) / 1e6
}

// treeCPU returns kernel+user time (100ns units) per process for this process and its descendants.
func treeCPU() map[uint32]uint64 {
	self := uint32(os.Getpid())
	pids := []uint32{self}
	for pid := range descendants(self) {
		pids = append(pids, pid)
	}
	out := map[uint32]uint64{}
	for _, pid := range pids {
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
		if err != nil {
			continue
		}
		var c, e, k, u windows.Filetime
		if windows.GetProcessTimes(h, &c, &e, &k, &u) == nil {
			out[pid] = ft(k) + ft(u)
		}
		windows.CloseHandle(h)
	}
	return out
}

func systemTimes() (idle, busy uint64) {
	var i, k, u windows.Filetime
	procGetSystemTimes.Call(uintptr(unsafe.Pointer(&i)), uintptr(unsafe.Pointer(&k)), uintptr(unsafe.Pointer(&u)))
	// kernel time includes idle time
	return ft(i), ft(k) + ft(u) - ft(i)
}

// CPUSample: system-wide busy % and our process tree's CPU in "% of one core" over the interval ending At.
type CPUSample struct {
	At      int64   `json:"at"`
	SysPct  float64 `json:"sysPct"`
	TreePct float64 `json:"treeCorePct"`
}

type cpuSampler struct {
	mu      sync.Mutex
	samples []CPUSample
}

func (c *cpuSampler) run(every time.Duration) {
	pi, pb := systemTimes()
	pt := treeCPU()
	last := time.Now()
	for range time.Tick(every) {
		i, b := systemTimes()
		t := treeCPU()
		// only processes seen in the previous sample count, so a new process does not add its whole history
		var dt uint64
		for pid, v := range t {
			if p, ok := pt[pid]; ok && v >= p {
				dt += v - p
			}
		}
		now := time.Now()
		di, db := float64(i-pi), float64(b-pb)
		s := CPUSample{At: now.UnixMilli()}
		if di+db > 0 {
			s.SysPct = 100 * db / (di + db)
		}
		wall := float64(now.Sub(last).Nanoseconds()) / 100 // 100ns units
		if wall > 0 {
			s.TreePct = 100 * float64(dt) / wall
		}
		c.mu.Lock()
		c.samples = append(c.samples, s)
		c.mu.Unlock()
		pi, pb, pt, last = i, b, t, now
	}
}

type CPUStats struct {
	Samples     int     `json:"samples"`
	SysMeanPct  float64 `json:"sysMeanPct"`
	SysMaxPct   float64 `json:"sysMaxPct"`
	TreeMeanPct float64 `json:"treeMeanCorePct"`
	TreeMaxPct  float64 `json:"treeMaxCorePct"`
}

func (c *cpuSampler) stats(from, to int64) CPUStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	var st CPUStats
	for _, s := range c.samples {
		if s.At < from || s.At > to {
			continue
		}
		st.Samples++
		st.SysMeanPct += s.SysPct
		st.TreeMeanPct += s.TreePct
		st.SysMaxPct = max(st.SysMaxPct, s.SysPct)
		st.TreeMaxPct = max(st.TreeMaxPct, s.TreePct)
	}
	if st.Samples > 0 {
		st.SysMeanPct /= float64(st.Samples)
		st.TreeMeanPct /= float64(st.Samples)
	}
	return st
}

func foregroundIs(hwnd uintptr) bool {
	fg, _, _ := procGetForeground.Call()
	return hwnd != 0 && fg == hwnd
}

func setForeground(hwnd uintptr) { procSetForeground.Call(hwnd) }

// pressKey presses and releases a virtual key, but only if our window is in front,
// so keystrokes can never land in another application.
func pressKey(hwnd uintptr, vk byte) bool {
	if !foregroundIs(hwnd) {
		return false
	}
	procKeybdEvent.Call(uintptr(vk), 0, 0, 0)
	procKeybdEvent.Call(uintptr(vk), 0, 2 /*KEYEVENTF_KEYUP*/, 0)
	return true
}
