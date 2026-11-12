package agent

import (
	"context"
	"slices"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// offerTools sets the tools the window's requests have until the next
// cut (TASK-033): those whose tool.When the project or the chat has. A
// tool that needs tables joins at the migration that makes the first one
// (refreshTools). Tools left out stay callable: a call to one runs as
// usual. When the project can't be read, every tool is offered.
func (r *runner) offerTools(ctx context.Context, t *turn) {
	w := &r.cs.win
	all := r.o.chatTools(t.ch)
	w.tools, w.toolsAt = all, -1
	v, err := r.p.DB.SchemaVersion(ctx)
	if err != nil {
		r.o.d.Log.Warn("agent: reading the schema version for the tool list", "project", r.cs.key.p, "err", err)
		return
	}
	var have tool.When
	tables, err := r.p.DB.Tables(ctx)
	if err != nil {
		r.o.d.Log.Warn("agent: reading the tables for the tool list", "project", r.cs.key.p, "err", err)
		return
	}
	if len(tables) > 0 {
		have |= tool.WhenTables
	}
	chats, err := r.p.Chats.Chats(ctx)
	if err != nil {
		r.o.d.Log.Warn("agent: reading the chats for the tool list", "project", r.cs.key.p, "err", err)
		return
	}
	for _, c := range chats {
		if c.ID != t.ch.ID && c.Kind != chat.KindMother {
			have |= tool.WhenChats | tool.WhenHistory
		}
	}
	if w.start > 1 {
		have |= tool.WhenHistory
	}
	// Two entries tell: cache/ and another one.
	l, err := r.p.DB.ListObjects(ctx, "", "", 2)
	if err != nil {
		r.o.d.Log.Warn("agent: listing the bucket for the tool list", "project", r.cs.key.p, "err", err)
		return
	}
	if len(l.Objects) > 0 || l.Next != "" || slices.ContainsFunc(l.Folders, func(f string) bool { return f != "cache/" }) {
		have |= tool.WhenBucket
	}
	tools := make([]tool.Tool, 0, len(all)) // not nil: nil is every tool
	for _, tl := range all {
		if when := tl.Spec().When; when == 0 || when&have != 0 {
			tools = append(tools, tl)
		} else if when&tool.WhenTables != 0 {
			w.toolsAt = v
		}
	}
	w.tools = tools
}

// refreshTools offers the tools that wait for a table once a migration
// has run since the list was set, by this chat or another, like the
// project card (3.1).
func (r *runner) refreshTools(ctx context.Context, t *turn) {
	w := &r.cs.win
	if w.tools == nil || w.toolsAt < 0 {
		return
	}
	v, err := r.p.DB.SchemaVersion(ctx)
	if err != nil || v == w.toolsAt {
		return
	}
	r.offerTools(ctx, t)
}

// chatTools is the tools chat ch gets: every tool of its kind, or those
// Deps.ChatTools names (SPIKE-044, spike-only).
func (o *Orchestrator) chatTools(ch chat.Chat) []tool.Tool {
	all := o.d.Tools.For(ch.Kind)
	if o.d.ChatTools == nil {
		return all
	}
	names, _ := o.d.ChatTools(ch)
	if names == nil {
		return all
	}
	return slices.DeleteFunc(all, func(tl tool.Tool) bool { return !slices.Contains(names, tl.Spec().Name) })
}

// refusal is the error for a call to a tool chat ch doesn't have under
// Deps.ChatTools (SPIKE-044, spike-only), or "" when the call may run.
func (o *Orchestrator) refusal(ch chat.Chat, name string) string {
	if o.d.ChatTools == nil {
		return ""
	}
	names, refuse := o.d.ChatTools(ch)
	if names == nil || slices.Contains(names, name) {
		return ""
	}
	if refuse != nil {
		return refuse(name)
	}
	return name + " is not one of this chat's tools"
}
