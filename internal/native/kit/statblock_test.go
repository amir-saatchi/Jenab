package kit

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// accessPrefix finds the element whose name starts with prefix.
func accessPrefix(tt *ui.Tester, prefix string) (ui.AccessInfo, bool) {
	for _, a := range tt.AccessAll() {
		if strings.HasPrefix(a.Name, prefix) {
			return a, true
		}
	}
	return ui.AccessInfo{}, false
}

// TestStatBlock: the value in its format, the change signed in green or
// red, the caption, as a group named by its label.
func TestStatBlock(t *testing.T) {
	spec := StatSpec{
		Label:        "Bitcoin",
		Value:        64250.5,
		Format:       FormatCurrency,
		Options:      FormatOptions{Currency: "USD"},
		Change:       0.025,
		ChangeFormat: FormatPercent,
		Caption:      "Against yesterday",
	}
	view := func(c *Context) {
		Column(c).Width(400).Padding(16).Children(func() { StatBlock(c, spec) })
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		spec.Change = 0.025
		tt.Frame()
		shows(t, tt, "$64,250.50", "+2.5%", "Against yesterday")
		if !HasRole(tt, "group", "Bitcoin") {
			t.Error("no group named Bitcoin")
		}
		k := Light
		if darkOf(tt) {
			k = Dark
		}
		up, ok := tt.Find("+2.5%")
		if !ok {
			t.Fatal("no change")
		}
		if !hasColor(tt.Image(), up, k.Tone(Green).Text) {
			t.Error("a rise is not green")
		}
		if _, ok := tt.AccessByRole(ui.RoleGroup, "Up +2.5%"); !ok {
			if _, ok := accessPrefix(tt, "Up "); !ok {
				t.Error("the rise is not read out")
			}
		}
		// A fall is red, no change muted.
		spec.Change = -0.1
		tt.Frame()
		down, ok := accessPrefix(tt, "Down ")
		if !ok {
			t.Fatal("the fall is not read out")
		}
		if !hasColor(tt.Image(), down.Bounds, k.Tone(Red).Text) {
			t.Error("a fall is not red")
		}
		spec.Change = 0
		tt.Frame()
		if _, ok := accessPrefix(tt, "No change "); !ok {
			t.Error("no change is not read out")
		}
		// NULL is a dash; a value with no change has no change line.
		spec.Value, spec.Change = nil, nil
		tt.Frame()
		shows(t, tt, "–")
		if _, ok := accessPrefix(tt, "No change"); ok {
			t.Error("a stat with no change shows one")
		}
		spec.Value = 64250.5
	})
}

// TestStatBlockPersian: a Persian stat lines up from the right, its
// number left to right in it.
func TestStatBlockPersian(t *testing.T) {
	spec := StatSpec{
		Label:        "قیمت طلا",
		Value:        1234,
		Format:       FormatNumber,
		Change:       -12,
		ChangeFormat: FormatNumber,
		Caption:      persian,
	}
	view := func(c *Context) {
		Column(c).Width(400).Padding(16).AlignItems(Stretch).Children(func() { StatBlock(c, spec) })
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		shows(t, tt, "1,234", persian)
		v, _ := tt.Find("1,234")
		cap, _ := tt.Find(persian)
		// Both end at the block's right edge.
		if v.X+v.W < cap.X+cap.W-1 || v.X < 200 {
			t.Errorf("the value is not at the right: %v, caption %v", v, cap)
		}
		if !HasRole(tt, "group", "قیمت طلا") {
			t.Error("no group named in Persian")
		}
	})
}

func TestStatBlockStates(t *testing.T) {
	spec := StatSpec{Label: "Bitcoin", Value: 1, Format: FormatNumber}
	view := func(c *Context) {
		Column(c).Width(400).Padding(16).AlignItems(Stretch).Children(func() { StatBlock(c, spec) })
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		spec.State = BlockState{Loading: true}
		tt.Frame()
		shows(t, tt, "Loading Bitcoin")
		spec.State = BlockState{Error: "The query returned 3 rows; a stat needs one."}
		tt.Frame()
		shows(t, tt, "Can't show Bitcoin", "The query returned 3 rows; a stat needs one.")
		spec.State = BlockState{Notice: persian}
		tt.Frame()
		shows(t, tt, persian, "1")
		if !HasRole(tt, "status", persian) {
			t.Error("the notice is not read out")
		}
		spec.State = BlockState{}
	})
}
