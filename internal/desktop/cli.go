package desktop

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
	"time"
)

// CLI runs agent-desktop's command-line program, one process per command
// (8.10). Before each start it checks that the state folder is private
// and that the program's SHA-256 is the pinned one; the program file stays
// open, shared for reading only, until the process has started, so it
// can't be swapped in between. No console window opens, and a call that
// passes its time limit is killed.
type CLI struct {
	Path string // agent-desktop.exe
	Home string // its state folder (AGENT_DESKTOP_HOME), made private on first use
	Hash string // the program's SHA-256 in hex; "" is SHA256
}

// ProgramPath is where Jenab looks for the program: next to its own
// executable, where the installer puts it.
func ProgramPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(exe), Program), nil
}

// Run runs one command with the time limit.
func (c *CLI) Run(ctx context.Context, timeout time.Duration, args ...string) (json.RawMessage, error) {
	return c.run(ctx, timeout, nil, args...)
}

// RunInput runs one command with input on its standard input, through a
// pipe: agent-desktop's --stdin refuses a terminal.
func (c *CLI) RunInput(ctx context.Context, timeout time.Duration, input string, args ...string) (json.RawMessage, error) {
	return c.run(ctx, timeout, strings.NewReader(input), args...)
}

func (c *CLI) run(ctx context.Context, timeout time.Duration, stdin io.Reader, args ...string) (json.RawMessage, error) {
	if err := prepareHome(c.Home); err != nil {
		return nil, err
	}
	want := c.Hash
	if want == "" {
		want = SHA256
	}
	f, err := openPinned(c.Path, want)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, c.Path, args...)
	cmd.Env = append(cleanEnv(), "AGENT_DESKTOP_HOME="+c.Home)
	configure(cmd)
	cmd.Stdin = stdin // nil is the null device
	cmd.WaitDelay = 2 * time.Second
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &limited{w: &out, n: maxOutput}, &limited{w: &errOut, n: 4 << 10}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("desktop: starting agent-desktop: %w", err)
	}
	f.Close() // started: the image is mapped
	runErr := cmd.Wait()
	switch {
	case ctx.Err() != nil:
		return nil, context.Cause(ctx)
	case errors.Is(cctx.Err(), context.DeadlineExceeded):
		return nil, ErrKilled
	}
	data, err := parseEnvelope(out.Bytes())
	var de *Error
	if err != nil && !errors.As(err, &de) && runErr != nil {
		return nil, fmt.Errorf("desktop: agent-desktop failed: %w: %s", runErr, clip(strings.TrimSpace(errOut.String()), 300))
	}
	return data, err
}

// maxOutput bounds an envelope; a browser page's full tree is ~100 KB.
const maxOutput = 16 << 20

// cleanEnv is Jenab's environment without agent-desktop's own variables,
// so a session or agent id set outside doesn't change what it does.
func cleanEnv() []string {
	var out []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(kv), "AGENT_DESKTOP_") {
			out = append(out, kv)
		}
	}
	return out
}

// checkHash reads r to the end and compares its SHA-256 with want.
func checkHash(r io.Reader, want string) error {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return fmt.Errorf("%w: %v", ErrProgram, err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, want) {
		return fmt.Errorf("%w: its SHA-256 is %s, not %s", ErrProgram, got, want)
	}
	return nil
}

// limited writes up to n bytes and drops the rest.
type limited struct {
	w io.Writer
	n int
}

func (l *limited) Write(p []byte) (int, error) {
	if len(p) > l.n {
		if _, err := l.w.Write(p[:l.n]); err != nil {
			return 0, err
		}
		l.n = 0
		return len(p), nil
	}
	l.n -= len(p)
	return l.w.Write(p)
}
