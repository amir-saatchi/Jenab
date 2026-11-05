package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// CLI runs agent-desktop.exe as a child process, one command per process, the way Jenab would
// (the VISO sidecar does the same in Rust). Each call has a time limit; when it passes, the
// process is killed and the call reports a timeout, not "not found".

type CLI struct {
	Bin  string // full path to agent-desktop.exe
	Home string // AGENT_DESKTOP_HOME; agent-desktop must create it itself (see README)
}

type Envelope struct {
	Version string          `json:"version"`
	OK      bool            `json:"ok"`
	Command string          `json:"command"`
	Data    json.RawMessage `json:"data"`
	Error   *EnvError       `json:"error"`
}

type EnvError struct {
	Code        string          `json:"code"`
	Message     string          `json:"message"`
	Suggestion  string          `json:"suggestion"`
	Disposition json.RawMessage `json:"disposition"`
	Details     json.RawMessage `json:"details"`
}

func (e *EnvError) Error() string { return e.Code + ": " + e.Message }

var ErrTimeout = errors.New("agent-desktop timed out and was killed")

const createNoWindow = 0x08000000

// Run runs one command. raw is the process's stdout, the JSON envelope.
func (c *CLI) Run(timeout time.Duration, args ...string) (env Envelope, raw []byte, took time.Duration, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Bin, args...)
	cmd.Env = append(os.Environ(), "AGENT_DESKTOP_HOME="+c.Home)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	cmd.WaitDelay = 2 * time.Second
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	start := time.Now()
	runErr := cmd.Run()
	took = time.Since(start)
	raw = out.Bytes()
	if ctx.Err() == context.DeadlineExceeded {
		return env, raw, took, ErrTimeout
	}
	if jerr := json.Unmarshal(raw, &env); jerr != nil {
		if runErr != nil {
			return env, raw, took, fmt.Errorf("%v: %s", runErr, bytes.TrimSpace(errb.Bytes()))
		}
		return env, raw, took, fmt.Errorf("bad envelope: %v", jerr)
	}
	if !env.OK && env.Error != nil {
		return env, raw, took, env.Error
	}
	return env, raw, took, nil
}

// Window is one entry of list-windows.
type Window struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	AppName         string `json:"app_name"`
	PID             int    `json:"pid"`
	ProcessInstance string `json:"process_instance"`
	IsFocused       bool   `json:"is_focused"`
	Minimized       bool   `json:"minimized"`
}

func (c *CLI) ListWindows(app string) ([]Window, error) {
	args := []string{"list-windows"}
	if app != "" {
		args = append(args, "--app", app)
	}
	env, _, _, err := c.Run(10*time.Second, args...)
	if err != nil {
		return nil, err
	}
	var ws []Window
	err = json.Unmarshal(env.Data, &ws)
	return ws, err
}

// WaitWindow polls list-windows until match returns true for a window of app.
func (c *CLI) WaitWindow(app string, limit time.Duration, match func(Window) bool) (Window, error) {
	deadline := time.Now().Add(limit)
	for {
		ws, err := c.ListWindows(app)
		if err == nil {
			for _, w := range ws {
				if match(w) {
					return w, nil
				}
			}
		}
		if time.Now().After(deadline) {
			return Window{}, fmt.Errorf("no %s window after %s (last error: %v)", app, limit, err)
		}
		time.Sleep(300 * time.Millisecond)
	}
}
