package kit

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// The components of the project screens (TASK-057), as components_test.go
// tests the others: light and dark, Persian text, a label, the keyboard.

func TestFieldAndCheckField(t *testing.T) {
	var name string
	var checked bool
	problem := ""
	view := func(c *Context) {
		Column(c).Gap(12).Width(360).Children(func() {
			Field(c, "Name", "What the project is called", problem, func() {
				TextInput(c, &name, "Name")
			})
			CheckField(c, &checked, persian, "Without them, the copy asks again")
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		name, checked, problem = "", false, ""
		tt.Frame()
		shows(t, tt, "Name", "What the project is called", persian, "Without them, the copy asks again")
		tabTo(t, tt, "Name")
		tt.Type(persian)
		tabTo(t, tt, persian)
		tt.Key(0, ui.KeySpace)
		tt.Frame()
		if name != persian || !checked {
			t.Errorf("typed %q, checked %v", name, checked)
		}
		if a, ok := tt.AccessByRole(ui.RoleCheckBox, persian); !ok || !strings.Contains(a.Description, "asks again") {
			t.Errorf("the box reads %+v", a)
		}
		// A problem takes the description's place and marks the field.
		problem = "That name is taken."
		tt.Frame()
		shows(t, tt, "That name is taken.")
		if tt.HasText("What the project is called") {
			t.Error("the description shows with the problem")
		}
		if a, ok := tt.AccessByRole(ui.RoleTextField, "Name"); !ok || !a.Invalid {
			t.Errorf("the field is not marked invalid: %+v", a)
		}
	})
}

func TestSegmentGroup(t *testing.T) {
	level := "standard"
	choices := []Segment{{Value: "strict", Label: "Strict"}, {Value: "standard", Label: "Standard", Icon: IconProjectFolder}, {Value: "auto", Label: persian}}
	view := func(c *Context) {
		between(c, func() { SegmentGroup(c, &level, "Approval level", choices) })
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		level = "standard"
		tt.Frame()
		shows(t, tt, "Strict", "Standard", persian)
		if _, ok := tt.Access("Approval level"); !ok {
			t.Error("the group has no label")
		}
		if err := tt.Click(persian); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if level != "auto" {
			t.Errorf("chose %q", level)
		}
		// One Tab stop; the arrows choose in it.
		tabTo(t, tt, "Before")
		tt.Key(0, ui.KeyTab)
		tt.Key(0, ui.KeyHome)
		tt.Frame()
		if level != "strict" {
			t.Errorf("Home chose %q", level)
		}
		tt.Key(0, ui.KeyTab)
		if !tt.Focused("After") {
			t.Error("the segments are more than one Tab stop")
		}
	})
}

func TestSegmentGroupWithNothingChosen(t *testing.T) {
	format := ""
	view := func(c *Context) {
		SegmentGroup(c, &format, "Export as", []Segment{{Value: "zip", Label: "Zip file"}, {Value: "folder", Label: "Folder"}})
	}
	tt := window(t, false, view)
	tt.Frame()
	if format != "" {
		t.Errorf("a frame chose %q", format)
	}
	if err := tt.Click("Folder"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if format != "folder" {
		t.Errorf("chose %q", format)
	}
}

func TestProgress(t *testing.T) {
	view := func(c *Context) {
		Column(c).Gap(8).Width(300).Children(func() {
			Text(c, persian)
			Progress(c, 0.4, "Export progress")
			between(c, func() {})
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		shows(t, tt, persian)
		if _, ok := tt.Access("Export progress"); !ok {
			t.Error("the bar has no label")
		}
		passesOver(t, tt)
	})
}

func TestItemRowAndPath(t *testing.T) {
	removed := 0
	view := func(c *Context) {
		Column(c).Gap(8).Width(320).Children(func() {
			ItemRow(c, "nas.local", func() {
				Path(c, "nas.local").Grow(1)
				if IconButton(c, IconX, "Remove nas.local", Ghost).Clicked() {
					removed++
				}
			})
			ItemRow(c, persian, func() { Text(c, persian).Grow(1) })
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		removed = 0
		shows(t, tt, "nas.local", persian)
		if _, ok := tt.AccessByRole(ui.RoleGroup, "nas.local"); !ok {
			t.Error("the row is no group named by its label")
		}
		tabTo(t, tt, "Remove nas.local")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if removed != 1 {
			t.Errorf("%d removes", removed)
		}
	})
}

func TestPathInput(t *testing.T) {
	var host string
	view := func(c *Context) {
		Column(c).Width(300).Children(func() { PathInput(c, &host, "Add a host").Placeholder("127.0.0.1 or nas.local") })
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		host = ""
		shows(t, tt, "Add a host", "127.0.0.1 or nas.local")
		tabTo(t, tt, "Add a host")
		tt.Type("رایانه.local")
		tt.Frame()
		if host != "رایانه.local" {
			t.Errorf("typed %q", host)
		}
	})
}

func TestTextLink(t *testing.T) {
	clicks := 0
	view := func(c *Context) {
		Row(c).Gap(4).Children(func() {
			Text(c, persian)
			if TextLink(c, "change in Settings").Clicked() {
				clicks++
			}
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		clicks = 0
		shows(t, tt, persian, "change in Settings")
		tabTo(t, tt, "change in Settings")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if err := tt.Click("change in Settings"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if clicks != 2 {
			t.Errorf("%d clicks", clicks)
		}
	})
}

func TestActionButton(t *testing.T) {
	clicks, busy := 0, false
	view := func(c *Context) {
		between(c, func() {
			if ActionButton(c, IconPlus, persian, Default, false).Clicked() {
				clicks++
			}
			ActionButton(c, nil, "Create", Default, busy)
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		clicks, busy = 0, false
		tt.Frame()
		shows(t, tt, persian, "Create")
		tabTo(t, tt, persian)
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if clicks != 1 {
			t.Errorf("%d clicks", clicks)
		}
		tt.Key(0, ui.KeyTab)
		if !tt.Focused("Create") {
			t.Error("Tab did not reach Create")
		}
		// While busy a spinner shows and the button takes no focus.
		busy = true
		tt.Frame()
		tabTo(t, tt, persian)
		tt.Key(0, ui.KeyTab)
		if !tt.Focused("After") {
			t.Error("a busy button took the focus")
		}
	})
}

func TestDialogTextFooterAndLocked(t *testing.T) {
	open, busy := false, false
	view := func(c *Context) {
		Dialog(c, Locked(&open, busy), "Export chat", func() {
			DialogText(c, "Save "+persian+" as a file.")
			DialogFooter(c, func() {
				if Button(c, "Cancel", Outline).Clicked() {
					open = false
				}
				Button(c, "Export…", Default)
			})
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		open, busy = true, true
		tt.Frame()
		shows(t, tt, "Save "+persian+" as a file.", "Cancel", "Export…")
		// While busy, Escape leaves it open.
		tt.Key(0, ui.KeyEscape)
		tt.Frame()
		if !open || !tt.HasText("Export…") {
			t.Fatal("Escape closed a busy dialog")
		}
		busy = false
		tt.Frame()
		tabTo(t, tt, "Export…")
		tt.Key(0, ui.KeyEscape)
		tt.Frame()
		if open {
			t.Error("Escape did not close it once done")
		}
	})
}

func TestNavRow(t *testing.T) {
	clicks, chosen := 0, ""
	shown := false
	view := func(c *Context) {
		Column(c).Width(260).AlignItems(Stretch).Children(func() {
			Button(c, "Before", Outline)
			if NavRow(c, "a", persian, true, "Chat menu: "+persian, func(menu bool) {
				shown = menu
				Lines(c, persian, "Project home", Neutral)
			}, func(m *Menu) {
				if m.Item("Export chat…").Chosen() {
					chosen = "export"
				}
			}) {
				clicks++
			}
			NavRow(c, "b", "Notes", false, "Chat menu: Notes", func(bool) { Lines(c, "Notes", "", Neutral) }, func(m *Menu) { m.Item("Export chat…") })
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		clicks, chosen = 0, ""
		tt.Frame()
		shows(t, tt, persian, "Notes")
		if shown {
			t.Error("the menu's button shows on a row neither hovered nor focused")
		}
		if err := tt.Click(persian); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if clicks != 1 {
			t.Errorf("%d clicks", clicks)
		}
		// Tab goes to the row, then its menu button, which shows.
		tabTo(t, tt, "Before")
		tt.Key(0, ui.KeyTab)
		tt.Key(0, ui.KeyTab)
		tt.Frame()
		if !tt.Focused("Chat menu: " + persian) {
			t.Fatal("Tab did not reach the row's menu button")
		}
		if !shown {
			t.Error("the menu's button hides while it has the focus")
		}
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if err := tt.ChooseMenuItem("Export chat…"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if chosen != "export" {
			t.Errorf("chose %q", chosen)
		}
	})
}

func TestMenuTitleAndTileSpinner(t *testing.T) {
	chosen := ""
	view := func(c *Context) {
		Row(c).Gap(8).Children(func() {
			Column(c).Width(240).Children(func() {
				MenuTitle(c, persian, `C:\Jenab\projects\demo`, "Project menu", func(m *Menu) {
					if m.Item("Project settings").Chosen() {
						chosen = "settings"
					}
					m.Item("Export…")
				})
			})
			RailTile(c, "Notes · working", false, func() {
				TileFace(c, "No", false, false)
				TileSpinner(c, "Working")
			})
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		chosen = ""
		shows(t, tt, persian, `C:\Jenab\projects\demo`, "Notes · working")
		// The tile's label tells it works; its spinner is what it shows.
		tabTo(t, tt, "Project menu")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if menu := strings.Join(tt.Menu(), "|"); !strings.Contains(menu, "Project settings") || !strings.Contains(menu, "Export…") {
			t.Fatalf("the menu is %q", menu)
		}
		if err := tt.ChooseMenuItem("Project settings"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if chosen != "settings" {
			t.Errorf("chose %q", chosen)
		}
	})
}
