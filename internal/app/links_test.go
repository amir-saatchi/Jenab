package app

import (
	"context"
	"testing"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
)

// linkEvents counts the project:links events the sink got for p so far.
func (s *sink) linkEvents(p id.Project) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, e := range s.events {
		if c, ok := e.data.(project.LinksChanged); ok && e.name == EventLinks && c.Project == p {
			n++
		}
	}
	return n
}

// The Links tab's service lists, adds, edits and deletes links as the
// user; a delete comes back with its change; each write sends
// project:links; the store's refusals read as messages.
func TestLinkService(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	ls := e.svc.Links
	op, err := e.svc.Project.Create(ctx, "Reading")
	if err != nil {
		t.Fatal(err)
	}
	if l, err := ls.List(ctx, op.ID); err != nil || l == nil || len(l) != 0 {
		t.Fatalf("empty list %#v, %v", l, err)
	}
	a, err := ls.Add(ctx, op.ID, LinkInput{URL: "https://example.com/a", Title: "A", Tags: []string{"btc"}})
	if err != nil || a.Change == "" || a.Link.CreatedBy != id.SourceUser {
		t.Fatalf("add %+v %v", a, err)
	}
	b, err := ls.Add(ctx, op.ID, LinkInput{URL: "https://example.com/b", Title: "یادداشت فارسی"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = ls.Add(ctx, op.ID, LinkInput{URL: "https://example.com/a"})
	uiErr(t, err, KindInvalid)
	_, err = ls.Add(ctx, op.ID, LinkInput{URL: "example.com"})
	uiErr(t, err, KindInvalid)
	if ue := err.(*UIError); ue.Message != "A URL starts with its scheme, such as https://." {
		t.Fatalf("message %q", ue.Message)
	}

	l, err := ls.List(ctx, op.ID)
	if err != nil || len(l) != 2 || l[0].ID != b.Link.ID || l[1].Tags[0] != "btc" || l[0].Tags == nil {
		t.Fatalf("list %+v %v", l, err)
	}
	ed, err := ls.Edit(ctx, op.ID, a.Link.ID, LinkInput{URL: "https://example.com/a2", Title: "A2", Note: "n"})
	if err != nil || ed.Link.URL != "https://example.com/a2" || ed.Link.ID != a.Link.ID {
		t.Fatalf("edit %+v %v", ed, err)
	}
	_, err = ls.Edit(ctx, op.ID, "", LinkInput{URL: "https://x.test"})
	uiErr(t, err, KindInvalid)

	d, err := ls.Delete(ctx, op.ID, a.Link.ID)
	if err != nil || d.Change == "" {
		t.Fatalf("delete %+v %v", d, err)
	}
	_, err = ls.Delete(ctx, op.ID, a.Link.ID)
	uiErr(t, err, KindNotFound)
	if err := ls.UndoDelete(ctx, op.ID, d.Change); err != nil {
		t.Fatal(err)
	}
	uiErr(t, ls.UndoDelete(ctx, op.ID, d.Change), KindBusy)
	if l, _ := ls.List(ctx, op.ID); len(l) != 2 || l[1].Title != "A2" {
		t.Fatalf("after the undo %+v", l)
	}
	// add, add, edit, delete, undo
	if n := e.sink.linkEvents(op.ID); n != 5 {
		t.Fatalf("%d project:links events, want 5", n)
	}

	// A delete whose URL is saved again can't be undone, and says why.
	d, err = ls.Delete(ctx, op.ID, b.Link.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ls.Add(ctx, op.ID, LinkInput{URL: b.Link.URL}); err != nil {
		t.Fatal(err)
	}
	err = ls.UndoDelete(ctx, op.ID, d.Change)
	uiErr(t, err, KindBusy)
	if m := err.(*UIError).Message; m != "A link with this URL was saved since, so the delete can't be undone." {
		t.Fatalf("message %q", m)
	}
}

// A conflict that isn't the URL says the link changed.
func TestUndoDeleteConflictText(t *testing.T) {
	if m := undoDeleteConflict([]store.UndoConflict{{Reason: store.ConflictChanged}}); m != "The link changed after it was deleted, so the delete can't be undone." {
		t.Fatalf("changed: %q", m)
	}
	if m := undoDeleteConflict([]store.UndoConflict{{Reason: store.ConflictChanged}, {Reason: store.ConflictConstraint}}); m != "A link with this URL was saved since, so the delete can't be undone." {
		t.Fatalf("constraint: %q", m)
	}
}
