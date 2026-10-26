package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

// The SDK checks (ticket point 1): the same list on both SDKs, over stdio
// and Streamable HTTP against the fake server.

type check struct {
	SDK       string `json:"sdk"`
	Transport string `json:"transport"`
	Name      string `json:"check"`
	OK        bool   `json:"ok"`
	Detail    string `json:"detail"`
}

type checker struct {
	fake    string // the fake server's exe
	scratch string
	out     []check
}

func (c *checker) add(sdk, tr, name string, ok bool, format string, args ...any) {
	ch := check{SDK: sdk, Transport: tr, Name: name, OK: ok, Detail: fmt.Sprintf(format, args...)}
	c.out = append(c.out, ch)
	mark := "ok  "
	if !ok {
		mark = "FAIL"
	}
	fmt.Printf("%s %-7s %-6s %-22s %s\n", mark, sdk, tr, name, ch.Detail)
}

type connectFunc func(ctx context.Context, logPath string, child bool, j *job) (mcpClient, error)

func (c *checker) stdioConnect(sdk string) connectFunc {
	return func(ctx context.Context, logPath string, child bool, j *job) (mcpClient, error) {
		args := []string{"-log", logPath}
		if child {
			args = append(args, "-child")
		}
		if sdk == "go-sdk" {
			return goStdio(ctx, exec.Command(c.fake, args...), j)
		}
		return m3Stdio(ctx, c.fake, args, j)
	}
}

func readLog(path string) string {
	b, _ := os.ReadFile(path)
	return string(b)
}

func (c *checker) logPath(name string) string {
	return filepath.Join(c.scratch, fmt.Sprintf("fake-%s-%d.log", name, time.Now().UnixNano()))
}

func (c *checker) run(ctx context.Context) {
	for _, sdk := range []string{"go-sdk", "mcp-go"} {
		c.stdio(ctx, sdk)
		c.http(ctx, sdk)
	}
}

// common runs the checks that are the same over both transports.
func (c *checker) common(ctx context.Context, sdk, tr string, cl mcpClient, logPath string) {
	tools, err := cl.Tools(ctx)
	c.add(sdk, tr, "list tools", err == nil && len(tools) == 10, "%d tools, err=%v", len(tools), err)

	r, err := cl.Call(ctx, "echo", json.RawMessage(`{"text":"hi"}`), nil)
	c.add(sdk, tr, "call", err == nil && r.Text == "hi", "text=%q err=%v", r.Text, err)

	r, err = cl.Call(ctx, "fail", json.RawMessage(`{"msg":"x"}`), nil)
	c.add(sdk, tr, "tool error", err == nil && r.IsError, "isError=%v text=%q err=%v", r.IsError, r.Text, err)

	_, err = cl.Call(ctx, "nope", json.RawMessage(`{}`), nil)
	c.add(sdk, tr, "unknown tool", err != nil, "Go error: %v", short(err))

	// The fake server doesn't check its arguments (a go-sdk server with a
	// raw schema doesn't), so a client can't count on servers to.
	r, err = cl.Call(ctx, "echo", json.RawMessage(`{"text":5}`), nil)
	c.add(sdk, tr, "bad arguments (info)", true, "passed through: isError=%v text=%q err=%v", r.IsError, short(errors.New(r.Text)), short(err))

	// Progress, five times: a message that comes after the result is lost.
	var counts []string
	allThree := true
	for range 5 {
		var n atomic.Int32
		r, err = cl.Call(ctx, "slow", json.RawMessage(`{"seconds":3}`), func(float64, float64, string) { n.Add(1) })
		got := n.Load()
		time.Sleep(300 * time.Millisecond)
		counts = append(counts, fmt.Sprintf("%d/%d", got, n.Load()))
		allThree = allThree && err == nil && got == 3
	}
	c.add(sdk, tr, "progress", allThree, "messages of 3 (when the result came/after 0.3s), 5 calls: %s", strings.Join(counts, " "))

	cctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	start := time.Now()
	_, err = cl.Call(cctx, "slow", json.RawMessage(`{"seconds":20}`), func(float64, float64, string) {})
	took := time.Since(start)
	cancel()
	time.Sleep(500 * time.Millisecond)
	seen := strings.Contains(readLog(logPath), "slow cancelled")
	r2, err2 := cl.Call(ctx, "echo", json.RawMessage(`{"text":"after"}`), nil)
	c.add(sdk, tr, "cancel", err != nil && took < 3*time.Second && seen && err2 == nil && r2.Text == "after",
		"returned after %.1fs (%v); server saw the cancel: %v; next call ok: %v", took.Seconds(), short(err), seen, err2 == nil)

	start = time.Now()
	r, err = cl.Call(ctx, "big", json.RawMessage(`{"kb":6000}`), nil)
	c.add(sdk, tr, "6 MB result", err == nil && len(r.Text) >= 6000*1024, "%d bytes in %.1fs, err=%v", len(r.Text), time.Since(start).Seconds(), short(err))
}

func (c *checker) stdio(ctx context.Context, sdk string) {
	const tr = "stdio"
	connect := c.stdioConnect(sdk)
	lp := c.logPath(sdk + "-stdio")
	j, _ := newJob()
	defer j.close()
	start := time.Now()
	cl, err := connect(ctx, lp, false, j)
	if err != nil {
		c.add(sdk, tr, "connect", false, "%v", err)
		return
	}
	c.add(sdk, tr, "connect", true, "protocol %s in %dms", cl.Version(), time.Since(start).Milliseconds())
	c.common(ctx, sdk, tr, cl, lp)

	// A crash, then a restart.
	start = time.Now()
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	_, err = cl.Call(cctx, "crash", json.RawMessage(`{}`), nil)
	cancel()
	took := time.Since(start)
	_, err2 := cl.Call(ctx, "echo", json.RawMessage(`{"text":"x"}`), nil)
	c.add(sdk, tr, "crash", err != nil && took < 5*time.Second && err2 != nil,
		"call returned after %.1fs: %v; next call: %v", took.Seconds(), short(err), short(err2))
	cl.Close()
	start = time.Now()
	cl, err = connect(ctx, lp, false, j)
	if err == nil {
		took := time.Since(start)
		r, err2 := cl.Call(ctx, "echo", json.RawMessage(`{"text":"back"}`), nil)
		c.add(sdk, tr, "restart", err2 == nil && r.Text == "back", "new process %d connected in %dms, answers: %v", cl.PID(), took.Milliseconds(), err2 == nil)
		cl.Close()
	} else {
		c.add(sdk, tr, "restart", false, "%v", err)
	}

	// The process tree, with a launcher-like child: the SDK's Close alone,
	// then the Job Object.
	for _, useJob := range []bool{false, true} {
		var jj *job
		if useJob {
			jj, _ = newJob()
		}
		cl, err := connect(ctx, c.logPath(sdk+"-tree"), true, jj)
		if err != nil {
			c.add(sdk, tr, "tree", false, "%v", err)
			jj.close()
			continue
		}
		time.Sleep(700 * time.Millisecond) // the child starts
		before := tree(cl.PID())
		start := time.Now()
		cerr := cl.Close()
		closeTook := time.Since(start)
		time.Sleep(300 * time.Millisecond)
		afterClose := still(before)
		name := "tree: Close only"
		if useJob {
			name = "tree: Close + job"
			jj.close()
			time.Sleep(300 * time.Millisecond)
		}
		left := still(before)
		c.add(sdk, tr, name, len(left) == 0, "before: %s; after Close (%.1fs, err=%v): %s; at the end: %s",
			procList(before), closeTook.Seconds(), short(cerr), procList(afterClose), procList(left))
		for _, p := range left { // the spike's own leftovers, if still the same fake.exe
			if err := killProc(p, "fake.exe"); err != nil {
				log.Printf("leftover %s(%d) not stopped: %v", p.Exe, p.PID, err)
			}
		}
	}
}

func (c *checker) http(ctx context.Context, sdk string) {
	const tr = "http"
	lp := c.logPath(sdk + "-http")
	j, _ := newJob()
	defer j.close()
	cmd := exec.Command(c.fake, "-http", "127.0.0.1:0", "-log", lp)
	suspended(cmd)
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		c.add(sdk, tr, "start server", false, "%v", err)
		return
	}
	if err := j.adopt(cmd.Process.Pid); err != nil {
		c.add(sdk, tr, "start server", false, "%v", err)
		return
	}
	line, _ := bufio.NewReader(out).ReadString('\n')
	addr := strings.TrimSpace(strings.TrimPrefix(line, "listening "))
	url := "http://" + addr + "/mcp"
	start := time.Now()
	var cl mcpClient
	var err error
	if sdk == "go-sdk" {
		cl, err = goHTTP(ctx, url)
	} else {
		cl, err = m3HTTP(ctx, url, "")
		if err != nil {
			// The go-sdk server answers server/discover with legacy
			// versions only; mcp-go stops there instead of falling back.
			c.add(sdk, tr, "connect (default)", false, "%v", short(err))
			start = time.Now()
			cl, err = m3HTTP(ctx, url, "2025-11-25")
		}
	}
	if err != nil {
		c.add(sdk, tr, "connect", false, "%v", err)
		return
	}
	defer cl.Close()
	c.add(sdk, tr, "connect", true, "protocol %s in %dms", cl.Version(), time.Since(start).Milliseconds())
	c.common(ctx, sdk, tr, cl, lp)
}

func short(err error) string {
	if err == nil {
		return "<nil>"
	}
	s := err.Error()
	if len(s) > 140 {
		s = s[:140] + "…"
	}
	return s
}

func writeChecks(path string, cs []check) error {
	var b strings.Builder
	b.WriteString("| SDK | Transport | Check | OK | Detail |\n|---|---|---|---|---|\n")
	for _, c := range cs {
		ok := "yes"
		if !c.OK {
			ok = "**no**"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", c.SDK, c.Transport, c.Name, ok, strings.ReplaceAll(c.Detail, "|", "\\|"))
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
