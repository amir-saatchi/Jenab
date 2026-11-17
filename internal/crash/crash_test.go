package crash

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeKey stands for a provider's key that reached a panic's message.
const fakeKey = "sk-test-0123456789abcdef"

func redact(s string) string { return strings.ReplaceAll(s, fakeKey, "[secret:provider:test]") }

// The child is shaped like cmd/desktop's run: it starts the crash output,
// defers its clean-up, runs the UI loop on its own goroutine, as MyGo
// does, and stops the output only after a normal quit. A closure posted
// to the loop panics, and nothing recovers it. The panic runs the defers
// first, which once included stop and left an empty file (TASK-067).
func crashChild(dir string) {
	_, stop, err := Start(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	defer fmt.Fprintln(os.Stderr, "the clean-up ran")
	ui := make(chan func(), 1)
	ui <- func() { panic("the call to " + fakeKey + " broke") }
	close(ui)
	for f := range ui {
		f()
	}
	stop()
}

// A panic in a posted closure writes the crash file, and the next start
// makes it a crash log without the secret (TASK-067).
func TestAPanicInAPostedClosureWritesTheCrashFile(t *testing.T) {
	if dir := os.Getenv("JENAB_CRASH_CHILD"); dir != "" {
		crashChild(dir)
		return
	}
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestAPanicInAPostedClosureWritesTheCrashFile$")
	cmd.Env = append(os.Environ(), "JENAB_CRASH_CHILD="+dir)
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 2 {
		t.Fatalf("the child ended with %v; want exit status 2\n%s", err, out)
	}
	raws, _ := filepath.Glob(filepath.Join(dir, "crash-*.raw"))
	if len(raws) != 1 {
		t.Fatalf("crash files %v; want one", raws)
	}

	logs, err := Collect(dir, "", redact)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("logs %v; want one", logs)
	}
	b, err := os.ReadFile(logs[0])
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, want := range []string{"panic: the call to [secret:provider:test] broke", "goroutine ", "crashChild"} {
		if !strings.Contains(text, want) {
			t.Errorf("the crash log has no %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, fakeKey) {
		t.Error("the crash log has the key")
	}
	if raws, _ := filepath.Glob(filepath.Join(dir, "crash-*.raw")); len(raws) != 0 {
		t.Errorf("crash files left: %v", raws)
	}
	// The next start finds nothing new.
	if again, err := Collect(dir, "", redact); err != nil || len(again) != 0 {
		t.Errorf("a second Collect: %v, %v; want nothing", again, err)
	}
}

// A run that quits normally leaves nothing behind.
func TestStopRemovesTheEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path, stop, err := Start(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("no crash file while running: %v", err)
	}
	stop()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the empty crash file is still there: %v", err)
	}
}

// Collect skips this run's file, removes empty ones and keeps the newest
// Keep logs.
func TestCollectKeepsTheLastFew(t *testing.T) {
	dir := t.TempDir()
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for i := range Keep {
		write(fmt.Sprintf("crash-20260101-00000%d-1.log", i), "old")
	}
	write("crash-20261117-090000-7.raw", "")            // killed, not crashed
	write("crash-20261117-100000-8.raw", "panic: boom") // crashed
	write("crash-20261117-110000-9.raw", "")            // this run
	write("jenab.log", "the app's log")
	logs, err := Collect(dir, filepath.Join(dir, "crash-20261117-110000-9.raw"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || filepath.Base(logs[0]) != "crash-20261117-100000-8.log" {
		t.Fatalf("logs %v", logs)
	}
	var names []string
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		names = append(names, e.Name())
	}
	want := []string{
		"crash-20260101-000001-1.log", "crash-20260101-000002-1.log", "crash-20260101-000003-1.log",
		"crash-20260101-000004-1.log", "crash-20261117-100000-8.log", "crash-20261117-110000-9.raw", "jenab.log",
	}
	if strings.Join(names, " ") != strings.Join(want, " ") {
		t.Errorf("files\n%v\nwant\n%v", names, want)
	}
}
