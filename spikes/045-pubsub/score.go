package main

import (
	"regexp"
	"slices"
	"strings"
)

// score is one run's measures (the ticket's list).
type score struct {
	// Subscribe (condition 3): the subscriptions agents made.
	AgentSubs  []subRec
	NeedMade   int // needed subscriptions made, of len(needs())
	NeedRight  int // of them, with the right filter
	Unneeded   int // neither needed nor allowed
	Allowed    int // reasonable but not needed
	AtCreation int // made in the agent's first turn
	Later      int
	Unsubs     int
	Dups       int
	// Delivery.
	Delivered int // notices given
	Own       int // skipped: the chat caused the event
	// Publish (conditions 4 and 5).
	Pubs         int  // reports published (not refused)
	Published    bool // in the need turn
	RightChannel bool // on alerts
	TitleOK      bool // the need turn's title says what happened
	Noise        int  // reports published in routine turns
	NoiseTurns   int  // routine turns with a report
	RoutineTurns int
	Extra        int // reports in turns that changed or found something no subscriber needs
	Refused      int // publish calls to a channel that doesn't exist
	ReadReport   int // read_report calls
	// Use: at the subscriber's next turn after the event.
	DocsVolume, Reviewer, Reporter, DocsEth, DocsEnd bool
	// Cost.
	Prompt, Notice, Block5, Requests int
	// Calls outside the chat's groups.
	GroupRefused int
}

// needs are the needed subscriptions: one per agent's Need.
func needs() int {
	n := 0
	for _, a := range agents {
		n += len(a.Need)
	}
	return n
}

var (
	dropRe  = regexp.MustCompile(`(?i)6[.,]4\d?\s*%|66[,.]?700|[-−–]\s?6[.,]\d|-6\s?%|−6\s?%`)
	titleRe = regexp.MustCompile(`(?i)(btc|bitcoin)`)
	moveRe  = regexp.MustCompile(`(?i)\d+([.,]\d+)?\s*%|drop|fell|fall|down|plunge|crash|slump|tumble|move`)
)

func scoreRun(r runResult) score {
	var s score
	first := map[string]int{}
	for i, m := range script {
		if m.First {
			first[m.To] = i + 1
		}
	}
	// Subscribe.
	for _, x := range r.Subs {
		if x.By != "agent" {
			continue
		}
		s.AgentSubs = append(s.AgentSubs, x)
		if x.Ended {
			s.Unsubs++
		}
		if x.Msg == first[x.Agent] {
			s.AtCreation++
		} else {
			s.Later++
		}
	}
	for _, a := range agents {
		var mine []subRec
		for _, x := range s.AgentSubs {
			if x.Agent == a.Key {
				mine = append(mine, x)
			}
		}
		used := map[string]bool{}
		for _, need := range a.Need {
			made, right := false, false
			for _, x := range mine {
				if m, rt := matchesKey(x.sub, need); m {
					made = true
					used[x.ID] = true
					right = right || rt
				}
			}
			if made {
				s.NeedMade++
			}
			if right {
				s.NeedRight++
			}
		}
		for _, x := range mine {
			switch {
			case used[x.ID]:
			case allowedSub(a, x.sub):
				s.Allowed++
			default:
				s.Unneeded++
			}
		}
	}
	s.Dups = r.Dups
	s.Delivered, s.Own = len(r.Deliveries), r.Own
	// Publish.
	pubTurns := map[int]int{}
	for _, p := range r.Pubs {
		if p.Refused {
			s.Refused++
			continue
		}
		s.Pubs++
		pubTurns[p.Msg]++
		if p.Msg < 1 || p.Msg > len(script) {
			continue
		}
		m := script[p.Msg-1]
		switch {
		case m.Pub == pubNeed && p.Agent == m.To:
			s.Published = true
			if p.Channel == "alerts" {
				s.RightChannel = true
			}
			if titleRe.MatchString(p.Title) && moveRe.MatchString(p.Title) {
				s.TitleOK = true
			}
		case m.Pub == pubRoutine:
			s.Noise++
		default:
			s.Extra++
		}
	}
	for i, m := range script {
		if m.Pub == pubRoutine {
			s.RoutineTurns++
			if pubTurns[i+1] > 0 {
				s.NoiseTurns++
			}
		}
	}
	for _, c := range r.Calls {
		if c.Tool == "read_report" {
			s.ReadReport++
		}
		if c.Refused {
			s.GroupRefused++
		}
	}
	// Use.
	for _, t := range r.Turns {
		s.Prompt += t.Prompt
		s.Notice += t.NoticeTokens
		s.Block5 += t.Block5
		s.Requests += t.Requests
		low := strings.ToLower(t.Docs)
		switch t.Msg {
		case useDocsVolume:
			s.DocsVolume = strings.Contains(low, "volume")
		case useReviewer:
			// Only the notice has r_101 and the 503: the card doesn't show runs.
			s.Reviewer = strings.Contains(t.Reply, "r_101") || strings.Contains(t.Reply, "503") || slices.ContainsFunc(r.Calls, func(c call) bool {
				return c.Msg == useReviewer && c.Tool == "run_status" && strings.Contains(c.Args, "r_101")
			})
		case useReporter:
			s.Reporter = dropRe.MatchString(t.Draft) || dropRe.MatchString(t.Reply)
		case useDocsEth:
			s.DocsEth = strings.Contains(low, "eth_prices")
		}
	}
	if n := len(r.Turns); n > 0 {
		low := strings.ToLower(r.Turns[n-1].Docs)
		s.DocsEnd = strings.Contains(low, "volume") && strings.Contains(low, "eth_prices")
	}
	return s
}
