package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"

	"burrow/spikes/selfupdate/internal/spike"
)

type Scn struct {
	Name, Group string
	Install     string // file in bin/ copied to the install dir as the running version ("" = keep what is installed)
	Cfg         spike.AppConfig
	Feed        func()
	KeepData    string // reuse this scenario's data dir
	OnLog       func(r *Run, e spike.Entry)
	AfterExit   func(r *Run)
	MaxSettle   time.Duration
}

type proc struct {
	pid   int
	h     windows.Handle
	role  string
	ver   string
	start time.Time
}

type Run struct {
	S           *Scn
	LogPath     string
	Start       time.Time
	OldExit     time.Time
	OldPID      int
	OldExitCode int
	mu          sync.Mutex
	Entries     []spike.Entry
	procs       map[int]*proc
	Facts       map[string]any
	InstallDir  []string
	TempNew     []string
	HelperLogs  map[string]string
	ExitCodes   map[string]uint32
	Killed      []string
	lastEntry   time.Time
}

func (r *Run) fact(k string, v any) {
	r.mu.Lock()
	r.Facts[k] = v
	r.mu.Unlock()
}

func (r *Run) find(ev string, pred func(spike.Entry) bool) spike.Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.Entries {
		if e.S("ev") == ev && (pred == nil || pred(e)) {
			return e
		}
	}
	return nil
}

func (r *Run) all(ev string) []spike.Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []spike.Entry
	for _, e := range r.Entries {
		if e.S("ev") == ev {
			out = append(out, e)
		}
	}
	return out
}

func (r *Run) helper() *proc {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.procs {
		if p.role == "helper" {
			return p
		}
	}
	return nil
}

var allRuns []*Run
var known = map[int]*proc{} // every process of ours we ever saw, for final cleanup
var knownMu sync.Mutex

func selected(name string) bool { return onlyRe == nil || onlyRe.MatchString(name) }

func dataDir(name string) string { return filepath.Join(outDir, "data", name) }

func install(bin string) {
	// Plain per-user folder copy: no installer, no registry, no shortcuts.
	_ = os.RemoveAll(installDir)
	must(os.MkdirAll(installDir, 0o755))
	copyFile(filepath.Join(binDir, bin), exePath)
}

func copyFile(src, dst string) {
	in, err := os.Open(src)
	must(err)
	defer in.Close()
	out, err := os.Create(dst)
	must(err)
	_, err = io.Copy(out, in)
	must(err)
	must(out.Close())
}

func runScn(s *Scn) *Run {
	r := &Run{S: s, procs: map[int]*proc{}, Facts: map[string]any{}, HelperLogs: map[string]string{}, ExitCodes: map[string]uint32{}}
	allRuns = append(allRuns, r)
	logf("=== %s", s.Name)
	fd.Reset()
	fd.SetScenario(s.Name)
	if s.Feed != nil {
		s.Feed()
	}
	if s.Install != "" {
		install(s.Install)
	}
	c := s.Cfg
	c.Scenario = s.Name
	r.LogPath = filepath.Join(outDir, "logs", s.Name+".jsonl")
	c.Log = r.LogPath
	if s.KeepData != "" {
		c.DataDir = dataDir(s.KeepData)
	} else {
		c.DataDir = dataDir(s.Name)
		_ = os.RemoveAll(c.DataDir)
	}
	cfgPath := filepath.Join(outDir, "cfg", s.Name+".json")
	must(c.Save(cfgPath))
	before := tempEntries()

	cmd := exec.Command(exePath)
	cmd.Dir = installDir
	cmd.Env = append(os.Environ(), spike.EnvConfig+"="+cfgPath)
	r.Start = time.Now()
	if err := cmd.Start(); err != nil {
		r.fact("startError", err.Error())
		logf("start failed: %v", err)
		return r
	}
	r.OldPID = cmd.Process.Pid
	r.track(r.OldPID, "old", "")
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		r.OldExit = time.Now()
		r.OldExitCode = cmd.ProcessState.ExitCode()
		close(exited)
	}()
	stopTail := make(chan struct{})
	tailDone := make(chan struct{})
	go r.tail(stopTail, tailDone)

	select {
	case <-exited:
	case <-time.After(120 * time.Second):
		r.fact("oldTimeout", true)
		_ = cmd.Process.Kill()
		<-exited
	}
	if s.AfterExit != nil {
		s.AfterExit(r)
	}
	// Settle: every process we saw has exited and the log has been quiet.
	maxSettle := s.MaxSettle
	if maxSettle == 0 {
		maxSettle = 60 * time.Second
	}
	// The quiet condition must hold for 2.5 s in a row: between the helper's
	// exit and the relaunched app's first log line nobody may look alive.
	deadline := time.Now().Add(maxSettle)
	var calmSince time.Time
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		r.mu.Lock()
		quiet := time.Since(r.lastEntry) > 1500*time.Millisecond
		r.mu.Unlock()
		if quiet && !r.anyAlive() && time.Since(r.OldExit) > time.Second {
			if calmSince.IsZero() {
				calmSince = time.Now()
			}
			if time.Since(calmSince) > 2500*time.Millisecond {
				break
			}
		} else {
			calmSince = time.Time{}
		}
	}
	close(stopTail)
	<-tailDone
	r.killAlive("settle timeout")
	r.collect(before)
	logf("    %d log lines, old exit code %d, procs %s", len(r.Entries), r.OldExitCode, r.procSummary())
	return r
}

func (r *Run) track(pid int, role, ver string) *proc {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.procs[pid]; ok {
		if p.role == "old" && role != "old" {
			return p
		}
		if role != "" {
			p.role = role
		}
		if ver != "" {
			p.ver = ver
		}
		return p
	}
	h, _ := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, uint32(pid))
	p := &proc{pid: pid, h: h, role: role, ver: ver, start: time.Now()}
	r.procs[pid] = p
	knownMu.Lock()
	known[pid] = p
	knownMu.Unlock()
	return p
}

func alive(p *proc) bool {
	if p.h == 0 {
		return false
	}
	var code uint32
	if windows.GetExitCodeProcess(p.h, &code) != nil {
		return false
	}
	return code == 259
}

func (r *Run) anyAlive() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.procs {
		if alive(p) {
			return true
		}
	}
	return false
}

func kill(p *proc) bool {
	if p == nil || p.h == 0 || !alive(p) {
		return false
	}
	return windows.TerminateProcess(p.h, 99) == nil
}

func (r *Run) killAlive(why string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.procs {
		if kill(p) {
			r.Killed = append(r.Killed, fmt.Sprintf("%s pid %d (%s)", p.role, p.pid, why))
		}
	}
}

func killAllKnown(why string) {
	knownMu.Lock()
	defer knownMu.Unlock()
	n := 0
	for _, p := range known {
		if kill(p) {
			n++
			logf("killed leftover %s pid %d (%s)", p.role, p.pid, why)
		}
	}
	logf("leftover processes of ours at %s: %d", why, n)
}

func (r *Run) procSummary() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var parts []string
	for _, p := range r.procs {
		var code uint32
		if p.h != 0 {
			_ = windows.GetExitCodeProcess(p.h, &code)
		}
		parts = append(parts, fmt.Sprintf("%s/%s:%d", p.role, p.ver, code))
		r.ExitCodes[fmt.Sprintf("%s-%d", p.role, p.pid)] = code
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

func (r *Run) tail(stop, done chan struct{}) {
	defer close(done)
	var off int64
	var buf []byte
	for {
		select {
		case <-stop:
			r.readNew(&off, &buf)
			return
		default:
		}
		r.readNew(&off, &buf)
		time.Sleep(10 * time.Millisecond)
	}
}

func (r *Run) readNew(off *int64, buf *[]byte) {
	f, err := os.Open(r.LogPath)
	if err != nil {
		return
	}
	defer f.Close()
	if _, err := f.Seek(*off, io.SeekStart); err != nil {
		return
	}
	b, _ := io.ReadAll(f)
	*off += int64(len(b))
	*buf = append(*buf, b...)
	for {
		i := strings.IndexByte(string(*buf), '\n')
		if i < 0 {
			return
		}
		line := (*buf)[:i]
		*buf = (*buf)[i+1:]
		es := spike.ReadLogBytes(line)
		for _, e := range es {
			r.onEntry(e)
		}
	}
}

func (r *Run) onEntry(e spike.Entry) {
	pid := int(e.F("pid"))
	if e.S("ev") == "main-entered" && pid != 0 {
		role := "app"
		if e.S("helperEnv") == "1" {
			role = "helper"
		} else if args, ok := e["args"].([]any); ok && len(args) > 0 && args[0] == "--child" {
			role = "child"
		}
		if pid == r.OldPID {
			role = "old"
		}
		r.track(pid, role, e.S("ver"))
	}
	r.mu.Lock()
	r.Entries = append(r.Entries, e)
	r.lastEntry = time.Now()
	r.mu.Unlock()
	if r.S.OnLog != nil {
		r.S.OnLog(r, e)
	}
}

func (r *Run) collect(before map[string]bool) {
	es, _ := os.ReadDir(installDir)
	for _, e := range es {
		fi, _ := e.Info()
		var sz int64
		if fi != nil {
			sz = fi.Size()
		}
		desc := fmt.Sprintf("%s (%s", e.Name(), human(sz))
		if !e.IsDir() {
			desc += ", " + whichBuild(filepath.Join(installDir, e.Name()))
		}
		r.InstallDir = append(r.InstallDir, desc+")")
	}
	for name := range tempEntries() {
		if !before[name] {
			full := filepath.Join(os.TempDir(), name)
			if strings.HasSuffix(name, ".log") {
				b, _ := os.ReadFile(full)
				r.HelperLogs[name] = string(b)
				continue
			}
			var inside []string
			sub, _ := os.ReadDir(full)
			for _, x := range sub {
				fi, _ := x.Info()
				var sz int64
				if fi != nil {
					sz = fi.Size()
				}
				inside = append(inside, fmt.Sprintf("%s %s", x.Name(), human(sz)))
			}
			r.TempNew = append(r.TempNew, name+"/["+strings.Join(inside, ", ")+"]")
		}
	}
	sort.Strings(r.TempNew)
}

var buildHashes map[string]string

// whichBuild names the bin/ build a file is identical to.
func whichBuild(p string) string {
	if buildHashes == nil {
		buildHashes = map[string]string{}
		es, _ := os.ReadDir(binDir)
		for _, e := range es {
			buildHashes[fileHash(filepath.Join(binDir, e.Name()))] = e.Name()
		}
	}
	h := fileHash(p)
	if n, ok := buildHashes[h]; ok {
		return "= " + n
	}
	if h == "" {
		return "unreadable"
	}
	return "sha " + h[:12]
}
