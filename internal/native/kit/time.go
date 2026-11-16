package kit

import (
	"sync"
	"time"

	"github.com/egoist/mygo/i18n"
)

// Times and dates are written in the user's regional formats, as Windows
// is set (MyGo's Context.Region, from the user's time format), in the
// app's language, English: "01:11 PM" with a 12-hour clock, "13:11" with a
// 24-hour one. The React app wrote them with toLocaleTimeString and
// toLocaleDateString; these give the same strings. A frame follows a
// change of the settings.

// Region is the user's regional formats, which a Tester sets with
// SetRegion as the user does in Windows' settings.
type Region = i18n.Region

// Clock writes t's hour and minute with two digits each, as the React app
// shows a message's time, a card's answer and a rewind (toLocaleTimeString
// with hour and minute "2-digit"): "01:11 PM" or "13:11". t is written in
// its own zone.
func Clock(c *Context, t time.Time) string {
	return formatTime(c, t, i18n.DateOptions{Skeleton: "jjmm"})
}

// ShortTime writes t's time in the short style, as the React app's
// "Last active" and notes lines (timeStyle "short"): "1:11 PM" or "13:11".
func ShortTime(c *Context, t time.Time) string {
	return formatTime(c, t, i18n.DateOptions{Time: i18n.ShortStyle})
}

// MediumDate writes t's date in the medium style (dateStyle "medium"):
// "Nov 16, 2026", or "16 Nov 2026" with British formats.
func MediumDate(c *Context, t time.Time) string {
	return formatTime(c, t, i18n.DateOptions{Date: i18n.MediumStyle})
}

func formatTime(c *Context, t time.Time, o i18n.DateOptions) string {
	return i18n.FormatDate(formatLocale(c), t, o)
}

// formats is the locale formatLocale found last, for the language and
// regional formats it found it for: they change rarely, and every frame
// writes times.
var formats struct {
	sync.Mutex
	known  bool
	lang   string
	region Region
	locale string
}

// formatLocale is the locale the kit writes dates and times in: the app's
// language with the user's regional formats (Region.FormatLocale). Where
// the system says nothing of them, as for a Tester's zero Region, they
// are those CLDR gives the language, as MyGo's own controls take them.
func formatLocale(c *Context) string {
	lang, sys := c.Locale(), c.Region()
	if lang == "" {
		lang = "en"
	}
	formats.Lock()
	defer formats.Unlock()
	if formats.known && formats.lang == lang && formats.region == sys {
		return formats.locale
	}
	r := sys
	if r.HourCycle != 12 && r.HourCycle != 24 {
		r = i18n.RegionFor(lang)
		r.Locale = ""
	}
	formats.known, formats.lang, formats.region, formats.locale = true, lang, sys, r.FormatLocale(lang)
	return formats.locale
}
