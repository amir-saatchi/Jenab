package agent

import (
	"context"
	"maps"
	"slices"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// offerTools sets the tools the window's requests have until the next
// cut (TASK-033, TASK-047): those that are on (8.10) and whose tool.When
// the project or the chat has. The list doesn't change between cuts, so
// the cached prefix and the model's thinking hold (3.1), with one
// exception: a tool that needs a table joins at the migration that makes
// the project's first one (joinTableTools). Offered before, query led a
// model to make a table for a note (TASK-047). Tools left out stay
// callable: a call to one runs as usual. When the project can't be read,
// every tool that is on is offered.
func (r *runner) offerTools(ctx context.Context, t *turn) {
	w := &r.cs.win
	w.tools, w.switches, w.tablesAt = r.toolList(ctx, t), r.switches(t), -1
	have := toolNamesOf(w.tools)
	for _, tl := range r.o.d.Tools.For(t.ch.Kind) {
		if s := tl.Spec(); s.When&tool.WhenTables != 0 && s.IsOn() && !slices.Contains(have, s.Name) {
			if v, err := r.p.DB.SchemaVersion(ctx); err == nil {
				w.tablesAt = v
			}
			break
		}
	}
}

// joinTableTools adds the tools that wait for a table once a migration
// has made the project's first one since the cut, by this chat or
// another, like the project card (3.1). It happens once in a project's
// life, so the one change to the tool list is rare.
func (r *runner) joinTableTools(ctx context.Context, t *turn) {
	w := &r.cs.win
	if w.tablesAt < 0 {
		return
	}
	v, err := r.p.DB.SchemaVersion(ctx)
	if err != nil || v == w.tablesAt {
		return
	}
	w.tablesAt = v
	if tables, err := r.p.DB.Tables(ctx); err != nil || len(tables) == 0 {
		return
	}
	var tools []tool.Tool
	have := toolNamesOf(w.tools)
	for _, tl := range r.o.d.Tools.For(t.ch.Kind) {
		if s := tl.Spec(); slices.Contains(have, s.Name) || s.When&tool.WhenTables != 0 && s.IsOn() {
			tools = append(tools, tl)
		}
	}
	w.tools, w.tablesAt = tools, -1
}

// switches are the tools with a Settings switch (8.10), and whether each
// is on now.
func (r *runner) switches(t *turn) map[string]bool {
	var out map[string]bool
	for _, tl := range r.o.d.Tools.For(t.ch.Kind) {
		if s := tl.Spec(); s.On != nil {
			if out == nil {
				out = map[string]bool{}
			}
			out[s.Name] = s.IsOn()
		}
	}
	return out
}

// toolList is the tools a cut would offer now (offerTools).
func (r *runner) toolList(ctx context.Context, t *turn) []tool.Tool {
	all := slices.DeleteFunc(slices.Clone(r.o.d.Tools.For(t.ch.Kind)), func(tl tool.Tool) bool { return !tl.Spec().IsOn() })
	if all == nil {
		all = []tool.Tool{} // not nil: nil is every tool
	}
	var have tool.When
	tables, err := r.p.DB.Tables(ctx)
	if err != nil {
		r.o.d.Log.Warn("agent: reading the tables for the tool list", "project", r.cs.key.p, "err", err)
		return all
	}
	if len(tables) > 0 {
		have |= tool.WhenTables
	}
	chats, err := r.p.Chats.Chats(ctx)
	if err != nil {
		r.o.d.Log.Warn("agent: reading the chats for the tool list", "project", r.cs.key.p, "err", err)
		return all
	}
	for _, c := range chats {
		if c.ID != t.ch.ID && c.Kind != chat.KindMother {
			have |= tool.WhenChats | tool.WhenHistory
		}
	}
	if r.cs.win.start > 1 {
		have |= tool.WhenHistory
	}
	// Two entries tell: cache/ and another one.
	l, err := r.p.DB.ListObjects(ctx, "", "", 2)
	if err != nil {
		r.o.d.Log.Warn("agent: listing the bucket for the tool list", "project", r.cs.key.p, "err", err)
		return all
	}
	if len(l.Objects) > 0 || l.Next != "" || slices.ContainsFunc(l.Folders, func(f string) bool { return f != "cache/" }) {
		have |= tool.WhenBucket
	}
	tools := make([]tool.Tool, 0, len(all))
	for _, tl := range all {
		if when := tl.Spec().When; when == 0 || when&have != 0 {
			tools = append(tools, tl)
		}
	}
	return tools
}

// toolNames are the tools' names, in their order.
func toolNamesOf(ts []tool.Tool) []string {
	out := make([]string, len(ts))
	for i, tl := range ts {
		out[i] = tl.Spec().Name
	}
	return out
}

// switched reports whether a tool was turned on or off (8.10) since the
// window's list was set. The change applies at the next turn, with a cut.
func (r *runner) switched(t *turn) bool {
	return !maps.Equal(r.switches(t), r.cs.win.switches)
}
