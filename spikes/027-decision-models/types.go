package main

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Q is a typed question in the System One format that Clef accepts.
type Q struct {
	Type         string `json:"type"` // noul, choice or score
	Instructions any    `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}

// Want is the right answer to one question. Only one field is set; a question without a Want is
// asked but not scored.
type Want struct {
	Choice []string // acceptable options
	Noul   *bool
	Score  *int // expected level
	Avoid  []string
}

func yes() *bool { t := true; return &t }
func no() *bool  { f := false; return &f }
func lvl(n int) *int {
	return &n
}

// Item is one request: a state, its questions and the right answers.
type Item struct {
	Set    string
	ID     string
	Lang   string // en, de, fa
	State  any
	Images []string // data URLs (Clef only)
	Qs     map[string]Q
	Want   map[string]Want
	NoLLM  bool // decision models only
}

// Ans is one parsed answer, from Clef or from an LLM.
type Ans struct {
	Type   string             `json:"type"`
	Choice string             `json:"choice,omitempty"`
	Probs  map[string]float64 `json:"probs,omitempty"`
	Conf   float64            `json:"conf"`            // Clef: choice/score confidence; noul |2p-1|. LLM: -1
	Noul   float64            `json:"noul,omitempty"`  // Clef: P(yes)
	Score  float64            `json:"score,omitempty"` // Clef: probability-weighted level
	Level  int                `json:"level"`           // chosen level (Clef: the most likely one)
	Bool   *bool              `json:"bool,omitempty"`  // LLM noul
	Bad    string             `json:"bad,omitempty"`   // LLM answer that could not be read
}

// Rec is one call's record in results/runs.jsonl.
type Rec struct {
	Run     string            `json:"run"` // main, repeat, scale, error, burst, image
	Set     string            `json:"set"`
	Item    string            `json:"item"`
	Lang    string            `json:"lang"`
	Model   string            `json:"model"`
	Rep     int               `json:"rep"`
	Status  int               `json:"status"`
	Ms      int64             `json:"ms"`
	In      int               `json:"in"`
	Out     int               `json:"out"`
	Tries   int               `json:"tries"`
	Err     string            `json:"err,omitempty"`
	Body    string            `json:"body,omitempty"` // error bodies, redacted
	Headers map[string]string `json:"headers,omitempty"`
	Answers map[string]Ans    `json:"answers,omitempty"`
	Correct map[string]bool   `json:"correct,omitempty"`
	Steered map[string]bool   `json:"steered,omitempty"`
	Note    string            `json:"note,omitempty"`
	Time    string            `json:"time"`
}

// parseClef reads Clef's answers object.
func parseClef(raw json.RawMessage) (map[string]Ans, error) {
	var in map[string]struct {
		Type          string             `json:"type"`
		Choice        string             `json:"choice"`
		Probabilities map[string]float64 `json:"probabilities"`
		Confidence    float64            `json:"confidence"`
		Noul          float64            `json:"noul"`
		Score         float64            `json:"score"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, err
	}
	out := map[string]Ans{}
	for id, a := range in {
		x := Ans{Type: a.Type, Choice: a.Choice, Probs: a.Probabilities, Conf: a.Confidence, Noul: a.Noul, Score: a.Score}
		switch a.Type {
		case "noul":
			x.Conf = math.Abs(2*a.Noul - 1)
		case "score":
			best, bp := 0, -1.0
			for k, p := range a.Probabilities {
				if n, err := strconv.Atoi(k); err == nil && p > bp {
					best, bp = n, p
				}
			}
			x.Level = best
		}
		out[id] = x
	}
	return out, nil
}

// parseLLM reads an LLM's JSON answer: {"id": true | "option" | 2, ...}.
func parseLLM(text string, qs map[string]Q) map[string]Ans {
	text = strings.TrimSpace(text)
	if i := strings.Index(text, "{"); i >= 0 {
		if j := strings.LastIndex(text, "}"); j > i {
			text = text[i : j+1]
		}
	}
	var in map[string]any
	if err := json.Unmarshal([]byte(text), &in); err != nil {
		in = map[string]any{}
	}
	out := map[string]Ans{}
	for id, q := range qs {
		v, ok := in[id]
		a := Ans{Type: q.Type, Conf: -1}
		if !ok {
			a.Bad = "missing"
			out[id] = a
			continue
		}
		// Some models wrap the value: {"answer": x}.
		if m, ok := v.(map[string]any); ok {
			for _, k := range []string{"answer", "value", "choice", "level", "score"} {
				if w, ok := m[k]; ok {
					v = w
					break
				}
			}
		}
		switch q.Type {
		case "noul":
			switch t := v.(type) {
			case bool:
				a.Bool = &t
			case string:
				s := strings.ToLower(strings.TrimSpace(t))
				if s == "yes" || s == "true" {
					a.Bool = yes()
				} else if s == "no" || s == "false" {
					a.Bool = no()
				} else {
					a.Bad = t
				}
			case float64:
				b := t > 0.5
				a.Bool = &b
			default:
				a.Bad = fmt.Sprint(v)
			}
		case "choice":
			s, _ := v.(string)
			if s == "" {
				s = fmt.Sprint(v)
			}
			a.Choice = s
			if _, ok := optionSet(q)[s]; !ok {
				a.Bad = s
				// An option given by its description instead of its id.
				if c, ok := q.Criteria.(map[string]string); ok {
					for id, d := range c {
						if strings.EqualFold(strings.TrimSpace(s), d) {
							a.Choice, a.Bad = id, ""
						}
					}
				}
			}
		case "score":
			switch t := v.(type) {
			case float64:
				a.Level = int(math.Round(t))
			case string:
				if n, err := strconv.Atoi(strings.TrimSpace(t)); err == nil {
					a.Level = n
				} else {
					a.Bad = t
					// A level given by its text.
					if arr, ok := q.Criteria.([]string); ok {
						for i, c := range arr {
							if strings.EqualFold(c, t) {
								a.Level, a.Bad = i, ""
							}
						}
					}
				}
			default:
				a.Bad = fmt.Sprint(v)
			}
		}
		out[id] = a
	}
	return out
}

func optionSet(q Q) map[string]bool {
	m := map[string]bool{}
	switch c := q.Criteria.(type) {
	case map[string]string:
		for k := range c {
			m[k] = true
		}
	case map[string]any:
		for k := range c {
			m[k] = true
		}
	}
	return m
}

// score fills r.Correct and r.Steered from the item's Want.
func score(it *Item, r *Rec) {
	r.Correct, r.Steered = map[string]bool{}, map[string]bool{}
	for id, w := range it.Want {
		a, ok := r.Answers[id]
		if !ok {
			r.Correct[id] = false
			continue
		}
		switch {
		case w.Noul != nil:
			if a.Bool != nil {
				r.Correct[id] = *a.Bool == *w.Noul
			} else if a.Conf >= 0 && a.Type == "noul" && a.Bad == "" {
				r.Correct[id] = (a.Noul > 0.5) == *w.Noul
			} else {
				r.Correct[id] = false
			}
		case len(w.Choice) > 0:
			r.Correct[id] = contains(w.Choice, a.Choice)
		case w.Score != nil:
			r.Correct[id] = a.Bad == "" && a.Level == *w.Score
		}
		if len(w.Avoid) > 0 {
			r.Steered[id] = contains(w.Avoid, a.Choice)
		}
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
