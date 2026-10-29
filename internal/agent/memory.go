package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/store"
)

// Memory in the context (SPEC 3.1 blocks 2 and 3, 3.3). The blocks are
// built at a cut and kept until the next one, so the cached prefix holds.
// An edit by another chat, by the user or by an undo reaches the chat
// before its next request as a notice, and joins the block at the next
// cut. The chat's own edits need neither: its tool results have them.

// The memory blocks' names, as the turn inspector shows them.
const (
	userMemoryBlock    = "User memory"
	projectMemoryBlock = "Project memory"
)

// memSeen is a section as the chat has seen it: in its blocks, a notice
// or its own edit.
type memSeen struct {
	rev     int
	content string
}

// memory reads both memories, live sections and tombstones. Without a
// registry, as in some tests, user memory is empty.
func (r *runner) memory(ctx context.Context) ([]store.MemorySection, error) {
	var all []store.MemorySection
	if r.p.Registry != nil {
		u, err := r.p.Registry.UserMemory(ctx)
		if err != nil {
			return nil, err
		}
		all = append(all, u...)
	}
	p, err := r.p.DB.Memory(ctx)
	if err != nil {
		return nil, err
	}
	return append(all, p...), nil
}

// memoryBlocks are blocks 2 and 3, and the sections they show. A memory
// with no sections has no block. Memory is best effort: a failure is
// logged and leaves the blocks out, as a turn can go on without them.
func (r *runner) memoryBlocks(ctx context.Context) ([]provider.Block, map[string]memSeen) {
	seen := map[string]memSeen{}
	all, err := r.memory(ctx)
	if err != nil {
		r.o.d.Log.Error("agent: reading memory", "project", r.cs.key.p, "chat", r.cs.key.c, "err", err)
		return nil, seen
	}
	var blocks []provider.Block
	for _, scope := range []store.MemoryScope{store.ScopeUser, store.ScopeProject} {
		var ss []store.MemorySection
		for _, m := range all {
			if m.Scope == scope {
				seen[m.Target()] = memSeen{m.Revision, m.Content}
				if m.Content != "" {
					ss = append(ss, m)
				}
			}
		}
		if len(ss) == 0 {
			continue
		}
		store.SortMemory(ss)
		var b strings.Builder
		if scope == store.ScopeUser {
			b.WriteString("User memory, for all of the user's projects:")
		} else {
			b.WriteString("Project memory, for every chat in this project:")
		}
		for _, m := range ss {
			fmt.Fprintf(&b, "\n\n[%s, revision %d]\n%s", m.Section, m.Revision, store.QuoteMemory(m.Content))
		}
		name := userMemoryBlock
		if scope == store.ScopeProject {
			name = projectMemoryBlock
		}
		blocks = append(blocks, provider.Block{Text: b.String(), Name: name})
	}
	return blocks, seen
}

// memoryNotices adds a notice to turn t for each section changed since
// the chat last saw it, other than by the chat itself. It is best effort,
// like the blocks.
func (r *runner) memoryNotices(ctx context.Context, t *turn) {
	w := &r.cs.win
	if w.mem == nil {
		return
	}
	all, err := r.memory(ctx)
	if err != nil {
		r.o.d.Log.Error("agent: reading memory", "project", r.cs.key.p, "chat", r.cs.key.c, "err", err)
		return
	}
	var changed []store.MemorySection
	var msgs []id.Message
	for _, m := range all {
		// The content too: a restore can bring a revision back with other content.
		if s, ok := w.mem[m.Target()]; ok && s.rev == m.Revision && s.content == m.Content {
			continue
		}
		changed = append(changed, m)
		if m.Source.Kind() == "message" {
			msgs = append(msgs, id.Message(m.Source.ID()))
		}
	}
	if len(changed) == 0 {
		return
	}
	chats, err := r.p.Chats.MessageChats(ctx, msgs)
	if err != nil {
		r.o.d.Log.Error("agent: reading memory", "project", r.cs.key.p, "chat", r.cs.key.c, "err", err)
		return
	}
	var parts []chat.Part
	for _, m := range changed {
		own := m.Source.Kind() == "message" && chats[id.Message(m.Source.ID())] == t.ch.ID
		if !own {
			n := chat.Notice{Kind: chat.NoticeMemoryChanged, Text: memoryNotice(m), Target: m.Target()}
			parts = append(parts, chat.Part{Kind: chat.PartNotice, Notice: &n})
		}
	}
	if len(parts) > 0 {
		_, err := r.o.write(ctx, r.cs, r.p, chat.Message{Chat: t.ch.ID, Turn: t.n, Role: chat.RoleUser, Parts: parts})
		if err != nil {
			r.o.d.Log.Error("agent: writing a memory notice", "project", r.cs.key.p, "chat", r.cs.key.c, "err", err)
			return
		}
	}
	for _, m := range changed {
		w.mem[m.Target()] = memSeen{m.Revision, m.Content}
	}
}

// memoryNotice is the notice of an edit the chat didn't make. A message
// from a chat not in this project is from a chat of another project, as
// user memory serves them all.
func memoryNotice(m store.MemorySection) string {
	by := "another chat"
	switch m.Source.Kind() {
	case "message":
	case "refresh":
		by = "Refresh memory"
	default:
		by = "the user"
	}
	name := fmt.Sprintf("%s memory, section %s", m.Scope, m.Section)
	if m.Content == "" {
		return fmt.Sprintf("[memory updated by %s: %s was deleted, now at revision %d]", by, name, m.Revision)
	}
	body, short := shortMemory(store.QuoteMemory(m.Content))
	if short {
		// No revision: an edit made from part of the section is refused,
		// and the refusal shows all of it.
		return fmt.Sprintf("[memory updated by %s: %s]\n%s", by, name, body)
	}
	return fmt.Sprintf("[memory updated by %s: %s, revision %d]\n%s", by, name, m.Revision, body)
}

// noticeMax is how much of a section a notice shows, in characters: a
// section can be most of its memory's cap, and the notice goes to every
// other chat. The block has all of it after the next cut.
const noticeMax = 400

func shortMemory(s string) (string, bool) {
	r := []rune(s)
	if len(r) <= noticeMax {
		return s, false
	}
	return string(r[:noticeMax]) + "… [shortened: the whole section joins the memory block later]", true
}

// afterMemory is where the project card goes in the system blocks: after
// block 1 and the memory blocks (3.1).
func afterMemory(sys []provider.Block) int {
	i := 1
	for i < len(sys) && (sys[i].Name == userMemoryBlock || sys[i].Name == projectMemoryBlock) {
		i++
	}
	return min(i, len(sys))
}
