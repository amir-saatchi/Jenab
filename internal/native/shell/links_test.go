package shell_test

import (
	"slices"
	"testing"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state"
	"github.com/amir-saatchi/jenab/internal/project"
)

// The Links tab (P3-11): a project's saved links, the newest first, with
// a search, and the user's open, copy, add, edit and delete with Undo.

const (
	btcTitle  = "Bitcoin news"
	btcURL    = "https://www.example.com/btc"
	faTitle   = "می‌خواهم یک کتاب بخرم" // a right-to-left title, with a half-space
	faNote    = "یادداشتی درباره‌ی بازار"
	faURL     = "https://news.example.ir/x"
	unsafeURL = "javascript:alert(1)"
)

var agent = id.SourceOf("message", "m1")

// startLinks is the Files fixture with three saved links, the oldest an
// unsafe one the user saved, and the Links tab shown.
func startLinks(t *testing.T, dark bool) *fixture {
	t.Helper()
	x := startFiles(t, dark)
	x.f.AddLink(p, id.SourceUser, unsafeURL, "", "")
	x.f.AddLink(p, agent, faURL, faTitle, faNote, "خبر")
	x.f.AddLink(p, agent, btcURL, btcTitle, "Daily prices", "btc", "crypto")
	x.click(t, "Links")
	if !x.settle(func() bool { return len(x.st.Links.List(p).Links) == 3 }) {
		t.Fatalf("the links did not load: %q", x.tt.Texts())
	}
	x.settle(nil)
	return x
}

func bothLinks(t *testing.T, test func(t *testing.T, x *fixture)) {
	for _, dark := range []bool{false, true} {
		name := "light"
		if dark {
			name = "dark"
		}
		t.Run(name, func(t *testing.T) { test(t, startLinks(t, dark)) })
	}
}

// at is where s first shows among the window's texts, or -1.
func at(x *fixture, s string) int { return slices.Index(x.tt.Texts(), s) }

func (x *fixture) search(t *testing.T, q string) {
	t.Helper()
	x.st.Links.Query[p] = ""
	x.settle(nil)
	x.click(t, "Search links")
	x.tt.Type(q)
	x.settle(nil)
}

func TestTheLinksTabListsTheSavedLinks(t *testing.T) {
	bothLinks(t, func(t *testing.T, x *fixture) {
		shows(t, x.tt, btcTitle, "example.com", "Daily prices", "btc", "crypto", faTitle, "news.example.ir", faNote, "خبر",
			"javascript:", "Doesn't open")
		if !x.tt.HasText("Saved by the agent · ") || !x.tt.HasText("Saved by you · ") {
			t.Errorf("who saved them is missing: %q", x.tt.Texts())
		}
		if !kit.HasRole(x.tt, "tree", "Links") {
			t.Error("the links are not a tree")
		}
		for _, name := range []string{btcTitle, faTitle, "javascript:"} {
			if !kit.HasRole(x.tt, "treeitem", name) {
				t.Errorf("no row named %q", name)
			}
		}
		// The newest first.
		if a, b, c := at(x, btcTitle), at(x, faTitle), at(x, "javascript:"); a < 0 || !(a < b && b < c) {
			t.Errorf("the order is %d, %d, %d", a, b, c)
		}
		// The Persian title starts on the right of its row.
		r, _ := x.tt.Find(faTitle)
		h, ok := x.tt.Find("⁨news.example.ir⁩") // drawn isolated
		if !ok {
			t.Fatal("no host line")
		}
		img := x.tt.Image()
		bg := img.RGBAAt(int(r.X+r.W-2), int(r.Y+r.H-2))
		left, right := 0, 0
		for y := int(r.Y) + 2; y < int(h.Y); y++ {
			for xx := int(r.X); xx < int(r.X+r.W); xx++ {
				if img.RGBAAt(xx, y) == bg {
					continue
				}
				if xx < int(r.X+r.W/3) {
					left++
				} else if xx > int(r.X+2*r.W/3) {
					right++
				}
			}
		}
		if right == 0 || left > right/4 {
			t.Errorf("the Persian title's ink: %d on the left, %d on the right", left, right)
		}
		shot(t, x, "links-tab")
	})
}

func TestTheLinksSearch(t *testing.T) {
	bothLinks(t, func(t *testing.T, x *fixture) {
		// By a tag, by the host, by a Persian title typed with an Arabic
		// yeh and without its half-space; not by the note.
		for q, want := range map[string]string{"CRYPTO": btcTitle, "news.example": faTitle, "ميخواهم": faTitle, "javascript": "javascript:"} {
			x.search(t, q)
			for _, title := range []string{btcTitle, faTitle, "javascript:"} {
				if got := x.tt.HasText(title) && at(x, title) >= 0; got != (title == want) {
					t.Errorf("search %q: %q shown %v", q, title, got)
				}
			}
		}
		x.search(t, "Daily prices")
		shows(t, x.tt, "No links match")
		shot(t, x, "links-no-match")
	})
}

func TestALinkOpensOnlyWhenSafe(t *testing.T) {
	bothLinks(t, func(t *testing.T, x *fixture) {
		x.click(t, btcTitle)
		if got := x.tt.OpenedURLs(); !slices.Equal(got, []string{btcURL}) {
			t.Fatalf("opened %q", got)
		}
		x.click(t, "javascript:")
		if got := x.tt.OpenedURLs(); len(got) != 1 {
			t.Fatalf("the unsafe link opened: %q", got)
		}
		shows(t, x.tt, "This link doesn't open from the app.")
		// Its menu has no Open; the safe one's does.
		if err := x.tt.RightClick("javascript:"); err != nil {
			t.Fatal(err)
		}
		x.settle(nil)
		if slices.Contains(x.tt.Menu(), "Open") || !slices.Contains(x.tt.Menu(), "Copy link") {
			t.Errorf("the unsafe link's menu %q", x.tt.Menu())
		}
		x.tt.Key(0, kit.KeyEscape)
		x.settle(nil)
		if err := x.tt.RightClick(faTitle); err != nil {
			t.Fatal(err)
		}
		x.settle(nil)
		if err := x.tt.ChooseMenuItem("Open"); err != nil {
			t.Fatalf("%v; menu %q", err, x.tt.Menu())
		}
		x.settle(nil)
		if got := x.tt.OpenedURLs(); !slices.Equal(got, []string{btcURL, faURL}) {
			t.Fatalf("opened %q", got)
		}
	})
}

func TestALinkIsCopied(t *testing.T) {
	bothLinks(t, func(t *testing.T, x *fixture) {
		if err := x.tt.RightClick(faTitle); err != nil {
			t.Fatal(err)
		}
		x.settle(nil)
		if err := x.tt.ChooseMenuItem("Copy link"); err != nil {
			t.Fatalf("%v; menu %q", err, x.tt.Menu())
		}
		x.settle(nil)
		if got := x.tt.Clipboard(); got != faURL {
			t.Errorf("copied %q", got)
		}
	})
}

func TestALinkIsAddedAndEdited(t *testing.T) {
	bothLinks(t, func(t *testing.T, x *fixture) {
		x.click(t, "Add link")
		if !kit.HasRole(x.tt, "dialog", "Add link") {
			t.Fatalf("no Add link dialog: %q", x.tt.Texts())
		}
		// A URL without its scheme is refused, and the dialog stays.
		x.tt.Type("example.org")
		x.click(t, "Save")
		x.settle(func() bool { e := x.st.Links.Editor; return e != nil && !e.Busy })
		x.settle(nil)
		shows(t, x.tt, "A URL starts with its scheme, such as https://.")
		x.st.Links.Editor.URL = ""
		x.settle(nil)
		x.click(t, "URL")
		x.tt.Type("https://example.org/fa")
		x.click(t, "Title")
		x.tt.Type("کتاب‌های تازه")
		x.click(t, "Tags")
		x.tt.Type("کتاب، books")
		shot(t, x, "links-add")
		x.click(t, "Save")
		if !x.settle(func() bool { return x.st.Links.Editor == nil && len(x.st.Links.List(p).Links) == 4 }) {
			t.Fatalf("the link was not added: %q", x.tt.Texts())
		}
		x.settle(nil)
		got := x.st.Links.List(p).Links[0]
		if got.URL != "https://example.org/fa" || got.Title != "کتاب‌های تازه" || !slices.Equal(got.Tags, []string{"کتاب", "books"}) || got.CreatedBy != id.SourceUser {
			t.Fatalf("added %+v", got)
		}
		if at(x, "کتاب‌های تازه") < 0 || at(x, "کتاب‌های تازه") > at(x, btcTitle) {
			t.Error("the new link is not first")
		}

		// Edit the Bitcoin link's title.
		if err := x.tt.RightClick(btcTitle); err != nil {
			t.Fatal(err)
		}
		x.settle(nil)
		if err := x.tt.ChooseMenuItem("Edit"); err != nil {
			t.Fatalf("%v; menu %q", err, x.tt.Menu())
		}
		x.settle(nil)
		if !kit.HasRole(x.tt, "dialog", "Edit link") || x.st.Links.Editor.URL != btcURL || x.st.Links.Editor.Tags != "btc, crypto" {
			t.Fatalf("the Edit dialog: %+v", x.st.Links.Editor)
		}
		x.click(t, "Title")
		x.tt.Type(" today")
		x.click(t, "Save")
		x.settle(func() bool { return x.st.Links.Editor == nil })
		x.settle(func() bool { return x.tt.HasText(btcTitle + " today") })
		shows(t, x.tt, btcTitle+" today")
	})
}

func TestALinkIsDeletedAndPutBack(t *testing.T) {
	bothLinks(t, func(t *testing.T, x *fixture) {
		if err := x.tt.RightClick(faTitle); err != nil {
			t.Fatal(err)
		}
		x.settle(nil)
		if err := x.tt.ChooseMenuItem("Delete"); err != nil {
			t.Fatalf("%v; menu %q", err, x.tt.Menu())
		}
		// No time passes until Undo: the toast closes after a few seconds.
		x.loop.Drain(func() bool { return len(x.f.Links.Deleted) == 1 })
		x.tt.Frame()
		x.loop.Drain(func() bool { return len(x.st.Links.List(p).Links) == 2 })
		x.tt.Frame()
		shows(t, x.tt, "Deleted "+faTitle)
		if at(x, faNote) >= 0 {
			t.Error("the deleted link still shows")
		}
		shot(t, x, "links-deleted")
		x.click(t, "Undo")
		x.settle(func() bool { return len(x.f.Links.Deleted) == 0 })
		x.settle(func() bool { return len(x.st.Links.List(p).Links) == 3 })
		x.settle(nil)
		shows(t, x.tt, faTitle+" is back", faNote)
	})
}

// A link the agent saves shows at once, on project:links.
func TestTheLinksTabIsLive(t *testing.T) {
	bothLinks(t, func(t *testing.T, x *fixture) {
		x.f.AddLink(p, agent, "mailto:someone@example.com", "Write to the editor", "")
		x.st.Apply(app.EventLinks, project.LinksChanged{Project: p})
		if !x.settle(func() bool { return len(x.st.Links.List(p).Links) == 4 }) {
			t.Fatal("the list was not read again")
		}
		x.settle(nil)
		shows(t, x.tt, "Write to the editor", "someone@example.com")
		if at(x, "Write to the editor") > at(x, btcTitle) {
			t.Error("the agent's new link is not first")
		}
	})
}

// With no links the tab says how they come.
func TestTheLinksTabEmpty(t *testing.T) {
	both := func(dark bool) {
		x := startFiles(t, dark)
		x.click(t, "Links")
		x.settle(func() bool { return x.st.Links.List(p).Loaded })
		x.settle(nil)
		shows(t, x.tt, "No links yet")
		if x.st.Shell.RightTab != state.TabLinks {
			t.Error("the tab did not switch")
		}
	}
	both(false)
	both(true)
}

// The dialog belongs to its project's Links tab: another tab, the sidebar
// hidden or another project closes it, so a save can't land in a project
// the window no longer shows.
func TestTheLinkDialogClosesWithItsTab(t *testing.T) {
	x := startLinks(t, false)
	open := func() {
		t.Helper()
		x.click(t, "Add link")
		if !kit.HasRole(x.tt, "dialog", "Add link") {
			t.Fatalf("no Add link dialog: %q", x.tt.Texts())
		}
		x.tt.Type("https://example.org/half-typed")
		x.settle(nil)
	}
	closed := func(when string) {
		t.Helper()
		x.settle(nil)
		if x.st.Links.Editor != nil || kit.HasRole(x.tt, "dialog", "Add link") {
			t.Fatalf("after %s the dialog stays: %+v", when, x.st.Links.Editor)
		}
	}

	// The dialog is modal, so the change comes from elsewhere: a shortcut,
	// a link that shows a file, a project opened from a notification.
	open()
	x.st.Shell.RightTab = state.TabFiles
	closed("another tab")
	x.st.Shell.RightTab = state.TabLinks
	x.settle(nil)
	if kit.HasRole(x.tt, "dialog", "Add link") {
		t.Fatal("the dialog came back with its tab")
	}

	open()
	x.st.Shell.RightHidden = true
	closed("the sidebar hidden")
	x.st.Shell.RightHidden = false
	x.settle(nil)

	open()
	q, mother := x.f.AddProject("Other")
	x.f.AddLink(q, id.SourceUser, "https://other.example", "Other's link", "")
	x.st.OpenProject(q, mother)
	if !x.settle(func() bool { c := x.st.Nav.Current(); return c != nil && c.Project == q }) {
		t.Fatal("the other project didn't open")
	}
	closed("another project")
	if !x.settle(func() bool { return x.tt.HasText("Other's link") }) {
		t.Fatalf("the other project's links: %q", x.tt.Texts())
	}
	// The first project's list and search are let go.
	if _, ok := x.st.Links.Lists[p]; ok {
		t.Error("the first project's links are still held")
	}
	if x.tt.HasText(btcTitle) {
		t.Error("the first project's links show")
	}
}
