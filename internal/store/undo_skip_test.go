package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
)

// skipKind is a kind of item a turn changes and a later change stands in
// the way of, for TestUndoSkippedLater.
type skipKind struct {
	name  string
	setup func(t *testing.T, p *ProjectDB)
	turn  func(t *testing.T, p *ProjectDB, src id.Source) // changes the item
	later func(t *testing.T, p *ProjectDB, src id.Source) // changes it again
	state func(t *testing.T, p *ProjectDB) string
}

func projectMemory(t *testing.T, p *ProjectDB, name string) string {
	t.Helper()
	ss, err := p.Memory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return section(ss, name).Content
}

func editMemory(t *testing.T, p *ProjectDB, src id.Source, content string) {
	t.Helper()
	ss, err := p.Memory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpdateMemory(context.Background(), MemoryEdit{Section: "goals", Content: content,
		Expected: section(ss, "goals").Revision, Source: src}); err != nil {
		t.Fatal(err)
	}
}

var skipKinds = []skipKind{
	{
		name: "row",
		setup: func(t *testing.T, p *ProjectDB) {
			if _, err := p.Insert(context.Background(), limit.Interactive, id.SourceUser, "prices", []Row{{"coin": "BTC", "date": "x", "price": 1.0, "note": "orig"}}, ConflictFail); err != nil {
				t.Fatal(err)
			}
		},
		turn: func(t *testing.T, p *ProjectDB, src id.Source) {
			if _, err := p.Update(context.Background(), limit.Interactive, src, UpdateReq{Table: "prices", Set: Row{"note": "T"}, Keys: []Row{{"coin": "BTC", "date": "x"}}}); err != nil {
				t.Fatal(err)
			}
		},
		later: func(t *testing.T, p *ProjectDB, src id.Source) {
			if _, err := p.Update(context.Background(), limit.Interactive, src, UpdateReq{Table: "prices", Set: Row{"note": "L"}, Keys: []Row{{"coin": "BTC", "date": "x"}}}); err != nil {
				t.Fatal(err)
			}
		},
		state: func(t *testing.T, p *ProjectDB) string { return dump(t, p, "prices") },
	},
	{
		name:  "file",
		setup: func(t *testing.T, p *ProjectDB) { put(t, p, "a.txt", "a0") },
		turn:  func(t *testing.T, p *ProjectDB, src id.Source) { putFrom(t, p, src, "a.txt", "aT") },
		later: func(t *testing.T, p *ProjectDB, src id.Source) { putFrom(t, p, src, "a.txt", "aL") },
		state: func(t *testing.T, p *ProjectDB) string { return objectsDump(t, p) + read(t, p, "a.txt") },
	},
	{
		name: "object",
		setup: func(t *testing.T, p *ProjectDB) {
			put(t, p, "a.txt", "a0")
			if _, err := p.Insert(context.Background(), limit.Interactive, id.SourceUser, "docs", []Row{{"id": 1, "file": "a.txt"}}, ConflictFail); err != nil {
				t.Fatal(err)
			}
		},
		turn: func(t *testing.T, p *ProjectDB, src id.Source) {
			if _, err := p.MoveObject(context.Background(), limit.Interactive, src, "a.txt", "b.txt"); err != nil {
				t.Fatal(err)
			}
		},
		later: func(t *testing.T, p *ProjectDB, src id.Source) { putFrom(t, p, src, "b.txt", "bL") },
		state: func(t *testing.T, p *ProjectDB) string { return objectsDump(t, p) + dump(t, p, "docs") },
	},
	{
		name:  "project memory",
		setup: func(t *testing.T, p *ProjectDB) { editMemory(t, p, id.SourceUser, "O") },
		turn:  func(t *testing.T, p *ProjectDB, src id.Source) { editMemory(t, p, src, "T") },
		later: func(t *testing.T, p *ProjectDB, src id.Source) { editMemory(t, p, src, "L") },
		state: func(t *testing.T, p *ProjectDB) string { return projectMemory(t, p, "goals") },
	},
}

// An item skipped by an undo stays undoable (TASK-022): the turn reads as
// partly undone, a redo puts back only what was undone, and once the
// later change is undone, Undo turn undoes the rest. Then the turn redoes
// and undoes whole.
func TestUndoSkippedLater(t *testing.T) {
	for _, k := range skipKinds {
		t.Run(k.name, func(t *testing.T) {
			p := pricesProject(t)
			ctx := context.Background()
			mustExec(t, p.DB, `CREATE TABLE docs (id INTEGER PRIMARY KEY, file TEXT)`)
			mustExec(t, p.DB, `INSERT INTO _jenab_columns VALUES ('docs', 'file', 'object_ref')`)
			if _, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "prices", []Row{{"coin": "ETH", "date": "y", "price": 1.0, "note": "orig"}}, ConflictFail); err != nil {
				t.Fatal(err)
			}
			k.setup(t, p)
			state := func() string { return k.state(t, p) + "\n" + dump(t, p, "prices") }
			turn, later := []id.Source{msg("01TURN")}, []id.Source{msg("01LATER")}
			was := state()
			k.turn(t, p, turn[0])
			if _, err := p.Update(ctx, limit.Interactive, turn[0], UpdateReq{Table: "prices", Set: Row{"note": "T"}, Keys: []Row{{"coin": "ETH", "date": "y"}}}); err != nil {
				t.Fatal(err)
			}
			afterTurn := state()
			k.later(t, p, later[0])
			afterLater := state()

			r, err := p.Undo(ctx, turn, UndoOptions{})
			if !errors.Is(err, ErrUndoConflict) || len(r.Conflicts) == 0 {
				t.Fatalf("undo under the later change: %+v, %v", r.Conflicts, err)
			}
			var skip []UndoItem
			for _, c := range r.Conflicts {
				skip = append(skip, c.Item)
			}
			undoOK(t, p, turn, UndoOptions{Skip: skip})
			if one[string](t, p.DB, `SELECT note FROM prices WHERE coin = 'ETH'`) != "orig" {
				t.Error("the rest of the turn wasn't undone")
			}
			partly := func(when string) {
				t.Helper()
				st, err := p.UndoState(ctx, turn)
				if err != nil || st.Undo.Changes == 0 || st.Redo.Changes == 0 {
					t.Errorf("%s: not partly undone: %+v, %v", when, st, err)
				}
				if st.Redo.Summary() != "1 row in prices" {
					t.Errorf("%s: redo would put back %q", when, st.Redo.Summary())
				}
			}
			partly("after the skip")

			// Redo puts back only what was undone; the later change stays.
			undoOK(t, p, turn, UndoOptions{Redo: true})
			if now := state(); now != afterLater {
				t.Errorf("after the redo:\n%s\nwant:\n%s", now, afterLater)
			}
			if st, _ := p.UndoState(ctx, turn); st.Undo.Changes == 0 || st.Redo.Changes != 0 {
				t.Errorf("after the redo: %+v", st)
			}
			if _, err := p.Undo(ctx, turn, UndoOptions{}); !errors.Is(err, ErrUndoConflict) {
				t.Fatalf("undo still under the later change: %v", err)
			}
			undoOK(t, p, turn, UndoOptions{Skip: skip})
			partly("after the second skip")

			// The later change undone, the rest of the turn undoes.
			undoOK(t, p, later, UndoOptions{})
			r = undoOK(t, p, turn, UndoOptions{})
			if now := state(); now != was {
				t.Errorf("after undoing the rest:\n%s\nwant:\n%s", now, was)
			}
			if len(r.Skipped) != 0 {
				t.Errorf("undo of the rest skipped %v", r.Skipped)
			}
			if st, _ := p.UndoState(ctx, turn); st.Undo.Changes != 0 || st.Redo.Changes == 0 {
				t.Errorf("after undoing the rest: %+v", st)
			}
			if _, err := p.Undo(ctx, turn, UndoOptions{}); !errors.Is(err, ErrNothingToUndo) {
				t.Errorf("a third undo: %v", err)
			}

			undoOK(t, p, turn, UndoOptions{Redo: true})
			if now := state(); now != afterTurn {
				t.Errorf("after the redo of all:\n%s\nwant:\n%s", now, afterTurn)
			}
			undoOK(t, p, turn, UndoOptions{})
			if now := state(); now != was {
				t.Errorf("after undoing again:\n%s\nwant:\n%s", now, was)
			}
		})
	}
}

// A memory edit's chip reads its own item: skipped, it is in effect; once
// the rest is undone, it is undone.
func TestChangeUndoneAfterUndoRest(t *testing.T) {
	p, _ := openTestProject(t)
	ctx := context.Background()
	editMemory(t, p, id.SourceUser, "O")
	editMemory(t, p, msg("01T"), "T")
	cs := changeList(t, p)
	edit := id.Change(cs[len(cs)-1].ID)
	editMemory(t, p, msg("01L"), "L")
	item := UndoItem{memoryTable, "memory:project/goals"}
	undoOK(t, p, []id.Source{msg("01T")}, UndoOptions{Skip: []UndoItem{item}})
	if u, err := p.ChangeUndone(ctx, edit); err != nil || u {
		t.Errorf("skipped: undone %v, %v", u, err)
	}
	undoOK(t, p, []id.Source{msg("01L")}, UndoOptions{})
	if _, err := p.UndoChange(ctx, edit, UndoOptions{}); err != nil {
		t.Fatalf("the chip's undo after the skip: %v", err)
	}
	if u, err := p.ChangeUndone(ctx, edit); err != nil || !u {
		t.Errorf("undone: undone %v, %v", u, err)
	}
	if got := projectMemory(t, p, "goals"); got != "O" {
		t.Errorf("content %q", got)
	}
}

// Redoing two turns oldest first is no conflict (P2-03): undo B, undo A,
// redo A, redo B. A's undo and redo after B's undo cancel out.
func TestRedoOldestFirst(t *testing.T) {
	p := pricesProject(t)
	ctx := context.Background()
	if _, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "prices", []Row{{"coin": "BTC", "date": "d1", "price": 1.0, "note": "orig"}}, ConflictFail); err != nil {
		t.Fatal(err)
	}
	a, b := []id.Source{msg("01TURNA")}, []id.Source{msg("01TURNB")}
	note := func(n string, src []id.Source) {
		if _, err := p.Insert(ctx, limit.Interactive, src[0], "prices", []Row{{"coin": "BTC", "date": "d1", "price": 1.0, "note": n}}, ConflictUpsert); err != nil {
			t.Fatal(err)
		}
	}
	got := func() string { return one[string](t, p.DB, "SELECT note FROM prices") }
	note("A", a)
	note("B", b)
	undoOK(t, p, b, UndoOptions{})
	undoOK(t, p, a, UndoOptions{})
	undoOK(t, p, a, UndoOptions{Redo: true})
	if got() != "A" {
		t.Fatalf("after redoing A: %q", got())
	}
	undoOK(t, p, b, UndoOptions{Redo: true})
	if got() != "B" {
		t.Errorf("after redoing B: %q", got())
	}
	// And back: undo B, then A, still go through.
	undoOK(t, p, b, UndoOptions{})
	undoOK(t, p, a, UndoOptions{})
	if got() != "orig" {
		t.Errorf("after undoing both again: %q", got())
	}
}

// A chat's role skipped by an undo stays undoable, as other items: its
// chain doesn't flip, and once the later change is undone, it undoes.
func TestUndoRolesSkippedLater(t *testing.T) {
	c := openTestChats(t)
	ctx := context.Background()
	ch := newChat(t, c, chat.Chat{Title: "Prices", Role: "orig"})
	role := func() string {
		got, err := c.Chat(ctx, ch.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got.Role
	}
	turn, later := []id.Source{msg("01T")}, []id.Source{msg("01L")}
	if _, err := c.SetRole(ctx, ch.ID, "t", turn[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SetRole(ctx, ch.ID, "l", later[0]); err != nil {
		t.Fatal(err)
	}
	item := UndoItem{RoleItem, string(ch.ID)}
	if _, err := c.UndoRoles(ctx, turn, UndoOptions{Skip: []UndoItem{item}}); err != nil {
		t.Fatal(err)
	}
	if undo, redo, err := split(c.RoleUndoState(ctx, turn)); err != nil || undo.Changes != 1 || redo.Changes != 0 {
		t.Errorf("after the skip: %+v %+v, %v", undo, redo, err)
	}
	if _, err := c.UndoRoles(ctx, turn, UndoOptions{}); !errors.Is(err, ErrUndoConflict) {
		t.Errorf("undo under the later change: %v", err)
	}
	if _, err := c.UndoRoles(ctx, later, UndoOptions{}); err != nil || role() != "t" {
		t.Fatalf("undo of the later change: %v, role %q", err, role())
	}
	if _, err := c.UndoRoles(ctx, turn, UndoOptions{}); err != nil || role() != "orig" {
		t.Fatalf("undo of the rest: %v, role %q", err, role())
	}
	if _, err := c.UndoRoles(ctx, turn, UndoOptions{Redo: true}); err != nil || role() != "t" {
		t.Fatalf("redo: %v, role %q", err, role())
	}
	if _, err := c.UndoRoles(ctx, turn, UndoOptions{}); err != nil || role() != "orig" {
		t.Fatalf("undo again: %v, role %q", err, role())
	}
}

// Redoing two turns oldest first is no conflict for memory and roles
// either: undo B, undo A, redo A, redo B.
func TestRedoOldestFirstMemoryAndRoles(t *testing.T) {
	for _, m := range undoScopes(t, id.Project(id.New())) {
		t.Run(string(m.scope), func(t *testing.T) {
			content := func() string {
				t.Helper()
				ss, err := m.read()
				if err != nil {
					t.Fatal(err)
				}
				return section(ss, "goals").Content
			}
			if _, err := m.update(MemoryEdit{Section: "goals", Content: "A", Source: msg("a")}); err != nil {
				t.Fatal(err)
			}
			if _, err := m.update(MemoryEdit{Section: "goals", Content: "B", Expected: 1, Source: msg("b")}); err != nil {
				t.Fatal(err)
			}
			for _, step := range []struct {
				src  string
				redo bool
				want string
			}{{"b", false, "A"}, {"a", false, ""}, {"a", true, "A"}, {"b", true, "B"}, {"b", false, "A"}, {"a", false, ""}} {
				if n, err := m.undoTurn(msg(step.src), UndoOptions{Redo: step.redo}); err != nil || n == 0 {
					t.Fatalf("%s (redo %v): %v", step.src, step.redo, err)
				}
				if got := content(); got != step.want {
					t.Fatalf("%s (redo %v): %q, want %q", step.src, step.redo, got, step.want)
				}
			}
		})
	}
	t.Run("role", func(t *testing.T) {
		c := openTestChats(t)
		ctx := context.Background()
		ch := newChat(t, c, chat.Chat{Title: "Prices", Role: "orig"})
		a, b := []id.Source{msg("01A")}, []id.Source{msg("01B")}
		if _, err := c.SetRole(ctx, ch.ID, "a", a[0]); err != nil {
			t.Fatal(err)
		}
		if _, err := c.SetRole(ctx, ch.ID, "b", b[0]); err != nil {
			t.Fatal(err)
		}
		for _, step := range []struct {
			src  []id.Source
			redo bool
			want string
		}{{b, false, "a"}, {a, false, "orig"}, {a, true, "a"}, {b, true, "b"}, {b, false, "a"}, {a, false, "orig"}} {
			if _, err := c.UndoRoles(ctx, step.src, UndoOptions{Redo: step.redo}); err != nil {
				t.Fatalf("%s (redo %v): %v", step.src[0], step.redo, err)
			}
			if got, _ := c.Chat(ctx, ch.ID); got.Role != step.want {
				t.Fatalf("%s (redo %v): %q, want %q", step.src[0], step.redo, got.Role, step.want)
			}
		}
	})
}

// The cleanup drops an undo's rowSkipped entries with the rest of its row
// entries, so a chain it reached is history whole, even when its newest
// undo is inside the window: else a dropped skip would read as a flip and
// put back a value long undone.
func TestUndoPastAfterSkip(t *testing.T) {
	p := pricesProject(t)
	ctx := context.Background()
	if _, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "prices", []Row{
		{"coin": "BTC", "date": "x", "price": 1.0, "note": "orig"},
		{"coin": "ETH", "date": "y", "price": 1.0, "note": "orig"}}, ConflictFail); err != nil {
		t.Fatal(err)
	}
	turn, later := []id.Source{msg("01TURN")}, []id.Source{msg("01LATER")}
	if _, err := p.Update(ctx, limit.Interactive, turn[0], UpdateReq{Table: "prices", Set: Row{"note": "T"},
		Keys: []Row{{"coin": "BTC", "date": "x"}, {"coin": "ETH", "date": "y"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Update(ctx, limit.Interactive, later[0], UpdateReq{Table: "prices", Set: Row{"note": "L"}, Keys: []Row{{"coin": "BTC", "date": "x"}}}); err != nil {
		t.Fatal(err)
	}
	undoOK(t, p, turn, UndoOptions{Skip: []UndoItem{{"prices", `["BTC","x"]`}}})
	undoOK(t, p, later, UndoOptions{})
	cs := changeList(t, p)
	mark := cs[len(cs)-1].ID          // the undo of the later change
	undoOK(t, p, turn, UndoOptions{}) // the rest
	notes := func() string { return dump(t, p, "prices") }
	was := notes()

	// The base, the later change and the first undo pass the window; the
	// undo of the rest doesn't.
	mustExec(t, p.DB, `UPDATE _jenab_changes SET at = ? WHERE id <= ?`, formatTime(time.Now().Add(-100*24*time.Hour)), mark)
	if _, err := p.CleanChanges(ctx, 0); err != nil {
		t.Fatal(err)
	}
	st, err := p.UndoState(ctx, turn)
	if err != nil || (st.Undo.Changes > 0 && !st.Undo.Past) || (st.Redo.Changes > 0 && !st.Redo.Past) || st.Last != nil {
		t.Errorf("state past the window: %+v, %v", st, err)
	}
	// The turn's change is history: it reads as in effect, as it is.
	c := one[string](t, p.DB, `SELECT id FROM _jenab_changes WHERE source = ?`, string(turn[0]))
	if undone, err := p.ChangeUndone(ctx, id.Change(c)); err != nil || undone {
		t.Errorf("the change past the window reads as undone: %v, %v", undone, err)
	}
	for _, redo := range []bool{false, true} {
		if _, err := p.Undo(ctx, turn, UndoOptions{Redo: redo}); !errors.Is(err, ErrUndoWindow) && !errors.Is(err, ErrNothingToUndo) {
			t.Errorf("redo %v past the window: %v", redo, err)
		}
	}
	if now := notes(); now != was {
		t.Errorf("past the window, the rows changed:\n%s\nwant:\n%s", now, was)
	}
}

// A project whose undos were written before TASK-022 gets rowSkipped
// entries at its format update (skipEntries): an item an old undo skipped
// reads as in effect, and undoes once the later change is undone.
func TestSkipEntriesOldUndos(t *testing.T) {
	for _, k := range skipKinds {
		t.Run(k.name, func(t *testing.T) {
			ctx := context.Background()
			p, dir := openTestProject(t)
			mustExec(t, p.DB, `CREATE TABLE prices (coin TEXT, date TEXT, price REAL, note TEXT, PRIMARY KEY (coin, date))`)
			mustExec(t, p.DB, `CREATE TABLE docs (id INTEGER PRIMARY KEY, file TEXT)`)
			mustExec(t, p.DB, `INSERT INTO _jenab_columns VALUES ('docs', 'file', 'object_ref')`)
			if _, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "prices", []Row{{"coin": "ETH", "date": "y", "price": 1.0, "note": "orig"}}, ConflictFail); err != nil {
				t.Fatal(err)
			}
			k.setup(t, p)
			turn, later := []id.Source{msg("01TURN")}, []id.Source{msg("01LATER")}
			k.turn(t, p, turn[0])
			if _, err := p.Update(ctx, limit.Interactive, turn[0], UpdateReq{Table: "prices", Set: Row{"note": "T"}, Keys: []Row{{"coin": "ETH", "date": "y"}}}); err != nil {
				t.Fatal(err)
			}
			k.later(t, p, later[0])
			r, err := p.Undo(ctx, turn, UndoOptions{Check: true})
			if !errors.Is(err, ErrUndoConflict) {
				t.Fatalf("check: %v", err)
			}
			var skip []UndoItem
			for _, c := range r.Conflicts {
				skip = append(skip, c.Item)
			}
			undoOK(t, p, turn, UndoOptions{Skip: skip})

			// As the old engine left it: no rowSkipped entries, and every
			// chain of the turn flipped, one whose items were all skipped
			// by an entry with no rows.
			mustExec(t, p.DB, `DELETE FROM _jenab_change_rows WHERE kind = 's'`)
			for _, c := range changeList(t, p) {
				if c.Source != string(turn[0]) || one[int](t, p.DB, `SELECT count(*) FROM _jenab_changes WHERE source = ?`, "undo:"+c.ID) > 0 {
					continue
				}
				mustExec(t, p.DB, `INSERT INTO _jenab_changes (id, at, source, target, op) VALUES (?, ?, ?, ?, 'undo')`,
					id.New(), formatTime(time.Now()), "undo:"+c.ID, c.Target)
			}
			if st, _ := p.UndoState(ctx, turn); st.Undo.Changes != 0 {
				t.Fatalf("the old engine's state reads in effect: %+v", st)
			}
			mustExec(t, p.DB, `PRAGMA user_version = 4`)
			p.Close(ctx)
			p, err = OpenProject(ctx, dir)
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close(ctx)

			st, err := p.UndoState(ctx, turn)
			if err != nil || st.Undo.Changes == 0 || st.Redo.Summary() != "1 row in prices" {
				t.Fatalf("after the format update: %+v, %v", st, err)
			}
			for _, c := range changeList(t, p) {
				if c.Source == string(turn[0]) && c.Target == "memory:project/goals" {
					if u, err := p.ChangeUndone(ctx, id.Change(c.ID)); err != nil || u {
						t.Errorf("the chip reads the skipped edit as undone: %v", err)
					}
				}
			}
			undoOK(t, p, later, UndoOptions{})
			undoOK(t, p, turn, UndoOptions{})
			if st, _ := p.UndoState(ctx, turn); st.Undo.Changes != 0 || st.Redo.Changes == 0 {
				t.Errorf("after undoing the rest: %+v", st)
			}
			if k.name == "project memory" && projectMemory(t, p, "goals") != "O" {
				t.Errorf("memory %q", projectMemory(t, p, "goals"))
			}
		})
	}
}

// Last is the newest undo or redo: the items it skipped are those in the
// other state, so the footer offers the rest of that one.
func TestUndoLast(t *testing.T) {
	p := pricesProject(t)
	ctx := context.Background()
	var keys []Row
	for _, c := range []string{"A", "B", "C"} {
		if _, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "prices", []Row{{"coin": c, "date": "x", "price": 1.0, "note": "orig"}}, ConflictFail); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, Row{"coin": c, "date": "x"})
	}
	turn := []id.Source{msg("01TURN")}
	if _, err := p.Update(ctx, limit.Interactive, turn[0], UpdateReq{Table: "prices", Set: Row{"note": "T"}, Keys: keys}); err != nil {
		t.Fatal(err)
	}
	a, b := UndoItem{"prices", `["A","x"]`}, UndoItem{"prices", `["B","x"]`}
	state := func(when string, undo, redo int, last string) {
		t.Helper()
		st, err := p.UndoState(ctx, turn)
		if err != nil || rowsOf(st.Undo) != undo || rowsOf(st.Redo) != redo || lastOf(st) != last {
			t.Errorf("%s: %+v, last %s, %v", when, st, lastOf(st), err)
		}
	}
	state("in effect", 3, 0, "")
	undoOK(t, p, turn, UndoOptions{Skip: []UndoItem{a}})
	state("undo skipping A", 1, 2, "undo")
	undoOK(t, p, turn, UndoOptions{Redo: true, Skip: []UndoItem{b}})
	state("redo skipping B", 2, 1, "redo")
	undoOK(t, p, turn, UndoOptions{Redo: true})
	state("redone", 3, 0, "redo")
	undoOK(t, p, turn, UndoOptions{})
	undoOK(t, p, turn, UndoOptions{Redo: true, Skip: []UndoItem{a}})
	state("redo skipping A", 2, 1, "redo")
}

func rowsOf(p UndoPlan) int {
	n := 0
	for _, r := range p.Rows {
		n += r.Rows
	}
	return n
}

// lastOf names the way of st's newest undo or redo, "" for none.
func lastOf(st UndoState) string {
	switch {
	case st.Last == nil:
		return ""
	case st.Last.Redo:
		return "redo"
	}
	return "undo"
}

func split(st UndoState, err error) (undo, redo UndoPlan, _ error) { return st.Undo, st.Redo, err }

// User memory's and roles' Last: the newest entry that is no skip. A skip
// alone is no undo.
func TestUserMemoryAndRolesLast(t *testing.T) {
	ctx := context.Background()
	r := openTestRegistry(t)
	pid := id.Project(id.New())
	for i, s := range []string{"t", "b"} {
		if _, err := r.UpdateUserMemory(ctx, MemoryEdit{Section: "goals", Content: s, Expected: i, Source: msg(s)}, pid); err != nil {
			t.Fatal(err)
		}
	}
	turn := []id.Source{msg("t")}
	last := func(when, want string) {
		t.Helper()
		if st, err := r.UserMemoryUndoState(ctx, turn); err != nil || lastOf(st) != want {
			t.Errorf("%s: %+v, last %s, %v", when, st, lastOf(st), err)
		}
	}
	last("in effect", "")
	if _, err := r.UndoUserMemory(ctx, turn, pid, UndoOptions{Skip: []UndoItem{{UserMemoryItem, "goals"}}}); err != nil {
		t.Fatal(err)
	}
	last("after the skip", "")
	if _, err := r.UndoUserMemory(ctx, []id.Source{msg("b")}, pid, UndoOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.UndoUserMemory(ctx, turn, pid, UndoOptions{}); err != nil {
		t.Fatal(err)
	}
	last("undone", "undo")
	if _, err := r.UndoUserMemory(ctx, turn, pid, UndoOptions{Redo: true}); err != nil {
		t.Fatal(err)
	}
	last("redone", "redo")

	c := openTestChats(t)
	ch := newChat(t, c, chat.Chat{Title: "Prices"})
	m := msg("role")
	if _, err := c.SetRole(ctx, ch.ID, "an analyst", m); err != nil {
		t.Fatal(err)
	}
	roles := func(when, want string) {
		t.Helper()
		if st, err := c.RoleUndoState(ctx, []id.Source{m}); err != nil || lastOf(st) != want {
			t.Errorf("roles %s: %+v, last %s, %v", when, st, lastOf(st), err)
		}
	}
	roles("set", "")
	if _, err := c.UndoRoles(ctx, []id.Source{m}, UndoOptions{}); err != nil {
		t.Fatal(err)
	}
	roles("undone", "undo")
	if _, err := c.UndoRoles(ctx, []id.Source{m}, UndoOptions{Redo: true}); err != nil {
		t.Fatal(err)
	}
	roles("redone", "redo")
}

// A turn that changes a row more than once: a skipped undo leaves all of
// the row's changes in effect, and the undo of the rest reverts them
// newest first, to the row as it was before the turn.
func TestUndoSkippedLaterSameRow(t *testing.T) {
	p := pricesProject(t)
	ctx := context.Background()
	if _, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "prices", []Row{
		{"coin": "BTC", "date": "x", "price": 1.0, "note": "orig"},
		{"coin": "ETH", "date": "y", "price": 1.0, "note": "orig"}}, ConflictFail); err != nil {
		t.Fatal(err)
	}
	turn, later := []id.Source{msg("01TURN")}, []id.Source{msg("01LATER")}
	set := func(src id.Source, coin, date, note string) {
		t.Helper()
		if _, err := p.Update(ctx, limit.Interactive, src, UpdateReq{Table: "prices", Set: Row{"note": note}, Keys: []Row{{"coin": coin, "date": date}}}); err != nil {
			t.Fatal(err)
		}
	}
	notes := func() string { return dump(t, p, "prices") }
	was := notes()
	set(turn[0], "BTC", "x", "T1")
	set(turn[0], "ETH", "y", "T1")
	set(turn[0], "BTC", "x", "T2")
	afterTurn := notes()
	set(later[0], "BTC", "x", "L")
	btc := UndoItem{"prices", `["BTC","x"]`}
	if _, err := p.Undo(ctx, turn, UndoOptions{}); !errors.Is(err, ErrUndoConflict) {
		t.Fatalf("undo under the later change: %v", err)
	}
	r := undoOK(t, p, turn, UndoOptions{Skip: []UndoItem{btc}})
	if len(r.Reverted) != 1 {
		t.Errorf("the skip reverted %v, want ETH's change only", r.Reverted)
	}
	if got := one[string](t, p.DB, `SELECT note FROM prices WHERE coin = 'BTC'`); got != "L" {
		t.Errorf("BTC after the skip: %q", got)
	}
	if st, _ := p.UndoState(ctx, turn); st.Undo.Changes != 2 || rowsOf(st.Undo) != 1 || lastOf(st) != "undo" || rowsOf(st.Redo) != 1 {
		t.Errorf("after the skip: %+v", st)
	}
	undoOK(t, p, later, UndoOptions{})
	undoOK(t, p, turn, UndoOptions{})
	if now := notes(); now != was {
		t.Errorf("after the rest:\n%s\nwant:\n%s", now, was)
	}
	undoOK(t, p, turn, UndoOptions{Redo: true})
	if now := notes(); now != afterTurn {
		t.Errorf("after the redo:\n%s\nwant:\n%s", now, afterTurn)
	}
}
