package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

// The card is rebuilt at once after a migration in the turn, and not
// after a plain insert: the insert's row shows at the next cut (3.1).
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
	if !strings.Contains(cards[1], "- prices (0 rows): date TEXT NOT NULL, close REAL; key (date)") {
		t.Errorf("after the migration:\n%s", cards[1])
	}
	if cards[2] != cards[1] || cards[3] != cards[1] {
		t.Errorf("after the insert the card changed:\n%s\n---\n%s", cards[2], cards[3])
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
	if card := cardOf(t, calls[3]); !strings.Contains(card, "- prices (0 rows)") {
		t.Errorf("chat A, turn 2:\n%s", card)
	}
}

// A card that fails leaves the turn alone: the request goes without the
// card, or with the old one. It isn't tried again in the same turn, and
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
	cards := make([]string, len(calls))
	for i, req := range calls {
		cards[i] = cardOf(t, req)
	}
	if cards[0] != "" || cards[1] != "" || !strings.Contains(cards[2], "Tables: none yet.") || cards[3] != cards[2] {
		t.Errorf("turns 1 to 3: %q", cards[:4])
	}
	if cards[4] != cards[2] || cards[5] != cards[2] {
		t.Errorf("after the migration, with the card failing: %q", cards[4:6])
	}
	if !strings.Contains(cards[6], "- prices (0 rows)") {
		t.Errorf("in the next turn:\n%s", cards[6])
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
