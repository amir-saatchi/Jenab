// SPIKE-026: how many build and test commands can run at once on a normal
// machine before they hurt each other or the app (SPEC 8.5, run_command).
//
// The harness starts K copies of a workload at once. Each copy runs under a
// wrapper (this binary in "wrap" mode) that puts itself in two Job Objects
// before starting the command: the level job, shared by all K copies, and
// its own job. Children inherit both, so the jobs see the whole process tree:
// peak committed memory and CPU time per command and for all K together.
//
// While a level runs, the harness measures how late a 5 ms timer fires on a
// normal-priority thread (a stand-in for the app's UI thread) and the lowest
// free physical memory.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "wrap" {
		os.Exit(wrap(os.Args[2:]))
	}
	harness()
}

// ---- workloads ----

type workload struct {
	name string
	dir  string
	cmd  func(slot int) []string
	env  func(slot int) []string
}

var repo, scratch string

func workloads() map[string]workload {
	mock := filepath.Join(repo, "mockups")
	goEnv := func(slot int) []string {
		return []string{
			"GOCACHE=" + filepath.Join(scratch, "gocache", strconv.Itoa(slot)),
			"GOPROXY=off", "GOTOOLCHAIN=local", "GOFLAGS=-mod=readonly",
		}
	}
	return map[string]workload{
		// A cold build of the app: -a rebuilds every package (std, Wails and
		// ours) and links the binary, like a first build in a fresh checkout.
		"go-cold": {"go-cold", repo, func(slot int) []string {
			out := filepath.Join(scratch, "out", "go", strconv.Itoa(slot), "desktop.exe")
			return []string{"go", "build", "-a", "-o", out, "./cmd/desktop"}
		}, goEnv},
		// The tests an agent runs after a change: warm cache, -count=1.
		"go-test": {"go-test", repo, func(int) []string {
			return []string{"go", "test", "-count=1", "./internal/..."}
		}, goEnv},
		// The mockups' production build: 84 screens, Tailwind, React.
		"vite": {"vite", mock, func(slot int) []string {
			out := filepath.Join(scratch, "out", "vite", strconv.Itoa(slot))
			return []string{filepath.Join(mock, "node_modules", ".bin", "vite.exe"), "build", "--outDir", out, "--emptyOutDir", "--logLevel", "error"}
		}, nil},
		// A full type check of the mockups, not incremental.
		"tsc": {"tsc", mock, func(int) []string {
			return []string{filepath.Join(mock, "node_modules", ".bin", "tsc.exe"), "-p", "tsconfig.app.json", "--incremental", "false"}
		}, nil},
	}
}

// ---- harness ----

type result struct {
	Workload string  `json:"workload"`
	Slot     int     `json:"slot"`
	Seconds  float64 `json:"seconds"`
	CPU      float64 `json:"cpu_seconds"`
	PeakMB   float64 `json:"peak_mb"`
	Procs    uint32  `json:"processes"`
	Exit     int     `json:"exit"`
}

type level struct {
	Name      string   `json:"name"`
	Priority  string   `json:"priority"`
	Runs      []result `json:"runs"`
	WallSec   float64  `json:"wall_seconds"`
	CPUSec    float64  `json:"cpu_seconds"`
	PeakMB    float64  `json:"peak_mb_all"`
	MinFree   float64  `json:"min_free_mb"`
	MinCommit float64  `json:"min_commit_free_mb"`
	Aborted   bool     `json:"aborted"`
	LateP50   float64  `json:"timer_late_p50_ms"`
	LateP99   float64  `json:"timer_late_p99_ms"`
	LateMax   float64  `json:"timer_late_max_ms"`
}

func harness() {
	plan := flag.String("plan", "", "comma-separated levels: workload[+workload…]xK[@below]")
	flag.StringVar(&repo, "repo", "", "repository root")
	flag.StringVar(&scratch, "scratch", "", "folder for caches and outputs")
	out := flag.String("out", "", "JSON lines file to append results to")
	flag.Parse()
	if *plan == "" || repo == "" || scratch == "" {
		flag.Usage()
		os.Exit(2)
	}
	wl := workloads()
	f, err := os.OpenFile(*out, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	check(err)
	defer f.Close()

	for _, spec := range strings.Split(*plan, ",") {
		lv := runLevel(spec, wl)
		b, _ := json.Marshal(lv)
		fmt.Fprintln(f, string(b))
		fmt.Printf("%-24s %-6s wall %6.1fs  cpu %7.1fs  peak %6.0f MB  free≥%6.0f MB  commit≥%6.0f MB  late p50 %5.1f p99 %6.1f max %6.1f ms  aborted=%v\n",
			lv.Name, lv.Priority, lv.WallSec, lv.CPUSec, lv.PeakMB, lv.MinFree, lv.MinCommit, lv.LateP50, lv.LateP99, lv.LateMax, lv.Aborted)
		for _, r := range lv.Runs {
			fmt.Printf("    %-8s #%d  %6.1fs  cpu %6.1fs  peak %6.0f MB  procs %3d  exit %d\n", r.Workload, r.Slot, r.Seconds, r.CPU, r.PeakMB, r.Procs, r.Exit)
		}
		time.Sleep(10 * time.Second) // let the machine settle
	}
}

// runLevel runs one plan entry, e.g. "go-coldx4", "go-cold+vite+tscx1" or
// "go-coldx4@below".
func runLevel(spec string, wl map[string]workload) level {
	prio := "normal"
	if s, ok := strings.CutSuffix(spec, "@below"); ok {
		spec, prio = s, "below"
	}
	i := strings.LastIndex(spec, "x")
	k, err := strconv.Atoi(spec[i+1:])
	check(err)
	var jobs []workload
	for _, name := range strings.Split(spec[:i], "+") {
		w, ok := wl[name]
		if !ok {
			panic("unknown workload " + name)
		}
		for range k {
			jobs = append(jobs, w)
		}
	}

	job := newJob(true)
	defer windows.CloseHandle(job)

	stop := make(chan struct{})
	lateCh := make(chan []float64)
	go func() { lateCh <- probe(stop) }()
	lowCh := make(chan lows)
	go func() { lowCh <- sampleFree(stop, job) }()

	start := time.Now()
	results := make([]result, len(jobs))
	var wg sync.WaitGroup
	for n, w := range jobs {
		wg.Go(func() { results[n] = launch(job, w, n, prio) })
	}
	wg.Wait()
	wall := time.Since(start)
	close(stop)
	late := <-lateCh
	slices.Sort(late)

	acct := accounting(job)
	low := <-lowCh
	return level{
		Name: spec, Priority: prio, Runs: results,
		WallSec: wall.Seconds(),
		CPUSec:  float64(acct.TotalUserTime+acct.TotalKernelTime) / 1e7,
		PeakMB:  float64(peak(job)) / (1 << 20),
		MinFree: low.free, MinCommit: low.commit, Aborted: low.aborted,
		LateP50: pct(late, 0.50), LateP99: pct(late, 0.99), LateMax: pct(late, 1),
	}
}

func launch(job windows.Handle, w workload, slot int, prio string) result {
	self, _ := os.Executable()
	args := append([]string{"wrap", strconv.FormatUint(uint64(job), 10), prio, w.name, strconv.Itoa(slot), w.dir}, w.cmd(slot)...)
	cmd := exec.Command(self, args...)
	cmd.Env = os.Environ()
	if w.env != nil {
		cmd.Env = append(cmd.Env, w.env(slot)...)
	}
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{AdditionalInheritedHandles: []syscall.Handle{syscall.Handle(job)}}
	stdout, err := cmd.StdoutPipe()
	check(err)
	check(cmd.Start())
	var r result
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		if err := json.Unmarshal(sc.Bytes(), &r); err == nil {
			break
		}
	}
	cmd.Wait()
	return r
}

// probe measures how late a 5 ms sleep wakes up, every 5 ms, until stop.
func probe(stop chan struct{}) []float64 {
	var late []float64
	for {
		select {
		case <-stop:
			return late
		default:
		}
		t := time.Now()
		time.Sleep(5 * time.Millisecond)
		late = append(late, float64(time.Since(t)-5*time.Millisecond)/1e6)
	}
}

// guardMB is the commit headroom kept for the user's other apps: below it,
// the level's whole process tree is killed.
const guardMB = 1536

type lows struct {
	free, commit float64
	aborted      bool
}

func sampleFree(stop chan struct{}, job windows.Handle) lows {
	var l lows
	l.free, l.commit = mem()
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return l
		case <-t.C:
			f, c := mem()
			l.free, l.commit = min(l.free, f), min(l.commit, c)
			if c < guardMB && !l.aborted {
				l.aborted = true
				windows.TerminateJobObject(job, 99)
				fmt.Fprintf(os.Stderr, "commit headroom %.0f MB: level killed\n", c)
			}
		}
	}
}

func pct(s []float64, p float64) float64 {
	if len(s) == 0 {
		return 0
	}
	return s[min(len(s)-1, int(p*float64(len(s))))]
}

// ---- wrapper ----

// wrap: job prio workload slot dir cmd args…
func wrap(a []string) int {
	parent, _ := strconv.ParseUint(a[0], 10, 64)
	prio, name, dir := a[1], a[2], a[4]
	slot, _ := strconv.Atoi(a[3])
	me := windows.CurrentProcess()
	check(windows.AssignProcessToJobObject(windows.Handle(parent), me))
	own := newJob(false)
	check(windows.AssignProcessToJobObject(own, me))
	if prio == "below" {
		// Children of a below-normal process inherit its class.
		check(windows.SetPriorityClass(me, windows.BELOW_NORMAL_PRIORITY_CLASS))
	}

	logPath := filepath.Join(os.TempDir(), fmt.Sprintf("spike026-%s-%d.log", name, slot))
	logf, _ := os.Create(logPath)
	cmd := exec.Command(a[5], a[6:]...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = logf, logf
	start := time.Now()
	err := cmd.Run()
	took := time.Since(start)
	logf.Close()
	exit := 0
	if err != nil {
		exit = 1
		if ee, ok := err.(*exec.ExitError); ok {
			exit = ee.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "%s #%d failed (%v), see %s\n", name, slot, err, logPath)
	}
	acct := accounting(own)
	b, _ := json.Marshal(result{
		Workload: name, Slot: slot, Seconds: took.Seconds(),
		CPU:    float64(acct.TotalUserTime+acct.TotalKernelTime) / 1e7,
		PeakMB: float64(peak(own)) / (1 << 20), Procs: acct.TotalProcesses, Exit: exit,
	})
	fmt.Println(string(b))
	return 0
}

// ---- Windows helpers ----

func newJob(inheritable bool) windows.Handle {
	var sa *windows.SecurityAttributes
	if inheritable {
		sa = &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), InheritHandle: 1}
	}
	job, err := windows.CreateJobObject(sa, nil)
	check(err)
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	check(err)
	return job
}

func peak(job windows.Handle) uintptr {
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	check(windows.QueryInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil))
	return info.PeakJobMemoryUsed
}

type basicAccounting struct {
	TotalUserTime, TotalKernelTime                       int64
	ThisPeriodTotalUserTime, ThisPeriodTotalKernelTime   int64
	TotalPageFaultCount, TotalProcesses, ActiveProcesses uint32
	TotalTerminatedProcesses                             uint32
}

func accounting(job windows.Handle) basicAccounting {
	var a basicAccounting
	const jobObjectBasicAccountingInformation = 1
	check(windows.QueryInformationJobObject(job, jobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&a)), uint32(unsafe.Sizeof(a)), nil))
	return a
}

var globalMemoryStatusEx = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

type memoryStatusEx struct {
	Length, MemoryLoad                       uint32
	TotalPhys, AvailPhys                     uint64
	TotalPageFile, AvailPageFile             uint64
	TotalVirtual, AvailVirtual, AvailExtVirt uint64
}

// mem returns the free physical memory and the commit headroom, in MB.
func mem() (free, commit float64) {
	m := memoryStatusEx{Length: uint32(unsafe.Sizeof(memoryStatusEx{}))}
	globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m)))
	return float64(m.AvailPhys) / (1 << 20), float64(m.AvailPageFile) / (1 << 20)
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
