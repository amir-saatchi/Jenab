package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/store"
)

func TestUIError(t *testing.T) {
	b := base{log: slog.New(slog.DiscardHandler), redact: func(s string) string { return strings.ReplaceAll(s, testKey, "[key]") }}
	cases := []struct {
		err  error
		kind string
	}{
		{agent.ErrInTurn, KindBusy},
		{store.ErrTitleTaken, KindInvalid},
		{store.ErrReadOnly, KindReadOnly},
		{project.ErrClosed, KindClosing},
		{errors.Join(errors.New("chat 01J"), store.ErrNotFound), KindNotFound},
		{&provider.Error{Provider: "p", Kind: provider.BadRequest, Message: "bad key " + testKey}, KindProvider},
		{errors.New("disk full near " + testKey), KindInternal},
		{fmt.Errorf("agent: turn 3: %w", store.ErrNothingToUndo), KindBusy},
		{store.ErrUndoWindow, KindInvalid},
		{&agent.PartError{Part: agent.ErrRolesNotUndone, Name: "chat roles", Err: errors.New("disk full")}, KindInternal},
	}
	for _, c := range cases {
		err := c.err
		b.guard("test", &err)
		var ue *UIError
		if !errors.As(err, &ue) || ue.Kind != c.kind || strings.Contains(ue.Message+ue.Details, testKey) {
			t.Errorf("%v: %#v", c.err, err)
		}
	}
	var big error = fmt.Errorf("store: %w", &store.UndoTooBigError{Rows: 30000, Max: 20000})
	b.guard("test", &big)
	uiErr(t, big, KindInvalid)
	if ue := (*UIError)(nil); errors.As(big, &ue) && ue.Message != "This turn changed 30000 rows. Undo can revert at most 20000 at once." {
		t.Errorf("too big: %q", ue.Message)
	}
	disk := errors.New("disk full")
	var both error = errors.Join(&agent.PartError{Part: agent.ErrRolesNotUndone, Name: "chat roles", Err: disk},
		&agent.PartError{Part: agent.ErrUserMemoryNotUndone, Name: "user memory edits", Err: disk})
	b.guard("test", &both)
	if ue := (*UIError)(nil); !errors.As(both, &ue) ||
		ue.Message != "The turn was undone, but its chat roles and its edits of user memory weren't. Undo it again to finish." {
		t.Errorf("both sides failed: %#v", both)
	}
	var redo error = &agent.PartError{Part: agent.ErrUserMemoryNotUndone, Name: "user memory edits", Redo: true, Err: disk}
	b.guard("test", &redo)
	if ue := (*UIError)(nil); !errors.As(redo, &ue) || ue.Message != "The turn was redone, but its edits of user memory weren't. Redo it again to finish." {
		t.Errorf("a redo's part failed: %#v", redo)
	}
	var ui error = &UIError{Kind: KindBusy, Message: "Wait."}
	b.guard("test", &ui)
	uiErr(t, ui, KindBusy)
	var none error
	b.guard("test", &none)
	if none != nil {
		t.Errorf("nil became %#v", none)
	}

	// A panic in a service method is an internal error, not a crash.
	err := func() (err error) {
		defer b.guard("panics", &err)
		panic("boom")
	}()
	uiErr(t, err, KindInternal)

}

// A cancelled call is closing, not logged; a panic's value is redacted in
// the log; Connect's input errors are invalid.
func TestUIErrorQuiet(t *testing.T) {
	var buf bytes.Buffer
	b := base{log: slog.New(slog.NewTextHandler(&buf, nil)), redact: func(s string) string { return strings.ReplaceAll(s, testKey, "[key]") }}
	for _, c := range []error{context.Canceled, fmt.Errorf("query: %w", context.DeadlineExceeded)} {
		err := c
		b.guard("test", &err)
		uiErr(t, err, KindClosing)
	}
	if buf.Len() != 0 {
		t.Errorf("logged %q", buf.String())
	}
	for _, c := range []error{provider.ErrBadName, provider.ErrBadBaseURL, provider.ErrBadField, provider.ErrNeedKey, provider.ErrKeyTooLong} {
		err := fmt.Errorf("provider x: %w", c)
		b.guard("test", &err)
		uiErr(t, err, KindInvalid)
	}
	err := func() (err error) {
		defer b.guard("panics", &err)
		panic("boom " + testKey)
	}()
	uiErr(t, err, KindInternal)
	if strings.Contains(buf.String(), testKey) || !strings.Contains(buf.String(), "boom [key]") {
		t.Errorf("log %q", buf.String())
	}
}

// A UIError made from a known error still matches it with errors.Is, and
// carries only the sentinel, not the error's text.
func TestUIErrorKeepsItsKnownCause(t *testing.T) {
	b := base{log: slog.New(slog.DiscardHandler), redact: func(s string) string { return s }}
	all := []error{agent.ErrInTurn, agent.ErrNoRetry, agent.ErrRefreshing, store.ErrNotFound}
	for _, known := range all {
		var err error = fmt.Errorf("chat 01J: %w", known)
		b.guard("test", &err)
		if !errors.Is(err, known) {
			t.Errorf("%v: lost its cause: %#v", known, err)
		}
		for _, other := range all {
			if other != known && errors.Is(err, other) {
				t.Errorf("%v matches %v", known, other)
			}
		}
		var ue *UIError
		if errors.As(err, &ue) && ue.Cause != known {
			t.Errorf("%v: the cause is %v, not the sentinel alone", known, ue.Cause)
		}
	}
	var odd error = errors.New("disk full")
	b.guard("test", &odd)
	if errors.Unwrap(odd) != nil {
		t.Errorf("an unknown error has a cause: %#v", odd)
	}
}
