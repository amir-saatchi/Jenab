package agent

// The number check (SPIKE-032, a prototype): numbers in a turn's answer,
// and in what write tools save, that no source gave (8.3). Sources are
// everything the model was given: tool results in the window, the user's
// messages, the system blocks with the date and the card (3.1), and
// earlier turns. The check is spike-only: the scenario runner turns it on
// through Deps.Grounding to compare its conditions; the app leaves it off.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// GroundingMode is a condition of the check: whether it runs, and who
// hears of it.
type GroundingMode string

const (
	GroundingOff      GroundingMode = ""          // no check: today's loop
	GroundingMark     GroundingMode = "mark"      // check and mark the answer; the model is never told
	GroundingSameTurn GroundingMode = "same_turn" // mark, then one more request with the notice
	GroundingNextTurn GroundingMode = "next_turn" // mark, and the notice comes before the next message
)

// GroundingModes are the modes, for flags and files.
var GroundingModes = []GroundingMode{GroundingOff, GroundingMark, GroundingSameTurn, GroundingNextTurn}

// Grounding sets the check (SPIKE-032). Spike-only: only the scenario
// runner sets it; the app's zero value has no check.
type Grounding struct {
	Mode GroundingMode
	// K: the check fires at K or more numbers without a source. Computed
	// values (an average, a conversion) have none either, so K lets a few
	// through. 0 is DefaultGroundingK.
	K int
	// TellK: in the same_turn and next_turn modes, the model is told only
	// at TellK or more numbers without a source; the mark still comes at
	// K. 0 is K. (Round 2: two thresholds.)
	TellK int
	// TellSaved: the model is also told when a number in what it saved
	// has no source.
	TellSaved bool
	// Compute: block 1 asks the model to compute numbers with query, so
	// computed numbers have a source. (Round 2: tool-computed numbers.)
	Compute bool
}

// DefaultGroundingK is K when none is set.
const DefaultGroundingK = 3

// Threshold is K, or DefaultGroundingK when none is set.
func (g Grounding) Threshold() int {
	if g.K <= 0 {
		return DefaultGroundingK
	}
	return g.K
}

// tellThreshold is TellK, or K when none is set.
func (g Grounding) tellThreshold() int {
	if g.TellK <= 0 {
		return g.Threshold()
	}
	return g.TellK
}

// computeLine is block 1's line with Grounding.Compute and a query tool.
// It is the same for every model (1).
const computeLine = "Compute numbers with query instead of in your head: averages, sums, changes, conversions and what-ifs, " +
	"from the tables (SELECT AVG(close) FROM ...) or from numbers you have (SELECT 0.35 * 55190.2). " +
	"Then every number you give comes from a tool."

// Notice kinds of the check. The mark is for the user only: shape drops
// it, so the model never reads it. The app check is the notice the model
// reads in the same_turn and next_turn modes.
const (
	NoticeUnsourced chat.NoticeKind = "unsourced"
	NoticeAppCheck  chat.NoticeKind = "app_check"
)

func isMark(p chat.Part) bool { return p.Notice != nil && p.Notice.Kind == NoticeUnsourced }

// GroundingCheck is one run of the check on an answer, kept in the turn's
// trace (8.4).
type GroundingCheck struct {
	Request   int      // the index in the turn's trace of the request whose answer was checked
	Checked   int      // numbers checked, each value once
	Unsourced []string // numbers with no source, as written (Persian and Arabic digits as ASCII)
	Saved     int      // of them, in what write tools saved
	SavedLeft int      // of them, in what was saved before the notice and is still there (after a notice)
	Quoted    int      // numbers in the answer left out because the check's notice named them
	Pct       int      // numbers sourced only by reading a percentage the other way (5.2% = 0.052)
	Fired     bool     // at least K had no source: the answer is marked
	Told      bool     // the notice went to the model
	After     bool     // the answer came after the notice, in the same turn
	Took      time.Duration
}

// writeEffects are the effects of tools whose arguments are saved, so
// their numbers are checked like the answer's. A tool that also has the
// Network effect saves what it fetched, not its arguments (fetch_page's
// URL), so it isn't one.
const writeEffects = tool.WritesDB | tool.Bucket | tool.Memory

func writes(e tool.Effects) bool { return e&writeEffects != 0 && e&tool.Network == 0 }

// saved is one write call's arguments, until the turn's check.
type saved struct {
	key    string   // its "key" argument: a later write of the same key replaces it
	texts  []string // its string and number arguments
	before bool     // it ran before the turn's notice
}

// taint is a number a write call saved without a source, and where it
// went: a key (bucket, memory) or a table. A tool result that reads that
// place back and has the number, written the same, doesn't source it.
type taint struct {
	n          number
	key, table string
}

// noted keeps what an answer said and what its write calls saved, until
// the turn's check. Calls in the answer to the last request don't run.
// The numbers a write call saves without a source are kept for the chat,
// so they are no source when a tool reads them back.
func (r *runner) noted(t *turn, req provider.Request, resp *response, last bool) {
	if r.o.d.Grounding.Mode == GroundingOff {
		return
	}
	var src *sourceSet
	for _, p := range resp.parts {
		switch {
		case p.Text != nil:
			t.said = append(t.said, p.Text.Text)
		case p.ToolCall != nil && !last:
			if tl, ok := r.o.d.Tools.Get(p.ToolCall.Name); ok && writes(tl.Spec().Effects) {
				a := argsOf(p.ToolCall.Args)
				s := saved{key: a.Key, texts: argStrings(p.ToolCall.Args)}
				t.saved = append(t.saved, s)
				if a.Key == "" && a.Table == "" {
					continue // nowhere a read can be tied to
				}
				if src == nil {
					src = r.sources(req, t.n)
				}
				w := &r.cs.win
				res := checkNumbers(nil, s.texts, nil, *src, nil)
				for _, n := range res.missing {
					tn := taint{n: n, key: a.Key, table: a.Table}
					if !slices.ContainsFunc(w.tainted, func(o taint) bool { return o.n.same(n) && o.key == tn.key && o.table == tn.table }) {
						w.tainted = append(w.tainted, tn)
					}
				}
			}
		}
	}
}

// sources are the numbers the model was given in req (sourceTexts). A
// result of a tool that reads the project leaves out the numbers the chat
// saved without a source, where it reads back the place they went.
func (r *runner) sources(req provider.Request, turn int) *sourceSet {
	w := &r.cs.win
	reads := func(name string) bool {
		tl, ok := r.o.d.Tools.Get(name)
		return ok && tl.Spec().Effects&tool.ReadsDB != 0
	}
	given, read := sourceTexts(req, turn, w.fired, reads)
	src := newSourceSet(given, nil)
	for _, rd := range read {
		src.add([]string{rd.text}, tainted(w.tainted, rd.args))
	}
	src.sort()
	return &src
}

// tainted are the numbers of ts that a read with args reads back: the
// same key or ref, the same table, or a query of the table that selects
// its columns as they are. A computed result (AVG(close), close * 1.02)
// is never tainted: it is the tool's number, not the saved one.
func tainted(ts []taint, args json.RawMessage) []number {
	if len(ts) == 0 {
		return nil
	}
	a := argsOf(args)
	var out []number
	for _, t := range ts {
		switch {
		case t.key != "" && (a.Key == t.key || a.Ref == t.key),
			t.table != "" && strings.EqualFold(a.Table, t.table),
			t.table != "" && a.SQL != "" && readsAsIs(a.SQL, t.table):
			out = append(out, t.n)
		}
	}
	return out
}

// reSelect is a query's select list and what follows it.
var reSelect = regexp.MustCompile(`(?is)^\s*select\s+(.*?)\s+from\s+(.*)$`)

// reComputed finds a computation in a select list: a function call
// (AVG(, ROUND(, LAG(), arithmetic or a concatenation. A lone * is not
// one.
var reComputed = regexp.MustCompile(`\w\s*\(|[-+/%|]|[\w)]\s*\*\s*[\w(]`)

// readsAsIs reports whether sql selects from table without computing:
// SELECT date, close FROM prices WHERE ...
func readsAsIs(sql, table string) bool {
	m := reSelect.FindStringSubmatch(sql)
	if m == nil || reComputed.MatchString(m[1]) {
		return false
	}
	return regexp.MustCompile(`(?i)^["\x60\[]?` + regexp.QuoteMeta(table) + `\b`).MatchString(strings.TrimSpace(m[2]))
}

// ground checks the turn's answer against req, the request it answers:
// it records the verdict, marks an answer that fails, and tells the model
// in the same_turn and next_turn modes, once a turn.
func (r *runner) ground(ctx context.Context, t *turn, req provider.Request) error {
	g := r.o.d.Grounding
	if g.Mode == GroundingOff {
		return nil
	}
	// What was saved since the last check, and after a notice what was
	// saved before it and is still there: the notice may ask to fix it.
	var fresh, left []string
	for i, s := range t.saved {
		if s.key != "" && slices.ContainsFunc(t.saved[i+1:], func(o saved) bool { return o.key == s.key }) {
			continue
		}
		switch {
		case i >= t.checkedSaved:
			fresh = append(fresh, s.texts...)
		case t.told && s.before:
			left = append(left, s.texts...)
		}
	}
	if len(t.said)+len(fresh) == 0 {
		return nil
	}
	start := time.Now()
	w := &r.cs.win
	said := t.said
	t.said, t.checkedSaved = nil, len(t.saved)
	// The numbers the notice named may be quoted in the answer right after
	// it, in the same turn, as in "I made up 2,561.30": written the same,
	// and not in what is saved.
	quoted := t.flagged
	t.flagged = nil
	res := checkNumbers(said, fresh, left, *r.sources(req, t.n), quoted)
	v := GroundingCheck{Request: t.requests - 1, Checked: res.checked, Saved: res.saved, SavedLeft: res.left, Quoted: res.quoted, Pct: res.pct,
		After: t.told, Took: time.Since(start)}
	for _, n := range res.missing {
		v.Unsourced = append(v.Unsourced, n.text)
	}
	tell := !t.told && (g.Mode == GroundingSameTurn || g.Mode == GroundingNextTurn) &&
		(len(res.missing) >= g.tellThreshold() || g.TellSaved && res.saved > 0)
	v.Fired = len(res.missing) >= g.Threshold() || tell
	defer func() { r.o.d.Traces.grounding(t.trace, v) }()
	if !v.Fired {
		return nil
	}
	if w.fired == nil {
		w.fired = map[int]bool{}
	}
	w.fired[t.n] = true
	if err := r.notice(ctx, t, NoticeUnsourced, markText(v.Unsourced)); err != nil {
		return err
	}
	if !tell {
		return nil
	}
	if err := r.notice(ctx, t, NoticeAppCheck, checkText(v.Unsourced, v.Saved > 0)); err != nil {
		return err
	}
	t.told, v.Told = true, true
	t.flagged = res.missing[:min(noticeExamples, len(res.missing))]
	for i := range t.saved {
		t.saved[i].before = true
	}
	t.again = g.Mode == GroundingSameTurn
	return nil
}

// notice writes a notice as a user message of the turn.
func (r *runner) notice(ctx context.Context, t *turn, kind chat.NoticeKind, text string) error {
	n := chat.Notice{Kind: kind, Text: text}
	return r.writeMessage(ctx, chat.Message{Chat: t.ch.ID, Turn: t.n, Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartNotice, Notice: &n}}})
}

// checkText is the notice the model reads. It is the same for every
// model (1).
func checkText(missing []string, saved bool) string {
	where, fix := "your answer", ""
	if saved {
		where, fix = "your answer or in what you saved", ", and correct what you saved"
	}
	return fmt.Sprintf("[app check: %d numbers in %s appear in no tool result or message, e.g. %s. If no tool gave them, say so or get them with a tool%s.]",
		len(missing), where, examples(missing, noticeExamples), fix)
}

// noticeExamples is how many numbers the notice names.
const noticeExamples = 3

// markText is the mark the user sees on the answer (2.3).
func markText(missing []string) string {
	s := examples(missing, 3)
	if n := len(missing) - 3; n > 0 {
		s += fmt.Sprintf(" and %d more", n)
	}
	return "[some numbers have no source: " + s + "]"
}

func examples(ss []string, n int) string {
	return strings.Join(ss[:min(n, len(ss))], ", ")
}

// sourceTexts are the texts the model was given in req: the system
// blocks, the tool descriptions, the user's messages and notices, the
// tool results, and the answers of earlier turns, except turns whose check
// fired, so a made-up number doesn't become its own source. The check's
// own notice is left out: it lists the numbers. The results of tools that
// read the project, where the model may have saved numbers, come apart,
// in read, with their calls' arguments.
func sourceTexts(req provider.Request, turn int, fired map[int]bool, reads func(string) bool) (given []string, read []readText) {
	for _, b := range req.System {
		given = append(given, b.Text)
	}
	for _, td := range req.Tools {
		given = append(given, td.Description)
	}
	calls := map[string]*chat.ToolCall{}
	for _, m := range req.Messages {
		for _, p := range m.Parts {
			switch {
			case p.ToolCall != nil:
				calls[p.ToolCall.ID] = p.ToolCall
			case p.Text != nil && m.Role == chat.RoleUser:
				given = append(given, p.Text.Text)
			case p.Text != nil && m.Role == chat.RoleAssistant && m.Turn < turn && !fired[m.Turn]:
				given = append(given, p.Text.Text)
			case p.Notice != nil && p.Notice.Kind != NoticeAppCheck && p.Notice.Kind != NoticeUnsourced:
				given = append(given, p.Notice.Text)
			case p.ToolResult != nil && reads != nil && calls[p.ToolResult.CallID] != nil && reads(calls[p.ToolResult.CallID].Name):
				read = append(read, readText{p.ToolResult.Text, calls[p.ToolResult.CallID].Args})
			case p.ToolResult != nil:
				given = append(given, p.ToolResult.Text)
			}
		}
	}
	return given, read
}

// readText is the result of a tool that reads the project, and its
// call's arguments.
type readText struct {
	text string
	args json.RawMessage
}

// callArgs are the arguments that tie a write to a later read.
type callArgs struct {
	Key   string `json:"key"`   // bucket and memory
	Ref   string `json:"ref"`   // read_ref, search_ref: a ref is a bucket key
	Table string `json:"table"` // insert
	SQL   string `json:"sql"`   // query
}

func argsOf(args json.RawMessage) callArgs {
	var a callArgs
	json.Unmarshal(args, &a)
	return a
}

// argStrings are the string and number values in a call's arguments.
func argStrings(args json.RawMessage) []string {
	dec := json.NewDecoder(bytes.NewReader(args))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return nil
	}
	var out []string
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			out = append(out, x)
		case json.Number:
			out = append(out, x.String())
		case []any:
			for _, e := range x {
				walk(e)
			}
		case map[string]any:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(v)
	return out
}

// number is a number in a text.
type number struct {
	text  string  // as written, with ASCII digits
	value float64 // the absolute value, with k, M and the like applied
	half  float64 // half a unit of its last written digit: the rounding a match allows
	small bool    // not checked: a small integer, a year (1900-2100, 1300-1500) or list numbering
	pct   bool    // a percentage: 5.2% may be written 0.052
}

// near reports whether m and n are the same value within the coarser
// rounding of the two, so 61,240 = 61.2k = 61,240.00.
func (n number) near(m number) bool {
	return math.Abs(m.value-n.value) <= max(n.half, m.half)+eps(n.value)
}

// same reports whether m and n are the same value at the finer rounding
// of the two: 61,240 = 61,240.00, but 64k is not 63,870.40.
func (n number) same(m number) bool {
	return math.Abs(m.value-n.value) <= min(n.half, m.half)+eps(n.value)
}

// fraction is a percentage as a fraction: 5.2% as 0.052.
func (n number) fraction() number {
	n.value, n.half, n.pct = n.value/100, n.half/100, false
	return n
}

// checkResult is what checkNumbers finds.
type checkResult struct {
	checked int      // numbers checked, each value once
	missing []number // those no source has, in order
	saved   int      // of them, in saved or left
	left    int      // of them, in left
	quoted  int      // numbers in said left out as quoted
	pct     int      // numbers sourced only as a percentage read the other way
}

// checkNumbers checks the numbers in said, saved and left (what was saved
// before a notice and is still there) against src. Each value counts
// once, however it is written: the finest form of it is checked, and a
// rounding of it (2.52k, 2,520 for 2,520.50) goes with it, whatever the
// order. A rounding never stands for a finer number, so "about 64k" with
// a source can't hide a made-up 63,870.40. Numbers in said that are the
// same as one in quoted are left out.
func checkNumbers(said, saved, left []string, src sourceSet, quoted []number) checkResult {
	var res checkResult
	type seen struct {
		n     number
		where int // 1 said, 2 saved, 4 left
		at    int // the order it came in
		miss  bool
	}
	var found []seen
	for _, part := range []struct {
		where int
		texts []string
	}{{1, said}, {2, saved}, {4, left}} {
		for _, s := range part.texts {
			for _, n := range numbers(s) {
				if n.small {
					continue
				}
				if part.where == 1 && slices.ContainsFunc(quoted, n.same) {
					res.quoted++
					continue
				}
				found = append(found, seen{n: n, where: part.where, at: len(found)})
			}
		}
	}
	// The finest first, so a rounding finds the number it rounds.
	slices.SortStableFunc(found, func(a, b seen) int { return cmpFloat(a.n.half, b.n.half) })
	var all []seen
	for _, f := range found {
		if i := slices.IndexFunc(all, func(o seen) bool { return f.n.near(o.n) }); i >= 0 {
			all[i].where |= f.where
			all[i].at = min(all[i].at, f.at)
			continue
		}
		switch {
		case src.match(f.n):
		case src.matchPct(f.n):
			res.pct++
		default:
			f.miss = true
		}
		all = append(all, f)
	}
	res.checked = len(all)
	slices.SortFunc(all, func(a, b seen) int { return a.at - b.at })
	for _, s := range all {
		if !s.miss {
			continue
		}
		res.missing = append(res.missing, s.n)
		if s.where&6 != 0 {
			res.saved++
		}
		if s.where&4 != 0 {
			res.left++
		}
	}
	return res
}

// sourceSet is the sources' numbers: fine ones, rounded to a unit or
// less, sorted by value, and coarse ones (64,000, 1.21T), which are few.
// pcts are the percentages as fractions.
type sourceSet struct {
	fine, coarse, pcts []number
}

// newSourceSet reads the numbers of texts, except those the same as one
// of skip. Call sort before match.
func newSourceSet(texts []string, skip []number) sourceSet {
	var src sourceSet
	src.add(texts, skip)
	src.sort()
	return src
}

func (src *sourceSet) add(texts []string, skip []number) {
	for _, s := range texts {
		for _, n := range numbers(s) {
			if slices.ContainsFunc(skip, n.same) {
				continue
			}
			if n.pct {
				src.pcts = append(src.pcts, n.fraction())
			}
			if n.half > 0.5 {
				src.coarse = append(src.coarse, n)
			} else {
				src.fine = append(src.fine, n)
			}
		}
	}
}

func (src *sourceSet) sort() {
	slices.SortFunc(src.fine, func(a, b number) int { return cmpFloat(a.value, b.value) })
}

// match reports whether some source matches n within rounding.
func (src sourceSet) match(n number) bool {
	reach := max(n.half, 0.5) + eps(n.value)
	i, _ := slices.BinarySearchFunc(src.fine, n.value-reach, func(s number, v float64) int { return cmpFloat(s.value, v) })
	for ; i < len(src.fine) && src.fine[i].value <= n.value+reach; i++ {
		if n.near(src.fine[i]) {
			return true
		}
	}
	return slices.ContainsFunc(src.coarse, n.near)
}

// matchPct reports whether n matches a source as a percentage written
// the other way: 5.2% for a source's 0.052, or 0.052 for a source's 5.2%.
func (src sourceSet) matchPct(n number) bool {
	if n.pct && src.match(n.fraction()) {
		return true
	}
	return !n.pct && slices.ContainsFunc(src.pcts, n.near)
}

func eps(v float64) float64 { return 1e-9 * max(1, math.Abs(v)) }

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// digits maps Persian and Arabic-Indic digits and separators to ASCII
// (5.8), so ۶۱٬۲۴۰٫۵ reads as 61,240.5.
func digits(r rune) rune {
	switch {
	case r >= '۰' && r <= '۹':
		return '0' + r - '۰'
	case r >= '٠' && r <= '٩':
		return '0' + r - '٠'
	}
	switch r {
	case '٫':
		return '.'
	case '٬':
		return ','
	case '٪':
		return '%'
	}
	return r
}

// Dates and times, left out: 2026-09-29, 1405/07/21, 29.09.2026, 9/29/26,
// 2026-09 and 08:00. It matches a whole run of digits, '-', '/', ':' and
// '.'.
var reDate = regexp.MustCompile(`^(?:(1[3-9]|2[01])\d\d[-/.](0?[1-9]|1[0-2])[-/.](0?[1-9]|[12]\d|3[01])|` +
	`(0?[1-9]|[12]\d|3[01])[-/.](0?[1-9]|1[0-2])[-/.]((1[3-9]|2[01])\d\d|\d\d)|` +
	`(0?[1-9]|1[0-2])/(0?[1-9]|[12]\d|3[01])/(\d\d\d\d|\d\d)|` +
	`(19|20)\d\d-(0[1-9]|1[0-2])|` +
	`\d{1,2}:\d\d(:\d\d)?)$`)

// scales are the suffixes written after a number, glued (61.2k) or as a
// word (410 million, ۲۱ میلیون). Words of more than two letters match in
// any case.
var scales = map[string]float64{
	"k": 1e3, "K": 1e3, "M": 1e6, "B": 1e9, "bn": 1e9, "T": 1e12,
	"thousand": 1e3, "million": 1e6, "billion": 1e9, "trillion": 1e12,
	"هزار": 1e3, "میلیون": 1e6, "میلیارد": 1e9, "تریلیون": 1e12,
}

// scaleOf is the scale a word after a number stands for, or 0.
func scaleOf(w string) float64 {
	if len([]rune(w)) > 2 {
		return scales[strings.ToLower(w)]
	}
	return scales[w]
}

// numbers finds the numbers in s. A number after a letter or '_' is part
// of a name (v3, r_1, gpt4, قیمت۲) and left out, as is one glued to
// letters and then digits (0x1f, 3a9c). Letters glued after a number are
// a unit (24h, 64512USD, ۶۴۵۱۲دلار): the number is read, in every script
// alike. Dates and times are left out. Thousands separators, decimals,
// k/M/B/T and their words, %, currency signs and Persian and Arabic
// digits are read.
func numbers(s string) []number {
	rs := []rune(strings.Map(digits, s))
	groupSpaces(rs)
	var out []number
	for i := 0; i < len(rs); {
		if !isDigit(rs[i]) {
			i++
			continue
		}
		start := i
		j := i
		for j < len(rs) && (isDigit(rs[j]) || strings.ContainsRune("-/:.", rs[j]) && j+1 < len(rs) && isDigit(rs[j+1])) {
			j++
		}
		if tok := string(rs[start:j]); (strings.ContainsAny(tok, "-/:") || strings.Count(tok, ".") == 2) && reDate.MatchString(tok) {
			i = j
			continue
		}
		j = i
		for j < len(rs) && (isDigit(rs[j]) || (rs[j] == ',' || rs[j] == '.') && j+1 < len(rs) && isDigit(rs[j+1])) {
			j++
		}
		i = j
		if start > 0 && (isLetter(rs[start-1]) || rs[start-1] == '_') {
			i = skipWord(rs, j)
			continue
		}
		scale, end := 1.0, j
		if j < len(rs) && isLetter(rs[j]) {
			w := word(rs, j)
			k := j + len([]rune(w))
			if k < len(rs) && (isDigit(rs[k]) || rs[k] == '_') {
				i = skipWord(rs, k)
				continue // a name or a hash: 0x1f, 3a9c
			}
			i = k
			if f := scaleOf(w); f > 0 {
				scale, end = f, k // 61.2k, ۲۱میلیون
			}
		} else if j < len(rs) && isSpace(rs[j]) {
			if f := scaleOf(word(rs, j+1)); f > 0 {
				scale = f // 410 million, $410 M, ۲۱ میلیون
			}
		}
		currency := false
		for k := start - 1; k >= 0 && k >= start-2; k-- {
			if strings.ContainsRune("$€£¥", rs[k]) {
				currency = true
			}
			if rs[k] != ' ' {
				break
			}
		}
		listItem := lineStart(rs, start) && end < len(rs) && (rs[end] == '.' || rs[end] == ')')
		pct := end < len(rs) && rs[end] == '%' || end+1 < len(rs) && isSpace(rs[end]) && rs[end+1] == '%'
		ns := parseRun(string(rs[start:j]), scale, currency || scale > 1, listItem)
		if pct && len(ns) > 0 {
			ns[len(ns)-1].pct = true
		}
		out = append(out, ns...)
	}
	return out
}

// groupSpaces turns the narrow, thin and no-break spaces that group
// digits by three (62 980.10, as SI and French write it) into commas.
// A plain space is left alone: in "7 100" it is two numbers as often as
// one.
func groupSpaces(rs []rune) {
	for k := 1; k+3 < len(rs); k++ {
		switch rs[k] {
		case ' ', ' ', ' ':
		default:
			continue
		}
		if isDigit(rs[k-1]) && isDigit(rs[k+1]) && isDigit(rs[k+2]) && isDigit(rs[k+3]) && (k+4 == len(rs) || !isDigit(rs[k+4])) {
			rs[k] = ','
		}
	}
}

// reRunPart is one number in a run: with commas that group by three
// (61,240.5), or without commas.
var reRunPart = regexp.MustCompile(`\d{1,3}(?:,\d{3})+(?:\.\d+)?|\d+(?:\.\d+)?`)

// parseRun reads a run of digits, commas and dots: one number with
// thousands separators, or a list (1,5,7 or 62980.10,63312.75) where the
// commas don't group by three. A run with two dots and no commas (a
// version, 1.2.3) is left out.
func parseRun(run string, scale float64, counted, listItem bool) []number {
	if strings.Count(run, ".") > 1 && !strings.Contains(run, ",") {
		return nil
	}
	// Commas group by three only where a group ends the digits: 23,62980.10
	// is a list.
	parts := []string{run}
	if strings.Contains(run, ",") {
		parts = commaParts(run)
	}
	var out []number
	for _, p := range parts {
		if strings.Count(p, ".") > 1 {
			continue
		}
		plain := strings.ReplaceAll(p, ",", "")
		v, err := strconv.ParseFloat(plain, 64)
		if err != nil {
			continue
		}
		whole, frac, _ := strings.Cut(plain, ".")
		var half float64
		if frac != "" {
			half = 0.5 * math.Pow10(-len(frac))
		} else {
			// Trailing zeros may be rounding (64,000 for 64,240), but at
			// least two digits are kept, so 60,000 is not 55,000 to 65,000.
			sig := strings.TrimLeft(whole, "0")
			tz := len(sig) - len(strings.TrimRight(sig, "0"))
			half = 0.5 * math.Pow10(min(tz, max(len(sig)-2, 0)))
		}
		n := number{text: p, value: v * scale, half: half * scale}
		if scale > 1 {
			n.text = p + suffixOf(scale)
		}
		// Years: 1900-2100, and 1300-1500 for the Solar Hijri calendar (1405).
		year := frac == "" && !strings.Contains(p, ",") && len(whole) == 4 && (v >= 1900 && v <= 2100 || v >= 1300 && v <= 1500)
		n.small = !counted && frac == "" && (v < 1000 || year) || listItem && frac == "" && v < 1000
		out = append(out, n)
	}
	return out
}

// commaParts splits a run with commas into its numbers. More than three
// groups of three (100,102,101,103) are a list, not one number of a
// trillion or more: those are written 1.2T.
func commaParts(run string) []string {
	var parts []string
	for _, m := range reRunPart.FindAllStringIndex(run, -1) {
		if m[1] < len(run) && isDigit(rune(run[m[1]])) || strings.Count(run[m[0]:m[1]], ",") > 3 {
			return strings.Split(run, ",")
		}
		parts = append(parts, run[m[0]:m[1]])
	}
	return parts
}

func suffixOf(scale float64) string {
	switch scale {
	case 1e3:
		return "k"
	case 1e6:
		return "M"
	case 1e9:
		return "B"
	}
	return "T"
}

func isDigit(r rune) bool  { return r >= '0' && r <= '9' }
func isLetter(r rune) bool { return unicode.IsLetter(r) }
func isSpace(r rune) bool  { return r == ' ' || r == ' ' || r == ' ' || r == ' ' }

// word is the run of letters at rs[i:].
func word(rs []rune, i int) string {
	j := i
	for j < len(rs) && isLetter(rs[j]) {
		j++
	}
	return string(rs[i:j])
}

// skipWord passes the letters, digits and '_' of a name.
func skipWord(rs []rune, i int) int {
	for i < len(rs) && (isLetter(rs[i]) || isDigit(rs[i]) || rs[i] == '_') {
		i++
	}
	return i
}

// lineStart reports whether only spaces, '-', '*' or '#' come before
// rs[i] on its line, as in list numbering.
func lineStart(rs []rune, i int) bool {
	for k := i - 1; k >= 0; k-- {
		switch rs[k] {
		case '\n':
			return true
		case ' ', '\t', '-', '*', '#':
			continue
		}
		return false
	}
	return true
}
