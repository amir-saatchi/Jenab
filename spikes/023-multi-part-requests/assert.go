package main

// Assertions on a finished run. Each returns pass, fail or skip (precondition not met, e.g. no
// finish turn because the model never started background work).

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

type AssertRes struct {
	Type   string  `json:"type"`
	Test   string  `json:"test"`
	Status string  `json:"status"` // pass, fail, skip
	Why    string  `json:"why,omitempty"`
	Value  float64 `json:"value,omitempty"`
}

var knownAssert = map[string]bool{
	"quick_before_finish": true, "slow_started": true, "background_started": true, "mentions_running": true,
	"text_before_tool": true, "answer_contains": true, "no_repeat": true, "no_restart": true, "after_message": true,
	"tool_called": true, "tool_not_called": true, "max_calls": true, "no_background": true, "max_turns": true,
	"no_false_running": true, "max_prompt_tokens": true, "right_target": true, "created_chat_only_for_lasting_job": true,
	"used_subagent_for_one_off": true, "no_delegation_when_simple": true, "no_resend_after_notice": true, "finish_turn": true,
}

// Claims that work is still going on. Used by mentions_running (should be there after starting
// background work) and no_false_running (must not be there when nothing was started).
var reRunning = regexp.MustCompile("(?i)(still running|is running|are running|now running|in the background|in progress|underway|under way|kicked off|" +
	"(has|have|'ve|is|are) (been |now )?(started|launched|sent)|(started|launched) (the|a|run|task|it)|" +
	"(am|'m|is|are) (now )?(fetching|running|getting|checking|reviewing|summari[sz]ing|working|reading|backfilling)|" +
	"(will|'ll) (let you know|report|update|share|post|get back|notify|pass|follow up|send you|give you)|" +
	"once (it|the run|the task|the pipeline|the subagent|they|that|the summary|the review|the reply|it's|its)|" +
	"when (it|the run|the task|the pipeline|they|the reply|the review|the summary)[^.]{0,30}(finish|complete|done|ready|come|arrive)|" +
	"as soon as|shortly|in the meantime|meanwhile)")

// stricter set for simple requests: phrases that promise later results
var reFalseRunning = regexp.MustCompile(`(?i)(in the background|still running|is running|(will|'ll) (let you know|report back|get back|notify|update you|follow up)|` +
	`once (it|the run|the task|the pipeline) (finishes|completes|is done)|as soon as (it|the run|the task)|in the meantime|meanwhile)`)

type view struct {
	o *Orch
}

func (v view) allParts() []Part {
	var out []Part
	for _, t := range v.o.turns {
		out = append(out, t.Parts...)
	}
	return out
}

func (v view) systemTurns() []*Turn {
	var out []*Turn
	for _, t := range v.o.turns {
		if t.Kind == "system" {
			out = append(out, t)
		}
	}
	return out
}

func textOf(ts []*Turn) string {
	var b strings.Builder
	for _, t := range ts {
		for _, p := range t.Parts {
			if p.Kind == "text" {
				b.WriteString(p.Text)
				b.WriteString("\n")
			}
		}
	}
	return b.String()
}

func callsOf(ts []*Turn) []Part {
	var out []Part
	for _, t := range ts {
		for _, p := range t.Parts {
			if p.Kind == "call" {
				out = append(out, p)
			}
		}
	}
	return out
}

func (v view) turnsSel(sel string) []*Turn {
	switch sel {
	case "first":
		if len(v.o.turns) > 0 {
			return v.o.turns[:1]
		}
		return nil
	case "finish":
		return v.systemTurns()
	case "last":
		for i := len(v.o.turns) - 1; i >= 0; i-- {
			if strings.TrimSpace(textOf(v.o.turns[i:i+1])) != "" {
				return v.o.turns[i : i+1]
			}
		}
		return nil
	case "user":
		var out []*Turn
		for _, t := range v.o.turns {
			if t.Kind == "user" {
				out = append(out, t)
			}
		}
		return out
	}
	return v.o.turns
}

func inSet(s string, set []string) bool {
	for _, x := range set {
		if x == s {
			return true
		}
	}
	return false
}

func resultOf(v view, callID string) string {
	for _, p := range v.allParts() {
		if p.Kind == "result" && p.CallID == callID {
			return p.Text
		}
	}
	return ""
}

func parseArgs(s string) map[string]any {
	var m map[string]any
	json.Unmarshal([]byte(s), &m)
	if m == nil {
		m = map[string]any{}
	}
	return m
}

// words for the n-gram measure: lower case, letters and digits only (so "64,210.50" is 64 210 50)
func words(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

const ngramN = 5

func ngrams(s string) map[string]bool {
	w := words(s)
	out := map[string]bool{}
	for i := 0; i+ngramN <= len(w); i++ {
		out[strings.Join(w[i:i+ngramN], " ")] = true
	}
	return out
}

// overlap: share of the finish text's word 5-grams that already appeared in earlier assistant text.
func overlap(finish, earlier string) float64 {
	f := ngrams(finish)
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

func mustRe(s string) *regexp.Regexp { return regexp.MustCompile(s) }

func checkAll(o *Orch, as []Assert, peakPrompt int) []AssertRes {
	v := view{o}
	var out []AssertRes
	for _, a := range as {
		st, why, val := check(v, a, peakPrompt)
		out = append(out, AssertRes{Type: a.Type, Test: a.Test, Status: st, Why: why, Value: val})
	}
	return out
}

func pf(ok bool, why string) (string, string, float64) {
	if ok {
		return "pass", why, 0
	}
	return "fail", why, 0
}

func check(v view, a Assert, peakPrompt int) (string, string, float64) {
	o := v.o
	switch a.Type {
	case "quick_before_finish":
		// time when the slow work is done
		slow := -1.0
		slowWhat := ""
		for _, t := range o.tasks {
			if len(a.Tools) == 0 || inSet(t.Tool, a.Tools) {
				if slow < 0 || t.Finish < slow {
					slow, slowWhat = t.Finish, t.Tool+" "+t.ID+" (background)"
				}
			}
		}
		// slow work done in the foreground ends with its last call in the opening turn
		fg, fgWhat := -1.0, ""
		if len(o.turns) > 0 {
			for _, p := range o.turns[0].Parts {
				if p.Kind == "result" && p.BgID == "" && (inSet(p.Tool, a.Tools) || (len(a.Tools) == 0 && p.Tool == "subagent")) && !strings.HasPrefix(p.Text, "error") {
					if p.T > fg {
						fg, fgWhat = p.T, p.Tool+" (foreground)"
					}
				}
			}
		}
		if fg >= 0 && (slow < 0 || fg < slow) {
			slow, slowWhat = fg, fgWhat
		}
		res := a.MatchAll
		if a.Match != "" {
			res = append(res, a.Match)
		}
		matched := make([]bool, len(res))
		quick := -1.0
		var acc strings.Builder
		for _, p := range v.allParts() {
			if p.Kind != "text" {
				continue
			}
			acc.WriteString(p.Text + "\n")
			all := true
			for i, r := range res {
				if !matched[i] && mustRe(r).MatchString(acc.String()) {
					matched[i] = true
				}
				all = all && matched[i]
			}
			if all {
				quick = p.T
				break
			}
		}
		switch {
		case quick < 0:
			return "fail", "quick part never answered", 0
		case slow < 0:
			return "pass", fmt.Sprintf("quick part at %.0f s; slow work never started", quick), quick
		case quick < slow:
			return "pass", fmt.Sprintf("quick part at %.0f s, slow work (%s) done at %.0f s", quick, slowWhat, slow), slow - quick
		default:
			return "fail", fmt.Sprintf("quick part at %.0f s, after the slow work (%s) was done at %.0f s", quick, slowWhat, slow), slow - quick
		}
	case "slow_started":
		for _, p := range v.allParts() {
			if p.Kind == "call" && inSet(p.Tool, a.Tools) && !strings.HasPrefix(resultOf(v, p.CallID), "error") {
				bg := "foreground"
				if p.BgID != "" {
					bg = "background " + p.BgID
				}
				return "pass", p.Tool + " " + bg, 0
			}
		}
		return "fail", "none of " + strings.Join(a.Tools, ", ") + " was started", 0
	case "background_started":
		for _, t := range o.tasks {
			if len(a.Tools) == 0 || inSet(t.Tool, a.Tools) {
				return "pass", t.Tool + " " + t.ID, 0
			}
		}
		return "fail", "no background task", 0
	case "mentions_running":
		if len(o.tasks) == 0 {
			return "skip", "no background task", 0
		}
		t0 := o.tasks[0]
		// text in the start turn from the response that started the task on
		startResp := 0
		for _, p := range v.allParts() {
			if p.Kind == "call" && p.BgID == t0.ID {
				startResp = p.Resp
			}
		}
		var b strings.Builder
		for _, p := range o.turns[t0.StartTurn-1].Parts {
			if p.Kind == "text" && p.Resp >= startResp {
				b.WriteString(p.Text + "\n")
			}
		}
		return pf(reRunning.MatchString(b.String()), short(b.String(), 120))
	case "text_before_tool":
		for _, p := range v.allParts() {
			if p.Kind == "text" && len(strings.TrimSpace(p.Text)) >= 20 {
				return "pass", short(p.Text, 80), 0
			}
			if p.Kind == "call" && (a.Tool == "" || p.Tool == a.Tool) {
				return "fail", "first " + p.Tool + " call came before any text", 0
			}
		}
		return "skip", "tool never called", 0
	case "answer_contains":
		ts := v.turnsSel(a.Turn)
		if a.Turn == "finish" && len(ts) == 0 {
			// no finish turn: fine if the rest was given in the same turn (polling or foreground
			// work), a failure if it was never given
			all := textOf(o.turns)
			if mustRe(a.Match).MatchString(all) {
				return "skip", "no finish turn; the rest was given in the first turn", 0
			}
			return "fail", "no finish turn and the rest was never given", 0
		}
		txt := textOf(ts)
		return pf(mustRe(a.Match).MatchString(txt), short(txt, 160))
	case "no_repeat":
		sys := v.systemTurns()
		if len(sys) == 0 {
			return "skip", "no finish turn", 0
		}
		maxOv := a.MaxOverlap
		if maxOv == 0 {
			maxOv = 0.3
		}
		worst := 0.0
		why := ""
		for _, st := range sys {
			var earlier strings.Builder
			for _, t := range o.turns {
				if t == st {
					break
				}
				earlier.WriteString(textOf([]*Turn{t}))
			}
			ft := textOf([]*Turn{st})
			ov := overlap(ft, earlier.String())
			if ov > worst {
				worst = ov
			}
			for _, m := range a.Markers {
				if mustRe(m).MatchString(ft) && mustRe(m).MatchString(earlier.String()) {
					why = "repeats: " + mustRe(m).FindString(ft)
				}
			}
		}
		if why != "" {
			return "fail", fmt.Sprintf("%s (5-gram overlap %.2f)", why, worst), worst
		}
		if worst > maxOv {
			return "fail", fmt.Sprintf("5-gram overlap %.2f > %.2f", worst, maxOv), worst
		}
		return "pass", fmt.Sprintf("5-gram overlap %.2f", worst), worst
	case "no_restart", "no_resend_after_notice":
		sys := v.systemTurns()
		if len(sys) == 0 {
			return "skip", "no finish turn", 0
		}
		tools := a.Tools
		if a.Type == "no_resend_after_notice" && len(tools) == 0 {
			tools = []string{"send_to_chat", "create_chat", "subagent"}
		}
		for _, c := range callsOf(sys) {
			if inSet(c.Tool, tools) {
				return "fail", "finish turn called " + c.Tool + " " + short(c.Args, 100), 0
			}
		}
		return "pass", "", 0
	case "finish_turn":
		return pf(len(v.systemTurns()) > 0, fmt.Sprintf("%d finish turns", len(v.systemTurns())))
	case "after_message":
		// the first scripted message after the opening one
		var after []Part
		found := false
		how := ""
		for _, m := range o.msgs {
			if m.idx >= 1 {
				how = m.how
				break
			}
		}
		for _, p := range v.allParts() {
			if !found && (p.Kind == "mid_user" || p.Kind == "user") && p.Msg >= 1 {
				found = true
				continue
			}
			if found {
				after = append(after, p)
			}
		}
		if !found {
			return "fail", "message was never delivered", 0
		}
		var args, txt strings.Builder
		for _, p := range after {
			if p.Kind == "call" {
				args.WriteString(p.Tool + " " + p.Args + "\n")
			}
			if p.Kind == "text" {
				txt.WriteString(p.Text + "\n")
			}
		}
		ok := true
		var why []string
		why = append(why, "delivered "+how)
		if a.ArgsMatch != "" && !mustRe(a.ArgsMatch).MatchString(args.String()) {
			ok = false
			why = append(why, "no later tool call matches "+a.ArgsMatch)
		}
		if a.AnswerRe != "" && !mustRe(a.AnswerRe).MatchString(txt.String()) {
			ok = false
			why = append(why, "later text does not match "+a.AnswerRe)
		}
		if a.Match != "" && mustRe(a.Match).MatchString(txt.String()+args.String()) == false {
			ok = false
			why = append(why, "no later text or call matches "+a.Match)
		}
		return pf(ok, strings.Join(why, "; "))
	case "tool_called", "tool_not_called":
		tools := a.Tools
		if a.Tool != "" {
			tools = append(tools, a.Tool)
		}
		var hit *Part
		for _, c := range callsOf(v.turnsSel(a.Turn)) {
			if inSet(c.Tool, tools) && (a.ArgsMatch == "" || mustRe(a.ArgsMatch).MatchString(c.Args)) && (a.ArgsNot == "" || !mustRe(a.ArgsNot).MatchString(c.Args)) {
				c := c
				hit = &c
				break
			}
		}
		if a.Type == "tool_called" {
			if hit == nil {
				return "fail", "not called: " + strings.Join(tools, ", "), 0
			}
			return "pass", hit.Tool + " " + short(hit.Args, 100), 0
		}
		if hit != nil {
			return "fail", "called " + hit.Tool + " " + short(hit.Args, 100), 0
		}
		return "pass", "", 0
	case "max_calls":
		n := 0
		for _, c := range callsOf(o.turns) {
			if c.Tool == a.Tool && (a.ArgsMatch == "" || mustRe(a.ArgsMatch).MatchString(c.Args)) && (a.ArgsNot == "" || !mustRe(a.ArgsNot).MatchString(c.Args)) {
				n++
			}
		}
		return pf(n <= a.N, fmt.Sprintf("%d calls", n))
	case "no_background":
		if len(o.tasks) > 0 {
			var ids []string
			for _, t := range o.tasks {
				ids = append(ids, t.Tool+" "+t.ID)
			}
			return "fail", "started " + strings.Join(ids, ", "), 0
		}
		for _, c := range callsOf(o.turns) {
			if c.Tool == "subagent" {
				return "fail", "foreground subagent", 0
			}
		}
		return "pass", "", 0
	case "max_turns":
		return pf(len(o.turns) <= a.N, fmt.Sprintf("%d turns", len(o.turns)))
	case "no_false_running":
		txt := textOf(o.turns)
		if m := reFalseRunning.FindString(txt); m != "" && len(o.tasks) == 0 {
			return "fail", "says: " + m, 0
		}
		return "pass", "", 0
	case "max_prompt_tokens":
		return pf(peakPrompt <= a.N, fmt.Sprintf("peak prompt %d", peakPrompt))
	case "right_target":
		var sent []string
		ok := false
		for _, c := range callsOf(o.turns) {
			if c.Tool != "send_to_chat" {
				continue
			}
			args := parseArgs(c.Args)
			id := argStr(args, "chat_id")
			sent = append(sent, id)
			if id == a.ChatID && (a.Match == "" || mustRe(a.Match).MatchString(argStr(args, "message"))) {
				ok = true
			}
		}
		wrong := []string{}
		for _, id := range sent {
			if id != a.ChatID {
				wrong = append(wrong, id)
			}
		}
		if len(wrong) > 0 {
			return "fail", "sent to " + strings.Join(wrong, ", "), 0
		}
		if !ok {
			if len(sent) > 0 {
				return "fail", "sent to " + a.ChatID + " without the needed context (" + a.Match + ")", 0
			}
			var used []string
			for _, c := range callsOf(o.turns) {
				used = append(used, c.Tool)
			}
			return "fail", "no send_to_chat; tools used: " + strings.Join(used, ", "), 0
		}
		return "pass", "", 0
	case "created_chat_only_for_lasting_job":
		expect := a.Expect != nil && *a.Expect
		var created []Part
		for _, c := range callsOf(o.turns) {
			if c.Tool == "create_chat" {
				created = append(created, c)
			}
		}
		if !expect {
			if len(created) > 0 {
				return "fail", "created a chat for a one-off job: " + short(created[0].Args, 120), 0
			}
			return "pass", "", 0
		}
		if len(created) == 0 {
			var used []string
			for _, c := range callsOf(o.turns) {
				used = append(used, c.Tool)
			}
			return "fail", "no create_chat; tools used: " + strings.Join(used, ", "), 0
		}
		args := parseArgs(created[0].Args)
		if a.RoleMatch != "" && !mustRe(a.RoleMatch).MatchString(argStr(args, "role")) {
			return "fail", "role does not fit: " + short(argStr(args, "role"), 100), 0
		}
		if len(created) > 1 {
			return "fail", fmt.Sprintf("%d chats created", len(created)), 0
		}
		return "pass", "role: " + short(argStr(args, "role"), 100), 0
	case "used_subagent_for_one_off":
		sub, other := false, ""
		for _, c := range callsOf(o.turns) {
			switch c.Tool {
			case "subagent":
				sub = true
			case "create_chat", "send_to_chat":
				other = c.Tool
			}
		}
		if other != "" {
			return "fail", "used " + other, 0
		}
		if !sub {
			var used []string
			for _, c := range callsOf(o.turns) {
				used = append(used, c.Tool)
			}
			return "fail", "no subagent; tools used: " + strings.Join(used, ", "), 0
		}
		return "pass", "", 0
	case "no_delegation_when_simple":
		for _, c := range callsOf(o.turns) {
			if delegationTools[c.Tool] || c.Tool == "run_pipeline" {
				return "fail", "called " + c.Tool + " " + short(c.Args, 100), 0
			}
		}
		return "pass", "", 0
	}
	return "fail", "unknown assertion", 0
}

// testStatus: per test, fail if any assertion of that test failed, na if all were skipped.
func testStatus(res []AssertRes) map[string]string {
	out := map[string]string{}
	for _, r := range res {
		if r.Test == "info" || r.Test == "" {
			continue
		}
		cur := out[r.Test]
		switch r.Status {
		case "fail":
			out[r.Test] = "fail"
		case "pass":
			if cur != "fail" {
				out[r.Test] = "pass"
			}
		case "skip":
			if cur == "" {
				out[r.Test] = "na"
			}
		}
	}
	return out
}
