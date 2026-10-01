package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Limits used by every candidate, where the library supports them.
const (
	stepLimit  = 10_000_000      // Starlark and Risor: interpreter steps per run
	allocLimit = 10_000_000      // Tengo: object allocations per run
	stackLimit = 1_000           // call depth, where it can be set
	strLimit   = 16 << 20        // Tengo: max string / bytes length
	runTimeout = 2 * time.Second // cancel after this; the real step timeout would come from the pipeline
)

// RunOpts configures one script run.
type RunOpts struct {
	Default bool // library defaults (no limits, default globals) instead of the Burrow sandbox
	NoSteps bool // sandbox, but without the step / alloc limit (to test cancel alone)
	DB      *DB  // exposes db.query and the transform functions when set
}

// Engine is one embedded language.
type Engine interface {
	Name() string
	Run(ctx context.Context, src string, o RunOpts) (any, error)
}

var engines = []Engine{starlarkEngine{}, gojaEngine{}, luaEngine{}, risorEngine{}, tengoEngine{}}

func engineByName(name string) Engine {
	for _, e := range engines {
		if e.Name() == name {
			return e
		}
	}
	return nil
}

// Cancel causes: the step timeout or the memory watchdog.
var (
	errTimeout  = errors.New("timeout")
	errWatchdog = errors.New("memory watchdog")
)

func causeText(ctx context.Context) string {
	if c := context.Cause(ctx); c != nil {
		return c.Error()
	}
	return "cancelled"
}

// classify turns an engine error into a short "stopped by" label.
func classify(err error, ctx context.Context) string {
	if err == nil {
		return "finished"
	}
	msg := strings.ToLower(err.Error())
	cause := context.Cause(ctx)
	switch {
	case strings.Contains(msg, "too many steps"), strings.Contains(msg, "step limit"):
		return "step limit"
	case strings.Contains(msg, "allocation limit"):
		return "alloc limit"
	case errors.Is(cause, errWatchdog) || strings.Contains(msg, "memory watchdog"):
		return "watchdog cancel"
	case errors.Is(cause, errTimeout) || strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline"):
		return "timeout cancel"
	case strings.Contains(msg, "index out of range [2048]"):
		return "stack limit (recovered Go panic)"
	case strings.Contains(msg, "called recursively"):
		return "recursion rejected"
	case strings.Contains(msg, "stack overflow"), strings.Contains(msg, "call stack"), strings.Contains(msg, "maximum call stack"), strings.Contains(msg, "stack depth"):
		return "stack limit"
	case strings.Contains(msg, "exceeding string size limit"), strings.Contains(msg, "size limit"), strings.Contains(msg, "excessive repeat"):
		return "size limit"
	}
	return "error: " + firstLine(err.Error())
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.ReplaceAll(s, "|", "/")
	if len(s) > 90 {
		s = s[:90] + "…"
	}
	return s
}

// short renders a script result for a table cell.
func short(v any) string {
	s := fmt.Sprintf("%v", v)
	if len(s) > 40 {
		s = s[:40] + "…"
	}
	return strings.ReplaceAll(s, "|", "/")
}
