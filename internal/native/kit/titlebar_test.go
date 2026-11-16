package kit

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// titleBarView is a chat list beside a main area, whose bar has a
// right-to-left title, a button and two toggles, then ControlsRoom.
func titleBarView(clicked *string, files *bool) func(c *Context) {
	return func(c *Context) {
		Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			HeadedSidePanel(c, "Chats", func() { Title(c, "Demo").Grow(1) }, func() {
				Group(c, "Chats")
			})
			Main(c, func() {
				Bar(c, func() {
					Title(c, persian)
					Spacer(c)
					if IconButton(c, IconMenu, "Chat menu", Ghost).Size(32, 32).Clicked() {
						*clicked = "Chat menu"
					}
					BarGroup(c, "Project sidebar", func() {
						if BarToggle(c, IconPagesTab, "Pages", false).Changed() {
							*clicked = "Pages"
						}
						if BarToggle(c, IconFolder, "Files", *files).Changed() {
							*clicked = "Files"
							*files = !*files
						}
					})
					ControlsRoom(c)
				})
			})
		})
	}
}

// The bar's buttons take their clicks and the keyboard though the bar
// moves the window, its toggles are named toggle buttons pressed while
// on, and its end stays clear of the window's buttons.
func TestBarKeepsClearOfTheWindowButtons(t *testing.T) {
	clicked, files := "", true
	themes(t, titleBarView(&clicked, &files), func(t *testing.T, tt *ui.Tester) {
		clicked, files = "", true
		WindowsButtons(tt)
		shows(t, tt, persian, "Demo", "Chat menu", "Pages", "Files")
		if under := UnderWindowButtons(tt, 800); len(under) > 0 {
			t.Errorf("under the window's buttons: %q", under)
		}
		if !HasRole(tt, "group", "Project sidebar") {
			t.Error("the toggles are not a group named Project sidebar")
		}

		// Clear of the buttons: 8 DIPs between the last toggle and them.
		r, _ := tt.Find("Files")
		if end := r.X + r.W; end > 800-138-6 || end < 800-138-10 {
			t.Errorf("the last toggle ends at %v, want 8 before the buttons at %v", end, 800-138)
		}

		// A click on a button in the bar clicks it.
		if err := tt.Click("Chat menu"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if clicked != "Chat menu" {
			t.Errorf("a click in the bar clicked %q", clicked)
		}

		// The toggles: pressed while on, and Space presses them.
		a, ok := tt.AccessByRole(ui.RoleToggleButton, "Files")
		if !ok || !a.Checked {
			t.Fatalf("Files is %+v, want a toggle button pressed", a)
		}
		if a, _ := tt.AccessByRole(ui.RoleToggleButton, "Pages"); a.Checked {
			t.Error("Pages is pressed")
		}
		tabTo(t, tt, "Files")
		tt.Key(0, ui.KeySpace)
		tt.Frame()
		if clicked != "Files" || files {
			t.Errorf("Space on Files: clicked %q, on %v", clicked, files)
		}
		if a, _ := tt.AccessByRole(ui.RoleToggleButton, "Files"); a.Checked {
			t.Error("Files is still pressed")
		}

		// A double click on the bar's empty part clicks nothing in it.
		clicked = ""
		title, _ := tt.Find(persian)
		tt.ClickAt(title.X+title.W+20, title.Y+title.H/2)
		tt.ClickAt(title.X+title.W+20, title.Y+title.H/2)
		tt.Frame()
		if clicked != "" {
			t.Errorf("a double click on the bar clicked %q", clicked)
		}
	})
}

// Without the window's buttons, as where the system keeps its title bar,
// ControlsRoom takes no room but the bar's gap before it.
func TestControlsRoomIsEmptyWithoutWindowButtons(t *testing.T) {
	clicked, files := "", false
	tt := window(t, false, titleBarView(&clicked, &files))
	r, _ := tt.Find("Files")
	if end := r.X + r.W; end < 800-12-8-1 {
		t.Errorf("the last toggle ends at %v, want at the bar's padding and gap, %v", end, 800-12-8)
	}
}

// The side panel's head and the bar line up as one: as high, with their
// rules on one line, and taller with taller window buttons.
func TestPanelHeadLinesUpWithTheBar(t *testing.T) {
	clicked, files := "", false
	for _, bar := range []ui.TitleBar{{}, windowsButtonsRoom, {Height: 64, Right: 120, Bottom: 64}} {
		tt := window(t, true, titleBarView(&clicked, &files))
		tt.SetTitleBar(bar)
		head, _ := tt.Find("Demo")
		title, _ := tt.Find(persian)
		want := max(float32(BarHeight), bar.Height)
		// Both titles are centered in their bars.
		if mid := head.Y + head.H/2; mid < want/2-2 || mid > want/2+2 {
			t.Errorf("title bar %v: the panel's title is centered at %v, want %v", bar, mid, want/2)
		}
		if mid := title.Y + title.H/2; mid < want/2-2 || mid > want/2+2 {
			t.Errorf("title bar %v: the bar's title is centered at %v, want %v", bar, mid, want/2)
		}
		// The rules under them, at want-1 in the panel and in the bar: a
		// row unlike the bar's face above it and the content under it.
		img := tt.Image()
		y := int(want) - 1
		for _, x := range []int{100, 500} {
			rule := img.RGBAAt(x, y)
			if rule == img.RGBAAt(x, y-3) || rule == img.RGBAAt(x, y+3) {
				t.Errorf("title bar %v: no rule at %d, %d", bar, x, y)
			}
		}
	}
}

// PanelTitle is a heading.
func TestPanelTitle(t *testing.T) {
	themes(t, func(c *Context) {
		EndPanel(c, "Project", func() {
			PanelHead(c, func() { Spacer(c); ControlsRoom(c) })
			PanelTitle(c, persian)
		})
	}, func(t *testing.T, tt *ui.Tester) {
		if !HasRole(tt, "heading", persian) || !HasRole(tt, "complementary", "Project") {
			t.Error("no heading in the panel")
		}
	})
}

func TestThemeSourceOf(t *testing.T) {
	for s, want := range map[string]string{"light": "light", "dark": "dark", "system": "system", "": "system", "blue": "system"} {
		if got := string(ThemeSourceOf(s)); got != want {
			t.Errorf("ThemeSourceOf(%q) = %q, want %q", s, got, want)
		}
	}
}
