package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/amir-saatchi/jenab/internal/tool"
)

// Background tasks (SPEC 8.3) as the spike fakes them: `subagent` with
// background: true returns a task ID at once, and the harness starts the
// finish turn with a notice later, when the chat is idle. Results come
// from the test's rules, so every model gets the same ones. A task gets
// the rule of the source it names first.

// taskRule is a fake result: what a subagent returns for a task that
// matches.
type taskRule struct {
	Name   string // a short name, such as linkedin
	Match  string // a regex on the task's arguments, case-insensitive
	Result string
	Fail   bool // the task fails with Result as its error
	Empty  bool // the result adds nothing
	// Order: Part A delivers the finish notices by order; tasks of the
	// same order finish together, in one notice turn.
	Order int
	// After: Part B delivers the notice once the scripted message with
	// this index has been sent and its turn has ended.
	After int
	Facts []string // the checklist: regexes an answer that has this result matches
}

// otherRule is the result of a task that matches no rule.
var otherRule = &taskRule{Name: "other", Result: "The search found nothing that matches the task.", Empty: true, Order: 99}

// task is one subagent call.
type task struct {
	ID         string
	Args       string
	Background bool
	Rules      []*taskRule
	Message    string // the assistant message with the call
	Started    time.Time
	Notified   bool // its finish notice was sent, or it ran in the foreground
	Turn       int  // the turn whose notice reported it; 0 before
}

func (t *task) order() int {
	o := 1 << 30
	for _, r := range t.Rules {
		o = min(o, r.Order)
	}
	return o
}

func (t *task) after() int {
	a := 0
	for _, r := range t.Rules {
		a = max(a, r.After)
	}
	return a
}

func (t *task) failed() bool {
	return !slices.ContainsFunc(t.Rules, func(r *taskRule) bool { return !r.Fail })
}

// result is the task's result, or its error when it failed.
func (t *task) result() string {
	if len(t.Rules) == 1 {
		return strings.TrimSpace(t.Rules[0].Result)
	}
	var b strings.Builder
	for i, r := range t.Rules {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "%s:", r.Name)
		if r.Fail {
			b.WriteString(" failed:")
		}
		b.WriteString("\n" + strings.TrimSpace(r.Result))
	}
	return b.String()
}

// notice is the finish notice's text (8.3).
func (t *task) notice() string {
	verb := "finished"
	if t.failed() {
		verb = "failed"
	}
	return fmt.Sprintf("[task %s %s: %s]", t.ID, verb, t.result())
}

// tasks are a run's background tasks.
type tasks struct {
	mu     sync.Mutex
	rules  []*taskRule
	list   []*task
	polls  map[string]int // "turn/id": task_status calls in a turn
	turnOf func() int     // the chat's running turn
	// web answers a task that matches no rule, in Part B: the fake web's
	// search results for the task's text, so a subagent asked to search
	// gets what web_search would give.
	web func(text string) (string, bool)
}

// match finds a task's rule: the one named first in the task's text, as
// a task such as "search Spiegel for news about the GDL strike" is about
// Spiegel; then in its inputs.
func (ts *tasks) match(text, args string) []*taskRule {
	for _, s := range []string{text, args} {
		var best *taskRule
		at := len(s) + 1
		for _, r := range ts.rules {
			if loc := regexp.MustCompile("(?is)" + r.Match).FindStringIndex(s); loc != nil && loc[0] < at {
				best, at = r, loc[0]
			}
		}
		if best != nil {
			return []*taskRule{best}
		}
	}
	return []*taskRule{otherRule}
}

func (ts *tasks) get(id string) *task {
	for _, t := range ts.list {
		if t.ID == id {
			return t
		}
	}
	return nil
}

// due are the background tasks whose notice can go out now: Part A's
// with the lowest order, or Part B's whose message has been sent. Tasks
// of one order go together.
func (ts *tasks) due(sent int) []*task {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	var out []*task
	low := 1 << 30
	for _, t := range ts.list {
		if t.Notified || !t.Background || t.after() > sent-1 {
			continue
		}
		switch o := t.order(); {
		case o < low:
			low, out = o, []*task{t}
		case o == low:
			out = append(out, t)
		}
	}
	return out
}

func (ts *tasks) notified(group []*task) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	for _, t := range group {
		t.Notified = true
	}
}

const subagentDesc = "Run a task in an isolated context with its own tools (web_search, fetch_page); returns only the result. " +
	"With background: true it returns a task ID right away and a notice follows when it finishes; " +
	"otherwise the call returns when the task is done, which can take minutes."

// foregroundTook is how long a subagent without background takes.
const foregroundTook = 3 * time.Second

func (ts *tasks) tools() []tool.Tool {
	type subagentArgs struct {
		Task       string         `json:"task"`
		Inputs     map[string]any `json:"inputs"`
		Background bool           `json:"background"`
	}
	type statusArgs struct {
		ID string `json:"id"`
	}
	sub := tool.Spec{Name: "subagent", Description: subagentDesc, Effects: tool.ReadsDB, Background: true, Schema: json.RawMessage(`{
		"type":"object","required":["task"],
		"properties":{"task":{"type":"string"},"inputs":{"type":"object"},"background":{"type":"boolean"}}}`)}
	status := tool.Spec{Name: "task_status", Description: "Status of a background task; a notice follows when it finishes, so there is no need to poll.",
		Effects: tool.ReadsDB, Schema: json.RawMessage(`{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`)}
	return []tool.Tool{
		tool.Func(sub, func(ctx context.Context, env *tool.Env, a subagentArgs) (tool.Result, error) {
			raw, _ := json.Marshal(a)
			rules := ts.match(a.Task, string(raw))
			if rules[0] == otherRule && ts.web != nil {
				if res, ok := ts.web(a.Task + "\n" + string(raw)); ok {
					rules = []*taskRule{{Name: "web", Result: res, Order: 99}}
				}
			}
			ts.mu.Lock()
			t := &task{ID: fmt.Sprintf("t_%d", len(ts.list)+1), Args: string(raw), Background: a.Background,
				Rules: rules, Message: string(env.Message), Started: time.Now()}
			ts.list = append(ts.list, t)
			ts.mu.Unlock()
			if t.Background {
				return tool.Result{Text: fmt.Sprintf("started: task %s. It goes on in the background; a notice follows when it finishes.", t.ID)}, nil
			}
			select {
			case <-time.After(foregroundTook):
			case <-ctx.Done():
				return tool.Result{}, ctx.Err()
			}
			ts.mu.Lock()
			t.Notified = true
			ts.mu.Unlock()
			if t.failed() {
				return tool.Result{}, tool.Errorf("%s", t.result())
			}
			return tool.Result{Text: t.result()}, nil
		}),
		tool.Func(status, func(_ context.Context, _ *tool.Env, a statusArgs) (tool.Result, error) {
			ts.mu.Lock()
			defer ts.mu.Unlock()
			t := ts.get(strings.TrimSpace(a.ID))
			if t == nil {
				var ids []string
				for _, t := range ts.list {
					ids = append(ids, t.ID)
				}
				known := "none"
				if len(ids) > 0 {
					known = strings.Join(ids, ", ")
				}
				return tool.Result{}, tool.Errorf("no task %q (tasks: %s)", a.ID, known)
			}
			if t.Notified {
				verb := "finished"
				if t.failed() {
					verb = "failed"
				}
				return tool.Result{Text: fmt.Sprintf("%s: %s", verb, t.result())}, nil
			}
			k := fmt.Sprintf("%d/%s", ts.turnOf(), t.ID)
			ts.polls[k]++
			if ts.polls[k] > 1 {
				return tool.Result{Text: "still running; a notice will follow"}, nil // the polling rule (8.3)
			}
			return tool.Result{Text: fmt.Sprintf("running for %d s", int(time.Since(t.Started).Seconds()))}, nil
		}),
	}
}
