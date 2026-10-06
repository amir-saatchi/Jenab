package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// Approvals and questions (SPEC 8.8). A card or form is a part of the
// tool message; the turn waits for its answer without holding a
// transaction, and the answer is written into the same part.

var (
	// ErrNotWaiting: the chat doesn't wait for that card or form, for
	// example because it was answered or the turn stopped.
	ErrNotWaiting = errors.New("agent: the chat doesn't wait for that answer")
	ErrBadAnswer  = errors.New("agent: the answer doesn't fit the card")
)

// Answer is the user's answer to the card or form a turn waits for.
type Answer struct {
	Message id.Message `json:"message"` // the card's message and part index
	Index   int        `json:"index"`
	Grant   chat.Grant `json:"grant,omitempty"` // an approval card's option
	// Answers are a question form's, by header: the picked labels or the
	// Other text.
	Answers map[string][]string `json:"answers,omitempty"`
	Note    string              `json:"note,omitempty"` // a Deny note, or a note with the form
}

// decision is what an approval level does with one kind of approval.
type decision int

const (
	ask decision = iota
	autoApprove
	// autoIfTrusted approves only if the turn hasn't read untrusted
	// content since the user's last message, so a web page can't steer the
	// agent into sending data somewhere.
	autoIfTrusted
)

// rules are the 8.8 table by approval kind: Strict, Standard, Auto. Kinds
// not listed, such as destructive migrations and connections, ask at every
// level. Schema changes, schedules, memory edits, MCP tools and commands
// come with their phases.
var rules = map[string][3]decision{
	"host":     {ask, ask, autoIfTrusted},
	"starlark": {ask, ask, autoApprove},
}

// auto reports whether level l approves an approval of kind without
// asking.
func auto(l project.Level, kind string, untrusted bool) bool {
	i := map[project.Level]int{project.Strict: 0, project.Standard: 1, project.Auto: 2}[l]
	switch rules[kind][i] {
	case autoApprove:
		return true
	case autoIfTrusted:
		return !untrusted
	}
	return false
}

// approve splits a call's approvals in one place: those already given
// are dropped, and the level and the turn's untrusted mark decide which of
// the rest are approved without asking (8.8).
func (r *runner) approve(ctx context.Context, n tool.Needs) (asks, autos []chat.Approval, err error) {
	if len(n.Approvals) == 0 {
		return nil, nil, nil
	}
	level, err := r.p.Level(ctx)
	if err != nil {
		return nil, nil, err
	}
	r.cs.mu.Lock()
	untrusted := r.cs.untrusted
	r.cs.mu.Unlock()
	for _, a := range n.Approvals {
		ok, err := r.p.DB.Approved(ctx, a.Kind, a.Target)
		if err != nil {
			return nil, nil, err
		}
		switch {
		case ok:
		case auto(level, a.Kind, untrusted):
			autos = append(autos, a)
		default:
			asks = append(asks, a)
		}
	}
	return asks, autos, nil
}

// autoApprove records an approval the level gave, and shows it as a chip:
// the card, answered by "auto". It grants Always where the card has it.
func (r *runner) autoApprove(ctx context.Context, tm *toolMsg, a chat.Approval) error {
	a.Answer = chat.GrantOnce
	if _, ok := a.Option(chat.GrantAlways); ok {
		a.Answer = chat.GrantAlways
	}
	at := stamp()
	a.By, a.AnsweredAt = id.SourceAuto, &at
	if err := r.record(ctx, a); err != nil {
		return err
	}
	_, err := tm.add(ctx, chat.Part{Kind: chat.PartApproval, Approval: &a})
	return err
}

// record stores an answered approval in _jenab_approvals.
func (r *runner) record(ctx context.Context, a chat.Approval) error {
	wctx, cancel := writing(ctx)
	defer cancel()
	return r.p.DB.RecordApproval(wctx, store.Approval{ID: a.ID, Kind: a.Kind, Target: a.Target, Answer: a.Answer,
		Note: a.Note, Source: a.By, CreatedAt: *a.AnsweredAt})
}

// denied is the result of a call the user denied.
func denied(a *chat.Approval) string {
	s := "not run: the user denied it (" + a.Ask + ")"
	if a.Note != "" {
		s += "; their note: " + a.Note
	}
	return s
}

// pending is the card or form a turn waits for.
type pending struct {
	w     chat.Waiting
	part  chat.Part
	reply chan Answer // buffered, so Answer and Send never block
}

// await writes a card or form to the tool message and waits for its
// answer, which it writes into the part. Stop and the project's close end
// the wait; the part is then marked as stopped.
func (r *runner) await(ctx context.Context, tm *toolMsg, p chat.Part) (chat.Part, error) {
	cs := r.cs
	// cs.mu is held from the write until the wait is set, so an answer to
	// the card's PartDone finds it.
	cs.mu.Lock()
	i, err := tm.add(ctx, p)
	if err != nil {
		cs.mu.Unlock()
		return p, err
	}
	pd := &pending{w: chat.Waiting{Message: tm.id, Index: i, Kind: p.Kind, Text: waitText(p)}, part: p, reply: make(chan Answer, 1)}
	cs.waiting = pd
	r.o.publishStatus(cs)
	cs.mu.Unlock()
	var a Answer
	select {
	case a = <-pd.reply: // the status is already "working"
	case <-ctx.Done():
		cs.mu.Lock()
		if cs.waiting == pd {
			cs.waiting = nil
			r.o.publishStatus(cs)
		}
		cs.mu.Unlock()
		p = stoppedCard(p)
		if err := tm.set(ctx, i, p); err != nil {
			r.o.d.Log.Error("agent: closing a card", "chat", cs.key.c, "err", err)
		}
		return p, context.Cause(ctx)
	}
	p = answered(p, a)
	if p.Approval != nil {
		if err := r.record(ctx, *p.Approval); err != nil {
			return p, err
		}
	}
	return p, tm.set(ctx, i, p)
}

// waitText is the card or form in one line, for the bar above the
// composer.
func waitText(p chat.Part) string {
	if p.Approval != nil {
		return p.Approval.Ask
	}
	q := p.Question.Questions
	if len(q) > 1 {
		return fmt.Sprintf("%s (and %d more)", q[0].Question, len(q)-1)
	}
	return q[0].Question
}

func answered(p chat.Part, a Answer) chat.Part {
	at := stamp()
	switch {
	case p.Approval != nil:
		c := *p.Approval
		c.Answer, c.Note, c.By, c.AnsweredAt = a.Grant, a.Note, id.SourceUser, &at
		p.Approval = &c
	case p.Question != nil:
		q := *p.Question
		q.Answers, q.Note, q.AnsweredAt = a.Answers, a.Note, &at
		p.Question = &q
	}
	return p
}

func stoppedCard(p chat.Part) chat.Part {
	switch {
	case p.Approval != nil:
		c := *p.Approval
		c.Stopped = true
		p.Approval = &c
	case p.Question != nil:
		q := *p.Question
		q.Stopped = true
		p.Question = &q
	}
	return p
}

// stamp is now, as stored times are.
func stamp() time.Time { return time.Now().UTC().Truncate(time.Millisecond) }

// Answer answers the card or form the chat's turn waits for (8.8).
func (o *Orchestrator) Answer(p id.Project, c id.Chat, a Answer) error {
	cs := o.find(p, c)
	if cs == nil {
		return ErrNotWaiting
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	w := cs.waiting
	if w == nil || w.w.Message != a.Message || w.w.Index != a.Index {
		return ErrNotWaiting
	}
	a, err := fit(w.part, a)
	if err != nil {
		return err
	}
	o.deliver(cs, a)
	return nil
}

// deliver ends the wait with a. cs.mu is held.
func (o *Orchestrator) deliver(cs *chatState, a Answer) {
	cs.waiting.reply <- a
	cs.waiting = nil
	o.publishStatus(cs)
}

// fit checks an answer against its card or form and tidies it: an
// approval's grant must be one of its options; a form's answers need known
// headers, one value for a question without multi, and at least one
// answered question.
func fit(p chat.Part, a Answer) (Answer, error) {
	a.Note = strings.TrimSpace(a.Note)
	if c := p.Approval; c != nil {
		if _, ok := c.Option(a.Grant); !ok || len(a.Answers) > 0 {
			return a, fmt.Errorf("%w: %q is not an option", ErrBadAnswer, a.Grant)
		}
		return a, nil
	}
	if a.Grant != "" {
		return a, fmt.Errorf("%w: a question form has no grant", ErrBadAnswer)
	}
	items := map[string]chat.QuestionItem{}
	for _, it := range p.Question.Questions {
		items[it.Header] = it
	}
	out := map[string][]string{}
	for h, vs := range a.Answers {
		it, ok := items[h]
		if !ok {
			return a, fmt.Errorf("%w: no question %q", ErrBadAnswer, h)
		}
		var keep []string
		for _, v := range vs {
			if v = strings.TrimSpace(v); v != "" {
				keep = append(keep, v)
			}
		}
		if len(keep) > 1 && !it.Multi {
			return a, fmt.Errorf("%w: %q takes one answer", ErrBadAnswer, h)
		}
		if len(keep) > 0 {
			out[h] = keep
		}
	}
	if len(out) == 0 {
		return a, fmt.Errorf("%w: no question is answered", ErrBadAnswer)
	}
	a.Answers = out
	return a, nil
}

// SetLevel changes the project's approval level from the chat's chip
// (8.8). It applies from the next tool call, and adds a notice to the chat.
func (o *Orchestrator) SetLevel(ctx context.Context, p id.Project, c id.Chat, l project.Level) error {
	if _, err := project.ParseLevel(string(l)); err != nil {
		return err
	}
	if err := o.enter(); err != nil {
		return err
	}
	defer o.leave()
	proj, err := o.d.Projects.Open(ctx, p)
	if err != nil {
		return err
	}
	defer proj.Release()
	old, err := proj.Level(ctx)
	if err != nil || old == l {
		return err
	}
	if err := proj.SetLevel(ctx, l); err != nil {
		return err
	}
	cs := o.state(p, c)
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if err := o.load(ctx, cs, proj); err != nil {
		return err
	}
	n := chat.Notice{Kind: chat.NoticeApprovalLevel, Text: fmt.Sprintf("[the approval level changed from %s to %s]", old, l)}
	// In the running or last turn; turns start at 1.
	_, err = o.write(ctx, cs, proj, chat.Message{Chat: c, Turn: max(cs.turn, 1), Role: chat.RoleUser,
		Parts: []chat.Part{{Kind: chat.PartNotice, Notice: &n}}})
	return err
}
