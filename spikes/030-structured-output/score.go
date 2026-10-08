package main

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// score is how a valid answer compares with the right one.
type score struct {
	Right  bool   `json:"right"`
	Total  int    `json:"total"`   // values compared
	Wrong  int    `json:"wrong"`   // values that differ
	MadeUp int    `json:"made_up"` // a value where the input has none, not found in the input
	Copied int    `json:"copied"`  // a value where the input has none, copied from nearby text
	Detail string `json:"detail,omitempty"`
}

func scoreAnswer(t *Task, v any) score {
	switch t.Kind {
	case "select":
		return scoreSelect(t, v)
	case "decide":
		return scoreDecide(t, v)
	}
	s := score{}
	var diffs []string
	compare("", t.Expect, v, normText(t.Input), &s, &diffs)
	s.Right = s.Wrong == 0
	s.Detail = strings.Join(diffs, "; ")
	return s
}

func scoreSelect(t *Task, v any) score {
	var want, got []int
	for _, e := range t.Expect.([]any) {
		want = append(want, toInt(e))
	}
	for _, p := range v.(map[string]any)["picks"].([]any) {
		got = append(got, toInt(p.(map[string]any)["index"]))
	}
	s := score{Total: len(want)}
	for _, g := range got {
		if !slices.Contains(want, g) {
			s.Wrong++
		}
	}
	s.Right = slices.Equal(want, got)
	switch {
	case s.Right:
	case s.Wrong == 0:
		s.Detail = fmt.Sprintf("right items, wrong order: %v", got)
	default:
		s.Detail = fmt.Sprintf("got %v, want %v", got, want)
	}
	return s
}

func scoreDecide(t *Task, v any) score {
	want := t.Expect.([]any)
	s := score{Total: 3 * len(want)}
	var diffs []string
	for _, a := range v.(map[string]any)["answers"].([]any) {
		m := a.(map[string]any)
		i := toInt(m["index"])
		if i < 0 || i >= len(want) {
			continue
		}
		w := want[i].([]any)
		if m["complaint"] != w[0] {
			s.Wrong++
			diffs = append(diffs, fmt.Sprintf("%d complaint", i))
		}
		if m["topic"] != w[1] {
			s.Wrong++
			diffs = append(diffs, fmt.Sprintf("%d topic %v", i, m["topic"]))
		}
		if d := toInt(m["urgency"]) - toInt(w[2]); d < -1 || d > 1 {
			s.Wrong++
			diffs = append(diffs, fmt.Sprintf("%d urgency %v", i, m["urgency"]))
		}
	}
	s.Right = s.Wrong == 0
	s.Detail = strings.Join(diffs, ", ")
	return s
}

// compare walks the expected value. {$any: [...]} accepts any of its
// values; an empty string counts as null.
func compare(path string, want, got any, in string, s *score, diffs *[]string) {
	if m, ok := want.(map[string]any); ok {
		if alts, ok := m["$any"].([]any); ok && len(m) == 1 {
			for _, a := range alts {
				var sub score
				var d []string
				compare(path, a, got, in, &sub, &d)
				if sub.Wrong == 0 {
					s.Total += max(sub.Total, 1)
					return
				}
			}
			s.Total++
			s.Wrong++
			*diffs = append(*diffs, fmt.Sprintf("%s=%s", path, short(got)))
			return
		}
		gm, _ := got.(map[string]any)
		for k, w := range m {
			compare(path+"/"+k, w, gm[k], in, s, diffs)
		}
		return
	}
	if wl, ok := want.([]any); ok {
		gl, _ := got.([]any)
		for i, w := range wl {
			var g any
			if i < len(gl) {
				g = gl[i]
			}
			compare(fmt.Sprintf("%s/%d", path, i), w, g, in, s, diffs)
		}
		if len(gl) > len(wl) {
			s.Total++
			s.Wrong++
			*diffs = append(*diffs, fmt.Sprintf("%s has %d extra", path, len(gl)-len(wl)))
		}
		return
	}
	s.Total++
	if gs, ok := got.(string); ok && strings.TrimSpace(gs) == "" {
		got = nil
	}
	if want == nil {
		if got != nil {
			s.Wrong++
			if g, ok := got.(string); ok && strings.Contains(in, normText(g)) {
				s.Copied++
				*diffs = append(*diffs, fmt.Sprintf("%s copied %s", path, short(got)))
			} else {
				s.MadeUp++
				*diffs = append(*diffs, fmt.Sprintf("%s made up %s", path, short(got)))
			}
		}
		return
	}
	if !same(want, got) {
		s.Wrong++
		*diffs = append(*diffs, fmt.Sprintf("%s=%s", path, short(got)))
	}
}

func same(want, got any) bool {
	if wf, ok := number(want); ok {
		gf, ok := number(got)
		return ok && math.Abs(wf-gf) <= 1e-9*math.Max(1, math.Abs(wf))
	}
	switch w := want.(type) {
	case bool:
		return got == w
	case string:
		g, ok := got.(string)
		return ok && normText(g) == normText(w)
	}
	return false
}

func number(v any) (float64, bool) {
	switch n := v.(type) {
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case float64:
		return n, true
	case int:
		return float64(n), true
	}
	return 0, false
}

// normText ignores case, spaces, the zero-width non-joiner and the Arabic
// forms of Persian letters and digits.
func normText(s string) string {
	s = norm.NFC.String(strings.ToLower(s))
	r := strings.NewReplacer("‌", "", "ي", "ی", "ك", "ک", "×", "x",
		"۰", "0", "۱", "1", "۲", "2", "۳", "3", "۴", "4", "۵", "5", "۶", "6", "۷", "7", "۸", "8", "۹", "9")
	s = r.Replace(s)
	return strings.Join(strings.Fields(s), "")
}

func short(v any) string {
	b, _ := json.Marshal(v)
	r := []rune(string(b))
	if len(r) > 60 {
		return string(r[:60]) + "…"
	}
	return string(r)
}
