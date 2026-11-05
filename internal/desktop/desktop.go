// Package desktop is desktop control (SPEC 8.10, SPIKE-039): an agent
// reads and uses one window of another app that the user picks, through
// agent-desktop's command-line program, run as a child process. Windows
// only in v1.
//
// The package has the runner (the pinned program, its JSON envelope and
// its errors), the compact snapshot form, the guards (the deny list,
// password fields, consent, CAPTCHAs) and the five tools with their
// sessions. Tests use a fake Runner.
package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// The pinned program: agent-desktop's Windows x64 command-line build, from
// its GitHub release, checked against SHA256 before each start (8.10).
// Jenab never downloads it: the installer puts it next to Jenab's
// executable, with its Apache-2.0 license (build/agent-desktop). A new
// version changes these three lines and the license folder only.
const (
	Version = "1.2.0"
	// SHA256 is agent-desktop.exe's, from
	// agent-desktop-v1.2.0-x86_64-pc-windows-msvc.tar.gz, whose SHA-256
	// a4199f74…20c3e8 is in the release's checksums.txt.
	SHA256 = "d274667031322e50b9c2b1ab81c2ca46835d52cc932769a5c34bd4d481fd94c3"
	// Program is its file name, next to Jenab's executable.
	Program = "agent-desktop.exe"
)

// envelopeVersion is the version of agent-desktop's JSON envelope that
// this package reads.
const envelopeVersion = "2.6"

// Call limits (8.10): a call is killed after callTimeout, or firstTimeout
// for a session's first snapshot, as an app's accessibility tree can take
// long at its cold start.
const (
	callTimeout  = 15 * time.Second
	firstTimeout = 30 * time.Second
)

// Runner runs one agent-desktop command and returns the data of its
// envelope. A failed command is an *Error; a call that passed its timeout
// was killed and is ErrKilled.
type Runner interface {
	Run(ctx context.Context, timeout time.Duration, args ...string) (json.RawMessage, error)
	// RunInput is Run with input on the command's standard input, for
	// --stdin: text the agent writes never goes in the arguments, which
	// other programs on the computer can read in the process list (8.10).
	RunInput(ctx context.Context, timeout time.Duration, input string, args ...string) (json.RawMessage, error)
}

var (
	// ErrKilled: the call passed its time limit and the process was killed.
	ErrKilled = errors.New("desktop: agent-desktop took too long and was stopped")
	// ErrUnsupported: desktop control needs Windows in v1.
	ErrUnsupported = errors.New("desktop: desktop control works only on Windows")
	// ErrProgram: the program is missing or isn't the pinned one.
	ErrProgram = errors.New("desktop: agent-desktop is missing or not the pinned version")
	// ErrHome: the state folder isn't private.
	ErrHome = errors.New("desktop: agent-desktop's state folder is not private")
)

// Error codes of agent-desktop's envelope that the tools handle.
const (
	CodeWindowNotFound  = "WINDOW_NOT_FOUND"
	CodeAppUnresponsive = "APP_UNRESPONSIVE"
	CodeTimeout         = "TIMEOUT"
	CodeStaleRef        = "STALE_REF"
	CodeElementNotFound = "ELEMENT_NOT_FOUND"
	CodeSnapshotMissing = "SNAPSHOT_NOT_FOUND"
	CodePolicyDenied    = "POLICY_DENIED"
	// CodeAmbiguous (1.1.0): the ref wasn't proven unique when its snapshot
	// was read, or two live elements match it now; nothing was sent.
	CodeAmbiguous = "AMBIGUOUS_TARGET"
)

// Error is a command agent-desktop answered with ok: false.
type Error struct {
	Code       string
	Message    string
	Suggestion string
	// Kind is its details.kind, such as "identity_not_unique", when it has one.
	Kind string
	// Delivery and Retry are its disposition: whether the action reached
	// the app ("not_delivered", …) and whether trying again is "safe".
	Delivery string
	Retry    string
}

func (e *Error) Error() string { return "agent-desktop: " + e.Code + ": " + e.Message }

type envelope struct {
	Version string          `json:"version"`
	OK      bool            `json:"ok"`
	Command string          `json:"command"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code        string          `json:"code"`
		Message     string          `json:"message"`
		Suggestion  string          `json:"suggestion"`
		Details     json.RawMessage `json:"details"`
		Disposition struct {
			Delivery string `json:"delivery"`
			Retry    string `json:"retry"`
		} `json:"disposition"`
	} `json:"error"`
}

// parseEnvelope reads one envelope: its data, or its error.
func parseEnvelope(out []byte) (json.RawMessage, error) {
	var env envelope
	if err := json.Unmarshal(out, &env); err != nil {
		return nil, fmt.Errorf("desktop: agent-desktop's answer is not JSON: %w", err)
	}
	if env.Version != envelopeVersion {
		return nil, fmt.Errorf("desktop: agent-desktop's answer has version %q; this build reads %s", env.Version, envelopeVersion)
	}
	if !env.OK {
		if env.Error == nil {
			return nil, &Error{Code: "UNKNOWN", Message: "the command failed without an error"}
		}
		e := env.Error
		var d struct {
			Kind string `json:"kind"`
		}
		_ = json.Unmarshal(e.Details, &d) // details differ by error; only kind is read
		return nil, &Error{Code: e.Code, Message: e.Message, Suggestion: e.Suggestion, Kind: d.Kind, Delivery: e.Disposition.Delivery, Retry: e.Disposition.Retry}
	}
	return env.Data, nil
}

// Window is one entry of list-windows.
type Window struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	App   string `json:"app_name"`
	PID   int    `json:"pid"`
	// Instance is agent-desktop's process instance. The lock uses it, not
	// the window owner's process: a UWP app's frame window belongs to
	// ApplicationFrameHost (SPIKE-039).
	Instance  string `json:"process_instance"`
	Minimized bool   `json:"minimized"`
	Visible   bool   `json:"visible"`
}

// listWindows lists the visible windows, of one app if app is set.
func listWindows(ctx context.Context, r Runner, app string) ([]Window, error) {
	args := []string{"list-windows"}
	if app != "" {
		args = append(args, "--app", app)
	}
	data, err := r.Run(ctx, callTimeout, args...)
	if err != nil {
		return nil, err
	}
	var ws []Window
	if err := json.Unmarshal(data, &ws); err != nil {
		return nil, fmt.Errorf("desktop: reading list-windows: %w", err)
	}
	return ws, nil
}
