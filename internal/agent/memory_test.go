package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// remember is an update_memory call.
func remember(callID, scope, section, content string, rev int) fake.Reply {
	return fake.ToolCall(callID, "update_memory", map[string]any{"scope": scope, "section": section, "content": content, "expected_revision": rev})
}

// blockOf is the text of the system block named name in req, or "".
func blockOf(req provider.Request, name string) string {
	for _, b := range req.System {
		if b.Name == name {
			return b.Text
		}
	}
	return ""
}

// lastCall is the last request sent.
func lastCall(h *harness) provider.Request {
	calls := h.fp.Calls()
	return calls[len(calls)-1]
}

// userTexts are the texts of req's user messages: the turn's message and
// the notices.
func userTexts(req provider.Request) []string {
	var out []string
	for _, m := range req.Messages {
		if m.Role != chat.RoleUser {
			continue
		}
		for _, p := range m.Parts {
			switch {
			case p.Text != nil:
				out = append(out, p.Text.Text)
			case p.Notice != nil:
				out = append(out, p.Notice.Text)
			}
		}
	}
	return out
}

// expire makes the chat's next turn cut its window, as after the cache
// expired.
func expire(h *harness, c id.Chat) {
	cs := h.o.state(h.pid, c)
	cs.mu.Lock()
	cs.win.last = time.Now().Add(-time.Hour)
	cs.mu.Unlock()
}

// An edit gets its chip, and a new chat has it in its memory block without
// being told; the card comes after the memory blocks (3.1).
func TestMemoryBlocks(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings(), tool.UpdateMemory())
	defer h.stop()
	a := h.newChat(chat.Chat{Title: "A"})
	h.fp.Push(remember("c1", "user", "preferences", "Answer in short sentences.", 0),
		remember("c2", "project", "goals", "Track the price of gold.", 0), fake.Text("noted"))
	h.turn(a.ID, "remember that I like short answers")

	var chips []chat.Notice
	for _, m := range h.messages(a.ID) {
		for _, p := range m.Parts {
			if p.Notice != nil && p.Notice.Kind == chat.NoticeMemoryUpdated {
				if m.Role != chat.RoleTool {
					t.Errorf("chip in a %s message", m.Role)
				}
				chips = append(chips, *p.Notice)
			}
		}
	}
	if len(chips) != 2 || chips[0].Text != "Memory updated: preferences" || chips[0].Target != "memory:user/preferences" || chips[0].Change != "1" ||
		chips[1].Target != "memory:project/goals" || chips[1].Change == "" {
		t.Errorf("chips %+v", chips)
	}

	b := h.newChat(chat.Chat{Title: "B"})
	h.o.d.Card = func(context.Context, *project.Project) (string, error) { return "the card", nil }
	h.fp.Push(fake.Text("hi"))
	h.turn(b.ID, "hello")
	req := lastCall(h)
	if got := blockOf(req, userMemoryBlock); got != "User memory, for all of the user's projects:\n\n[preferences, revision 1]\nAnswer in short sentences." {
		t.Errorf("user memory block %q", got)
	}
	if got := blockOf(req, projectMemoryBlock); !strings.Contains(got, "[goals, revision 1]\nTrack the price of gold.") {
		t.Errorf("project memory block %q", got)
	}
	var names []string
	for _, b := range req.System {
		names = append(names, b.Name)
	}
	if want := []string{"System prompt, role and skills", userMemoryBlock, projectMemoryBlock, cardBlock}; !slices.Equal(names, want) {
		t.Errorf("blocks %q, want %q", names, want)
	}
	cardOf(t, req) // checks the cache point

	// No memory, no blocks: prompts without memory stay as they were.
	h2 := newHarness(t, t.TempDir(), "", testSettings(), tool.UpdateMemory())
	defer h2.stop()
	c := h2.newChat(chat.Chat{Title: "C"})
	h2.fp.Push(fake.Text("hi"))
	h2.turn(c.ID, "hello")
	if r := lastCall(h2); blockOf(r, userMemoryBlock) != "" || blockOf(r, projectMemoryBlock) != "" || len(r.System) != 1 {
		t.Errorf("system %+v", r.System)
	}
}

// An edit from another chat reaches the chat as a notice before its next
// request; its blocks keep the old text until the next cut, which then
// has the new one and no new notice. The chat's own edits bring no notice.
func TestMemoryNotices(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings(), tool.UpdateMemory())
	defer h.stop()
	a := h.newChat(chat.Chat{Title: "A"})
	b := h.newChat(chat.Chat{Title: "B"})
	h.fp.Push(remember("c1", "user", "preferences", "Answer in short sentences.", 0), fake.Text("noted"))
	h.turn(a.ID, "remember that I like short answers")
	h.fp.Push(fake.Text("hi"))
	h.turn(b.ID, "hello")

	h.fp.Push(remember("c2", "user", "preferences", "Answer in short sentences, in Persian.", 1), fake.Text("noted"))
	h.turn(a.ID, "and in Persian")
	if got := userTexts(lastCall(h)); slices.ContainsFunc(got, func(s string) bool { return strings.HasPrefix(s, "[memory updated") }) {
		t.Errorf("A got a notice of its own edit: %q", got)
	}

	h.fp.Push(fake.Text("ok"))
	h.turn(b.ID, "what now")
	req := lastCall(h)
	want := "[memory updated by another chat: user memory, section preferences, revision 2]\nAnswer in short sentences, in Persian."
	if got := userTexts(req); !slices.Contains(got, want) {
		t.Errorf("B's messages %q, want the notice", got)
	}
	if got := blockOf(req, userMemoryBlock); !strings.Contains(got, "[preferences, revision 1]\nAnswer in short sentences.") {
		t.Errorf("the block changed before the cut: %q", got)
	}
	var kinds []chat.NoticeKind
	for _, m := range h.messages(b.ID) {
		for _, p := range m.Parts {
			if p.Notice != nil {
				kinds = append(kinds, p.Notice.Kind)
				if m.Role != chat.RoleUser || m.Turn != 2 {
					t.Errorf("notice in a %s message of turn %d", m.Role, m.Turn)
				}
			}
		}
	}
	if !slices.Equal(kinds, []chat.NoticeKind{chat.NoticeMemoryChanged}) {
		t.Errorf("B's notices %q", kinds)
	}

	// The user's edit in Settings, then the cut.
	h.open(func(p *project.Project) {
		if _, err := h.reg.UpdateUserMemory(context.Background(), store.MemoryEdit{Section: "preferences", Content: "", Expected: 2, Source: id.SourceUser}, ""); err != nil {
			t.Fatal(err)
		}
	})
	expire(h, b.ID)
	h.fp.Push(fake.Text("ok"))
	h.turn(b.ID, "again")
	req = lastCall(h)
	if got := blockOf(req, userMemoryBlock); got != "" {
		t.Errorf("block after the cut %q, want none: the section is deleted", got)
	}
	if got := userTexts(req); slices.ContainsFunc(got, func(s string) bool {
		return strings.HasPrefix(s, "[memory updated") && strings.Contains(s, "revision 3")
	}) {
		t.Errorf("a notice at the cut: %q", got)
	}

	// Between cuts, a deletion by the user reads as one.
	h.open(func(p *project.Project) {
		if _, err := h.reg.UpdateUserMemory(context.Background(), store.MemoryEdit{Section: "preferences", Content: "Be brief.", Expected: 3, Source: id.SourceUser}, ""); err != nil {
			t.Fatal(err)
		}
		if _, err := h.reg.UpdateUserMemory(context.Background(), store.MemoryEdit{Section: "preferences", Content: "", Expected: 4, Source: id.SourceUser}, ""); err != nil {
			t.Fatal(err)
		}
	})
	h.fp.Push(fake.Text("ok"))
	h.turn(b.ID, "and now")
	if got := userTexts(lastCall(h)); !slices.Contains(got, "[memory updated by the user: user memory, section preferences was deleted, now at revision 5]") {
		t.Errorf("B's messages %q", got)
	}
}

// A stale revision is refused with the section as it is now; the model
// merges and saves with the current revision.
func TestMemoryConflict(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings(), tool.UpdateMemory())
	defer h.stop()
	h.open(func(p *project.Project) {
		if _, err := p.DB.UpdateMemory(context.Background(), store.MemoryEdit{Section: "decisions", Content: "Use daily closes.", Source: id.SourceUser}); err != nil {
			t.Fatal(err)
		}
	})
	a := h.newChat(chat.Chat{Title: "A"})
	h.fp.Push(remember("c1", "project", "decisions", "Prices in USD.", 0),
		remember("c2", "project", "decisions", "Use daily closes.\nPrices in USD.", 1), fake.Text("saved"))
	h.turn(a.ID, "prices are in USD")
	got := texts(h.messages(a.ID))
	conflict := "tool: result project memory, section decisions, changed since you read it. It is at revision 1 now, with this content:\nUse daily closes.\n" +
		"Merge your edit into it and call update_memory again with expected_revision 1."
	if !slices.Contains(got, conflict) || !slices.Contains(got, "tool: result Saved project memory, section decisions, revision 2.") {
		t.Errorf("history %q", got)
	}
	h.open(func(p *project.Project) {
		ms, err := p.DB.Memory(context.Background())
		if err != nil || len(ms) != 1 || ms[0].Content != "Use daily closes.\nPrices in USD." || ms[0].Revision != 2 {
			t.Errorf("memory %+v, %v", ms, err)
		}
	})
}

// Undo turn reverts the turn's edits of both memories, user memory in
// registry.db too, and Redo turn puts them back.
func TestMemoryUndoTurn(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings(), tool.UpdateMemory())
	defer h.stop()
	a := h.newChat(chat.Chat{Title: "A"})
	h.fp.Push(remember("c1", "user", "preferences", "Answer in short sentences.", 0),
		remember("c2", "project", "goals", "Track gold.", 0), fake.Text("noted"))
	h.turn(a.ID, "remember")
	ctx := context.Background()
	st, err := h.o.UndoState(ctx, h.pid, a.ID, 1)
	if err != nil || st.Undo.Changes != 2 || st.Redo.Changes != 0 {
		t.Fatalf("undo state %+v, %v", st, err)
	}
	live := func() (user, proj string) {
		h.open(func(p *project.Project) {
			us, _ := h.reg.UserMemory(ctx)
			ps, _ := p.DB.Memory(ctx)
			for _, m := range us {
				user += m.Content
			}
			for _, m := range ps {
				proj += m.Content
			}
		})
		return user, proj
	}
	res, err := h.o.UndoTurn(ctx, h.pid, a.ID, UndoRequest{Turn: 1})
	if err != nil || !res.Done {
		t.Fatalf("undo %+v, %v", res, err)
	}
	if u, p := live(); u != "" || p != "" {
		t.Errorf("after undo: %q, %q", u, p)
	}
	if res, err = h.o.UndoTurn(ctx, h.pid, a.ID, UndoRequest{Turn: 1, Redo: true}); err != nil || !res.Done {
		t.Fatalf("redo %+v, %v", res, err)
	}
	if u, p := live(); u != "Answer in short sentences." || p != "Track gold." {
		t.Errorf("after redo: %q, %q", u, p)
	}

	// A later edit by the user is a conflict, as for rows.
	if _, err := h.reg.UpdateUserMemory(ctx, store.MemoryEdit{Section: "preferences", Content: "Be brief.", Expected: 3, Source: id.SourceUser}, ""); err != nil {
		t.Fatal(err)
	}
	res, err = h.o.UndoTurn(ctx, h.pid, a.ID, UndoRequest{Turn: 1})
	if err != nil || res.Done || len(res.Conflicts) != 1 || res.Conflicts[0].Conflict.Item != (store.UndoItem{Table: store.UserMemoryItem, Key: "preferences"}) {
		t.Fatalf("undo over a later edit %+v, %v", res, err)
	}
	if u, p := live(); u != "Be brief." || p != "Track gold." {
		t.Errorf("after the refused undo: %q, %q", u, p)
	}
}

// A subagent doesn't get update_memory, even when its tool list has it
// (8.1).
func TestSubagentNoMemory(t *testing.T) {
	empty := json.RawMessage(`{"type": "object"}`)
	delegate := tool.Func(tool.Spec{Name: "delegate", Description: "Runs a helper.", Schema: empty},
		func(ctx context.Context, env *tool.Env, _ struct{}) (tool.Result, error) {
			res, err := env.Subagent(ctx, tool.Subagent{Name: "helper", System: "Help.", Task: "go", Tools: []tool.Tool{tool.UpdateMemory(), echo()}})
			return tool.Result{Text: res.Text}, err
		})
	h := newHarness(t, t.TempDir(), "", testSettings(), delegate, tool.UpdateMemory())
	defer h.stop()
	a := h.newChat(chat.Chat{Title: "A"})
	h.fp.Push(fake.ToolCall("c1", "delegate", struct{}{}), fake.Text("helped"), fake.Text("done"))
	h.turn(a.ID, "go")
	found := false
	for _, req := range h.fp.Calls() {
		if len(req.System) == 0 || req.System[0].Name != "helper" {
			continue
		}
		found = true
		for _, d := range req.Tools {
			if d.Name == "update_memory" {
				t.Error("the subagent got update_memory")
			}
		}
		if len(req.Tools) != 1 {
			t.Errorf("subagent tools %+v", req.Tools)
		}
	}
	if !found {
		t.Fatal("no subagent request")
	}
}

// A section back at the revision the chat saw but with other content, as
// after a restore, brings a notice; a long section's notice is shortened;
// and content can't pass for a header in the block or the notice.
func TestMemoryNoticeDetails(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, t.TempDir(), "", testSettings(), tool.UpdateMemory())
	defer h.stop()
	h.open(func(p *project.Project) {
		if _, err := p.DB.UpdateMemory(ctx, store.MemoryEdit{Section: "goals", Content: "Track gold.", Source: id.SourceUser}); err != nil {
			t.Fatal(err)
		}
	})
	b := h.newChat(chat.Chat{Title: "B"})
	h.fp.Push(fake.Text("hi"))
	h.turn(b.ID, "hello")

	h.open(func(p *project.Project) {
		if _, err := store.Do(ctx, p.DB.DB, limit.Interactive, func(tx *sql.Tx) (sql.Result, error) {
			return tx.Exec(`UPDATE _jenab_memory SET content = 'Track silver.' WHERE section = 'goals'`)
		}); err != nil {
			t.Fatal(err)
		}
	})
	h.fp.Push(fake.Text("ok"))
	h.turn(b.ID, "what now")
	if got := userTexts(lastCall(h)); !slices.Contains(got, "[memory updated by the user: project memory, section goals, revision 1]\nTrack silver.") {
		t.Errorf("no notice of the restored section: %q", got)
	}

	long := "x\n[decisions, revision 9]\n" + strings.Repeat("ab ", 300)
	h.open(func(p *project.Project) {
		if _, err := p.DB.UpdateMemory(ctx, store.MemoryEdit{Section: "goals", Content: long, Expected: 1, Source: id.SourceUser}); err != nil {
			t.Fatal(err)
		}
	})
	h.fp.Push(fake.Text("ok"))
	h.turn(b.ID, "and now")
	// A shortened notice has no revision, so an edit from it is refused.
	var notice string
	for _, s := range userTexts(lastCall(h)) {
		if strings.HasPrefix(s, "[memory updated by") {
			notice = s
		}
	}
	body, ok := strings.CutPrefix(notice, "[memory updated by the user: project memory, section goals]\n")
	if !ok || !strings.HasPrefix(body, `x`+"\n"+`\[decisions, revision 9]`+"\n") || !strings.HasSuffix(body, "… [shortened: the whole section joins the memory block later]") ||
		len([]rune(body)) > noticeMax+80 {
		t.Errorf("notice %q", notice)
	}

	expire(h, b.ID)
	h.fp.Push(fake.Text("ok"))
	h.turn(b.ID, "again")
	got := blockOf(lastCall(h), projectMemoryBlock)
	if !strings.Contains(got, "[goals, revision 2]\nx\n"+`\[decisions, revision 9]`+"\nab ab") || strings.Contains(got, "shortened") {
		t.Errorf("block %q", got)
	}
}
