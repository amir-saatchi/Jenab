package kit

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/i18n"
)

// CellFormat is how a data block shows a value (SPEC 5.3): a table's
// cell, a stat's number.
type CellFormat string

const (
	FormatText     CellFormat = "text"
	FormatNumber   CellFormat = "number"
	FormatCurrency CellFormat = "currency"
	FormatPercent  CellFormat = "percent"
	FormatDate     CellFormat = "date"
	FormatDateTime CellFormat = "datetime"
	FormatURL      CellFormat = "url"
	FormatBoolean  CellFormat = "boolean"
	FormatBadge    CellFormat = "badge"
	FormatImage    CellFormat = "image"
	FormatFile     CellFormat = "file"
)

// FormatOptions are a format's options (SPEC 5.3's format_options).
type FormatOptions struct {
	// Currency is the ISO 4217 code of a currency value, as USD.
	Currency string
	// Decimals, when set, is how many digits a number, an amount or a
	// percent shows after the point; nil leaves the format's own.
	Decimals *int
	// Tones maps a badge's values to their tones; other values are
	// Neutral.
	Tones map[string]Tone
}

// Numeric reports whether f shows numbers, which line up at the end of
// their column in figures of one width, left to right.
func (f CellFormat) Numeric() bool {
	return f == FormatNumber || f == FormatCurrency || f == FormatPercent
}

// NoNumber is what a number that is none shows, as NaN or an infinity
// a query computed: a dash.
const NoNumber = "–"

// FormatValue writes v as format f shows it, in the user's regional
// formats: a number with its separators, an amount in its currency, a
// share as a percent (0.25 is 25%, 0.256 is 25.6%), a date in the medium
// style, a boolean as Yes or No. nil, SQL's NULL, and a zero time are "";
// NaN and the infinities are NoNumber. A value the format can't read, as
// a word in a number column, is written as it is. Dates and times are
// read as TimeOf reads them.
func FormatValue(c *Context, v any, f CellFormat, o FormatOptions) string {
	return formatValue(formatLocale(c), v, f, o, false)
}

// formatValue is FormatValue in locale; signed writes a plus before a
// number above zero, as a stat's change.
func formatValue(locale string, v any, f CellFormat, o FormatOptions, signed bool) string {
	if v == nil {
		return ""
	}
	if t, ok := v.(time.Time); ok && t.IsZero() {
		return ""
	}
	switch f {
	case FormatNumber, FormatCurrency, FormatPercent:
		n, ok := numberOf(v)
		if !ok {
			return plainText(v)
		}
		opts := i18n.NumberOptions{Percent: f == FormatPercent}
		if f == FormatCurrency {
			opts.Currency = i18n.CurrencyOf(o.Currency)
		}
		switch {
		case o.Decimals != nil:
			d := max(0, min(*o.Decimals, 20))
			opts.MinFractionDigits, opts.MaxFractionDigits = d, d
			opts.Integer = d == 0
		case f == FormatPercent:
			// A share is often below a percent: 0.4%, not 0%.
			opts.MaxFractionDigits = 1
		}
		if signed {
			opts.SignDisplay = i18n.SignExceptZero
		}
		return i18n.FormatNumber(locale, n, opts)
	case FormatDate, FormatDateTime:
		t, day, ok := timeOf(v)
		if !ok {
			return plainText(v)
		}
		o := i18n.DateOptions{Date: i18n.MediumStyle}
		if f == FormatDateTime && !day {
			o.Time = i18n.ShortStyle
		}
		return i18n.FormatDate(locale, t, o)
	case FormatBoolean:
		b, ok := boolOf(v)
		switch {
		case !ok:
			return plainText(v)
		case b:
			return "Yes"
		}
		return "No"
	}
	return plainText(v)
}

// plainText is v as text: a string as it is, a number in its fewest
// digits (NaN and the infinities as NoNumber), bytes as text, a zero
// time as "".
func plainText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case []byte:
		return string(x)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return NoNumber
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case float32:
		if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
			return NoNumber
		}
		return strconv.FormatFloat(float64(x), 'f', -1, 32)
	case bool:
		return strconv.FormatBool(x)
	case time.Time:
		if x.IsZero() {
			return ""
		}
		return x.Format(time.RFC3339)
	}
	return i18n.FormatNumber("en", v, i18n.NumberOptions{NoGrouping: true})
}

// numberOf reads v as a number i18n writes: integers and floats as they
// are, a string of a decimal number as its text, exact. NaN and the
// infinities are no number to show.
func numberOf(v any) (any, bool) {
	switch x := v.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return x, true
	case float64:
		return x, !math.IsNaN(x) && !math.IsInf(x, 0)
	case float32:
		return x, !math.IsNaN(float64(x)) && !math.IsInf(float64(x), 0)
	case []byte:
		return numberOf(string(x))
	case string:
		s := strings.TrimSpace(x)
		if f, err := strconv.ParseFloat(s, 64); err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, false
		}
		return s, true
	}
	return nil, false
}

// floatOf is v as a float, as for the sign of a change.
func floatOf(v any) (float64, bool) {
	n, ok := numberOf(v)
	if !ok {
		return 0, false
	}
	switch x := n.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case string:
		f, err := strconv.ParseFloat(x, 64)
		return f, err == nil
	}
	f, err := strconv.ParseFloat(plainText(n), 64)
	return f, err == nil
}

// boolOf reads v as SQLite stores a boolean: 1 or 0, or true and false.
func boolOf(v any) (b, ok bool) {
	switch x := v.(type) {
	case bool:
		return x, true
	case []byte:
		return boolOf(string(x))
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "1", "true", "yes":
			return true, true
		case "0", "false", "no":
			return false, true
		}
		return false, false
	}
	if f, ok := floatOf(v); ok {
		return f != 0, true
	}
	return false, false
}

// dateLayouts are the ways SQLite's date functions and Go write a date
// or a time as text; those with Z07 carry a zone.
var dateLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04Z07:00",
	"2006-01-02 15:04Z07:00",
	"2006-01-02T15:04",
	"2006-01-02 15:04",
}

// TimeOf reads v as a date or a time: a time.Time, or text as SQLite's
// date functions write it (2026-11-20, 2026-11-20 08:00:00) or RFC 3339.
//
//   - A day, with no time of day, is a day on the calendar, shown as it
//     is in every zone: text with a date alone, and a time.Time or text
//     whose time is midnight in its own zone, as a driver reads a DATE
//     column (so an instant at exactly midnight UTC shows as its day).
//   - Any other time is an instant, shown in the computer's zone: a
//     time.Time in any zone, text with a zone, and text without one,
//     which is read as UTC, as SQLite's datetime('now') and the app's
//     timestamps (SPEC 1, RFC 3339 in UTC) write it. A query that keeps
//     local times writes them with their zone, or as a day.
//
// A zero time.Time is no time.
func TimeOf(v any) (time.Time, bool) {
	t, _, ok := timeOf(v)
	return t, ok
}

// timeOf is TimeOf, and whether t is a day rather than an instant.
func timeOf(v any) (t time.Time, day, ok bool) {
	switch x := v.(type) {
	case time.Time:
		if x.IsZero() {
			return time.Time{}, false, false
		}
		h, m, s := x.Clock()
		if h == 0 && m == 0 && s == 0 && x.Nanosecond() == 0 {
			return x, true, true
		}
		return x.Local(), false, true
	case []byte:
		return timeOf(string(x))
	case string:
		s := strings.TrimSpace(x)
		if t, err := time.Parse(time.DateOnly, s); err == nil {
			return t, true, true
		}
		for _, l := range dateLayouts {
			if t, err := time.Parse(l, s); err == nil {
				// Without a zone, time.Parse reads it as UTC. Read as
				// a time.Time is, the same instant is the same day or
				// time however it came.
				return timeOf(t)
			}
		}
	}
	return time.Time{}, false, false
}
