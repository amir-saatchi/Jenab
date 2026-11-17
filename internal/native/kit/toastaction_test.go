package kit

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// The crash notice stays until it is closed or its button is clicked, as
// an Undo toast does not (TASK-067).
func TestStickyToastActionStays(t *testing.T) {
	opened := 0
	view := func(c *Context) {
		if Button(c, "Crash", Outline).Clicked() {
			StickyToastAction(c, "Jenab closed unexpectedly", "What happened is in crash.log.", "Open the log", ToastWarning, func() { opened++ })
		}
		if Button(c, "Delete", Outline).Clicked() {
			ToastAction(c, "Deleted a.md", "", "Undo", func() {})
		}
		Toasts(c)
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		opened = 0
		for _, b := range []string{"Crash", "Delete"} {
			if err := tt.Click(b); err != nil {
				t.Fatal(err)
			}
		}
		tt.Frame()
		tt.Advance(time.Minute)
		tt.Frame()
		if tt.HasText("Deleted a.md") {
			t.Error("the Undo toast stayed a minute")
		}
		shows(t, tt, "Jenab closed unexpectedly", "What happened is in crash.log.", "Open the log")
		if err := tt.Click("Open the log"); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
		tt.Frame()
		if opened != 1 || tt.HasText("Jenab closed unexpectedly") {
			t.Errorf("Open the log: %d, %q", opened, tt.Texts())
		}
	})
}
