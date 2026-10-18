package agent

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
)

func TestUndoTurn(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings())
	defer h.stop()
	ctx := context.Background()
	c := h.newChat(chat.Chat{Title: "Prices"})
	other := h.newChat(chat.Chat{Title: "Other"})
	state := func() (n int, role string) {
		h.open(func(p *project.Project) {
			ns, err := store.Query(ctx, p.DB.DB, "SELECT count(*) FROM prices", nil, func(r *sql.Rows) (int, error) {
				var n int
				return n, r.Scan(&n)
			})
			if err != nil {
				t.Fatal(err)
			}
			n = ns[0]
			info, err := p.Chats.Chat(ctx, c.ID)
			if err != nil {
				t.Fatal(err)
			}
			role = info.Role
		})
		return n, role
	}
	lastNotice := func(ch id.Chat) string {
		got := texts(h.messages(ch))
		return got[len(got)-1]
	}
	// Turn 1 adds three rows and sets the chat's role, as its tools would.
	m := h.send(c.ID, "add prices")
	h.wait()
	src := id.SourceOf("message", string(m))
	h.open(func(p *project.Project) {
		if _, err := store.Do(ctx, p.DB.DB, limit.Interactive, func(tx *sql.Tx) (int, error) {
			_, err := tx.Exec("CREATE TABLE prices (id INTEGER PRIMARY KEY, price REAL)")
			return 0, err
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := p.DB.Insert(ctx, limit.Interactive, src, "prices", []store.Row{{"id": 1, "price": 10}, {"id": 2, "price": 20}, {"id": 3, "price": 30}}, store.ConflictFail); err != nil {
			t.Fatal(err)
		}
		if _, err := p.Chats.SetRole(ctx, c.ID, "an analyst", src); err != nil {
			t.Fatal(err)
		}
	})

	st, err := h.o.UndoState(ctx, h.pid, c.ID, 1)
	if err != nil || st.Undo.Changes != 2 || st.Undo.Summary != "3 rows in prices, 1 chat role" || st.Redo.Changes != 0 ||
		st.Undo.Display != "3 rows in ⁨prices⁩, 1 chat role" {
		t.Fatalf("%+v, %v", st, err)
	}
	r, err := h.o.UndoTurn(ctx, h.pid, c.ID, UndoRequest{Turn: 1})
	if err != nil || !r.Done || len(r.Conflicts) != 0 {
		t.Fatalf("undo: %+v, %v", r, err)
	}
	if n, role := state(); n != 0 || role != "" {
		t.Errorf("after the undo: %d rows, role %q", n, role)
	}
	if got := lastNotice(c.ID); got != "user: [turn 1 was undone: 3 rows in prices, 1 chat role]" {
		t.Errorf("notice %q", got)
	}
	if st, _ := h.o.UndoState(ctx, h.pid, c.ID, 1); st.Undo.Changes != 0 || st.Redo.Changes != 2 {
		t.Errorf("state after the undo: %+v", st)
	}
	if r, err := h.o.UndoTurn(ctx, h.pid, c.ID, UndoRequest{Turn: 1, Redo: true}); err != nil || !r.Done {
		t.Fatalf("redo: %+v, %v", r, err)
	}
	if n, role := state(); n != 3 || role != "an analyst" {
		t.Errorf("after the redo: %d rows, role %q", n, role)
	}
	if got := lastNotice(c.ID); got != "user: [turn 1 was redone: 3 rows in prices, 1 chat role]" {
		t.Errorf("notice %q", got)
	}

	// Another chat's turn changes row 2: a conflict, labelled with the chat.
	m2 := h.send(other.ID, "fix a price")
	h.wait()
	h.open(func(p *project.Project) {
		if _, err := p.DB.Insert(ctx, limit.Interactive, id.SourceOf("message", string(m2)), "prices", []store.Row{{"id": 2, "price": 21}}, store.ConflictUpsert); err != nil {
			t.Fatal(err)
		}
	})
	r, err = h.o.UndoTurn(ctx, h.pid, c.ID, UndoRequest{Turn: 1})
	if err != nil || r.Done || len(r.Conflicts) != 1 {
		t.Fatalf("undo with a conflict: %+v, %v", r, err)
	}
	x := r.Conflicts[0]
	if x.Conflict.Item != (store.UndoItem{Table: "prices", Key: "[2]"}) || x.Chat != other.ID || x.ChatTitle != "Other" {
		t.Errorf("conflict %+v", x)
	}
	if n, role := state(); n != 3 || role != "an analyst" {
		t.Errorf("a refused undo changed things: %d rows, role %q", n, role)
	}
	r, err = h.o.UndoTurn(ctx, h.pid, c.ID, UndoRequest{Turn: 1, Skip: []store.UndoItem{x.Conflict.Item}})
	if err != nil || !r.Done || r.Skipped != 1 {
		t.Fatalf("undo skipping the conflict: %+v, %v", r, err)
	}
	if n, role := state(); n != 1 || role != "" {
		t.Errorf("after skipping: %d rows, role %q", n, role)
	}
	if got := lastNotice(c.ID); got != "user: [turn 1 was undone: 2 rows in prices, 1 chat role; 1 item changed since was left as it is]" {
		t.Errorf("notice %q", got)
	}
	// The notices sit in the chat's last turn; a new turn sees them.
	h.turn(c.ID, "what changed?")
	calls := h.fp.Calls()
	got := texts(calls[len(calls)-1].Messages)
	if !slices.Contains(got, "user: [turn 1 was undone: 3 rows in prices, 1 chat role]") {
		t.Errorf("the model didn't get the notice: %q", got)
	}
	// No bidi isolates reach the model, which might copy them into SQL.
	for _, m := range got {
		if strings.ContainsAny(m, "⁦⁧⁨⁩") {
			t.Errorf("isolates in %q", m)
		}
	}
}

// turnWithRole makes turn 1 of a new chat insert a row and set the chat's
// role, as its tools would.
func turnWithRole(t *testing.T, h *harness) chat.Chat {
	t.Helper()
	ctx := context.Background()
	c := h.newChat(chat.Chat{Title: "Prices"})
	src := id.SourceOf("message", string(h.send(c.ID, "go")))
	h.wait()
	h.open(func(p *project.Project) {
		if _, err := store.Do(ctx, p.DB.DB, limit.Interactive, func(tx *sql.Tx) (int, error) {
			_, err := tx.Exec("CREATE TABLE prices (id INTEGER PRIMARY KEY, price REAL)")
			return 0, err
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := p.DB.Insert(ctx, limit.Interactive, src, "prices", []store.Row{{"id": 1, "price": 10}}, store.ConflictFail); err != nil {
			t.Fatal(err)
		}
		if _, err := p.Chats.SetRole(ctx, c.ID, "an analyst", src); err != nil {
			t.Fatal(err)
		}
	})
	return c
}

// A turn half undone, as a crash between project.db's undo and the roles'
// leaves it, offers both; Undo turn finishes the undo.
func TestUndoTurnHalfDone(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings())
	defer h.stop()
	ctx := context.Background()
	c := turnWithRole(t, h)
	h.open(func(p *project.Project) {
		srcs, err := turnSources(ctx, p, c.ID, 1)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := p.DB.Undo(ctx, srcs, store.UndoOptions{}); err != nil {
			t.Fatal(err)
		}
	})
	st, err := h.o.UndoState(ctx, h.pid, c.ID, 1)
	if err != nil || st.Undo.Summary != "1 chat role" || st.Redo.Summary != "1 row in prices" {
		t.Fatalf("half-done state %+v, %v", st, err)
	}
	r, err := h.o.UndoTurn(ctx, h.pid, c.ID, UndoRequest{Turn: 1})
	if err != nil || !r.Done || r.Summary != "1 chat role" {
		t.Fatalf("finishing the undo: %+v, %v", r, err)
	}
	if st, _ := h.o.UndoState(ctx, h.pid, c.ID, 1); st.Undo.Changes != 0 || st.Redo.Changes != 2 {
		t.Errorf("state after finishing %+v", st)
	}
}

// When the roles fail after project.db's undo is in, the chat still gets
// the notice of what was done, and the error says the roles weren't.
func TestUndoTurnRolesFail(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings())
	defer h.stop()
	ctx := context.Background()
	c := turnWithRole(t, h)
	old := undoRoles
	defer func() { undoRoles = old }()
	undoRoles = func(context.Context, *store.ChatsDB, []id.Source, store.UndoOptions) (store.RoleUndo, error) {
		return store.RoleUndo{}, errors.New("disk full")
	}
	r, err := h.o.UndoTurn(ctx, h.pid, c.ID, UndoRequest{Turn: 1})
	if !errors.Is(err, ErrRolesNotUndone) || !r.Done {
		t.Fatalf("%+v, %v", r, err)
	}
	got := texts(h.messages(c.ID))
	if n := got[len(got)-1]; n != "user: [turn 1 was undone: 1 row in prices]" {
		t.Errorf("notice %q", n)
	}
	undoRoles = old
	if r, err := h.o.UndoTurn(ctx, h.pid, c.ID, UndoRequest{Turn: 1}); err != nil || r.Summary != "1 chat role" {
		t.Errorf("undoing again: %+v, %v", r, err)
	}
}

// A role changed between the check and the undo is skipped, and its chain
// flips with the rest: the turn ends undone, not partly undone, so Undo
// turn doesn't offer "the rest" again and again.
func TestUndoTurnRoleRace(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings())
	defer h.stop()
	ctx := context.Background()
	c := turnWithRole(t, h)
	old := undoRoles
	defer func() { undoRoles = old }()
	raced := false
	undoRoles = func(ctx context.Context, cs *store.ChatsDB, srcs []id.Source, o store.UndoOptions) (store.RoleUndo, error) {
		if !raced {
			raced = true
			if _, err := cs.SetRole(ctx, c.ID, "changed meanwhile", id.SourceUser); err != nil {
				t.Fatal(err)
			}
		}
		return cs.UndoRoles(ctx, srcs, o)
	}
	r, err := h.o.UndoTurn(ctx, h.pid, c.ID, UndoRequest{Turn: 1})
	if err != nil || !r.Done || r.Skipped != 1 || r.Summary != "1 row in prices" {
		t.Fatalf("%+v, %v", r, err)
	}
	// The role's chain only keeps the role now: it doesn't count, and a
	// redo, then an undo skipping it, says nothing of it.
	st, err := h.o.UndoState(ctx, h.pid, c.ID, 1)
	if err != nil || st.Undo.Changes != 0 || st.Redo.Changes != 1 || st.Redo.Summary != "1 row in prices" {
		t.Errorf("state after the race %+v, %v", st, err)
	}
	if r, err := h.o.UndoTurn(ctx, h.pid, c.ID, UndoRequest{Turn: 1, Redo: true}); err != nil || !r.Done || r.Skipped != 0 {
		t.Errorf("redo: %+v, %v", r, err)
	}
	r, err = h.o.UndoTurn(ctx, h.pid, c.ID, UndoRequest{Turn: 1, Skip: []store.UndoItem{{Table: store.RoleItem, Key: string(c.ID)}}})
	if err != nil || !r.Done || r.Skipped != 0 {
		t.Errorf("undo skipping the no-op role: %+v, %v", r, err)
	}
	if got := texts(h.messages(c.ID)); got[len(got)-1] != "user: [turn 1 was undone: 1 row in prices]" {
		t.Errorf("notice %q", got[len(got)-1])
	}
}
