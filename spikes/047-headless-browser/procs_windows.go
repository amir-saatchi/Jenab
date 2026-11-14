package main

// The processes agent-browser starts (the CLI, its daemon, the browser and
// its children), their memory and CPU, and their TCP and UDP sockets,
// sampled every 100 ms.

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type proc struct {
	PID, PPID uint32
	Name      string
}

func processes() map[uint32]proc {
	out := map[uint32]proc{}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return out
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		out[e.ProcessID] = proc{PID: e.ProcessID, PPID: e.ParentProcessID, Name: windows.UTF16ToString(e.ExeFile[:])}
	}
	return out
}

// tree is every agent-browser process and its descendants.
func tree(ps map[uint32]proc, root string) map[uint32]proc {
	in := map[uint32]proc{}
	for pid, p := range ps {
		if strings.EqualFold(p.Name, root) {
			in[pid] = p
		}
	}
	for changed := true; changed; {
		changed = false
		for pid, p := range ps {
			if _, ok := in[pid]; ok {
				continue
			}
			if _, ok := in[p.PPID]; ok && p.PPID != 0 && bornAfter(pid, p.PPID) {
				in[pid] = p
				changed = true
			}
		}
	}
	return in
}

// bornAfter guards against stale parent IDs: Windows reuses process IDs,
// so a process whose parent died long ago can name a new process as its
// parent. A real child starts after its parent.
func bornAfter(child, parent uint32) bool {
	c, ok1 := created(child)
	p, ok2 := created(parent)
	return ok1 && ok2 && c >= p
}

func created(pid uint32) (int64, bool) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return 0, false
	}
	defer windows.CloseHandle(h)
	var c, x, k, u windows.Filetime
	if windows.GetProcessTimes(h, &c, &x, &k, &u) != nil {
		return 0, false
	}
	return c.Nanoseconds(), true
}

var (
	kernel32     = windows.NewLazySystemDLL("kernel32.dll")
	procMemInfo  = kernel32.NewProc("K32GetProcessMemoryInfo")
	iphlpapi     = windows.NewLazySystemDLL("iphlpapi.dll")
	procTCPTable = iphlpapi.NewProc("GetExtendedTcpTable")
	procUDPTable = iphlpapi.NewProc("GetExtendedUdpTable")
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

// usage is a process's working set, private bytes and CPU time.
func usage(pid uint32) (ws, priv uint64, cpu time.Duration, ok bool) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return 0, 0, 0, false
	}
	defer windows.CloseHandle(h)
	var m memCounters
	m.CB = uint32(unsafe.Sizeof(m))
	if r, _, _ := procMemInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&m)), uintptr(m.CB)); r == 0 {
		return 0, 0, 0, false
	}
	var c, x, k, u windows.Filetime
	if err := windows.GetProcessTimes(h, &c, &x, &k, &u); err != nil {
		return 0, 0, 0, false
	}
	t := func(f windows.Filetime) time.Duration {
		return time.Duration(uint64(f.HighDateTime)<<32|uint64(f.LowDateTime)) * 100
	}
	return uint64(m.WorkingSetSize), uint64(m.PrivateUsage), t(k) + t(u), true
}

// Sock is one socket of a process in the tree.
type Sock struct {
	Proto  string // tcp, udp
	Local  netip.AddrPort
	Remote netip.AddrPort // tcp only
	State  uint32
	PID    uint32
	Name   string
}

func (s Sock) String() string {
	if s.Proto == "udp" {
		return fmt.Sprintf("udp %s (%s)", s.Local, s.Name)
	}
	return fmt.Sprintf("tcp %s -> %s state %d (%s)", s.Local, s.Remote, s.State, s.Name)
}

const tcpListen = 2

func port(v uint32) uint16 { return binary.BigEndian.Uint16([]byte{byte(v), byte(v >> 8)}) }

func sockets() []Sock {
	var out []Sock
	for _, af := range []uint32{windows.AF_INET, windows.AF_INET6} {
		b := ipTable(procTCPTable, af, 5) // TCP_TABLE_OWNER_PID_ALL
		if len(b) >= 4 {
			n := binary.LittleEndian.Uint32(b)
			for i := range n {
				if af == windows.AF_INET {
					r := b[4+i*24:]
					u := func(o int) uint32 { return binary.LittleEndian.Uint32(r[o:]) }
					out = append(out, Sock{Proto: "tcp", State: u(0),
						Local:  netip.AddrPortFrom(netip.AddrFrom4([4]byte(r[4:8])), port(u(8))),
						Remote: netip.AddrPortFrom(netip.AddrFrom4([4]byte(r[12:16])), port(u(16))), PID: u(20)})
				} else {
					r := b[4+i*56:]
					u := func(o int) uint32 { return binary.LittleEndian.Uint32(r[o:]) }
					out = append(out, Sock{Proto: "tcp", State: u(48),
						Local:  netip.AddrPortFrom(netip.AddrFrom16([16]byte(r[0:16])).Unmap(), port(u(20))),
						Remote: netip.AddrPortFrom(netip.AddrFrom16([16]byte(r[24:40])).Unmap(), port(u(44))), PID: u(52)})
				}
			}
		}
		b = ipTable(procUDPTable, af, 1) // UDP_TABLE_OWNER_PID
		if len(b) >= 4 {
			n := binary.LittleEndian.Uint32(b)
			for i := range n {
				if af == windows.AF_INET {
					r := b[4+i*12:]
					u := func(o int) uint32 { return binary.LittleEndian.Uint32(r[o:]) }
					out = append(out, Sock{Proto: "udp", Local: netip.AddrPortFrom(netip.AddrFrom4([4]byte(r[0:4])), port(u(4))), PID: u(8)})
				} else {
					r := b[4+i*28:]
					u := func(o int) uint32 { return binary.LittleEndian.Uint32(r[o:]) }
					out = append(out, Sock{Proto: "udp", Local: netip.AddrPortFrom(netip.AddrFrom16([16]byte(r[0:16])).Unmap(), port(u(20))), PID: u(24)})
				}
			}
		}
	}
	return out
}

func ipTable(p *windows.LazyProc, af, class uint32) []byte {
	size := uint32(64 << 10)
	for range 4 {
		b := make([]byte, size)
		r, _, _ := p.Call(uintptr(unsafe.Pointer(&b[0])), uintptr(unsafe.Pointer(&size)), 0, uintptr(af), uintptr(class), 0)
		if r == 0 {
			return b
		}
		if r != uintptr(windows.ERROR_INSUFFICIENT_BUFFER) {
			return nil
		}
	}
	return nil
}

// Sampler watches the tree.
type Sampler struct {
	Root  string // the program's file name
	Proxy netip.AddrPort

	mu       sync.Mutex
	stop     chan struct{}
	done     chan struct{}
	PeakWS   uint64
	PeakPriv uint64
	PeakCPU  float64 // percent of one core, over a 500 ms window
	CPUTotal time.Duration
	MaxProcs int
	Names    map[string]int // image name -> max count at once
	Seen     map[string]Sock
	cpuStart map[uint32]time.Duration
	cpuLast  map[uint32]time.Duration
}

func NewSampler(root string, proxy netip.AddrPort) *Sampler {
	return &Sampler{Root: root, Proxy: proxy}
}

func (s *Sampler) Start() {
	s.stop, s.done = make(chan struct{}), make(chan struct{})
	s.Names, s.Seen = map[string]int{}, map[string]Sock{}
	s.cpuStart, s.cpuLast = map[uint32]time.Duration{}, map[uint32]time.Duration{}
	s.PeakWS, s.PeakPriv, s.PeakCPU, s.CPUTotal, s.MaxProcs = 0, 0, 0, 0, 0
	for pid := range tree(processes(), s.Root) {
		if _, _, c, ok := usage(pid); ok {
			s.cpuStart[pid] = c // already running: count from now
		}
	}
	go s.loop()
}

func (s *Sampler) Stop() {
	close(s.stop)
	<-s.done
	for pid, last := range s.cpuLast {
		s.CPUTotal += last - s.cpuStart[pid]
	}
}

func (s *Sampler) loop() {
	defer close(s.done)
	var hist []cpuPt
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		s.sample(&hist)
		select {
		case <-s.stop:
			s.sample(&hist)
			return
		case <-tick.C:
		}
	}
}

type cpuPt struct {
	at  time.Time
	cpu time.Duration
}

func (s *Sampler) sample(hist *[]cpuPt) {
	t := tree(processes(), s.Root)
	var ws, priv uint64
	var cpu time.Duration
	names := map[string]int{}
	for pid, p := range t {
		w, pr, c, ok := usage(pid)
		if !ok {
			continue
		}
		ws += w
		priv += pr
		if _, seen := s.cpuStart[pid]; !seen {
			// a process started during the sample counts from zero
			s.cpuStart[pid] = 0
		}
		s.cpuLast[pid] = c
		names[p.Name]++
	}
	for pid, c := range s.cpuLast {
		cpu += c - s.cpuStart[pid]
	}
	now := time.Now()
	*hist = append(*hist, cpuPt{now, cpu})
	for len(*hist) > 1 && now.Sub((*hist)[0].at) > 500*time.Millisecond {
		*hist = (*hist)[1:]
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if h := *hist; len(h) > 1 {
		if dt := h[len(h)-1].at.Sub(h[0].at); dt > 0 {
			if pct := 100 * float64(h[len(h)-1].cpu-h[0].cpu) / float64(dt); pct > s.PeakCPU {
				s.PeakCPU = pct
			}
		}
	}
	s.PeakWS = max(s.PeakWS, ws)
	s.PeakPriv = max(s.PeakPriv, priv)
	s.MaxProcs = max(s.MaxProcs, len(t))
	for n, c := range names {
		s.Names[n] = max(s.Names[n], c)
	}
	if len(t) == 0 {
		return
	}
	all := sockets()
	listening := map[uint16]bool{}
	for _, k := range all {
		if _, in := t[k.PID]; in && k.Proto == "tcp" && k.State == tcpListen {
			listening[k.Local.Port()] = true
		}
	}
	for _, k := range all {
		p, in := t[k.PID]
		if !in {
			continue
		}
		k.Name = p.Name
		key := k.Proto + " " + k.Local.String() + " " + k.Remote.String() + " " + p.Name
		if k.Proto == "tcp" {
			switch {
			case k.State == tcpListen:
				key = "listen " + k.Local.String() + " " + p.Name
			case k.Remote == s.Proxy:
				key = "to-proxy " + p.Name
			case k.Remote.Addr().IsLoopback() && listening[k.Remote.Port()]:
				key = "internal " + p.Name
			case k.Remote.Addr().IsLoopback() && listening[k.Local.Port()]:
				key = "internal " + p.Name
			}
		}
		if _, ok := s.Seen[key]; !ok {
			s.Seen[key] = k
		}
	}
}

// Outside lists the sockets that are neither the proxy, internal links
// nor listening sockets.
func (s *Sampler) Outside() []string {
	var out []string
	for key, k := range s.Seen {
		if strings.HasPrefix(key, "to-proxy") || strings.HasPrefix(key, "internal") || strings.HasPrefix(key, "listen") {
			continue
		}
		out = append(out, k.String())
	}
	sort.Strings(out)
	return out
}

func (s *Sampler) Listening() []string {
	var out []string
	for key := range s.Seen {
		if strings.HasPrefix(key, "listen") {
			out = append(out, strings.TrimPrefix(key, "listen "))
		}
	}
	sort.Strings(out)
	return out
}

// Left lists the processes of the tree that are still running.
func Left(root string) []string {
	var out []string
	for _, p := range tree(processes(), root) {
		out = append(out, fmt.Sprintf("%s(%d)", p.Name, p.PID))
	}
	sort.Strings(out)
	return out
}
