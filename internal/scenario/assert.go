package scenario

import (
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/amir-saatchi/jenab/internal/chat"
)

// Check results.
const (
	Pass = "pass"
	Fail = "fail"
	Skip = "skip" // its precondition didn't happen, such as a finish turn
)

// Result is one check of one run.
type Result struct {
	Type   string  `json:"type"`
	Test   string  `json:"test,omitempty"`
	Status string  `json:"status"`
	Why    string  `json:"why,omitempty"`
	Value  float64 `json:"value,omitempty"`
}

// assertFields are the assertion types and the fields each needs.
var assertFields = map[string][]string{
	"text_before_tool":    nil,
	"tool_called":         {"tools"},
	"tool_not_called":     {"tools"},
	"max_calls":           {"tools"},
	"max_prompt_tokens":   {"n"},
	"answer_contains":     {"match"},
	"answer_delivered":    nil,
	"no_repeat":           nil,
	"max_status_polls":    {"tools"},
	"skill_loaded":        {"skills"},
	"skill_loaded_before": {"skills"},
	"only_skills":         nil,
	"created_skills":      {"tools", "skills"},
	"after_message":       nil,
	"no_background":       nil,
	"max_turns":           {"n"},
	"no_false_running":    nil,
}

func assertTypes() []string { return slices.Sorted(maps.Keys(assertFields)) }

// check runs a scenario's assertions on its record.
func check(rec *record, sc *Scenario) []Result {
	out := make([]Result, 0, len(sc.Asserts))
	for _, a := range sc.Asserts {
		st, why, v := rec.check(a)
		out = append(out, Result{Type: a.Type, Test: a.Test, Status: st, Why: why, Value: v})
	}
	return out
}

// Text that says work goes on after the answer. no_false_running fails
// on it when nothing was started (SPIKE-023).
var reFalseRunning = regexp.MustCompile(`(?i)(in the background|still running|is running|(will|'ll) (let you know|report back|get back|notify|update you|follow up)|` +
	`once (it|the run|the task|the pipeline) (finishes|completes|is done)|as soon as (it|the run|the task)|in the meantime|meanwhile)`)

func pf(ok bool, why string) (string, string, float64) {
	if ok {
		return Pass, why, 0
	}
	return Fail, why, 0
}

func (rec *record) check(a Assert) (string, string, float64) {
	tools := a.Tools
	if a.Tool != "" {
		tools = append(slices.Clone(tools), a.Tool)
	}
	skills := a.Skills
	if a.Skill != "" {
		skills = append(slices.Clone(skills), a.Skill)
	}
	switch a.Type {
	case "text_before_tool":
		for _, e := range rec.events {
			if e.kind == evText && len(strings.TrimSpace(e.text)) >= 20 {
				return Pass, clip(e.text, 80), 0
			}
			if e.kind == evCall && (len(tools) == 0 || slices.Contains(tools, e.tool)) {
				return Fail, "the first " + e.tool + " call came before any text", 0
			}
		}
		return Skip, "no tool was called", 0
	case "tool_called", "tool_not_called":
		var hit *event
		for _, e := range rec.calls(a.Turn) {
			if slices.Contains(tools, e.tool) && argsFit(a, e.args) {
				hit = &e
				break
			}
		}
		if a.Type == "tool_called" {
			if hit == nil {
				return Fail, "not called: " + strings.Join(tools, ", "), 0
			}
			return Pass, hit.tool + " " + clip(hit.args, 100), 0
		}
		if hit != nil {
			return Fail, "called " + hit.tool + " " + clip(hit.args, 100), 0
		}
		return Pass, "", 0
	case "max_calls":
		n := 0
		for _, e := range rec.calls("") {
			if slices.Contains(tools, e.tool) && argsFit(a, e.args) {
				n++
			}
		}
		st, why, _ := pf(n <= a.N, fmt.Sprintf("%d calls", n))
		return st, why, float64(n)
	case "max_prompt_tokens":
		if len(rec.requests) == 0 {
			return Skip, "no requests", 0
		}
		p := rec.peak()
		st, why, _ := pf(p <= a.N, fmt.Sprintf("peak prompt %d tokens", p))
		return st, why, float64(p)
	case "answer_contains":
		re := regexp.MustCompile(a.Match)
		if a.Turn == "finish" && len(rec.finishTurns()) == 0 {
			// The rest given in the same turn is fine; never given isn't.
			if re.MatchString(rec.text("")) {
				return Skip, "no finish turn; the rest was given in the first turn", 0
			}
			return Fail, "no finish turn, and the rest was never given", 0
		}
		txt := rec.text(a.Turn)
		return pf(re.MatchString(txt), clip(txt, 160))
	case "answer_delivered":
		last := rec.turns()
		var final string
		for _, e := range rec.events {
			if e.turn != last {
				continue
			}
			switch {
			case e.kind == evNotice && (e.notice == chat.NoticeTurnFailed || e.notice == chat.NoticeAnswerCut):
				return Fail, "the turn ended with " + string(e.notice) + ": " + clip(e.text, 120), 0
			case e.kind == evText && e.final:
				final += e.text + "\n"
			}
		}
		if strings.TrimSpace(final) == "" {
			return Fail, "the last turn has no answer", 0
		}
		if a.Match != "" && !regexp.MustCompile(a.Match).MatchString(final) {
			return Fail, "the answer doesn't match " + a.Match + ": " + clip(final, 120), 0
		}
		return Pass, clip(final, 120), 0
	case "no_repeat":
		return rec.noRepeat(a)
	case "max_status_polls":
		n := map[string]int{}
		worst, which := 0, ""
		for _, e := range rec.calls("") {
			if !slices.Contains(tools, e.tool) {
				continue
			}
			k := e.tool + " " + canonical(e.args)
			if n[k]++; n[k] > worst {
				worst, which = n[k], k
			}
		}
		limit := max(a.N, 1)
		if worst > limit {
			return Fail, fmt.Sprintf("%d calls of %s", worst, clip(which, 100)), float64(worst)
		}
		return Pass, fmt.Sprintf("at most %d calls for one target", worst), float64(worst)
	case "skill_loaded":
		var missing []string
		for _, sk := range skills {
			if !slices.Contains(rec.sc.Skills, sk) && !slices.ContainsFunc(rec.events, func(e event) bool { return e.kind == evSkill && e.skill == sk }) {
				missing = append(missing, sk)
			}
		}
		if len(missing) > 0 {
			return Fail, "never loaded: " + strings.Join(missing, ", "), 0
		}
		return Pass, "", 0
	case "skill_loaded_before":
		return rec.skillBefore(skills, tools)
	case "only_skills":
		var extra []string
		for _, e := range rec.calls("") {
			if e.tool != "load_skill" {
				continue
			}
			if n := argText([]byte(e.args), "name"); !slices.Contains(skills, n) && !slices.Contains(extra, n) {
				extra = append(extra, n)
			}
		}
		if len(extra) > 0 {
			return Fail, "loaded " + strings.Join(extra, ", "), float64(len(extra))
		}
		return Pass, "", 0
	case "created_skills":
		return rec.createdSkills(tools, skills, a.Allow)
	case "after_message":
		return rec.afterMessage(a)
	case "no_background":
		var started []string
		for _, e := range rec.calls("") {
			if isBackground(rec.sc.set.tool(e.tool), e.args) {
				started = append(started, e.tool)
			}
		}
		if len(started) > 0 {
			return Fail, "started " + strings.Join(started, ", "), 0
		}
		return Pass, "", 0
	case "max_turns":
		n := rec.turns()
		st, why, _ := pf(n <= a.N, fmt.Sprintf("%d turns", n))
		return st, why, float64(n)
	case "no_false_running":
		for _, e := range rec.calls("") {
			if isBackground(rec.sc.set.tool(e.tool), e.args) {
				return Pass, "background work was started", 0
			}
		}
		if m := reFalseRunning.FindString(rec.text("")); m != "" {
			return Fail, "says: " + m, 0
		}
		return Pass, "", 0
	}
	return Fail, "unknown assertion", 0
}

func argsFit(a Assert, args string) bool {
	return (a.ArgsMatch == "" || regexp.MustCompile(a.ArgsMatch).MatchString(args)) &&
		(a.ArgsNot == "" || !regexp.MustCompile(a.ArgsNot).MatchString(args))
}

// canonical is JSON with sorted keys and no spaces, so two calls with the
// same arguments compare equal.
func canonical(args string) string {
	var v any
	if json.Unmarshal([]byte(args), &v) != nil {
		return args
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// inTurns returns the turns sel picks: first, last (the last with the
// agent's text), finish (turns a notice started), or any.
func (rec *record) inTurns(sel string) func(int) bool {
	switch sel {
	case "first":
		return func(t int) bool { return t == 1 }
	case "last":
		last := 0
		for _, e := range rec.events {
			if e.kind == evText && strings.TrimSpace(e.text) != "" {
				last = e.turn
			}
		}
		return func(t int) bool { return t == last }
	case "finish":
		fin := rec.finishTurns()
		return func(t int) bool { return slices.Contains(fin, t) }
	}
	return func(int) bool { return true }
}

// finishTurns are the turns a task_finished notice started (8.3).
func (rec *record) finishTurns() []int {
	var out []int
	seen := map[int]bool{}
	for _, e := range rec.events {
		if seen[e.turn] {
			continue
		}
		seen[e.turn] = true
		if e.kind == evNotice && e.notice == chat.NoticeTaskFinished {
			out = append(out, e.turn)
		}
	}
	return out
}

func (rec *record) calls(sel string) []event {
	in := rec.inTurns(sel)
	var out []event
	for _, e := range rec.events {
		if e.kind == evCall && in(e.turn) {
			out = append(out, e)
		}
	}
	return out
}

// text is the agent's text in the turns sel picks.
func (rec *record) text(sel string) string {
	in := rec.inTurns(sel)
	var b strings.Builder
	for _, e := range rec.events {
		if e.kind == evText && in(e.turn) {
			b.WriteString(e.text + "\n")
		}
	}
	return b.String()
}

// skillBefore checks that each skill was in the chat before the gate: the
// first call of one of tools, or the first answer without tool calls.
// Skills loaded at the start count.
func (rec *record) skillBefore(skills, tools []string) (string, string, float64) {
	gate := slices.IndexFunc(rec.events, func(e event) bool {
		return e.kind == evCall && slices.Contains(tools, e.tool) || e.kind == evText && e.final && strings.TrimSpace(e.text) != ""
	})
	if gate < 0 {
		return Fail, "the agent neither answered nor called " + strings.Join(tools, ", "), 0
	}
	what := "the answer"
	if g := rec.events[gate]; g.kind == evCall {
		what = "the first " + g.tool + " call"
	}
	loaded := slices.Clone(rec.sc.Skills)
	for _, e := range rec.events[:gate] {
		if e.kind == evSkill {
			loaded = append(loaded, e.skill)
		}
	}
	var missing []string
	for _, s := range skills {
		if !slices.Contains(loaded, s) {
			missing = append(missing, s)
		}
	}
	if len(missing) > 0 {
		return Fail, strings.Join(missing, ", ") + " not loaded before " + what, 0
	}
	return Pass, "loaded before " + what, 0
}

// createdSkills checks the skills of the first call of tools, such as
// create_chat: every expected skill, and none outside expected and allow.
func (rec *record) createdSkills(tools, want, allow []string) (string, string, float64) {
	i := slices.IndexFunc(rec.events, func(e event) bool { return e.kind == evCall && slices.Contains(tools, e.tool) })
	if i < 0 {
		var used []string
		for _, e := range rec.calls("") {
			used = append(used, e.tool)
		}
		return Fail, "not called: " + strings.Join(tools, ", ") + "; called: " + strings.Join(used, ", "), 0
	}
	got := strList(rec.events[i].args, "skills")
	var missing, outside, extra []string
	for _, s := range want {
		if !slices.Contains(got, s) {
			missing = append(missing, s)
		}
	}
	for _, s := range got {
		switch {
		case slices.Contains(want, s):
		case slices.Contains(allow, s):
			extra = append(extra, s)
		default:
			outside = append(outside, s)
		}
	}
	switch {
	case len(missing) > 0 || len(outside) > 0:
		return Fail, fmt.Sprintf("skills %v: missing %v, not fitting %v", got, missing, outside), 0
	case len(extra) > 0:
		return Pass, fmt.Sprintf("acceptable: skills %v", got), 0
	}
	return Pass, fmt.Sprintf("exact: skills %v", got), 1
}

// strList reads a list of strings from a call's arguments. Some models
// send it as a JSON string or a comma list.
func strList(args, name string) []string {
	var m map[string]any
	if json.Unmarshal([]byte(args), &m) != nil {
		return nil
	}
	var out []string
	switch v := m[name].(type) {
	case []any:
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, strings.ToLower(strings.TrimSpace(s)))
			}
		}
	case string:
		var l []string
		if json.Unmarshal([]byte(v), &l) != nil {
			l = strings.Split(v, ",")
		}
		for _, x := range l {
			if x = strings.TrimSpace(x); x != "" {
				out = append(out, strings.ToLower(x))
			}
		}
	}
	return out
}

// afterMessage checks what came after the first scripted message after
// the opening one: a later call or text that takes it up.
func (rec *record) afterMessage(a Assert) (string, string, float64) {
	at := slices.IndexFunc(rec.events, func(e event) bool { return e.kind == evUser && e.msg >= 1 })
	if at < 0 {
		return Fail, "the message never arrived", 0
	}
	how := "between turns"
	if slices.ContainsFunc(rec.events[:at], func(e event) bool { return e.turn == rec.events[at].turn }) {
		how = "during a turn"
	}
	var args, txt strings.Builder
	for _, e := range rec.events[at+1:] {
		switch e.kind {
		case evCall:
			args.WriteString(e.tool + " " + e.args + "\n")
		case evText:
			txt.WriteString(e.text + "\n")
		}
	}
	ok, why := true, []string{"arrived " + how}
	if a.ArgsMatch != "" && !regexp.MustCompile(a.ArgsMatch).MatchString(args.String()) {
		ok, why = false, append(why, "no later call matches "+a.ArgsMatch)
	}
	if a.AnswerMatch != "" && !regexp.MustCompile(a.AnswerMatch).MatchString(txt.String()) {
		ok, why = false, append(why, "the later text doesn't match "+a.AnswerMatch)
	}
	if a.Match != "" && !regexp.MustCompile(a.Match).MatchString(txt.String()+args.String()) {
		ok, why = false, append(why, "no later text or call matches "+a.Match)
	}
	return pf(ok, strings.Join(why, "; "))
}

// noRepeat checks that a finish turn doesn't repeat what the agent said
// before: the share of its word 5-grams seen earlier stays at most
// MaxOverlap (0.3 by default), and no marker appears in both.
func (rec *record) noRepeat(a Assert) (string, string, float64) {
	fin := rec.finishTurns()
	if len(fin) == 0 {
		return Skip, "no finish turn", 0
	}
	limit := a.MaxOverlap
	if limit == 0 {
		limit = 0.3
	}
	worst, why := 0.0, ""
	for _, t := range fin {
		var earlier, now strings.Builder
		for _, e := range rec.events {
			if e.kind != evText {
				continue
			}
			switch {
			case e.turn < t:
				earlier.WriteString(e.text + "\n")
			case e.turn == t:
				now.WriteString(e.text + "\n")
			}
		}
		worst = max(worst, overlap(now.String(), earlier.String()))
		for _, m := range a.Markers {
			re := regexp.MustCompile(m)
			if re.MatchString(now.String()) && re.MatchString(earlier.String()) {
				why = "repeats " + re.FindString(now.String())
			}
		}
	}
	switch {
	case why != "":
		return Fail, fmt.Sprintf("%s (5-gram overlap %.2f)", why, worst), worst
	case worst > limit:
		return Fail, fmt.Sprintf("5-gram overlap %.2f > %.2f", worst, limit), worst
	}
	return Pass, fmt.Sprintf("5-gram overlap %.2f", worst), worst
}

const ngramN = 5

// words are lower-case runs of letters and digits, so "64,210.50" is 64
// 210 50.
func words(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

func ngrams(s string) map[string]bool {
	w := words(s)
	out := map[string]bool{}
	for i := 0; i+ngramN <= len(w); i++ {
		out[strings.Join(w[i:i+ngramN], " ")] = true
	}
	return out
}

// overlap is the share of now's word 5-grams that appear in earlier.
func overlap(now, earlier string) float64 {
	f := ngrams(now)
	if len(f) == 0 {
		return 0
	}
	e := ngrams(earlier)
	n := 0
	for g := range f {
		if e[g] {
			n++
		}
	}
	return float64(n) / float64(len(f))
}
