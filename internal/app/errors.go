package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/bucket"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/store"
)

// UIError is what the frontend gets for a failed call (Q25): a kind it can
// act on, a short plain-English message, and the details for *Copy
// details*. In JavaScript it is the cause of the rejected call.
type UIError struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// The kinds of UIError.
const (
	KindInvalid  = "invalid"   // the input can be fixed: a bad title, an empty message
	KindNotFound = "not_found" // the project, chat or object is gone
	KindBusy     = "busy"      // not now: the chat is in a turn, nothing waits
	KindReadOnly = "read_only" // the project is open read-only after damage (2.7)
	KindClosing  = "closing"   // the app is shutting down
	KindProvider = "provider"  // a model provider failed (3.8)
	KindInternal = "internal"  // a bug or a system failure; it is logged
)

func (e *UIError) Error() string { return e.Message }

// MarshalJSON fixes the shape the frontend reads.
func (e *UIError) MarshalJSON() ([]byte, error) {
	type plain UIError
	return json.Marshal((*plain)(e))
}

// known are the errors with their own message, checked in order.
var known = []struct {
	err  error
	kind string
	msg  string
}{
	{agent.ErrRefused, KindClosing, "Jenab is shutting down."},
	{project.ErrClosed, KindClosing, "Jenab is shutting down."},
	{agent.ErrEmpty, KindInvalid, "The message is empty."},
	{agent.ErrInTurn, KindBusy, "The chat is in a turn. Wait for it to end, or stop it."},
	{agent.ErrNoRetry, KindBusy, "There is nothing to retry."},
	{agent.ErrNotWaiting, KindBusy, "The chat no longer waits for this answer."},
	{agent.ErrBadAnswer, KindInvalid, "The answer doesn't fit the card."},
	{project.ErrNoName, KindInvalid, "A project needs a name."},
	{store.ErrMother, KindInvalid, "The Mother chat can't be archived or deleted, or get a role."},
	{store.ErrTitleTaken, KindInvalid, "Another chat has this title."},
	{store.ErrBadTitle, KindInvalid, "A title needs 1 to 100 characters on one line."},
	{store.ErrTooLong, KindInvalid, "The text is too long."},
	{store.ErrReadOnly, KindReadOnly, "The project is open read-only after damage."},
	{store.ErrNewerFormat, KindInvalid, "The project was saved by a newer version of Jenab."},
	{bucket.ErrBadKey, KindInvalid, "That isn't a usable file name."},
	{store.ErrNotFound, KindNotFound, "It no longer exists."},
	{bucket.ErrNotFound, KindNotFound, "It no longer exists."},
	{provider.ErrUnknownModel, KindInvalid, "That model isn't connected."},
	{provider.ErrUnknownProvider, KindNotFound, "That provider isn't connected."},
}

// toUI turns err into a *UIError. Known kinds get their message; anything
// else is "Something went wrong" and is logged (Q24: errors are logged
// once, here). redact removes secrets from the details (6.7).
func toUI(err error, log *slog.Logger, redact func(string) string, what string) error {
	var ue *UIError
	if errors.As(err, &ue) {
		return ue
	}
	details := redact(err.Error())
	for _, k := range known {
		if errors.Is(err, k.err) {
			return &UIError{Kind: k.kind, Message: k.msg, Details: details}
		}
	}
	var pe *provider.Error
	if errors.As(err, &pe) {
		return &UIError{Kind: KindProvider, Message: redact(pe.Error()), Details: details}
	}
	var le *levelError
	if errors.As(err, &le) {
		return &UIError{Kind: KindInvalid, Message: le.Error(), Details: details}
	}
	log.Error("app: call failed", "call", what, "err", details)
	return &UIError{Kind: KindInternal, Message: "Something went wrong.", Details: details}
}

// levelError is a bad approval level from the frontend.
type levelError struct{ level string }

func (e *levelError) Error() string {
	return fmt.Sprintf("%q is not an approval level; use strict, standard or auto", e.level)
}

// guard ends every service method (Q26): it recovers a panic, logs it with
// its stack, and turns *err into a *UIError.
func (b *base) guard(what string, err *error) {
	if r := recover(); r != nil {
		b.log.Error("app: panic", "call", what, "panic", r, "stack", string(debug.Stack()))
		*err = &UIError{Kind: KindInternal, Message: "Something went wrong.", Details: b.redact(fmt.Sprint(r))}
		return
	}
	if *err != nil {
		*err = toUI(*err, b.log, b.redact, what)
	}
}
