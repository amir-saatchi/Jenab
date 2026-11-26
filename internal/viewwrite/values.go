package viewwrite

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/amir-saatchi/jenab/internal/bucket"
	"github.com/amir-saatchi/jenab/internal/view"
)

// Value is a form field's value as the form block gives it, the kit's
// FormValue field for field (P3-08): Text for text, textarea and select (an
// option's text, as Option.Text), a number as typed and a file's bucket
// key; Number the number read from it; Int the same number exactly when
// it is whole and fits, as a float64 rounds those past 2^53; Date a date;
// Checked a check box.
type Value struct {
	Text    string     `json:"text,omitempty"`
	Number  *float64   `json:"number,omitempty"`
	Int     *int64     `json:"int,omitempty"`
	Date    *time.Time `json:"date,omitempty"`
	Checked bool       `json:"checked,omitempty"`
}

// Option is one choice of a select: Text as the form shows it, and the
// value it writes.
type Option struct {
	Text  string `json:"text"`
	Value any    `json:"value"` // string, int64, float64 or bool
}

// dateText is a date or a time as text, from a date field, now() or
// today(): it fits a column for dates as well as a text one.
type dateText string

// read reads the value v of field f as its control gives it: nil when it
// is empty. problem says why it can't be read.
func read(f view.FormField, v Value, opts []Option) (any, string) {
	var out any
	switch f.Control {
	case "checkbox":
		if f.Required && !v.Checked {
			return nil, "must be checked"
		}
		return v.Checked, "" // unchecked is false, never NULL (P3-08)
	case "number":
		switch {
		case v.Int != nil:
			out = *v.Int
		case v.Number != nil:
			if math.IsNaN(*v.Number) || math.IsInf(*v.Number, 0) {
				return nil, "is not a number"
			}
			out = *v.Number
		case strings.TrimSpace(v.Text) != "":
			n, ok := parseNumber(v.Text)
			if !ok {
				return nil, "is not a number"
			}
			out = n
		}
	case "date":
		switch {
		case v.Date != nil:
			// A date is a day (P3-08): the year, month and day the user
			// picked, in the time's own zone, never moved through UTC,
			// so a date at midnight in +03:30 stays that day.
			y, m, d := v.Date.Date()
			out = dateText(fmt.Sprintf("%04d-%02d-%02d", y, int(m), d))
		case strings.TrimSpace(v.Text) != "":
			d, err := time.Parse(time.DateOnly, strings.TrimSpace(v.Text))
			if err != nil {
				return nil, "is not a date; write it as 2026-11-20"
			}
			out = dateText(d.Format(time.DateOnly))
		}
	case "select":
		if v.Text == "" {
			break
		}
		found := false
		for _, o := range opts {
			if o.Text == v.Text {
				out, found = o.Value, true
				break
			}
		}
		if !found {
			return nil, "is not one of the choices"
		}
	case "file":
		key := strings.TrimSpace(v.Text)
		if key == "" {
			break
		}
		if bucket.CheckKey(key) != nil || !strings.HasPrefix(key, f.Prefix) {
			return nil, "is not a file uploaded to this field"
		}
		out = key
	default: // text, textarea
		if strings.TrimSpace(v.Text) == "" {
			break
		}
		if !utf8.ValidString(v.Text) {
			return nil, "is not valid text"
		}
		out = v.Text
	}
	if out == nil && f.Required {
		return nil, "needs a value"
	}
	return out, ""
}

// affinity is the type a column declared decl stores values as, by
// SQLite's rules in their order (3.1); "" for none, or ANY in a STRICT
// table. A STRICT table's BLOB is "BLOB": it takes only bytes.
func affinity(decl string, strict bool) string {
	d := strings.ToUpper(decl)
	switch {
	case strict && d == "ANY":
		return ""
	case strict && d == "BLOB":
		return "BLOB"
	case strings.Contains(d, "INT"):
		return "INTEGER"
	case strings.Contains(d, "CHAR"), strings.Contains(d, "CLOB"), strings.Contains(d, "TEXT"):
		return "TEXT"
	case d == "", strings.Contains(d, "BLOB"):
		return ""
	case strings.Contains(d, "REAL"), strings.Contains(d, "FLOA"), strings.Contains(d, "DOUB"):
		return "REAL"
	}
	return "NUMERIC"
}

// forDates reports a NUMERIC column declared for dates or times, as
// DATE, DATETIME or TIMESTAMP: SQLite keeps a date in it as text.
func forDates(decl string) bool {
	d := strings.ToUpper(decl)
	return strings.Contains(d, "DATE") || strings.Contains(d, "TIME")
}

// fit checks v, read from a field or a set literal, against a column
// declared decl, and returns it as it is written: a number in text for a
// number column made a number, a check box 1 or 0. problem says why it
// doesn't fit, as SPEC 5.5 has it checked in Go before the write. Values
// SQLite would store as another type than the column's, such as "abc" in
// an INTEGER column, don't fit.
func fit(v any, decl string, strict bool) (any, string) {
	aff := affinity(decl, strict)
	if v == nil {
		return nil, ""
	}
	if aff == "BLOB" {
		return nil, "is in a BLOB column, which a form can't write"
	}
	switch x := v.(type) {
	case dateText:
		switch {
		case aff == "TEXT", aff == "", aff == "NUMERIC" && forDates(decl):
			return string(x), ""
		}
		return nil, "is a date, but the column " + wants(aff)
	case string:
		switch aff {
		case "INTEGER":
			if n, ok := parseNumber(x); ok {
				if i, ok := whole(n); ok {
					return i, ""
				}
			}
			return nil, "must be a whole number"
		case "REAL":
			if n, ok := parseNumber(x); ok {
				return float(n), ""
			}
			return nil, "must be a number"
		case "NUMERIC":
			if n, ok := parseNumber(x); ok {
				return n, ""
			}
			if forDates(decl) {
				return x, ""
			}
			return nil, "must be a number"
		}
		return x, ""
	case int64:
		switch aff {
		case "TEXT":
			return strconv.FormatInt(x, 10), ""
		case "REAL":
			return float64(x), ""
		}
		return x, ""
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return nil, "is not a number"
		}
		switch aff {
		case "INTEGER":
			if i, ok := whole(x); ok {
				return i, ""
			}
			return nil, "must be a whole number"
		case "TEXT":
			return strconv.FormatFloat(x, 'f', -1, 64), ""
		}
		return x, ""
	case bool:
		if aff == "TEXT" {
			return nil, "is true or false, but the column " + wants(aff)
		}
		if x {
			return int64(1), ""
		}
		return int64(0), ""
	}
	return nil, "can't be stored"
}

// wants says what a column of affinity aff takes.
func wants(aff string) string {
	switch aff {
	case "INTEGER":
		return "takes whole numbers"
	case "REAL", "NUMERIC":
		return "takes numbers"
	case "TEXT":
		return "takes text"
	}
	return "can't take it"
}

// whole returns n as an int64 when it is a whole number that fits.
func whole(n any) (int64, bool) {
	switch x := n.(type) {
	case int64:
		return x, true
	case float64:
		if x == math.Trunc(x) && x >= -(1<<63) && x < 1<<63 {
			return int64(x), true
		}
	}
	return 0, false
}

func float(n any) float64 {
	switch x := n.(type) {
	case int64:
		return float64(x)
	case float64:
		return x
	}
	return math.NaN()
}

// parseNumber reads s as a number, as typed in a text field: Persian and
// Arabic digits and decimal separator, Persian thousands separators, a
// sign. A whole number is an int64, exactly; NaN and the infinities are
// no number.
func parseNumber(s string) (any, bool) {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r >= '۰' && r <= '۹':
			b.WriteRune('0' + r - '۰')
		case r >= '٠' && r <= '٩':
			b.WriteRune('0' + r - '٠')
		case r == '٫':
			b.WriteByte('.')
		case r == '٬':
		case r == '−':
			b.WriteByte('-')
		default:
			b.WriteRune(r)
		}
	}
	t := b.String()
	if t == "" || strings.ContainsAny(t, "xXpP_") { // no hex, no Go digit separators
		return nil, false
	}
	if i, err := strconv.ParseInt(t, 10, 64); err == nil {
		return i, true
	}
	f, err := strconv.ParseFloat(t, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return nil, false
	}
	return f, true
}

// text writes v as text, numbers in Go's shortest form, as viewrun gives
// a select's options and the kit shows them.
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
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	}
	return ""
}

// optionsOf makes a select's options from their values.
func optionsOf(vals []any) []Option {
	out := make([]Option, 0, len(vals))
	for _, v := range vals {
		out = append(out, Option{Text: text(v), Value: v})
	}
	return out
}

// valueOf is the value a field shows for v, a row's value as the driver
// gives it (an update form's load) or a field's default.
func valueOf(f view.FormField, v any, opts []Option, today time.Time) Value {
	if b, ok := v.([]byte); ok {
		v = string(b)
	}
	if v == nil {
		return Value{}
	}
	switch f.Control {
	case "checkbox":
		return Value{Checked: truthy(v)}
	case "number":
		var out Value
		n := v
		if s, ok := v.(string); ok {
			if p, ok := parseNumber(s); ok {
				n = p
			}
		}
		switch x := n.(type) {
		case int64:
			fl := float64(x)
			out = Value{Text: strconv.FormatInt(x, 10), Int: &x, Number: &fl}
		case float64:
			out = Value{Text: strconv.FormatFloat(x, 'f', -1, 64), Number: &x}
			if i, ok := whole(x); ok && math.Abs(x) <= 1<<53 {
				out.Int = &i
			}
		default:
			out.Text = text(v)
		}
		return out
	case "date":
		s, ok := v.(string)
		if !ok {
			return Value{}
		}
		if t, ok := view.RelativeDate(s, today); ok {
			d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
			return Value{Date: &d}
		}
		if len(s) >= len(time.DateOnly) {
			if t, err := time.Parse(time.DateOnly, s[:len(time.DateOnly)]); err == nil {
				return Value{Date: &t}
			}
		}
		return Value{Text: s}
	case "select":
		t := text(v)
		for _, o := range opts {
			if o.Text == t {
				return Value{Text: o.Text}
			}
		}
		return Value{Text: t}
	}
	return Value{Text: text(v)}
}

// truthy reads a stored value as a check box: 1, true, "1", "true", "yes".
func truthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case int64:
		return x != 0
	case float64:
		return x != 0
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "1", "true", "yes", "on":
			return true
		}
	}
	return false
}
