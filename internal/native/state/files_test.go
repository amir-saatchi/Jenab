package state

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/id"
)

// frontend/src/shell/files.test.ts, ported.

const (
	projP id.Project = "01JABCDEFGHJKMNPQRSTVWXYZ0"
	projQ id.Project = "01JZZZZZZZZZZZZZZZZZZZZZZZ"
)

func TestFolderNavigation(t *testing.T) {
	same(t, Crumbs(""), []Crumb{{"Files", ""}})
	same(t, Crumbs("docs/2026/"), []Crumb{{"Files", ""}, {"docs", "docs/"}, {"2026", "docs/2026/"}})
	same(t, Crumbs("cache/pages/"), []Crumb{{"Files", ""}, {"App cache", "cache/"}, {"pages", "cache/pages/"}})
	same(t, ParentOf("a.txt"), "")
	same(t, ParentOf("docs/2026/a.txt"), "docs/2026/")
	same(t, ParentOf("docs/2026/"), "docs/")
	same(t, NameOf("docs/2026/a.txt"), "a.txt")
	same(t, NameOf("docs/2026/"), "2026")
}

// fakeList has pages of two entries, like the Go side's pages of 100.
func fakeList(all map[string][]string) (ListFunc, *[]string) {
	var calls []string
	list := func(_ context.Context, _ id.Project, prefix, after string) (app.ObjectPage, error) {
		calls = append(calls, prefix+"|"+after)
		names := slices.Sorted(slices.Values(all[prefix]))
		from := 0
		if after != "" {
			from = slices.Index(names, after) + 1
		}
		end := min(from+2, len(names))
		pg := app.ObjectPage{Folders: []string{}}
		for _, n := range names[from:end] {
			if strings.HasSuffix(n, "/") {
				pg.Folders = append(pg.Folders, n)
			} else {
				pg.Objects = append(pg.Objects, app.Object{Key: n, MIME: "text/plain", Size: 1})
			}
		}
		if end < len(names) {
			pg.Next = names[end-1]
		}
		return pg, nil
	}
	return list, &calls
}

func keys(os []app.Object) []string {
	var out []string
	for _, o := range os {
		out = append(out, o.Key)
	}
	return out
}

func TestALongFolderLoadsPageByPage(t *testing.T) {
	ctx := context.Background()
	list, calls := fakeList(map[string][]string{"docs/": {"docs/old/", "docs/a.txt", "docs/b.txt", "docs/c.txt"}})
	f, err := ListFolder(ctx, list, projP, "docs/", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	same(t, keys(f.Objects), []string{"docs/a.txt", "docs/b.txt"})
	same(t, f.Next, "docs/b.txt")
	f, _ = ListFolder(ctx, list, projP, "docs/", &f, f.Next)
	same(t, keys(f.Objects), []string{"docs/a.txt", "docs/b.txt", "docs/c.txt"})
	same(t, f.Folders, []string{"docs/old/"})
	same(t, f.Next, "")
	same(t, *calls, []string{"docs/|", "docs/|docs/b.txt"})
}

func TestCacheIsHiddenFromTheTopFolderOnly(t *testing.T) {
	same(t, ShownFolders("", []string{"a/", Cache, "z/"}), []string{"a/", "z/"})
	same(t, ShownFolders("x/", []string{"x/cache/"}), []string{"x/cache/"})
	same(t, ShownFolders(Cache, []string{"cache/pages/", "cache/tool/"}), []string{"cache/pages/", "cache/tool/"})
	list, _ := fakeList(map[string][]string{"": {"a/", Cache}, Cache: {"cache/pages/"}})
	top, _ := ListFolder(context.Background(), list, projP, "", nil, "")
	same(t, top.Folders, []string{"a/"})
	cache, _ := ListFolder(context.Background(), list, projP, Cache, nil, "")
	same(t, cache.Folders, []string{"cache/pages/"})
}

func TestLinks(t *testing.T) {
	p, key, ok := ParseLink("jenab://" + string(projP) + "/docs/a.txt")
	if !ok || p != projP || key != "docs/a.txt" {
		t.Fatalf("ParseLink = %q %q %v", p, key, ok)
	}
	same(t, LinkOf(projP, "docs/a.txt"), "jenab://"+string(projP)+"/docs/a.txt")
	for _, bad := range []string{"jenab://" + string(projP) + "/", "jenab://" + string(projP) + "/../a", "jenab://" + string(projP) + "/a//b",
		"jenab://nope/a.txt", "https://x/a", "jenab://" + string(projP) + "/a b"} {
		if _, _, ok := ParseLink(bad); ok {
			t.Errorf("%q parsed", bad)
		}
	}
	link := "jenab://" + string(projP) + "/docs/report.pdf"
	same(t, SplitLinks("See "+link+". Done"), []Piece{{Text: "See "}, {Text: link, Link: true}, {Text: ". Done"}})
	same(t, SplitLinks("فایل "+link), []Piece{{Text: "فایل "}, {Text: link, Link: true}})
	same(t, SplitLinks("no links"), []Piece{{Text: "no links"}})
}

func TestKindsSizesAndWho(t *testing.T) {
	for mime, want := range map[string]FileKind{
		"text/markdown; charset=utf-8": KindMarkdown, "text/html; charset=utf-8": KindText, "application/json": KindText,
		"image/svg+xml": KindImage, "image/tiff": KindOther, "application/pdf": KindOther, "audio/mpeg": KindAudio,
		"application/octet-stream": KindOther, "image/png": KindImage,
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": KindOther,
	} {
		if got := KindOf(mime); got != want {
			t.Errorf("KindOf(%q) = %q, want %q", mime, got, want)
		}
	}
	same(t, FormatSize(900), "900 B")
	same(t, FormatSize(1536), "1.5 KB")
	same(t, FormatSize(5<<20), "5.0 MB")
	same(t, FormatSize(12<<20), "12 MB")
	same(t, WhoText("user"), "you")
	same(t, WhoText("message:01J"), "the agent")
	same(t, WhoText("run:01J"), "a pipeline")
	same(t, TypeName("text/plain; charset=utf-8"), "text/plain")
	same(t, TypeName(""), "unknown")
}

// fakeBucket is the files of one project, for the stores' tests.
type fakeBucket struct {
	files   map[string]string // key → text
	heads   []string
	deleted map[id.Change]string
}

func (b *fakeBucket) List(ctx context.Context, p id.Project, prefix, after string) (app.ObjectPage, error) {
	pg := app.ObjectPage{Folders: []string{}}
	for k := range b.files {
		if strings.HasPrefix(k, prefix) && !strings.Contains(k[len(prefix):], "/") {
			pg.Objects = append(pg.Objects, app.Object{Key: k, MIME: "text/plain"})
		}
	}
	return pg, nil
}

func (b *fakeBucket) Head(_ context.Context, p id.Project, key string) (app.Object, error) {
	b.heads = append(b.heads, string(p)+"/"+key)
	text, ok := b.files[key]
	if !ok {
		return app.Object{}, &app.UIError{Kind: app.KindNotFound, Message: "It no longer exists."}
	}
	return app.Object{Key: key, MIME: "text/markdown", Size: int64(len(text)), Version: 1}, nil
}

func (b *fakeBucket) Versions(context.Context, id.Project, string) ([]app.ObjectVersion, error) {
	return []app.ObjectVersion{{Version: 1}}, nil
}

func (b *fakeBucket) Read(_ context.Context, _ id.Project, key string, max int64) (app.ObjectBytes, error) {
	text := b.files[key]
	return app.ObjectBytes{Data: []byte(text[:min(int64(len(text)), max)]), Cut: int64(len(text)) > max}, nil
}

func (b *fakeBucket) Delete(_ context.Context, _ id.Project, key string) (app.Deleted, error) {
	delete(b.files, key)
	b.deleted["ch1"] = key
	return app.Deleted{Change: "ch1"}, nil
}

func (b *fakeBucket) UndoDelete(_ context.Context, _ id.Project, ch id.Change) error {
	b.files[b.deleted[ch]] = "back"
	return nil
}

func (b *fakeBucket) OpenWithSystem(context.Context, id.Project, string) error { return nil }

func filesStores(t *testing.T, b *fakeBucket) (*Stores, *loop) {
	l := newLoop()
	s := NewStores(API{Bucket: b}, l.post)
	t.Cleanup(s.Close)
	s.Nav.Go(ChatPlace(projP, "c1"))
	return s, l
}

func TestOpeningALinkOpensTheFileNextToTheChatAndShowsItsFolder(t *testing.T) {
	b := &fakeBucket{files: map[string]string{"docs/2026/a.txt": "one"}}
	s, l := filesStores(t, b)
	done := false
	s.OpenLink("jenab://"+string(projP)+"/docs/2026/a.txt", func() { done = true })
	until(t, l, func() bool { return done })
	same(t, b.heads, []string{string(projP) + "/docs/2026/a.txt"})
	same(t, s.Shell.Open[ChatRef{projP, "c1"}], Docked{Kind: "file", Key: "docs/2026/a.txt"})
	same(t, s.Shell.Folder[projP], "docs/2026/")
	same(t, s.Shell.RightTab, TabFiles)
}

func TestALinkToAnotherProjectOrAMissingFileOpensNothing(t *testing.T) {
	b := &fakeBucket{files: map[string]string{}}
	s, l := filesStores(t, b)
	done := 0
	s.OpenLink("jenab://"+string(projQ)+"/a.txt", func() { done++ })
	s.OpenLink("jenab://"+string(projP)+"/gone.txt", func() { done++ })
	s.OpenLink("jenab://bad", func() { done++ })
	until(t, l, func() bool { return done == 3 })
	same(t, b.heads, []string{string(projP) + "/gone.txt"})
	if _, ok := s.Shell.Open[ChatRef{projP, "c1"}]; ok {
		t.Fatal("something was docked")
	}
	same(t, s.Shell.RightTab, TabPages)
	toasts := s.TakeToasts()
	if len(toasts) != 2 || toasts[0].Text != "This link is to a file in another project." || toasts[1].Text != "gone.txt no longer exists. It was deleted or moved." {
		t.Fatalf("toasts = %+v", toasts)
	}
}

func TestTheDockOpenCloseAndADeletedFileCloses(t *testing.T) {
	b := &fakeBucket{files: map[string]string{"a.txt": "one"}, deleted: map[id.Change]string{}}
	s, l := filesStores(t, b)
	sh := s.Shell
	s.OpenFile(projP, "c1", "a.txt")
	s.OpenFile(projP, "c2", "a.txt")
	s.OpenFile(projP, "c3", "b.txt")
	rev := sh.Rev[projP]
	s.DeleteFile(projP, "a.txt")
	until(t, l, func() bool { return sh.Rev[projP] > rev })
	_, c1 := sh.Open[ChatRef{projP, "c1"}]
	_, c2 := sh.Open[ChatRef{projP, "c2"}]
	if c1 || c2 {
		t.Fatal("a deleted file stays docked")
	}
	same(t, sh.Open[ChatRef{projP, "c3"}], Docked{Kind: "file", Key: "b.txt"})
	toasts := s.TakeToasts()
	if len(toasts) != 1 || toasts[0].Text != "Deleted a.txt" || toasts[0].Action != "Undo" {
		t.Fatalf("toasts = %+v", toasts)
	}
	toasts[0].OnAction()
	until(t, l, func() bool { return b.files["a.txt"] == "back" && sh.Rev[projP] > rev+1 })

	sh.Swap(projP)
	same(t, sh.Side[projP], DockLeft)
	sh.ChatHidden[ChatRef{projP, "c3"}] = true
	sh.Close(ChatRef{projP, "c3"})
	if _, ok := sh.Open[ChatRef{projP, "c3"}]; ok || sh.ChatHidden[ChatRef{projP, "c3"}] {
		t.Fatal("closing left the dock open or the chat hidden")
	}
	s.OpenFile(projP, "c3", "b.txt")
	sh.ListShown = true
	s.OpenFile(projP, "c3", "c.txt")
	same(t, sh.ListShown, true)
	s.OpenFile(projP, "c1", "c.txt")
	same(t, sh.ListShown, false)
}

// While something is docked the chat list hides until the user shows it;
// otherwise hiding it is the user's own choice (useListHidden).
func TestTheChatListHidesWhileSomethingIsDocked(t *testing.T) {
	s, _ := filesStores(t, &fakeBucket{files: map[string]string{}})
	same(t, s.ListHidden(), false)
	s.SetListHidden(true)
	same(t, s.Nav.SidebarHidden, true)
	s.SetListHidden(false)
	s.OpenFile(projP, "c1", "a.txt")
	same(t, s.ListHidden(), true)
	s.SetListHidden(false)
	same(t, s.ListHidden(), false)
	same(t, s.Nav.SidebarHidden, false)
	s.Shell.Close(ChatRef{projP, "c1"})
	same(t, s.ListHidden(), false)
}

func TestTheFileViewReadsTheHeadVersionsAndText(t *testing.T) {
	big := strings.Repeat("x", TextLimit+10)
	b := &fakeBucket{files: map[string]string{"a.md": "# سلام", "big.txt": big}}
	s, l := filesStores(t, b)
	v := s.Files.View(projP, "a.md", 0)
	until(t, l, func() bool { return s.Files.View(projP, "a.md", 0).Loaded })
	v = s.Files.View(projP, "a.md", 0)
	if v.Object == nil || string(v.Data) != "# سلام" || v.Cut || len(v.Versions) != 1 {
		t.Fatalf("view = %+v", v)
	}
	until(t, l, func() bool { return s.Files.View(projP, "big.txt", 0).Loaded })
	v = s.Files.View(projP, "big.txt", 0)
	if len(v.Data) != TextLimit || !v.Cut {
		t.Fatalf("a big file: %d bytes, cut %v", len(v.Data), v.Cut)
	}
	until(t, l, func() bool { return s.Files.View(projP, "gone.md", 0).Err != "" })
	same(t, s.Files.View(projP, "gone.md", 0).NotFound, true)
	// A change from the app reads it again.
	b.files["a.md"] = "two"
	until(t, l, func() bool { return string(s.Files.View(projP, "a.md", 1).Data) == "two" })
}
