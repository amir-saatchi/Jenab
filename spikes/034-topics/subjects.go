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

// Condition 4 is SPIKE-029's proposed combined design, the round 4 it
// never ran: round 2's size (one subject per piece of work, named with a
// noun), round 3's summary and notes that are only added to, open items
// instead of a status, and merge_subjects. The code is SPIKE-029's
// (subjects.go, rule v4), with round 2's size in the rule.

// subject is one piece of the chat's work.
type subject struct {
	ID         string   `json:"id"`
	Subject    string   `json:"subject"`
	Summary    string   `json:"summary,omitempty"`
	Notes      []note   `json:"notes,omitempty"`
	Open       []string `json:"open,omitempty"`
	Labels     []string `json:"labels,omitempty"` // the scripted labels of the messages that wrote it
	MergedInto string   `json:"merged_into,omitempty"`
}

// subjectEvent is one update_subject or merge_subjects call that worked.
type subjectEvent struct {
	Msg     int      `json:"msg"`
	ID      string   `json:"id"`
	Created bool     `json:"created"`
	Open    int      `json:"open"`
	Merged  []string `json:"merged,omitempty"`
}

type subjects struct {
	mu     sync.Mutex
	list   []*subject
	events []subjectEvent
	cur    int
	label  string
	seq    int
}

func (s *subjects) at(msg int, label string) {
	s.mu.Lock()
	s.cur, s.label = msg, label
	s.mu.Unlock()
}

const subjectRule = `Subjects keep this chat's work across turns: one per piece of work, such as a table, a pipeline or a decision, with a short summary, notes and what is still open.
Name a subject after the work itself, with a noun ("Weather data table"), never after a step ("Update the schedule").
When your work creates, changes or decides something, call update_subject. If the work already has a subject, update that one by its id, also for a change, a fix, a cancellation, a saved convention or a changed row: add a note that says what changed and why, and update the summary when the current state changed. Never create a second subject for the same work; create one only for new work. Call it together with your last tool call. Skip it for plain questions.
Keep open as the full list of what is still waiting, and on what: when an item is finished or dropped, take it off the list and add a note.
When the user says two subjects are one thing, join them with merge_subjects.`

func (s *subjects) find(sid string) *subject {
	sid = strings.TrimSpace(sid)
	for range len(s.list) + 1 {
		i := slices.IndexFunc(s.list, func(x *subject) bool { return strings.EqualFold(x.ID, sid) })
		if i < 0 {
			return nil
		}
		if s.list[i].MergedInto == "" {
			return s.list[i]
		}
		sid = s.list[i].MergedInto
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

// index is one line per live subject. The caller holds s.mu.
func (s *subjects) index() string {
	var b strings.Builder
	for _, x := range s.list {
		if x.MergedInto != "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "%s · %s · %s", x.ID, x.Subject, state(x.Open))
	}
	if b.Len() == 0 {
		return "No subjects yet."
	}
	return b.String()
}

// block is block 5 in condition 4: the rule and the index, as of the
// last cut.
func (s *subjects) block() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return "Subjects\n" + subjectRule + "\n\nSubjects in this chat (as of the last cut):\n" + s.index()
}

// labelled is the live subjects that hold a label.
func (s *subjects) labelled(label string) []*subject {
	var out []*subject
	for _, x := range s.list {
		if x.MergedInto == "" && slices.Contains(x.Labels, label) {
			out = append(out, x)
		}
	}
	return out
}

// openFor is the open items of the label's subjects.
func (s *subjects) openFor(label string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, x := range s.labelled(label) {
		n += len(x.Open)
	}
	return n
}

// joined says whether one live subject holds all the labels, and none
// other holds any of them.
func (s *subjects) joined(labels []string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	var hold []*subject
	for _, l := range labels {
		for _, x := range s.labelled(l) {
			if !slices.Contains(hold, x) {
				hold = append(hold, x)
			}
		}
	}
	return len(hold) == 1
}

func (s *subjects) snapshot() ([]subject, []subjectEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]subject, len(s.list))
	for i, x := range s.list {
		out[i] = *x
		out[i].Notes = slices.Clone(x.Notes)
		out[i].Open = slices.Clone(x.Open)
		out[i].Labels = slices.Clone(x.Labels)
	}
	return out, slices.Clone(s.events)
}

func (s *subjects) tools() []tool.Tool {
	type getArgs struct {
		ID string `json:"id"`
	}
	get := tool.Func(tool.Spec{
		Name:        "get_subject",
		Description: "Read a subject of this chat by id: its summary, notes and open items. Without id, lists every subject: id, title and what is open.",
		Schema:      json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"id":{"type":"string"}}}`),
		Effects:     tool.ReadsDB,
	}, func(_ context.Context, _ *tool.Env, a getArgs) (tool.Result, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if strings.TrimSpace(a.ID) == "" {
			return tool.Result{Text: s.index()}, nil
		}
		x := s.find(a.ID)
		if x == nil {
			return tool.Result{}, tool.Errorf("no subject %s", a.ID)
		}
		b, _ := json.MarshalIndent(struct {
			ID      string   `json:"id"`
			Subject string   `json:"subject"`
			Summary string   `json:"summary,omitempty"`
			Notes   []note   `json:"notes,omitempty"`
			Open    []string `json:"open"`
		}{x.ID, x.Subject, x.Summary, x.Notes, x.Open}, "", "  ")
		text := string(b)
		if !strings.EqualFold(x.ID, strings.TrimSpace(a.ID)) {
			text = fmt.Sprintf("%s was merged into %s.\n%s", strings.TrimSpace(a.ID), x.ID, text)
		}
		return tool.Result{Text: text}, nil
	})
	return []tool.Tool{s.update(), get, s.merge()}
}

func (s *subjects) update() tool.Tool {
	type args struct {
		ID      string    `json:"id"`
		Subject string    `json:"subject"`
		Summary *string   `json:"summary"`
		Note    string    `json:"note"`
		Open    *[]string `json:"open"`
	}
	return tool.Func(tool.Spec{
		Name: "update_subject",
		Description: "Create or update a subject: one piece of this chat's work, such as a table, a pipeline or a decision, kept across turns with a summary, notes and what is still open. " +
			"With id it updates that subject, and fields you leave out stay as they are: use it for every change to work that has a subject. " +
			"Without id it creates a subject for new work; subject is then needed, named after the work, not the step. " +
			"summary is the current state and replaces the old one. note is added to the subject's notes, which are never rewritten: say what changed and why. " +
			"open lists what is still waiting, and on what; it replaces the old list, so send all of it, or [] when nothing is waiting.",
		Schema: json.RawMessage(`{
			"type":"object","additionalProperties":false,
			"properties":{
				"id":{"type":"string","description":"the subject to update; leave it out to create one"},
				"subject":{"type":"string","description":"a short title, named after the work"},
				"summary":{"type":"string","description":"the current state; replaces the old summary"},
				"note":{"type":"string","description":"added to the notes: what changed and why"},
				"open":{"type":"array","items":{"type":"string"},"description":"the full list; replaces the old one"}}}`),
		Effects: tool.ReadsDB,
	}, func(_ context.Context, env *tool.Env, a args) (tool.Result, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		var x *subject
		created := false
		if strings.TrimSpace(a.ID) != "" {
			if x = s.find(a.ID); x == nil {
				return tool.Result{}, tool.Errorf("no subject %s (subjects: %s); leave id out to create one", a.ID, s.ids())
			}
			if t := strings.TrimSpace(a.Subject); t != "" {
				x.Subject = t
			}
		} else {
			t := strings.TrimSpace(a.Subject)
			if t == "" {
				return tool.Result{}, tool.Errorf("a new subject needs subject, a short title")
			}
			x, created = &subject{ID: fmt.Sprintf("s%d", len(s.list)+1), Subject: t}, true
			s.list = append(s.list, x)
		}
		if a.Summary != nil {
			x.Summary = *a.Summary
		}
		if t := strings.TrimSpace(a.Note); t != "" {
			s.seq++
			x.Notes = append(x.Notes, note{Text: t, Source: string(env.Message), Seq: s.seq})
		}
		if a.Open != nil {
			x.Open = slices.Clone(*a.Open)
		}
		if s.label != mainLabel && !slices.Contains(x.Labels, s.label) {
			x.Labels = append(x.Labels, s.label)
		}
		s.events = append(s.events, subjectEvent{Msg: s.cur, ID: x.ID, Created: created, Open: len(x.Open)})
		verb := "updated"
		if created {
			verb = "created"
		}
		return tool.Result{Text: fmt.Sprintf("subject %s %s: %s · %s", x.ID, verb, x.Subject, state(x.Open))}, nil
	})
}

func (s *subjects) merge() tool.Tool {
	type args struct {
		IDs     []string `json:"ids"`
		Subject string   `json:"subject"`
		Summary *string  `json:"summary"`
	}
	return tool.Func(tool.Spec{
		Name: "merge_subjects",
		Description: "Join subjects that the user says are one thing. The first id is kept; the others' notes and open items move into it in time order, and their ids lead to it from now on. " +
			"subject is the joined title. summary is the current state; without it the old summaries are joined.",
		Schema: json.RawMessage(`{
			"type":"object","required":["ids","subject"],"additionalProperties":false,
			"properties":{
				"ids":{"type":"array","items":{"type":"string"},"minItems":2,"description":"the subjects to join; the first is kept"},
				"subject":{"type":"string","description":"the joined title"},
				"summary":{"type":"string"}}}`),
		Effects: tool.ReadsDB,
	}, func(_ context.Context, _ *tool.Env, a args) (tool.Result, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		var xs []*subject
		for _, sid := range a.IDs {
			x := s.find(sid)
			if x == nil {
				return tool.Result{}, tool.Errorf("no subject %s (subjects: %s)", sid, s.ids())
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
		var was, summaries, merged []string
		for _, x := range xs {
			was = append(was, fmt.Sprintf("%s (%s)", x.ID, x.Subject))
			if x.Summary != "" {
				summaries = append(summaries, x.Summary)
			}
		}
		for _, x := range xs[1:] {
			keep.Notes = append(keep.Notes, x.Notes...)
			for _, o := range x.Open {
				if !slices.Contains(keep.Open, o) {
					keep.Open = append(keep.Open, o)
				}
			}
			for _, l := range x.Labels {
				if !slices.Contains(keep.Labels, l) {
					keep.Labels = append(keep.Labels, l)
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
		s.events = append(s.events, subjectEvent{Msg: s.cur, ID: keep.ID, Open: len(keep.Open), Merged: merged})
		return tool.Result{Text: fmt.Sprintf("merged %s into %s: %s · %s", strings.Join(was, " + "), keep.ID, keep.Subject, state(keep.Open))}, nil
	})
}
