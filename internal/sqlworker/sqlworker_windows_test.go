package sqlworker

import (
	"bufio"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// TestWorkerDiesWithParent kills an app that has a worker: the job's last
// handle closes with it, and the worker goes too.
func TestWorkerDiesWithParent(t *testing.T) {
	path := openDB(t, 1, 1)
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), parentEnv+"="+path)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Wait()
	defer cmd.Process.Kill()
	line, _ := bufio.NewReader(out).ReadString('\n')
	pid, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatalf("helper said %q", line)
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		t.Fatalf("open worker %d: %v", pid, err)
	}
	defer windows.CloseHandle(h)
	if ev, _ := windows.WaitForSingleObject(h, 0); ev != uint32(windows.WAIT_TIMEOUT) {
		t.Fatal("the worker ended before its parent")
	}
	cmd.Process.Kill()
	if ev, _ := windows.WaitForSingleObject(h, 5000); ev != windows.WAIT_OBJECT_0 {
		t.Errorf("worker %d still runs 5 s after its parent was killed", pid)
	}
}
