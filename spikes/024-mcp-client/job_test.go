//go:build windows

package main

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// startChild starts the fake server with a child in a job, as Viso's shim
// starting the Viso app. FAKE is the fake server's exe.
func startChild(t *testing.T) (j *job, shim, child proc) {
	fake := os.Getenv("FAKE")
	if fake == "" {
		t.Skip("FAKE not set")
	}
	j, err := newJob()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(fake, "-child")
	suspended(cmd)
	// Kept open (and referenced until the test ends, so the GC can't close
	// it), so the server keeps running.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stdin.Close() })
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := j.adopt(cmd.Process.Pid); err != nil {
		t.Fatal(err)
	}
	stop := j.watch(cmd.Process.Pid)
	defer stop()
	time.Sleep(time.Second)
	tr := tree(cmd.Process.Pid)
	for _, p := range tr {
		if p.PID != cmd.Process.Pid && p.Exe == "fake.exe" {
			child = p
		}
	}
	if len(tr) == 0 || child.PID == 0 {
		t.Fatalf("no child: tree %s", procList(tr))
	}
	return j, tr[0], child
}

// TestCloseOnly: closeOnly stops the shim and leaves the child running.
func TestCloseOnly(t *testing.T) {
	j, shim, child := startChild(t)
	others, err := j.closeOnly(shim.PID)
	time.Sleep(300 * time.Millisecond)
	t.Logf("left %v err %v; running: %s", others, err, procList(still([]proc{shim, child})))
	if len(still([]proc{child})) != 1 || len(still([]proc{shim})) != 0 {
		t.Fatal("wrong processes stopped")
	}
	if err := killProc(child, "fake.exe"); err != nil {
		t.Fatal(err)
	}
}

// TestWatchDisarms: once the job holds the child, closing the job handle
// (as a crash of the spike would) kills nothing.
func TestWatchDisarms(t *testing.T) {
	j, shim, child := startChild(t)
	j.mu.Lock()
	armed := j.armed
	windows.CloseHandle(j.h) // what the OS does when the spike dies
	j.h = 0
	j.mu.Unlock()
	time.Sleep(300 * time.Millisecond)
	t.Logf("armed %v; running: %s", armed, procList(still([]proc{shim, child})))
	if armed || len(still([]proc{shim, child})) != 2 {
		t.Fatal("the watch didn't disarm the job")
	}
	_ = killProc(child, "fake.exe")
	_ = killProc(shim, "fake.exe")
}
