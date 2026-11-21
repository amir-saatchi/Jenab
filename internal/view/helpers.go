package view

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/sqlguard"
)

// paramsOf returns the named parameters of q, without their prefix, each
// once.
func paramsOf(q string) ([]string, error) {
	ps, err := sqlguard.Params(q)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, p := range ps {
		if strings.HasPrefix(p.Name, ":") {
			out = append(out, p.Name[1:])
		}
	}
	return out, nil
}

// namedParams returns the :name parameters of q without the colon, and
// reports at rel each other kind: a config binds only :name, and SQLite
// takes @from and $from for parameters other than :from. qmark is the
// message for a ? or ?NNN. ok is false when q has no parameters to read,
// which the SQL check reports.
func namedParams(c *Check, rel, q, qmark string) (names []string, ok bool) {
	ps, err := sqlguard.Params(q)
	if err != nil {
		return nil, false
	}
	for _, p := range ps {
		switch {
		case p.Name == "":
			c.Errorf(StepSQL, rel, "%s", qmark)
		case p.Name[0] != ':':
			c.Errorf(StepSQL, rel, "write :%s, not %s: only :name parameters are bound", p.Name[1:], p.Name)
		default:
			names = append(names, p.Name[1:])
		}
	}
	return names, true
}

// RelativeDate reads a date filter's default (SPEC 5.2): an ISO date, or
// today, today-7d, today-3m, today-1y, start_of_month or start_of_year,
// counted back from today. Months and years run on as SQLite's do. It is
// the kit's RelativeDate, kept here without the UI.
func RelativeDate(expr string, today time.Time) (time.Time, bool) {
	y, m, d := today.Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, today.Location())
	s := strings.TrimSpace(expr)
	switch s {
	case "today":
		return day, true
	case "start_of_month":
		return time.Date(y, m, 1, 0, 0, 0, 0, today.Location()), true
	case "start_of_year":
		return time.Date(y, 1, 1, 0, 0, 0, 0, today.Location()), true
	}
	if n, ok := strings.CutPrefix(s, "today-"); ok && len(n) >= 2 {
		count, err := strconv.Atoi(n[:len(n)-1])
		if err != nil || count < 0 || n[0] == '+' || n[0] == '-' {
			return time.Time{}, false
		}
		switch n[len(n)-1] {
		case 'd':
			return day.AddDate(0, 0, -count), true
		case 'm':
			return day.AddDate(0, -count, 0), true
		case 'y':
			return day.AddDate(-count, 0, 0), true
		}
		return time.Time{}, false
	}
	t, err := time.ParseInLocation(time.DateOnly, s, today.Location())
	return t, err == nil
}

// DateDefault reports whether s is a date filter's default.
func DateDefault(s string) bool {
	_, ok := RelativeDate(s, time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))
	return ok
}

// suggest returns "; did you mean x?" for the name in have closest to
// name, when one is close; "" otherwise.
func suggest(name string, have []string) string {
	best, bestD := "", 0
	ln := strings.ToLower(name)
	for _, h := range have {
		if h == name {
			continue
		}
		if strings.EqualFold(h, name) {
			return fmt.Sprintf("; names are matched as written, so write it as %s", h)
		}
		d := distance(ln, strings.ToLower(h))
		if best == "" || d < bestD {
			best, bestD = h, d
		}
	}
	limit := max(1, len([]rune(name))/3)
	if best == "" || bestD > limit {
		best = ""
		for _, h := range have {
			lh := strings.ToLower(h)
			if h != name && len(ln) >= 3 && len(lh) >= 3 && (strings.HasPrefix(lh, ln) || strings.HasPrefix(ln, lh)) {
				best = h
				break
			}
		}
		if best == "" {
			return ""
		}
	}
	return fmt.Sprintf("; did you mean %s?", best)
}

// distance is the edit distance between a and b, by runes, a swap of two
// letters next to each other counting as one edit.
func distance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	pp := make([]int, len(rb)+1)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				cur[j] = min(cur[j], pp[j-2]+1) // a swap of two letters
			}
		}
		pp, prev, cur = prev, cur, pp
	}
	return prev[len(rb)]
}

// affinity is SQLite's type affinity of a declared column type (3.1 of
// SQLite's datatype page); "" is a column with no type, or STRICT's ANY.
func affinity(decl string) string {
	t := strings.ToUpper(decl)
	switch {
	case t == "" || t == "ANY":
		return ""
	case strings.Contains(t, "INT"):
		return "INTEGER"
	case strings.Contains(t, "CHAR"), strings.Contains(t, "CLOB"), strings.Contains(t, "TEXT"):
		return "TEXT"
	case strings.Contains(t, "BLOB"):
		return ""
	case strings.Contains(t, "REAL"), strings.Contains(t, "FLOA"), strings.Contains(t, "DOUB"):
		return "REAL"
	}
	return "NUMERIC"
}

// valueFits checks a literal for a column: "" when it fits, else why not.
// now() and today() are the time as text.
func valueFits(v any, col string, decl string, notNull bool) string {
	aff := affinity(decl)
	switch v := v.(type) {
	case nil:
		if notNull {
			return fmt.Sprintf("can't be null: %s is NOT NULL", col)
		}
	case string:
		if v == "now()" || v == "today()" {
			if aff != "TEXT" && aff != "" {
				return fmt.Sprintf("%s gives the time as text, but %s is %s", v, col, decl)
			}
			return ""
		}
		if aff == "INTEGER" || aff == "REAL" || aff == "NUMERIC" {
			if _, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err != nil {
				return fmt.Sprintf("is text, but %s is %s; give a number", col, decl)
			}
		}
	case bool:
		if aff == "TEXT" {
			return fmt.Sprintf("is true or false, but %s is %s; give text", col, decl)
		}
	case int64:
		if aff == "TEXT" {
			return fmt.Sprintf("is a number, but %s is %s; quote it", col, decl)
		}
	case float64:
		switch aff {
		case "TEXT":
			return fmt.Sprintf("is a number, but %s is %s; quote it", col, decl)
		case "INTEGER":
			if v != float64(int64(v)) {
				return fmt.Sprintf("has a fraction, but %s is %s", col, decl)
			}
		}
	}
	return ""
}
