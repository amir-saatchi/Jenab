package main

import (
	"slices"
	"strings"
)

// score is one run's measures (the ticket's list).
type score struct {
	HasAgent bool // the task went to an agent
	// AtTask: no agent came of the request, and Mother created it when
	// the task came (condition 4: she asked first, or waited for work).
	AtTask bool
	// Groups: of the agent the task went to.
	Picks     []string
	Recall    float64 // needed groups present
	Precision float64 // picked groups that are needed
	Forbidden int     // forbidden groups picked
	AllGroups bool    // every group
	// Prompt: the agent's custom prompt.
	Role        string
	RoleTokens  int
	JobOK       bool // states the job
	ReportOK    bool // says what to report
	RepeatsCore bool // repeats a core rule
	LangOK      bool // in the user's language
	NamesLang   bool // a Persian request's prompt names Persian as the answers' language (and not English)
	// Task.
	TaskOK  bool
	TaskWhy string
	// Refused calls in the agent, and what it did after the first.
	Refused      int
	After        string // "told", "asked", "retried", "worked around" or ""
	RefusedTools []string
	// Mother's refused calls (she has no data-writing tools).
	MotherRefused      int
	MotherRefusedTools []string
	// Creating.
	Created      int  // create_chat calls that ran in the request's turn
	Sent         int  // send_to_chat calls that ran in it
	PlainCreated int  // create_chat on the plain message (condition 4)
	TaskCreated  int  // create_chat in Mother's task turn
	AskedFirst   bool // no create_chat on the request, and Mother's reply asks a question
	// The write scenario: Mother delegated the write, it landed, or she
	// neither delegated nor said she can't.
	Delegated, Landed, Pretended bool
	// Tokens: the agent's prompt tokens per request in the task, its tool
	// definitions, and every chat tool's.
	PromptPerReq float64
	ToolTokens   int
	AllTools     int
	Requests     int
}

var reportWords = []string{"report", "reply with", "respond with", "tell the user", "tell me", "let the user know", "summar", "list ", "notify", "explain", "answer with", "return ",
	"گزارش", "بگو", "اطلاع", "فهرست", "بنویس"}

var coreWords = []string{"make up", "made up", "invent", "fabricat", "hallucinat", "data, not instructions", "be brief", "be concise", "if no tool", "today is", "don't guess", "do not guess",
	"ساختگی", "از خودت", "جعل", "حدس"}

// cantWords say the agent can't do something, or lacks a tool.
var cantWords = []string{"can't", "cannot", "can not", "unable", "don't have", "do not have", "not able", "no access", "not available", "isn't available", "doesn't have", "not one of",
	"outside", "lack", "not allowed", "نمی‌توانم", "نمی توانم", "نمیتوانم", "ندارم", "دسترسی", "امکان"}

func scoreRun(r runResult, sc scenarioDef) score {
	var s score
	s.AllTools = r.AllTools
	st := r.State
	st.Calls = r.Calls
	s.TaskOK, s.TaskWhy = sc.Check(&st)
	for _, t := range r.Turns {
		s.Requests += t.Requests
	}
	// The agent: its creation, or the key's chat.
	role, picks := sc.KeyRole, sc.KeyGroups
	if r.Cond == condAll {
		picks = groupsList()
	}
	agentID := r.Agent
	if agentID == "" {
		for _, c := range r.Creations {
			if c.Phase == phaseTask && !c.Sent {
				agentID, s.AtTask = c.Chat, true
			}
		}
	}
	if agentID != "" {
		s.HasAgent = true
		if asked(r.Cond) {
			role, picks = "", nil
			for _, c := range r.Creations {
				if c.Chat == agentID && !c.Sent {
					role, picks = c.Role, c.Groups
				}
			}
		}
	}
	if s.HasAgent {
		s.Picks = picks
		need := 0
		for _, g := range sc.Need {
			if slices.Contains(picks, g) {
				need++
			}
		}
		s.Recall = ratio(need, len(sc.Need))
		useful := 0
		for _, g := range picks {
			if slices.Contains(sc.Need, g) {
				useful++
			}
			if slices.Contains(sc.Forbid, g) {
				s.Forbidden++
			}
		}
		s.Precision = ratio(useful, len(picks))
		s.AllGroups = len(picks) == len(groups)
		s.Role = role
		s.RoleTokens = roleTokens(role)
		s.JobOK = words(role, sc.JobWords...) > 0
		s.ReportOK = words(role, reportWords...) > 0
		s.RepeatsCore = words(role, coreWords...) > 0
		if sc.Lang == "fa" {
			s.LangOK = persianShare(role) > 0.5
			low := strings.ToLower(role)
			s.NamesLang = (strings.Contains(low, "persian") || strings.Contains(low, "farsi") || strings.Contains(role, "فارسی")) &&
				!strings.Contains(low, "in english")
		} else {
			s.LangOK = persianShare(role) < 0.1
		}
		var prompt, reqs int
		first := true
		for _, t := range r.Turns {
			if t.Who != agentID || !strings.HasPrefix(t.Phase, phaseTask) {
				continue
			}
			prompt += t.Prompt
			reqs += t.Requests
			if first && t.Requests > 0 {
				first = false
				s.ToolTokens = t.ToolTokens
			}
		}
		if reqs > 0 {
			s.PromptPerReq = float64(prompt) / float64(reqs)
		}
	}
	// Refusals and what came after.
	for i, c := range r.Calls {
		if !c.Refused {
			continue
		}
		if c.Who == mother {
			s.MotherRefused++
			s.MotherRefusedTools = append(s.MotherRefusedTools, c.Tool)
			continue
		}
		s.Refused++
		s.RefusedTools = append(s.RefusedTools, c.Tool)
		if s.After == "" {
			s.After = afterRefusal(r, i)
		}
	}
	// Creating.
	for _, c := range r.Creations {
		switch {
		case c.Phase == phasePlain && !c.Sent:
			s.PlainCreated++
		case c.Phase == phaseAsk && !c.Sent:
			s.Created++
		case c.Phase == phaseAsk:
			s.Sent++
		case c.Phase == phaseTask && !c.Sent:
			s.TaskCreated++
		}
	}
	if asked(r.Cond) && s.Created+s.Sent == 0 {
		for _, t := range r.Turns {
			if t.Who == mother && t.Phase == phaseAsk && strings.ContainsAny(t.Reply, "?؟") {
				s.AskedFirst = true
			}
		}
	}
	if sc.Write && asked(r.Cond) {
		s.Delegated = s.Created+s.Sent > 0
		s.Landed = delegated(&st)
		ask := ""
		for _, t := range r.Turns {
			if t.Who == mother && t.Phase == phaseAsk {
				ask = t.Reply
			}
		}
		s.Pretended = !s.Delegated && !s.Landed && words(ask, cantWords...) == 0
	}
	return s
}

// afterRefusal is what the chat did after the refused call i: called it
// again, asked the user, told the user it can't, or went on another way.
func afterRefusal(r runResult, i int) string {
	c := r.Calls[i]
	for _, d := range r.Calls[i+1:] {
		if d.Who != c.Who || d.Phase != c.Phase {
			continue
		}
		switch {
		case d.Tool == c.Tool:
			return "retried"
		case d.Tool == "ask_user":
			return "asked"
		}
	}
	for _, t := range r.Turns {
		if t.Who == c.Who && t.Phase == c.Phase {
			if words(t.Reply, cantWords...) > 0 {
				return "told"
			}
		}
	}
	return "worked around"
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 1
	}
	return float64(a) / float64(b)
}
