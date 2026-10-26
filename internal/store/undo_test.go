package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
)

func msg(s string) id.Source { return id.SourceOf("message", s) }

// objectsDump is the bucket index, for comparing states.
func objectsDump(t *testing.T, p *ProjectDB) string {
	t.Helper()
	return dump(t, p, "_jenab_objects", "_jenab_object_versions")
}

func undoOK(t *testing.T, p *ProjectDB, srcs []id.Source, o UndoOptions) UndoResult {
	t.Helper()
	r, err := p.Undo(context.Background(), srcs, o)
	if err != nil {
		t.Fatalf("undo (redo %v): %v; conflicts %+v", o.Redo, err, r.Conflicts)
	}
	return r
}

func putFrom(t *testing.T, p *ProjectDB, src id.Source, key, content string) Object {
	t.Helper()
	o, err := p.PutObject(context.Background(), limit.Interactive, src, key, strings.NewReader(content), PutOptions{Source: "agent"})
	if err != nil {
		t.Fatalf("put %s: %v", key, err)
	}
	return o
}

// A turn with an insert, an update, a delete and bucket puts is undone,
// redone and undone again, each time exactly; other changes stay. (The
// memory edit comes with P2-09: there is no project memory yet.)
func TestUndoAndRedo(t *testing.T) {
	p := pricesProject(t)
	ctx := context.Background()
	if _, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "prices", []Row{
		{"coin": "BTC", "date": "1", "price": 10.5, "note": []byte{0, 1, 2}},
		{"coin": "BTC", "date": "2", "price": 20},
		{"coin": "ETH", "date": "1", "price": 1},
	}, ConflictFail); err != nil {
		t.Fatal(err)
	}
	put(t, p, "notes/old.txt", "old")
	tables := []string{"prices"}
	before, objsBefore := dump(t, p, tables...), objectsDump(t, p)

	// The turn: two messages, as an assistant message per tool round.
	m1, m2 := msg("01TURN1"), msg("01TURN2")
	turn := []id.Source{m1, m2, msg("01TURNUSER")} // the user's message has no changes
	if _, err := p.Insert(ctx, limit.Interactive, m1, "prices", []Row{{"coin": "SOL", "date": "1", "price": 3}}, ConflictFail); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Update(ctx, limit.Interactive, m2, UpdateReq{Table: "prices", Set: Row{"price": 11.25, "note": "x"}, Keys: []Row{{"coin": "BTC", "date": "1"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Delete(ctx, limit.Interactive, m2, "prices", []Row{{"coin": "BTC", "date": "2"}}); err != nil {
		t.Fatal(err)
	}
	putFrom(t, p, m1, "notes/new.txt", "new")
	putFrom(t, p, m2, "notes/old.txt", "changed")
	// Another chat's change to another row stays.
	if _, err := p.Insert(ctx, limit.Interactive, msg("01OTHER"), "prices", []Row{{"coin": "ADA", "date": "1", "price": 0.5}}, ConflictFail); err != nil {
		t.Fatal(err)
	}
	after, objsAfter := dump(t, p, tables...), objectsDump(t, p)

	st, err := p.UndoState(ctx, turn)
	if err != nil {
		t.Fatal(err)
	}
	if st.Undo.Changes != 5 || st.Redo.Changes != 0 || st.Undo.Summary() != "3 rows in prices, 2 files" {
		t.Errorf("state before undo: %+v (%s)", st, st.Undo.Summary())
	}

	r := undoOK(t, p, turn, UndoOptions{})
	if len(r.Reverted) != 5 || len(r.Written) != 5 {
		t.Errorf("result %+v", r)
	}
	for i, w := range r.Written {
		c := lastChangeOf(t, p, w)
		if c.Source != "undo:"+string(r.Reverted[i]) || c.Op != "undo" {
			t.Errorf("undo entry %+v for %s", c, r.Reverted[i])
		}
	}
	want := strings.Replace(before, "prices: ", "prices: [ADA 1 0.5 <nil>] ", 1)
	if now := dump(t, p, tables...); now != want {
		t.Errorf("after undo:\n%s\nwant:\n%s", now, want)
	}
	if now := objectsDump(t, p); now != objsBefore {
		t.Errorf("objects after undo:\n%s\nwant:\n%s", now, objsBefore)
	}
	if got := read(t, p, "notes/old.txt"); got != "old" {
		t.Errorf("old.txt reads %q", got)
	}
	st, _ = p.UndoState(ctx, turn)
	if st.Undo.Changes != 0 || st.Redo.Changes != 5 {
		t.Errorf("state after undo: %+v", st)
	}
	if _, err := p.Undo(ctx, turn, UndoOptions{}); !errors.Is(err, ErrNothingToUndo) {
		t.Errorf("a second undo: %v", err)
	}

	// The sweep keeps the bytes of the undone put, so redo can bring them back.
	if _, err := p.SweepObjects(ctx); err != nil {
		t.Fatal(err)
	}
	undoOK(t, p, turn, UndoOptions{Redo: true})
	if now := dump(t, p, tables...); now != after {
		t.Errorf("after redo:\n%s\nwant:\n%s", now, after)
	}
	if now := objectsDump(t, p); now != objsAfter {
		t.Errorf("objects after redo:\n%s\nwant:\n%s", now, objsAfter)
	}
	if got := read(t, p, "notes/new.txt"); got != "new" {
		t.Errorf("new.txt reads %q", got)
	}
	undoOK(t, p, turn, UndoOptions{})
	if now := dump(t, p, tables...); now != want {
		t.Errorf("after the second undo:\n%s\nwant:\n%s", now, want)
	}
	if now := objectsDump(t, p); now != objsBefore {
		t.Errorf("objects after the second undo:\n%s", now)
	}
}

func lastChangeOf(t *testing.T, p *ProjectDB, c id.Change) changeRow {
	t.Helper()
	for _, x := range changeList(t, p) {
		if x.ID == string(c) {
			return x
		}
	}
	t.Fatalf("no change %s", c)
	return changeRow{}
}

// A later change from another chat to a row of the turn stops the undo,
// which changes nothing and lists it; skipping it undoes the rest.
func TestUndoConflictFromAnotherChat(t *testing.T) {
	p := pricesProject(t)
	ctx := context.Background()
	seedPrices(t, p, 100)
	turn := []id.Source{msg("01A")}
	if _, err := p.Update(ctx, limit.Interactive, turn[0], UpdateReq{Table: "prices", Set: Row{"note": "turn"},
		Keys: []Row{{"coin": "C1", "date": "d000000"}, {"coin": "C2", "date": "d000000"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Update(ctx, limit.Interactive, msg("01B"), UpdateReq{Table: "prices", Set: Row{"note": "other chat"},
		Keys: []Row{{"coin": "C2", "date": "d000000"}}}); err != nil {
		t.Fatal(err)
	}
	was := dump(t, p, "prices")
	n := len(changeList(t, p))
	r, err := p.Undo(ctx, turn, UndoOptions{})
	if !errors.Is(err, ErrUndoConflict) || !errors.Is(err, ErrConflict) {
		t.Fatalf("undo: %v", err)
	}
	if len(r.Conflicts) != 1 {
		t.Fatalf("conflicts %+v", r.Conflicts)
	}
	c := r.Conflicts[0]
	if c.Item != (UndoItem{"prices", `["C2","d000000"]`}) || c.Reason != ConflictChanged || c.Source != msg("01B") || c.At == nil {
		t.Errorf("conflict %+v", c)
	}
	if dump(t, p, "prices") != was || len(changeList(t, p)) != n {
		t.Error("an undo with a conflict changed something")
	}
	// Check runs it and keeps nothing.
	if r, err := p.Undo(ctx, turn, UndoOptions{Skip: []UndoItem{c.Item}, Check: true}); err != nil || len(r.Written) != 1 {
		t.Errorf("check: %+v, %v", r, err)
	}
	if dump(t, p, "prices") != was || len(changeList(t, p)) != n {
		t.Error("a checked undo changed something")
	}
	r = undoOK(t, p, turn, UndoOptions{Skip: []UndoItem{c.Item}})
	if len(r.Skipped) != 1 || r.Plan.Summary() != "1 row in prices" {
		t.Errorf("skip: %+v", r)
	}
	if v := one[string](t, p.DB, `SELECT group_concat(coin || '=' || note, ' ') FROM prices WHERE date = 'd000000' AND coin IN ('C1', 'C2')`); v != "C1=seed C2=other chat" {
		t.Errorf("after the skipping undo: %s", v)
	}
}

// Changes at or below the cleanup's watermark are history only.
func TestUndoPastWindow(t *testing.T) {
	p := pricesProject(t)
	ctx := context.Background()
	turn := []id.Source{msg("01OLD")}
	if _, err := p.Insert(ctx, limit.Interactive, turn[0], "prices", []Row{{"coin": "BTC", "date": "1"}}, ConflictFail); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := p.CleanChanges(ctx, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	st, err := p.UndoState(ctx, turn)
	if err != nil || !st.Undo.Past || st.Undo.Changes != 1 {
		t.Errorf("state %+v, %v", st, err)
	}
	if _, err := p.Undo(ctx, turn, UndoOptions{}); !errors.Is(err, ErrUndoWindow) {
		t.Errorf("undo past the window: %v", err)
	}
	if one[int](t, p.DB, "SELECT count(*) FROM prices") != 1 {
		t.Error("the refused undo changed something")
	}
}

// A migration in a turn is kept, and the rest is undone.
func TestUndoKeepsMigrations(t *testing.T) {
	p := pricesProject(t)
	ctx := context.Background()
	m := msg("01MIG")
	// A migration as P2-04 will write it: its DDL, then writeMigration's
	// change and a marker row per table it changes, in one transaction.
	if _, err := Do(ctx, p.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		if _, err := tx.Exec(`CREATE TABLE coins (sym TEXT PRIMARY KEY, name TEXT)`); err != nil {
			return struct{}{}, err
		}
		_, err := writeMigration(tx, m, []string{"coins"})
		return struct{}{}, err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Insert(ctx, limit.Interactive, m, "coins", []Row{{"sym": "BTC", "name": "Bitcoin"}}, ConflictFail); err != nil {
		t.Fatal(err)
	}
	st, _ := p.UndoState(ctx, []id.Source{m})
	if st.Migrations != 1 || st.Undo.Changes != 1 {
		t.Errorf("state %+v", st)
	}
	r := undoOK(t, p, []id.Source{m}, UndoOptions{})
	if len(r.Migrations) != 1 || len(r.Reverted) != 1 {
		t.Errorf("result %+v", r)
	}
	if one[int](t, p.DB, "SELECT count(*) FROM sqlite_schema WHERE name = 'coins'") != 1 || one[int](t, p.DB, "SELECT count(*) FROM coins") != 0 {
		t.Error("the migration was undone, or the insert wasn't")
	}

	// A turn with only a migration has nothing to undo.
	m2 := msg("01MIG2")
	if _, err := Do(ctx, p.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		_, err := writeMigration(tx, m2, []string{"coins", "prices"})
		return struct{}{}, err
	}); err != nil {
		t.Fatal(err)
	}
	if r, err := p.Undo(ctx, []id.Source{m2}, UndoOptions{}); !errors.Is(err, ErrNothingToUndo) || len(r.Migrations) != 1 {
		t.Errorf("a migration alone: %+v, %v", r, err)
	}
}

// Undo follows the row entries' rowids, not the changes' IDs: the chunks
// of different requests take turns.
func TestUndoFollowsRowids(t *testing.T) {
	p := pricesProject(t)
	ctx := context.Background()
	m := msg("01ORDER")
	// a is the older change, but b inserted the row first, and a updated it.
	a, b := newChange(m, "table:prices", "update"), newChange(m, "table:prices", "insert")
	if _, err := Do(ctx, p.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		if _, err := tx.Exec(`INSERT INTO prices VALUES ('BTC', '1', 1, 'b')`); err != nil {
			return struct{}{}, err
		}
		if err := writeChange(tx, b, []rowEntry{{table: "prices", key: `["BTC","1"]`, kind: rowInserted}}); err != nil {
			return struct{}{}, err
		}
		if _, err := tx.Exec(`UPDATE prices SET note = 'a'`); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, writeChange(tx, a, []rowEntry{{table: "prices", key: `["BTC","1"]`, kind: rowUpdated,
			before: sql.NullString{String: `{"coin":"BTC","date":"1","price":1.0,"note":"b"}`, Valid: true}}})
	}); err != nil {
		t.Fatal(err)
	}
	undoOK(t, p, []id.Source{m}, UndoOptions{})
	if n := one[int](t, p.DB, "SELECT count(*) FROM prices"); n != 0 {
		t.Errorf("%d rows after the undo; undo went by change ID", n)
	}
}

func cascadeSchema(t *testing.T, p *ProjectDB) []string {
	for _, q := range []string{
		`CREATE TABLE tp (id INTEGER PRIMARY KEY, code TEXT UNIQUE)`,
		`CREATE TABLE ta (id INTEGER PRIMARY KEY, p INTEGER REFERENCES tp ON DELETE CASCADE)`,
		`CREATE TABLE tb (id INTEGER PRIMARY KEY, a INTEGER REFERENCES ta ON DELETE CASCADE)`,
		`CREATE TABLE tr (id INTEGER PRIMARY KEY, b INTEGER REFERENCES tb ON DELETE CASCADE, p INTEGER REFERENCES tp ON DELETE CASCADE)`,
		`CREATE TABLE tn (id INTEGER PRIMARY KEY, r INTEGER REFERENCES tr ON DELETE SET NULL)`,
		`CREATE TABLE tu (id INTEGER PRIMARY KEY, code TEXT REFERENCES tp (code) ON UPDATE CASCADE ON DELETE CASCADE)`,
		`CREATE TABLE node (id INTEGER PRIMARY KEY, up INTEGER REFERENCES node ON DELETE CASCADE)`,
		`INSERT INTO tp VALUES (1, 'one'), (2, 'two')`,
		`INSERT INTO ta VALUES (10, 1), (11, 2)`,
		`INSERT INTO tb VALUES (20, 10)`,
		`INSERT INTO tr VALUES (30, 20, 1), (31, NULL, 2)`,
		`INSERT INTO tn VALUES (50, 30)`,
		`INSERT INTO tu VALUES (60, 'two')`,
		`INSERT INTO node VALUES (1, NULL), (2, 1), (3, 2), (4, 3), (5, 1), (6, 5), (7, 3)`,
	} {
		mustExec(t, p.DB, q)
	}
	return []string{"tp", "ta", "tb", "tr", "tn", "tu", "node"}
}

// A turn whose writes cascade is undone and redone exactly, also with
// foreign keys checked at each statement, so the order alone is right.
func TestUndoCascades(t *testing.T) {
	for _, deferred := range []bool{true, false} {
		t.Run(fmt.Sprintf("deferred=%v", deferred), func(t *testing.T) {
			undoDeferFK = deferred
			defer func() { undoDeferFK = true }()
			p, _ := openTestProject(t)
			ctx := context.Background()
			tables := cascadeSchema(t, p)
			was := dump(t, p, tables...)
			m := msg("01CASCADE")
			if r, err := p.Delete(ctx, limit.Interactive, m, "tp", []Row{{"id": 1}}); err != nil || r.Cascaded != 4 {
				t.Fatalf("delete: %+v, %v", r, err)
			}
			if _, err := p.Update(ctx, limit.Interactive, m, UpdateReq{Table: "tp", Set: Row{"code": "TWO"}, Keys: []Row{{"id": 2}}}); err != nil {
				t.Fatal(err)
			}
			if _, err := p.Delete(ctx, limit.Interactive, m, "node", []Row{{"id": 3}, {"id": 1}}); err != nil {
				t.Fatal(err)
			}
			if _, err := p.Insert(ctx, limit.Interactive, m, "ta", []Row{{"id": 12, "p": 2}}, ConflictFail); err != nil {
				t.Fatal(err)
			}
			if _, err := p.Insert(ctx, limit.Interactive, m, "tb", []Row{{"id": 21, "a": 12}}, ConflictFail); err != nil {
				t.Fatal(err)
			}
			after := dump(t, p, tables...)
			undoOK(t, p, []id.Source{m}, UndoOptions{})
			if now := dump(t, p, tables...); now != was {
				t.Errorf("after undo:\n%s\nwant:\n%s", now, was)
			}
			undoOK(t, p, []id.Source{m}, UndoOptions{Redo: true})
			if now := dump(t, p, tables...); now != after {
				t.Errorf("after redo:\n%s\nwant:\n%s", now, after)
			}
			undoOK(t, p, []id.Source{m}, UndoOptions{})
			if now := dump(t, p, tables...); now != was {
				t.Errorf("after the second undo:\n%s\nwant:\n%s", now, was)
			}
			if n := one[int](t, p.DB, "SELECT count(*) FROM pragma_foreign_key_check"); n != 0 {
				t.Errorf("%d foreign key violations", n)
			}
		})
	}
}

// Rows outside the undo that foreign keys tie to its rows are conflicts:
// one an action would reach, one that still refers to a row the undo
// removes, and one whose parent is gone.
func TestUndoForeignKeyConflicts(t *testing.T) {
	p, _ := openTestProject(t)
	ctx := context.Background()
	for _, q := range []string{
		`CREATE TABLE par (id INTEGER PRIMARY KEY)`,
		`CREATE TABLE kid (id INTEGER PRIMARY KEY, p INTEGER REFERENCES par ON DELETE CASCADE)`,
		`CREATE TABLE ref (id INTEGER PRIMARY KEY, p INTEGER REFERENCES par)`,
		`INSERT INTO par VALUES (9)`,
		`INSERT INTO ref VALUES (90, 9)`,
	} {
		mustExec(t, p.DB, q)
	}
	turn := []id.Source{msg("01T")}
	other := msg("01O")
	mustInsert := func(src id.Source, tab string, r Row) {
		t.Helper()
		if _, err := p.Insert(ctx, limit.Interactive, src, tab, []Row{r}, ConflictFail); err != nil {
			t.Fatal(err)
		}
	}
	mustInsert(turn[0], "par", Row{"id": 1})
	mustInsert(turn[0], "par", Row{"id": 2})
	if _, err := p.Delete(ctx, limit.Interactive, turn[0], "ref", []Row{{"id": 90}}); err != nil {
		t.Fatal(err)
	}
	mustInsert(other, "kid", Row{"id": 10, "p": 1}) // a cascade would delete it
	mustInsert(other, "ref", Row{"id": 20, "p": 2}) // it would point to nothing
	// par 9 is ref 90's parent.
	if _, err := p.Delete(ctx, limit.Interactive, other, "par", []Row{{"id": 9}}); err != nil {
		t.Fatal(err)
	}
	r, err := p.Undo(ctx, turn, UndoOptions{})
	if !errors.Is(err, ErrUndoConflict) {
		t.Fatalf("undo: %v", err)
	}
	var got []string
	for _, c := range r.Conflicts {
		s := fmt.Sprintf("%s %s%s", c.Reason, c.Item.Table, c.Item.Key)
		if c.Reached != nil {
			s += " -> " + c.Reached.Table + c.Reached.Key
		}
		got = append(got, s)
	}
	slices.Sort(got)
	want := []string{"cascade par[1] -> kid[10]", "referenced par[2] -> ref[20]", "referenced ref[90]"}
	if !slices.Equal(got, want) {
		t.Errorf("conflicts\n%v\nwant\n%v", got, want)
	}
	skip := make([]UndoItem, len(r.Conflicts))
	for i, c := range r.Conflicts {
		skip[i] = c.Item
	}
	// Skipping every item reverts and writes nothing: the turn is still in
	// effect, and its undo can be tried again (TASK-022).
	if r, err := p.Undo(ctx, turn, UndoOptions{Skip: skip}); err != nil || len(r.Written) != 0 || len(r.Skipped) == 0 {
		t.Errorf("skipping all: %+v, %v", r, err)
	}
	if st, _ := p.UndoState(ctx, turn); st.Undo.Changes == 0 || st.Redo.Changes != 0 {
		t.Errorf("state after skipping all: %+v", st)
	}
	if _, err := p.Undo(ctx, turn, UndoOptions{}); !errors.Is(err, ErrUndoConflict) {
		t.Errorf("undo again: %v", err)
	}
}

// Undo of each bucket operation, and its redo: a move takes its versions
// and the rows that refer to it back; a delete brings the object back.
func TestUndoObjects(t *testing.T) {
	p, _ := openTestProject(t)
	ctx := context.Background()
	mustExec(t, p.DB, `CREATE TABLE docs (id INTEGER PRIMARY KEY, file TEXT)`)
	mustExec(t, p.DB, `INSERT INTO _jenab_columns VALUES ('docs', 'file', 'object_ref')`)
	put(t, p, "a.txt", "a1")
	put(t, p, "a.txt", "a2")
	put(t, p, "b.txt", "old b") // a deleted object's versions stay under its key
	if err := p.DeleteObject(ctx, limit.Interactive, id.SourceUser, "b.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "docs", []Row{{"id": 1, "file": "a.txt"}}, ConflictFail); err != nil {
		t.Fatal(err)
	}
	state := func() string { return objectsDump(t, p) + "\n" + dump(t, p, "docs") }
	was := state()
	for _, step := range []struct {
		name string
		do   func(src id.Source)
	}{
		{"move", func(src id.Source) {
			if _, err := p.MoveObject(ctx, limit.Interactive, src, "a.txt", "b.txt"); err != nil {
				t.Fatal(err)
			}
		}},
		{"delete", func(src id.Source) {
			if err := p.DeleteObject(ctx, limit.Interactive, src, "a.txt"); err != nil {
				t.Fatal(err)
			}
		}},
		{"put", func(src id.Source) { putFrom(t, p, src, "a.txt", "a3") }},
		{"put same", func(src id.Source) { putFrom(t, p, src, "a.txt", "a2") }},
		{"put new", func(src id.Source) { putFrom(t, p, src, "c.txt", "c") }},
		{"put and move", func(src id.Source) {
			putFrom(t, p, src, "c.txt", "c")
			if _, err := p.MoveObject(ctx, limit.Interactive, src, "c.txt", "d/c.txt"); err != nil {
				t.Fatal(err)
			}
			if _, err := p.MoveObject(ctx, limit.Interactive, src, "a.txt", "c.txt"); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		src := msg("01" + strings.ToUpper(strings.ReplaceAll(step.name, " ", "")))
		step.do(src)
		after := state()
		undoOK(t, p, []id.Source{src}, UndoOptions{})
		if now := state(); now != was {
			t.Errorf("%s: after undo:\n%s\nwant:\n%s", step.name, now, was)
		}
		if got := read(t, p, "a.txt"); got != "a2" {
			t.Errorf("%s: a.txt reads %q", step.name, got)
		}
		undoOK(t, p, []id.Source{src}, UndoOptions{Redo: true})
		if now := state(); now != after {
			t.Errorf("%s: after redo:\n%s\nwant:\n%s", step.name, now, after)
		}
		undoOK(t, p, []id.Source{src}, UndoOptions{})
		if now := state(); now != was {
			t.Errorf("%s: after the second undo:\n%s", step.name, now)
		}
	}

	// A row outside the undo that refers to a new object stops its undo.
	src := msg("01REFS")
	putFrom(t, p, src, "e.txt", "e")
	if _, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "docs", []Row{{"id": 2, "file": "e.txt"}}, ConflictFail); err != nil {
		t.Fatal(err)
	}
	r, err := p.Undo(ctx, []id.Source{src}, UndoOptions{})
	if !errors.Is(err, ErrUndoConflict) || len(r.Conflicts) != 1 || r.Conflicts[0].Reason != ConflictReferenced ||
		*r.Conflicts[0].Reached != (UndoItem{"docs", "[2]"}) {
		t.Errorf("a referenced object: %+v, %v", r.Conflicts, err)
	}
}

// A chat's role changed by a turn is undone and redone; a later change by
// the user is a conflict.
func TestUndoRoles(t *testing.T) {
	c := openTestChats(t)
	ctx := context.Background()
	ch := newChat(t, c, chat.Chat{Title: "Prices", Role: "first"})
	m := msg("01ROLE")
	if _, err := c.SetRole(ctx, ch.ID, "second", m); err != nil {
		t.Fatal(err)
	}
	role := func() string {
		got, err := c.Chat(ctx, ch.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got.Role
	}
	undo, redo, err := split(c.RoleUndoState(ctx, []id.Source{m}))
	if err != nil || undo.Changes != 1 || redo.Changes != 0 || undo.Summary() != "1 chat role" {
		t.Errorf("state %+v %+v, %v", undo, redo, err)
	}
	r, err := c.UndoRoles(ctx, []id.Source{m}, UndoOptions{})
	if err != nil || role() != "first" || r.Seq[ch.ID] == 0 {
		t.Fatalf("undo: %+v, %v, role %q", r, err, role())
	}
	if _, err := c.UndoRoles(ctx, []id.Source{m}, UndoOptions{Redo: true}); err != nil || role() != "second" {
		t.Fatalf("redo: %v, role %q", err, role())
	}
	rc, _ := c.RoleChanges(ctx, ch.ID)
	if len(rc) != 4 || rc[2].Source != "undo:role:2" || rc[3].Source != "undo:role:3" {
		t.Errorf("role changes %+v", rc)
	}
	if _, err := c.SetRole(ctx, ch.ID, "third", id.SourceUser); err != nil {
		t.Fatal(err)
	}
	r, err = c.UndoRoles(ctx, []id.Source{m}, UndoOptions{})
	if !errors.Is(err, ErrUndoConflict) || len(r.Conflicts) != 1 || r.Conflicts[0].Source != id.SourceUser || role() != "third" {
		t.Errorf("conflict: %+v, %v, role %q", r, err, role())
	}
	if r, err := c.UndoRoles(ctx, []id.Source{m}, UndoOptions{Skip: []UndoItem{{RoleItem, string(ch.ID)}}}); err != nil || len(r.Seq) != 0 || role() != "third" {
		t.Errorf("skip: %+v, %v", r, err)
	}
	// The skipped role has no record and stays in effect (TASK-022): undo
	// is offered again, redo has nothing to do.
	rc, _ = c.RoleChanges(ctx, ch.ID)
	if last := rc[len(rc)-1]; last.Source != id.SourceUser || len(rc) != 5 {
		t.Errorf("after the skip: %+v", rc)
	}
	if undo, redo, err := split(c.RoleUndoState(ctx, []id.Source{m})); err != nil || undo.Changes != 1 || redo.Changes != 0 {
		t.Errorf("state after the skip %+v %+v, %v", undo, redo, err)
	}
	if r, err := c.UndoRoles(ctx, []id.Source{m}, UndoOptions{Redo: true}); err != nil || r.Plan.Changes != 0 {
		t.Errorf("redo of the skip: %+v, %v", r, err)
	}
	if _, err := c.UndoRoles(ctx, []id.Source{m}, UndoOptions{}); !errors.Is(err, ErrUndoConflict) {
		t.Errorf("undo again: %v", err)
	}
}

// A record that keeps the role, as undos before TASK-022 wrote for a
// skipped role, flips nothing: the role reads as in effect.
func TestUndoRolesOldSkipRecord(t *testing.T) {
	c := openTestChats(t)
	ctx := context.Background()
	ch := newChat(t, c, chat.Chat{Title: "Prices", Role: "first"})
	m := msg("01ROLE")
	if _, err := c.SetRole(ctx, ch.ID, "second", m); err != nil {
		t.Fatal(err)
	}
	if _, err := Do(ctx, c.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		return struct{}{}, recordRole(tx, ch.ID, "second", "second", roleUndoSource(2))
	}); err != nil {
		t.Fatal(err)
	}
	if undo, redo, err := split(c.RoleUndoState(ctx, []id.Source{m})); err != nil || undo.Changes != 1 || redo.Changes != 0 {
		t.Errorf("state %+v %+v, %v", undo, redo, err)
	}
	if _, err := c.UndoRoles(ctx, []id.Source{m}, UndoOptions{}); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.Chat(ctx, ch.ID); got.Role != "first" {
		t.Errorf("role %q", got.Role)
	}
	if _, err := c.UndoRoles(ctx, []id.Source{m}, UndoOptions{Redo: true}); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.Chat(ctx, ch.ID); got.Role != "second" {
		t.Errorf("role after redo %q", got.Role)
	}
}

// Turns undone newest first don't stand in each other's way over a role,
// unless the newer undo skipped it.
func TestUndoRolesNewestFirst(t *testing.T) {
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
	a, b, d := []id.Source{msg("01A")}, []id.Source{msg("01B")}, []id.Source{msg("01D")}
	for _, x := range []struct {
		src  []id.Source
		role string
	}{{a, "a"}, {b, "b"}, {d, "d"}} {
		if _, err := c.SetRole(ctx, ch.ID, x.role, x.src[0]); err != nil {
			t.Fatal(err)
		}
	}
	item := UndoItem{RoleItem, string(ch.ID)}
	// B under D: skip it. Undo D: the role is B's again, so A conflicts.
	if _, err := c.UndoRoles(ctx, b, UndoOptions{}); !errors.Is(err, ErrUndoConflict) {
		t.Fatalf("undo of B under D: %v", err)
	}
	if _, err := c.UndoRoles(ctx, b, UndoOptions{Skip: []UndoItem{item}}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.UndoRoles(ctx, d, UndoOptions{}); err != nil || role() != "b" {
		t.Fatalf("undo of D: %v, role %q", err, role())
	}
	if r, err := c.UndoRoles(ctx, a, UndoOptions{}); !errors.Is(err, ErrUndoConflict) || len(r.Conflicts) != 1 {
		t.Errorf("undo of A under B's live role: %+v, %v, role %q", r, err, role())
	}
	// Without the skip, undoing B, then A, goes through.
	ch2 := newChat(t, c, chat.Chat{Title: "Other", Role: "orig"})
	a2, b2 := []id.Source{msg("01A2")}, []id.Source{msg("01B2")}
	if _, err := c.SetRole(ctx, ch2.ID, "a", a2[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SetRole(ctx, ch2.ID, "b", b2[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := c.UndoRoles(ctx, a2, UndoOptions{}); !errors.Is(err, ErrUndoConflict) {
		t.Fatalf("undo of A under B: %v", err)
	}
	if _, err := c.UndoRoles(ctx, b2, UndoOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.UndoRoles(ctx, a2, UndoOptions{}); err != nil {
		t.Errorf("undo of A after B: %v", err)
	}
	if got, _ := c.Chat(ctx, ch2.ID); got.Role != "orig" {
		t.Errorf("role %q", got.Role)
	}
}

// N-08: undoing a 5,000-row turn takes under 1 s, on a table of 100,000
// rows; a turn of inserts, and one of upserts that half update. The time
// is checked only with JENAB_PERF_TEST=1, like the upsert's (P2-02).
func TestUndoSpeed(t *testing.T) {
	p := pricesProject(t)
	ctx := context.Background()
	seedPrices(t, p, 100_000)
	var worst time.Duration
	for i, upd := range []float64{0, 0.5} {
		m := msg(fmt.Sprintf("01SPEED%d", i))
		before := one[float64](t, p.DB, "SELECT total(price) + count(*) FROM prices")
		r, err := p.Insert(ctx, limit.Interactive, m, "prices", upsertRows(5000, upd, 0, 10_000+i*200), ConflictUpsert)
		if err != nil || r.Inserted+r.Updated != 5000 {
			t.Fatalf("%+v, %v", r, err)
		}
		start := time.Now()
		res := undoOK(t, p, []id.Source{m}, UndoOptions{})
		took := time.Since(start)
		worst = max(worst, took)
		t.Logf("undo of a 5,000-row turn, %3.0f%% updates: %v", upd*100, took.Round(time.Millisecond))
		if res.Plan.Rows[0].Rows != 5000 || one[float64](t, p.DB, "SELECT total(price) + count(*) FROM prices") != before {
			t.Errorf("the undo didn't restore the table: %+v", res.Plan)
		}
		start = time.Now()
		undoOK(t, p, []id.Source{m}, UndoOptions{Redo: true})
		t.Logf("its redo: %v", time.Since(start).Round(time.Millisecond))
	}
	if os.Getenv("JENAB_PERF_TEST") == "1" && worst > time.Second {
		t.Errorf("an undo took %v, over 1 s (N-08)", worst)
	}
}

// A change that left its rows as they were still flips with the rest: the
// undo writes its entry, so the turn reads as undone and Redo is offered.
func TestUndoNoOpChange(t *testing.T) {
	p := pricesProject(t)
	ctx := context.Background()
	if _, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "prices", []Row{{"coin": "BTC", "date": "d1", "price": 1.0, "note": "x"}}, ConflictFail); err != nil {
		t.Fatal(err)
	}
	m := msg("01NOOP")
	turn := []id.Source{m}
	if _, err := p.Insert(ctx, limit.Interactive, m, "prices", []Row{{"coin": "BTC", "date": "d1", "price": 1.0, "note": "x"}}, ConflictUpsert); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Insert(ctx, limit.Interactive, m, "prices", []Row{{"coin": "ETH", "date": "d1", "price": 2.0, "note": "y"}}, ConflictFail); err != nil {
		t.Fatal(err)
	}
	if st, _ := p.UndoState(ctx, turn); st.Undo.Changes != 2 || st.Redo.Changes != 0 {
		t.Fatalf("state %+v", st)
	}
	if r := undoOK(t, p, turn, UndoOptions{}); len(r.Written) != 2 {
		t.Errorf("undo wrote %d entries for 2 changes", len(r.Written))
	}
	if st, _ := p.UndoState(ctx, turn); st.Undo.Changes != 0 || st.Redo.Changes != 2 {
		t.Errorf("state after the undo %+v", st)
	}
	if one[int](t, p.DB, "SELECT count(*) FROM prices") != 1 {
		t.Error("the insert wasn't undone")
	}
	undoOK(t, p, turn, UndoOptions{Redo: true})
	if st, _ := p.UndoState(ctx, turn); st.Undo.Changes != 2 || st.Redo.Changes != 0 || one[int](t, p.DB, "SELECT count(*) FROM prices") != 2 {
		t.Errorf("state after the redo %+v", st)
	}
}

// Turns undone newest first don't stand in each other's way: a later
// change whose chain is undone is in effect no more. A redone one is.
func TestUndoNewestFirst(t *testing.T) {
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
	if _, err := p.Undo(ctx, a, UndoOptions{}); !errors.Is(err, ErrUndoConflict) {
		t.Fatalf("undo of A under B: %v", err)
	}
	undoOK(t, p, b, UndoOptions{})
	undoOK(t, p, a, UndoOptions{})
	if got() != "orig" {
		t.Fatalf("after undoing B, then A: %q", got())
	}
	// B's values were the newest; A's undo only took back what B had
	// written over, so redoing B puts B's values back.
	undoOK(t, p, b, UndoOptions{Redo: true})
	if got() != "B" {
		t.Errorf("after redoing B: %q", got())
	}
	// Now B is in effect again, and redoing A would write over it.
	if r, err := p.Undo(ctx, a, UndoOptions{Redo: true}); !errors.Is(err, ErrUndoConflict) || len(r.Conflicts) != 1 {
		t.Errorf("redo of A under redone B: %+v, %v", r.Conflicts, err)
	}
}

// A chain undone with the item skipped didn't put the item back, so its
// change still counts: undoing A under B's live value is a conflict.
func TestUndoSkippedChainCounts(t *testing.T) {
	p := pricesProject(t)
	ctx := context.Background()
	if _, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "prices", []Row{{"coin": "BTC", "date": "d1", "price": 0.0}}, ConflictFail); err != nil {
		t.Fatal(err)
	}
	a, b, c := []id.Source{msg("01TURNA")}, []id.Source{msg("01TURNB")}, []id.Source{msg("01TURNC")}
	for i, src := range [][]id.Source{a, b, c} {
		if _, err := p.Insert(ctx, limit.Interactive, src[0], "prices", []Row{{"coin": "BTC", "date": "d1", "price": float64(i + 1)}}, ConflictUpsert); err != nil {
			t.Fatal(err)
		}
	}
	price := func() float64 { return one[float64](t, p.DB, "SELECT price FROM prices") }
	r, err := p.Undo(ctx, b, UndoOptions{})
	if !errors.Is(err, ErrUndoConflict) || len(r.Conflicts) != 1 {
		t.Fatalf("undo of B under C: %+v, %v", r.Conflicts, err)
	}
	undoOK(t, p, b, UndoOptions{Skip: []UndoItem{r.Conflicts[0].Item}})
	undoOK(t, p, c, UndoOptions{})
	if price() != 2 {
		t.Fatalf("after undoing C: %v", price())
	}
	if r, err := p.Undo(ctx, a, UndoOptions{}); !errors.Is(err, ErrUndoConflict) || len(r.Conflicts) != 1 {
		t.Errorf("undo of A under B's live value: %+v, %v; price %v", r.Conflicts, err, price())
	}
}

// Skipping a move's object skips the whole move, the rows that point to
// it included, and the object's conflict is listed once.
func TestUndoSkipsWholeMove(t *testing.T) {
	p, _ := openTestProject(t)
	ctx := context.Background()
	mustExec(t, p.DB, `CREATE TABLE docs (id INTEGER PRIMARY KEY, file TEXT)`)
	mustExec(t, p.DB, `INSERT INTO _jenab_columns VALUES ('docs', 'file', 'object_ref')`)
	put(t, p, "a.txt", "a1")
	if _, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "docs", []Row{{"id": 1, "file": "a.txt"}}, ConflictFail); err != nil {
		t.Fatal(err)
	}
	m := msg("01MOVE")
	turn := []id.Source{m}
	if _, err := p.MoveObject(ctx, limit.Interactive, m, "a.txt", "b.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Insert(ctx, limit.Interactive, m, "docs", []Row{{"id": 2, "file": nil}}, ConflictFail); err != nil {
		t.Fatal(err)
	}
	putFrom(t, p, msg("01OTHER"), "b.txt", "new b")
	r, err := p.Undo(ctx, turn, UndoOptions{})
	if !errors.Is(err, ErrUndoConflict) || len(r.Conflicts) != 1 || r.Conflicts[0].Item != (UndoItem{"_jenab_objects", "object:b.txt"}) {
		t.Fatalf("conflicts %+v, %v", r.Conflicts, err)
	}
	r = undoOK(t, p, turn, UndoOptions{Skip: []UndoItem{r.Conflicts[0].Item}})
	if f := one[string](t, p.DB, "SELECT file FROM docs WHERE id = 1"); f != "b.txt" {
		t.Errorf("docs 1 points to %q, but the move stayed", f)
	}
	if one[int](t, p.DB, "SELECT count(*) FROM docs WHERE id = 2") != 0 {
		t.Error("the rest of the turn wasn't undone")
	}
	if s := r.Plan.Summary(); s != "1 row in docs" {
		t.Errorf("summary %q", s)
	}
	if len(r.Skipped) != 3 { // object:a.txt, object:b.txt and docs[1]
		t.Errorf("skipped %v", r.Skipped)
	}
}

// Skipping a table as a whole skips a move whose reference rows are in
// it, in the same round: no third dialog.
func TestUndoTableSkipWidensMove(t *testing.T) {
	p, _ := openTestProject(t)
	ctx := context.Background()
	mustExec(t, p.DB, `CREATE TABLE docs (id INTEGER PRIMARY KEY, file TEXT)`)
	mustExec(t, p.DB, `CREATE TABLE notes (id INTEGER PRIMARY KEY)`)
	mustExec(t, p.DB, `INSERT INTO _jenab_columns VALUES ('docs', 'file', 'object_ref')`)
	put(t, p, "a.txt", "a1")
	if _, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "docs", []Row{{"id": 1, "file": "a.txt"}}, ConflictFail); err != nil {
		t.Fatal(err)
	}
	m := msg("01MOVE")
	turn := []id.Source{m}
	if _, err := p.MoveObject(ctx, limit.Interactive, m, "a.txt", "b.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Insert(ctx, limit.Interactive, m, "notes", []Row{{"id": 1}}, ConflictFail); err != nil {
		t.Fatal(err)
	}
	if _, err := Do(ctx, p.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		_, err := writeMigration(tx, id.SourceOf("migration", "01M"), []string{"docs"})
		return struct{}{}, err
	}); err != nil {
		t.Fatal(err)
	}
	r, err := p.Undo(ctx, turn, UndoOptions{})
	if !errors.Is(err, ErrUndoConflict) || len(r.Conflicts) != 1 || r.Conflicts[0].Item != (UndoItem{Table: "docs"}) {
		t.Fatalf("conflicts %+v, %v", r.Conflicts, err)
	}
	r = undoOK(t, p, turn, UndoOptions{Skip: []UndoItem{{Table: "docs"}}})
	if f := one[string](t, p.DB, "SELECT file FROM docs WHERE id = 1"); f != "b.txt" {
		t.Errorf("docs 1 points to %q, but the move stayed", f)
	}
	if k := one[string](t, p.DB, "SELECT group_concat(key) FROM _jenab_objects"); k != "b.txt" {
		t.Errorf("objects %q, but the move stayed", k)
	}
	if one[int](t, p.DB, "SELECT count(*) FROM notes") != 0 {
		t.Error("the rest of the turn wasn't undone")
	}
	if len(r.Skipped) != 3 { // docs[1], object:a.txt and object:b.txt
		t.Errorf("skipped %v", r.Skipped)
	}
}

// A migration of a table after the turn's change to it stands in the way
// of the table's rows: its marker entry comes later, whatever its ID. The
// migration's change here gets an ID from before the turn, so its ULID is
// the smaller one, and is written after the turn's rows, as a long
// migration would be.
func TestUndoLaterMigration(t *testing.T) {
	p := pricesProject(t)
	ctx := context.Background()
	mustExec(t, p.DB, `CREATE TABLE coins (sym TEXT PRIMARY KEY)`)
	early := id.Change(id.New()) // the migration started before the turn
	m := msg("01BEFOREMIG")
	turn := []id.Source{m}
	if _, err := p.Insert(ctx, limit.Interactive, m, "prices", []Row{{"coin": "BTC", "date": "d1", "price": 1.0}}, ConflictFail); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Insert(ctx, limit.Interactive, m, "coins", []Row{{"sym": "BTC"}}, ConflictFail); err != nil {
		t.Fatal(err)
	}
	// migrate writes a migration with writeMigration; with an ID, the
	// change gets it, as if made before it was written.
	migrate := func(src string, as id.Change, tables ...string) {
		if _, err := Do(ctx, p.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
			c, err := writeMigration(tx, id.SourceOf("migration", src), tables)
			if err == nil && as != "" {
				if _, err = tx.Exec(`UPDATE _jenab_changes SET id = ? WHERE id = ?`, as, c); err == nil {
					_, err = tx.Exec(`UPDATE _jenab_change_rows SET change_id = ? WHERE change_id = ?`, as, c)
				}
			}
			return struct{}{}, err
		}); err != nil {
			t.Fatal(err)
		}
	}
	migrate("01OTHER", "", "other")
	undoOK(t, p, turn, UndoOptions{Check: true}) // another table's migration is no conflict
	migrate("01M", early, "prices")
	r, err := p.Undo(ctx, turn, UndoOptions{})
	if !errors.Is(err, ErrUndoConflict) || len(r.Conflicts) != 1 {
		t.Fatalf("conflicts %+v, %v", r.Conflicts, err)
	}
	if c := r.Conflicts[0]; c.Item != (UndoItem{Table: "prices"}) || c.Reason != ConflictSchema || c.Source != "migration:01M" {
		t.Errorf("conflict %+v", c)
	}
	undoOK(t, p, turn, UndoOptions{Skip: []UndoItem{{Table: "prices"}}})
	if one[int](t, p.DB, "SELECT count(*) FROM prices") != 1 || one[int](t, p.DB, "SELECT count(*) FROM coins") != 0 {
		t.Error("prices wasn't left, or coins wasn't undone")
	}
}

// An undo over MaxUndoEntries row entries is refused, saying how many. A
// turn of MaxUndoEntries rows, or JENAB_UNDO_ROWS, is measured with
// JENAB_PERF_TEST=1.
func TestUndoTooBig(t *testing.T) {
	p := pricesProject(t)
	ctx := context.Background()
	n := 1000
	if os.Getenv("JENAB_PERF_TEST") == "1" {
		n = MaxUndoEntries
		if v, err := strconv.Atoi(os.Getenv("JENAB_UNDO_ROWS")); err == nil {
			n = v
		}
		seedPrices(t, p, 100_000)
	}
	m := msg("01BIG")
	turn := []id.Source{m}
	if _, err := p.Insert(ctx, limit.Interactive, m, "prices", upsertRows(n, 0.5, 0, 10_000), ConflictUpsert); err != nil {
		t.Fatal(err)
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	alloc := ms.TotalAlloc
	start := time.Now()
	undoOK(t, p, turn, UndoOptions{})
	took := time.Since(start)
	runtime.ReadMemStats(&ms)
	t.Logf("undo of %d rows: %v, %d MiB allocated", n, took.Round(time.Millisecond), (ms.TotalAlloc-alloc)>>20)
	start = time.Now()
	undoOK(t, p, turn, UndoOptions{Redo: true})
	t.Logf("its redo: %v", time.Since(start).Round(time.Millisecond))

	old := maxUndoEntries
	maxUndoEntries = n - 1
	defer func() { maxUndoEntries = old }()
	st, _ := p.UndoState(ctx, turn)
	if !st.Undo.TooBig || st.Undo.Entries != n {
		t.Errorf("state %+v", st.Undo)
	}
	_, err := p.Undo(ctx, turn, UndoOptions{})
	if !errors.Is(err, ErrUndoTooBig) || err.Error() != fmt.Sprintf("this turn changed %d rows; undo can revert at most %d at once", n, n-1) {
		t.Errorf("err %v", err)
	}
}

// A REAL ±Inf in a before-image, which SQLite's JSON writes as 9.0e+999,
// comes back as it was.
func TestUndoInfinity(t *testing.T) {
	p := pricesProject(t)
	ctx := context.Background()
	mustExec(t, p.DB, `INSERT INTO prices VALUES ('BTC', 'd1', 9e999, 'up'), ('ETH', 'd1', -9e999, 'down')`)
	m := msg("01INF")
	if _, err := p.Insert(ctx, limit.Interactive, m, "prices", []Row{{"coin": "BTC", "date": "d1", "price": 1.0}, {"coin": "ETH", "date": "d1", "price": 2.0}}, ConflictUpsert); err != nil {
		t.Fatal(err)
	}
	undoOK(t, p, []id.Source{m}, UndoOptions{})
	if got := one[string](t, p.DB, "SELECT group_concat(price, ' ') FROM (SELECT price FROM prices ORDER BY coin)"); got != "Inf -Inf" {
		t.Errorf("prices %q", got)
	}
	// Only stored before-images may hold it: a tool's number that
	// overflows is still an error.
	for _, n := range []json.Number{"1e400", "-1e400"} {
		if _, err := p.Insert(ctx, limit.Interactive, m, "prices", []Row{{"coin": "SOL", "date": "d1", "price": n}}, ConflictFail); err == nil {
			t.Errorf("%s was stored", n)
		}
	}
}

// Summary, which the model reads, has the names as they are; Display, for
// the UI, isolates each table name, so a Persian one keeps the English
// sentence around it in order.
func TestUndoSummaryIsolates(t *testing.T) {
	p := UndoPlan{Rows: []TableRows{{Table: "قیمت‌ها", Rows: 12}, {Table: "_jenab_objects", Rows: 1}, {Table: "news", Rows: 1}}}
	if s := p.Summary(); s != "12 rows in قیمت‌ها, 1 file, 1 row in news" {
		t.Errorf("summary %q", s)
	}
	if s := p.Display(); s != "12 rows in ⁨قیمت‌ها⁩, 1 file, 1 row in ⁨news⁩" {
		t.Errorf("display %q", s)
	}
}
