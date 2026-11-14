package main

// Part A.1: running it.

import (
	"context"
	"flag"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func partRun(ctx context.Context) {
	e, err := newEnv()
	if err != nil {
		p("setup: %v", err)
		return
	}
	p("## 1. Running it\n")

	// --- the pinned program ---
	p("### The pinned program\n")
	p("- SHA-256 of the file used: `%s` (pinned: `%s`)", fileHash(*flagAB), *flagHash)
	bad := filepath.Join(*flagHome, "tampered.exe")
	if b, err := os.ReadFile(*flagAB); err == nil {
		os.WriteFile(bad, append(b, 0), 0o700)
		a := e.ab(browser{"cft", *flagCfT}, "tamper")
		a.Path = bad
		a.Setup()
		r := a.Run(ctx, 10*time.Second, "--version")
		p("- A copy with one byte added: %s", clip(r.Error, 160))
		os.Remove(bad)
	}

	// --- without Node ---
	_, nodeErr := exec.LookPath("node")
	cmd := exec.Command(*flagAB, "--version")
	cmd.Env = []string{"PATH=" + os.Getenv("SystemRoot") + `\System32`, "SystemRoot=" + os.Getenv("SystemRoot")}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	v, err := cmd.Output()
	p("- Node on PATH here: %v. `--version` with PATH set to System32 only: %q (err %v)", nodeErr == nil, strings.TrimSpace(string(v)), err)

	// --- browsers ---
	p("\n### Browsers\n")
	p("| Browser | Program | Size on disk | Starts under agent-browser | Cold open + get html | Error |")
	p("|---|---|---|---|---|---|")
	for _, b := range []browser{{"cft", *flagCfT}, {"shell", *flagShell}, {"chrome", *flagChrome}, {"edge", *flagEdge}} {
		if b.Path == "" {
			continue
		}
		a := e.ab(b, "run")
		a.Setup()
		t0 := time.Now()
		r := a.Run(ctx, 60*time.Second, "open", e.S.Base()+"/js/spa-inline")
		ok := r.Success
		var g Result
		if ok {
			g = a.Run(ctx, 20*time.Second, "get", "html", "html")
			ok = g.Success && strings.Contains(g.field("html"), "Avonmouth")
		}
		el := time.Since(t0)
		a.Close(ctx)
		waitGone(10 * time.Second)
		dir := filepath.Dir(b.Path)
		if b.ID == "edge" {
			dir = filepath.Join(dir, "157.0.4341.46")
		}
		if b.ID == "chrome" {
			dir = filepath.Join(dir, "157.0.8103.51")
		}
		p("| %s | %s | %d MB | %v | %s | %s |", b.ID, filepath.Base(b.Path), dirSize(dir)>>20, r.Success, el.Round(10*time.Millisecond), clip(strings.ReplaceAll(r.Error+g.Error, "\n", " "), 140))
	}

	// --- state folder ---
	a := e.ab(browser{"cft", *flagCfT}, "files")
	a.Setup()
	a.Run(ctx, 60*time.Second, "open", e.S.Base()+"/js/spa-fetch")
	pid := a.DaemonPID()
	var files []string
	filepath.Walk(a.Home, func(path string, fi os.FileInfo, err error) error {
		rel, _ := filepath.Rel(a.Home, path)
		if err == nil && strings.Count(rel, string(filepath.Separator)) < 2 && rel != "." {
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	p("\n### The state folder while a session runs\n")
	p("Daemon pid file: %d. Entries (two levels): %s", pid, strings.Join(files, ", "))
	for _, f := range []string{"sock/default.port", "sock/default.stream"} {
		b, _ := os.ReadFile(filepath.Join(a.Home, f))
		p("- %s: %s", f, strings.TrimSpace(string(b)))
	}
	s := NewSampler(root, netip.AddrPort{})
	s.Start()
	time.Sleep(300 * time.Millisecond)
	s.Stop()
	p("- Listening sockets of the tree: %s", strings.Join(s.Listening(), "; "))
	p("- Processes: %v", s.Names)
	r := a.Close(ctx)
	d, left := waitGone(10 * time.Second)
	p("\n### Clean-up\n")
	p("- `close`: success %v; tree gone after %s; left: %v", r.Success, d.Round(100*time.Millisecond), left)
	_, err = os.Stat(filepath.Join(a.Home, "tmp"))
	var tmpLeft []string
	es, _ := os.ReadDir(filepath.Join(a.Home, "tmp"))
	for _, e := range es {
		tmpLeft = append(tmpLeft, e.Name())
	}
	p("- Temporary profiles left in the state folder's tmp after `close`: %v", tmpLeft)

	// killed call
	a = e.ab(browser{"cft", *flagCfT}, "killed")
	a.Idle = "15s"
	a.Setup()
	a.Run(ctx, 60*time.Second, "open", e.S.Base()+"/")
	r = a.Run(ctx, 1500*time.Millisecond, "open", e.S.Base()+"/js/spa-delayed")
	r2 := a.Run(ctx, 2*time.Millisecond, "get", "title")
	time.Sleep(time.Second)
	p("- A call killed after 1.5 s (killed=%v) and one after 2 ms (killed=%v): left right after: %v", r.Killed, r2.Killed, Left(root))
	r = a.Run(ctx, 20*time.Second, "get", "title")
	p("- The next call on that session: success %v, title %q", r.Success, r.field("title"))
	t0 := time.Now()
	d, left = waitGone(40 * time.Second)
	p("- With no `close`, the idle timeout (15 s) ended the tree after %s; left: %v", time.Since(t0).Round(time.Second), left)
	_ = d

	// Jenab stopped
	for _, job := range []bool{false, true} {
		a = e.ab(browser{"cft", *flagCfT}, "stopped")
		a.Idle = "20s"
		a.Setup()
		cmd := exec.Command(os.Args[0], "-part", "fakejenab", "-ab", *flagAB, "-hash", *flagHash, "-home", a.Home, "-cft", *flagCfT, "-url", e.S.Base()+"/", "-proxyaddr", e.PAddr, fmt.Sprintf("-job=%v", job))
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
		cmd.Stdout = os.Stderr
		cmd.Start()
		for i := 0; i < 100 && a.DaemonPID() == 0; i++ {
			time.Sleep(200 * time.Millisecond)
		}
		time.Sleep(3 * time.Second)
		before := len(Left(root))
		cmd.Process.Kill()
		cmd.Wait()
		time.Sleep(2 * time.Second)
		after := Left(root)
		d, left := waitGone(40 * time.Second)
		p("- Fake Jenab killed (job object with kill-on-close: %v): %d processes before, %d two seconds after; all gone after %s more; left: %v",
			job, before, len(after), d.Round(time.Second), left)
	}
}

var (
	flagURL       = flag.String("url", "", "fakejenab: the page to open")
	flagProxyAddr = flag.String("proxyaddr", "", "fakejenab: the proxy")
	flagJob       = flag.Bool("job", false, "fakejenab: use a kill-on-close job object")
)

// fakeJenab stands in for Jenab: optionally puts itself in a job object
// that kills its processes when its last handle closes, opens a page and
// waits to be killed.
func fakeJenab() {
	if *flagJob {
		j, err := windows.CreateJobObject(nil, nil)
		if err == nil {
			info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
			info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
			windows.SetInformationJobObject(j, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
			windows.AssignProcessToJobObject(j, windows.CurrentProcess())
		}
	}
	a := &AB{Path: *flagAB, Hash: *flagHash, Home: *flagHome, Browser: *flagCfT, Proxy: "http://" + *flagProxyAddr, Bypass: true, Idle: "20s"}
	a.Setup()
	r := a.Run(context.Background(), 60*time.Second, "open", *flagURL)
	fmt.Fprintln(os.Stderr, "fakejenab open:", r.Success, r.Error)
	time.Sleep(time.Hour)
}
