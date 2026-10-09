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
	AnswerN    int
	AnswerOK   int
	CallN      int
	CallOK     int
	History    int // search_history and read_messages calls
	GetSubject int
	Updates    int // update_subject calls that worked
	Alone      int // ...that were the only call of their response
	Subjects   int // subjects at the end
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
	What     string // "status", "answer" or "call"
	OK       bool
}

func scoreRun(r runResult, sc scenarioDef) score {
	var s score
	s.Failed = r.Error != ""
	s.Subjects = len(r.Subjects)
	topicOf := map[string]string{} // subject ID -> the topic it was first written for
	status := map[string]string{}  // subject ID -> its status so far
	lastTouch := map[string]int{}  // subject ID -> the order of its last event
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
			case "update_subject":
				if c.Error {
					continue
				}
				s.Updates++
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
		for id, t := range topicOf {
			if t == m.Topic && m.Topic != "" {
				prior = append(prior, id)
			}
		}
		hadSubject := len(prior) > 0
		for _, e := range r.Events {
			if e.Msg != mr.Index {
				continue
			}
			order++
			if e.Created {
				switch {
				case m.Kind == kindQuestion || m.Kind == kindFollowup:
					s.Noise++
				case m.Kind == kindChange && hadSubject:
					s.Duplicates++
				}
				if m.Topic != "" {
					topicOf[e.ID] = m.Topic
				}
			} else if _, ok := topicOf[e.ID]; !ok && m.Topic != "" {
				topicOf[e.ID] = m.Topic
			}
			status[e.ID], lastTouch[e.ID] = e.Status, order
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
		if len(m.Status) > 0 && r.Cond >= condTool {
			// The topic's subject, written last. If the topic had subjects
			// before this message, only those count: a new one made for a
			// change is a duplicate, and the old one still has the old
			// status. Otherwise any subject written in this message counts.
			cands := prior
			if len(cands) == 0 {
				for _, e := range r.Events {
					if e.Msg == mr.Index && !slices.Contains(cands, e.ID) {
						cands = append(cands, e.ID)
					}
				}
			}
			best, at := "", -1
			for _, id := range cands {
				if lastTouch[id] > at {
					best, at = id, lastTouch[id]
				}
			}
			ok := best != "" && slices.Contains(m.Status, status[best])
			s.StatusN++
			if ok {
				s.StatusOK++
			}
			s.checks = append(s.checks, checkResult{sc.ID, mr.Index, "status", ok})
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
		if len(m.Status) > 0 && r.Cond >= condTool {
			s.StatusN++
			s.checks = append(s.checks, checkResult{sc.ID, i, "status", false})
		}
		if len(m.Answer) > 0 {
			s.AnswerN++
			s.checks = append(s.checks, checkResult{sc.ID, i, "answer", false})
		}
		if len(m.Call) > 0 {
			s.CallN++
			s.checks = append(s.checks, checkResult{sc.ID, i, "call", false})
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
