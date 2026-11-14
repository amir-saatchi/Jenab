package main

// Running agent-browser as Jenab would: the pinned program, checked by its
// SHA-256 before each start; a private state folder that holds its socket
// folder, temporary profile and config; a trusted config file passed with
// AGENT_BROWSER_CONFIG, so ~/.agent-browser/config.json is never read; no
// console window; a time limit per call.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// AB is one agent-browser setup.
type AB struct {
	Path    string // agent-browser-win32-x64.exe
	Hash    string // its SHA-256
	Home    string // the private state folder
	Browser string // --executable-path

	Proxy      string   // http://127.0.0.1:port; "" for none
	Bypass     bool     // add --proxy-bypass-list=<-loopback>
	Args       []string // more browser arguments
	Allowed    []string // --allowed-domains
	Boundaries bool
	MaxOutput  int
	Policy     map[string]any // the action policy; nil for none
	Idle       string         // --idle-timeout
	Session    string
	Env        []string // more environment, NAME=value
}

// Result is agent-browser's --json envelope.
type Result struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error"`

	Raw     string
	Stderr  string
	Elapsed time.Duration
	Killed  bool
}

func (a *AB) sock() string { return filepath.Join(a.Home, "sock") }

// Setup makes the state folder and writes the config and policy.
func (a *AB) Setup() error {
	if err := prepareHome(a.Home); err != nil {
		return err
	}
	for _, d := range []string{"sock", "tmp", "downloads", "shots"} {
		if err := os.MkdirAll(filepath.Join(a.Home, d), 0o700); err != nil {
			return err
		}
	}
	cfg := map[string]any{
		"json":              true,
		"executablePath":    a.Browser,
		"downloadPath":      filepath.Join(a.Home, "downloads"),
		"screenshotDir":     filepath.Join(a.Home, "shots"),
		"idleTimeout":       a.Idle,
		"noWebmcp":          true,
		"hideScrollbars":    true,
		"contentBoundaries": a.Boundaries,
	}
	if a.Idle == "" {
		cfg["idleTimeout"] = "60s"
	}
	if a.MaxOutput > 0 {
		cfg["maxOutput"] = a.MaxOutput
	}
	if a.Proxy != "" {
		cfg["proxy"] = a.Proxy
	}
	args := append([]string(nil), a.Args...)
	if a.Bypass {
		args = append(args, "--proxy-bypass-list=<-loopback>")
	}
	if len(args) > 0 {
		cfg["args"] = strings.Join(args, "\n")
	}
	if len(a.Allowed) > 0 {
		cfg["allowedDomains"] = a.Allowed
	}
	if a.Policy != nil {
		p := filepath.Join(a.Home, "policy.json")
		b, _ := json.MarshalIndent(a.Policy, "", "  ")
		if err := os.WriteFile(p, b, 0o600); err != nil {
			return err
		}
		cfg["actionPolicy"] = p
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(filepath.Join(a.Home, "config.json"), b, 0o600)
}

// env is this process's environment without agent-browser's variables or
// proxy variables, plus the spike's.
func (a *AB) env() []string {
	var out []string
	for _, kv := range os.Environ() {
		k := strings.ToUpper(strings.SplitN(kv, "=", 2)[0])
		switch {
		case strings.HasPrefix(k, "AGENT_BROWSER_"), k == "HTTP_PROXY", k == "HTTPS_PROXY", k == "ALL_PROXY", k == "NO_PROXY",
			k == "TEMP", k == "TMP", strings.HasPrefix(k, "AI_GATEWAY_"), strings.HasPrefix(k, "BROWSER_USE_"):
			continue
		}
		out = append(out, kv)
	}
	tmp := filepath.Join(a.Home, "tmp")
	out = append(out,
		"AGENT_BROWSER_CONFIG="+filepath.Join(a.Home, "config.json"),
		"AGENT_BROWSER_SOCKET_DIR="+a.sock(),
		"AGENT_BROWSER_NO_VERSION_CHECK=1",
		"TEMP="+tmp, "TMP="+tmp,
	)
	if a.Session != "" {
		out = append(out, "AGENT_BROWSER_SESSION="+a.Session)
	}
	return append(out, a.Env...)
}

// Run runs one command with a time limit.
func (a *AB) Run(ctx context.Context, timeout time.Duration, args ...string) Result {
	return a.run(ctx, timeout, nil, args...)
}

func (a *AB) run(ctx context.Context, timeout time.Duration, stdin io.Reader, args ...string) Result {
	var r Result
	f, err := openPinned(a.Path, a.Hash)
	if err != nil {
		r.Error = "pinned program: " + err.Error()
		return r
	}
	defer f.Close()
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, a.Path, append([]string{"--json"}, args...)...)
	cmd.Env = a.env()
	cmd.Dir = a.Home
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	cmd.Stdin = stdin
	cmd.WaitDelay = 2 * time.Second
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	t0 := time.Now()
	if err := cmd.Start(); err != nil {
		r.Error = err.Error()
		return r
	}
	f.Close()
	cmd.Wait()
	r.Elapsed = time.Since(t0)
	r.Killed = errors.Is(cctx.Err(), context.DeadlineExceeded)
	r.Raw, r.Stderr = out.String(), errOut.String()
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &r); err != nil && r.Error == "" {
		r.Error = fmt.Sprintf("no JSON: %q %q", clip(r.Raw, 200), clip(r.Stderr, 200))
	}
	return r
}

// Close closes the session's browser.
func (a *AB) Close(ctx context.Context) Result { return a.Run(ctx, 20*time.Second, "close") }

// DaemonPID reads the daemon's pid file.
func (a *AB) DaemonPID() int {
	s := a.Session
	if s == "" {
		s = "default"
	}
	b, err := os.ReadFile(filepath.Join(a.sock(), s+".pid"))
	if err != nil {
		return 0
	}
	var pid int
	fmt.Sscan(strings.TrimSpace(string(b)), &pid)
	return pid
}

func checkHash(r io.Reader, want string) error {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, want) {
		return fmt.Errorf("SHA-256 is %s, not %s", got, want)
	}
	return nil
}

func fileHash(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil))
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// field reads one string field of a result's data.
func (r Result) field(name string) string {
	var m map[string]any
	json.Unmarshal(r.Data, &m)
	if s, ok := m[name].(string); ok {
		return s
	}
	return ""
}
