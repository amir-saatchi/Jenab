package kit

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// Each kit component in a light and a dark window: it shows Persian text,
// assistive technology finds it by its label, and the keyboard reaches it,
// or passes over it when it takes no input.

func TestButton(t *testing.T) {
	clicks := 0
	view := func(c *Context) {
		Row(c).Gap(8).Children(func() {
			for _, v := range []Variant{Default, Secondary, Outline, Ghost, Destructive} {
				if Button(c, "Save", v).Key(v).Clicked() {
					clicks++
				}
			}
			IconButton(c, IconPlus, "Add", Ghost)
			Disable(Button(c, "Off", Outline), true)
			Button(c, persian, Outline)
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		clicks = 0
		shows(t, tt, "Save", "Add", persian)
		if err := tt.Click("Save"); err != nil || clicks != 1 {
			t.Fatalf("click: %v, %d clicks", err, clicks)
		}
		tabTo(t, tt, "Add")
		tt.Key(0, ui.KeyTab)
		if tt.Focused("Off") {
			t.Error("a disabled button took the focus")
		}
		if !tt.Focused(persian) {
			t.Error("Tab did not go on to the next button")
		}
		tt.Key(0, ui.KeyEnter)
	})
}

func TestTextInputAndTextArea(t *testing.T) {
	var name, notes string
	view := func(c *Context) {
		Column(c).Gap(8).Width(300).Children(func() {
			TextInput(c, &name, "Name").Placeholder("Your name")
			TextArea(c, &notes, "Notes").Lines(3, 6)
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		name, notes = "", ""
		shows(t, tt, "Name", "Notes", "Your name")
		tabTo(t, tt, "Name")
		tt.Type(persian)
		tabTo(t, tt, "Notes")
		tt.Type("one\ttwo")
		tt.Frame()
		if name != persian || notes != "one\ttwo" {
			t.Errorf("typed %q and %q", name, notes)
		}
		// What the fields hold shows, which MyGo's Texts leaves out.
		shows(t, tt, persian, "one\ttwo")
		if tt.HasText(persian) {
			t.Log("MyGo's Texts now has a field's text: Shows can read it there")
		}
	})
}

func TestTooltip(t *testing.T) {
	view := func(c *Context) {
		between(c, func() { Tooltip(IconButton(c, IconCopy, "Copy", Ghost), persian) })
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		tabTo(t, tt, "Copy")
		tt.Advance(time.Second)
		shows(t, tt, persian)
	})
}

func TestMenus(t *testing.T) {
	chosen := ""
	view := func(c *Context) {
		Row(c).Gap(8).Children(func() {
			build := func(m *Menu) {
				if m.Item("Rename").Chosen() {
					chosen = "rename"
				}
				if m.Item(persian).Chosen() {
					chosen = "persian"
				}
			}
			MenuButton(c, "Actions", build)
			MoreButton(c, "More", build)
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		chosen = ""
		shows(t, tt, "Actions", "More")
		if err := tt.Click("Actions"); err != nil {
			t.Fatal(err)
		}
		shows(t, tt, "Rename", persian)
		if err := tt.Click(persian); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if chosen != "persian" {
			t.Errorf("chose %q", chosen)
		}
		tabTo(t, tt, "More")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		shows(t, tt, "Rename")
		tt.Key(0, ui.KeyEscape)
	})
}

func TestDialog(t *testing.T) {
	open := false // themes checks the window's background first
	var name string
	view := func(c *Context) {
		Dialog(c, &open, persian, func() {
			TextInput(c, &name, "Name")
			Row(c).Justify(End).Gap(8).Children(func() {
				if Button(c, "Cancel", Outline).Clicked() {
					open = false
				}
			})
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		open = true
		tt.Frame()
		shows(t, tt, persian, "Name")
		if !tt.Focused("Name") {
			t.Error("the dialog did not take the focus")
		}
		tt.Key(0, ui.KeyTab)
		tt.Key(0, ui.KeyTab)
		if !tt.Focused("Name") {
			t.Error("Tab left the dialog")
		}
		tt.Key(0, ui.KeyEscape)
		tt.Frame()
		if open {
			t.Error("Escape did not close it")
		}
	})
}

func TestAlertDialog(t *testing.T) {
	open, got := false, -2
	view := func(c *Context) {
		if r := AlertDialog(c, &open, "Delete the chat?", persian, "Cancel", "Delete"); r != -1 {
			got = r
		}
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		open, got = true, -2
		tt.Frame()
		shows(t, tt, "Delete the chat?", persian)
		if !tt.Focused("Delete") {
			t.Error("the default button has no focus")
		}
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if got != 1 || open {
			t.Errorf("Enter: got %d, open %v", got, open)
		}
		open = true
		tt.Frame()
		tt.Key(0, ui.KeyEscape)
		tt.Frame()
		if got != AlertDismissed {
			t.Errorf("Escape: got %d", got)
		}
	})
}

func TestBadgeSpinnerAlertEmpty(t *testing.T) {
	view := func(c *Context) {
		Column(c).Gap(8).Children(func() {
			between(c, func() {
				for _, tone := range Tones {
					Badge(c, string(tone), tone)
				}
				Badge(c, persian, Blue)
				Spinner(c, "Loading")
			})
			Alert(c, Red, "Can't reach the provider", persian)
			Alert(c, Amber, persian, "")
			Empty(c, IconInbox, "No chats yet", persian, func() { Button(c, "New chat", Default) })
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		shows(t, tt, "neutral", "purple", persian, "Loading", "Can't reach the provider", "No chats yet")
		passesOver(t, tt)
		tabTo(t, tt, "New chat")
	})
}

func TestSidebar(t *testing.T) {
	selected, open := "a", true
	view := func(c *Context) {
		Row(c).Grow(1).Children(func() {
			Sidebar(c, &selected, func() {
				SidebarSection(c, "Chats", &open, func() {
					SidebarItem(c, "a", IconChat, "Prices")
					SidebarItem(c, "b", IconChat, persian).Children(func() { Badge(c, "2", Blue) })
				})
				SidebarSection(c, "Projects", nil, func() {
					SidebarItem(c, "c", nil, "Coins")
				})
			}).Width(240)
			Button(c, "After", Outline)
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		selected, open = "a", true
		shows(t, tt, "Chats", "Prices", persian, "Coins")
		if err := tt.Click(persian); err != nil || selected != "b" {
			t.Fatalf("click: %v, selected %q", err, selected)
		}
		tt.Frame()
		// The item chosen is sidebar-accent gray while the sidebar has the
		// focus too, as in the web app; MyGo would fill it with the accent.
		k := &Light
		if near(tt.Image().RGBAAt(799, 599), Dark.Background) {
			k = &Dark
		}
		if r, ok := tt.Find("Prices"); ok {
			// Between the icon and the text, as the badge is at the end.
			x := int(r.X - 3)
			if got := tt.Image().RGBAAt(x, int(r.Y+r.H/2)); !near(got, k.Sidebar) {
				t.Errorf("an item not chosen is %v, want the sidebar %v", got, k.Sidebar)
			}
			// The Persian item below it, chosen.
			b, _ := tt.Find(persian)
			if got := tt.Image().RGBAAt(x, int(b.Y+b.H/2)); !near(got, k.SidebarAccent) {
				t.Errorf("the item chosen, focused, is %v, want sidebar-accent %v", got, k.SidebarAccent)
			}
		} else {
			t.Fatal("no Prices")
		}
		tt.Key(0, ui.KeyDown)
		tt.Frame()
		if selected != "c" {
			t.Errorf("Down chose %q", selected)
		}
		tt.Key(0, ui.KeyTab)
		if !tt.Focused("After") {
			t.Error("Tab did not leave the sidebar")
		}
		if err := tt.Click("Chats"); err != nil {
			t.Fatal(err)
		}
		if open {
			t.Error("the section's title did not close it")
		}
		selected, open = "a", true
	})
}

func TestToasts(t *testing.T) {
	shown := false
	view := func(c *Context) {
		if !shown {
			shown = true
			Toast(c, persian, ToastError, true)
			Toast(c, "Saved", ToastInfo, false)
		}
		Button(c, "Before", Outline)
		Toasts(c)
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		shown = false
		tt.Frame()
		shows(t, tt, persian, "Saved")
		said := false
		for _, a := range tt.Announcements() {
			said = said || a == persian
		}
		if !said {
			t.Errorf("not read out: %q", tt.Announcements())
		}
		tt.Advance(5 * time.Second)
		tt.Frame()
		if tt.HasText("Saved") || !tt.HasText(persian) {
			t.Errorf("after 5s: %q", tt.Texts())
		}
		tabTo(t, tt, "Close")
		tt.Key(0, ui.KeyEnter)
		tt.Advance(time.Second)
		tt.Frame()
		if tt.HasText(persian) {
			t.Error("Close did not close the sticky toast")
		}
	})
}
