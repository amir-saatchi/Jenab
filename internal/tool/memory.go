package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/store"
)

// update_memory (SPEC 3.3, 8.1). The content rule is in the description,
// the same for every model (1). Subagents don't get it: they can't change
// memory (8.1), and pipelines and the schema agent have no such tool.

const memoryDescription = `Edit user memory or project memory, one section at a time. Memory is part of every chat's context: user memory in all of the user's projects, project memory in every chat of this project.
- scope "user" is for what applies to all of the user's work, such as how they like answers. scope "project" is for what every chat in this project needs.
- Memory holds intent: goals, preferences, decisions with their reasons, and conventions. Never write values that live in tables, such as prices, counts or lists of tracked items.
- Save what the user asks you to remember, and lasting goals, preferences and decisions they state.
- content replaces the whole section, so keep what should stay. Empty content deletes the section.
- Sections: goals, preferences, decisions, conventions; you may add others.
- expected_revision is the section's revision as memory shows it, 0 for a section that isn't there. If the section changed since, the edit is refused with its current content: merge your edit into it and call again with its revision.
- Caps: user memory 1,000 tokens, project memory 2,000.`

type memoryArgs struct {
	Scope            string `json:"scope"`
	Section          string `json:"section"`
	Content          string `json:"content"`
	ExpectedRevision int    `json:"expected_revision"`
}

// UpdateMemory is the update_memory tool.
func UpdateMemory() Tool {
	return Func(Spec{
		Name:        "update_memory",
		Description: memoryDescription,
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"scope": {"type": "string", "enum": ["user", "project"]},
				"section": {"type": "string", "description": "the section's name, such as preferences"},
				"content": {"type": "string", "description": "the whole new text of the section; empty deletes it"},
				"expected_revision": {"type": "integer", "minimum": 0, "description": "the section's revision as memory shows it; 0 if it isn't there"}
			},
			"required": ["scope", "section", "content", "expected_revision"],
			"additionalProperties": false
		}`),
		Effects: Memory,
	}, runUpdateMemory)
}

func runUpdateMemory(ctx context.Context, env *Env, a memoryArgs) (Result, error) {
	scope := store.MemoryScope(a.Scope)
	e := store.MemoryEdit{Section: a.Section, Content: store.UnquoteMemory(a.Content), Expected: a.ExpectedRevision, Source: env.Source}
	var w store.MemoryWrite
	var err error
	if scope == store.ScopeUser {
		if env.Project.Registry == nil {
			return Result{}, errors.New("tool: user memory needs the registry")
		}
		w, err = env.Project.Registry.UpdateUserMemory(ctx, e, env.Project.ID)
		if err == nil {
			// The project's log points to the edit (SPEC 2.5). It is
			// history only, as undo reads registry.db, so the edit
			// stands without it.
			_ = env.Project.DB.LogUserMemory(context.WithoutCancel(ctx), w)
		}
	} else {
		w, err = env.Project.DB.UpdateMemory(ctx, e)
	}
	if err != nil {
		return Result{}, memoryError(err, scope)
	}
	name := fmt.Sprintf("%s memory, section %s,", scope, w.Section.Section)
	switch w.Op {
	case "":
		if w.Section.Content == "" {
			return Result{Text: fmt.Sprintf("No change: %s is not there.", strings.TrimSuffix(name, ","))}, nil
		}
		return Result{Text: fmt.Sprintf("No change: %s already has this text (revision %d).", name, w.Section.Revision)}, nil
	case "delete":
		return Result{Text: fmt.Sprintf("Deleted %s now at revision %d.", name, w.Section.Revision), Notices: []chat.Notice{memoryChip(w)}}, nil
	}
	return Result{Text: fmt.Sprintf("Saved %s revision %d.", name, w.Section.Revision), Notices: []chat.Notice{memoryChip(w)}}, nil
}

// memoryChip is the chat's chip for an edit: Memory updated: preferences
// [undo] [edit] (SPEC 3.3).
func memoryChip(w store.MemoryWrite) chat.Notice {
	text := "Memory updated: " + w.Section.Section
	if w.Op == "delete" {
		text += " (deleted)"
	}
	return chat.Notice{Kind: chat.NoticeMemoryUpdated, Text: text, Target: w.Section.Target(), Change: w.Change}
}

// memoryError turns what the model can fix into an *Error: a stale
// revision with the section as it is now, the cap, a bad name.
func memoryError(err error, scope store.MemoryScope) error {
	var ce *store.MemoryConflictError
	var cap *store.MemoryCapError
	switch {
	case errors.As(err, &ce):
		c := ce.Current
		if c.Content == "" {
			return &Error{Msg: fmt.Sprintf("%s memory, section %s, was deleted at revision %d since you read it. Call update_memory again with expected_revision %d to write it.",
				scope, c.Section, c.Revision, c.Revision), Err: err}
		}
		return &Error{Msg: fmt.Sprintf("%s memory, section %s, changed since you read it. It is at revision %d now, with this content:\n%s\nMerge your edit into it and call update_memory again with expected_revision %d.",
			scope, c.Section, c.Revision, store.QuoteMemory(c.Content), c.Revision), Err: err}
	case errors.As(err, &cap):
		return &Error{Msg: fmt.Sprintf("not saved: %s memory would be %s tokens, over its cap of %s. Make it shorter: keep intent only, and merge or drop what is no longer needed.",
			scope, count(cap.Tokens), count(cap.Cap)), Err: err}
	case errors.Is(err, store.ErrReadOnly):
		return &Error{Msg: "the project is open read-only after damage, so nothing can be changed; tell the user", Err: err}
	case errors.Is(err, store.ErrInvalid):
		return &Error{Msg: strings.TrimPrefix(err.Error(), "store: invalid write: "), Err: err}
	}
	return err
}
