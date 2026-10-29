package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
	"github.com/amir-saatchi/jenab/internal/store"
)

// A refresh that brings back a section the user deleted saves it without
// a card at Standard, and its chip says it changed a section the user
// wrote (SPEC 3.5: changes or deletes; TASK-030).
func TestRefreshDeletedSection(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		h.edit("goals", "Mine.", 0)
		h.edit("goals", "", 1)
		c := h.newChat(chat.Chat{Title: "Deleted"})
		h.addTurns(c.ID, 1, 1)
		h.fp.Push(mapReply(mapItem("goal", "project", "Weekly")), mergeReply(edit("project", "goals", "Weekly."), ""))
		h.refresh(c.ID, false)
		h.wait()
		if got, _ := h.section(store.ScopeProject, "goals"); got != "Weekly." {
			t.Errorf("goals %q", got)
		}
		if cs := cards(h.messages(c.ID)); len(cs) != 0 {
			t.Errorf("cards %+v", cs)
		}
		if chips := refreshChips(h.messages(c.ID)); len(chips) != 1 || chips[0].Note != noteUserWrote {
			t.Errorf("chips %+v", chips)
		}
	})
}

// Memory read again before the card: a section the user changed while
// the merge ran is dropped and named, so the card shows what is saved.
func TestRefreshStaleCard(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		h.setLevel(project.Strict)
		c := h.newChat(chat.Chat{Title: "Stale"})
		h.addTurns(c.ID, 1, 1)
		merge := mergeReply(edit("project", "goals", "Weekly.")+", "+edit("project", "decisions", "Organic only."), "")
		merge.Wait = time.Second
		h.fp.Push(mapReply(mapItem("goal", "project", "Weekly")), merge)
		h.refresh(c.ID, false)
		synctest.Wait() // the merge's answer is on its way
		h.edit("goals", "Mine.", 0)
		time.Sleep(time.Minute) // the fake clock: the answer arrives
		w := h.waiting(c.ID)
		if w.Text != "Save Refresh memory's edit to project memory, section decisions?" {
			t.Fatalf("waiting %+v", w)
		}
		if card := cards(h.messages(c.ID)); len(card) != 1 || len(card[0].Edits) != 1 || card[0].Edits[0].Target != "memory:project/decisions" {
			t.Fatalf("card %+v", card)
		}
		h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantOnce})
		h.wait()
		if got, _ := h.section(store.ScopeProject, "goals"); got != "Mine." {
			t.Errorf("goals %q", got)
		}
		if got, _ := h.section(store.ScopeProject, "decisions"); got != "Organic only." {
			t.Errorf("decisions %q", got)
		}
		chips := refreshChips(h.messages(c.ID))
		if len(chips) != 1 || chips[0].Text != "Memory refreshed: 1 added; not saved: project memory, section goals (changed meanwhile)" {
			t.Errorf("chips %+v", chips)
		}
	})
}

// A section the user wrote that changes again while the card waits: its
// edit isn't saved, so the chip has no "section you wrote" note.
func TestRefreshStaleUserSection(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		h.edit("goals", "Mine.", 0)
		h.setLevel(project.Strict)
		c := h.newChat(chat.Chat{Title: "Stale mine"})
		h.addTurns(c.ID, 1, 1)
		h.fp.Push(mapReply(mapItem("goal", "project", "Weekly")),
			mergeReply(edit("project", "goals", "Weekly.")+", "+edit("project", "decisions", "Organic only."), ""))
		h.refresh(c.ID, false)
		w := h.waiting(c.ID)
		if w.Text != "Save Refresh memory's edits to 2 sections?" {
			t.Fatalf("waiting %+v", w)
		}
		h.edit("goals", "Mine 2.", 1)
		h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantOnce})
		h.wait()
		if got, _ := h.section(store.ScopeProject, "goals"); got != "Mine 2." {
			t.Errorf("goals %q", got)
		}
		if got, _ := h.section(store.ScopeProject, "decisions"); got != "Organic only." {
			t.Errorf("decisions %q", got)
		}
		chips := refreshChips(h.messages(c.ID))
		if len(chips) != 1 || chips[0].Text != "Memory refreshed: 1 added; not saved: project memory, section goals (changed meanwhile)" || chips[0].Note != "" {
			t.Errorf("chips %+v", chips)
		}
	})
}

// The end failing after the batch is written: the notice says what
// changed and keeps the batch for the undo and the note, the progress
// stays, and the next turn cuts.
func TestRefreshFinishFails(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		orig := finishReview
		finishReview = func(context.Context, *store.ChatsDB, store.FinishReview) (store.Finished, error) {
			return store.Finished{}, errors.New("disk full")
		}
		defer func() { finishReview = orig }()
		c := h.newChat(chat.Chat{Title: "Finish"})
		h.addPage(c.ID, 1)
		h.fp.Push(mapReply(mapItem("goal", "project", "Weekly")), mergeReply(edit("project", "goals", "Weekly."), ""))
		h.refresh(c.ID, false)
		h.wait()
		if got, _ := h.section(store.ScopeProject, "goals"); got != "Weekly." {
			t.Errorf("goals %q", got)
		}
		chips := refreshChips(h.messages(c.ID))
		want := "Memory refreshed: 1 added (project memory, section goals), but the refresh couldn't finish, so the next one reviews these turns again: disk full"
		// The edits were saved, so the note is kept.
		if len(chips) != 1 || chips[0].Text != want || len(chips[0].Edits) != 1 || chips[0].Edits[0].Change == "" || chips[0].Note != noteUntrusted {
			t.Errorf("chips %+v", chips)
		}
		if r := h.review(c.ID); r.UpTo != "" {
			t.Errorf("progress %+v", r)
		}
		if !h.o.state(h.pid, c.ID).takeCut() {
			t.Error("no cut after the edits")
		}
	})
}

// Save, then Stop as the answer arrives: whichever comes first, the card
// says what happened. Saved is kept to; stopped writes nothing.
func TestRefreshSaveThenStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		h.setLevel(project.Strict)
		for i := range 6 {
			c := h.newChat(chat.Chat{Title: fmt.Sprintf("Stop %d", i)})
			h.addTurns(c.ID, 1, 1)
			sec := fmt.Sprintf("s%d", i)
			h.fp.Push(mapReply(mapItem("goal", "project", "Kept")), mergeReply(edit("project", sec, "Kept."), ""))
			h.refresh(c.ID, false)
			w := h.waiting(c.ID)
			h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantOnce})
			h.o.Stop(h.pid, c.ID)
			h.wait()
			ms := h.messages(c.ID)
			card, chips := cards(ms)[0], refreshChips(ms)
			got, _ := h.section(store.ScopeProject, sec)
			switch {
			case card.Stopped:
				if got != "" || len(chips) != 0 {
					t.Errorf("%d: stopped, but %q and %+v", i, got, chips)
				}
			case card.Answer == chat.GrantOnce:
				if got != "Kept." || len(chips) != 1 || chips[0].Text != "Memory refreshed: 1 added" {
					t.Errorf("%d: saved, but %q and %+v", i, got, chips)
				}
			default:
				t.Errorf("%d: card %+v", i, card)
			}
		}
	})
}

// A refresh started before the turn that would cut keeps the cut, though
// it is stopped.
func TestRefreshKeepsCut(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Cut"})
		h.addTurns(c.ID, 1, 1)
		h.fp.Push(mapReply(mapItem("goal", "project", "Weekly")), mergeReply(edit("project", "goals", "Weekly."), ""))
		h.refresh(c.ID, false)
		h.wait()
		h.addTurns(c.ID, 2, 1)
		h.fp.Push(fake.Reply{Hang: true})
		h.refresh(c.ID, false)
		synctest.Wait()
		h.o.Stop(h.pid, c.ID)
		h.wait()
		if !h.o.state(h.pid, c.ID).takeCut() {
			t.Error("the second refresh dropped the first one's cut")
		}
	})
}

// Stop while the card waits: the card shows stopped, nothing is written,
// the chunks are kept, and the next refresh only merges.
func TestRefreshStopAtCard(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		h.setLevel(project.Strict)
		c := h.newChat(chat.Chat{Title: "Card"})
		h.addTurns(c.ID, 1, 7)
		h.fp.Push(mapReply(mapItem("goal", "project", "Weekly")), mapReply(), mergeReply(edit("project", "goals", "Weekly."), "s"))
		h.refresh(c.ID, false)
		h.waiting(c.ID)
		h.o.Stop(h.pid, c.ID)
		h.wait()
		ms := h.messages(c.ID)
		if card := cards(ms); len(card) != 1 || !card[0].Stopped {
			t.Errorf("card %+v", card)
		}
		if got, _ := h.section(store.ScopeProject, "goals"); got != "" {
			t.Errorf("goals %q", got)
		}
		if chips := refreshChips(ms); len(chips) != 0 {
			t.Errorf("chips %+v", chips)
		}
		if n := h.notes(c.ID); n.Summary != "" {
			t.Errorf("summary %q", n.Summary)
		}
		if r := h.review(c.ID); r.UpTo != "" || len(r.Chunks) != 2 {
			t.Errorf("progress %+v", r)
		}
		if st := h.o.State(h.pid, c.ID); st != chat.StateIdle {
			t.Errorf("state %s", st)
		}
		n := len(h.fp.Calls())
		h.fp.Push(mergeReply(edit("project", "goals", "Weekly."), "s"))
		h.refresh(c.ID, false)
		w := h.waiting(c.ID)
		h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantOnce})
		h.wait()
		if got := len(h.fp.Calls()) - n; got != 1 {
			t.Errorf("%d calls after the stop", got)
		}
		if got, _ := h.section(store.ScopeProject, "goals"); got != "Weekly." {
			t.Errorf("goals %q", got)
		}
	})
}

// Stop during the merge: nothing is written, both chunks are kept.
func TestRefreshStopInMerge(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Merge"})
		h.addTurns(c.ID, 1, 7)
		h.fp.Push(mapReply(mapItem("goal", "project", "Weekly")), mapReply(), fake.Reply{Hang: true})
		h.refresh(c.ID, false)
		synctest.Wait()
		h.o.Stop(h.pid, c.ID)
		h.wait()
		if got := len(h.fp.Calls()); got != 3 {
			t.Fatalf("%d calls", got)
		}
		if got, _ := h.section(store.ScopeProject, "goals"); got != "" {
			t.Errorf("goals %q", got)
		}
		if chips := refreshChips(h.messages(c.ID)); len(chips) != 0 {
			t.Errorf("chips %+v", chips)
		}
		if r := h.review(c.ID); r.UpTo != "" || len(r.Chunks) != 2 {
			t.Errorf("progress %+v", r)
		}
		if st := h.o.State(h.pid, c.ID); st != chat.StateIdle {
			t.Errorf("state %s", st)
		}
	})
}

// During a refresh, Retry, Delete and Undo turn are refused.
func TestRefreshRefuses(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Busy"})
		h.addTurns(c.ID, 1, 1)
		h.fp.Push(fake.Reply{Hang: true})
		h.refresh(c.ID, false)
		synctest.Wait()
		if err := h.o.Retry(ctx, h.pid, c.ID); !errors.Is(err, ErrRefreshing) {
			t.Errorf("Retry: %v", err)
		}
		if err := h.o.Delete(ctx, h.pid, c.ID); !errors.Is(err, ErrInTurn) {
			t.Errorf("Delete: %v", err)
		}
		if _, err := h.o.UndoTurn(ctx, h.pid, c.ID, UndoRequest{Turn: 1}); !errors.Is(err, ErrRefreshing) {
			t.Errorf("Undo turn: %v", err)
		}
		h.o.Stop(h.pid, c.ID)
		h.wait()
		if len(h.messages(c.ID)) != 2 {
			t.Error("the chat changed")
		}
	})
}

// A message sent while the map calls run waits: its turn runs after the
// refresh, with the cut, and isn't reviewed.
func TestRefreshQueuedInMap(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Queued"})
		h.addTurns(c.ID, 1, 2)
		m := mapReply(mapItem("goal", "project", "Weekly"))
		m.Wait = time.Second
		h.fp.Push(m, mergeReply(edit("project", "goals", "Weekly."), "s"), fake.Text("hello"))
		h.refresh(c.ID, false)
		synctest.Wait() // the map's answer is on its way
		h.send(c.ID, "and another thing")
		if st := h.o.State(h.pid, c.ID); st != chat.StateWorking {
			t.Errorf("state %s", st)
		}
		h.wait()
		calls := h.fp.Calls()
		if len(calls) != 3 {
			t.Fatalf("calls %q", callTexts(calls))
		}
		if strings.Contains(callTexts(calls)[0], "another thing") {
			t.Error("the queued message was reviewed")
		}
		var sys strings.Builder
		for _, b := range calls[2].System {
			sys.WriteString(b.Text)
		}
		if !strings.Contains(sys.String(), "Weekly.") {
			t.Errorf("the queued turn didn't read the new memory:\n%s", sys.String())
		}
		if tx := texts(h.messages(c.ID)); tx[len(tx)-1] != "assistant: hello" {
			t.Errorf("texts %q", tx)
		}
		if r := h.review(c.ID); r.UpToTurn != 2 {
			t.Errorf("progress %+v", r)
		}
	})
}

// A restart mid-refresh: nothing is applied, the finished chunk is kept,
// and the chat isn't left busy.
func TestRefreshRestart(t *testing.T) {
	root := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, root, "", testSettings())
		c := h.newChat(chat.Chat{Title: "Restart"})
		h.addTurns(c.ID, 1, 7)
		h.fp.Push(mapReply(mapItem("goal", "project", "Weekly")), fake.Reply{Hang: true})
		h.refresh(c.ID, false)
		synctest.Wait()
		pid := h.pid
		h.stop()

		h = newHarness(t, root, pid, testSettings())
		defer h.stop()
		if st := h.o.State(h.pid, c.ID); st != chat.StateIdle {
			t.Errorf("state %s", st)
		}
		if got, _ := h.section(store.ScopeProject, "goals"); got != "" {
			t.Errorf("goals %q", got)
		}
		if r := h.review(c.ID); r.UpTo != "" || len(r.Chunks) != 1 {
			t.Errorf("progress %+v", r)
		}
		h.fp.Push(mapReply(), mergeReply(edit("project", "goals", "Weekly."), ""))
		h.refresh(c.ID, false)
		h.wait()
		if got := len(h.fp.Calls()); got != 2 {
			t.Errorf("%d calls after the restart", got)
		}
		if got, _ := h.section(store.ScopeProject, "goals"); got != "Weekly." {
			t.Errorf("goals %q", got)
		}
	})
}

// Full refresh maps every chunk again, saved ones too, and drops them at
// its end; a cancelled one leaves the last progress.
func TestRefreshFullDiscards(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Full"})
		h.addTurns(c.ID, 1, 7)
		h.fp.Push(mapReply(mapItem("goal", "project", "Weekly")), fake.Reply{Hang: true})
		h.refresh(c.ID, false)
		synctest.Wait()
		h.o.Stop(h.pid, c.ID)
		h.wait()
		if r := h.review(c.ID); len(r.Chunks) != 1 {
			t.Fatalf("progress %+v", r)
		}
		n := len(h.fp.Calls())
		h.fp.Push(mapReply(mapItem("goal", "project", "Weekly")), mapReply(), mergeReply(edit("project", "goals", "Weekly."), ""))
		h.refresh(c.ID, true)
		h.wait()
		if got := len(h.fp.Calls()) - n; got != 3 {
			t.Errorf("a full refresh made %d calls", got)
		}
		if r := h.review(c.ID); r.UpToTurn != 7 || len(r.Chunks) != 0 {
			t.Errorf("progress %+v", r)
		}

		// A cancelled Full refresh keeps the progress it would replace.
		h.addTurns(c.ID, 8, 1)
		h.fp.Push(fake.Reply{Hang: true}, fake.Reply{Hang: true})
		h.refresh(c.ID, true)
		synctest.Wait()
		h.o.Stop(h.pid, c.ID)
		h.wait()
		if r := h.review(c.ID); r.UpToTurn != 7 {
			t.Errorf("progress after a cancelled Full refresh %+v", r)
		}
	})
}

// The merge's items are read leniently: a wrong list fails nothing when
// the edits and the summary are right.
func TestRefreshLenientItems(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Items"})
		h.addTurns(c.ID, 1, 1)
		h.fp.Push(mapReply(mapItem("open_question", "chat", "Which day?")),
			fake.Text(`{"items": [{"kind": "wish", "scope": "everywhere"}], "edits": [`+edit("project", "goals", "Weekly.")+`], "summary": "s"}`))
		h.refresh(c.ID, false)
		h.wait()
		if got := len(h.fp.Calls()); got != 2 {
			t.Errorf("%d calls", got)
		}
		if got, _ := h.section(store.ScopeProject, "goals"); got != "Weekly." {
			t.Errorf("goals %q", got)
		}
	})
}

// Notes with no room left: the merge is asked for no summary, and the
// chip says so.
func TestRefreshNotesFull(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Notes"})
		h.addTurns(c.ID, 1, 1)
		h.open(func(p *project.Project) {
			if _, err := p.Chats.SaveNotes(context.Background(), chat.SessionNote{Chat: c.ID, Content: strings.Repeat("abcd", store.MaxNotesTokens)}); err != nil {
				t.Fatal(err)
			}
		})
		h.fp.Push(mapReply(mapItem("goal", "project", "Weekly")), mergeReply(edit("project", "goals", "Weekly."), "A summary anyway."))
		h.refresh(c.ID, false)
		h.wait()
		calls := h.fp.Calls()
		if sys := calls[1].System[0].Text; !strings.Contains(sys, "write no summary") || strings.Contains(sys, "At most 0 tokens") {
			t.Errorf("merge prompt:\n%s", sys)
		}
		chips := refreshChips(h.messages(c.ID))
		if len(chips) != 1 || chips[0].Text != "Memory refreshed: 1 added; no summary was written: the session notes are full" {
			t.Errorf("chips %+v", chips)
		}
		if n := h.notes(c.ID); n.Summary != "" {
			t.Errorf("summary %q", n.Summary)
		}
	})
}

// The chip counts the sections the batch wrote.
func TestRefreshChipCounts(t *testing.T) {
	applied := []chat.MemoryDiff{{After: "a"}, {Before: "a", After: "b"}, {Before: "b"}, {After: "c"}}
	if got := chipText(applied, nil, false, false, nil); got != "Memory refreshed: 2 added, 1 changed, 1 deleted" {
		t.Errorf("chip %q", got)
	}
}

// A refresh knows whether its turns read untrusted content.
func TestRefreshUntrusted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		plan := func(c chat.Chat) refreshPlan {
			var pl refreshPlan
			h.open(func(p *project.Project) {
				var err error
				if pl, err = h.o.planRefresh(context.Background(), p, c.ID, false, 0); err != nil {
					t.Fatal(err)
				}
			})
			return pl
		}
		clean := h.newChat(chat.Chat{Title: "Clean"})
		h.addTurns(clean.ID, 1, 1)
		if plan(clean).untrusted {
			t.Error("a chat without tools is untrusted")
		}
		web := h.newChat(chat.Chat{Title: "Web"})
		h.addPage(web.ID, 1)
		if !plan(web).untrusted {
			t.Error("a page's text isn't untrusted")
		}
	})
}

// A refresh that starts after a turn ended, while its runner hasn't
// looked at the queue again: a message sent then waits for the
// refresh's end.
func TestRefreshAfterTurn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "After"})
		h.addTurns(c.ID, 1, 2)
		m := mapReply(mapItem("goal", "project", "Weekly"))
		m.Wait = time.Second
		h.fp.Push(fake.Text("first"), m, mergeReply(edit("project", "goals", "Weekly."), "s"), fake.Text("second"))
		once := true
		orig := turnEnded
		turnEnded = func(*chatState) {
			if !once {
				return
			}
			once = false
			if err := h.o.Refresh(context.Background(), h.pid, c.ID, RefreshOptions{}); err != nil {
				t.Errorf("refresh: %v", err)
			}
			if _, err := h.o.Send(context.Background(), h.pid, c.ID, UserMessage{Text: "and another thing"}); err != nil {
				t.Errorf("send: %v", err)
			}
		}
		defer func() { turnEnded = orig }()
		h.send(c.ID, "a question")
		h.wait()
		calls := callTexts(h.fp.Calls())
		if len(calls) != 4 || !strings.HasPrefix(calls[1], "Conversation part") || strings.Contains(calls[1], "another thing") {
			t.Fatalf("calls %q", calls)
		}
		var sys strings.Builder
		for _, b := range h.fp.Calls()[3].System {
			sys.WriteString(b.Text)
		}
		if !strings.Contains(sys.String(), "Weekly.") {
			t.Error("the queued turn ran before the refresh's end")
		}
	})
}

// Stop as a map call ends: its chunk is saved all the same (D6).
func TestRefreshStopAsMapEnds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Map end"})
		h.addTurns(c.ID, 1, 1)
		h.fp.Push(mapReply(mapItem("goal", "project", "Weekly")))
		orig := mapped
		mapped = func() { h.o.Stop(h.pid, c.ID) }
		defer func() { mapped = orig }()
		h.refresh(c.ID, false)
		h.wait()
		if r := h.review(c.ID); r.UpTo != "" || len(r.Chunks) != 1 {
			t.Errorf("progress %+v", r)
		}
		if n := len(h.fp.Calls()); n != 1 {
			t.Errorf("%d calls", n)
		}
	})
}

// Stop with the card's answer, then the end fails with nothing written:
// the notice still says so, for a denied card and for an answer that
// couldn't be recorded.
func TestRefreshEndFailsAfterCard(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		h.setLevel(project.Strict)
		var c chat.Chat
		stopFail := func() error {
			h.o.Stop(h.pid, c.ID)
			return errors.New("disk full")
		}
		origF, origR := finishReview, recordApproval
		defer func() { finishReview, recordApproval = origF, origR }()
		failFinish := func(context.Context, *store.ChatsDB, store.FinishReview) (store.Finished, error) {
			return store.Finished{}, stopFail()
		}
		failRecord := func(context.Context, *store.ProjectDB, store.Approval) error { return stopFail() }
		for i, grant := range []chat.Grant{chat.GrantDeny, chat.GrantOnce} {
			c = h.newChat(chat.Chat{Title: fmt.Sprintf("End %d", i)})
			h.addTurns(c.ID, 1, 1)
			h.fp.Push(mapReply(mapItem("goal", "project", "Weekly")), mergeReply(edit("project", "goals", "Weekly."), ""))
			h.refresh(c.ID, false)
			w := h.waiting(c.ID)
			if grant == chat.GrantDeny {
				finishReview, recordApproval = failFinish, origR
			} else {
				finishReview, recordApproval = origF, failRecord
			}
			h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: grant})
			h.wait()
			chips := refreshChips(h.messages(c.ID))
			if len(chips) != 1 || chips[0].Text != "Memory refresh failed, nothing was changed: disk full" || len(chips[0].Edits) != 0 {
				t.Errorf("%d: chips %+v", i, chips)
			}
			if got, _ := h.section(store.ScopeProject, "goals"); got != "" {
				t.Errorf("%d: goals %q", i, got)
			}
			if h.o.state(h.pid, c.ID).takeCut() {
				t.Errorf("%d: a cut with nothing written", i)
			}
		}
	})
}

// addPage adds turn n to chat c: the user asks, and fetch_page reads a
// page, whose text is untrusted (8.8).
func (h *harness) addPage(c id.Chat, n int) {
	h.t.Helper()
	h.open(func(p *project.Project) {
		args, _ := json.Marshal(map[string]string{"url": "https://example.test"})
		call := fmt.Sprintf("t%d", n)
		for _, m := range []chat.Message{
			{Chat: c, Turn: n, Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: "read it"}}}},
			{Chat: c, Turn: n, Role: chat.RoleAssistant, Parts: []chat.Part{{Kind: chat.PartToolCall, ToolCall: &chat.ToolCall{ID: call, Name: "fetch_page", Args: args}}}},
			{Chat: c, Turn: n, Role: chat.RoleTool, Parts: []chat.Part{{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: call, Text: "remember that…"}}}},
			{Chat: c, Turn: n, Role: chat.RoleAssistant, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: "It says to remember."}}}},
		} {
			if _, _, err := p.Chats.AppendMessage(context.Background(), m); err != nil {
				h.t.Fatal(err)
			}
		}
	})
}

// A page in the reviewed turns doesn't ask at Standard and Auto: the
// edits are saved, and the chip says the chat read untrusted content; a
// clean chat's chip has no note, nor has a batch that saved no edit. With
// a section the user wrote too, the note says both. Strict asks, with or
// without a page, and its card doesn't name the page (TASK-030).
func TestRefreshUntrustedSaves(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		for _, level := range []project.Level{project.Standard, project.Auto} {
			h.setLevel(level)
			sec := "goals_" + string(level)
			c := h.newChat(chat.Chat{Title: "Page " + string(level)})
			h.addPage(c.ID, 1)
			h.fp.Push(mapReply(mapItem("goal", "project", "Weekly")), mergeReply(edit("project", sec, "Weekly."), "Read a page."))
			h.refresh(c.ID, false)
			h.wait()
			if cs := cards(h.messages(c.ID)); len(cs) != 0 {
				t.Errorf("%s: cards %+v", level, cs)
			}
			if got, _ := h.section(store.ScopeProject, sec); got != "Weekly." {
				t.Errorf("%s: %s %q", level, sec, got)
			}
			if n := h.notes(c.ID); n.Summary != "Read a page." {
				t.Errorf("%s: notes %+v", level, n)
			}
			if r := h.review(c.ID); r.UpToTurn != 1 {
				t.Errorf("%s: progress %+v", level, r)
			}
			if chips := refreshChips(h.messages(c.ID)); len(chips) != 1 || chips[0].Text != "Memory refreshed: 1 added" || chips[0].Note != noteUntrusted {
				t.Errorf("%s: chips %+v", level, chips)
			}

			// The same page, with an edit that changes nothing: no note.
			same := h.newChat(chat.Chat{Title: "Same " + string(level)})
			h.addPage(same.ID, 1)
			h.fp.Push(mapReply(mapItem("goal", "project", "Weekly")), mergeReply(edit("project", sec, "Weekly."), ""))
			h.refresh(same.ID, false)
			h.wait()
			if chips := refreshChips(h.messages(same.ID)); len(chips) != 1 || chips[0].Text != "Memory refreshed: nothing new" || chips[0].Note != "" {
				t.Errorf("%s: an empty batch's chips %+v", level, chips)
			}

			clean := h.newChat(chat.Chat{Title: "Clean " + string(level)})
			h.addTurns(clean.ID, 1, 1)
			h.fp.Push(mapReply(mapItem("goal", "project", "Monthly")), mergeReply(edit("project", sec, "Monthly."), ""))
			h.refresh(clean.ID, false)
			h.wait()
			if got, _ := h.section(store.ScopeProject, sec); got != "Monthly." {
				t.Errorf("%s: clean %s %q", level, sec, got)
			}
			if chips := refreshChips(h.messages(clean.ID)); len(chips) != 1 || chips[0].Note != "" || len(cards(h.messages(clean.ID))) != 0 {
				t.Errorf("%s: a clean chat's chips %+v", level, chips)
			}
		}

		// A page and a section the user wrote: one line says both.
		h.setLevel(project.Standard)
		h.edit("decisions", "Mine.", 0)
		c := h.newChat(chat.Chat{Title: "Both"})
		h.addPage(c.ID, 1)
		h.fp.Push(mapReply(mapItem("decision", "project", "Weekly")), mergeReply(edit("project", "decisions", "Mine.\nWeekly."), ""))
		h.refresh(c.ID, false)
		h.wait()
		const both = "From a chat that read web pages, files or MCP results; changed a section you wrote"
		if chips := refreshChips(h.messages(c.ID)); len(chips) != 1 || chips[0].Note != both {
			t.Errorf("chips %+v", chips)
		}
		if got, _ := h.section(store.ScopeProject, "decisions"); got != "Mine.\nWeekly." {
			t.Errorf("decisions %q", got)
		}

		// Strict asks; the card checks the edits, not the page.
		h.setLevel(project.Strict)
		c = h.newChat(chat.Chat{Title: "Strict"})
		h.addPage(c.ID, 1)
		h.fp.Push(mapReply(mapItem("goal", "project", "Daily")), mergeReply(edit("project", "plans", "Daily."), ""))
		h.refresh(c.ID, false)
		w := h.waiting(c.ID)
		if card := cards(h.messages(c.ID)); len(card) != 1 ||
			card[0].Risk != "Project memory is in the context of every chat in this project. Check that each edit is what you want kept." {
			t.Fatalf("cards %+v", card)
		}
		h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantDeny})
		h.wait()
		if got, _ := h.section(store.ScopeProject, "plans"); got != "" {
			t.Errorf("plans %q", got)
		}
		if chips := refreshChips(h.messages(c.ID)); len(chips) != 1 || chips[0].Text != "Memory refresh: the edits were not saved" || chips[0].Note != "" {
			t.Errorf("chips %+v", chips)
		}
	})
}

// The map calls see the reviewed turns and the turn before them, given
// as context: a page in either gives the chip its note, one further back
// doesn't. A refresh that only writes the summary has no note. None asks
// at Standard (TASK-030).
func TestRefreshUntrustedContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Page"})
		h.addPage(c.ID, 1)
		h.fp.Push(mapReply(mapItem("progress", "chat", "Read a page")), mergeReply("", "Read a page."))
		h.refresh(c.ID, false)
		h.wait()
		if n := h.notes(c.ID); n.Summary != "Read a page." {
			t.Errorf("notes %+v", n)
		}
		if r := h.review(c.ID); r.UpToTurn != 1 {
			t.Fatalf("progress %+v", r)
		}
		// note is the last chip's note.
		note := func() string {
			chips := refreshChips(h.messages(c.ID))
			return chips[len(chips)-1].Note
		}
		if n := note(); n != "" {
			t.Errorf("a summary-only refresh's note %q", n)
		}

		// Turn 1, with the page, is now the context turn.
		h.addTurns(c.ID, 2, 1)
		h.fp.Push(mapReply(mapItem("goal", "project", "Weekly")), mergeReply(edit("project", "goals", "Weekly."), ""))
		h.refresh(c.ID, false)
		h.wait()
		if got, _ := h.section(store.ScopeProject, "goals"); got != "Weekly." {
			t.Errorf("goals %q", got)
		}
		if n := note(); n != noteUntrusted {
			t.Errorf("the context turn's page: note %q", n)
		}

		// The page is two turns back: not shown, no note.
		h.addTurns(c.ID, 3, 1)
		h.fp.Push(mapReply(mapItem("goal", "project", "Monthly")), mergeReply(edit("project", "plans", "Monthly."), ""))
		h.refresh(c.ID, false)
		h.wait()
		if got, _ := h.section(store.ScopeProject, "plans"); got != "Monthly." {
			t.Errorf("plans %q", got)
		}
		if n := note(); n != "" {
			t.Errorf("a page before the context turn: note %q", n)
		}
		if cs := cards(h.messages(c.ID)); len(cs) != 0 {
			t.Errorf("cards %+v", cs)
		}
	})
}
