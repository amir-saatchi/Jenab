package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
)

func saveLink(t *testing.T, p *ProjectDB, s LinkSave) LinkWrite {
	t.Helper()
	w, err := p.SaveLink(context.Background(), s)
	if err != nil {
		t.Fatalf("save %+v: %v", s, err)
	}
	return w
}

func linkURLs(t *testing.T, p *ProjectDB) []string {
	t.Helper()
	ls, err := p.Links(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, l := range ls {
		out = append(out, l.URL)
	}
	return out
}

// Links are listed newest first; a save and a delete are each one change
// with the link before and after it, and the hook hears of each.
func TestLinksSaveListDelete(t *testing.T) {
	ctx := context.Background()
	p, _ := openTestProject(t)
	var heard atomic.Int32
	p.OnLinks(func() { heard.Add(1) })

	a := saveLink(t, p, LinkSave{URL: " https://example.com/a ", Title: "  First   one ", Tags: []string{"news", "News", " btc "}, Source: msg("t1")})
	if a.Op != "save" || a.Change == "" || a.Link.URL != "https://example.com/a" || a.Link.Title != "First one" {
		t.Fatalf("save %+v", a)
	}
	if got := a.Link.Tags; len(got) != 2 || got[0] != "news" || got[1] != "btc" {
		t.Fatalf("tags %q", got)
	}
	b := saveLink(t, p, LinkSave{URL: "https://example.org/b", Title: "قیمت بیت‌کوین", Note: "یادداشت", Source: id.SourceUser})
	if got := linkURLs(t, p); len(got) != 2 || got[0] != b.Link.URL || got[1] != a.Link.URL {
		t.Fatalf("order %q", got)
	}
	if n := one[int](t, p.DB, `SELECT count(*) FROM _jenab_changes WHERE target = ? AND op = 'save'`, LinkTarget(a.Link.ID)); n != 1 {
		t.Fatalf("changes %d", n)
	}

	// The same save again writes nothing.
	if w := saveLink(t, p, LinkSave{ID: a.Link.ID, URL: a.Link.URL, Title: "First one", Tags: []string{"news", "btc"}, Source: id.SourceUser}); w.Op != "" {
		t.Fatalf("a save of the same link wrote %+v", w)
	}

	d, err := p.DeleteLink(ctx, id.SourceUser, a.Link.ID)
	if err != nil || d.Op != "delete" || d.Link.Title != "First one" {
		t.Fatalf("delete %+v %v", d, err)
	}
	if got := linkURLs(t, p); len(got) != 1 {
		t.Fatalf("after the delete %q", got)
	}
	if _, err := p.DeleteLink(ctx, id.SourceUser, a.Link.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a second delete: %v", err)
	}
	if _, err := p.LinkByID(ctx, a.Link.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("read of a deleted link: %v", err)
	}
	if heard.Load() != 3 {
		t.Fatalf("the hook heard %d changes, want 3", heard.Load())
	}
}

// A URL is saved once: an add of a saved URL is refused, save_link's merge
// updates that link, and an edit can't take another link's URL.
func TestLinksURLOnce(t *testing.T) {
	ctx := context.Background()
	p, _ := openTestProject(t)
	a := saveLink(t, p, LinkSave{URL: "https://example.com/a", Title: "A", Source: id.SourceUser})
	b := saveLink(t, p, LinkSave{URL: "https://example.com/b", Title: "B", Source: id.SourceUser})
	if _, err := p.SaveLink(ctx, LinkSave{URL: a.Link.URL, Source: id.SourceUser}); !errors.Is(err, ErrLinkTaken) || !errors.Is(err, ErrInvalid) {
		t.Fatalf("a second add: %v", err)
	}
	m := saveLink(t, p, LinkSave{URL: a.Link.URL, Title: "A again", Merge: true, Source: msg("t1")})
	if m.Link.ID != a.Link.ID || m.Link.Title != "A again" || m.Link.CreatedBy != id.SourceUser || !m.Link.CreatedAt.Equal(a.Link.CreatedAt) {
		t.Fatalf("merge %+v", m.Link)
	}
	if _, err := p.SaveLink(ctx, LinkSave{ID: b.Link.ID, URL: a.Link.URL, Source: id.SourceUser}); !errors.Is(err, ErrLinkTaken) {
		t.Fatalf("an edit to a taken URL: %v", err)
	}
	e := saveLink(t, p, LinkSave{ID: b.Link.ID, URL: "https://example.com/c", Title: "C", Source: id.SourceUser})
	if e.Link.ID != b.Link.ID || e.Link.URL != "https://example.com/c" {
		t.Fatalf("edit %+v", e.Link)
	}
	if _, err := p.SaveLink(ctx, LinkSave{ID: "nope", URL: "https://x.test", Source: id.SourceUser}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an edit of no link: %v", err)
	}
}

func TestCleanLink(t *testing.T) {
	bad := []string{"", "example.com", "/relative", "https://a b", "https://a\tb", "https://a\x00", "\u202ehttps://a.test"}
	for _, u := range bad {
		if _, err := CleanLink(LinkSave{URL: u}); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q: %v", u, err)
		}
	}
	// Any scheme is kept: what opens is the Links tab's choice (SPEC 4.5).
	for _, u := range []string{"https://example.com", "mailto:a@example.com", "javascript:alert(1)", "file:///c:/x"} {
		if _, err := CleanLink(LinkSave{URL: u}); err != nil {
			t.Errorf("%q: %v", u, err)
		}
	}
	if _, err := CleanLink(LinkSave{URL: "https://a.test", Title: string(make([]rune, MaxLinkTitle+1))}); err == nil {
		t.Error("a long title passed")
	}
}

// A user's delete is undone by its change; the agent's save is undone
// with its turn; an edit after the save stands in the way.
func TestLinksUndo(t *testing.T) {
	ctx := context.Background()
	p, _ := openTestProject(t)
	var heard atomic.Int32
	p.OnLinks(func() { heard.Add(1) })
	a := saveLink(t, p, LinkSave{URL: "https://example.com/a", Title: "A", Note: "n", Tags: []string{"x"}, Source: id.SourceUser})
	d, err := p.DeleteLink(ctx, id.SourceUser, a.Link.ID)
	if err != nil {
		t.Fatal(err)
	}
	heard.Store(0)
	r, err := p.UndoChange(ctx, d.Change, UndoOptions{})
	if err != nil || !r.Links {
		t.Fatalf("undo %+v %v", r, err)
	}
	back, err := p.LinkByID(ctx, a.Link.ID)
	if err != nil || !sameLink(&back, &a.Link) {
		t.Fatalf("back %+v %v", back, err)
	}
	if heard.Load() != 1 {
		t.Fatalf("the undo was heard %d times", heard.Load())
	}
	if _, err := p.UndoChange(ctx, d.Change, UndoOptions{}); !errors.Is(err, ErrNothingToUndo) {
		t.Fatalf("a second undo: %v", err)
	}

	// The agent's save goes with its turn.
	saveLink(t, p, LinkSave{URL: "https://example.com/agent", Source: msg("t1")})
	undoOK(t, p, []id.Source{msg("t1")}, UndoOptions{})
	if got := linkURLs(t, p); len(got) != 1 || got[0] != a.Link.URL {
		t.Fatalf("after Undo turn %q", got)
	}
	undoOK(t, p, []id.Source{msg("t1")}, UndoOptions{Redo: true})
	if got := linkURLs(t, p); len(got) != 2 {
		t.Fatalf("after Redo %q", got)
	}

	// The agent's edit, then the user's: the turn's undo is a conflict.
	saveLink(t, p, LinkSave{ID: a.Link.ID, URL: a.Link.URL, Title: "A by the agent", Source: msg("t2")})
	saveLink(t, p, LinkSave{ID: a.Link.ID, URL: a.Link.URL, Title: "A by the user", Source: id.SourceUser})
	r, err = p.Undo(ctx, []id.Source{msg("t2")}, UndoOptions{})
	if !errors.Is(err, ErrUndoConflict) || len(r.Conflicts) == 0 || r.Conflicts[0].Reason != ConflictChanged || r.Conflicts[0].Name != "A by the user" {
		t.Fatalf("undo over a later edit: %+v %v", r, err)
	}

	// A delete whose URL was saved again meanwhile can't come back.
	d, err = p.DeleteLink(ctx, id.SourceUser, a.Link.ID)
	if err != nil {
		t.Fatal(err)
	}
	saveLink(t, p, LinkSave{URL: a.Link.URL, Source: id.SourceUser})
	if r, err := p.UndoChange(ctx, d.Change, UndoOptions{}); !errors.Is(err, ErrUndoConflict) || len(r.Conflicts) != 1 || r.Conflicts[0].Reason != ConflictConstraint {
		t.Fatalf("undo onto a taken URL: %+v %v", r, err)
	}
}

// A project.db from before saved links gets their table, its views kept,
// and the card counts them.
func TestLinkFormatUpdate(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	before := Format{Name: "project", Steps: ProjectFormat.Steps[:len(ProjectFormat.Steps)-1]}
	old, err := Open(ctx, filepath.Join(dir, "project.db"), Options{Format: before, Snapshots: filepath.Join(dir, "snapshots")})
	if err != nil {
		t.Fatal(err)
	}
	op := newProjectDB(old, dir)
	saveView(t, op, msg("t1"), "btc_prices", priceView, 0)
	if err := op.Close(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := OpenProject(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close(ctx)
	if _, err := p.Config(ctx, KindView, "btc_prices"); err != nil {
		t.Fatalf("the view is lost: %v", err)
	}
	saveLink(t, p, LinkSave{URL: "https://example.com", Source: id.SourceUser})
	if got := linkURLs(t, p); len(got) != 1 {
		t.Fatalf("links %q", got)
	}
	if c, err := p.Card(ctx, time.Now()); err != nil || !strings.HasSuffix(c, "Saved links: 1.") {
		t.Fatalf("card %q %v", c, err)
	}
	if err := p.Close(ctx); err != nil {
		t.Fatal(err)
	}
	// The update kept the old file as a backup, and an app from before
	// saved links won't open the new one.
	backups, _ := filepath.Glob(filepath.Join(dir, "snapshots", fmt.Sprintf("project-pre-update-*-v%d.db", len(before.Steps))))
	if len(backups) != 1 {
		t.Fatalf("backups %v", backups)
	}
	b, err := Open(ctx, backups[0], Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if v := one[int](t, b, "PRAGMA user_version"); v != len(before.Steps) {
		t.Errorf("the backup's user_version %d", v)
	}
	b.Close(ctx)
	if _, err := Open(ctx, filepath.Join(dir, "project.db"), Options{Format: before, Snapshots: filepath.Join(dir, "snapshots")}); !errors.Is(err, ErrNewerFormat) {
		t.Fatalf("the older app opened the new file: %v", err)
	}
}

// A URL is saved normalized: the scheme and host in lower case, an http
// address with no path gets "/", so the same address is one link. A URL
// that doesn't parse says so.
func TestNormalLinkURL(t *testing.T) {
	for in, want := range map[string]string{
		"HTTPS://Example.COM":           "https://example.com/",
		"https://example.com/":          "https://example.com/",
		"https://Example.com?q=A":       "https://example.com/?q=A",
		"https://Example.com#Top":       "https://example.com/#Top",
		"https://User@Example.com/Path": "https://User@example.com/Path",
		"http://[::1]:8080":             "http://[::1]:8080/",
		"MAILTO:Someone@Example.com":    "mailto:Someone@Example.com",
		"ftp://Files.Example.com":       "ftp://files.example.com",
		"javascript:alert(1)":           "javascript:alert(1)",
		"https://example.com/A/B?C=D#E": "https://example.com/A/B?C=D#E",
	} {
		got, err := NormalLinkURL(in)
		if err != nil || got != want {
			t.Errorf("NormalLinkURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	_, err := CleanLink(LinkSave{URL: "http://[::1"})
	var le *LinkError
	if !errors.As(err, &le) || le.Msg != "This URL can't be read. Check its host and its characters." {
		t.Errorf("a URL that doesn't parse: %v", err)
	}
	ctx := context.Background()
	p, _ := openTestProject(t)
	saveLink(t, p, LinkSave{URL: "https://a.example", Source: id.SourceUser})
	for _, u := range []string{"https://a.example/", "HTTPS://A.EXAMPLE"} {
		if _, err := p.SaveLink(ctx, LinkSave{URL: u, Source: id.SourceUser}); !errors.Is(err, ErrLinkTaken) {
			t.Errorf("%q saved as a second link: %v", u, err)
		}
	}
}

// Tags have no commas, which separate them in the Links tab; they are
// kept once each as search folds them; direction overrides and isolates
// are refused in a URL and dropped from the title, the note and the tags.
func TestCleanLinkTagsAndDirection(t *testing.T) {
	for _, tag := range []string{"a,b", "کتاب،رمان"} {
		_, err := CleanLink(LinkSave{URL: "https://a.test", Tags: []string{tag}})
		var le *LinkError
		if !errors.As(err, &le) || le.Msg != "A tag has no comma: commas separate tags." {
			t.Errorf("%q: %v", tag, err)
		}
	}
	s, err := CleanLink(LinkSave{URL: "https://a.test", Tags: []string{"کتاب", "كتاب", "Books", "BOOKS", "ي", "ی"}})
	if err != nil || strings.Join(s.Tags, "|") != "کتاب|Books|ي" {
		t.Errorf("tags %q, %v", s.Tags, err)
	}
	for _, u := range []string{"https://evil.test/‮google.com", "https://a.test/⁦x⁩", "https://a.test/‏x"} {
		if _, err := CleanLink(LinkSave{URL: u}); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q passed: %v", u, err)
		}
	}
	s, err = CleanLink(LinkSave{URL: "https://a.test", Title: "‮abc‬ x", Note: "⁧note⁩", Tags: []string{"‭t"}})
	if err != nil || s.Title != "abc x" || s.Note != "note" || s.Tags[0] != "t" {
		t.Errorf("cleaned %+v, %v", s, err)
	}
	// ZWNJ and the marks stay in a title: Persian words need them.
	if s, _ := CleanLink(LinkSave{URL: "https://a.test", Title: "می‌خواهم‏"}); s.Title != "می‌خواهم‏" {
		t.Errorf("title %q", s.Title)
	}
}

// save_link's merge keeps what it leaves empty: the user's title, note and
// tags stay when the agent saves the source again without them.
func TestLinkMergeKeepsFields(t *testing.T) {
	p, _ := openTestProject(t)
	a := saveLink(t, p, LinkSave{URL: "https://a.test/x", Title: "Mine", Note: "my note", Tags: []string{"mine"}, Source: id.SourceUser})
	m := saveLink(t, p, LinkSave{URL: "https://a.test/x", Merge: true, Source: msg("t1")})
	if m.Op != "" || m.Link.Title != "Mine" || m.Link.Note != "my note" || len(m.Link.Tags) != 1 {
		t.Fatalf("an empty merge changed the link: %+v", m)
	}
	m = saveLink(t, p, LinkSave{URL: "https://a.test/x", Note: "the agent's note", Merge: true, Source: msg("t1")})
	if m.Link.ID != a.Link.ID || m.Link.Title != "Mine" || m.Link.Note != "the agent's note" || m.Link.Tags[0] != "mine" {
		t.Fatalf("merge %+v", m.Link)
	}
}

// A redo whose link's URL was saved again meanwhile is a conflict, and
// the conflict names the link.
func TestLinkRedoOntoATakenURL(t *testing.T) {
	ctx := context.Background()
	p, _ := openTestProject(t)
	saveLink(t, p, LinkSave{URL: "https://a.test/src", Title: "Source", Source: msg("t1")})
	undoOK(t, p, []id.Source{msg("t1")}, UndoOptions{})
	saveLink(t, p, LinkSave{URL: "https://a.test/src", Title: "Mine", Source: id.SourceUser})
	r, err := p.Undo(ctx, []id.Source{msg("t1")}, UndoOptions{Redo: true})
	if !errors.Is(err, ErrUndoConflict) || len(r.Conflicts) != 1 || r.Conflicts[0].Reason != ConflictConstraint || r.Conflicts[0].Name != "Source" {
		t.Fatalf("redo onto a taken URL: %+v %v", r, err)
	}
	if got := linkURLs(t, p); len(got) != 1 {
		t.Fatalf("links %q", got)
	}
}
