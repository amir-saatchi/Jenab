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
// Rules v1 and v2 keep an outcome that each update replaces; v3 and v4 keep
// a summary and notes that are only added to, and v4 has no status.
type subject struct {
	ID         string   `json:"id"`
	Subject    string   `json:"subject"`
	Status     string   `json:"status,omitempty"`
	Outcome    string   `json:"outcome,omitempty"`
	Summary    string   `json:"summary,omitempty"`
	Notes      []note   `json:"notes,omitempty"`
	Open       []string `json:"open,omitempty"`
	Source     []string `json:"source"`                // the assistant messages that wrote it
	MergedInto string   `json:"merged_into,omitempty"` // set when merge_subjects joined it into another
}

// note is one line added to a subject. Seq orders the notes of merged
// subjects in time.
type note struct {
	Text   string `json:"text"`
	Source string `json:"source,omitempty"`
	Seq    int    `json:"seq"`
}

var statuses = []string{"open", "in_progress", "blocked", "done", "dropped"}

// subjectEvent is one successful update_subject or merge_subjects call,
// for scoring.
type subjectEvent struct {
	Msg     int      `json:"msg"`   // the scripted message's index
	Nudge   bool     `json:"nudge"` // made in the turn after the app's check
	ID      string   `json:"id"`
	Created bool     `json:"created"`
	Status  string   `json:"status"`
	Open    int      `json:"open"`             // open items after the call
	Merged  []string `json:"merged,omitempty"` // the subjects joined into ID
}

// subjects are a chat's subjects. cur and nudge say which scripted
// message the harness is playing, so each call is scored against it.
type subjects struct {
	mu     sync.Mutex
	list   []*subject
	events []subjectEvent
	cur    int
	nudge  bool
	rule   string // "v1" … "v4", see subjectRules
	seq    int
}

// wide is true for the rules where a subject is a goal with notes (v3, v4).
func (s *subjects) wide() bool { return s.rule == "v3" || s.rule == "v4" }

// noStatus is true for v4, where the open items are the only state.
func (s *subjects) noStatus() bool { return s.rule == "v4" }

func (s *subjects) at(msg int, nudge bool) {
	s.mu.Lock()
	s.cur, s.nudge = msg, nudge
	s.mu.Unlock()
}

// find returns the subject with this id, following merges.
func (s *subjects) find(id string) *subject {
	id = strings.TrimSpace(id)
	for range len(s.list) + 1 {
		i := slices.IndexFunc(s.list, func(x *subject) bool { return strings.EqualFold(x.ID, id) })
		if i < 0 {
			return nil
		}
		if s.list[i].MergedInto == "" {
			return s.list[i]
		}
		id = s.list[i].MergedInto
	}
	return nil
}

func (s *subjects) ids() string {
	var ids []string
	for _, x := range s.list {
		if x.MergedInto == "" {
			ids = append(ids, x.ID)
		}
	}
	return strings.Join(ids, ", ")
}

// state is what the index shows after the title: the status, or in v4
// the number of open items.
func (s *subjects) state(x *subject) string {
	if !s.noStatus() {
		return x.Status
	}
	if len(x.Open) == 0 {
		return "nothing open"
	}
	return fmt.Sprintf("%d open", len(x.Open))
}

// index is one line per subject: id, title and state. Merged subjects
// are left out.
func (s *subjects) index() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var b strings.Builder
	for _, x := range s.list {
		if x.MergedInto != "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "%s · %s · %s", x.ID, x.Subject, s.state(x))
	}
	if b.Len() == 0 {
		return "No subjects yet."
	}
	return b.String()
}

func (s *subjects) snapshot() []subject {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]subject, len(s.list))
	for i, x := range s.list {
		out[i] = *x
		out[i].Notes = slices.Clone(x.Notes)
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

// The prompt rules, the same for every model (SPEC 1).
//   - v1 (round 1) let models name subjects after steps ("Update daily_btc
//     schedule"), so a change got a new subject.
//   - v2 (round 2) names the work and says a change updates its subject.
//   - v3 (round 3, B) makes a subject one goal that can grow, with notes
//     that are only added to, and merge_subjects.
//   - v4 (round 3, C) is v3 with the open items as the only state.
//
// v3 and v4 take their example from outside the test chats.
var subjectRules = map[string]string{
	"v1": `Subjects keep this chat's work across turns: one per piece of work, with its status and outcome.
When your work creates, changes or decides something, call update_subject: update the matching subject from the list, or create one if none matches. Call it together with your last tool call. Skip it for plain questions.`,
	"v2": `Subjects keep this chat's work across turns: one per piece of work, such as a table, a pipeline or a decision, with its status and outcome.
Name a subject after the work itself, with a noun ("BTC price pipeline"), never after a step ("Update the schedule").
When your work creates, changes or decides something, call update_subject. If the work already has a subject, update that one by its id, also for a change, a fix or a cancellation: never create a second subject for the same work. Create one only for new work. Call it together with your last tool call. Skip it for plain questions.`,
	"v3": `Subjects keep this chat's work across turns: one per goal of the user, such as collecting some data or keeping a list, with its status, a short summary and notes.
Name a subject after the goal, with a noun ("Weather data collection"), never after a step ("Update the schedule").
When your work creates, changes or decides something, call update_subject. If the work belongs to a goal that has a subject, update that one by its id, also for a change, a fix, a cancellation, a saved convention or a changed row: add a note that says what changed and why, and update the summary when the current state changed. A subject may grow to cover more work for its goal: rename it when its title gets too narrow. Create one only for a new goal. Call it together with your last tool call. Skip it for plain questions.
When two subjects turn out to be the same goal, join them with merge_subjects.`,
	"v4": `Subjects keep this chat's work across turns: one per goal of the user, such as collecting some data or keeping a list, with a short summary, notes and what is still open.
Name a subject after the goal, with a noun ("Weather data collection"), never after a step ("Update the schedule").
When your work creates, changes or decides something, call update_subject. If the work belongs to a goal that has a subject, update that one by its id, also for a change, a fix, a cancellation, a saved convention or a changed row: add a note that says what changed and why, and update the summary when the current state changed. A subject may grow to cover more work for its goal: rename it when its title gets too narrow. Create one only for a new goal. Call it together with your last tool call. Skip it for plain questions.
Keep open as the full list of what is still waiting, and on what: when an item is finished or dropped, take it off the list and add a note.
When two subjects turn out to be the same goal, join them with merge_subjects.`,
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
	"v3": "Create or update a subject: one goal of the user in this chat, kept across turns with its status, a summary and notes. " +
		"With id it updates that subject, and fields you leave out stay as they are: use it for every change to work for that goal. " +
		"Without id it creates a subject for a new goal; subject is then needed, named after the goal, not the step. " +
		"summary is the current state and replaces the old one. note is added to the subject's notes, which are never rewritten: say what changed and why. " +
		"open lists what is still open and replaces the old list.",
	"v4": "Create or update a subject: one goal of the user in this chat, kept across turns with a summary, notes and what is still open. " +
		"With id it updates that subject, and fields you leave out stay as they are: use it for every change to work for that goal. " +
		"Without id it creates a subject for a new goal; subject is then needed, named after the goal, not the step. " +
		"summary is the current state and replaces the old one. note is added to the subject's notes, which are never rewritten: say what changed and why. " +
		"open lists what is still waiting, and on what; it replaces the old list, so send all of it, or [] when nothing is waiting.",
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
	type getArgs struct {
		ID string `json:"id"`
	}
	get := tool.Func(tool.Spec{
		Name:        "get_subject",
		Description: "Read a subject of this chat by id: everything it holds and the messages it came from (read them with read_messages). Without id, lists every subject: id, title and state.",
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
		text := string(b)
		if !strings.EqualFold(x.ID, strings.TrimSpace(a.ID)) {
			text = fmt.Sprintf("%s was merged into %s.\n%s", strings.TrimSpace(a.ID), x.ID, text)
		}
		return tool.Result{Text: text}, nil
	})
	if s.wide() {
		return []tool.Tool{s.updateWide(), get, s.merge()}
	}
	return []tool.Tool{s.updateNarrow(), get}
}

// updateNarrow is update_subject for v1 and v2.
func (s *subjects) updateNarrow() tool.Tool {
	type updateArgs struct {
		ID      string    `json:"id"`
		Subject string    `json:"subject"`
		Status  string    `json:"status"`
		Outcome *string   `json:"outcome"`
		Open    *[]string `json:"open"`
	}
	return tool.Func(tool.Spec{
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
		x, created, err := s.target(a.ID, a.Subject)
		if err != nil {
			return tool.Result{}, err
		}
		x.Status = a.Status
		if a.Outcome != nil {
			x.Outcome = *a.Outcome
		}
		if a.Open != nil {
			x.Open = *a.Open
		}
		return s.written(env, x, created), nil
	})
}

// updateWide is update_subject for v3 and v4: a summary that is replaced,
// notes that are only added to, and in v4 no status.
func (s *subjects) updateWide() tool.Tool {
	type updateArgs struct {
		ID      string    `json:"id"`
		Subject string    `json:"subject"`
		Status  string    `json:"status"`
		Summary *string   `json:"summary"`
		Note    string    `json:"note"`
		Open    *[]string `json:"open"`
	}
	status, required := `"status":{"type":"string","enum":["open","in_progress","blocked","done","dropped"]},`, `"required":["status"],`
	if s.noStatus() {
		status, required = "", ""
	}
	return tool.Func(tool.Spec{
		Name:        "update_subject",
		Description: updateDescriptions[s.rule],
		Schema: json.RawMessage(`{
			"type":"object",` + required + `"additionalProperties":false,
			"properties":{
				"id":{"type":"string","description":"the subject to update; leave it out to create one"},
				"subject":{"type":"string","description":"a short title, named after the goal"},
				` + status + `
				"summary":{"type":"string","description":"the current state; replaces the old summary"},
				"note":{"type":"string","description":"added to the notes: what changed and why"},
				"open":{"type":"array","items":{"type":"string"},"description":"the full list; replaces the old one"}}}`),
		Effects: tool.ReadsDB,
	}, func(_ context.Context, env *tool.Env, a updateArgs) (tool.Result, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		x, created, err := s.target(a.ID, a.Subject)
		if err != nil {
			return tool.Result{}, err
		}
		if !s.noStatus() {
			x.Status = a.Status
		}
		if a.Summary != nil {
			x.Summary = *a.Summary
		}
		if t := strings.TrimSpace(a.Note); t != "" {
			s.seq++
			n := note{Text: t, Seq: s.seq}
			if env != nil {
				n.Source = string(env.Message)
			}
			x.Notes = append(x.Notes, n)
		}
		if a.Open != nil {
			x.Open = *a.Open
		}
		return s.written(env, x, created), nil
	})
}

// target is the subject an update writes: the one with id, or a new one.
// The caller holds s.mu.
func (s *subjects) target(id, title string) (*subject, bool, error) {
	if strings.TrimSpace(id) != "" {
		x := s.find(id)
		if x == nil {
			return nil, false, tool.Errorf("no subject %s (subjects: %s); leave id out to create one", id, s.ids())
		}
		if t := strings.TrimSpace(title); t != "" {
			x.Subject = t
		}
		return x, false, nil
	}
	t := strings.TrimSpace(title)
	if t == "" {
		return nil, false, tool.Errorf("a new subject needs subject, a short title")
	}
	x := &subject{ID: fmt.Sprintf("s%d", len(s.list)+1), Subject: t}
	s.list = append(s.list, x)
	return x, true, nil
}

// written records an update and returns the tool's result. The caller
// holds s.mu.
func (s *subjects) written(env *tool.Env, x *subject, created bool) tool.Result {
	if env != nil && env.Message != "" && !slices.Contains(x.Source, string(env.Message)) {
		x.Source = append(x.Source, string(env.Message))
	}
	s.events = append(s.events, subjectEvent{Msg: s.cur, Nudge: s.nudge, ID: x.ID, Created: created, Status: x.Status, Open: len(x.Open)})
	verb := "updated"
	if created {
		verb = "created"
	}
	return tool.Result{Text: fmt.Sprintf("subject %s %s: %s · %s", x.ID, verb, x.Subject, s.state(x))}
}

// merge is merge_subjects (v3, v4). The model writes the title; the app
// joins the notes in time order and the open items, so nothing is
// rewritten. The other ids lead to the kept subject from now on.
func (s *subjects) merge() tool.Tool {
	type mergeArgs struct {
		IDs     []string `json:"ids"`
		Subject string   `json:"subject"`
		Status  string   `json:"status"`
		Summary *string  `json:"summary"`
	}
	desc := "Join subjects that turned out to be the same goal. The first id is kept; the others' notes and open items move into it in time order, and their ids lead to it from now on. " +
		"subject is the joined title, named after the goal. summary is the current state; without it the old summaries are joined."
	status, required := `"status":{"type":"string","enum":["open","in_progress","blocked","done","dropped"]},`, `"required":["ids","subject","status"],`
	if s.noStatus() {
		status, required = "", `"required":["ids","subject"],`
	} else {
		desc += " status is the joined subject's status."
	}
	return tool.Func(tool.Spec{
		Name:        "merge_subjects",
		Description: desc,
		Schema: json.RawMessage(`{
			"type":"object",` + required + `"additionalProperties":false,
			"properties":{
				"ids":{"type":"array","items":{"type":"string"},"minItems":2,"description":"the subjects to join; the first is kept"},
				"subject":{"type":"string","description":"the joined title"},
				` + status + `
				"summary":{"type":"string"}}}`),
		Effects: tool.ReadsDB,
	}, func(_ context.Context, env *tool.Env, a mergeArgs) (tool.Result, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		var xs []*subject
		for _, id := range a.IDs {
			x := s.find(id)
			if x == nil {
				return tool.Result{}, tool.Errorf("no subject %s (subjects: %s)", id, s.ids())
			}
			if !slices.Contains(xs, x) {
				xs = append(xs, x)
			}
		}
		if len(xs) < 2 {
			return tool.Result{}, tool.Errorf("merge_subjects needs two different subjects")
		}
		if strings.TrimSpace(a.Subject) == "" {
			return tool.Result{}, tool.Errorf("merge_subjects needs subject, the joined title")
		}
		keep := xs[0]
		var was []string
		summaries := []string{}
		for _, x := range xs {
			was = append(was, fmt.Sprintf("%s (%s)", x.ID, x.Subject))
			if x.Summary != "" {
				summaries = append(summaries, x.Summary)
			}
		}
		var merged []string
		for _, x := range xs[1:] {
			keep.Notes = append(keep.Notes, x.Notes...)
			for _, o := range x.Open {
				if !slices.Contains(keep.Open, o) {
					keep.Open = append(keep.Open, o)
				}
			}
			for _, src := range x.Source {
				if !slices.Contains(keep.Source, src) {
					keep.Source = append(keep.Source, src)
				}
			}
			x.MergedInto = keep.ID
			merged = append(merged, x.ID)
		}
		slices.SortStableFunc(keep.Notes, func(a, b note) int { return a.Seq - b.Seq })
		keep.Subject = strings.TrimSpace(a.Subject)
		if a.Summary != nil {
			keep.Summary = *a.Summary
		} else {
			keep.Summary = strings.Join(summaries, " / ")
		}
		if !s.noStatus() {
			keep.Status = a.Status
		}
		if env != nil && env.Message != "" && !slices.Contains(keep.Source, string(env.Message)) {
			keep.Source = append(keep.Source, string(env.Message))
		}
		s.events = append(s.events, subjectEvent{Msg: s.cur, Nudge: s.nudge, ID: keep.ID, Status: keep.Status, Open: len(keep.Open), Merged: merged})
		return tool.Result{Text: fmt.Sprintf("merged %s into %s: %s · %s", strings.Join(was, " + "), keep.ID, keep.Subject, s.state(keep))}, nil
	})
}
