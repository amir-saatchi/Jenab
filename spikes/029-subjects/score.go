package main

import (
	"regexp"
	"slices"
	"strings"
)

// score is what one run measured.
type score struct {
	Work       int // work and change messages
	SubjOwn    int // ...whose own turn updated a subject
	SubjCheck  int // ...that updated one on their own or after the app's check
	Nudges     int
	Noise      int // subjects created for questions and follow-ups
	Duplicates int // subjects created for a change to work that had one
	ChangeN    int // changes to work that had a subject
	ChangeUpd  int // ...that updated that subject
	StatusN    int
	StatusOK   int
	WaitN      int // messages that leave work waiting
	WaitOK     int // ...after which its subject shows it: open or blocked, or an open item
	MergeN     int // merge messages
	MergeOK    int // ...after which one subject that isn't dropped covers the work
	Merges     int // merge_subjects calls that worked
	AnswerN    int
	AnswerOK   int
	CallN      int
	CallOK     int
	History    int // search_history and read_messages calls
	GetSubject int
	Updates    int // update_subject and merge_subjects calls that worked
	Alone      int // ...that were the only call of their response
	Subjects   int // subjects at the end, not merged into another
	Requests   int
	Prompt     int
	Output     int
	ReqErrors  int
	Failed     bool // the run ended early

	checks []checkResult
}

// checkResult is one scored check of a message, for the detail table.
type checkResult struct {
	Scenario string
	Msg      int
	What     string // "status", "waiting", "merge", "answer" or "call"
	OK       bool
}

// scoreRun scores one run. A subject belongs to the topics of the
// messages that wrote it, so a subject that grows to cover more work
// (rules v3 and v4) belongs to all of them, and a merged subject passes
// its topics to the one it was merged into.
func scoreRun(r runResult, sc scenarioDef) score {
	var s score
	s.Failed = r.Error != ""
	for _, x := range r.Subjects {
		if x.MergedInto == "" {
			s.Subjects++
		}
	}
	topics := map[string]map[string]bool{} // subject ID -> the topics it was written for
	status := map[string]string{}          // subject ID -> its status so far
	open := map[string]int{}               // subject ID -> its open items so far
	lastTouch := map[string]int{}          // subject ID -> the order of its last event
	addTopic := func(id, t string) {
		if t == "" {
			return
		}
		if topics[id] == nil {
			topics[id] = map[string]bool{}
		}
		topics[id][t] = true
	}
	order := 0
	for _, mr := range r.Messages {
		m := sc.Messages[mr.Index]
		s.Requests += mr.Requests
		s.Prompt += mr.Prompt
		s.Output += mr.Output
		s.ReqErrors += len(mr.Errors)
		if mr.Nudged {
			s.Nudges++
		}
		var own, any bool
		for _, c := range mr.Calls {
			switch c.Tool {
			case "search_history", "read_messages":
				s.History++
			case "get_subject":
				s.GetSubject++
			case "update_subject", "merge_subjects":
				if c.Error {
					continue
				}
				s.Updates++
				if c.Tool == "merge_subjects" {
					s.Merges++
				}
				any = true
				if !c.Nudge {
					own = true
				}
				if c.Alone {
					s.Alone++
				}
			}
		}
		var prior []string // the topic's subjects before this message
		for id, ts := range topics {
			if m.Topic != "" && ts[m.Topic] {
				prior = append(prior, id)
			}
		}
		hadSubject := len(prior) > 0
		for _, e := range r.Events {
			if e.Msg != mr.Index {
				continue
			}
			order++
			for _, o := range e.Merged {
				for t := range topics[o] {
					addTopic(e.ID, t)
				}
				delete(topics, o)
				prior = slices.DeleteFunc(prior, func(id string) bool { return id == o })
			}
			if e.Created {
				switch {
				case m.Kind == kindQuestion || m.Kind == kindFollowup:
					s.Noise++
				case m.Kind == kindChange && hadSubject:
					s.Duplicates++
				}
				for _, t := range m.Merge {
					addTopic(e.ID, t)
				}
			}
			addTopic(e.ID, m.Topic)
			status[e.ID], open[e.ID], lastTouch[e.ID] = e.Status, e.Open, order
		}
		if m.Kind == kindChange && hadSubject && r.Cond >= condTool {
			s.ChangeN++
			if slices.ContainsFunc(r.Events, func(e subjectEvent) bool {
				return e.Msg == mr.Index && !e.Created && slices.Contains(prior, e.ID)
			}) {
				s.ChangeUpd++
			}
		}
		if m.Kind == kindWork || m.Kind == kindChange {
			s.Work++
			if own {
				s.SubjOwn++
			}
			if any {
				s.SubjCheck++
			}
		}
		// The topic's subject, written last. If the topic had subjects
		// before this message, only those count: a new one made for a
		// change is a duplicate, and the old one still has the old status.
		// Otherwise any subject written in this message counts.
		best := ""
		if len(m.Status) > 0 && r.Cond >= condTool {
			cands := slices.Clone(prior)
			if len(cands) == 0 {
				for _, e := range r.Events {
					if e.Msg == mr.Index && !slices.Contains(cands, e.ID) {
						cands = append(cands, e.ID)
					}
				}
			}
			at := -1
			for _, id := range cands {
				if lastTouch[id] > at {
					best, at = id, lastTouch[id]
				}
			}
		}
		// Rule v4 has no status: a goal with several parts has none.
		if len(m.Status) > 0 && r.Cond >= condTool && r.rule() != "v4" {
			ok := best != "" && slices.Contains(m.Status, status[best])
			s.StatusN++
			if ok {
				s.StatusOK++
			}
			s.checks = append(s.checks, checkResult{sc.ID, mr.Index, "status", ok})
		}
		if r.Cond >= condTool && (slices.Contains(m.Status, "open") || slices.Contains(m.Status, "blocked")) {
			ok := best != "" && (status[best] == "open" || status[best] == "blocked" || open[best] > 0)
			s.WaitN++
			if ok {
				s.WaitOK++
			}
			s.checks = append(s.checks, checkResult{sc.ID, mr.Index, "waiting", ok})
		}
		if len(m.Merge) > 0 && r.Cond >= condTool {
			n := 0
			for id, ts := range topics {
				if status[id] == "dropped" {
					continue
				}
				if slices.ContainsFunc(m.Merge, func(t string) bool { return ts[t] }) {
					n++
				}
			}
			ok := n == 1
			s.MergeN++
			if ok {
				s.MergeOK++
			}
			s.checks = append(s.checks, checkResult{sc.ID, mr.Index, "merge", ok})
		}
		if len(m.Answer) > 0 {
			ok := matchAll(mr.Answer, m.Answer)
			s.AnswerN++
			if ok {
				s.AnswerOK++
			}
			s.checks = append(s.checks, checkResult{sc.ID, mr.Index, "answer", ok})
		}
		if len(m.Call) > 0 {
			ok := slices.ContainsFunc(mr.Calls, func(c call) bool {
				if c.Error || (m.CallTool != "" && c.Tool != m.CallTool) || (m.CallTool == "" && !slices.Contains(writeTools, c.Tool)) {
					return false
				}
				return matchAll(c.Args, m.Call)
			})
			s.CallN++
			if ok {
				s.CallOK++
			}
			s.checks = append(s.checks, checkResult{sc.ID, mr.Index, "call", ok})
		}
	}
	// Messages the run never reached fail their checks.
	for i := len(r.Messages); i < len(sc.Messages); i++ {
		m := sc.Messages[i]
		if m.Kind == kindWork || m.Kind == kindChange {
			s.Work++
		}
		fail := func(what string, n *int) {
			*n++
			s.checks = append(s.checks, checkResult{sc.ID, i, what, false})
		}
		if len(m.Status) > 0 && r.Cond >= condTool && r.rule() != "v4" {
			fail("status", &s.StatusN)
		}
		if r.Cond >= condTool && (slices.Contains(m.Status, "open") || slices.Contains(m.Status, "blocked")) {
			fail("waiting", &s.WaitN)
		}
		if len(m.Merge) > 0 && r.Cond >= condTool {
			fail("merge", &s.MergeN)
		}
		if len(m.Answer) > 0 {
			fail("answer", &s.AnswerN)
		}
		if len(m.Call) > 0 {
			fail("call", &s.CallN)
		}
	}
	return s
}

var reCache = map[string]*regexp.Regexp{}

func matchAll(text string, patterns []string) bool {
	text = strings.ToLower(text)
	for _, p := range patterns {
		re, ok := reCache[p]
		if !ok {
			re = regexp.MustCompile("(?i)" + p)
			reCache[p] = re
		}
		if !re.MatchString(text) {
			return false
		}
	}
	return true
}

func (s *score) add(o score) {
	s.Work += o.Work
	s.SubjOwn += o.SubjOwn
	s.SubjCheck += o.SubjCheck
	s.Nudges += o.Nudges
	s.Noise += o.Noise
	s.Duplicates += o.Duplicates
	s.ChangeN += o.ChangeN
	s.ChangeUpd += o.ChangeUpd
	s.StatusN += o.StatusN
	s.StatusOK += o.StatusOK
	s.WaitN += o.WaitN
	s.WaitOK += o.WaitOK
	s.MergeN += o.MergeN
	s.MergeOK += o.MergeOK
	s.Merges += o.Merges
	s.AnswerN += o.AnswerN
	s.AnswerOK += o.AnswerOK
	s.CallN += o.CallN
	s.CallOK += o.CallOK
	s.History += o.History
	s.GetSubject += o.GetSubject
	s.Updates += o.Updates
	s.Alone += o.Alone
	s.Subjects += o.Subjects
	s.Requests += o.Requests
	s.Prompt += o.Prompt
	s.Output += o.Output
	s.ReqErrors += o.ReqErrors
}
