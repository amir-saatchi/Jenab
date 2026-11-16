package kit

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSettingsLayout(t *testing.T) {
	var nav SettingsNav
	backs := 0
	var on bool
	pages := []string{"General", "Memory", "Usage"}
	view := func(c *Context) {
		Root(c, func() {
			if SettingsLayout(c, "", pages, &nav, false, func() {
				Group(c, persian)
				PageItem(c, IconFolderCog, "Project settings", false)
			}, func() {
				SettingsTitle(c, "Settings", "")
				for i, p := range pages {
					if i > 0 {
						SettingsDivider(c)
					}
					SettingsSection(c, p, "About "+p, func() {
						FactRow(c, "Data folder", func() { Mono(c, `C:\Data`) }, func() { Button(c, "Change…", Outline) })
						SettingRow(c, "Switch "+p, persian, func() { Switch(c, &on, "Switch "+p) })
						Box(c).Height(400)
					})
				}
			}) {
				backs++
			}
		})
	}
	for _, dark := range []bool{false, true} {
		if err := RegisterFonts(); err != nil {
			t.Fatal(err)
		}
		tt := ui.NewTester(func(c *ui.Context) { view(c) }, 1000, 600)
		tt.SetDark(dark)
		tt.Frame()
		shows(t, tt, "Back to chats", "General", "Memory", "Usage", "Settings", persian, "Project settings", "Data folder", `C:\Data`, "About General")
		if nav.Active != "General" {
			t.Errorf("the first page is not chosen but %q", nav.Active)
		}
		before, _ := tt.Find("About Usage")
		if err := tt.Click("Usage"); err != nil {
			t.Fatal(err)
		}
		tt.Advance(2e9)
		tt.Frame()
		after, _ := tt.Find("About Usage")
		if nav.Active != "Usage" || after.Y >= before.Y || after.Y > 200 {
			t.Errorf("Usage is %q at %v, was at %v", nav.Active, after.Y, before.Y)
		}
		if err := tt.Click("Back to chats"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if backs != 1 {
			t.Errorf("Back to chats reported %d clicks", backs)
		}
		backs, nav = 0, SettingsNav{}
	}
}
