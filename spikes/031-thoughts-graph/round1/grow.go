package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

type options struct {
	Mode        string // siblings or one
	K           int    // thoughts per call in siblings mode
	MaxThoughts int
	MaxDepth    int
	Baseline    bool
}

// grow builds one graph: define the problem, expand one thought per call
// until the model says finish or the budget is used, then write the answer.
// Every thought, edge, weight change and call is stored as it happens.
func grow(ctx context.Context, st *store, c *client, p problem, o options) (*graph, error) {
	g := &graph{Problem: p, Model: c.model, Mode: o.Mode, MaxDepth: o.MaxDepth}
	if err := st.newGraph(ctx, g, o); err != nil {
		return nil, err
	}
	stop := func(why string, err error) (*graph, error) {
		g.Finish = why
		return g, errors.Join(err, st.finish(context.WithoutCancel(ctx), g))
	}

	rec := &callRec{Purpose: "define"}
	a, err := c.thoughts(ctx, defineMsg(p), 1, checkDefine, rec)
	call, serr := st.addCall(ctx, g, rec)
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
		k := o.K
		if o.Mode == "one" {
			k = 1
		}
		k = min(k, o.MaxThoughts-len(g.nodes))
		prompt := expandMsg(g, focus, k, o.MaxThoughts-len(g.nodes))
		rec := &callRec{Purpose: "expand", Focus: focus, IndexChars: len(g.index())}
		a, err := c.thoughts(ctx, prompt, k, checkExpand(g, focus), rec)
		call, serr := st.addCall(ctx, g, rec)
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
			if n := g.bestExcept(focus); n != 0 {
				focus = n
			}
			continue
		}
		fails = 0
		if err := apply(ctx, st, c, g, focus, a, call); err != nil {
			return stop("broke", err)
		}
		next, finish := parseNext(a.Next)
		switch {
		case finish && g.has("solution"):
			c.logf("model: finish")
			g.Finish = "model"
		case finish:
			g.EarlyFinish++
			focus = g.best()
		case g.expandable(g.get(next)):
			g.NextPicked++
			focus = next
		default:
			g.NextInvalid++
			focus = g.best()
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
	answer, err := c.text(ctx, concludeSystem, concludeMsg(g), rec)
	if _, serr := st.addCall(ctx, g, rec); serr != nil || err != nil {
		g.Finish += "+no_answer"
		return stop(g.Finish, errors.Join(err, serr))
	}
	g.Answer, g.Used = usedLine(answer)

	if o.Baseline {
		rec = &callRec{Purpose: "baseline"}
		g.Baseline, err = c.text(ctx, baselineSystem, p.Text, rec)
		if _, serr := st.addCall(ctx, g, rec); serr != nil || err != nil {
			c.logf("baseline failed: %v", errors.Join(err, serr))
		}
	}
	return g, st.finish(ctx, g)
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

func (g *graph) bestExcept(n int) int {
	t := g.get(n)
	state := t.State
	t.State = "skip"
	defer func() { t.State = state }()
	return g.best()
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
						errs = append(errs, fmt.Sprintf("thoughts[%d]: merge_with has #%d, which isn't in the index", i, m))
					}
				}
				if !ok {
					errs = append(errs, fmt.Sprintf("thoughts[%d]: a merge needs merge_with with at least one other thought from the index", i))
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
