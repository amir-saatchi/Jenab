package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

type options struct {
	Mode        string // index: the model sees the index and names next; path: the path only, the controller picks
	K           int    // the most thoughts per call
	MaxThoughts int
	MaxDepth    int
	MaxTokens   int
}

// args is the answer of a define or expand call.
type args struct {
	Thoughts []struct {
		Kind      string  `json:"kind"`
		Title     string  `json:"title"`
		Content   string  `json:"content"`
		Weight    float64 `json:"weight"`
		Rationale string  `json:"rationale"`
		Status    string  `json:"status"`
		MergeWith []int   `json:"merge_with"`
	} `json:"thoughts"`
	Reweight []struct {
		ID     int     `json:"id"`
		Weight float64 `json:"weight"`
	} `json:"reweight"`
	Next string `json:"next"`
}

// decodeInto decodes the answer into a and runs check on it.
func decodeInto(a *args, check func(*args) error) func(json.RawMessage) error {
	return func(raw json.RawMessage) error {
		*a = args{}
		if err := json.Unmarshal(raw, a); err != nil {
			return err
		}
		return check(a)
	}
}

// grow builds one graph: define the problem, expand one thought per call
// until the model says finish (index mode) or the budget is used, then
// write the answer. Every thought, edge, weight change and call is stored
// as it happens.
func grow(ctx context.Context, st *store, c *client, p problem, o options) (*graph, error) {
	index := o.Mode == "index"
	g := &graph{Problem: p, Model: c.model, Mode: o.Mode, MaxDepth: o.MaxDepth}
	if err := st.newGraph(ctx, g, o); err != nil {
		return nil, err
	}
	stop := func(why string, err error) (*graph, error) {
		g.Finish = why
		return g, errors.Join(err, st.finish(context.WithoutCancel(ctx), g))
	}

	var a args
	sch := thoughtsSchema(1, false)
	rec := &callRec{Purpose: "define"}
	err := c.askJSON(ctx, systemPrompt(index, sch), defineMsg(p), sch, false, o.MaxTokens, decodeInto(&a, checkDefine), rec)
	call, serr := st.addCall(ctx, g.ID, c.model, rec)
	if err != nil || serr != nil {
		return stop("broke", errors.Join(err, serr))
	}
	root := &thought{Num: 1, Kind: "problem", Title: a.Thoughts[0].Title, Content: a.Thoughts[0].Content,
		Weight: 1, Rationale: a.Thoughts[0].Rationale, State: "open"}
	g.nodes = append(g.nodes, root)
	if err := st.addThought(ctx, g, root, call); err != nil {
		return stop("broke", err)
	}
	c.logf("#1 %s", root.Title)

	focus, fails := 1, 0
	for len(g.nodes) < o.MaxThoughts {
		room := o.MaxThoughts - len(g.nodes)
		k := min(o.K, room)
		sch := thoughtsSchema(k, index)
		rec := &callRec{Purpose: "expand", Focus: focus}
		if index {
			rec.IndexChars = len(g.index())
		}
		err := c.askJSON(ctx, systemPrompt(index, sch), expandMsg(g, focus, k, room, index), sch, false, o.MaxTokens, decodeInto(&a, checkExpand(g, focus)), rec)
		call, serr := st.addCall(ctx, g.ID, c.model, rec)
		if serr != nil {
			return stop("broke", serr)
		}
		g.Expansions++
		if errors.Is(err, errQuota) || ctx.Err() != nil {
			return stop("broke", err)
		}
		if err != nil {
			c.logf("expand #%d failed: %s", focus, clip(err.Error()))
			if fails++; fails == 3 {
				return stop("broke", err)
			}
			// Try another thought; this one stays as it is.
			if n := g.pickExcept(focus); n != 0 {
				focus = n
			}
			continue
		}
		fails = 0
		if err := apply(ctx, st, c, g, focus, &a, call); err != nil {
			return stop("broke", err)
		}
		if !index {
			focus = g.pick()
		} else {
			next, finish := parseNext(a.Next)
			switch {
			case finish && g.has("solution"):
				c.logf("model: finish")
				g.Finish = "model"
			case finish:
				g.EarlyFinish++
				focus = g.pick()
			case g.expandable(g.get(next)):
				g.NextPicked++
				focus = next
			default:
				g.NextInvalid++
				focus = g.pick()
			}
		}
		if g.Finish != "" {
			break
		}
		if focus == 0 {
			g.Finish = "no_open"
			break
		}
	}
	if g.Finish == "" {
		g.Finish = "budget"
	}

	rec = &callRec{Purpose: "conclude", IndexChars: len(g.index())}
	answer, err := c.text(ctx, concludeSystem, concludeMsg(g), false, o.MaxTokens, rec)
	call, serr = st.addCall(ctx, g.ID, c.model, rec)
	if serr != nil || err != nil {
		g.Finish += "+no_answer"
		return stop(g.Finish, errors.Join(err, serr))
	}
	g.Answer, g.Used = usedLine(answer)
	if err := st.finish(ctx, g); err != nil {
		return g, err
	}
	_, err = st.addAnswer(ctx, &answerRow{Problem: p.ID, Model: c.model, Kind: o.Mode, GraphID: g.ID, CallID: call, Text: g.Answer, Used: g.Used, Valid: true})
	return g, err
}

// baseline answers the task in one call, with thinking off (plain) or on.
// A thinking answer with no thinking in it, neither thinking text nor
// thought tokens, is kept but not valid: the request didn't do what it
// says. Some models think without sending any thinking text.
func baseline(ctx context.Context, st *store, c *client, p problem, thinking bool, maxTokens int) (*answerRow, error) {
	kind := "plain"
	if thinking {
		kind = "thinking"
	}
	rec := &callRec{Purpose: "baseline_" + kind}
	text, err := c.text(ctx, baselineSystem, p.Text, thinking, maxTokens, rec)
	call, serr := st.addCall(ctx, 0, c.model, rec)
	if err != nil || serr != nil {
		return nil, errors.Join(err, serr)
	}
	a := &answerRow{Problem: p.ID, Model: c.model, Kind: kind, CallID: call, Text: strings.TrimSpace(text), Valid: true}
	if thinking && rec.Thinking == 0 && rec.Usage.Thought == 0 {
		a.Valid, a.Note = false, "no thinking came back"
	}
	_, err = st.addAnswer(ctx, a)
	return a, err
}

// apply adds the new thoughts under focus and the weight changes.
func apply(ctx context.Context, st *store, c *client, g *graph, focus int, a *args, call int64) error {
	f := g.get(focus)
	for _, nt := range a.Thoughts {
		t := &thought{Num: len(g.nodes) + 1, Kind: nt.Kind, Title: strings.TrimSpace(nt.Title), Content: nt.Content,
			Weight: nt.Weight, Rationale: nt.Rationale, State: nt.Status, Depth: f.Depth + 1, Parents: []int{focus}}
		if t.Kind == "merge" {
			for _, m := range nt.MergeWith {
				if m != focus && g.get(m) != nil && !slices.Contains(t.Parents, m) {
					t.Parents = append(t.Parents, m)
				}
			}
		}
		g.nodes = append(g.nodes, t)
		for _, p := range t.Parents {
			g.get(p).Children = append(g.get(p).Children, t.Num)
		}
		if err := st.addThought(ctx, g, t, call); err != nil {
			return err
		}
		c.logf("#%d → #%d [%s] %s · w %.2f · %s", focus, t.Num, t.Kind, t.Title, t.Weight, t.State)
	}
	if f.State == "open" {
		f.State = "expanded"
		if err := st.setState(ctx, f); err != nil {
			return err
		}
	}
	for _, rw := range a.Reweight {
		t := g.get(rw.ID)
		if t == nil || t.Num == 1 || t.Weight == rw.Weight {
			continue
		}
		old := t.Weight
		t.Weight = rw.Weight
		if err := st.reweight(ctx, call, t, old); err != nil {
			return err
		}
	}
	return nil
}

// pick is the controller's choice. Breadth first: back to #1 while fewer
// than 2 solutions are alive, then a solution that hasn't been expanded
// (the highest weight first), then the open thought with the highest
// weight, then the best thought.
func (g *graph) pick() int {
	alive := 0
	var b *thought
	for _, t := range g.nodes {
		if t.Kind != "solution" || t.State == "dead_end" {
			continue
		}
		alive++
		if t.State == "open" && g.expandable(t) && (b == nil || t.Weight > b.Weight) {
			b = t
		}
	}
	switch {
	case alive < 2 && g.expandable(g.get(1)):
		return 1
	case b != nil:
		return b.Num
	}
	for _, t := range g.nodes {
		if t.Num != 1 && t.State == "open" && g.expandable(t) && (b == nil || t.Weight > b.Weight) {
			b = t
		}
	}
	if b != nil {
		return b.Num
	}
	return g.best()
}

func (g *graph) pickExcept(n int) int {
	t := g.get(n)
	state := t.State
	t.State = "skip"
	defer func() { t.State = state }()
	return g.pick()
}

func checkDefine(a *args) error {
	if len(a.Thoughts) != 1 || a.Thoughts[0].Kind != "problem" {
		return errors.New("add exactly one thought, of kind problem")
	}
	return nil
}

// checkExpand rejects what the schema can't: a second problem, and a merge
// without other thoughts that exist.
func checkExpand(g *graph, focus int) func(*args) error {
	return func(a *args) error {
		var errs []string
		for i, t := range a.Thoughts {
			switch t.Kind {
			case "problem":
				errs = append(errs, fmt.Sprintf("thoughts[%d]: only #1 is a problem; use solution, step, critique or merge", i))
			case "merge":
				ok := false
				for _, m := range t.MergeWith {
					if m != focus && g.get(m) != nil {
						ok = true
					} else if g.get(m) == nil {
						errs = append(errs, fmt.Sprintf("thoughts[%d]: merge_with has #%d, which isn't in the graph", i, m))
					}
				}
				if !ok {
					errs = append(errs, fmt.Sprintf("thoughts[%d]: a merge needs merge_with with at least one other thought from the graph", i))
				}
			}
		}
		if len(errs) > 0 {
			return errors.New(strings.Join(errs, "\n"))
		}
		return nil
	}
}

// parseNext reads "#7", "7" or "finish".
func parseNext(s string) (int, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if strings.Contains(s, "finish") {
		return 0, true
	}
	n, err := strconv.Atoi(strings.TrimPrefix(s, "#"))
	if err != nil {
		return 0, false
	}
	return n, false
}

func clip(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 160 {
		return string(r[:160]) + "…"
	}
	return s
}
