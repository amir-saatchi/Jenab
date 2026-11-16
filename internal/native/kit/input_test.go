package kit

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TASK-062: a text field takes the direction of its first strong letter,
// as dir="auto": Persian text starts at the right, Latin text and an
// empty field at the left, and nothing around the field moves.

func TestTextFieldsTakeTheirTextsDirection(t *testing.T) {
	var one, lines, composer string
	view := func(c *Context) {
		Column(c).Gap(12).Padding(16).Width(420).AlignItems(Stretch).Children(func() {
			Row(c).Gap(8).AlignItems(Center).Children(func() {
				Text(c, "Name")
				TextInput(c, &one, "Name").Grow(1)
			})
			TextArea(c, &lines, "Notes").Lines(2, 4)
			InputGroup(c, func() { ComposerText(c, &composer, "Message") })
		})
	}
	field := func(t *testing.T, tt *ui.Tester, label string) ui.Rect {
		t.Helper()
		a, ok := tt.AccessByRole(ui.RoleTextField, label)
		if !ok {
			t.Fatalf("no field %q", label)
		}
		return a.Bounds
	}
	// caretAt reports which half of the field the caret is in, after
	// what was typed.
	caretAt := func(t *testing.T, tt *ui.Tester, label string) string {
		t.Helper()
		tt.Frame()
		r := field(t, tt, label)
		caret, ok := tt.TextCaret()
		if !ok {
			t.Fatalf("no caret in %q", label)
		}
		if caret.X > r.X+r.W/2 {
			return "right"
		}
		return "left"
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		for _, label := range []string{"Name", "Notes", "Message"} {
			t.Run(label, func(t *testing.T) {
				one, lines, composer = "", "", ""
				tt.Frame()
				tabTo(t, tt, label)
				if got := caretAt(t, tt, label); got != "left" {
					t.Errorf("an empty field's caret is on the %s", got)
				}
				// The first strong letter decides: a digit first, then
				// Persian. The text is short, so its end, the caret, stays right of
				// the middle.
				tt.Type("۲ قیمت طلا")
				if got := caretAt(t, tt, label); got != "right" {
					t.Errorf("Persian text ends on the %s", got)
				}
				// Latin text reads left to right, so its caret, at its
				// end, is short of the middle.
				tt.Key(Ctrl, ui.KeyA)
				tt.Type("Gold")
				if got := caretAt(t, tt, label); got != "left" {
					t.Errorf("Latin text ends on the %s", got)
				}
			})
		}
		// The layout is never turned: the label stays before the field,
		// on the left, with Persian in it.
		one = persian
		tt.Frame()
		tt.Frame()
		label, ok := tt.Find("Name")
		if !ok {
			t.Fatal("no label")
		}
		if r := field(t, tt, "Name"); label.X > r.X {
			t.Errorf("the label moved after the field: label %v, field %v", label, r)
		}
		if a, _ := tt.AccessByRole(ui.RoleTextField, "Name"); a.Value != persian {
			t.Errorf("the field reads %q", a.Value)
		}
	})
}

// A path keeps its own direction, set after the field's.
func TestPathInputStaysLeftToRight(t *testing.T) {
	path := `C:\Jenab\قیمت`
	view := func(c *Context) {
		Column(c).Padding(16).Width(420).AlignItems(Stretch).Children(func() { PathInput(c, &path, "Folder") })
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		path = ""
		tt.Frame()
		tabTo(t, tt, "Folder")
		tt.Type("قیمت")
		tt.Frame()
		a, _ := tt.AccessByRole(ui.RoleTextField, "Folder")
		caret, ok := tt.TextCaret()
		if !ok || caret.X > a.Bounds.X+a.Bounds.W/2 {
			t.Errorf("the path field turned right to left: caret %v in %v", caret, a.Bounds)
		}
	})
}
