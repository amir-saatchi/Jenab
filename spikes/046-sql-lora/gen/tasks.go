package main

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// task is one scenario: a request, its checks and a scripted reply that
// passes them.
type task struct {
	id, title, text, why string
	short                bool
	asserts              []assertYAML
	script               []reply
	skip                 bool  // a variant whose answer isn't clear-cut, such as a tie
	err                  error // a template that doesn't fit its fixture
}

// Kinds of request, as the ticket lists them.
const (
	kTotal    = "total"    // totals, averages and counts
	kJoin     = "join"     // joins
	kDates    = "dates"    // date ranges
	kFilter   = "filter"   // filters and sorting
	kTop      = "top"      // top-N
	kInsert   = "insert"   // adding rows
	kRefused  = "refused"  // an internal table the guard refuses
	kNoAnswer = "noanswer" // a question the data can't answer
)

// Q is a question with a reference query: the cells of its result are
// the answer.
type Q struct {
	ID     string
	Kind   string
	EN, FA string   // FA "" for English only, such as an answer that is a date or a category name
	SQL    string   // the reference query
	Tables []string // tables a right query reads
	Cols   []int    // the columns the answer must have; nil is all
	Dates  []int    // columns that hold dates, matched in the ways English writes them
	// Top: the SQL gives rows ordered by their last column, largest first;
	// the task asks for the first N, the largest of these with no tie at
	// the N-th row. EN and FA have %d for N.
	Top   []int
	Short bool // in the -short set: the card has table names only
	Why   string
}

func (f *fixture) question(q Q) []*task {
	rows, err := f.rows(q.SQL)
	if err != nil {
		return []*task{{id: f.id(q.ID, "en"), err: fmt.Errorf("%s: %w", q.SQL, err)}}
	}
	if len(rows) == 0 {
		return []*task{{id: f.id(q.ID, "en"), err: fmt.Errorf("%s: no rows", q.SQL)}}
	}
	en, fa := q.EN, q.FA
	if q.Top != nil {
		n := topN(rows, q.Top)
		if n == 0 {
			return []*task{{id: f.id(q.ID, "en"), skip: true}}
		}
		rows = rows[:n]
		en = fmt.Sprintf(en, n)
		if fa != "" {
			fa = fmt.Sprintf(fa, n)
		}
	}
	cols := q.Cols
	if cols == nil {
		for i := range rows[0] {
			cols = append(cols, i)
		}
	}
	// No answer, or none: a chat may say "none" for 0, which a check on
	// the number can't take.
	if v := rows[0][cols[0]]; v == nil || v == int64(0) || v == float64(0) {
		return []*task{{id: f.id(q.ID, "en"), skip: true}}
	}
	var out []*task
	for _, lang := range []string{"en", "fa"} {
		text := en
		if lang == "fa" {
			if text = fa; text == "" {
				continue
			}
		}
		t := &task{id: f.id(q.ID, lang), title: en, text: text, short: q.Short}
		t.why = fmt.Sprintf("%s: read %s with query; the answer has %s.", q.Kind, strings.Join(q.Tables, " and "), show(rows, cols))
		if q.Why != "" {
			t.why = q.Why + " " + t.why
		}
		for _, tb := range q.Tables {
			t.add(assertYAML{Type: "tool_called", Test: "query", Tool: "query", ArgsMatch: `(?i)\b` + regexp.QuoteMeta(tb) + `\b`})
		}
		if q.Short {
			t.add(assertYAML{Type: "tool_called", Test: "describe", Tool: "describe_table", ArgsMatch: `(?i)\b` + regexp.QuoteMeta(q.Tables[0]) + `\b`})
		}
		t.add(assertYAML{Type: "tool_called", Test: "result", Tool: "query", ResultMatch: resultRe(rows[0][cols[0]])})
		seen := map[string]bool{}
		for _, r := range rows {
			for _, c := range cols {
				re := answerRe(r[c], lang == "en" && contains(q.Dates, c))
				if re != "" && !seen[re] {
					seen[re] = true
					t.add(assertYAML{Type: "answer_contains", Test: "answer", Match: re, Final: true})
				}
			}
		}
		t.end(lang, "query", 4)
		if q.Short {
			for _, tb := range q.Tables {
				t.script = append(t.script, reply{Calls: []call{{Name: "describe_table", Args: map[string]any{"name": tb}}}})
			}
		}
		t.script = append(t.script, reply{Calls: []call{{Name: "query", Args: map[string]any{"sql": q.SQL}}}})
		t.script = append(t.script, reply{Text: says(lang, show(rows, cols))})
		out = append(out, t)
	}
	return out
}

// I adds a row with insert. Lookups are queries a right chat runs first,
// such as an ID from a code; Row has what they found.
type I struct {
	ID      string
	EN, FA  string
	Table   string
	Row     map[string]any // the row the scripted call sends; args must have each value
	Lookups []string       // reference queries before the insert
	Check   string         // db_query: SQL whose rows show the new row
	Short   bool
	Why     string
}

func (f *fixture) insert(in I) []*task {
	// The row as the fixture stores it, from a copy, so later tasks see the
	// fixture as it was.
	tx, err := f.db.Begin()
	if err != nil {
		return []*task{{id: f.id(in.ID, "en"), err: err}}
	}
	defer tx.Rollback()
	var cols, marks []string
	var vals []any
	for _, c := range sortedKeys(in.Row) {
		cols, marks, vals = append(cols, c), append(marks, "?"), append(vals, in.Row[c])
	}
	if _, err := tx.Exec(fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", in.Table, strings.Join(cols, ", "), strings.Join(marks, ", ")), vals...); err != nil {
		return []*task{{id: f.id(in.ID, "en"), err: fmt.Errorf("insert: %w", err)}}
	}
	rs, err := tx.Query(in.Check)
	if err != nil {
		return []*task{{id: f.id(in.ID, "en"), err: fmt.Errorf("%s: %w", in.Check, err)}}
	}
	lines, err := scan(rs)
	if err != nil || len(lines) != 1 {
		return []*task{{id: f.id(in.ID, "en"), err: fmt.Errorf("%s: want one row, got %d (%v)", in.Check, len(lines), err)}}
	}
	cells := make([]string, len(lines[0]))
	for i, v := range lines[0] {
		cells[i] = fmt.Sprint(v)
	}
	line := strings.Join(cells, "|")
	var out []*task
	for _, lang := range []string{"en", "fa"} {
		text := in.EN
		if lang == "fa" {
			if text = in.FA; text == "" {
				continue
			}
		}
		t := &task{id: f.id(in.ID, lang), title: in.EN, text: text, short: in.Short}
		t.why = fmt.Sprintf("insert: add the row to %s with insert; at the end the check reads %s.", in.Table, line)
		if in.Why != "" {
			t.why = in.Why + " " + t.why
		}
		if in.Short {
			t.add(assertYAML{Type: "tool_called", Test: "describe", Tool: "describe_table", ArgsMatch: `(?i)\b` + regexp.QuoteMeta(in.Table) + `\b`})
		}
		t.add(assertYAML{Type: "tool_called", Test: "insert", Tool: "insert", ArgsMatch: `"table":\s*"` + regexp.QuoteMeta(in.Table) + `"`})
		for _, c := range sortedKeys(in.Row) {
			t.add(assertYAML{Type: "tool_called", Test: "insert", Tool: "insert", ArgsMatch: argRe(in.Row[c])})
		}
		t.add(assertYAML{Type: "db_query", Test: "row", Query: in.Check, Match: `(?m)^` + regexp.QuoteMeta(line) + `$`})
		t.add(assertYAML{Type: "answer_lacks", Test: "says", Match: refusal(lang), Final: true})
		t.add(assertYAML{Type: "max_calls", Test: "info", Tool: "insert", N: 1})
		t.end(lang, "", 0)
		if in.Short {
			t.script = append(t.script, reply{Calls: []call{{Name: "describe_table", Args: map[string]any{"name": in.Table}}}})
		}
		for _, l := range in.Lookups {
			t.script = append(t.script, reply{Calls: []call{{Name: "query", Args: map[string]any{"sql": l}}}})
		}
		t.script = append(t.script, reply{Calls: []call{{Name: "insert", Args: map[string]any{"table": in.Table, "rows": []map[string]any{in.Row}}}}})
		done := "Done: I added the row to " + in.Table + "."
		if lang == "fa" {
			done = "انجام شد: ردیف به " + in.Table + " اضافه شد."
		}
		t.script = append(t.script, reply{Text: done})
		out = append(out, t)
	}
	return out
}

// R asks to run a query on an internal table, which the guard refuses;
// the chat gets the answer another way: describe_table, or a query on the
// project's own tables.
type R struct {
	ID       string
	EN, FA   string
	Internal string // the query the user asks for
	Table    string // the table the right way reads
	FixSQL   string // "" is describe_table; else the reference query whose first cell is the answer
	Answer   string // with describe_table: a literal the answer must have
}

func (f *fixture) refused(r R) []*task {
	var answer any = r.Answer
	if r.FixSQL != "" {
		rows, err := f.rows(r.FixSQL)
		if err != nil || len(rows) != 1 {
			return []*task{{id: f.id(r.ID, "en"), err: fmt.Errorf("%s: want one row (%v)", r.FixSQL, err)}}
		}
		answer = rows[0][0]
	}
	var out []*task
	for _, lang := range []string{"en", "fa"} {
		text := r.EN
		if lang == "fa" {
			if text = r.FA; text == "" {
				continue
			}
		}
		t := &task{id: f.id(r.ID, lang), title: "Run a query on an internal table, then find the answer another way", text: text}
		fix := "describe_table"
		if r.FixSQL != "" {
			fix = "query"
		}
		t.why = fmt.Sprintf("refused: run the query as asked; query refuses the internal table; the chat turns to %s on %s and answers %v.", fix, r.Table, answer)
		t.add(assertYAML{Type: "tool_called", Test: "refused", Tool: "query", ArgsMatch: `_jenab_`, ResultMatch: `(?i)refused`})
		t.add(assertYAML{Type: "tool_called", Test: "fixed", Tool: fix, ArgsMatch: `(?i)\b` + regexp.QuoteMeta(r.Table) + `\b`})
		t.add(assertYAML{Type: "answer_contains", Test: "answer", Match: answerRe(answer, false), Final: true})
		t.end(lang, "query", 4)
		t.script = append(t.script, reply{Calls: []call{{Name: "query", Args: map[string]any{"sql": r.Internal}}}})
		if r.FixSQL == "" {
			t.script = append(t.script, reply{Calls: []call{{Name: "describe_table", Args: map[string]any{"name": r.Table}}}})
		} else {
			t.script = append(t.script, reply{Calls: []call{{Name: "query", Args: map[string]any{"sql": r.FixSQL}}}})
		}
		t.script = append(t.script, reply{Text: says(lang, fmt.Sprint(answer))})
		out = append(out, t)
	}
	return out
}

// U is a question the data can't answer: no column or row has it. The
// right answer says so, without making a value up.
type U struct {
	ID     string
	EN, FA string
	Table  string // the table a chat would look at
	Lacks  string // a regex for a made-up answer, such as a phone number; "" for none
	Short  bool
	Why    string
}

func (f *fixture) noAnswer(u U) []*task {
	var out []*task
	for _, lang := range []string{"en", "fa"} {
		text := u.EN
		if lang == "fa" {
			if text = u.FA; text == "" {
				continue
			}
		}
		t := &task{id: f.id(u.ID, lang), title: u.EN, text: text, short: u.Short}
		t.why = "noanswer: " + u.Why + " The answer says the data doesn't have it."
		t.add(assertYAML{Type: "answer_contains", Test: "says", Match: cannot(lang), Final: true})
		if u.Lacks != "" {
			t.add(assertYAML{Type: "answer_lacks", Test: "says", Match: u.Lacks, Final: true})
		}
		t.add(assertYAML{Type: "tool_called", Test: "info", Tools: []string{"describe_table", "query"}})
		t.end(lang, "query", 4)
		t.script = append(t.script, reply{Calls: []call{{Name: "describe_table", Args: map[string]any{"name": u.Table}}}})
		no := "The " + u.Table + " table has no column for that, so the data can't answer it."
		if lang == "fa" {
			no = "جدول " + u.Table + " ستونی برای این ندارد، پس داده‌ها پاسخی برای آن ندارند."
		}
		t.script = append(t.script, reply{Text: no})
		out = append(out, t)
	}
	return out
}

func (t *task) add(a assertYAML) { t.asserts = append(t.asserts, a) }

// end adds the checks every task has: a Persian answer to a Persian
// request, and the info checks.
func (t *task) end(lang, tool string, n int) {
	if lang == "fa" {
		t.add(assertYAML{Type: "answer_contains", Test: "persian", Match: `\p{Arabic}`, Final: true})
	}
	if tool != "" {
		t.add(assertYAML{Type: "max_calls", Test: "info", Tool: tool, N: n})
	}
	t.add(assertYAML{Type: "no_turn_failed", Test: "info"})
}

func (f *fixture) id(base, lang string) string { return f.name + "_" + base + "_" + lang }

// rows runs a reference query on the fixture. A DATE cell comes back
// as a time; the query tool prints it as the date stored, so it is that
// here too.
func (f *fixture) rows(q string) ([][]any, error) {
	rs, err := f.db.Query(q)
	if err != nil {
		return nil, err
	}
	rows, err := scan(rs)
	for _, r := range rows {
		for i, v := range r {
			if t, ok := v.(time.Time); ok {
				if t.Hour()+t.Minute()+t.Second() == 0 {
					r[i] = t.Format(time.DateOnly)
				} else {
					r[i] = t.Format(time.DateTime)
				}
			}
		}
	}
	return rows, err
}

type rowScanner interface {
	Columns() ([]string, error)
	Next() bool
	Scan(...any) error
	Err() error
	Close() error
}

func scan(rs rowScanner) ([][]any, error) {
	defer rs.Close()
	cols, err := rs.Columns()
	if err != nil {
		return nil, err
	}
	var out [][]any
	for rs.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rs.Scan(ptrs...); err != nil {
			return nil, err
		}
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				vals[i] = string(b)
			}
		}
		out = append(out, vals)
	}
	return out, rs.Err()
}

// topN is the largest n in ns with no tie between row n and row n+1 on
// the last column, or 0.
func topN(rows [][]any, ns []int) int {
	for _, n := range ns {
		if n > len(rows) {
			continue
		}
		last := len(rows[0]) - 1
		if n == len(rows) || fmt.Sprint(rows[n-1][last]) != fmt.Sprint(rows[n][last]) {
			return n
		}
	}
	return 0
}

// show is the answer's cells as text, rows split by "; ".
func show(rows [][]any, cols []int) string {
	var rs []string
	for _, r := range rows {
		var cs []string
		for _, c := range cols {
			cs = append(cs, cell(r[c]))
		}
		rs = append(rs, strings.Join(cs, ", "))
	}
	return strings.Join(rs, "; ")
}

// cell is a value as the query tool prints it.
func cell(v any) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return fmt.Sprint(v)
}

func says(lang, answer string) string {
	if lang == "fa" {
		return "بر پایهٔ داده‌های پروژه: " + answer
	}
	return "From the project's data: " + answer + "."
}

// Regexes.

// Digits as the answers may write them: Latin, Persian and Arabic-Indic.
var digitClass = func() [10]string {
	var out [10]string
	for d := range 10 {
		out[d] = fmt.Sprintf("[%d%c%c]", d, rune(0x06F0+d), rune(0x0660+d))
	}
	return out
}()

const (
	notDigit  = `[^0-9\x{06F0}-\x{06F9}\x{0660}-\x{0669}]`
	groupSep  = `[,\x{066C}\x{00A0}\x{202F} ]?` // a thousands separator, or none
	decimalPt = `[.\x{066B}/]`                  // Persian writes the decimal point as ٫ or /
)

// answerRe matches v in an answer: a number in the ways it is written
// (separators, Persian digits, rounded to cents or to a whole number), a
// date as English writes it, or text as it is.
func answerRe(v any, date bool) string {
	switch x := v.(type) {
	case nil:
		return ""
	case int64:
		return `(?:^|` + notDigit + `)` + numForm(strconv.FormatInt(x, 10)) + `(?:` + decimalPt + `0+)?(?:` + notDigit + `|$)`
	case float64:
		forms := []string{strconv.FormatFloat(x, 'f', -1, 64), strconv.FormatFloat(x, 'f', 2, 64)}
		if math.Abs(x) >= 100 {
			forms = append(forms, strconv.FormatFloat(math.Round(x), 'f', 0, 64))
		}
		var alts []string
		seen := map[string]bool{}
		for _, s := range forms {
			if p := numForm(s); !seen[p] {
				seen[p] = true
				alts = append(alts, p)
			}
		}
		return `(?:^|` + notDigit + `)(?:` + strings.Join(alts, "|") + `)(?:` + notDigit + `|$)`
	case string:
		if date {
			if re := dateRe(x); re != "" {
				return re
			}
		}
		return `(?i)` + regexp.QuoteMeta(x)
	}
	return `(?i)` + regexp.QuoteMeta(fmt.Sprint(v))
}

// numForm is a number's pattern: its whole part with or without thousands
// separators, and its decimals with or without trailing zeros.
func numForm(s string) string {
	whole, frac, _ := strings.Cut(s, ".")
	neg := strings.HasPrefix(whole, "-")
	whole = strings.TrimPrefix(whole, "-")
	var b strings.Builder
	if neg {
		b.WriteString(`[-\x{2212}]`)
	}
	for i, r := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteString(groupSep)
		}
		b.WriteString(digitClass[r-'0'])
	}
	frac = strings.TrimRight(frac, "0")
	if frac == "" {
		b.WriteString(`(?:` + decimalPt + `0+)?`)
		return b.String()
	}
	b.WriteString(decimalPt)
	for _, r := range frac {
		b.WriteString(digitClass[r-'0'])
	}
	b.WriteString(`0*`)
	return b.String()
}

var months = []string{"Jan(?:uary)?", "Feb(?:ruary)?", "Mar(?:ch)?", "Apr(?:il)?", "May", "June?", "July?", "Aug(?:ust)?", "Sep(?:t(?:ember)?)?", "Oct(?:ober)?", "Nov(?:ember)?", "Dec(?:ember)?"}

// dateRe matches a YYYY-MM-DD date as it is, or as "September 27" or
// "27 September".
func dateRe(s string) string {
	var y, m, d int
	if n, _ := fmt.Sscanf(s, "%4d-%2d-%2d", &y, &m, &d); n != 3 || m < 1 || m > 12 {
		return ""
	}
	mon := months[m-1]
	return fmt.Sprintf(`(?i)%s|\b%s\.?\s+0?%d(?:st|nd|rd|th)?\b|\b0?%d(?:st|nd|rd|th)?\s+(?:of\s+)?%s\b`, regexp.QuoteMeta(s), mon, d, d, mon)
}

// resultRe matches v as a cell of the query tool's result: rows of cells
// split by " | ". A number may be rounded or not.
func resultRe(v any) string {
	var p string
	switch x := v.(type) {
	case int64:
		p = strconv.FormatInt(x, 10) + `(?:\.0+)?`
	case float64:
		lo, hi := math.Floor(x), math.Round(x)
		p = strconv.FormatFloat(lo, 'f', 0, 64) + `(?:\.\d+)?`
		if hi != lo {
			p = `(?:` + p + `|` + strconv.FormatFloat(hi, 'f', 0, 64) + `(?:\.0+)?)`
		}
	default:
		p = regexp.QuoteMeta(fmt.Sprint(v))
	}
	return `(?mi)(?:^|\| )` + p + `(?: \||$)`
}

// argRe matches a value in the insert call's arguments as JSON.
func argRe(v any) string {
	switch x := v.(type) {
	case string:
		return `(?i)"` + regexp.QuoteMeta(x) + `"`
	case int, int64:
		return `[:\[,\s"]` + fmt.Sprint(x) + `(?:\.0+)?["\s,}\]]`
	case float64:
		s := strconv.FormatFloat(x, 'f', -1, 64)
		if !strings.Contains(s, ".") {
			return `[:\[,\s"]` + s + `(?:\.0+)?["\s,}\]]`
		}
		return `[:\[,\s"]` + regexp.QuoteMeta(s) + `0*["\s,}\]]`
	}
	return regexp.QuoteMeta(fmt.Sprint(v))
}

// refusal is text that says the chat can't do it.
func refusal(lang string) string {
	if lang == "fa" {
		return `نمی‌توانم|نمیتوانم|امکان‌پذیر نیست|ممکن نیست`
	}
	return `(?i)can't|cannot|unable|not (yet )?(possible|supported)`
}

// cannot is text that says the data doesn't have the answer.
func cannot(lang string) string {
	if lang == "fa" {
		return `ندار|نیست|وجود ندارد|نمی‌توان|نمیتوان|نمی‌شود|ثبت نشده|هیچ`
	}
	return `(?i)\bno\b|\bnot\b|n't\b|\bcannot\b|\bunable\b|\bnone\b|\bwithout\b`
}

func contains(xs []int, x int) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]any) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
