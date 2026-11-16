package kit

import (
	"bytes"
	"image"
	"image/png"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// The components of the files panel, the file view and the dock
// (TASK-056), as components_test.go's.

func TestTree(t *testing.T) {
	open := false
	var clicked []string
	deleted := ""
	view := func(c *Context) {
		between(c, func() {
			Tree(c, "Files", func(rows TreeRows) {
				for _, name := range []string{"notes.md", persian + ".md"} {
					menu := func(m *Menu) {
						if m.Item("Delete").Chosen() {
							deleted = name
						}
					}
					r := rows.Item(name, name, nil, name == "notes.md", func() { Lines(c, name, "", Neutral) }, menu)
					if r.Clicked() {
						clicked = append(clicked, name)
					}
				}
				r := rows.Item("docs/", "docs", &open, false, func() { Text(c, "docs") }, nil)
				r.Group(func() {
					rows.Item("docs/a.txt", "a.txt", nil, false, func() { Text(c, "a.txt") }, nil)
				})
			}).Width(260)
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		open, clicked, deleted = false, nil, ""
		shows(t, tt, "notes.md", persian+".md", "docs")
		if _, ok := tt.AccessByRole(ui.RoleTree, "Files"); !ok {
			t.Fatal("no tree named Files")
		}
		if n, ok := tt.AccessByRole(ui.RoleTreeItem, "notes.md"); !ok || !n.Selected {
			t.Errorf("the active row: %+v %v", n, ok)
		}
		// One Tab stop, the rows' "…" passed over.
		tabTo(t, tt, "notes.md")
		tt.Key(0, ui.KeyDown)
		if !tt.Focused(persian + ".md") {
			t.Fatal("Down did not move to the next row")
		}
		tt.Key(0, ui.KeyEnter)
		tt.Key(0, ui.KeyEnd)
		if !tt.Focused("docs") {
			t.Fatal("End did not move to the last row")
		}
		tt.Key(0, ui.KeyRight)
		tt.Frame()
		if !open || !Shows(tt, "a.txt") {
			t.Fatal("Right did not open the branch")
		}
		tt.Key(0, ui.KeyRight)
		if !tt.Focused("a.txt") {
			t.Fatal("Right did not move into the branch")
		}
		tt.Key(0, ui.KeyTab)
		if !tt.Focused("After") {
			t.Error("Tab stopped inside the tree")
		}
		if len(clicked) != 1 || clicked[0] != persian+".md" {
			t.Errorf("clicked %q", clicked)
		}
		// The menu, from a right click.
		if err := tt.RightClick("notes.md"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem("Delete"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if deleted != "notes.md" {
			t.Errorf("deleted %q", deleted)
		}
	})
}

func TestSplit(t *testing.T) {
	var width float32
	start := false
	view := func(c *Context) {
		Split(c, &width, 200, start, "Chat and file", func() {
			Column(c).Grow(1).Children(func() { Button(c, "Main", Outline) })
		}, func() {
			Column(c).Grow(1).Children(func() { Text(c, persian) })
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		width, start = 0, false
		tt.Frame()
		shows(t, tt, "Main", persian)
		if _, ok := tt.AccessByRole(ui.RoleGroup, "Chat and file"); !ok {
			t.Error("no group named Chat and file")
		}
		div, ok := tt.AccessByRole(ui.RoleSplitter, "")
		if !ok {
			t.Fatal("no divider")
		}
		// A third of 800, on the end: the line at about 533.
		if x := div.Bounds.X; x < 525 || x > 540 {
			t.Errorf("the divider is at %v, want about 533", x)
		}
		side, _ := tt.Find(persian)
		if side.X < div.Bounds.X {
			t.Errorf("the side pane is at %v, before the divider at %v", side.X, div.Bounds.X)
		}
		// The arrows move the line their way, growing the side pane on the
		// end with Left.
		tabTo(t, tt, div.Name)
		tt.Key(0, ui.KeyLeft)
		tt.Frame()
		if width <= 266 {
			t.Errorf("Left made the side pane %v wide", width)
		}
		// It keeps min, and leaves main as much.
		width = 50
		tt.Frame()
		tt.Frame()
		if div, _ = tt.AccessByRole(ui.RoleSplitter, ""); div.Bounds.X < 595 || div.Bounds.X > 601 {
			t.Errorf("a narrow side pane put the divider at %v", div.Bounds.X)
		}
		width = 700
		tt.Frame()
		tt.Frame()
		if div, _ = tt.AccessByRole(ui.RoleSplitter, ""); div.Bounds.X < 198 || div.Bounds.X > 202 {
			t.Errorf("a wide side pane put the divider at %v", div.Bounds.X)
		}
		// On the start edge.
		width, start = 0, true
		tt.Frame()
		tt.Frame()
		div, _ = tt.AccessByRole(ui.RoleSplitter, "")
		side, _ = tt.Find(persian)
		if div.Bounds.X < 260 || div.Bounds.X > 275 || side.X > div.Bounds.X {
			t.Errorf("on the start edge: the divider at %v, the side pane at %v", div.Bounds.X, side.X)
		}
	})
}

func TestTabs(t *testing.T) {
	tab := 0
	view := func(c *Context) {
		between(c, func() {
			Tabs(c, &tab, "Sidebar", "Pages", "Files", persian).Width(320)
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		tab = 0
		shows(t, tt, "Pages", "Files", persian)
		if _, ok := tt.AccessByRole(ui.RoleTabList, "Sidebar"); !ok {
			t.Error("no tab list named Sidebar")
		}
		if err := tt.Click("Files"); err != nil || tab != 1 {
			t.Fatalf("click: %v, tab %d", err, tab)
		}
		tt.Key(0, ui.KeyRight)
		tt.Frame()
		if tab != 2 {
			t.Errorf("Right chose tab %d", tab)
		}
		if n, _ := tt.AccessByRole(ui.RoleTab, persian); !n.Selected {
			t.Error("the tab chosen isn't selected")
		}
		tt.Key(0, ui.KeyTab)
		if !tt.Focused("After") {
			t.Error("the tabs are more than one stop")
		}
	})
}

func TestPathBar(t *testing.T) {
	chosen := -1
	view := func(c *Context) {
		between(c, func() { PathBar(c, []string{"Files", "docs", persian}, &chosen) })
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		chosen = -1
		shows(t, tt, "Files", "docs", persian)
		if _, ok := tt.AccessByRole(ui.RoleNavigation, "Path"); !ok {
			t.Error("no navigation named Path")
		}
		tabTo(t, tt, "Files")
		tt.Key(0, ui.KeyTab)
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if chosen != 1 {
			t.Errorf("Enter on docs chose %d", chosen)
		}
		tt.Key(0, ui.KeyTab)
		if !tt.Focused("After") {
			t.Error("the place shown took the focus")
		}
	})
}

func TestStatusBar(t *testing.T) {
	clicks := 0
	view := func(c *Context) {
		// Above the corner, which themes checks.
		Column(c).Grow(1).Padding(0, 0, 8, 0).Children(func() {
			Spacer(c)
			StatusBar(c, func() {
				if StatusButton(c, "2 working · 1 waiting").Clicked() {
					clicks++
				}
				Spacer(c)
				StatusText(c, "Z.ai paused, retry in 30 s", Amber, "")
				StatusText(c, persian, Neutral, "Free memory, of 16 GB RAM")
				StatusText(c, "Memory low", Red, "")
			})
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		clicks = 0
		shows(t, tt, "2 working · 1 waiting", "Z.ai paused, retry in 30 s", persian, "Memory low")
		if n, ok := tt.AccessByRole(ui.RoleContentInfo, "Status"); !ok || n.Bounds.H != 24 || n.Bounds.Y != 568 {
			t.Errorf("the bar: %+v %v", n.Bounds, ok)
		}
		tabTo(t, tt, "2 working · 1 waiting")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if clicks != 1 {
			t.Errorf("%d clicks", clicks)
		}
		r, _ := tt.Find(persian)
		tt.Move(r.X+r.W/2, r.Y+r.H/2)
		tt.Advance(time.Second)
		shows(t, tt, "Free memory, of 16 GB RAM")
	})
}

func TestPicture(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 400, 300))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	okPic, okBad := false, true
	view := func(c *Context) {
		between(c, func() {
			_, okPic = Picture(c, "a", data, persian, 150)
			_, okBad = Picture(c, "b", []byte("not a picture"), "Bad", 150)
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		if !okPic || okBad {
			t.Fatalf("decoded: %v, %v", okPic, okBad)
		}
		n, ok := tt.AccessByRole(ui.RoleImage, persian)
		if !ok || n.Bounds.H != 150 || n.Bounds.W != 200 {
			t.Errorf("the picture: %+v %v", n.Bounds, ok)
		}
		passesOver(t, tt)
	})
}

func TestCopyTextAndToastAction(t *testing.T) {
	undone, copied := 0, false
	view := func(c *Context) {
		if Button(c, "Copy link", Outline).Clicked() {
			CopyText(c, "jenab://p/"+persian)
			copied = true
		}
		if Button(c, "Delete", Outline).Clicked() {
			ToastAction(c, "Deleted a.md", "From the app cache.", "Undo", func() { undone++ })
		}
		Toasts(c)
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		undone, copied = 0, false
		if err := tt.Click("Copy link"); err != nil || !copied || tt.Clipboard() != "jenab://p/"+persian {
			t.Fatalf("copy: %v, %q", err, tt.Clipboard())
		}
		if err := tt.Click("Delete"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		shows(t, tt, "Deleted a.md", "From the app cache.", "Undo")
		if err := tt.Click("Undo"); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
		tt.Frame()
		if undone != 1 || tt.HasText("Deleted a.md") {
			t.Errorf("Undo: %d, %q", undone, tt.Texts())
		}
	})
}

func TestItem(t *testing.T) {
	view := func(c *Context) {
		between(c, func() {
			Column(c).Gap(8).Width(320).Children(func() {
				Item(c, "Version 2", "Current", Neutral, "1.5 KB · by you")
				Item(c, "Version 1", "", Neutral, persian)
			})
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		shows(t, tt, "Version 2", "Current", "1.5 KB · by you", "Version 1", persian)
		passesOver(t, tt)
	})
}

func TestLinkTextAndBubble(t *testing.T) {
	const link = "jenab://01JABCDEFGHJKMNPQRSTVWXYZ0/docs/a.md"
	var opened []string
	open := func(l string) { opened = append(opened, l) }
	parts := []TextPart{{Text: persian + " "}, {Text: link, Link: true}}
	view := func(c *Context) {
		between(c, func() {
			Column(c).Gap(8).Width(500).Children(func() {
				LinkBubble(c, parts, false, open)
				LinkText(c, []TextPart{{Text: "Stored as "}, {Text: "cache/tool/x.txt", Link: true}}, open).FontSize(12)
			})
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		opened = nil
		shows(t, tt, persian, link, "cache/tool/x.txt")
		if _, ok := tt.AccessByRole(ui.RoleLink, link); !ok {
			t.Error("the link isn't a link")
		}
		if err := tt.Click(link); err != nil {
			t.Fatal(err)
		}
		tabTo(t, tt, "cache/tool/x.txt")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if len(opened) != 2 || opened[0] != link || opened[1] != "cache/tool/x.txt" {
			t.Errorf("opened %q", opened)
		}
		if len(tt.OpenedURLs()) != 0 {
			t.Errorf("the browser opened %q", tt.OpenedURLs())
		}
	})
}

func TestMarkdownOwnsItsLinks(t *testing.T) {
	const link = "jenab://01JABCDEFGHJKMNPQRSTVWXYZ0/docs/a.md"
	var owned []string
	md := &Markdown{
		Own: func(u string) bool {
			if len(u) > 8 && u[:8] == "jenab://" {
				owned = append(owned, u)
				return true
			}
			return false
		},
		CodeLink: func(code string) (string, bool) { return code, code == link },
	}
	src := persian + " <" + link + "> and `" + link + "` and [the web](https://example.com)."
	view := func(c *Context) { Column(c).Width(700).Children(func() { md.View(c, src) }) }
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		owned = nil
		if _, ok := tt.AccessByRole(ui.RoleLink, "the web"); !ok {
			t.Fatalf("no links in %q", tt.Texts())
		}
		var links []ui.AccessInfo
		for _, n := range tt.AccessAll() {
			if n.Role == ui.RoleLink {
				links = append(links, n)
			}
		}
		if len(links) != 3 {
			t.Fatalf("%d links: %+v", len(links), links)
		}
		for _, n := range links[:2] {
			tt.ClickAt(n.Bounds.X+n.Bounds.W/2, n.Bounds.Y+n.Bounds.H/2)
		}
		if err := tt.Click("the web"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if len(owned) != 2 || owned[0] != link || owned[1] != link {
			t.Errorf("owned %q", owned)
		}
		if u := tt.OpenedURLs(); len(u) != 1 || u[0] != "https://example.com" {
			t.Errorf("the browser opened %q", u)
		}
	})
}

func TestEndPanel(t *testing.T) {
	view := func(c *Context) {
		Row(c).Grow(1).Children(func() {
			Column(c).Grow(1).Children(func() { Button(c, "Before", Outline) })
			EndPanel(c, "Project", func() {
				Text(c, persian)
				Button(c, "After", Outline)
			})
			// The window's corner, which themes checks.
			Box(c).Width(8)
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		shows(t, tt, persian)
		n, ok := tt.AccessByRole(ui.RoleComplementary, "Project")
		if !ok || n.Bounds.W < 286 || n.Bounds.W > 288 {
			t.Errorf("the panel: %+v %v", n.Bounds, ok)
		}
		tabTo(t, tt, "Before")
		tt.Key(0, ui.KeyTab)
		if !tt.Focused("After") {
			t.Error("Tab did not reach the panel")
		}
	})
}
