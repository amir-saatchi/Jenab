package main

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/amir-saatchi/jenab/internal/tool"
)

// subject is one piece of the chat's work, kept across turns (SPIKE-029).
type subject struct {
	ID      string   `json:"id"`
	Subject string   `json:"subject"`
	Status  string   `json:"status"`
	Outcome string   `json:"outcome"`
	Open    []string `json:"open,omitempty"`
	Source  []string `json:"source"` // the assistant messages that wrote it
}

var statuses = []string{"open", "in_progress", "blocked", "done", "dropped"}

// subjectEvent is one successful update_subject call, for scoring.
type subjectEvent struct {
	Msg     int    `json:"msg"`   // the scripted message's index
	Nudge   bool   `json:"nudge"` // made in the turn after the app's check
	ID      string `json:"id"`
	Created bool   `json:"created"`
	Status  string `json:"status"`
}

// subjects are a chat's subjects. cur and nudge say which scripted
// message the harness is playing, so each call is scored against it.
type subjects struct {
	mu     sync.Mutex
	list   []*subject
	events []subjectEvent
	cur    int
	nudge  bool
	rule   string // "v1" (round 1) or "v2" (round 2)
}

func (s *subjects) at(msg int, nudge bool) {
	s.mu.Lock()
	s.cur, s.nudge = msg, nudge
	s.mu.Unlock()
}

func (s *subjects) find(id string) *subject {
	for _, x := range s.list {
		if strings.EqualFold(x.ID, strings.TrimSpace(id)) {
			return x
		}
	}
	return nil
}

// index is one line per subject: id, title and status.
func (s *subjects) index() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.list) == 0 {
		return "No subjects yet."
	}
	var b strings.Builder
	for i, x := range s.list {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "%s · %s · %s", x.ID, x.Subject, x.Status)
	}
	return b.String()
}

func (s *subjects) snapshot() []subject {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]subject, len(s.list))
	for i, x := range s.list {
		out[i] = *x
		out[i].Open = slices.Clone(x.Open)
		out[i].Source = slices.Clone(x.Source)
	}
	return out
}

func (s *subjects) log() []subjectEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.events)
}

// The prompt rules, the same for every model (SPEC 1). Round 1's rule let
// models name subjects after steps ("Update daily_btc schedule"), so a
// change got a new subject; round 2's names the work and says a change
// updates its subject.
var subjectRules = map[string]string{
	"v1": `Subjects keep this chat's work across turns: one per piece of work, with its status and outcome.
When your work creates, changes or decides something, call update_subject: update the matching subject from the list, or create one if none matches. Call it together with your last tool call. Skip it for plain questions.`,
	"v2": `Subjects keep this chat's work across turns: one per piece of work, such as a table, a pipeline or a decision, with its status and outcome.
Name a subject after the work itself, with a noun ("BTC price pipeline"), never after a step ("Update the schedule").
When your work creates, changes or decides something, call update_subject. If the work already has a subject, update that one by its id, also for a change, a fix or a cancellation: never create a second subject for the same work. Create one only for new work. Call it together with your last tool call. Skip it for plain questions.`,
}

// updateDescriptions are update_subject's descriptions, by rule.
var updateDescriptions = map[string]string{
	"v1": "Create or update a subject: one piece of this chat's work, kept across turns with its status and outcome. " +
		"Without id it creates a subject, and subject is needed; with id it updates that one, and fields you leave out stay as they are. " +
		"outcome says what was done or decided, and why. open lists questions that are still open.",
	"v2": "Create or update a subject: one piece of this chat's work, such as a table, a pipeline or a decision, kept across turns with its status and outcome. " +
		"With id it updates that subject, and fields you leave out stay as they are: use it for every change to work that has a subject. " +
		"Without id it creates a subject for new work; subject is then needed, named after the work, not the step. " +
		"outcome says the work's current state: what was done or decided, and why. open lists questions that are still open.",
}

// block is what conditions 2–4 add to the card: the rule, and in
// conditions 3 and 4 the index.
func (s *subjects) block(withIndex bool) string {
	rule := subjectRules[s.rule]
	if !withIndex {
		return "Subjects\n" + rule
	}
	return "Subjects\n" + rule + "\n\nSubjects in this chat (as of the last cut):\n" + s.index()
}

func (s *subjects) tools() []tool.Tool {
	type updateArgs struct {
		ID      string    `json:"id"`
		Subject string    `json:"subject"`
		Status  string    `json:"status"`
		Outcome *string   `json:"outcome"`
		Open    *[]string `json:"open"`
	}
	type getArgs struct {
		ID string `json:"id"`
	}
	update := tool.Func(tool.Spec{
		Name:        "update_subject",
		Description: updateDescriptions[s.rule],
		Schema: json.RawMessage(`{
			"type":"object","required":["status"],"additionalProperties":false,
			"properties":{
				"id":{"type":"string","description":"the subject to update; leave it out to create one"},
				"subject":{"type":"string","description":"a short title"},
				"status":{"type":"string","enum":["open","in_progress","blocked","done","dropped"]},
				"outcome":{"type":"string"},
				"open":{"type":"array","items":{"type":"string"}}}}`),
		Effects: tool.ReadsDB,
	}, func(_ context.Context, env *tool.Env, a updateArgs) (tool.Result, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		var x *subject
		created := false
		if strings.TrimSpace(a.ID) != "" {
			if x = s.find(a.ID); x == nil {
				ids := make([]string, len(s.list))
				for i, y := range s.list {
					ids[i] = y.ID
				}
				return tool.Result{}, tool.Errorf("no subject %s (subjects: %s); leave id out to create one", a.ID, strings.Join(ids, ", "))
			}
		} else {
			if strings.TrimSpace(a.Subject) == "" {
				return tool.Result{}, tool.Errorf("a new subject needs subject, a short title")
			}
			x, created = &subject{ID: fmt.Sprintf("s%d", len(s.list)+1)}, true
			s.list = append(s.list, x)
		}
		if strings.TrimSpace(a.Subject) != "" {
			x.Subject = strings.TrimSpace(a.Subject)
		}
		x.Status = a.Status
		if a.Outcome != nil {
			x.Outcome = *a.Outcome
		}
		if a.Open != nil {
			x.Open = *a.Open
		}
		if env != nil && env.Message != "" && !slices.Contains(x.Source, string(env.Message)) {
			x.Source = append(x.Source, string(env.Message))
		}
		s.events = append(s.events, subjectEvent{Msg: s.cur, Nudge: s.nudge, ID: x.ID, Created: created, Status: x.Status})
		verb := "updated"
		if created {
			verb = "created"
		}
		return tool.Result{Text: fmt.Sprintf("subject %s %s: %s · %s", x.ID, verb, x.Subject, x.Status)}, nil
	})
	get := tool.Func(tool.Spec{
		Name:        "get_subject",
		Description: "Read a subject of this chat by id: its outcome, open questions and the messages it came from (read them with read_messages). Without id, lists every subject: id, title and status.",
		Schema:      json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"id":{"type":"string"}}}`),
		Effects:     tool.ReadsDB,
	}, func(_ context.Context, _ *tool.Env, a getArgs) (tool.Result, error) {
		if strings.TrimSpace(a.ID) == "" {
			return tool.Result{Text: s.index()}, nil
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		x := s.find(a.ID)
		if x == nil {
			return tool.Result{}, tool.Errorf("no subject %s", a.ID)
		}
		b, _ := json.MarshalIndent(x, "", "  ")
		return tool.Result{Text: string(b)}, nil
	})
	return []tool.Tool{update, get}
}
