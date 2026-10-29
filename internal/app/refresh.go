package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
)

// Refresh memory from the chat's header (SPEC 3.5): the estimate for the
// dialog before a long refresh, the start, and the undo of a batch from
// its chip. Stop cancels a refresh, as it stops a turn.

// RefreshEstimate is what a refresh of the chat would cost now.
func (s *ChatService) RefreshEstimate(ctx context.Context, p id.Project, c id.Chat, full bool) (e agent.RefreshEstimate, err error) {
	defer s.guard("chat.refresh_estimate", &err)
	return s.orch.EstimateRefresh(ctx, p, c, agent.RefreshOptions{Full: full})
}

// RefreshMemory starts a refresh in the background. The chat's status
// shows its progress; its chip, or a notice, shows its end.
func (s *ChatService) RefreshMemory(ctx context.Context, p id.Project, c id.Chat, full bool) (err error) {
	defer s.guard("chat.refresh_memory", &err)
	return s.orch.Refresh(ctx, p, c, agent.RefreshOptions{Full: full})
}

// RefreshUndoRequest is the [undo] of a chip "Memory refreshed: …", or its
// redo: the chip's notice's Edits.
type RefreshUndoRequest struct {
	Edits []chat.MemoryDiff `json:"edits"`
	Redo  bool              `json:"redo"`
}

// batch is a refresh's edits by memory: project change IDs and user
// memory entries.
type batch struct {
	changes []id.Change
	entries []int64
}

func batchOf(edits []chat.MemoryDiff) (batch, error) {
	var b batch
	for _, e := range edits {
		switch {
		case strings.HasPrefix(e.Target, store.MemoryTarget(store.ScopeUser, "")):
			n, err := strconv.ParseInt(e.Change, 10, 64)
			if err != nil {
				return b, fmt.Errorf("app: %q is not a user memory change: %w", e.Change, err)
			}
			b.entries = append(b.entries, n)
		case strings.HasPrefix(e.Target, store.MemoryTarget(store.ScopeProject, "")) && e.Change != "":
			b.changes = append(b.changes, id.Change(e.Change))
		default:
			return b, fmt.Errorf("app: %q is not a memory edit", e.Target)
		}
	}
	if len(b.changes)+len(b.entries) == 0 {
		return b, store.ErrNothingToUndo
	}
	return b, nil
}

// undone reports whether every edit of the batch is undone.
func (b batch) undone(ctx context.Context, proj *project.Project) (bool, error) {
	for _, c := range b.changes {
		if u, err := proj.DB.ChangeUndone(ctx, c); err != nil || !u {
			return false, err
		}
	}
	for _, n := range b.entries {
		if u, err := proj.Registry.UserMemoryChangeUndone(ctx, n); err != nil || !u {
			return false, err
		}
	}
	return true, nil
}

// RefreshUndone reports whether a refresh chip's batch is undone now.
func (s *ChatService) RefreshUndone(ctx context.Context, p id.Project, edits []chat.MemoryDiff) (undone bool, err error) {
	defer s.guard("chat.refresh_undone", &err)
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		b, err := batchOf(edits)
		if err != nil {
			return err
		}
		undone, err = b.undone(ctx, proj)
		return err
	})
	return undone, err
}

// RefreshUndo says what UndoRefresh did. With Changed it did nothing:
// Sections names the sections that changed since.
type RefreshUndo struct {
	Done     bool     `json:"done"`
	Changed  bool     `json:"changed"`
	Undone   bool     `json:"undone"` // every edit of the batch is undone now
	Sections []string `json:"sections,omitempty"`
}

// UndoRefresh reverts a refresh's batch from its chip, or puts it back,
// all or nothing: both memories are checked first, as Undo turn does, and
// when a section changed since, nothing changes and Sections names it.
// Project memory and user memory then commit apart, so a crash between
// them leaves one half; the same again does the other. A half that is
// already as asked is left out. The chats see the change as the user's
// (SPEC 3.3).
func (s *ChatService) UndoRefresh(ctx context.Context, p id.Project, r RefreshUndoRequest) (res RefreshUndo, err error) {
	defer s.guard("chat.undo_refresh", &err)
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		b, err := batchOf(r.Edits)
		if err != nil {
			return err
		}
		o := store.UndoOptions{Redo: r.Redo}
		check := o
		check.Check = true
		var conflicts []store.UndoConflict
		if len(b.changes) > 0 {
			x, err := proj.DB.UndoChanges(ctx, b.changes, check)
			switch {
			case errors.Is(err, store.ErrUndoConflict):
				conflicts = append(conflicts, x.Conflicts...)
			case errors.Is(err, store.ErrNothingToUndo):
				b.changes = nil
			case err != nil:
				return err
			}
		}
		if len(b.entries) > 0 {
			x, err := proj.Registry.UndoUserMemoryChanges(ctx, b.entries, proj.ID, check)
			switch {
			case errors.Is(err, store.ErrUndoConflict):
				conflicts = append(conflicts, x.Conflicts...)
			case errors.Is(err, store.ErrNothingToUndo):
				b.entries = nil
			case err != nil:
				return err
			}
		}
		if len(conflicts) == 0 {
			if len(b.changes) > 0 {
				x, err := proj.DB.UndoChanges(ctx, b.changes, o)
				if err != nil && !errors.Is(err, store.ErrUndoConflict) {
					return err
				}
				conflicts = append(conflicts, x.Conflicts...)
				res.Done = err == nil
			}
			// A conflict here came after the check: project memory is
			// done, user memory isn't, and Sections says why.
			if len(b.entries) > 0 && len(conflicts) == 0 {
				x, err := proj.Registry.UndoUserMemoryChanges(ctx, b.entries, proj.ID, o)
				if err != nil && !errors.Is(err, store.ErrUndoConflict) {
					return err
				}
				conflicts = append(conflicts, x.Conflicts...)
				res.Done = res.Done || err == nil
			}
		}
		for _, c := range conflicts {
			if n := sectionName(c.Item); !slices.Contains(res.Sections, n) {
				res.Sections = append(res.Sections, n)
			}
		}
		res.Changed = len(conflicts) > 0
		res.Undone, err = b.undone(ctx, proj)
		return err
	})
	return res, err
}

// sectionName is "user memory, section preferences" for an undo's item.
func sectionName(it store.UndoItem) string {
	if it.Table == store.UserMemoryItem {
		return "user memory, section " + it.Key
	}
	if scope, sec, ok := strings.Cut(strings.TrimPrefix(it.Key, "memory:"), "/"); ok {
		return scope + " memory, section " + sec
	}
	return it.Key
}
