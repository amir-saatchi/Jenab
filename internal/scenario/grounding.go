package scenario

// SPIKE-032's number check in the runner: its verdicts in each run, the
// cost of the notice's extra requests, two checks and the report's table.

import (
	"fmt"
	"slices"
	"strings"
)

// GroundResult is one verdict of the number check in a run.
type GroundResult struct {
	Turn      int      `json:"turn"`
	Request   int      `json:"request"` // the checked answer's request, from 1, as in the transcript
	Checked   int      `json:"checked"` // numbers checked
	Unsourced []string `json:"unsourced,omitempty"`
	Saved     int      `json:"saved,omitempty"`      // of them, in what write tools saved
	SavedLeft int      `json:"saved_left,omitempty"` // of them, saved before the notice and still there
	Quoted    int      `json:"quoted,omitempty"`     // numbers left out as quoted from the notice
	Pct       int      `json:"pct,omitempty"`        // numbers sourced only as a percentage read the other way
	Fired     bool     `json:"fired"`
	Told      bool     `json:"told,omitempty"`  // the model got the notice
	After     bool     `json:"after,omitempty"` // the answer after the notice, in the same turn
	Micros    int64    `json:"micros"`          // how long the check took
}

// GroundExtra is what the requests after the notice in its turn cost:
// the same_turn mode's correction.
type GroundExtra struct {
	Requests int     `json:"requests"`
	Prompt   int     `json:"prompt_tokens"`
	Output   int     `json:"output_tokens"`
	Seconds  float64 `json:"seconds"`
}

func (rec *record) groundResults() []GroundResult {
	var out []GroundResult
	for _, c := range rec.checks {
		out = append(out, GroundResult{Turn: c.turn, Request: c.request + 1, Checked: c.Checked, Unsourced: c.Unsourced, Saved: c.Saved,
			SavedLeft: c.SavedLeft, Quoted: c.Quoted, Pct: c.Pct, Fired: c.Fired, Told: c.Told, After: c.After, Micros: c.Took.Microseconds()})
	}
	return out
}

// groundExtra is the cost of the requests after the first notice in its
// turn, or nil without a notice. In the next_turn mode the notice adds no
// request, so there is none.
func (rec *record) groundExtra() *GroundExtra {
	i := slices.IndexFunc(rec.checks, func(c groundCheck) bool { return c.Told })
	if i < 0 {
		return nil
	}
	c := rec.checks[i]
	x := &GroundExtra{}
	for j := c.request + 1; j < len(rec.requests) && rec.reqTurn[j] == c.turn; j++ {
		q := rec.requests[j]
		x.Requests++
		x.Prompt += promptOf(q.Usage)
		x.Output += q.Usage.Output
		x.Seconds += q.Took.Seconds()
	}
	return x
}

// groundFired is the grounding_fired and grounding_quiet checks: whether
// the check fired on an answer the turns sel picks gave before any
// notice.
func (rec *record) groundFired(a Assert) (string, string, float64) {
	if rec.grounding == nil || rec.grounding.Mode == "" {
		return Skip, "the number check is off", 0
	}
	in := rec.inTurns(a.Turn)
	var fired, quiet []string
	worst := 0
	for _, c := range rec.checks {
		if !in(c.turn) || c.After {
			continue
		}
		worst = max(worst, len(c.Unsourced))
		s := fmt.Sprintf("turn %d: %d of %d without a source", c.turn, len(c.Unsourced), c.Checked)
		if len(c.Unsourced) > 0 {
			s += " (" + clip(strings.Join(c.Unsourced, ", "), 80) + ")"
		}
		if c.Fired {
			fired = append(fired, s)
		} else {
			quiet = append(quiet, s)
		}
	}
	if len(fired)+len(quiet) == 0 {
		return Skip, "no answer was checked", 0
	}
	why := strings.Join(append(fired, quiet...), "; ")
	ok := len(fired) > 0
	if a.Type == "grounding_quiet" {
		ok = !ok
	}
	st, why, _ := pf(ok, why)
	return st, why, float64(worst)
}

// groundTranscript is the transcript's section on the check.
func (rec *record) groundTranscript(b *strings.Builder) {
	if rec.grounding == nil || rec.grounding.Mode == "" {
		return
	}
	g := rec.grounding.agent()
	fmt.Fprintf(b, "\n## Number check\n\nMode %s, K %d.\n\n", g.Mode, g.Threshold())
	if v := rec.grounding.Variant(); v != "" {
		fmt.Fprintf(b, "Round 2: %s.\n\n", v)
	}
	if len(rec.checks) == 0 {
		b.WriteString("No answer was checked.\n")
	}
	for _, c := range rec.checks {
		var marks []string
		if c.Fired {
			marks = append(marks, "fired")
		}
		if c.Told {
			marks = append(marks, "told")
		}
		if c.After {
			marks = append(marks, "after the notice")
		}
		mark := ""
		if len(marks) > 0 {
			mark = " (" + strings.Join(marks, ", ") + ")"
		}
		fmt.Fprintf(b, "- Turn %d, request %d%s: %d of %d numbers without a source", c.turn, c.request+1, mark, len(c.Unsourced), c.Checked)
		if c.Saved > 0 {
			fmt.Fprintf(b, ", %d of them saved", c.Saved)
		}
		if c.SavedLeft > 0 {
			fmt.Fprintf(b, ", %d left in what was saved before the notice", c.SavedLeft)
		}
		if c.Quoted > 0 {
			fmt.Fprintf(b, ", %d quoted from the notice", c.Quoted)
		}
		if c.Pct > 0 {
			fmt.Fprintf(b, ", %d percentages read the other way", c.Pct)
		}
		if len(c.Unsourced) > 0 {
			b.WriteString(": " + clip(strings.Join(c.Unsourced, ", "), 400))
		}
		fmt.Fprintf(b, ". Took %d µs.\n", c.Took.Microseconds())
	}
}

// groundMarkdown is the report's table of the check: for each scenario
// and model, the unsourced numbers of each run's first checked answer,
// with * where the check fired.
func (rep *Report) groundMarkdown(b *strings.Builder) {
	if !slices.ContainsFunc(rep.Runs, func(r Run) bool { return r.GroundMode != "" }) {
		return
	}
	b.WriteString("\n## Number check\n\nUnsourced numbers in each run's first checked answer, * where the check fired; → the answer after the notice.\n\n| Scenario |")
	for _, m := range rep.Models {
		b.WriteString(" " + m + " |")
	}
	b.WriteString("\n|---|" + strings.Repeat("---|", len(rep.Models)) + "\n")
	for _, sc := range rep.scenarios() {
		b.WriteString("| " + sc + " |")
		for _, m := range rep.Models {
			var cells []string
			for _, r := range rep.Runs {
				if r.Model == m && r.Scenario == sc && r.Skipped == "" {
					cells = append(cells, r.groundCell())
				}
			}
			b.WriteString(" " + strings.Join(cells, ", ") + " |")
		}
		b.WriteString("\n")
	}
}

// groundCell is a run's verdicts: each answer's unsourced count, * if it
// fired, → the count after the notice, and / between turns.
func (r Run) groundCell() string {
	s := ""
	for i, g := range r.Grounding {
		if g.After {
			s += fmt.Sprintf("→%d", len(g.Unsourced))
			continue
		}
		if i > 0 {
			s += "/"
		}
		s += fmt.Sprint(len(g.Unsourced))
		if g.Fired {
			s += "*"
		}
	}
	if s == "" {
		return "–"
	}
	return s
}
