package agent

import (
	"context"
	"database/sql"
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
)

// cardMarks is the turns of ms with history cards' mark of outside data.
func cardMarks(ms []chat.Message) []int {
	var out []int
	for _, m := range ms {
		if slices.ContainsFunc(m.Parts, cardMark) {
			out = append(out, m.Turn)
		}
	}
	return out
}

// appendOnly fails t unless each request is the one before with more at
// the end.
func appendOnly(t *testing.T, calls []provider.Request) {
	t.Helper()
	for i := 1; i < len(calls); i++ {
		prev, got := texts(calls[i-1].Messages), texts(calls[i].Messages)
		if len(got) < len(prev) || !slices.Equal(got[:len(prev)], prev) {
			t.Errorf("request %d is not request %d with more at the end:\n%q\n%q", i+1, i, prev, got)
		}
	}
}

// loadOutside is the chat's newest turn with outside data as load finds
// it, as after a restart.
func loadOutside(h *harness, c id.Chat) int {
	h.t.Helper()
	outside := 0
	h.open(func(p *project.Project) {
		cs := h.o.state(h.pid, c)
		cs.mu.Lock()
		defer cs.mu.Unlock()
		if err := h.o.load(context.Background(), cs, p); err != nil {
			h.t.Fatal(err)
		}
		outside = cs.outside
	})
	return outside
}

// TestHistoryCardMarkStored: a card of a turn that read outside data
// stores a mark in the new turn, which the requests leave out, so they
// stay append-only, and which load counts after a restart.
func TestHistoryCardMarkStored(t *testing.T) {
	root := t.TempDir()
	h := newHarness(t, root, "", testSettings(), echo(), outside())
	c := h.newChat(chat.Chat{Title: "Coins"})
	old := volTurn
	old.tool, old.args = "outside", map[string]any{}
	seedChat(h, c.ID, longChat(map[int]oldTurn{4: old}))
	h.fp.Push(fake.ToolCall("c1", "echo", map[string]any{"text": "sol"}), fake.Text("Saved it for SOL."))
	h.turn(c.ID, "Make that weekly volatility table for SOL too.")
	if got := cardMarks(h.messages(c.ID)); !slices.Equal(got, []int{23}) {
		t.Fatalf("marks in turns %v, want [23]", got)
	}
	calls := h.fp.Calls()
	if len(calls) != 2 {
		t.Fatalf("%d requests, want 2", len(calls))
	}
	appendOnly(t, calls)
	for i, call := range calls {
		if got := cardMarks(call.Messages); len(got) > 0 {
			t.Errorf("request %d has the mark in turns %v", i+1, got)
		}
	}
	h.stop()
	h = newHarness(t, root, h.pid, testSettings(), echo(), outside())
	defer h.stop()
	if got := loadOutside(h, c.ID); got != 23 {
		t.Errorf("after a restart, outside %d, want 23", got)
	}
}

// TestHistoryCardOfMarkedTurn: a card of a turn whose own cards showed
// outside data marks the new turn too.
func TestHistoryCardOfMarkedTurn(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings(), echo())
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Tides"})
	turns := make([]oldTurn, 44)
	for i := range turns {
		turns[i] = filler(i + 1)
	}
	turns[22] = oldTurn{user: "Show the lunar tides of Kiel harbour for March.", answer: "Saved kiel_tides, the lunar tides of Kiel harbour.", mark: true}
	seedChat(h, c.ID, turns)
	h.fp.Push(fake.Text("ok"))
	h.turn(c.ID, "Make that lunar tides chart for Kiel harbour in April.")
	if cards := cardsIn(h.fp.Calls()[0].Messages); len(cards) != 1 || !strings.HasPrefix(cards[0], "45: [Turn 23 of this chat") {
		t.Fatalf("cards: %q", cards)
	}
	cs := h.o.find(h.pid, c.ID)
	cs.mu.Lock()
	outside := cs.outside
	cs.mu.Unlock()
	if outside != 45 {
		t.Errorf("outside %d, want 45", outside)
	}
	if got := cardMarks(h.messages(c.ID)); !slices.Equal(got, []int{23, 45}) {
		t.Errorf("marks in turns %v, want [23 45]", got)
	}
}

// TestHistoryCardsMemo: a turn's cards are found once, so a word that
// became common during the turn leaves its later requests as they were.
func TestHistoryCardsMemo(t *testing.T) {
	started, release := make(chan string), make(chan struct{})
	h := newHarness(t, t.TempDir(), "", testSettings(), blocker(started, release))
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Coins"})
	other := h.newChat(chat.Chat{Title: "Other"}).ID
	seedChat(h, c.ID, longChat(map[int]oldTurn{4: volTurn}))
	h.fp.Push(fake.ToolCall("c1", "block", map[string]any{}), fake.Text("Saved it for SOL."))
	text := "Make that weekly volatility table for SOL too."
	h.send(c.ID, text)
	<-started
	h.open(func(p *project.Project) {
		ctx := context.Background()
		for i := range 40 {
			m := chat.Message{Chat: other, Turn: i + 1, Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: "The weekly volatility table for it, please."}}}}
			if _, _, err := p.Chats.AppendMessage(ctx, m); err != nil {
				t.Fatal(err)
			}
		}
		scores, err := CardScores(ctx, p.Chats, c.ID, text, 23)
		if err != nil {
			t.Fatal(err)
		}
		if got := PickCards(scores, 20); len(got) > 0 {
			t.Fatalf("the words are still rare: cards for %v", got)
		}
	})
	close(release)
	h.wait()
	calls := h.fp.Calls()
	if len(calls) != 2 {
		t.Fatalf("%d requests, want 2", len(calls))
	}
	first := cardsIn(calls[0].Messages)
	if len(first) != 1 || !strings.HasPrefix(first[0], "23: [Turn 4 of this chat") {
		t.Fatalf("cards: %q", first)
	}
	if got := cardsIn(calls[1].Messages); !slices.Equal(got, first) {
		t.Errorf("second request's cards: %q", got)
	}
	appendOnly(t, calls)
}

// TestHistoryCardsCut: a real cut keeps a card while its turn is in the
// window and drops it once the turn falls before it.
func TestHistoryCardsCut(t *testing.T) {
	set := testSettings()
	set.Context.HistoryMinTurns, set.Context.HistoryMaxTurns = 2, 3
	h := newHarness(t, t.TempDir(), "", set, echo())
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Coins"})
	seedChat(h, c.ID, longChat(map[int]oldTurn{4: volTurn}))
	cs := func() (start int, card bool) {
		s := h.o.find(h.pid, c.ID)
		s.mu.Lock()
		defer s.mu.Unlock()
		_, card = s.win.cards[23]
		return s.win.start, card
	}
	h.fp.Push(fake.Text("Saved it for SOL."))
	h.turn(c.ID, "Make that weekly volatility table for SOL too.")
	if start, card := cs(); start != 21 || !card {
		t.Fatalf("after turn 23: start %d, card %v", start, card)
	}
	for _, n := range []int{24, 25} {
		h.fp.Push(fake.Text("Fine."))
		h.turn(c.ID, "Thanks!")
		if start, card := cs(); !card || (n == 25) != (start == 23) {
			t.Fatalf("after turn %d: start %d, card %v", n, start, card)
		}
	}
	calls := h.fp.Calls()
	if got := cardsIn(calls[len(calls)-1].Messages); len(got) != 1 || !strings.HasPrefix(got[0], "23: [Turn 4") {
		t.Errorf("turn 25's cards after its cut: %q", got)
	}
	for range 2 {
		h.fp.Push(fake.Text("Fine."))
		h.turn(c.ID, "Thanks!")
	}
	if start, card := cs(); start != 25 || card {
		t.Errorf("after turn 27: start %d, card %v", start, card)
	}
}

// TestHistoryCardsCutEstimate: the window's estimate counts its cards, so
// a window that fits without them is cut.
func TestHistoryCardsCutEstimate(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings(), echo())
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Coins"})
	old := volTurn
	old.args = map[string]any{"text": strings.Repeat("btc weekly volatility ", 90)}
	old.answer += " " + strings.Repeat("Each week is the spread of daily closes. ", 25)
	seedChat(h, c.ID, longChat(map[int]oldTurn{4: old}))
	h.fp.Push(fake.Text("Saved it for SOL."))
	h.turn(c.ID, "Make that weekly volatility table for SOL too.")
	s := h.o.find(h.pid, c.ID)
	s.mu.Lock()
	start := s.win.start
	var with, without int
	h.open(func(p *project.Project) {
		ms, _, err := p.Chats.Messages(context.Background(), c.ID, start, 23)
		if err != nil {
			t.Fatal(err)
		}
		with = provider.EstimateTokens(provider.Request{Messages: shape(withCards(ms, s.win.cards), &s.win)})
		without = provider.EstimateTokens(provider.Request{Messages: shape(ms, &s.win)})
	})
	s.mu.Unlock()
	if start != 20 || with-without < 400 {
		t.Fatalf("start %d; %d tokens with cards, %d without", start, with, without)
	}
	h.set.Context.HistoryMaxTokens = (with + without) / 2
	h.fp.Push(fake.Text("Fine."))
	h.turn(c.ID, "Thanks!")
	s.mu.Lock()
	start = s.win.start
	s.mu.Unlock()
	if start != 21 {
		t.Errorf("no cut: start %d, want 21 (%d tokens with cards, %d without, limit %d)", start, with, without, h.set.Context.HistoryMaxTokens)
	}
}

// TestHistoryCardsShownOnce: a turn a card in the window shows gets no
// second card.
func TestHistoryCardsShownOnce(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings(), echo())
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Coins"})
	seedChat(h, c.ID, longChat(map[int]oldTurn{4: volTurn}))
	h.fp.Push(fake.Text("Here is btc_weekly_vol."), fake.Text("Saved it for SOL."))
	h.turn(c.ID, "Show the btc_weekly_vol view.")
	h.turn(c.ID, "Make that volatility table for SOL over 26 weeks.")
	calls := h.fp.Calls()
	first := cardsIn(calls[0].Messages)
	if len(first) != 1 || !strings.HasPrefix(first[0], "23: [Turn 4 of this chat") {
		t.Fatalf("turn 23's cards: %q", first)
	}
	if got := cardsIn(calls[1].Messages); !slices.Equal(got, first) {
		t.Errorf("turn 24's cards: %q", got)
	}
	h.open(func(p *project.Project) {
		scores, err := CardScores(context.Background(), p.Chats, c.ID, "Make that volatility table for SOL over 26 weeks.", 24)
		if err != nil {
			t.Fatal(err)
		}
		if got := PickCards(scores, 20); !slices.Equal(got, []int{4}) {
			t.Errorf("turn 24 would card %v without the rule, want [4]", got)
		}
	})
}

// TestHistoryCardsUndone: a turn that was undone gets no card.
func TestHistoryCardsUndone(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings(), echo())
	defer h.stop()
	ctx := context.Background()
	c := h.newChat(chat.Chat{Title: "Coins"})
	seedChat(h, c.ID, longChat(map[int]oldTurn{4: volTurn}))
	var msg id.Message
	for _, m := range h.messages(c.ID) {
		if m.Turn == 4 && m.Role == chat.RoleAssistant {
			msg = m.ID
		}
	}
	h.open(func(p *project.Project) {
		if _, err := store.Do(ctx, p.DB.DB, limit.Interactive, func(tx *sql.Tx) (int, error) {
			_, err := tx.Exec("CREATE TABLE prices (id INTEGER PRIMARY KEY, price REAL)")
			return 0, err
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := p.DB.Insert(ctx, limit.Interactive, id.SourceOf("message", string(msg)), "prices", []store.Row{{"id": 1, "price": 10}}, store.ConflictFail); err != nil {
			t.Fatal(err)
		}
	})
	if r, err := h.o.UndoTurn(ctx, h.pid, c.ID, UndoRequest{Turn: 4}); err != nil || !r.Done {
		t.Fatalf("undo: %+v, %v", r, err)
	}
	h.fp.Push(fake.Text("ok"))
	h.turn(c.ID, "Make that weekly volatility table for SOL too.")
	if cards := cardsIn(h.fp.Calls()[0].Messages); len(cards) > 0 {
		t.Errorf("cards: %q", cards)
	}
}

// TestTextPartsCache: a project's count of text parts is reused until it
// is partsFresh old.
func TestTextPartsCache(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings())
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Coins"})
	seedChat(h, c.ID, longChat(nil))
	h.open(func(p *project.Project) {
		ctx := context.Background()
		count := func() int {
			n, err := h.o.textParts(ctx, h.pid, p.Chats)
			if err != nil {
				t.Fatal(err)
			}
			return n
		}
		first := count()
		m := chat.Message{Chat: c.ID, Turn: 23, Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: "One more."}}}}
		if _, _, err := p.Chats.AppendMessage(ctx, m); err != nil {
			t.Fatal(err)
		}
		if got := count(); got != first {
			t.Errorf("counted again too soon: %d, then %d", first, got)
		}
		h.o.mu.Lock()
		h.o.parts[h.pid] = partsCount{first, time.Now().Add(-partsFresh)}
		h.o.mu.Unlock()
		if got := count(); got != first+1 {
			t.Errorf("a stale count: %d, want %d", got, first+1)
		}
		if first != 44 {
			t.Errorf("%d text parts, want 44", first)
		}
	})
}
