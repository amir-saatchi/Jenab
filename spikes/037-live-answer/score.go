package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// norm lowercases s and writes Persian and Arabic digits as ASCII, so a
// fact's regex finds "۵۲" as 52.
func norm(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= '۰' && r <= '۹':
			return '0' + (r - '۰')
		case r >= '٠' && r <= '٩':
			return '0' + (r - '٠')
		case r == '٬':
			return ','
		case r == '٫':
			return '.'
		case unicode.IsSpace(r):
			return ' '
		case unicode.Is(unicode.Pd, r):
			return '-'
		}
		return unicode.ToLower(r)
	}, s)
}

func has(text, re string) bool {
	return regexp.MustCompile("(?is)" + re).MatchString(norm(text))
}

func cover(text string, facts []string) (n int) {
	for _, f := range facts {
		if has(text, f) {
			n++
		}
	}
	return n
}

func frac(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return float64(n) / float64(d)
}

// words are the longer words of s, for how much of an earlier answer a
// later one repeats.
func words(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(norm(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len([]rune(w)) >= 5 {
			out[w] = true
		}
	}
	return out
}

func overlap(prev, cur string) float64 {
	p, c := words(prev), words(cur)
	n := 0
	for w := range p {
		if c[w] {
			n++
		}
	}
	return frac(n, len(p))
}

func squash(s string) string { return strings.Join(strings.Fields(s), " ") }

// --- Part A ---

// revA is one finish turn: what its answer kept and how it was written.
type revA struct {
	Turn       int     `json:"turn"`
	Tasks      int     `json:"tasks"`
	Class      string  `json:"class"`       // whole, rest, appended, none
	NewCov     float64 `json:"new_cov"`     // the arriving tasks' facts
	EarlierCov float64 `json:"earlier_cov"` // facts of tasks that arrived before
	EarlierN   int     `json:"earlier_n"`
	Dropped    int     `json:"dropped"` // earlier facts the previous revision had and this one lacks
	Overlap    float64 `json:"overlap"` // the previous answer's words this one repeats
	Output     int     `json:"output"`
	Requests   int     `json:"requests"`
	Prompt     int     `json:"prompt"`
	CacheRead  int     `json:"cache_read"`
	Below      bool    `json:"below,omitempty"` // a user message came since: the live answer goes below
	Chars      int     `json:"chars"`
}

type scoreA struct {
	Started    int // background tasks started
	Foreground int // subagent calls without background
	Revs       []revA
	Slots      int     // answers the user sees for the request at the end
	FinalView  float64 // facts of every arrived task, in the whole view
	FinalLast  float64 // ...in the last answer alone
	Union      float64 // ...in all answers together
	AllFacts   int
	Notes      []string
	// Edge cases; nil when the test doesn't have it or it didn't happen.
	FailedSaid   *bool // the failure is in the finish turn's answer
	FailedInView *bool // ...and in the view at the end
	EmptyClass   string
	TogetherBoth *bool // both tasks of the shared finish turn are in its answer
	LateSat      *bool // the answer to the late message takes Saturdays into account
	LateFinalSat *bool // ...and so does the last finish turn
	Persian      float64
	PersianOK    *bool
}

func pb(b bool) *bool { return &b }

func taskFacts(t taskRec, td testDef) []string {
	var out []string
	for _, n := range t.Rules {
		for _, r := range td.Tasks {
			if r.Name == n && !r.Fail {
				out = append(out, r.Facts...)
			}
		}
	}
	return out
}

func scoreRunA(r runResult, td testDef) scoreA {
	var s scoreA
	for _, t := range r.Turns {
		for _, c := range t.Calls {
			if c.Tool != "subagent" {
				continue
			}
			var a struct {
				Background bool `json:"background"`
			}
			_ = json.Unmarshal([]byte(c.Args), &a)
			if a.Background {
				s.Started++
			} else {
				s.Foreground++
			}
		}
	}
	taskByID := map[string]taskRec{}
	for _, t := range r.Tasks {
		taskByID[t.ID] = t
	}
	// The user's view: one slot per user turn; a finish turn replaces its
	// task's slot in the live condition while no user message came since,
	// else it goes below.
	type slot struct {
		turn int
		text string
	}
	var view []slot
	lastUser := 0
	var arrived []string // facts of tasks that arrived so far
	prev := ""           // the previous revision of the request's answer
	prevFacts := map[string]bool{}
	var answers []string
	for _, t := range r.Turns {
		if t.Kind == "user" {
			lastUser = t.N
			if t.Answer != "" {
				view = append(view, slot{t.N, t.Answer})
				answers = append(answers, t.Answer)
			}
			if t.Msg == 0 {
				prev = t.Answer
			}
			continue
		}
		var nf []string
		started := 0
		for _, id := range t.Tasks {
			nf = append(nf, taskFacts(taskByID[id], td)...)
			started = max(started, taskByID[id].Turn)
		}
		rv := revA{Turn: t.N, Tasks: len(t.Tasks), Chars: len([]rune(t.Answer)), EarlierN: len(arrived), Below: lastUser != started}
		for _, q := range t.Requests {
			rv.Output += q.Output
			rv.Prompt += q.Prompt
			rv.CacheRead += q.CacheRead
			rv.Requests++
		}
		rv.NewCov = frac(cover(t.Answer, nf), len(nf))
		rv.EarlierCov = frac(cover(t.Answer, arrived), len(arrived))
		for f := range prevFacts {
			if !has(t.Answer, f) {
				rv.Dropped++
			}
		}
		rv.Overlap = overlap(prev, t.Answer)
		pa, ca := squash(norm(prev)), squash(norm(t.Answer))
		switch {
		case strings.TrimSpace(t.Answer) == "":
			rv.Class = "none"
		case pa != "" && strings.HasPrefix(ca, pa) && !strings.HasPrefix(strings.TrimSpace(ca[len(pa):]), "|"):
			rv.Class = "appended"
		case len(arrived) > 0 && rv.EarlierCov >= 0.6, len(arrived) == 0 && rv.Overlap >= 0.4:
			rv.Class = "whole"
		default:
			rv.Class = "rest"
		}
		s.Revs = append(s.Revs, rv)
		if t.Answer != "" {
			answers = append(answers, t.Answer)
			i := slices.IndexFunc(view, func(v slot) bool { return v.turn == started })
			if r.Cond == condLive && !rv.Below && i >= 0 {
				view[i].text = t.Answer
			} else {
				view = append(view, slot{t.N, t.Answer})
			}
			if !rv.Below {
				prev = t.Answer
			}
		}
		arrived = append(arrived, nf...)
		prevFacts = map[string]bool{}
		for _, f := range arrived {
			if has(prev, f) {
				prevFacts[f] = true
			}
		}
	}
	var all []string
	for _, v := range view {
		all = append(all, v.text)
	}
	s.Slots = len(view)
	s.AllFacts = len(arrived)
	s.FinalView = frac(cover(strings.Join(all, "\n\n"), arrived), len(arrived))
	if len(view) > 0 {
		s.FinalLast = frac(cover(view[len(view)-1].text, arrived), len(arrived))
	}
	s.Union = frac(cover(strings.Join(answers, "\n\n"), arrived), len(arrived))
	if s.Started == 0 {
		s.Notes = append(s.Notes, "no background task")
	}

	finishOf := func(rule string) *turnRec {
		for i, t := range r.Turns {
			for _, id := range t.Tasks {
				if slices.Contains(taskByID[id].Rules, rule) {
					return &r.Turns[i]
				}
			}
		}
		return nil
	}
	revOf := func(n int) *revA {
		for i := range s.Revs {
			if s.Revs[i].Turn == n {
				return &s.Revs[i]
			}
		}
		return nil
	}
	switch td.Kind {
	case "failed":
		if t := finishOf("kleinanzeigen"); t != nil {
			said := `kleinanzeigen[^\n]*(fail|block|403|couldn.?t|could not|unable|no (listings|results)|not (be )?(read|access)|error|unavailable)|(fail|block|403|couldn.?t|could not|unable|error)[^\n]*kleinanzeigen`
			s.FailedSaid = pb(has(t.Answer, said))
			s.FailedInView = pb(has(strings.Join(all, "\n"), said))
		}
	case "empty":
		if t := finishOf("spiegel"); t != nil {
			if rv := revOf(t.N); rv != nil {
				s.EmptyClass = rv.Class
			}
		}
	case "together":
		for _, t := range r.Turns {
			if t.Kind != "finish" {
				continue
			}
			var rules []string
			for _, id := range t.Tasks {
				rules = append(rules, taskByID[id].Rules...)
			}
			if slices.Contains(rules, "ticketmaster") && slices.Contains(rules, "viagogo") {
				s.TogetherBoth = pb(has(t.Answer, `104[.,]50`) && has(t.Answer, `178`))
			}
		}
	case "late":
		sat := `saturday|samstag|\bsat\b`
		lastFinish, after := "", false
		for _, t := range r.Turns {
			if t.Kind == "user" && t.Msg == 1 {
				s.LateSat, after = pb(has(t.Answer, sat)), true
			}
			if t.Kind == "finish" && after && t.Answer != "" {
				lastFinish = t.Answer
			}
		}
		if lastFinish != "" {
			s.LateFinalSat = pb(has(lastFinish, sat))
		}
	}
	if td.Persian {
		ratio, n := 1.0, 0
		for _, t := range r.Turns {
			if t.Answer == "" {
				continue
			}
			v := persianShare(t.Answer)
			ratio = min(ratio, v)
			n++
		}
		if n > 0 {
			s.Persian = ratio
			s.PersianOK = pb(ratio >= 0.5)
		}
	}
	return s
}

// persianShare is the share of Persian letters among the letters of an
// answer's lines outside tables, as company names in a table stay Latin.
func persianShare(s string) float64 {
	fa, all := 0, 0
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "|") {
			continue
		}
		for _, r := range l {
			if !unicode.IsLetter(r) {
				continue
			}
			all++
			if unicode.Is(unicode.Arabic, r) {
				fa++
			}
		}
	}
	if all == 0 {
		return 1
	}
	return frac(fa, all)
}

// --- Part B ---

type followB struct {
	Msg   int    `json:"msg"`
	Kind  string `json:"kind"`
	Right bool   `json:"right"`
	Read  bool   `json:"read"` // the turn read the history or a ref
	Calls int    `json:"calls"`
	Note  string `json:"note,omitempty"`
}

type scoreB struct {
	Requests    int
	Prompt      int // all requests' prompt tokens
	PromptMax   int
	CacheRead   int
	EstHistory  int // the earlier turns in all requests, estimated
	Cuts        int
	Follow      []followB
	Reads       map[string]int
	Repeated    []string
	WrongNames  []string
	LateCalls   int // tool calls in user turns from message 7 on
	LateTurns   int
	FakeCalls   []string
	WrongClaims []string
	TraceTok    int // the trace in the last request
	TraceTurns  int // earlier turns in that request
}

var readTools = []string{"search_history", "read_messages", "read_ref", "search_ref"}

var reFake = regexp.MustCompile(`(?m)^\s*[\[(]?\s*(fetch_page|web_search|create_table|insert_rows|read_table|subagent|task_status|read_ref|search_ref|read_messages|search_history|full steps)\b|→\s*(ref|task)\b`)

func argOf(args, k string) string {
	var m map[string]any
	_ = json.Unmarshal([]byte(args), &m)
	s, _ := m[k].(string)
	return s
}

func scoreRunB(r runResult, td testDef) scoreB {
	s := scoreB{Reads: map[string]int{}}
	first := 0
	for _, t := range r.Turns {
		for _, q := range t.Requests {
			s.Requests++
			s.Prompt += q.Prompt
			s.PromptMax = max(s.PromptMax, q.Prompt)
			s.CacheRead += q.CacheRead
			s.EstHistory += q.EstHistory
			if q.First > first {
				if first > 0 {
					s.Cuts++
				}
				first = q.First
			}
		}
	}
	if n := len(r.Turns); n > 0 {
		last := r.Turns[n-1]
		if k := len(last.Requests); k > 0 {
			s.TraceTok = last.Requests[0].EstTrace
		}
		s.TraceTurns = n - 1
	}

	fetched := map[string]int{} // URL: the turn that fetched it first
	searched := map[string]int{}
	ruleTurn := map[string]int{}
	for _, t := range r.Tasks {
		for _, rl := range t.Rules {
			if at, ok := ruleTurn[rl]; ok && at < t.Turn {
				s.Repeated = append(s.Repeated, fmt.Sprintf("t%d subagent %s again", t.Turn, rl))
			} else if !ok {
				ruleTurn[rl] = t.Turn
			}
		}
	}
	turn1 := ""
	for _, t := range r.Turns {
		if t.Kind == "user" && t.Msg == 0 {
			turn1 = t.Answer
		}
		for _, c := range t.Calls {
			switch c.Tool {
			case "fetch_page":
				u := normURL(argOf(c.Args, "url"))
				if at, ok := fetched[u]; ok && at < t.N && !c.Error {
					s.Repeated = append(s.Repeated, fmt.Sprintf("t%d fetch_page %s again", t.N, u))
				} else if !ok && !c.Error {
					fetched[u] = t.N
				}
			case "web_search":
				q := squash(norm(argOf(c.Args, "query")))
				if at, ok := searched[q]; ok && at < t.N {
					s.Repeated = append(s.Repeated, fmt.Sprintf("t%d web_search %q again", t.N, q))
				} else if !ok {
					searched[q] = t.N
				}
			case "insert_rows", "read_table", "task_status", "read_ref", "search_ref":
				if c.Error {
					s.WrongNames = append(s.WrongNames, fmt.Sprintf("t%d %s %s: %s", t.N, c.Tool, clip(c.Args, 80), clip(c.Result, 120)))
				}
			}
			if slices.Contains(readTools, c.Tool) {
				s.Reads[c.Tool]++
			}
		}
		if t.Kind == "user" && t.Msg >= 7 {
			s.LateTurns++
			s.LateCalls += len(t.Calls)
		}
		for _, x := range t.Texts {
			for _, m := range reFake.FindAllString(x, -1) {
				s.FakeCalls = append(s.FakeCalls, fmt.Sprintf("t%d %s", t.N, strings.TrimSpace(m)))
			}
		}
	}

	for _, t := range r.Turns {
		if t.Kind != "user" {
			continue
		}
		ck, ok := td.Checks[t.Msg]
		if !ok {
			continue
		}
		f := followB{Msg: t.Msg, Kind: ck.Kind, Calls: len(t.Calls)}
		for _, c := range t.Calls {
			if slices.Contains(readTools, c.Tool) || c.Tool == "read_table" || c.Tool == "fetch_page" {
				f.Read = true
			}
		}
		ans := t.Answer
		all := func(text string, res []string) bool {
			for _, re := range res {
				if !has(text, re) {
					return false
				}
			}
			return true
		}
		anyOf := func(text string, res []string) bool {
			if len(res) == 0 {
				return true
			}
			for _, re := range res {
				if has(text, re) {
					return true
				}
			}
			return false
		}
		switch ck.Kind {
		case "table":
			text := ans
			for _, c := range t.Calls {
				if c.Tool == "insert_rows" && !c.Error {
					text += "\n" + c.Args
				}
			}
			f.Right = all(text, ck.Answer)
		case "task":
			f.Right = anyOf(ans, ck.Any) && !has(ans, `\b(is|has) (finished|done|complete)`)
			var want string
			for _, tk := range r.Tasks {
				if slices.Contains(tk.Rules, ck.Rule) {
					want = tk.ID
				}
			}
			for _, c := range t.Calls {
				if c.Tool == "task_status" {
					if got := argOf(c.Args, "id"); want != "" && strings.TrimSpace(got) != want {
						f.Note = fmt.Sprintf("task_status %s, the task is %s", got, want)
					}
				}
			}
			if want == "" {
				f.Note = "no " + ck.Rule + " task was started"
			}
		case "more":
			// New pages fetched in the turn, or in the finish turns of
			// tasks it started, before the next message.
			n := 0
			for _, u := range r.Turns {
				if u.N < t.N || u.N > t.N && u.Kind == "user" {
					continue
				}
				for _, c := range u.Calls {
					if c.Tool == "fetch_page" && !c.Error {
						if at := fetched[normURL(argOf(c.Args, "url"))]; at == u.N {
							n++
						}
					}
				}
			}
			f.Right = n >= ck.Pages
			f.Note = fmt.Sprintf("%d new pages", n)
		case "fact":
			var named []string
			for _, nm := range ck.Names {
				if has(turn1, regexp.QuoteMeta(nm)) {
					named = append(named, nm)
				}
			}
			k := 0
			for _, nm := range named {
				if has(ans, regexp.QuoteMeta(nm)) {
					k++
				}
			}
			f.Right = k >= min(2, len(named)) && len(named) > 0
			f.Note = fmt.Sprintf("%d of %v", k, named)
		case "claims":
			var wrong []string
			for _, re := range ck.Not {
				if has(ans, re) {
					wrong = append(wrong, re)
				}
			}
			s.WrongClaims = append(s.WrongClaims, wrong...)
			f.Right = all(ans, ck.Answer) && len(wrong) == 0
			var miss []string
			for _, re := range ck.Answer {
				if !has(ans, re) {
					miss = append(miss, re)
				}
			}
			if len(miss) > 0 || len(wrong) > 0 {
				f.Note = fmt.Sprintf("missing %v wrong %v", miss, wrong)
			}
		default: // fact-like: detail, source, ref
			f.Right = all(ans, ck.Answer) && anyOf(ans, ck.Any)
		}
		s.Follow = append(s.Follow, f)
	}
	return s
}

// oneLine is a run's log line.
func oneLine(r runResult, td testDef) string {
	if td.Part == "A" {
		s := scoreRunA(r, td)
		cls := map[string]int{}
		for _, v := range s.Revs {
			cls[v.Class]++
		}
		return fmt.Sprintf("bg %d fg %d finish %v view %.0f%% slots %d", s.Started, s.Foreground, cls, 100*s.FinalView, s.Slots)
	}
	s := scoreRunB(r, td)
	ok := 0
	for _, f := range s.Follow {
		if f.Right {
			ok++
		}
	}
	return fmt.Sprintf("follow-ups %d/%d prompt %d (max %d) cuts %d reads %v repeated %d", ok, len(s.Follow), s.Prompt, s.PromptMax, s.Cuts, s.Reads, len(s.Repeated))
}
