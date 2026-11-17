package kit

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// settings/format.test.ts's readNumber cases.
func TestReadNumber(t *testing.T) {
	val := func(r NumberRead, v *int) any {
		switch {
		case r == NumberSame:
			return "same"
		case r == NumberBad:
			return "bad"
		case v == nil:
			return "nil"
		}
		return *v
	}
	cases := []struct {
		text, shown string
		min         int
		optional    bool
		want        any
	}{
		{" 4 ", "4", 1, false, "same"},
		{"8", "4", 1, false, 8},
		{"0", "4", 1, false, "bad"},
		{"0", "4", 0, false, 0},
		{"2.5", "4", 1, false, "bad"},
		{"", "4", 0, false, "bad"}, // empty is no zero
		{"", "4", 1, true, "nil"},
		{"", "", 1, true, "same"},
		// Persian and Arabic digits are the same numbers (TASK-069).
		{"۴", "4", 1, false, "same"},
		{"۱۲", "4", 1, false, 12},
		{"١٢", "4", 1, false, 12},
		{" ۱2 ", "4", 1, false, 12},
		{"۰", "4", 1, false, "bad"},
		{"۱٫۵", "4", 1, false, "bad"},
	}
	for _, tc := range cases {
		if got := val(ReadNumber(tc.text, tc.shown, tc.min, tc.optional)); got != tc.want {
			t.Errorf("ReadNumber(%q, %q, %d, %v) = %v, want %v", tc.text, tc.shown, tc.min, tc.optional, got, tc.want)
		}
	}
}

func TestNumberInput(t *testing.T) {
	var value *int
	var commits []any
	var last Number
	view := func(c *Context) {
		between(c, func() {
			Text(c, persian)
			n := NumberInput(c, value, "Parallel calls", NumberSpec{Min: 1, Optional: true})
			n.Element.Placeholder("2")
			if n.Committed {
				if n.Value == nil {
					commits = append(commits, "nil")
				} else {
					commits = append(commits, *n.Value)
				}
				value = n.Value
			}
			last = n
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		four := 4
		value, commits = &four, nil
		tt.Frame()
		shows(t, tt, persian, "4", "Parallel calls")
		tabTo(t, tt, "Parallel calls")

		// Enter commits a new value.
		tt.Key(ui.Ctrl, ui.KeyA)
		tt.Type("8")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if len(commits) != 1 || commits[0] != 8 {
			t.Fatalf("Enter committed %v, want [8]", commits)
		}

		// Below the least goes back, marked; nothing is committed.
		tt.Key(ui.Ctrl, ui.KeyA)
		tt.Type("0")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if len(commits) != 1 || !Shows(tt, "8") {
			t.Errorf("0 committed %v, the field shows %q", commits, tt.Texts())
		}
		if last.st.bad != "At least 1" {
			t.Errorf("the field is marked %q", last.st.bad)
		}

		// Escape puts the value back.
		tt.Key(ui.Ctrl, ui.KeyA)
		tt.Type("12")
		tt.Key(0, ui.KeyEscape)
		tt.Frame()
		if !Shows(tt, "8") || Shows(tt, "12") {
			t.Errorf("Escape left %q", tt.Texts())
		}

		// Leaving the field commits; an emptied optional field is nil.
		tt.Key(ui.Ctrl, ui.KeyA)
		tt.Key(0, ui.KeyBackspace)
		tt.Key(0, ui.KeyTab)
		tt.Frame()
		if len(commits) != 2 || commits[1] != "nil" || !tt.Focused("After") {
			t.Errorf("leaving committed %v", commits)
		}
		if last.st.bad != "" {
			t.Errorf("a good commit left the mark %q", last.st.bad)
		}

		// A failed save marks it and puts the value shown back.
		three := 3
		value = &three
		tt.Frame()
		tabTo(t, tt, "Parallel calls")
		tt.Key(ui.Ctrl, ui.KeyA)
		tt.Type("5")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		value = &three
		last.Failed()
		tt.Frame()
		if !Shows(tt, "3") || last.st.bad != "Not saved" {
			t.Errorf("a failed save shows %q, marked %q", tt.Texts(), last.st.bad)
		}
	})
}

// TASK-069: Enter and then leaving the field commit a value once while its
// save is still on its way; a new value, or the same one after the value
// shown changed or the save failed, commits again.
func TestNumberInputCommitsOnce(t *testing.T) {
	var value *int
	var commits []int
	var last Number
	view := func(c *Context) {
		between(c, func() {
			n := NumberInput(c, value, "Parallel calls", NumberSpec{Min: 1})
			if n.Committed {
				commits = append(commits, *n.Value) // saved later: value stays
			}
			last = n
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		four := 4
		value, commits = &four, nil
		tt.Frame()
		tabTo(t, tt, "Parallel calls")
		tt.Key(ui.Ctrl, ui.KeyA)
		tt.Type("۸")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		tt.Key(0, ui.KeyTab)
		tt.Frame()
		if len(commits) != 1 || commits[0] != 8 {
			t.Fatalf("Enter, then Tab committed %v, want [8]", commits)
		}

		// Another value while the first is on its way commits.
		tabTo(t, tt, "Parallel calls")
		tt.Key(ui.Ctrl, ui.KeyA)
		tt.Type("9")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if len(commits) != 2 || commits[1] != 9 {
			t.Fatalf("a new value committed %v, want [8 9]", commits)
		}

		// The save failed: the same value again commits.
		last.Failed()
		tt.Frame()
		tt.Key(ui.Ctrl, ui.KeyA)
		tt.Type("9")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if len(commits) != 3 {
			t.Fatalf("after a failed save, 9 committed %v", commits)
		}

		// The value arrived: the field shows it, and the same text is no change.
		nine := 9
		value = &nine
		tt.Frame()
		tt.Key(ui.Ctrl, ui.KeyA)
		tt.Type("٩")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if len(commits) != 3 {
			t.Errorf("the value shown, typed again, committed %v", commits)
		}
	})
}

// TASK-076: a save that ends without changing the value shown ends the
// commit too, so the same text commits again; an older commit's save
// doesn't end a newer one's.
func TestNumberInputCommitsAgainAfterASave(t *testing.T) {
	var value *int
	var commits []int
	var saves []Number
	view := func(c *Context) {
		between(c, func() {
			n := NumberInput(c, value, "Parallel calls", NumberSpec{Min: 1})
			if n.Committed {
				commits = append(commits, *n.Value)
				saves = append(saves, n)
			}
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		four := 4
		value, commits, saves = &four, nil, nil
		tt.Frame()
		tabTo(t, tt, "Parallel calls")
		commit := func(text string) {
			tt.Key(ui.Ctrl, ui.KeyA)
			tt.Type(text)
			tt.Key(0, ui.KeyEnter)
			tt.Frame()
		}
		commit("8")
		saves[0].Saved() // saved, but the value shown is still 4
		tt.Frame()
		commit("8")
		if len(commits) != 2 {
			t.Fatalf("after its save, 8 again committed %v, want [8 8]", commits)
		}

		// 9 is sent, then 8's second save ends: 9 still holds.
		commit("9")
		saves[1].Saved()
		tt.Frame()
		tt.Key(0, ui.KeyTab)
		tt.Frame()
		if len(commits) != 3 || commits[2] != 9 {
			t.Errorf("an older save let 9 commit twice: %v", commits)
		}
	})
}
