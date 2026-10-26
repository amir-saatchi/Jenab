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
	// The skipped row stays in effect: the turn is partly undone.
	st, err = h.o.UndoState(ctx, h.pid, c.ID, 1)
	if err != nil || st.Undo.Summary != "1 row in prices" || st.Redo.Summary != "2 rows in prices, 1 chat role" || st.Undo.Left != 1 || st.Redo.Left != 0 {
		t.Fatalf("partly undone: %+v, %v", st, err)
	}
	// Skipping it again changes nothing, with no notice.
	r, err = h.o.UndoTurn(ctx, h.pid, c.ID, UndoRequest{Turn: 1, Skip: []store.UndoItem{x.Conflict.Item}})
	if err != nil || r.Done || r.Skipped != 1 || len(r.Conflicts) != 0 {
		t.Fatalf("undo skipping all: %+v, %v", r, err)
	}
	if got := lastNotice(c.ID); got != "user: [turn 1 was undone: 2 rows in prices, 1 chat role; 1 item changed since was left as it is]" {
		t.Errorf("notice after skipping all %q", got)
	}
	// Once the other chat's turn is undone, the rest of turn 1 is.
	if r, err := h.o.UndoTurn(ctx, h.pid, other.ID, UndoRequest{Turn: 1}); err != nil || !r.Done {
		t.Fatalf("undo of the other turn: %+v, %v", r, err)
	}
	if r, err := h.o.UndoTurn(ctx, h.pid, c.ID, UndoRequest{Turn: 1}); err != nil || !r.Done || r.Skipped != 0 {
		t.Fatalf("undo of the rest: %+v, %v", r, err)
	}
	if n, role := state(); n != 0 || role != "" {
		t.Errorf("after the rest: %d rows, role %q", n, role)
	}
	if got := lastNotice(c.ID); got != "user: [the rest of turn 1 was undone: 1 row in prices]" {
		t.Errorf("notice %q", got)
	}
	// Redo puts back all three, and the role.
	if r, err := h.o.UndoTurn(ctx, h.pid, c.ID, UndoRequest{Turn: 1, Redo: true}); err != nil || !r.Done {
		t.Fatalf("redo of all: %+v, %v", r, err)
	}
	if n, role := state(); n != 3 || role != "an analyst" {
		t.Errorf("after the redo of all: %d rows, role %q", n, role)
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
	fail := undoRoles
	r, err := h.o.UndoTurn(ctx, h.pid, c.ID, UndoRequest{Turn: 1})
	if !errors.Is(err, ErrRolesNotUndone) || !r.Done || err.Error() != "agent: the turn was undone, but its chat roles weren't: disk full" {
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
	// The same for a redo: the error says redone.
	undoRoles = fail
	r, err = h.o.UndoTurn(ctx, h.pid, c.ID, UndoRequest{Turn: 1, Redo: true})
	var pe *PartError
	if !errors.Is(err, ErrRolesNotUndone) || !errors.As(err, &pe) || !pe.Redo || !r.Done ||
		err.Error() != "agent: the turn was redone, but its chat roles weren't: disk full" {
		t.Fatalf("redo: %+v, %v", r, err)
	}
	undoRoles = old
	if r, err := h.o.UndoTurn(ctx, h.pid, c.ID, UndoRequest{Turn: 1, Redo: true}); err != nil || r.Summary != "1 chat role" {
		t.Errorf("redoing again: %+v, %v", r, err)
	}
}

// The turn's side comes from its newest undo or redo across the
// databases: after an undo that skipped a user memory section and a redo
// that skipped a row, the row is what's left, not the section.
func TestUndoTurnSideAcrossDatabases(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings())
	defer h.stop()
	ctx := context.Background()
	c := h.newChat(chat.Chat{Title: "Prices"})
	h.open(func(p *project.Project) {
		if _, err := store.Do(ctx, p.DB.DB, limit.Interactive, func(tx *sql.Tx) (int, error) {
			_, err := tx.Exec("CREATE TABLE prices (id INTEGER PRIMARY KEY, price REAL)")
			return 0, err
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := p.DB.Insert(ctx, limit.Interactive, id.SourceUser, "prices", []store.Row{{"id": 1, "price": 10}, {"id": 2, "price": 20}}, store.ConflictFail); err != nil {
			t.Fatal(err)
		}
	})
	// Turn 1 changes rows 1 and 2 and the user memory section goals.
	m := h.send(c.ID, "fix prices")
	h.wait()
	src := id.SourceOf("message", string(m))
	h.open(func(p *project.Project) {
		if _, err := p.DB.Update(ctx, limit.Interactive, src, store.UpdateReq{Table: "prices", Set: store.Row{"price": 0}, Keys: []store.Row{{"id": 1}, {"id": 2}}}); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := h.reg.UpdateUserMemory(ctx, store.MemoryEdit{Section: "goals", Content: "T", Source: src}, h.pid); err != nil {
		t.Fatal(err)
	}
	// A later edit of goals: the undo skips it.
	if _, err := h.reg.UpdateUserMemory(ctx, store.MemoryEdit{Section: "goals", Content: "L", Expected: 1, Source: id.SourceUser}, ""); err != nil {
		t.Fatal(err)
	}
	goals := store.UndoItem{Table: store.UserMemoryItem, Key: "goals"}
	if r, err := h.o.UndoTurn(ctx, h.pid, c.ID, UndoRequest{Turn: 1, Skip: []store.UndoItem{goals}}); err != nil || !r.Done {
		t.Fatalf("undo skipping goals: %+v, %v", r, err)
	}
	st, err := h.o.UndoState(ctx, h.pid, c.ID, 1)
	if err != nil || st.Undo.Left != 1 || st.Redo.Left != 0 || st.Undo.Summary != "1 user memory section" {
		t.Fatalf("after the undo: %+v, %v", st, err)
	}
	// A later change of row 1, while undone: the redo skips it.
	other := id.SourceOf("message", "01OTHER")
	h.open(func(p *project.Project) {
		if _, err := p.DB.Update(ctx, limit.Interactive, other, store.UpdateReq{Table: "prices", Set: store.Row{"price": 11}, Keys: []store.Row{{"id": 1}}}); err != nil {
			t.Fatal(err)
		}
	})
	row1 := store.UndoItem{Table: "prices", Key: "[1]"}
	if r, err := h.o.UndoTurn(ctx, h.pid, c.ID, UndoRequest{Turn: 1, Redo: true, Skip: []store.UndoItem{row1}}); err != nil || !r.Done {
		t.Fatalf("redo skipping row 1: %+v, %v", r, err)
	}
	st, err = h.o.UndoState(ctx, h.pid, c.ID, 1)
	if err != nil || st.Redo.Left != 1 || st.Undo.Left != 0 || st.Redo.Summary != "1 row in prices" {
		t.Fatalf("after the redo: %+v, %v", st, err)
	}
	// Once the later change is undone, the redo does the rest: row 1, and
	// not goals, which the undo left in effect.
	h.open(func(p *project.Project) {
		if _, err := p.DB.Undo(ctx, []id.Source{other}, store.UndoOptions{}); err != nil {
			t.Fatal(err)
		}
	})
	if r, err := h.o.UndoTurn(ctx, h.pid, c.ID, UndoRequest{Turn: 1, Redo: true}); err != nil || !r.Done {
		t.Fatalf("redo of the rest: %+v, %v", r, err)
	}
	got := texts(h.messages(c.ID))
	if n := got[len(got)-1]; n != "user: [the rest of turn 1 was redone: 1 row in prices]" {
		t.Errorf("notice %q", n)
	}
	if st, err := h.o.UndoState(ctx, h.pid, c.ID, 1); err != nil || st.Redo.Changes != 0 || st.Undo.Summary != "2 rows in prices, 1 user memory section" {
		t.Errorf("after the rest: %+v, %v", st, err)
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
	// The role changed meanwhile stays in effect: the turn is partly
	// undone, and Undo turn again has the user's change as a conflict.
	st, err := h.o.UndoState(ctx, h.pid, c.ID, 1)
	if err != nil || st.Undo.Summary != "1 chat role" || st.Redo.Summary != "1 row in prices" {
		t.Errorf("state after the race %+v, %v", st, err)
	}
	r, err = h.o.UndoTurn(ctx, h.pid, c.ID, UndoRequest{Turn: 1})
	if err != nil || r.Done || len(r.Conflicts) != 1 || r.Conflicts[0].Conflict.Item != (store.UndoItem{Table: store.RoleItem, Key: string(c.ID)}) {
		t.Errorf("undo of the rest: %+v, %v", r, err)
	}
	// A redo puts back only what was undone.
	if r, err := h.o.UndoTurn(ctx, h.pid, c.ID, UndoRequest{Turn: 1, Redo: true}); err != nil || !r.Done || r.Skipped != 0 || r.Summary != "1 row in prices" {
		t.Errorf("redo: %+v, %v", r, err)
	}
	if got := texts(h.messages(c.ID)); got[len(got)-1] != "user: [turn 1 was redone: 1 row in prices]" {
		t.Errorf("notice %q", got[len(got)-1])
	}
	if st, err := h.o.UndoState(ctx, h.pid, c.ID, 1); err != nil || st.Undo.Changes != 2 || st.Redo.Changes != 0 {
		t.Errorf("state after the redo %+v, %v", st, err)
	}
}
