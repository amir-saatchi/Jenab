package kit

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// Times follow the user's regional formats, as the Wails app's
// toLocaleTimeString does, and a frame follows a change of them. The
// stamps are plain text, so the test has no keyboard step: a message's
// time is read with its message.
func TestTimesFollowTheRegion(t *testing.T) {
	at := time.Date(2026, 11, 16, 13, 11, 0, 0, time.UTC)
	early := time.Date(2026, 11, 16, 9, 5, 0, 0, time.UTC)
	view := func(c *Context) {
		Column(c).Label("Stamps").Children(func() {
			Text(c, persian+" · "+Clock(c, at))
			Text(c, "early "+Clock(c, early))
			Text(c, "Last active "+ShortTime(c, at))
			Text(c, "on "+MediumDate(c, at))
		})
	}
	cases := []struct {
		region                     Region
		clock, early, short, date string
	}{
		{Region{Locale: "en-US", HourCycle: 12}, "01:11 PM", "09:05 AM", "1:11 PM", "Nov 16, 2026"},
		{Region{Locale: "en-GB", HourCycle: 24}, "13:11", "09:05", "13:11", "16 Nov 2026"},
		// English formats with a 24-hour clock, as Windows' settings allow.
		{Region{Locale: "en-US", HourCycle: 24}, "13:11", "09:05", "13:11", "Nov 16, 2026"},
		// Persian formats: the app's language stays English, with its digits.
		{Region{Locale: "fa-IR", HourCycle: 24}, "13:11", "09:05", "13:11", "Nov 16, 2026"},
		// No formats from the system: English's own, 12 hours.
		{Region{}, "01:11 PM", "09:05 AM", "1:11 PM", "Nov 16, 2026"},
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		for _, cs := range cases {
			tt.SetRegion(cs.region)
			tt.Frame()
			shows(t, tt, persian+" · "+cs.clock, "early "+cs.early, "Last active "+cs.short, "on "+cs.date)
		}
		if _, ok := tt.Access("Stamps"); !ok {
			t.Error("the stamps have no label")
		}
	})
}
