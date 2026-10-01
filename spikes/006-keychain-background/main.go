// SPIKE-006 (Windows part): can a background Burrow process read secrets from
// Windows Credential Manager without prompting the user?
//
// Build and run:
//
//	go build -o kc.exe . && go build -ldflags -H=windowsgui -o kc-bg.exe . && go build -o other.exe ./other
//	./kc.exe run > results.md
//	./kc.exe watch 30        # optional: read every 10 s for 30 minutes (lock the screen during it)
//
// Test entries use the service "burrow-spike006" and are deleted at the end.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/zalando/go-keyring"
)

const service = "burrow-spike006"

// secrets stored by the test: name -> value.
var secrets = map[string]string{
	"API_KEY":                               "sk-test-" + strings.Repeat("a", 40),
	"MAX_SIZE":                              strings.Repeat("x", 2560),
	"UNICODE":                               "schlüssel-کلید-🔑",
	"EMPTY":                                 "",
	"LONG_NAME_" + strings.Repeat("N", 400): "v",
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: kc run | child <out.json> | watch <minutes> | cleanup")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "run":
		run()
	case "child":
		child(os.Args[2])
	case "watch":
		watch(os.Args[2])
	case "cleanup":
		cleanup()
	}
}

// childResult is what a background child reports back.
type childResult struct {
	PID, ParentPID int
	HasConsole     bool
	SessionID      uint32
	Locked         string
	Reads          map[string]string // name -> "ok", "wrong value" or the error
	ReadTime       string
}

func child(out string) {
	r := childResult{PID: os.Getpid(), ParentPID: os.Getppid(), HasConsole: hasConsole(), Reads: map[string]string{}}
	r.SessionID, r.Locked = session()
	start := time.Now()
	for name, want := range secrets {
		got, err := keyring.Get(service, name)
		switch {
		case err != nil:
			r.Reads[name] = "error: " + err.Error()
		case got != want:
			r.Reads[name] = "wrong value"
		default:
			r.Reads[name] = "ok"
		}
	}
	r.ReadTime = time.Since(start).String()
	b, _ := json.MarshalIndent(r, "", "  ")
	os.WriteFile(out+".tmp", b, 0o600)
	os.Rename(out+".tmp", out)
}

func run() {
	exe, _ := os.Executable()
	dir := filepath.Dir(exe)
	bg := filepath.Join(dir, "kc-bg.exe")
	fmt.Printf("# SPIKE-006 results (Windows)\n\nzalando/go-keyring v0.2.8 (danieljoos/wincred v1.2.3), Go %s, %s.\n", runtime.Version(), time.Now().Format("2006-01-02"))
	sid, locked := session()
	fmt.Printf("Harness: PID %d, session %d, screen %s.\n", os.Getpid(), sid, locked)

	// 1. store
	fmt.Println("\n## 1. Store (foreground process)\n")
	fmt.Println("| Name | Value size | Result |")
	fmt.Println("|---|---|---|")
	for _, name := range sortedNames() {
		fmt.Printf("| `%s` | %d bytes | %s |\n", short(name), len(secrets[name]), errText(keyring.Set(service, name, secrets[name])))
	}
	for _, c := range []struct{ name, value string }{
		{"TOO_BIG", strings.Repeat("x", 2561)},
		{"TOO_BIG_UNICODE", strings.Repeat("ü", 1281)}, // 2562 bytes, 1281 characters
	} {
		fmt.Printf("| `%s` | %d bytes | %s |\n", c.name, len(c.value), errText(keyring.Set(service, c.name, c.value)))
	}
	_, err := keyring.Get(service, "NOT_THERE")
	fmt.Printf("\nReading a name that does not exist: %s (`errors.Is(err, keyring.ErrNotFound)` = %v).\n", errText(err), errors.Is(err, keyring.ErrNotFound))

	// 2. read speed
	const n = 1000
	start := time.Now()
	for i := 0; i < n; i++ {
		if _, err := keyring.Get(service, "API_KEY"); err != nil {
			panic(err)
		}
	}
	fmt.Printf("\n## 2. Read speed\n\n%d reads of `API_KEY` in the same process: %.1f µs per read.\n", n, float64(time.Since(start).Microseconds())/n)

	// 3. background children
	fmt.Println("\n## 3. Reads from windowless background processes\n")
	fmt.Println("`kc-bg.exe` is built with `-H windowsgui` (no console, no window), like Burrow in tray mode.\n")
	fmt.Println("| How it was started | Console | Parent PID | Session | Screen | Reads ok | Time for all reads | Prompt seen |")
	fmt.Println("|---|---|---|---|---|---|---|---|")
	launches := []struct {
		label string
		cmd   func(out string) *exec.Cmd
	}{
		{"detached from the harness (`DETACHED_PROCESS`)", func(out string) *exec.Cmd {
			c := exec.Command(bg, "child", out)
			c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008 | 0x00000200, HideWindow: true} // DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP
			return c
		}},
		{"by Explorer (`explorer.exe kc-bg.exe`), as from the Start menu or at login", func(out string) *exec.Cmd {
			// explorer.exe passes no arguments, so the child reads its output path from a file next to it
			os.WriteFile(filepath.Join(dir, "kc-bg.args"), []byte(out), 0o600)
			return exec.Command("explorer.exe", bg)
		}},
		{"hidden, by PowerShell `Start-Process -WindowStyle Hidden`", func(out string) *exec.Cmd {
			return exec.Command("powershell", "-NoProfile", "-Command", fmt.Sprintf("Start-Process -WindowStyle Hidden -FilePath '%s' -ArgumentList 'child','%s'", bg, out))
		}},
	}
	for i, l := range launches {
		out := filepath.Join(dir, fmt.Sprintf("child-%d.json", i))
		os.Remove(out)
		if err := l.cmd(out).Start(); err != nil {
			fmt.Printf("| %s | start failed: %v |||||||\n", l.label, err)
			continue
		}
		r, err := waitResult(out, 20*time.Second)
		if err != nil {
			fmt.Printf("| %s | %v |||||||\n", l.label, err)
			continue
		}
		fmt.Printf("| %s | %v | %d | %d | %s | %s | %s | no |\n", l.label, r.HasConsole, r.ParentPID, r.SessionID, r.Locked, readsOK(r.Reads), r.ReadTime)
		os.Remove(out)
	}
	os.Remove(filepath.Join(dir, "kc-bg.args"))

	// 4. other programs of the same user
	fmt.Println("\n## 4. Other programs of the same user\n")
	out, err := exec.Command(filepath.Join(dir, "other.exe"), service+":API_KEY").CombinedOutput()
	fmt.Printf("`other.exe`, a separate program that calls `CredReadW` directly (not go-keyring): %s\n", strings.TrimSpace(string(out))+errSuffix(err))
	out, _ = exec.Command("cmdkey", "/list:"+service+"*").CombinedOutput()
	fmt.Printf("\n`cmdkey /list:%s*` (the built-in tool; shows names, not values):\n\n```\n%s\n```\n", service, strings.TrimSpace(strings.ReplaceAll(string(out), "\r", "")))

	// 5. cleanup
	fmt.Println("\n## 5. Delete\n")
	fmt.Printf("`keyring.DeleteAll(%q)`: %s. Entries left: %d.\n", service, errText(keyring.DeleteAll(service)), countLeft())
}

func cleanup() {
	fmt.Println(errText(keyring.DeleteAll(service)), "left:", countLeft())
}

// watch reads API_KEY every 10 s and logs the result with the screen state,
// so a read while the screen is locked (or after sleep) shows up in the log.
func watch(minutes string) {
	var m int
	fmt.Sscan(minutes, &m)
	if err := keyring.Set(service, "API_KEY", secrets["API_KEY"]); err != nil {
		panic(err)
	}
	exe, _ := os.Executable()
	logPath := filepath.Join(filepath.Dir(exe), "watch.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	var mu sync.Mutex
	logRead := func(event string) {
		mu.Lock()
		defer mu.Unlock()
		got, err := keyring.Get(service, "API_KEY")
		res := "ok"
		if err != nil {
			res = "error: " + err.Error()
		} else if got != secrets["API_KEY"] {
			res = "wrong value"
		}
		_, locked := session()
		fmt.Fprintf(f, "%s event=%s logonui=%s %s read=%s\n", time.Now().Format("15:04:05"), event, strings.Fields(locked)[0], lockSignals(), res)
	}
	// on lock/unlock, read once right away and once 5 s later
	listenSessionEvents(func(e string) {
		go logRead(e)
		time.AfterFunc(5*time.Second, func() { logRead(e + "+5s") })
	})
	end := time.Now().Add(time.Duration(m) * time.Minute)
	for time.Now().Before(end) {
		logRead("tick")
		time.Sleep(10 * time.Second)
	}
	keyring.Delete(service, "API_KEY")
}

func waitResult(path string, timeout time.Duration) (childResult, error) {
	var r childResult
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil {
			return r, json.Unmarshal(b, &r)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return r, fmt.Errorf("no result after %v", timeout)
}

func readsOK(m map[string]string) string {
	ok := 0
	var bad []string
	for name, res := range m {
		if res == "ok" {
			ok++
		} else {
			bad = append(bad, short(name)+": "+res)
		}
	}
	s := fmt.Sprintf("%d of %d", ok, len(m))
	if len(bad) > 0 {
		s += " (" + strings.Join(bad, "; ") + ")"
	}
	return s
}

func countLeft() int {
	n := 0
	for _, name := range append(sortedNames(), "TOO_BIG", "TOO_BIG_UNICODE") {
		if _, err := keyring.Get(service, name); err == nil {
			n++
		}
	}
	return n
}

func sortedNames() []string {
	return []string{"API_KEY", "MAX_SIZE", "UNICODE", "EMPTY", "LONG_NAME_" + strings.Repeat("N", 400)}
}

func short(s string) string {
	if len(s) > 30 {
		return s[:20] + fmt.Sprintf("… (%d bytes)", len(s))
	}
	return s
}

func errText(err error) string {
	if err == nil {
		return "ok"
	}
	return "error: " + err.Error()
}

func errSuffix(err error) string {
	if err == nil {
		return ""
	}
	return " (" + err.Error() + ")"
}
