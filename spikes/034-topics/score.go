package main

import (
	"regexp"
	"slices"
	"strings"
)

// score is what one run measured.
type score struct {
	// Proposals (condition 3).
	NewN, NewOK       int // non-main messages whose label had no topic yet, and those a proposal moved into a new topic
	ReturnN, ReturnOK int // messages whose label had a topic the user wasn't in, and those a proposal moved there
	Noise             int // proposals for main messages, or inside the label's own topic
	MergeProps        int // proposals on the merge message (declined, but not noise)
	Dups              int // a new topic proposed for a label that had one
	Wrong             int // another existing topic proposed
	Misses            int // non-main messages answered outside their topic
	PropSends         int // sends that only proposed (the proposal was accepted)
	PropReq           int
	PropPrompt        int
	Topics            int // live topics at the end

	// Subjects (condition 4).
	SubjNoise, SubjDups, Subjects int

	// Payoff.
	FollowN, FollowOK int // follow-ups after cuts
	CrossN, CrossOK   int // cross-topic questions in the main thread
	CallN, CallOK     int
	ReachN, ReachOK   int // decisions made in topics, asked in the main thread
	ReachRead         int // ...whose turn read history

	History, HistoryCross int // search_history and read_messages calls, and those that reach another thread

	NotesWrites, Alone int // update_notes / merge_topics / update_subject / merge_subjects calls that worked, and those alone in their response

	WaitN, WaitOK   int
	MergeN, MergeOK int
	MemN, MemOK     int
	Conflicts       int // update_memory refused for a stale revision
	ConflictFixed   int // ...followed by a saved update_memory in the same message
	ParallelRan     int // parallel steps sent at the same time

	Requests, Prompt, Output, CacheRead int
	SharedN, Shared, System             int // topics' first sends: shared prefix and system tokens
	ReqErrors                           int
	Failed                              bool

	checks []checkResult
}

// checkResult is one scored check, for the detail table.
type checkResult struct {
	Scenario string
	Msg      int
	What     string
	OK       bool
}

func scoreRun(r runResult, sc scenarioDef) score {
	var s score
	s.Failed = r.Error != ""
	add := func(i int, what string, ok bool, n, okN *int) {
		*n++
		if ok {
			*okN++
		}
		s.checks = append(s.checks, checkResult{sc.ID, i, what, ok})
	}
	byMsg := map[int][]proposal{}
	for _, p := range r.Proposals {
		byMsg[p.Msg] = append(byMsg[p.Msg], p)
		switch p.Verdict {
		case "noise":
			if p.Msg < len(sc.Messages) && sc.Messages[p.Msg].Kind == kindMerge {
				s.MergeProps++
			} else {
				s.Noise++
			}
		case "duplicate":
			s.Dups++
		case "wrong":
			s.Wrong++
		}
	}
	for _, t := range r.Threads {
		if t.ID != mainThread && t.MergedInto == "" {
			s.Topics++
		}
	}
	for _, x := range r.Subjects {
		if x.MergedInto == "" {
			s.Subjects++
		}
	}
	s.SubjNoise, s.SubjDups = subjectNoise(r, sc)

	seen := map[int]bool{}
	var earlier []call // the calls of the messages before, for Done
	for _, mr := range r.Messages {
		seen[mr.Index] = true
		m := sc.Messages[mr.Index]
		for _, sd := range mr.Sends {
			s.Requests += sd.Requests
			s.Prompt += sd.Prompt
			s.Output += sd.Output
			s.CacheRead += sd.CacheRead
			if sd.Moved != "" {
				s.PropSends++
				s.PropReq += sd.Requests
				s.PropPrompt += sd.Prompt
			}
			if sd.Shared > 0 {
				s.SharedN++
				s.Shared += sd.Shared
				s.System += sd.System
			}
		}
		s.ReqErrors += len(mr.Errors)
		if mr.Parallel && m.Parallel {
			s.ParallelRan++
		}
		read := false
		conflict := false
		for _, c := range mr.Calls {
			switch c.Tool {
			case "search_history", "read_messages":
				s.History++
				read = true
				if c.Cross && topics(r.Cond) { // conditions 1 and 4 have one chat
					s.HistoryCross++
				}
			case "update_memory":
				if c.Error && strings.Contains(c.Result, "changed since") {
					s.Conflicts++
					conflict = true
				} else if !c.Error && conflict {
					s.ConflictFixed++
					conflict = false
				}
			}
			if notesWrite(c.Tool) && !c.Error {
				s.NotesWrites++
				if c.Alone {
					s.Alone++
				}
			}
		}

		if r.Cond == condPropose && m.Label != mainLabel && len(mr.Sends) > 0 {
			first := mr.Sends[0]
			switch {
			case mr.Had == "":
				ok := slices.ContainsFunc(byMsg[mr.Index], func(p proposal) bool { return p.Accepted && p.Verdict == "new" })
				add(mr.Index, "new topic", ok, &s.NewN, &s.NewOK)
			case first.Thread != mr.Had:
				add(mr.Index, "return", first.Moved == mr.Had, &s.ReturnN, &s.ReturnOK)
			}
			if first.Moved == "" && (mr.Had == "" || first.Thread != mr.Had) {
				s.Misses++
			}
		}
		if len(m.Answer) > 0 {
			ok := matchAll(mr.Answer, m.Answer)
			switch {
			case m.Kind == kindFollowup:
				add(mr.Index, "follow-up", ok, &s.FollowN, &s.FollowOK)
			default:
				add(mr.Index, "cross", ok, &s.CrossN, &s.CrossOK)
			}
			if m.Decision {
				add(mr.Index, "reach", ok, &s.ReachN, &s.ReachOK)
				if read {
					s.ReachRead++
				}
			}
		}
		if len(m.Call) > 0 {
			ok := slices.ContainsFunc(mr.Calls, func(c call) bool {
				if c.Error || (m.CallTool != "" && c.Tool != m.CallTool) || (m.CallTool == "" && !slices.Contains(writeTools, c.Tool)) {
					return false
				}
				return matchAll(c.Args, m.Call)
			})
			if !ok && len(m.Done) > 0 {
				ok = slices.ContainsFunc(earlier, func(c call) bool {
					return !c.Error && slices.Contains(writeTools, c.Tool) && matchAll(c.Args, m.Done)
				})
			}
			add(mr.Index, "call", ok, &s.CallN, &s.CallOK)
		}
		earlier = append(earlier, mr.Calls...)
		if m.Waiting && r.Cond != condBaseline {
			add(mr.Index, "waiting", mr.Open > 0, &s.WaitN, &s.WaitOK)
		}
		if len(m.Merge) > 0 && r.Cond != condBaseline {
			add(mr.Index, "merge", mr.Joined != nil && *mr.Joined, &s.MergeN, &s.MergeOK)
		}
		if len(m.Memory) > 0 {
			add(mr.Index, "memory", matchAll(mr.Memory, m.Memory), &s.MemN, &s.MemOK)
		}
	}
	// Messages the run never reached fail their checks.
	for i, m := range sc.Messages {
		if seen[i] {
			continue
		}
		fail := func(what string, n *int) {
			*n++
			s.checks = append(s.checks, checkResult{sc.ID, i, what, false})
		}
		if len(m.Answer) > 0 {
			if m.Kind == kindFollowup {
				fail("follow-up", &s.FollowN)
			} else {
				fail("cross", &s.CrossN)
			}
			if m.Decision {
				fail("reach", &s.ReachN)
			}
		}
		if len(m.Call) > 0 {
			fail("call", &s.CallN)
		}
		if m.Waiting && r.Cond != condBaseline {
			fail("waiting", &s.WaitN)
		}
		if len(m.Merge) > 0 && r.Cond != condBaseline {
			fail("merge", &s.MergeN)
		}
		if len(m.Memory) > 0 {
			fail("memory", &s.MemN)
		}
	}
	return s
}

// subjectNoise counts, in condition 4, subjects created for main messages
// (noise) and subjects created for a label that already had one
// (duplicates).
func subjectNoise(r runResult, sc scenarioDef) (noise, dups int) {
	held := map[string][]string{} // subject -> labels
	for _, e := range r.Events {
		for _, o := range e.Merged {
			for _, l := range held[o] {
				if !slices.Contains(held[e.ID], l) {
					held[e.ID] = append(held[e.ID], l)
				}
			}
			delete(held, o)
		}
		if e.Msg >= len(sc.Messages) {
			continue
		}
		l := sc.Messages[e.Msg].Label
		if e.Created {
			if l == mainLabel {
				noise++
			} else {
				for id, ls := range held {
					if id != e.ID && slices.Contains(ls, l) {
						dups++
						break
					}
				}
			}
		}
		if l != mainLabel && !slices.Contains(held[e.ID], l) {
			held[e.ID] = append(held[e.ID], l)
		}
	}
	return noise, dups
}

var reCache = map[string]*regexp.Regexp{}

func matchAll(text string, patterns []string) bool {
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
