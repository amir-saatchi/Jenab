package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
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

// cardTools are a migration and an insert, straight on the project's
// database.
func cardTools() []tool.Tool {
	empty := json.RawMessage(`{"type": "object"}`)
	return []tool.Tool{
		tool.Func(tool.Spec{Name: "migrate", Description: "Makes prices.", Schema: empty},
			func(ctx context.Context, env *tool.Env, _ struct{}) (tool.Result, error) {
				_, err := env.Project.DB.Migrate(ctx, id.SourceUser, store.Migration{Steps: []json.RawMessage{json.RawMessage(createPrices)}}, store.MigrateOptions{})
				return tool.Result{Text: "made"}, err
			}),
		tool.Func(tool.Spec{Name: "add", Description: "Adds a price.", Schema: empty},
			func(ctx context.Context, env *tool.Env, _ struct{}) (tool.Result, error) {
				_, err := env.Project.DB.Insert(ctx, limit.Interactive, id.SourceUser, "prices",
					[]store.Row{{"date": "2026-10-01", "close": 1.5}}, store.ConflictFail)
				return tool.Result{Text: "added"}, err
			}),
	}
}

// realCard is the app's card.
func realCard(ctx context.Context, p *project.Project) (string, error) {
	return p.DB.Card(ctx, time.Now())
}

// cardOf is the project card block of req, or "".
func cardOf(t *testing.T, req provider.Request) string {
	t.Helper()
	card := ""
	for i, b := range req.System {
		if b.Cache != (i == len(req.System)-1) {
			t.Errorf("block %q: cache point %v", b.Name, b.Cache)
		}
		if b.Name == cardBlock {
			card = b.Text
		}
	}
	return card
}

// cardNotes are the notes of req that bring a new project card.
func cardNotes(req provider.Request) []string {
	var out []string
	for _, m := range req.Messages {
		for _, p := range m.Parts {
			if n := p.Notice; n != nil && strings.HasPrefix(n.Text, "[the project card changed") {
				out = append(out, n.Text)
			}
		}
	}
	return out
}

// After a migration in the turn the new card comes at once, as a note at
// the end: the card block stays until the next cut, and so does the
// note. A plain insert brings no note: its row shows at the next cut
// (3.1, TASK-047).
func TestCardAfterMigration(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings(), cardTools()...)
	defer h.stop()
	h.o.d.Card = realCard
	c := h.newChat(chat.Chat{Title: "Prices"})
	h.fp.Push(fake.ToolCall("c1", "migrate", struct{}{}), fake.ToolCall("c2", "add", struct{}{}), fake.Text("done"), fake.Text("hi"))
	h.turn(c.ID, "make a table and add a price")
	h.turn(c.ID, "hello")
	calls := h.fp.Calls()
	if len(calls) != 4 {
		t.Fatalf("%d requests, want 4", len(calls))
	}
	cards := make([]string, len(calls))
	for i, req := range calls {
		cards[i] = cardOf(t, req)
	}
	if !strings.Contains(cards[0], "Tables: none yet.") {
		t.Errorf("before the migration:\n%s", cards[0])
	}
	for i := range calls {
		if cards[i] != cards[0] {
			t.Errorf("request %d changed the card block:\n%s", i+1, cards[i])
		}
		notes := cardNotes(calls[i])
		if i == 0 && len(notes) != 0 || i > 0 && len(notes) != 1 {
			t.Fatalf("request %d: card notes %q", i+1, notes)
		}
		if i > 0 && !strings.Contains(notes[0], "- prices (0 rows): date TEXT NOT NULL, close REAL; key (date)") {
			t.Errorf("request %d, after the migration:\n%s", i+1, notes[0])
		}
	}
}

// After a cut the card has the insert's row.
func TestCardAtCut(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings(), cardTools()...)
	defer h.stop()
	h.o.d.Card = realCard
	c := h.newChat(chat.Chat{Title: "Prices"})
	h.fp.Push(fake.ToolCall("c1", "migrate", struct{}{}), fake.ToolCall("c2", "add", struct{}{}), fake.Text("done"), fake.Text("hi"))
	h.turn(c.ID, "make a table and add a price")
	h.o.mu.Lock()
	h.o.chats[key{h.pid, c.ID}].win.last = time.Time{} // as after a restart: the next turn cuts
	h.o.mu.Unlock()
	h.turn(c.ID, "hello")
	calls := h.fp.Calls()
	if card := cardOf(t, calls[len(calls)-1]); !strings.Contains(card, "- prices (1 row): date TEXT NOT NULL, close REAL; key (date); date 2026-10-01") {
		t.Errorf("after the cut:\n%s", card)
	}
}

// A migration in another chat reaches this chat's next request (3.1).
func TestCardAfterMigrationInOtherChat(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings(), cardTools()...)
	defer h.stop()
	h.o.d.Card = realCard
	a := h.newChat(chat.Chat{Title: "A"})
	b := h.newChat(chat.Chat{Title: "B"})
	h.fp.Push(fake.Text("hi"), fake.ToolCall("c1", "migrate", struct{}{}), fake.Text("made"), fake.Text("hi again"))
	h.turn(a.ID, "hello")
	h.turn(b.ID, "make a table")
	h.turn(a.ID, "hello again")
	calls := h.fp.Calls()
	if len(calls) != 4 {
		t.Fatalf("%d requests, want 4", len(calls))
	}
	if card := cardOf(t, calls[0]); !strings.Contains(card, "Tables: none yet.") {
		t.Errorf("chat A, turn 1:\n%s", card)
	}
	if notes := cardNotes(calls[3]); len(notes) != 1 || !strings.Contains(notes[0], "- prices (0 rows)") {
		t.Errorf("chat A, turn 2: %q", notes)
	}
}

// A card that fails leaves the turn alone: the request goes without the
// card, or with the one the model has. A card that works later comes as a
// note. It isn't tried again in the same turn, and
// the error is logged once until a card works again.
func TestCardFailure(t *testing.T) {
	var fail atomic.Bool
	var builds atomic.Int32
	heal := tool.Func(tool.Spec{Name: "heal", Description: "Mends the card.", Schema: json.RawMessage(`{"type": "object"}`)},
		func(context.Context, *tool.Env, struct{}) (tool.Result, error) {
			fail.Store(false)
			return tool.Result{Text: "mended"}, nil
		})
	h := newHarness(t, t.TempDir(), "", testSettings(), append(cardTools(), heal)...)
	defer h.stop()
	var log lockedBuffer
	h.o.d.Log = slog.New(slog.NewTextHandler(&log, nil))
	h.o.d.Card = func(ctx context.Context, p *project.Project) (string, error) {
		builds.Add(1)
		if fail.Load() {
			return "", errors.New("the card broke")
		}
		return realCard(ctx, p)
	}
	c := h.newChat(chat.Chat{Title: "Prices"})
	fail.Store(true)
	h.fp.Push(fake.Text("hi"))
	h.turn(c.ID, "hello") // the cut's card fails
	h.fp.Push(fake.Text("hi"))
	h.turn(c.ID, "hello") // it fails again, and isn't logged again
	fail.Store(false)
	h.fp.Push(fake.Text("hi"))
	h.turn(c.ID, "hello") // no cut, but the card is tried again
	fail.Store(true)      // the first request has no migration to rebuild for
	h.fp.Push(fake.ToolCall("c1", "migrate", struct{}{}), fake.ToolCall("c2", "heal", struct{}{}), fake.Text("done"))
	h.turn(c.ID, "make a table") // the migration's card fails, and isn't tried again in the turn
	if n := strings.Count(log.String(), "building the project card"); n != 2 {
		t.Errorf("%d errors logged, want 2, one each time it broke:\n%s", n, log.String())
	}
	h.fp.Push(fake.Text("hi"))
	h.turn(c.ID, "hello") // the next turn tries again
	calls := h.fp.Calls()
	if len(calls) != 7 {
		t.Fatalf("%d requests, want 7", len(calls))
	}
	notes := make([][]string, len(calls))
	for i, req := range calls {
		if card := cardOf(t, req); card != "" {
			t.Errorf("request %d has a card block: the cut's card failed", i+1)
		}
		notes[i] = cardNotes(req)
	}
	if len(notes[0]) != 0 || len(notes[1]) != 0 || len(notes[2]) != 1 || !strings.Contains(notes[2][0], "Tables: none yet.") {
		t.Errorf("turns 1 to 3: %q", notes[:3])
	}
	for i := 3; i < 6; i++ {
		if !slices.Equal(notes[i], notes[2]) {
			t.Errorf("request %d, with the card failing: %q", i+1, notes[i])
		}
	}
	if len(notes[6]) != 2 || !strings.Contains(notes[6][1], "- prices (0 rows)") {
		t.Errorf("in the next turn: %q", notes[6])
	}
	if n := builds.Load(); n != 5 {
		t.Errorf("%d card builds, want 5: one in each turn", n)
	}
	ms := h.messages(c.ID)
	var answers []string
	for _, m := range ms {
		if m.Role == chat.RoleAssistant && len(m.Parts) > 0 && m.Parts[0].Text != nil {
			answers = append(answers, m.Parts[0].Text.Text)
		}
	}
	if !slices.Contains(answers, "done") {
		t.Errorf("the migration's turn didn't end with its answer: %q", answers)
	}
}

// lockedBuffer is a log the runner writes while the test reads it.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// TestSpikeNotesBlock: Deps.Notes (SPIKE-034) gives block 5, chosen per
// chat, after the card, built at a cut and kept until the next one. Nil
// gives the chat's own session notes, none for a new chat.
func TestSpikeNotesBlock(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings())
	defer h.stop()
	h.o.d.Card = realCard
	var builds atomic.Int32
	h.o.d.Notes = func(_ context.Context, _ *project.Project, ch chat.Chat) (string, error) {
		return fmt.Sprintf("notes of %s, build %d", ch.Title, builds.Add(1)), nil
	}
	a := h.newChat(chat.Chat{Title: "A"})
	b := h.newChat(chat.Chat{Title: "B"})
	h.fp.Push(fake.Text("1"), fake.Text("2"), fake.Text("3"))
	h.turn(a.ID, "hello")
	h.turn(b.ID, "hello")
	h.turn(a.ID, "hello again")
	notes := func(req provider.Request) string {
		i := slices.IndexFunc(req.System, func(b provider.Block) bool { return b.Name == notesBlockName })
		if i < 0 {
			return ""
		}
		if j := slices.IndexFunc(req.System, func(b provider.Block) bool { return b.Name == cardBlock }); j < 0 || j > i {
			t.Errorf("the notes block at %d, the card at %d", i, j)
		}
		cardOf(t, req) // checks the cache point
		return req.System[i].Text
	}
	calls := h.fp.Calls()
	if len(calls) != 3 {
		t.Fatalf("%d requests, want 3", len(calls))
	}
	want := []string{"notes of A, build 1", "notes of B, build 2", "notes of A, build 1"}
	for i, req := range calls {
		if got := notes(req); got != want[i] {
			t.Errorf("request %d: notes %q, want %q", i+1, got, want[i])
		}
	}

	h.o.d.Notes = nil
	c := h.newChat(chat.Chat{Title: "C"})
	h.fp.Push(fake.Text("4"))
	h.turn(c.ID, "hello")
	if got := notes(h.fp.Calls()[3]); got != "" {
		t.Errorf("without Deps.Notes: %q", got)
	}
}

// TestSpikeNotices: Deps.Notices' notices reach the chat's next request
// once, in its turn, and nil writes nothing (SPIKE-034 round 2).
func TestSpikeNotices(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings())
	defer h.stop()
	a := h.newChat(chat.Chat{Title: "A"})
	b := h.newChat(chat.Chat{Title: "B"})
	var mu sync.Mutex
	queue := map[id.Chat][]chat.Notice{a.ID: {{Kind: NoticeSessionUpdate, Text: "Session s1 · Orders: done"}}}
	h.o.d.Notices = func(_ context.Context, _ *project.Project, ch chat.Chat) ([]chat.Notice, error) {
		mu.Lock()
		defer mu.Unlock()
		ns := queue[ch.ID]
		delete(queue, ch.ID)
		return ns, nil
	}
	h.fp.Push(fake.Text("1"), fake.Text("2"), fake.Text("3"))
	h.turn(a.ID, "hello")
	h.turn(b.ID, "hello")
	h.turn(a.ID, "hello again")
	calls := h.fp.Calls()
	has := func(req provider.Request) bool { return slices.Contains(userTexts(req), "Session s1 · Orders: done") }
	if !has(calls[0]) || has(calls[1]) {
		t.Errorf("the notice: A's first request %v, B's %v; want A's only", has(calls[0]), has(calls[1]))
	}
	var turns []int
	for _, m := range h.messages(a.ID) {
		for _, p := range m.Parts {
			if p.Notice != nil && p.Notice.Kind == NoticeSessionUpdate {
				turns = append(turns, m.Turn)
			}
		}
	}
	if !slices.Equal(turns, []int{1}) {
		t.Errorf("the notice is in turns %v of A, want [1]", turns)
	}

	h.o.d.Notices = nil
	h.fp.Push(fake.Text("4"))
	h.turn(b.ID, "hello again")
	if n := len(h.messages(b.ID)); n != 4 {
		t.Errorf("without Deps.Notices B has %d messages, want 4", n)
	}
}
