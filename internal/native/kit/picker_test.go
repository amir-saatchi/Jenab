package kit

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestPicker(t *testing.T) {
	selected := ""
	options := []PickerOption{{Value: "a", Label: "Prices · Mother"}, {Value: "b", Label: persian}, {Value: "c", Label: "Notes"}}
	view := func(c *Context) {
		between(c, func() { Picker(c, "Chat", &selected, options).Width(240) })
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		selected = ""
		shows(t, tt, "Chat") // nothing chosen yet
		if err := tt.Click("Chat"); err != nil {
			t.Fatal(err)
		}
		shows(t, tt, "Prices · Mother", persian, "Notes")
		if err := tt.Click(persian); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if selected != "b" {
			t.Errorf("chose %q", selected)
		}
		shows(t, tt, persian) // the trigger shows the choice
		if tt.HasText("Notes") {
			t.Error("the options stayed open")
		}
		// By the keyboard: Down opens it at the choice, Down moves, Enter
		// chooses.
		tabTo(t, tt, "Chat")
		tt.Key(0, ui.KeyDown)
		tt.Frame()
		shows(t, tt, "Notes")
		tt.Key(0, ui.KeyDown)
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if selected != "c" {
			t.Errorf("the keyboard chose %q", selected)
		}
		tt.Key(0, ui.KeyTab)
		if !tt.Focused("After") {
			t.Error("Tab did not go on past the picker")
		}
	})
}
