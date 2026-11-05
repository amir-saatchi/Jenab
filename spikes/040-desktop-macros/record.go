package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Recorded macros (-record): Jenab keeps every step of the session that worked (the model's
// actions, its waits that found their text, and the steps of a replay before it failed), and
// save_macro only names the macro, says which values change and what to log. The steps come
// from what happened, by role and exact name, so the model can't misname an element or keep a
// step that failed.

const recordSkill = `

Saved macros and recipes:
- list_windows also shows the macros and recipes saved for this app.
- If a macro fits the task, run it with run_macro first, with the task's values as params. It replays its steps without you and returns its log. If the result says the macro is old, check the result in the window with a snapshot before you answer.
- If a replay fails, it returns its log and a snapshot of the window: carry on by hand from there. When the task worked, save the fixed macro under the same name.
- A recipe is a short note on how to do a task in this app. Follow it, but still check each step.
- When you did a task by hand and it worked, save it before you answer. Don't ask the user: they see a notice and can undo it.
  1. save_macro with a name, a one-sentence description, the values that change from task to task as params (each with the value you used this time), and log: the role (and a part of the name) of the elements that hold what the user asked for. Jenab takes the steps from your actions that worked, in order, and the result lists them. If one of them was a mistake, save again with its number in skip.
  2. save_recipe with two to five sentences on how to do this task in this app, for when the macro breaks.`

var recordTools = []any{
	macroTools[0], // run_macro
	map[string]any{"name": "save_macro", "description": "Save the task you just did, so it can be replayed without you. Jenab takes the steps from your actions that worked. The same name replaces the old version.",
		"parameters": map[string]any{"type": "object", "required": []string{"name", "description", "params", "log"}, "properties": map[string]any{
			"name":        map[string]any{"type": "string", "description": "lower_snake_case, like google_search"},
			"description": map[string]any{"type": "string", "description": "What it does, in one sentence"},
			"params": map[string]any{"type": "array", "items": map[string]any{"type": "string"},
				"description": `The values that change from task to task, as name=value with the value you used this time, like ["query=blue folders"]`},
			"log": map[string]any{"type": "array", "items": map[string]any{"type": "object", "required": []string{"role"}, "properties": map[string]any{
				"role":  map[string]any{"type": "string", "description": "The role of the elements that hold the result: statictext, link, textfield ..."},
				"name":  map[string]any{"type": "string", "description": "A part of their name, or empty for all"},
				"limit": map[string]any{"type": "integer", "description": "How many to record (default 10, at most 50)"},
			}}, "description": "What a replay records and returns"},
			"skip": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "Numbers of recorded steps to leave out (from a previous save's result)"},
		}}},
	macroTools[2], // save_recipe
}

// digitNames are the names keypad buttons have (Calculator's, in English): a run of clicks on
// them that spells a param's value becomes one click_each step.
var digitNames = map[string]rune{"Zero": '0', "One": '1', "Two": '2', "Three": '3', "Four": '4', "Five": '5', "Six": '6', "Seven": '7', "Eight": '8', "Nine": '9'}

// did keeps one step that worked. A set_value right after another on the same element replaces it.
func (s *Session) did(st Step) {
	if n := len(s.done); n > 0 && st.Do == "set_value" && s.done[n-1].Do == "set_value" && s.done[n-1].Role == st.Role && s.done[n-1].Name == st.Name {
		s.done[n-1] = st
		return
	}
	s.done = append(s.done, st)
}

func (s *Session) saveRecorded(args json.RawMessage) string {
	var a struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Params      []string `json:"params"`
		Log         []Step   `json:"log"`
		Skip        []int    `json:"skip"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "error: " + err.Error()
	}
	listing := func(steps []Step) string {
		var b strings.Builder
		for i, st := range steps {
			fmt.Fprintf(&b, "%d %s\n", i+1, st)
		}
		return b.String()
	}
	skip := map[int]bool{}
	for _, k := range a.Skip {
		skip[k] = true
	}
	var steps []Step
	for i, st := range s.done {
		if !skip[i+1] {
			steps = append(steps, st)
		}
	}
	if len(steps) == 0 {
		return "not saved: no action of this session worked yet, so there is nothing to record"
	}
	type kv struct{ k, v string }
	var params []kv
	for _, p := range a.Params {
		k, v, ok := strings.Cut(p, "=")
		k = strings.Trim(strings.TrimSpace(k), "{}")
		if !ok || k == "" || v == "" {
			return fmt.Sprintf("not saved: params are name=value with the value you used this time, like %q", "query=blue folders")
		}
		params = append(params, kv{k, v})
	}
	// Longer values first, so a value that holds another one is replaced whole.
	sort.SliceStable(params, func(i, j int) bool { return len(params[i].v) > len(params[j].v) })
	used := map[string]bool{}
	var out []Step
	for i := 0; i < len(steps); i++ {
		st := steps[i]
		// A run of keypad clicks that spells a value becomes click_each.
		if st.Do == "click" {
			j, word := i, ""
			for j < len(steps) && steps[j].Do == "click" && steps[j].Role == st.Role {
				r, ok := digitNames[steps[j].Name]
				if !ok {
					break
				}
				word += string(r)
				j++
			}
			matched := false
			for _, p := range params {
				if word != "" && word == p.v {
					m := []string{}
					for name, r := range digitNames {
						m = append(m, string(r)+"="+name)
					}
					sort.Strings(m)
					out = append(out, Step{Do: "click_each", Role: st.Role, Text: "{" + p.k + "}", Map: m})
					used[p.k] = true
					i = j - 1
					matched = true
					break
				}
			}
			if matched {
				continue
			}
		}
		for _, p := range params {
			if st.Text != "" && strings.Contains(st.Text, p.v) {
				st.Text = strings.ReplaceAll(st.Text, p.v, "{"+p.k+"}")
				used[p.k] = true
			}
			if st.Name != "" && strings.Contains(st.Name, p.v) {
				st.Name = strings.ReplaceAll(st.Name, p.v, "{"+p.k+"}")
				used[p.k] = true
			}
		}
		out = append(out, st)
	}
	for _, p := range params {
		if !used[p.k] {
			return fmt.Sprintf("not saved: the value of %s (%q) is in none of the steps that worked:\n%sGive the value as you typed or clicked it, or leave the param out.", p.k, p.v, listing(steps))
		}
	}
	if len(a.Log) == 0 {
		return "not saved: say in log which elements hold the result, so a replay can return it"
	}
	for _, l := range a.Log {
		st := Step{Do: "log", Role: l.Role, Name: l.Name, Limit: l.Limit}
		for _, p := range params {
			if st.Name != "" && strings.Contains(st.Name, p.v) {
				st.Name = strings.ReplaceAll(st.Name, p.v, "{"+p.k+"}")
			}
		}
		out = append(out, st)
	}
	m := Macro{Name: a.Name, Description: a.Description, Steps: out}
	for _, p := range params {
		m.Params = append(m.Params, p.k)
	}
	sort.Strings(m.Params)
	if err := m.validate(); err != nil {
		s.errors = append(s.errors, "save_macro: "+err.Error())
		return "not saved: " + err.Error()
	}
	m.App, m.MadeBy, m.Untrusted = s.Win.AppName, s.model, true
	m.Made, m.LastWorked = time.Now(), time.Now()
	if s.br != nil {
		m.Site = s.lastSite
	}
	s.store.putMacro(&m)
	s.store.save()
	s.saved = append(s.saved, "macro:"+m.Name)
	notice := fmt.Sprintf("Macro saved: %s (%d steps) [Undo] [Edit]", m.Name, len(m.Steps))
	s.notices = append(s.notices, notice)
	fmt.Println("   notice:", notice)
	msg := fmt.Sprintf("saved macro %s, version %d, with these steps:\n%s", m.Name, m.Version, listing(m.Steps))
	if len(steps) != len(m.Steps)-len(a.Log) {
		msg += "Your actions as recorded (the numbers for skip):\n" + listing(steps)
	}
	return msg + "The user sees a notice and can undo it."
}
