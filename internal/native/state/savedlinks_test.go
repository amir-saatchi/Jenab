package state

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
)

// fakeLinks is the Links tab's service in memory.
type fakeLinks struct {
	mu      sync.Mutex
	links   []app.Link
	deleted map[id.Change]app.Link
	reads   int
	n       int
	listErr error         // List fails with it
	gate    chan struct{} // Add waits for it when set
}

func (f *fakeLinks) List(context.Context, id.Project) ([]app.Link, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	if f.listErr != nil {
		return nil, f.listErr
	}
	return slices.Clone(f.links), nil
}

func (f *fakeLinks) Add(_ context.Context, _ id.Project, in app.LinkInput) (app.LinkSaved, error) {
	if f.gate != nil {
		<-f.gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if in.URL == "" {
		return app.LinkSaved{}, &app.UIError{Kind: app.KindInvalid, Message: "A link needs its URL."}
	}
	f.n++
	l := app.Link{ID: string(rune('a' + f.n)), URL: in.URL, Title: in.Title, Note: in.Note, Tags: in.Tags, CreatedBy: id.SourceUser}
	f.links = append([]app.Link{l}, f.links...)
	return app.LinkSaved{Link: l, Change: "add"}, nil
}

func (f *fakeLinks) Edit(_ context.Context, _ id.Project, lid string, in app.LinkInput) (app.LinkSaved, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := slices.IndexFunc(f.links, func(l app.Link) bool { return l.ID == lid })
	if i < 0 {
		return app.LinkSaved{}, &app.UIError{Kind: app.KindNotFound, Message: "This link no longer exists. It was deleted."}
	}
	f.links[i].URL, f.links[i].Title, f.links[i].Note, f.links[i].Tags = in.URL, in.Title, in.Note, in.Tags
	return app.LinkSaved{Link: f.links[i], Change: "edit"}, nil
}

func (f *fakeLinks) Delete(_ context.Context, _ id.Project, lid string) (app.Deleted, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := slices.IndexFunc(f.links, func(l app.Link) bool { return l.ID == lid })
	if i < 0 {
		return app.Deleted{}, &app.UIError{Kind: app.KindNotFound, Message: "This link no longer exists. It was deleted."}
	}
	f.deleted[id.Change("ch-"+lid)] = f.links[i]
	f.links = slices.Delete(f.links, i, i+1)
	return app.Deleted{Change: id.Change("ch-" + lid)}, nil
}

func (f *fakeLinks) UndoDelete(_ context.Context, _ id.Project, ch id.Change) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	l, ok := f.deleted[ch]
	if !ok {
		return &app.UIError{Kind: app.KindBusy, Message: "The delete is undone already, or past the undo window."}
	}
	delete(f.deleted, ch)
	f.links = append(f.links, l)
	return nil
}

func (f *fakeLinks) count() (links, reads int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.links), f.reads
}

func TestLinkHost(t *testing.T) {
	for in, want := range map[string]string{
		"https://www.Example.com/a?b": "example.com",
		"http://news.example.ir:8080": "news.example.ir",
		"mailto:someone@example.com":  "someone@example.com",
		"javascript:alert(1)":         "javascript:",
		"ftp://files.example.com/x":   "ftp:",
		"file:///C:/x":                "file:",
		"not a url":                   "not a url",
	} {
		if got := LinkHost(in); got != want {
			t.Errorf("LinkHost(%q) = %q, want %q", in, got, want)
		}
	}
	if got := LinkTitle(app.Link{URL: "https://example.com"}); got != "example.com" {
		t.Errorf("an untitled link's title %q", got)
	}
}

func TestSplitTags(t *testing.T) {
	if got := SplitTags(" btc, خبر،  bitcoin ,,"); !slices.Equal(got, []string{"btc", "خبر", "bitcoin"}) {
		t.Errorf("SplitTags = %q", got)
	}
	if got := SplitTags("  "); got == nil || len(got) != 0 {
		t.Errorf("no tags = %#v", got)
	}
	if got := JoinTags([]string{"a", "b"}); got != "a, b" {
		t.Errorf("JoinTags = %q", got)
	}
}

// Search finds a link by its title, its host or a tag, in any case, and
// Persian with Arabic letters or without its half-spaces; not by its note.
func TestFilterLinks(t *testing.T) {
	links := []app.Link{
		{ID: "1", URL: "https://www.example.com/a", Title: "Bitcoin News", Note: "prices", Tags: []string{"btc"}},
		{ID: "2", URL: "https://news.example.ir/x", Title: "می‌خواهم یک کتاب", Tags: []string{"خبر"}},
		{ID: "3", URL: "mailto:someone@example.org", Tags: []string{}},
		{ID: "4", URL: "https://a.example/r", Title: "گزارش ۱۴۰۳ بـازارِ کالا", Tags: []string{"ﻛﺘﺎﺏ"}},
	}
	ids := func(q string) []string {
		var out []string
		for _, l := range FilterLinks(links, q) {
			out = append(out, l.ID)
		}
		return out
	}
	for q, want := range map[string][]string{
		"":              {"1", "2", "3", "4"},
		"  ":            {"1", "2", "3", "4"},
		"bitcoin":       {"1"},
		"NEWS":          {"1", "2"},
		"example.com":   {"1"},
		"www":           nil,
		"BTC":           {"1"},
		"خبر":           {"2"},
		"میخواهم":       {"2"},
		"يک":            {"2"},
		"someone@":      {"3"},
		"prices":        nil,
		"nothing-there": nil,
		// A space finds a ZWNJ; the digits, harakat, tatweel and
		// presentation forms fold as the chat search folds them.
		"می خواهم":     {"2"},
		"می  خواهم یک": {"2"},
		"1403":         {"4"},
		"۱۴۰۳":         {"4"},
		"١٤٠٣":         {"4"},
		"بازار کالا":   {"4"},
		"بازارِ":       {"4"},
		"کتاب":         {"2", "4"},
		"‏خبر":         {"2"},
	} {
		if got := ids(q); !slices.Equal(got, want) {
			t.Errorf("FilterLinks(%q) = %v, want %v", q, got, want)
		}
	}
}

func linksStores(t *testing.T, f *fakeLinks) (*Stores, *loop) {
	l := newLoop()
	s := NewStores(API{Links: f}, l.post)
	t.Cleanup(s.Close)
	return s, l
}

// The store reads a project's list once, and again after project:links;
// the dialog adds and edits, keeping a refusal; a delete toasts with Undo,
// which puts the link back.
func TestLinksStore(t *testing.T) {
	f := &fakeLinks{deleted: map[id.Change]app.Link{}, links: []app.Link{{ID: "x", URL: "https://example.com", Title: "Example", Tags: []string{}}}}
	s, l := linksStores(t, f)
	s.Links.List(projP)
	until(t, l, func() bool { return s.Links.List(projP).Loaded })
	v := s.Links.List(projP)
	if len(v.Links) != 1 || v.Err != "" {
		t.Fatalf("list %+v", v)
	}
	if s.Links.List(projP) != v {
		t.Fatal("an unchanged list was read again")
	}
	f.mu.Lock()
	f.links = append(f.links, app.Link{ID: "y", URL: "https://two.example", Tags: []string{}})
	f.mu.Unlock()
	s.Apply(app.EventLinks, project.LinksChanged{Project: projP})
	nv := s.Links.List(projP)
	if len(nv.Links) != 1 {
		t.Fatal("the list emptied while it was read again")
	}
	until(t, l, func() bool { return len(s.Links.List(projP).Links) == 2 })

	// A refusal stays in the dialog; a save closes it.
	s.Links.StartAdd(projP)
	s.Links.Save()
	until(t, l, func() bool { return !s.Links.Editor.Busy })
	if s.Links.Editor.Problem != "A link needs its URL." {
		t.Fatalf("problem %q", s.Links.Editor.Problem)
	}
	s.Links.Editor.URL, s.Links.Editor.Title, s.Links.Editor.Tags = " https://new.example ", "تازه", "a، b"
	s.Links.Save()
	until(t, l, func() bool { return s.Links.Editor == nil })
	until(t, l, func() bool { return len(s.Links.List(projP).Links) == 3 })
	got := s.Links.List(projP).Links[0]
	if got.URL != "https://new.example" || got.Title != "تازه" || !slices.Equal(got.Tags, []string{"a", "b"}) {
		t.Fatalf("added %+v", got)
	}
	s.Links.StartEdit(projP, got)
	if e := s.Links.Editor; e.ID != got.ID || e.Tags != "a, b" || e.URL != got.URL {
		t.Fatalf("editor %+v", e)
	}
	s.Links.Editor.Title = "New title"
	s.Links.Save()
	until(t, l, func() bool { return s.Links.Editor == nil && s.Links.List(projP).Links[0].Title == "New title" })

	// Delete, then Undo.
	s.DeleteLink(projP, s.Links.List(projP).Links[0])
	until(t, l, func() bool { return len(s.Toasts) > 0 })
	toasts := s.TakeToasts()
	if len(toasts) != 1 || toasts[0].Text != "Deleted New title" || toasts[0].Action != "Undo" {
		t.Fatalf("toasts %+v", toasts)
	}
	until(t, l, func() bool { return len(s.Links.List(projP).Links) == 2 })
	toasts[0].OnAction()
	until(t, l, func() bool { return len(s.Toasts) > 0 })
	if toasts = s.TakeToasts(); toasts[0].Text != "New title is back" {
		t.Fatalf("toasts %+v", toasts)
	}
	until(t, l, func() bool { return len(s.Links.List(projP).Links) == 3 })
	// A second Undo says it can't.
	toasts = nil
	s.undoDeleteLink(projP, "New title", "ch-b")
	until(t, l, func() bool { return len(s.Toasts) > 0 })
	if toasts = s.TakeToasts(); !toasts[0].Error {
		t.Fatalf("a second undo %+v", toasts)
	}
	if n, _ := f.count(); n != 3 {
		t.Fatalf("%d links", n)
	}
}

// Without the service the tab says so and saves nothing.
func TestLinksWithoutService(t *testing.T) {
	l := newLoop()
	s := NewStores(API{}, l.post)
	t.Cleanup(s.Close)
	if v := s.Links.List(projP); v.Err == "" || !v.Loaded {
		t.Fatalf("list %+v", v)
	}
	s.Links.StartAdd(projP)
	s.Links.Save()
	if s.Links.Editor.Problem == "" || s.Links.Editor.Busy {
		t.Fatalf("editor %+v", s.Links.Editor)
	}
	s.DeleteLink(projP, app.Link{ID: "x"})
	s.Links.CloseEditor()
	if s.Links.Editor != nil || len(s.Toasts) != 0 {
		t.Fatalf("editor %+v, toasts %+v", s.Links.Editor, s.Toasts)
	}
}

// The dialog belongs to its project's tab: Detach closes it even while a
// save runs, whose refusal then shows as a toast. No new edit starts
// while a save runs; once the dialog is detached, one does.
func TestLinksEditorDetach(t *testing.T) {
	f := &fakeLinks{deleted: map[id.Change]app.Link{}, gate: make(chan struct{})}
	s, l := linksStores(t, f)
	until(t, l, func() bool { return s.Links.List(projP).Loaded })
	if !s.Links.StartAdd(projP) {
		t.Fatal("StartAdd refused")
	}
	e := s.Links.Editor
	s.Links.Save() // no URL: refused, once the gate opens
	if !e.Busy {
		t.Fatal("the save isn't running")
	}
	if s.Links.StartAdd(projQ) || s.Links.StartEdit(projP, app.Link{ID: "x"}) || s.Links.Editor != e {
		t.Fatal("a new edit replaced a running save")
	}
	s.Links.CloseEditor()
	if s.Links.Editor != e {
		t.Fatal("the dialog closed while its save runs")
	}
	s.Links.Detach()
	if s.Links.Editor != nil || !s.Links.StartAdd(projQ) {
		t.Fatal("the dialog wasn't detached")
	}
	q := s.Links.Editor
	close(f.gate)
	until(t, l, func() bool { return len(s.Toasts) > 0 })
	if ts := s.TakeToasts(); len(ts) != 1 || !ts[0].Error || ts[0].Text != "A link needs its URL." {
		t.Fatalf("toasts %+v", ts)
	}
	if s.Links.Editor != q || q.Problem != "" {
		t.Fatalf("the detached save reached the new dialog: %+v", s.Links.Editor)
	}

	// A detached save that succeeds reads its project's list again.
	s.Links.Detach()
	s.Links.StartAdd(projP)
	s.Links.Editor.URL = "https://a.example"
	s.Links.Save()
	s.Links.Detach()
	until(t, l, func() bool { return len(s.Links.List(projP).Links) == 1 })
	if len(s.Toasts) != 0 {
		t.Fatalf("toasts %+v", s.Toasts)
	}
}

// After an error the list reads again on Changed, the Links tab's Try
// again, showing its spinner meanwhile.
func TestLinksRetry(t *testing.T) {
	f := &fakeLinks{deleted: map[id.Change]app.Link{}, listErr: errors.New("disk on fire"),
		links: []app.Link{{ID: "x", URL: "https://example.com", Tags: []string{}}}}
	s, l := linksStores(t, f)
	until(t, l, func() bool { return s.Links.List(projP).Loaded })
	if v := s.Links.List(projP); v.Err == "" || len(v.Links) != 0 {
		t.Fatalf("list %+v", v)
	}
	f.mu.Lock()
	f.listErr = nil
	f.mu.Unlock()
	s.Links.Changed(projP)
	if v := s.Links.List(projP); v.Loaded || v.Err != "" {
		t.Fatalf("the read again %+v", v)
	}
	until(t, l, func() bool { v := s.Links.List(projP); return v.Loaded && len(v.Links) == 1 && v.Err == "" })
}

// Keep drops other projects' lists, searches and counts.
func TestLinksKeep(t *testing.T) {
	f := &fakeLinks{deleted: map[id.Change]app.Link{}}
	s, l := linksStores(t, f)
	for _, p := range []id.Project{projP, projQ} {
		until(t, l, func() bool { return s.Links.List(p).Loaded })
		s.Links.Query[p] = "x"
		s.Links.Changed(p)
	}
	s.Links.Keep(projP)
	if len(s.Links.Lists) != 1 || s.Links.Lists[projP] == nil || len(s.Links.Query) != 1 || s.Links.Query[projP] != "x" ||
		len(s.Links.revs) != 1 || s.Links.revs[projP] == 0 {
		t.Fatalf("kept %v %v %v", s.Links.Lists, s.Links.Query, s.Links.revs)
	}
}
