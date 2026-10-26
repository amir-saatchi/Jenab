package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/unicode/norm"

	"github.com/amir-saatchi/jenab/internal/id"
)

func openTestRegistry(t *testing.T) *Registry {
	t.Helper()
	r, err := OpenRegistry(context.Background(), filepath.Join(t.TempDir(), "registry.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	return r
}

// section returns the section of ss named name, or a zero one.
func section(ss []MemorySection, name string) MemorySection {
	for _, s := range ss {
		if s.Section == name {
			return s
		}
	}
	return MemorySection{}
}

// memoryEditor is UpdateMemory or UpdateUserMemory, and Memory or
// UserMemory, so both scopes run the same checks.
type memoryEditor struct {
	scope  MemoryScope
	update func(MemoryEdit) (MemoryWrite, error)
	read   func() ([]MemorySection, error)
}

func editors(t *testing.T) []memoryEditor {
	ctx := context.Background()
	p, _ := openTestProject(t)
	r := openTestRegistry(t)
	return []memoryEditor{
		{ScopeProject, func(e MemoryEdit) (MemoryWrite, error) { return p.UpdateMemory(ctx, e) }, func() ([]MemorySection, error) { return p.Memory(ctx) }},
		{ScopeUser, func(e MemoryEdit) (MemoryWrite, error) { return r.UpdateUserMemory(ctx, e, "") }, func() ([]MemorySection, error) { return r.UserMemory(ctx) }},
	}
}

func TestMemoryRevisions(t *testing.T) {
	for _, m := range editors(t) {
		t.Run(string(m.scope), func(t *testing.T) {
			w, err := m.update(MemoryEdit{Section: "Preferences", Content: "Prices in EUR.", Source: msg("a")})
			if err != nil || w.Op != "save" || w.Section.Revision != 1 || w.Section.Section != "preferences" || w.Change == "" {
				t.Fatalf("first edit: %+v, %v", w, err)
			}
			// Another chat writes at revision 1.
			if _, err := m.update(MemoryEdit{Section: "preferences", Content: "Prices in EUR.\nDates as YYYY-MM-DD.", Expected: 1, Source: msg("b")}); err != nil {
				t.Fatal(err)
			}
			// The first chat still holds revision 1: refused, with the
			// current content to merge.
			_, err = m.update(MemoryEdit{Section: "preferences", Content: "Prices in EUR.\nShort answers.", Expected: 1, Source: msg("a")})
			var ce *MemoryConflictError
			if !errors.As(err, &ce) || !errors.Is(err, ErrConflict) {
				t.Fatalf("stale edit: %v", err)
			}
			if ce.Current.Revision != 2 || !strings.Contains(ce.Current.Content, "YYYY-MM-DD") {
				t.Fatalf("conflict has %+v", ce.Current)
			}
			merged := ce.Current.Content + "\nShort answers."
			w, err = m.update(MemoryEdit{Section: "preferences", Content: merged, Expected: ce.Current.Revision, Source: msg("a")})
			if err != nil || w.Section.Revision != 3 {
				t.Fatalf("merge: %+v, %v", w, err)
			}
			// The same content again changes nothing.
			if w, err := m.update(MemoryEdit{Section: "preferences", Content: merged + "\n", Expected: 3, Source: msg("a")}); err != nil || w.Op != "" || w.Section.Revision != 3 {
				t.Fatalf("same content: %+v, %v", w, err)
			}

			// Empty content deletes; the revision goes on.
			w, err = m.update(MemoryEdit{Section: "preferences", Content: "  ", Expected: 3, Source: msg("a")})
			if err != nil || w.Op != "delete" || w.Section.Revision != 4 || w.Section.Content != "" {
				t.Fatalf("delete: %+v, %v", w, err)
			}
			ss, err := m.read()
			if err != nil {
				t.Fatal(err)
			}
			if s := section(ss, "preferences"); s.Content != "" || s.Revision != 4 {
				t.Fatalf("after the delete: %+v", s)
			}
			// Deleting what isn't there changes nothing.
			if w, err := m.update(MemoryEdit{Section: "goals", Content: "", Source: msg("a")}); err != nil || w.Op != "" {
				t.Fatalf("delete of nothing: %+v, %v", w, err)
			}
			// A deleted section is written again from 0 or its revision,
			// but not from the revision before the delete.
			if _, err := m.update(MemoryEdit{Section: "preferences", Content: "x", Expected: 3, Source: msg("a")}); !errors.Is(err, ErrConflict) {
				t.Fatalf("write from before the delete: %v", err)
			}
			if w, err := m.update(MemoryEdit{Section: "preferences", Content: "Prices in USD.", Source: msg("a")}); err != nil || w.Section.Revision != 5 {
				t.Fatalf("write again: %+v, %v", w, err)
			}

			// Names.
			for _, bad := range []string{"", "a b", "a/b", strings.Repeat("x", 41)} {
				if _, err := m.update(MemoryEdit{Section: bad, Content: "x", Source: msg("a")}); !errors.Is(err, ErrInvalid) {
					t.Errorf("section %q: %v", bad, err)
				}
			}
			if w, err := m.update(MemoryEdit{Section: "ترجیح‌ها", Content: "قیمت‌ها به یورو", Source: msg("a")}); err != nil || w.Section.Section != "ترجیح‌ها" {
				t.Errorf("a Persian section: %+v, %v", w, err)
			}
			if _, err := m.update(MemoryEdit{Section: "goals", Content: "x"}); !errors.Is(err, ErrInvalid) {
				t.Errorf("an edit without a source: %v", err)
			}
		})
	}
}

func TestMemoryCaps(t *testing.T) {
	for _, m := range editors(t) {
		t.Run(string(m.scope), func(t *testing.T) {
			bytes := m.scope.Cap() * 4
			// One section just under the cap with its name counted.
			body := strings.Repeat("a", bytes-len("goals")-8)
			if _, err := m.update(MemoryEdit{Section: "goals", Content: body, Source: msg("a")}); err != nil {
				t.Fatalf("under the cap: %v", err)
			}
			// A second section takes it over: refused.
			_, err := m.update(MemoryEdit{Section: "preferences", Content: strings.Repeat("b", 100), Source: msg("a")})
			var ce *MemoryCapError
			if !errors.As(err, &ce) || ce.Cap != m.scope.Cap() || ce.Tokens <= ce.Cap || !errors.Is(err, ErrInvalid) {
				t.Fatalf("over the cap: %v", err)
			}
			// Making the big one shorter frees room.
			if _, err := m.update(MemoryEdit{Section: "goals", Content: body[:len(body)/2], Expected: 1, Source: msg("a")}); err != nil {
				t.Fatal(err)
			}
			if _, err := m.update(MemoryEdit{Section: "preferences", Content: strings.Repeat("b", 100), Source: msg("a")}); err != nil {
				t.Fatalf("after shortening: %v", err)
			}
			// Persian counts by bytes: 2 per letter.
			fa := strings.Repeat("ب", bytes/2)
			if _, err := m.update(MemoryEdit{Section: "notes", Content: fa, Source: msg("a")}); !errors.Is(err, ErrInvalid) {
				t.Fatalf("Persian over the cap: %v", err)
			}
		})
	}
}

func TestProjectMemoryLog(t *testing.T) {
	ctx := context.Background()
	p, _ := openTestProject(t)
	w, err := p.UpdateMemory(ctx, MemoryEdit{Section: "decisions", Content: "Track BTC only.", Source: msg("m1")})
	if err != nil {
		t.Fatal(err)
	}
	c := lastChangeOf(t, p, id.Change(w.Change))
	if c.Target != "memory:project/decisions" || c.Op != "save" || c.Before.Valid || !strings.Contains(c.After.String, "Track BTC only.") {
		t.Fatalf("change %+v", c)
	}
	if n := one[int](t, p.DB, `SELECT count(*) FROM _jenab_change_rows WHERE change_id = ? AND table_name = '_jenab_memory'
		AND row_key = 'memory:project/decisions' AND kind = 'i'`, w.Change); n != 1 {
		t.Fatalf("%d row entries", n)
	}
	w2, err := p.UpdateMemory(ctx, MemoryEdit{Section: "decisions", Expected: 1, Source: id.SourceUser})
	if err != nil {
		t.Fatal(err)
	}
	c = lastChangeOf(t, p, id.Change(w2.Change))
	if c.Op != "delete" || !strings.Contains(c.Before.String, "Track BTC only.") || c.Source != "user" {
		t.Fatalf("delete change %+v", c)
	}
	if n := one[int](t, p.DB, `SELECT count(*) FROM _jenab_change_rows WHERE change_id = ? AND kind = 'd'`, w2.Change); n != 1 {
		t.Fatalf("%d row entries for the delete", n)
	}
}

func TestProjectMemoryUndo(t *testing.T) {
	ctx := context.Background()
	p, _ := openTestProject(t)
	edit := func(src id.Source, content string, rev int) MemoryWrite {
		t.Helper()
		w, err := p.UpdateMemory(ctx, MemoryEdit{Section: "preferences", Content: content, Expected: rev, Source: src})
		if err != nil {
			t.Fatal(err)
		}
		return w
	}
	get := func() MemorySection {
		t.Helper()
		ss, err := p.Memory(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return section(ss, "preferences")
	}
	edit(msg("t1"), "Prices in EUR.", 0)
	edit(msg("t2"), "Prices in USD.", 1)

	// Undo turn 2: back to EUR, with a new revision.
	r := undoOK(t, p, []id.Source{msg("t2")}, UndoOptions{})
	if r.Plan.Summary() != "1 project memory section" {
		t.Errorf("summary %q", r.Plan.Summary())
	}
	if s := get(); s.Content != "Prices in EUR." || s.Revision != 3 || s.Source.Kind() != "undo" {
		t.Fatalf("after undo: %+v", s)
	}
	// An editor holding revision 2 is refused.
	if _, err := p.UpdateMemory(ctx, MemoryEdit{Section: "preferences", Content: "x", Expected: 2, Source: msg("t3")}); !errors.Is(err, ErrConflict) {
		t.Fatalf("edit from before the undo: %v", err)
	}
	// Redo.
	undoOK(t, p, []id.Source{msg("t2")}, UndoOptions{Redo: true})
	if s := get(); s.Content != "Prices in USD." || s.Revision != 4 {
		t.Fatalf("after redo: %+v", s)
	}

	// Undo of turn 1 now conflicts with turn 2's edit.
	_, err := p.Undo(ctx, []id.Source{msg("t1")}, UndoOptions{})
	if !errors.Is(err, ErrUndoConflict) {
		t.Fatalf("undo under a later edit: %v", err)
	}

	// The chip's undo reverts one edit, by its change.
	w := edit(id.SourceUser, "Prices in GBP.", 4)
	if _, err := p.UndoChange(ctx, id.Change(w.Change), UndoOptions{}); err != nil {
		t.Fatal(err)
	}
	if s := get(); s.Content != "Prices in USD." {
		t.Fatalf("after the chip's undo: %+v", s)
	}
	if _, err := p.UndoChange(ctx, id.Change(w.Change), UndoOptions{}); !errors.Is(err, ErrNothingToUndo) {
		t.Fatalf("undo twice: %v", err)
	}
	if _, err := p.UndoChange(ctx, id.Change(w.Change), UndoOptions{Redo: true}); err != nil {
		t.Fatal(err)
	}
	if s := get(); s.Content != "Prices in GBP." {
		t.Fatalf("after the chip's redo: %+v", s)
	}

	// Undo of a delete brings the section back; undo of a first write
	// deletes it.
	w = edit(msg("t5"), "", 7)
	undoOK(t, p, []id.Source{msg("t5")}, UndoOptions{})
	if s := get(); s.Content != "Prices in GBP." {
		t.Fatalf("after undoing the delete: %+v", s)
	}
	w, err = p.UpdateMemory(ctx, MemoryEdit{Section: "goals", Content: "Save more.", Source: msg("t6")})
	if err != nil {
		t.Fatal(err)
	}
	undoOK(t, p, []id.Source{msg("t6")}, UndoOptions{})
	ss, _ := p.Memory(ctx)
	if s := section(ss, "goals"); s.Content != "" || s.Revision != 2 {
		t.Fatalf("after undoing the first write: %+v", s)
	}
}

func TestUserMemoryUndo(t *testing.T) {
	ctx := context.Background()
	r := openTestRegistry(t)
	p, _ := openTestProject(t)
	pid := id.Project(id.New())
	edit := func(src id.Source, content string, rev int) MemoryWrite {
		t.Helper()
		w, err := r.UpdateUserMemory(ctx, MemoryEdit{Section: "preferences", Content: content, Expected: rev, Source: src}, pid)
		if err != nil {
			t.Fatal(err)
		}
		return w
	}
	get := func() MemorySection {
		t.Helper()
		ss, err := r.UserMemory(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return section(ss, "preferences")
	}
	edit(msg("t1"), "Prices in EUR.", 0)
	w2 := edit(msg("t2"), "Prices in USD.", 1)

	// The project's log points to the registry's entry, and project undo
	// leaves it alone.
	if err := p.LogUserMemory(ctx, w2); err != nil {
		t.Fatal(err)
	}
	if n := one[int](t, p.DB, `SELECT count(*) FROM _jenab_changes WHERE target = 'memory:user/preferences'
		AND after ->> 'user_change' = CAST(? AS INTEGER) AND source = 'message:t2'`, w2.Change); n != 1 {
		t.Fatalf("%d pointers", n)
	}
	st, err := p.UndoState(ctx, []id.Source{msg("t2")})
	if err != nil || st.Undo.Changes != 0 {
		t.Fatalf("project undo state %+v, %v", st, err)
	}

	undo, _, err := split(r.UserMemoryUndoState(ctx, []id.Source{msg("t2")}))
	if err != nil || undo.Changes != 1 || undo.Summary() != "1 user memory section" {
		t.Fatalf("undo state %+v, %v", undo, err)
	}
	if _, err := r.UndoUserMemory(ctx, []id.Source{msg("t2")}, "", UndoOptions{}); err != nil {
		t.Fatal(err)
	}
	if s := get(); s.Content != "Prices in EUR." || s.Revision != 3 {
		t.Fatalf("after undo: %+v", s)
	}
	_, redo, _ := split(r.UserMemoryUndoState(ctx, []id.Source{msg("t2")}))
	if redo.Changes != 1 {
		t.Fatalf("redo state %+v", redo)
	}
	if _, err := r.UndoUserMemory(ctx, []id.Source{msg("t2")}, "", UndoOptions{Redo: true}); err != nil {
		t.Fatal(err)
	}
	if s := get(); s.Content != "Prices in USD." || s.Revision != 4 {
		t.Fatalf("after redo: %+v", s)
	}

	// A later edit from Settings stands in the way of turn 1's undo,
	// until it is skipped.
	edit(id.SourceUser, "Prices in GBP.", 4)
	res, err := r.UndoUserMemory(ctx, []id.Source{msg("t1")}, "", UndoOptions{})
	if !errors.Is(err, ErrUndoConflict) || len(res.Conflicts) != 1 {
		t.Fatalf("undo under a later edit: %+v, %v", res, err)
	}
	if s := get(); s.Content != "Prices in GBP." {
		t.Fatalf("a refused undo changed the section: %+v", s)
	}
	res, err = r.UndoUserMemory(ctx, []id.Source{msg("t1")}, "", UndoOptions{Skip: []UndoItem{res.Conflicts[0].Item}})
	if err != nil || len(res.Skipped) != 1 {
		t.Fatalf("undo with a skip: %+v, %v", res, err)
	}
	if s := get(); s.Content != "Prices in GBP." {
		t.Fatalf("a skipped section changed: %+v", s)
	}

	// The chip's undo, by entry number.
	var n int64
	if _, err := fmt.Sscan(edit(msg("t9"), "Prices in CHF.", 5).Change, &n); err != nil {
		t.Fatal(err)
	}
	if _, err := r.UndoUserMemoryChange(ctx, n, "", UndoOptions{}); err != nil {
		t.Fatal(err)
	}
	if s := get(); s.Content != "Prices in GBP." {
		t.Fatalf("after the chip's undo: %+v", s)
	}
	if _, err := r.UndoUserMemoryChange(ctx, n, "", UndoOptions{}); !errors.Is(err, ErrNothingToUndo) {
		t.Fatalf("undo twice: %v", err)
	}
}

// undoScope is a scope's undo of a turn and of one edit, for tests that
// run on both.
type undoScope struct {
	memoryEditor
	// undoTurn returns how many edits it reverted. With none to revert,
	// project memory gives ErrNothingToUndo and user memory no error
	// (nothing).
	undoTurn   func(src id.Source, o UndoOptions) (int, error)
	nothing    error
	undoChange func(change string, redo bool) error
	undone     func(change string) bool
	item       func(section string) UndoItem
}

func undoScopes(t *testing.T, pid id.Project) []undoScope {
	ctx := context.Background()
	p, _ := openTestProject(t)
	r := openTestRegistry(t)
	num := func(change string) int64 {
		var n int64
		if _, err := fmt.Sscan(change, &n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	must := func(ok bool, err error) bool {
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	return []undoScope{
		{
			memoryEditor: memoryEditor{ScopeProject, func(e MemoryEdit) (MemoryWrite, error) { return p.UpdateMemory(ctx, e) }, func() ([]MemorySection, error) { return p.Memory(ctx) }},
			undoTurn: func(src id.Source, o UndoOptions) (int, error) {
				r, err := p.Undo(ctx, []id.Source{src}, o)
				return len(r.Reverted), err
			},
			nothing: ErrNothingToUndo,
			undoChange: func(c string, redo bool) error {
				_, err := p.UndoChange(ctx, id.Change(c), UndoOptions{Redo: redo})
				return err
			},
			undone: func(c string) bool { return must(p.ChangeUndone(ctx, id.Change(c))) },
			item:   func(s string) UndoItem { return UndoItem{memoryTable, "memory:project/" + s} },
		},
		{
			memoryEditor: memoryEditor{ScopeUser, func(e MemoryEdit) (MemoryWrite, error) { return r.UpdateUserMemory(ctx, e, pid) }, func() ([]MemorySection, error) { return r.UserMemory(ctx) }},
			undoTurn: func(src id.Source, o UndoOptions) (int, error) {
				res, err := r.UndoUserMemory(ctx, []id.Source{src}, pid, o)
				return res.Plan.Changes, err
			},
			undoChange: func(c string, redo bool) error {
				_, err := r.UndoUserMemoryChange(ctx, num(c), pid, UndoOptions{Redo: redo})
				return err
			},
			undone: func(c string) bool { return must(r.UserMemoryChangeUndone(ctx, num(c))) },
			item:   func(s string) UndoItem { return UndoItem{UserMemoryItem, s} },
		},
	}
}

// A turn that edits a section twice is undone and redone whole, and an
// edit undone since is no later change: the undo writes new revisions,
// so only the content tells whether the section is as an edit left it.
func TestMemoryUndoTwoEdits(t *testing.T) {
	pid := id.Project(id.New())
	for _, m := range undoScopes(t, pid) {
		t.Run(string(m.scope), func(t *testing.T) {
			edit := func(src id.Source, content string, rev int) MemoryWrite {
				t.Helper()
				w, err := m.update(MemoryEdit{Section: "preferences", Content: content, Expected: rev, Source: src})
				if err != nil {
					t.Fatal(err)
				}
				return w
			}
			get := func() MemorySection {
				t.Helper()
				ss, err := m.read()
				if err != nil {
					t.Fatal(err)
				}
				return section(ss, "preferences")
			}
			edit(msg("t1"), "A", 0)
			edit(msg("t1"), "B", 1)
			if n, err := m.undoTurn(msg("t1"), UndoOptions{}); err != nil || n == 0 {
				t.Fatalf("undo of two edits: %v", err)
			}
			if s := get(); s.Content != "" || s.Revision != 4 {
				t.Fatalf("after undo: %+v", s)
			}
			if n, err := m.undoTurn(msg("t1"), UndoOptions{Redo: true}); err != nil || n == 0 {
				t.Fatalf("redo of two edits: %v", err)
			}
			if s := get(); s.Content != "B" || s.Revision != 6 {
				t.Fatalf("after redo: %+v", s)
			}

			// A's turn, then B's edit, undone by its chip: A's turn undoes.
			edit(msg("a"), "one", 6)
			wb := edit(msg("b"), "two", 7)
			if m.undone(wb.Change) {
				t.Error("an edit not undone reads as undone")
			}
			if err := m.undoChange(wb.Change, false); err != nil {
				t.Fatal(err)
			}
			if !m.undone(wb.Change) {
				t.Error("an undone edit reads as not undone")
			}
			if n, err := m.undoTurn(msg("a"), UndoOptions{}); err != nil || n == 0 {
				t.Fatalf("undo past an undone edit: %v", err)
			}
			if s := get(); s.Content != "B" {
				t.Fatalf("after undoing A: %+v", s)
			}
			// The chip of B's edit, after the reload: undo is nothing to do,
			// and its state says redo.
			if err := m.undoChange(wb.Change, false); !errors.Is(err, ErrNothingToUndo) {
				t.Fatalf("undo of an undone edit: %v", err)
			}
		})
	}
}

// A memory edit whose undo skipped the section, because another chat
// edited it since, stays in effect (TASK-022): a redo leaves the section,
// an undo tries again, and once the other edit is undone the edit undoes
// and redoes. The same for user and project memory.
func TestMemoryUndoAfterSkip(t *testing.T) {
	for _, m := range undoScopes(t, id.Project(id.New())) {
		t.Run(string(m.scope), func(t *testing.T) {
			edit := func(src id.Source, content string, rev int) MemoryWrite {
				t.Helper()
				w, err := m.update(MemoryEdit{Section: "goals", Content: content, Expected: rev, Source: src})
				if err != nil {
					t.Fatal(err)
				}
				return w
			}
			want := func(when, content string) {
				t.Helper()
				ss, err := m.read()
				if err != nil {
					t.Fatal(err)
				}
				if s := section(ss, "goals"); s.Content != content {
					t.Fatalf("%s: %q, want %q", when, s.Content, content)
				}
			}
			edit(id.SourceUser, "O", 0)
			wt := edit(msg("t"), "A", 1)
			wb := edit(msg("b"), "B", 2)
			if _, err := m.undoTurn(msg("t"), UndoOptions{}); !errors.Is(err, ErrUndoConflict) {
				t.Fatalf("undo past another chat's edit: %v", err)
			}
			if n, err := m.undoTurn(msg("t"), UndoOptions{Skip: []UndoItem{m.item("goals")}}); err != nil || n != 0 {
				t.Fatalf("undo with the section skipped: %d reverted, %v", n, err)
			}
			want("after the skip", "B")
			if m.undone(wt.Change) {
				t.Error("after the skip, the edit reads as undone")
			}
			// Nothing is undone, so a redo has nothing to put back.
			if n, err := m.undoTurn(msg("t"), UndoOptions{Redo: true}); n != 0 || !errors.Is(err, m.nothing) {
				t.Fatalf("redo after the skip: %d reverted, %v", n, err)
			}
			want("after the redo", "B")
			if m.undone(wt.Change) {
				t.Error("after the redo, the edit reads as undone")
			}
			if _, err := m.undoTurn(msg("t"), UndoOptions{}); !errors.Is(err, ErrUndoConflict) {
				t.Fatalf("undo while the other edit stands: %v", err)
			}
			if err := m.undoChange(wb.Change, false); err != nil {
				t.Fatalf("undo of the other edit: %v", err)
			}
			want("after undoing the other edit", "A")
			if n, err := m.undoTurn(msg("t"), UndoOptions{}); err != nil || n == 0 {
				t.Fatalf("undo once the other edit is undone: %v", err)
			}
			want("after the undo", "O")
			if !m.undone(wt.Change) {
				t.Error("after the undo, the edit reads as not undone")
			}
			if n, err := m.undoTurn(msg("t"), UndoOptions{Redo: true}); err != nil || n == 0 {
				t.Fatalf("redo: %v", err)
			}
			want("after the redo", "A")
			if n, err := m.undoTurn(msg("t"), UndoOptions{}); err != nil || n == 0 {
				t.Fatalf("undo again: %v", err)
			}
			want("after undoing again", "O")
		})
	}
}

// A project memory edit whose undo skipped the section reads as not
// undone: it was never reverted.
func TestProjectMemoryUndoneAfterSkip(t *testing.T) {
	for _, m := range undoScopes(t, id.Project(id.New())) {
		if m.scope != ScopeProject {
			continue
		}
		edit := func(src id.Source, content string, rev int) MemoryWrite {
			t.Helper()
			w, err := m.update(MemoryEdit{Section: "goals", Content: content, Expected: rev, Source: src})
			if err != nil {
				t.Fatal(err)
			}
			return w
		}
		wt := edit(msg("t"), "A", 0)
		edit(msg("b"), "B", 1)
		if _, err := m.undoTurn(msg("t"), UndoOptions{}); !errors.Is(err, ErrUndoConflict) {
			t.Fatalf("undo past another chat's edit: %v", err)
		}
		if n, err := m.undoTurn(msg("t"), UndoOptions{Skip: []UndoItem{m.item("goals")}}); err != nil || n != 0 {
			t.Fatalf("undo with the section skipped: %v", err)
		}
		if m.undone(wt.Change) {
			t.Error("after the skip, the edit reads as undone")
		}
	}
}

// The entries of an undo name the project it was made in.
func TestUserMemoryUndoProject(t *testing.T) {
	ctx := context.Background()
	r := openTestRegistry(t)
	pid := id.Project(id.New())
	if _, err := r.UpdateUserMemory(ctx, MemoryEdit{Section: "goals", Content: "Save more.", Source: msg("t1")}, pid); err != nil {
		t.Fatal(err)
	}
	other := id.Project(id.New())
	if _, err := r.UndoUserMemory(ctx, []id.Source{msg("t1")}, other, UndoOptions{}); err != nil {
		t.Fatal(err)
	}
	var got []string
	err := r.tx(ctx, func(tx *sql.Tx) error {
		var err error
		got, err = queryTx(tx, `SELECT op || ' ' || project FROM user_memory_changes ORDER BY id`, nil, func(r *sql.Rows) (string, error) {
			var s string
			return s, r.Scan(&s)
		})
		return err
	})
	if err != nil || len(got) != 2 || got[0] != "save "+string(pid) || got[1] != "undo "+string(other) {
		t.Fatalf("%q, %v", got, err)
	}
}

// Section names: Persian words with their marks and ZWNJ, one section for
// the Arabic and Persian yeh and kaf, and no name that is only a joiner
// or a dash.
// QuoteMemory marks lines that look like a header, and UnquoteMemory
// gives back the text, lines marked before included.
func TestQuoteMemory(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"plain\n[x]", "plain\n" + `\[x]`},
		{"  [indented] ", `  \[indented] `},
		{`\[marked]`, `\\[marked]`},
		{"[a] b\n[]", "[a] b\n" + `\[]`},
		{"[سود، revision 2]", `\[سود، revision 2]`},
	} {
		if got := QuoteMemory(c.in); got != c.want {
			t.Errorf("QuoteMemory(%q) = %q, want %q", c.in, got, c.want)
		}
		if got := UnquoteMemory(c.want); got != c.in {
			t.Errorf("UnquoteMemory(%q) = %q, want %q", c.want, got, c.in)
		}
	}
}

func TestNormalizeSection(t *testing.T) {
	ok := map[string]string{
		" Preferences ":         "preferences",
		"خانهٔ":                 norm.NFC.String("خانهٔ"),
		"ترجیحات":               "ترجیحات",
		"ترجيحات":               "ترجیحات", // Arabic yeh
		"كارها":                 "کارها",   // Arabic kaf
		"می‌خواهم":              "می‌خواهم",
		"a-b_c":                 "a-b_c",
		"x":                     "x",
		strings.Repeat("ی", 40): strings.Repeat("ی", 40),
	}
	for in, want := range ok {
		if got, err := NormalizeSection(in); err != nil || got != want {
			t.Errorf("%q: %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "‌", "-", "_", "-a", "a-", "a‌", "‌می", "a b", "a/b", strings.Repeat("ی", 41), "ٔ"} {
		if got, err := NormalizeSection(in); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q: %q, %v; want ErrInvalid", in, got, err)
		}
	}
}

// The cleanup drops entries past the window; their chip's undo is refused
// as past it, and entry numbers aren't reused, so an old chip can't undo
// a new edit.
func TestCleanUserMemory(t *testing.T) {
	ctx := context.Background()
	r := openTestRegistry(t)
	w, err := r.UpdateUserMemory(ctx, MemoryEdit{Section: "goals", Content: "Save more.", Source: msg("t1")}, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := r.tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE user_memory_changes SET at = ?`, formatTime(time.Now().Add(-100*24*time.Hour)))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if n, err := r.CleanUserMemory(ctx, 0); err != nil || n != 1 {
		t.Fatalf("cleaned %d, %v", n, err)
	}
	var old int64
	fmt.Sscan(w.Change, &old)
	if _, err := r.UndoUserMemoryChange(ctx, old, "", UndoOptions{}); !errors.Is(err, ErrUndoWindow) {
		t.Fatalf("undo past the window: %v", err)
	}
	w2, err := r.UpdateUserMemory(ctx, MemoryEdit{Section: "goals", Content: "Save less.", Expected: 1, Source: msg("t2")}, "")
	if err != nil || w2.Change == w.Change {
		t.Fatalf("new entry %q after %q, %v", w2.Change, w.Change, err)
	}
	if n, err := r.CleanUserMemory(ctx, 0); err != nil || n != 0 {
		t.Fatalf("cleaned %d recent entries, %v", n, err)
	}
	if _, err := r.UndoUserMemoryChange(ctx, old+100, "", UndoOptions{}); !errors.Is(err, ErrNothingToUndo) {
		t.Fatalf("undo of an entry that never was: %v", err)
	}
}
