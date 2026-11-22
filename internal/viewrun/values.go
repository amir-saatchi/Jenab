package viewrun

import (
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/amir-saatchi/jenab/internal/view"
)

// The values each format needs (SPEC 5.3). A query's values come as the
// driver gives them (int64, float64, string, []byte or nil, text as
// stored); value turns them into what the kit's FormatValue reads for the
// format, and leaves a value it can't read as it is, which the kit writes
// as it is.

// Formats SPEC 5.3 lists.
const (
	FormatText     = "text"
	FormatNumber   = "number"
	FormatCurrency = "currency"
	FormatPercent  = "percent"
	FormatDate     = "date"
	FormatDateTime = "datetime"
	FormatURL      = "url"
	FormatBoolean  = "boolean"
	FormatBadge    = "badge"
	FormatImage    = "image"
	FormatFile     = "file"
)

// FormatOptions are a format's options, as the kit's FormatOptions holds
// them (its Tones as kit.Tone).
type FormatOptions struct {
	Currency string            // ISO 4217, as USD
	Decimals *int              // nil for the format's own
	Tones    map[string]string // a badge's value → its tone
}

func formatOptions(o *view.FormatOptions) FormatOptions {
	if o == nil {
		return FormatOptions{}
	}
	return FormatOptions{Currency: o.Currency, Decimals: o.Decimals, Tones: o.Tones}
}

// value is v as format f shows it:
//   - number, currency, percent: an int64 or float64; numeric text too,
//     except a whole number past int64, which stays text, exact;
//   - date, datetime: a time.Time, by the kit's TimeOf rule: a date alone,
//     or a time at midnight in its own zone, is a day; a time without a
//     zone is UTC;
//   - boolean: a bool, from 1 and 0, true and false, yes and no;
//   - badge: its text, which its tone is looked up by;
//   - text, url, image, file: text as stored.
//
// Bytes that are UTF-8 are text. NULL stays nil.
func value(f string, v any) any {
	if b, ok := v.([]byte); ok && utf8.Valid(b) {
		v = string(b)
	}
	if v == nil {
		return nil
	}
	switch f {
	case FormatNumber, FormatCurrency, FormatPercent:
		return number(v)
	case FormatDate, FormatDateTime:
		if t, _, ok := timeOf(v); ok {
			return t
		}
	case FormatBoolean:
		if b, ok := boolOf(v); ok {
			return b
		}
	case FormatBadge:
		return text(v)
	}
	return v
}

// number reads v as a number: an int64 or float64 as it is, numeric text
// as one of them. Text that Go reads as NaN or an infinity ("NaN", "Inf")
// is no number, and comes back as it is, as anything else does.
func number(v any) any {
	switch x := v.(type) {
	case int64, float64:
		return x
	case int:
		return int64(x)
	case []byte:
		return number(string(x))
	case string:
		s := strings.TrimSpace(x)
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			return i
		} else if wholeText(s) {
			return x // past int64: the kit writes it exactly
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil && finiteFloat(f) {
			return f
		}
	}
	return v
}

// wholeText reports whether s is a whole number in digits.
func wholeText(s string) bool {
	s = strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+")
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// floatOf reads v as a float64: NaN for none, and for NaN and the
// infinities, which a chart can't place.
func floatOf(v any) float64 {
	switch x := number(v).(type) {
	case int64:
		return float64(x)
	case float64:
		if finiteFloat(x) {
			return x
		}
	}
	return math.NaN()
}

func finiteFloat(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }

// finite is v, or nil for a NaN or an infinity, as a float or as text
// ("NaN", "Inf"): a stat shows no such value.
func finite(v any) any {
	switch x := v.(type) {
	case float64:
		if !finiteFloat(x) {
			return nil
		}
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(x), 64); err == nil && !finiteFloat(f) {
			return nil
		}
	}
	return v
}

// boolOf reads v as SQLite stores a boolean: 1 or 0, true or false, yes
// or no.
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
	case int64:
		return x != 0, true
	case float64:
		return x != 0, true
	}
	return false, false
}

// text writes v as text: numbers in Go's shortest form.
func text(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case []byte:
		return string(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case int:
		return strconv.Itoa(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	case time.Time:
		return x.Format(time.RFC3339Nano)
	}
	return ""
}

// textOrNumber keeps numbers and makes the rest text, as a select's
// options are.
func textOrNumber(v any) any {
	switch x := v.(type) {
	case int64, float64:
		return x
	}
	return text(v)
}

// dateLayouts are the ways SQLite's date functions and Go write a time as
// text, as the kit reads them (kit.TimeOf); those with Z07 carry a zone.
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

// timeOf reads v as the kit's TimeOf does: a time.Time, or text as
// SQLite's date functions write it or RFC 3339. day reports a day on the
// calendar: a date alone, or a time at midnight in its own zone. A time
// without a zone is UTC, as datetime('now') writes it.
func timeOf(v any) (t time.Time, day, ok bool) {
	switch x := v.(type) {
	case time.Time:
		if x.IsZero() {
			return time.Time{}, false, false
		}
		h, m, s := x.Clock()
		return x, h == 0 && m == 0 && s == 0 && x.Nanosecond() == 0, true
	case []byte:
		return timeOf(string(x))
	case string:
		s := strings.TrimSpace(x)
		if t, err := time.Parse(time.DateOnly, s); err == nil {
			return t, true, true
		}
		for _, l := range dateLayouts {
			if t, err := time.Parse(l, s); err == nil {
				return timeOf(t)
			}
		}
	}
	return time.Time{}, false, false
}
