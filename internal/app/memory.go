package app

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
)

// Memory in the settings (SPEC 3.3): user memory in Settings, project
// memory in the project's settings. The user's edits have source user, so
// the chats' notices say the user made them.

// MemoryView is one memory for its settings page.
type MemoryView struct {
	Scope    store.MemoryScope     `json:"scope"`
	Sections []store.MemorySection `json:"sections"` // the live ones, default sections first
	Tokens   int                   `json:"tokens"`
	Cap      int                   `json:"cap"`
}

// MemoryEdit is an edit of one section from the settings. Empty content
// deletes it.
type MemoryEdit struct {
	Section  string `json:"section"`
	Content  string `json:"content"`
	Revision int    `json:"revision"` // the revision the user edited; 0 for a new section
}

// MemorySaved is the memory after an edit. When the section changed since
// the user opened it, nothing is saved: Conflict is the section now, for
// the user to merge.
type MemorySaved struct {
	Memory   MemoryView           `json:"memory"`
	Conflict *store.MemorySection `json:"conflict,omitempty"`
}

func memoryView(scope store.MemoryScope, all []store.MemorySection) MemoryView {
	v := MemoryView{Scope: scope, Sections: []store.MemorySection{}, Tokens: store.MemoryTokens(all), Cap: scope.Cap()}
	for _, m := range all {
		if m.Content != "" {
			v.Sections = append(v.Sections, m)
		}
	}
	store.SortMemory(v.Sections)
	return v
}

func memoryEdit(e MemoryEdit) (store.MemoryEdit, error) {
	if _, err := store.NormalizeSection(e.Section); err != nil {
		return store.MemoryEdit{}, &UIError{Kind: KindInvalid, Message: "A section name is 1 to 40 letters, digits, _ or -, starting and ending with a letter or digit."}
	}
	return store.MemoryEdit{Section: e.Section, Content: strings.TrimSpace(e.Content), Expected: e.Revision, Source: id.SourceUser}, nil
}

// saved reads the memory after an edit, with the conflict if there was one.
func saved(scope store.MemoryScope, err error, read func() ([]store.MemorySection, error)) (MemorySaved, error) {
	var ce *store.MemoryConflictError
	if err != nil && !errors.As(err, &ce) {
		return MemorySaved{}, err
	}
	all, rerr := read()
	if rerr != nil {
		return MemorySaved{}, rerr
	}
	s := MemorySaved{Memory: memoryView(scope, all)}
	if ce != nil {
		s.Conflict = &ce.Current
	}
	return s, nil
}

// UserMemory is user memory, for Settings.
func (s *SettingsService) UserMemory(ctx context.Context) (v MemoryView, err error) {
	defer s.guard("settings.user_memory", &err)
	all, err := s.registry.UserMemory(ctx)
	return memoryView(store.ScopeUser, all), err
}

// SaveUserMemory saves the user's edit of a user memory section.
func (s *SettingsService) SaveUserMemory(ctx context.Context, e MemoryEdit) (r MemorySaved, err error) {
	defer s.guard("settings.save_user_memory", &err)
	me, err := memoryEdit(e)
	if err != nil {
		return r, err
	}
	_, err = s.registry.UpdateUserMemory(ctx, me, "")
	return saved(store.ScopeUser, err, func() ([]store.MemorySection, error) { return s.registry.UserMemory(ctx) })
}

// Memory is the project's memory, for its settings.
func (s *ProjectService) Memory(ctx context.Context, p id.Project) (v MemoryView, err error) {
	defer s.guard("project.memory", &err)
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		all, err := proj.DB.Memory(ctx)
		v = memoryView(store.ScopeProject, all)
		return err
	})
	return v, err
}

// SaveMemory saves the user's edit of a project memory section.
func (s *ProjectService) SaveMemory(ctx context.Context, p id.Project, e MemoryEdit) (r MemorySaved, err error) {
	defer s.guard("project.save_memory", &err)
	me, err := memoryEdit(e)
	if err != nil {
		return r, err
	}
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		_, err := proj.DB.UpdateMemory(ctx, me)
		r, err = saved(store.ScopeProject, err, func() ([]store.MemorySection, error) { return proj.DB.Memory(ctx) })
		return err
	})
	return r, err
}

// MemoryUndoRequest is the [undo] of a chip "Memory updated: …", or its
// redo: the chip's notice's Target and Change.
type MemoryUndoRequest struct {
	Target string `json:"target"` // memory:user/<section> or memory:project/<section>
	Change string `json:"change"`
	Redo   bool   `json:"redo"`
}

// MemoryUndo says what the chip's undo did. When the section changed
// since, nothing changed and Changed says so. Undone is the edit's state
// now, for the chip's button.
type MemoryUndo struct {
	Done    bool `json:"done"`
	Changed bool `json:"changed"`
	Undone  bool `json:"undone"`
}

// memoryChange is the edit a chip stands for: its undo, and whether it is
// undone now.
type memoryChange struct {
	undo   func(o store.UndoOptions) error
	undone func() (bool, error)
}

func memoryChangeOf(ctx context.Context, proj *project.Project, r MemoryUndoRequest) (memoryChange, error) {
	switch {
	case strings.HasPrefix(r.Target, store.MemoryTarget(store.ScopeUser, "")):
		n, err := strconv.ParseInt(r.Change, 10, 64)
		if err != nil {
			return memoryChange{}, fmt.Errorf("app: %q is not a user memory change: %w", r.Change, err)
		}
		return memoryChange{
			undo: func(o store.UndoOptions) error {
				_, err := proj.Registry.UndoUserMemoryChange(ctx, n, proj.ID, o)
				return err
			},
			undone: func() (bool, error) { return proj.Registry.UserMemoryChangeUndone(ctx, n) },
		}, nil
	case strings.HasPrefix(r.Target, store.MemoryTarget(store.ScopeProject, "")):
		c := id.Change(r.Change)
		return memoryChange{
			undo: func(o store.UndoOptions) error {
				_, err := proj.DB.UndoChange(ctx, c, o)
				return err
			},
			undone: func() (bool, error) { return proj.DB.ChangeUndone(ctx, c) },
		}, nil
	}
	return memoryChange{}, fmt.Errorf("app: %q is not a memory target", r.Target)
}

// MemoryUndone reports whether a chip's edit is undone now, so the chip
// offers redo after a reload or an undo of its turn.
func (s *ChatService) MemoryUndone(ctx context.Context, p id.Project, r MemoryUndoRequest) (undone bool, err error) {
	defer s.guard("chat.memory_undone", &err)
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		c, err := memoryChangeOf(ctx, proj, r)
		if err != nil {
			return err
		}
		undone, err = c.undone()
		return err
	})
	return undone, err
}

// UndoMemory reverts one memory edit from its chip, or puts it back. The
// chats see the change as an edit by the user (SPEC 3.3). An edit already
// undone, or already back, is no error: Undone says where it is.
func (s *ChatService) UndoMemory(ctx context.Context, p id.Project, r MemoryUndoRequest) (res MemoryUndo, err error) {
	defer s.guard("chat.undo_memory", &err)
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		c, err := memoryChangeOf(ctx, proj, r)
		if err != nil {
			return err
		}
		err = c.undo(store.UndoOptions{Redo: r.Redo})
		switch {
		case err == nil:
			res = MemoryUndo{Done: true, Undone: !r.Redo}
			return nil
		case errors.Is(err, store.ErrUndoConflict):
			res.Changed = true
		case !errors.Is(err, store.ErrNothingToUndo):
			return err
		}
		res.Undone, err = c.undone()
		return err
	})
	return res, err
}
