package agent

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
	"github.com/amir-saatchi/jenab/internal/store"
)

// addTurns writes turns from to from+n-1 of the chat straight to the
// store: a user message and an answer each.
func (h *harness) addTurns(c id.Chat, from, n int) {
	h.t.Helper()
	h.open(func(p *project.Project) {
		for i := from; i < from+n; i++ {
			for _, m := range []chat.Message{
				{Chat: c, Turn: i, Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: fmt.Sprintf("user line %d", i)}}}},
				{Chat: c, Turn: i, Role: chat.RoleAssistant, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: fmt.Sprintf("answer %d", i)}}}},
			} {
				if _, _, err := p.Chats.AppendMessage(context.Background(), m); err != nil {
					h.t.Fatal(err)
				}
			}
		}
	})
}

func (h *harness) refresh(c id.Chat, full bool) {
	h.t.Helper()
	if err := h.o.Refresh(context.Background(), h.pid, c, RefreshOptions{Full: full}); err != nil {
		h.t.Fatal(err)
	}
}

func mapReply(items ...string) fake.Reply {
	return fake.Text(`{"items": [` + strings.Join(items, ", ") + `]}`)
}

func mapItem(kind, scope, text string) string {
	return fmt.Sprintf(`{"kind": %q, "scope": %q, "text": %q, "source": ["m1"]}`, kind, scope, text)
}

func mergeReply(edits, summary string) fake.Reply {
	return fake.Text(`Here it is:
` + "```json\n" + `{"items": [{"kind": "decision", "text": "Reports are weekly", "scope": "project", "from": ["i1"], "label": "missing"}],
 "edits": [` + edits + `], "summary": ` + strconv.Quote(summary) + `}` + "\n```")
}

// withInput is r with n input tokens in its usage, so a big request
// passes the truncation check that the fake's 10 would fail.
func withInput(r fake.Reply, n int) fake.Reply {
	r.Events = slices.Clone(r.Events)
	r.Events[len(r.Events)-1] = fake.Done(provider.StopEnd, chat.Usage{Input: n, Output: 1})
	return r
}

func edit(scope, section, content string) string {
	return fmt.Sprintf(`{"scope": %q, "section": %q, "content": %q}`, scope, section, content)
}

// texts of the refresh's calls: the first user message of each.
func callTexts(calls []provider.Request) []string {
	var out []string
	for _, c := range calls {
		out = append(out, c.Messages[0].Parts[0].Text.Text)
	}
	return out
}

func refreshChips(ms []chat.Message) []chat.Notice {
	var out []chat.Notice
	for _, m := range ms {
		for _, p := range m.Parts {
			if p.Notice != nil && p.Notice.Kind == chat.NoticeMemoryRefreshed {
				out = append(out, *p.Notice)
			}
		}
	}
	return out
}

func (h *harness) review(c id.Chat) store.ReviewState {
	h.t.Helper()
	var r store.ReviewState
	h.open(func(p *project.Project) {
		var err error
		if r, err = p.Chats.Review(context.Background(), c); err != nil {
			h.t.Fatal(err)
		}
	})
	return r
}

func (h *harness) notes(c id.Chat) chat.SessionNote {
	h.t.Helper()
	var n chat.SessionNote
	h.open(func(p *project.Project) {
		var err error
		if n, err = p.Chats.Notes(context.Background(), c); err != nil {
			h.t.Fatal(err)
		}
	})
	return n
}

// The split: chunks of at most 5 turns and 12,000 tokens, at turn
// boundaries; a turn over the limit alone, and cut.
func TestRefreshSplit(t *testing.T) {
	turn := func(n, size int) reviewTurn {
		return reviewTurn{n: n, msgs: []reviewMsg{{id: id.Message(fmt.Sprintf("m%03d", n)), lines: []string{strings.Repeat("x", size*4)}}}}
	}
	var small []reviewTurn
	for i := 1; i <= 12; i++ {
		small = append(small, turn(i, 10))
	}
	sizes := func(g [][]reviewTurn) []int {
		var out []int
		for _, x := range g {
			out = append(out, len(x))
		}
		return out
	}
	if got := sizes(splitTurns(small)); fmt.Sprint(got) != "[5 5 2]" {
		t.Errorf("12 small turns: %v", got)
	}
	big := []reviewTurn{turn(1, 5000), turn(2, 5000), turn(3, 5000), turn(4, 20000), turn(5, 10)}
	g := splitTurns(big)
	if got := sizes(g); fmt.Sprint(got) != "[2 1 1 1]" {
		t.Fatalf("by tokens: %v", got)
	}
	k := buildChunk(g[2])
	if len(k.text)/4 > chunkTokens+20 || !strings.Contains(k.text, "left out") || k.first != "m004" || k.last != "m004" {
		t.Errorf("a turn over the limit: %d tokens, first %s", len(k.text)/4, k.first)
	}
	k = buildChunk(g[0])
	if k.first != "m001" || k.last != "m002" || k.labels["m2"] != "m002" || !strings.Contains(k.text, "[turn 2]\nm2 ") {
		t.Errorf("chunk %s..%s, labels %v", k.first, k.last, k.labels)
	}
}

// A refresh maps the chunks, merges, applies the edits with the refresh's
// source, saves the summary and the progress, and shows one chip; the
// next turn cuts and reads the new memory and summary. A second refresh
// reads only the new turns.
func TestRefresh(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Reports"})
		h.addTurns(c.ID, 1, 7)

		est, err := h.o.EstimateRefresh(context.Background(), h.pid, c.ID, RefreshOptions{})
		if err != nil || est.Turns != 7 || est.Chunks != 2 || est.Calls != 3 || est.Model != "fast" || est.Long {
			t.Fatalf("estimate %+v, %v", est, err)
		}
		h.fp.Push(mapReply(mapItem("decision", "project", "Reports are weekly")), mapReply(mapItem("progress", "chat", "Set up the report")),
			mergeReply(edit("project", "decisions", "Reports are weekly."), "Set up weekly reports."))
		h.refresh(c.ID, false)
		if err := h.o.Refresh(context.Background(), h.pid, c.ID, RefreshOptions{}); !errors.Is(err, ErrRefreshing) {
			t.Errorf("a second refresh at once: %v", err)
		}
		h.wait()

		calls := h.fp.Calls()
		if len(calls) != 3 {
			t.Fatalf("%d calls", len(calls))
		}
		for _, c := range calls {
			if !strings.Contains(c.Model, "small") || c.Thinking {
				t.Errorf("a call to %s, thinking %v", c.Model, c.Thinking)
			}
		}
		ts := callTexts(calls)
		maps := ts[0] + ts[1]
		// The second chunk has turn 5 as context; the first has none.
		if !strings.Contains(maps, "[turn 1]\nm1 user: user line 1") || !strings.Contains(maps, "[turn 7]") ||
			!strings.Contains(maps, "for context only:\n\n[turn 5]\nuser: user line 5\n") || strings.Count(maps, "for context only") != 1 {
			t.Errorf("map calls %q", ts[:2])
		}
		if !strings.Contains(ts[2], "Project memory now:\n(empty)") || !strings.Contains(ts[2], "i1 (") {
			t.Errorf("merge call %q", ts[2])
		}
		got, rev := h.section(store.ScopeProject, "decisions")
		if got != "Reports are weekly." || rev != 1 {
			t.Fatalf("decisions %q at %d", got, rev)
		}
		var src id.Source
		h.open(func(p *project.Project) {
			ss, _ := p.DB.Memory(context.Background())
			src = ss[0].Source
		})
		if src != id.SourceOf("refresh", string(c.ID)) {
			t.Errorf("source %q", src)
		}
		if n := h.notes(c.ID); n.Summary != "Set up weekly reports." {
			t.Errorf("notes %+v", n)
		}
		ms := h.messages(c.ID)
		chips := refreshChips(ms)
		if len(chips) != 1 || chips[0].Text != "Memory refreshed: 1 added" || len(chips[0].Edits) != 1 || chips[0].Edits[0].Change == "" ||
			chips[0].Edits[0].Target != "memory:project/decisions" {
			t.Fatalf("chips %+v", chips)
		}
		r := h.review(c.ID)
		if r.UpToTurn != 7 || len(r.Chunks) != 0 {
			t.Errorf("progress %+v", r)
		}
		if st := h.o.State(h.pid, c.ID); st != chat.StateIdle {
			t.Errorf("state %s", st)
		}

		// The next turn cuts: memory and the summary are in its blocks.
		h.turn(c.ID, "next")
		last := h.fp.Calls()[3]
		var sys strings.Builder
		for _, b := range last.System {
			sys.WriteString(b.Text + "\n")
		}
		if !strings.Contains(sys.String(), "Reports are weekly.") || !strings.Contains(sys.String(), "Set up weekly reports.") {
			t.Errorf("the next turn's blocks:\n%s", sys.String())
		}
		for _, m := range last.Messages {
			if m.Turn < 8-h.set.Context.HistoryMinTurns {
				t.Errorf("turn %d is still in the window", m.Turn)
				break
			}
		}

		// A second refresh reads only the turns after the first: 8 to 10.
		h.addTurns(c.ID, 9, 2)
		n := len(h.fp.Calls())
		h.fp.Push(mapReply(), fake.Text("unused"))
		h.refresh(c.ID, false)
		h.wait()
		calls = h.fp.Calls()[n:]
		if len(calls) != 1 { // no items: no merge
			t.Fatalf("%d calls for the new turns: %q", len(calls), callTexts(calls))
		}
		// Turn 7, the last one reviewed, comes first as context, unlabelled.
		before, part, _ := strings.Cut(callTexts(calls)[0], "Conversation part:")
		if !strings.Contains(before, "for context only:\n\n[turn 7]\nuser: user line 7\nassistant: answer 7\n") || strings.Contains(part, "user line 7") ||
			!strings.Contains(part, "[turn 8]\nm1 user: next") || !strings.Contains(part, "user line 10") {
			t.Errorf("the second refresh read %q", callTexts(calls)[0])
		}
		if chips := refreshChips(h.messages(c.ID)); chips[len(chips)-1].Text != "Memory refreshed: nothing new" {
			t.Errorf("chip %+v", chips[len(chips)-1])
		}
		if err := h.o.Refresh(context.Background(), h.pid, c.ID, RefreshOptions{}); !errors.Is(err, ErrNothingToRefresh) {
			t.Errorf("a refresh with nothing new: %v", err)
		}

		// Full refresh reads the whole chat again.
		n = len(h.fp.Calls())
		h.fp.Push(mapReply(), mapReply(), mapReply())
		h.refresh(c.ID, true)
		h.wait()
		if got := len(h.fp.Calls()) - n; got != 3 {
			t.Errorf("a full refresh made %d calls", got)
		}
	})
}

// At Strict the batch waits for its card; Save applies it.
func TestRefreshStrict(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		h.open(func(p *project.Project) {
			if err := p.SetLevel(context.Background(), project.Strict); err != nil {
				t.Fatal(err)
			}
		})
		c := h.newChat(chat.Chat{Title: "Strict"})
		h.addTurns(c.ID, 1, 2)
		h.fp.Push(mapReply(mapItem("preference", "user", "Short answers")),
			mergeReply(edit("user", "preferences", "Short answers.")+", "+edit("project", "goals", "Weekly reports."), "Talked."))
		h.refresh(c.ID, false)
		w := h.waiting(c.ID)
		if w.Kind != chat.PartApproval || w.Text != "Save Refresh memory's edits to 2 sections?" {
			t.Fatalf("waiting %+v", w)
		}
		if got, _ := h.section(store.ScopeUser, "preferences"); got != "" {
			t.Fatalf("applied before the answer: %q", got)
		}
		if err := h.o.Clear(context.Background(), h.pid, c.ID); !errors.Is(err, ErrInTurn) {
			t.Errorf("Clear during a refresh: %v", err)
		}
		card := cards(h.messages(c.ID))
		if len(card) != 1 || len(card[0].Edits) != 2 || card[0].Edits[0].After != "Short answers." {
			t.Fatalf("card %+v", card)
		}
		h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantOnce})
		h.wait()
		if got, _ := h.section(store.ScopeUser, "preferences"); got != "Short answers." {
			t.Errorf("preferences %q", got)
		}
		if got, _ := h.section(store.ScopeProject, "goals"); got != "Weekly reports." {
			t.Errorf("goals %q", got)
		}
		if card := cards(h.messages(c.ID)); card[0].Answer != chat.GrantOnce {
			t.Errorf("card %+v", card[0])
		}

		// Undo of the batch, by the chip's changes, reverts both.
		chip := refreshChips(h.messages(c.ID))[0]
		var changes []id.Change
		var entries []int64
		for _, e := range chip.Edits {
			if strings.HasPrefix(e.Target, "memory:user/") {
				n, _ := strconv.ParseInt(e.Change, 10, 64)
				entries = append(entries, n)
			} else {
				changes = append(changes, id.Change(e.Change))
			}
		}
		h.open(func(p *project.Project) {
			if _, err := p.DB.UndoChanges(context.Background(), changes, store.UndoOptions{}); err != nil {
				t.Fatal(err)
			}
		})
		if _, err := h.reg.UndoUserMemoryChanges(context.Background(), entries, h.pid, store.UndoOptions{}); err != nil {
			t.Fatal(err)
		}
		if got, _ := h.section(store.ScopeUser, "preferences"); got != "" {
			t.Errorf("preferences after undo %q", got)
		}
		if got, _ := h.section(store.ScopeProject, "goals"); got != "" {
			t.Errorf("goals after undo %q", got)
		}
	})
}

// At Standard, an edit of a section the user wrote last asks; a message
// sent while the card waits denies it: memory stays, the summary is
// saved, the progress moves on, and the message's turn runs after.
func TestRefreshUserSection(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		h.open(func(p *project.Project) {
			if _, err := p.DB.UpdateMemory(context.Background(), store.MemoryEdit{Section: "goals", Content: "Mine.", Source: id.SourceUser}); err != nil {
				t.Fatal(err)
			}
		})
		c := h.newChat(chat.Chat{Title: "User's"})
		h.addTurns(c.ID, 1, 2)
		h.fp.Push(mapReply(mapItem("goal", "project", "Monthly")), mergeReply(edit("project", "goals", "Monthly."), "Changed goals."))
		h.refresh(c.ID, false)
		w := h.waiting(c.ID)
		if w.Text != "Save Refresh memory's edit to project memory, section goals?" {
			t.Fatalf("waiting %+v", w)
		}
		h.fp.Push(fake.Text("hello"))
		h.send(c.ID, "never mind")
		h.wait()
		if got, _ := h.section(store.ScopeProject, "goals"); got != "Mine." {
			t.Errorf("goals %q", got)
		}
		if n := h.notes(c.ID); n.Summary != "Changed goals." {
			t.Errorf("summary %q", n.Summary)
		}
		if r := h.review(c.ID); r.UpToTurn != 2 {
			t.Errorf("progress %+v", r)
		}
		ms := h.messages(c.ID)
		chips := refreshChips(ms)
		if len(chips) != 1 || chips[0].Text != "Memory refresh: the edits were not saved" || len(chips[0].Edits) != 0 {
			t.Errorf("chips %+v", chips)
		}
		if card := cards(ms); len(card) != 1 || card[0].Answer != chat.GrantDeny || card[0].Note != "never mind" {
			t.Errorf("card %+v", card)
		}
		if tx := texts(ms); tx[len(tx)-1] != "assistant: hello" {
			t.Errorf("the queued turn didn't run: %q", tx)
		}
		// The card never reaches the model.
		for _, m := range h.fp.Calls()[2].Messages {
			for _, p := range m.Parts {
				if p.Approval != nil {
					t.Error("the card is in the request")
				}
			}
		}
	})
}

// A refresh's card, like its chip, sits in the last turn after it ended:
// a failed turn can still be retried after a restart.
func TestRefreshAfterFailedTurn(t *testing.T) {
	root := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		h := newHarness(t, root, "", testSettings())
		c := h.newChat(chat.Chat{Title: "Failed"})
		h.addTurns(c.ID, 1, 2)
		h.fp.Push(fake.Fail(&provider.Error{Kind: provider.BadRequest, Provider: "p", Status: 401, Message: "bad key"}))
		h.turn(c.ID, "price?")
		h.setLevel(project.Strict)
		h.fp.Push(mapReply(mapItem("goal", "project", "Weekly")), mergeReply(edit("project", "goals", "Weekly."), ""))
		h.refresh(c.ID, false)
		w := h.waiting(c.ID)
		h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantDeny})
		h.wait()
		if card := cards(h.messages(c.ID)); len(card) != 1 || card[0].Answer != chat.GrantDeny {
			t.Fatalf("card %+v", card)
		}
		pid := h.pid
		h.stop()

		h = newHarness(t, root, pid, testSettings())
		defer h.stop()
		h.fp.Push(fake.Text("42"))
		if err := h.o.Retry(ctx, h.pid, c.ID); err != nil {
			t.Fatalf("Retry after a refresh and a restart: %v", err)
		}
		h.wait()
	})
}

// Stop before the merge ends cancels the refresh: nothing is applied and
// the progress stays, but a finished chunk is kept, so the next refresh
// doesn't ask for it again.
func TestRefreshCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Cancel"})
		h.addTurns(c.ID, 1, 7)
		h.fp.Push(mapReply(mapItem("goal", "project", "Weekly")), fake.Reply{Hang: true})
		h.refresh(c.ID, false)
		synctest.Wait()
		if st := h.o.State(h.pid, c.ID); st != chat.StateWorking {
			t.Errorf("state %s", st)
		}
		h.o.Stop(h.pid, c.ID)
		h.wait()
		calls := h.fp.Calls()
		if len(calls) != 2 {
			t.Fatalf("calls %q", callTexts(calls))
		}
		if got, _ := h.section(store.ScopeProject, "goals"); got != "" {
			t.Errorf("goals %q", got)
		}
		if chips := refreshChips(h.messages(c.ID)); len(chips) != 0 {
			t.Errorf("chips %+v", chips)
		}
		r := h.review(c.ID)
		if r.UpTo != "" || len(r.Chunks) != 1 {
			t.Fatalf("progress %+v", r)
		}

		// Again: only the chunk not kept, and the merge.
		h.fp.Push(mapReply(), mergeReply(edit("project", "goals", "Weekly."), ""))
		h.refresh(c.ID, false)
		h.wait()
		if got := len(h.fp.Calls()) - len(calls); got != 2 {
			t.Errorf("%d calls after the cancel", got)
		}
		if got, _ := h.section(store.ScopeProject, "goals"); got != "Weekly." {
			t.Errorf("goals %q", got)
		}
	})
}

// A wrong answer gets one retry that says what was wrong.
func TestRefreshRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Retry"})
		h.addTurns(c.ID, 1, 1)
		h.fp.Push(fake.Text(`{"items": [{"kind": "wish", "text": "x"}]}`), mapReply(mapItem("goal", "project", "Weekly")),
			mergeReply(edit("project", "goals", "Weekly."), ""))
		h.refresh(c.ID, false)
		h.wait()
		calls := h.fp.Calls()
		if len(calls) != 3 || len(calls[1].Messages) != 3 {
			t.Fatalf("calls %q", callTexts(calls))
		}
		fix := calls[1].Messages[2].Parts[0].Text.Text
		if !strings.Contains(fix, "Your answer can't be used") || !strings.Contains(fix, "at /items/0") {
			t.Errorf("the retry's message %q", fix)
		}
		if got, _ := h.section(store.ScopeProject, "goals"); got != "Weekly." {
			t.Errorf("goals %q", got)
		}
	})
}

// A failed call ends the refresh with a notice; nothing is applied.
func TestRefreshFails(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Fails"})
		h.addTurns(c.ID, 1, 1)
		h.fp.Push(fake.Fail(&provider.Error{Kind: provider.BadRequest, Provider: "p", Status: 400, Message: "bad"}))
		h.refresh(c.ID, false)
		h.wait()
		chips := refreshChips(h.messages(c.ID))
		if len(chips) != 1 || !strings.HasPrefix(chips[0].Text, "Memory refresh failed") {
			t.Errorf("chips %+v", chips)
		}
		if r := h.review(c.ID); r.UpTo != "" {
			t.Errorf("progress %+v", r)
		}
	})
}

// An edit over a cap is sent back once; still over, it is dropped and
// named in the chip.
func TestRefreshCap(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Cap"})
		h.addTurns(c.ID, 1, 1)
		long := strings.Repeat("word ", 1000)
		bad := withInput(mergeReply(edit("user", "preferences", long)+", "+edit("project", "goals", "Weekly."), "s"), 5000)
		h.fp.Push(mapReply(mapItem("goal", "project", "Weekly")), bad, bad)
		h.refresh(c.ID, false)
		h.wait()
		calls := h.fp.Calls()
		if len(calls) != 3 || !strings.Contains(calls[2].Messages[2].Parts[0].Text.Text, "over its cap of 1000") {
			t.Fatalf("calls %d", len(calls))
		}
		chips := refreshChips(h.messages(c.ID))
		if len(chips) != 1 || !strings.Contains(chips[0].Text, "not saved: edit of user memory, section preferences") || len(chips[0].Edits) != 1 {
			t.Errorf("chips %+v", chips)
		}
	})
}

// Other chats learn of a refresh's edit as Refresh memory's.
func TestRefreshNotice(t *testing.T) {
	m := store.MemorySection{Scope: store.ScopeProject, Section: "goals", Content: "Weekly.", Revision: 2, Source: id.SourceOf("refresh", "c1")}
	if got := memoryNotice(m); !strings.HasPrefix(got, "[memory updated by Refresh memory: project memory, section goals, revision 2]") {
		t.Errorf("notice %q", got)
	}
}
