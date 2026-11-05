package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Session is one locked run: the window, every id this run's snapshots gave out, and the store
// of macros and recipes. The guards are SPIKE-039's, the same for the model's own actions and
// for replays.
type Session struct {
	cli        *CLI
	Win        Window
	owner      uint32
	snapN      int
	ids        map[string]string
	nodes      map[string]SnapNode
	counts     map[string]int
	errors     []string
	chars      int
	br         *BrowserNotes
	stop       string
	refPath    string
	store      *Store
	mode       string // macros, recipes, plain
	model      string
	replays    []ReplayRecord
	saved      []string // what the model saved, for the record
	notices    []string
	lastSite   string
	listed     bool            // the saved list went out (in the first list_windows or snapshot, as desktop_start would)
	checkSteps bool            // save_macro checks roles and names against this session's snapshots
	seen       strings.Builder // every snapshot text and every text a wait found, for the step check
	record     bool            // save_macro takes the steps from done (record.go)
	done       []Step          // every step of this session that worked, with the values used
}

var rePasswordName = regexp.MustCompile(`(?i)pass(word|code|wort)|\bpin\b|kennwort|verification code|رمز|گذرواژه`)

// isConsent: a button or link that accepts cookies, terms or other consent.
func isConsent(n SnapNode) bool {
	return (n.Role == "button" || n.Role == "link") && len([]rune(n.Name)) < 60 && reAccept.MatchString(n.Name)
}

// guard returns a refusal for an action on n, or "".
func (s *Session) guard(action string, n SnapNode) string {
	if action != "set_value" && isConsent(n) {
		if s.br != nil {
			s.br.AcceptRefused++
		}
		return "refused: the user doesn't allow accepting cookies, terms or other consent. Decline instead, or stop and tell the user."
	}
	if action == "set_value" || action == "press" || action == "select" {
		for _, st := range n.States {
			if st == "secure" {
				return "refused: this is a password field. Ask the user to type it."
			}
		}
		if rePasswordName.MatchString(n.Name) {
			return "refused: this looks like a password field. Ask the user to type it."
		}
	}
	return ""
}

// press sends one key from the short list to the app. The key goes to the element that has the
// focus, so the guards look at that one and the result names it.
func (s *Session) press(key string) string {
	switch key {
	case "return", "escape", "tab", "backspace":
	default:
		return "refused: press takes only return, escape, tab or backspace"
	}
	target := focusedNode(s.cli, s.Win.ID)
	if target != nil {
		if out := s.guard("press", *target); out != "" {
			return out
		}
	}
	fg := foregroundWindow()
	env, _, _, err := s.cli.Run(15*time.Second, "press", key, "--window-id", s.Win.ID)
	if err != nil {
		return "error: " + err.Error()
	}
	if s.br != nil {
		s.br.Presses++
		if foregroundWindow() != fg {
			s.br.Foreground++
		}
	}
	out := actSummary(env)
	if target != nil {
		out += fmt.Sprintf("; the key went to the focused element: %s %q", target.Role, clip(target.Name, 60))
	}
	return out
}

func (s *Session) waitText(text string, gone bool, seconds float64) string {
	if seconds <= 0 || seconds > 20 {
		seconds = min(max(seconds, 5), 20)
	}
	end := time.Now().Add(time.Duration(seconds * float64(time.Second)))
	for {
		env, _, _, err := s.cli.Run(20*time.Second, "find", "--window-id", s.Win.ID, "--text", text, "--count", "--timeout-ms", "15000")
		found := false
		if err == nil {
			var c struct {
				Count int `json:"count"`
			}
			json.Unmarshal(env.Data, &c)
			found = c.Count > 0
		}
		switch {
		case err != nil && time.Now().After(end):
			return "could not read the window: " + err.Error()
		case err == nil && found != gone:
			if !gone {
				s.seen.WriteString(" " + text)
			}
			return "done: the text is " + map[bool]string{true: "gone", false: "there"}[gone]
		case time.Now().After(end):
			return fmt.Sprintf("timed out after %.0f s", seconds)
		}
		time.Sleep(time.Second)
	}
}

// snapshotText takes a compact snapshot of the locked window and keeps its ids.
func (s *Session) snapshotText() string {
	if s.br != nil {
		return s.browserSnapshot()
	}
	snap, _, _, err := s.cli.snapshot(s.Win.ID, true, "--timeout-ms", "10000")
	if err != nil {
		s.errors = append(s.errors, "snapshot: "+err.Error())
		return "error: " + err.Error()
	}
	if snap.Window.ID != s.Win.ID {
		return "error: the snapshot was not of the chosen window"
	}
	s.snapN++
	c := compact(snap, s.snapN)
	s.seen.WriteString(c.Text)
	for k, v := range c.IDs {
		s.ids[k] = v
		s.nodes[k] = c.Nodes[k]
	}
	return c.Text
}

func (s *Session) call(name string, args json.RawMessage) string {
	s.counts[name]++
	out := s.dispatch(name, args)
	s.chars += len(out)
	return out
}

func (s *Session) savedFor() string {
	var b strings.Builder
	now := time.Now()
	n := 0
	if s.mode == "macros" {
		for _, m := range s.store.Macros {
			if m.App == s.Win.AppName {
				b.WriteString("- " + describeMacro(m, now) + "\n")
				n++
			}
		}
	}
	if s.mode != "plain" {
		for _, r := range s.store.Recipes {
			if r.App == s.Win.AppName {
				fmt.Fprintf(&b, "- recipe %s: %s\n", r.Name, r.Text)
				r.Uses++
				r.LastUsed = now
				n++
			}
		}
	}
	if s.mode == "plain" {
		return ""
	}
	if n == 0 {
		return "\nNothing is saved for this app yet."
	}
	return "\nSaved for this app:\n" + strings.TrimRight(b.String(), "\n")
}

func (s *Session) dispatch(name string, args json.RawMessage) string {
	if !s.locked() {
		return "error: the chosen window is gone. Stop and tell the user."
	}
	switch name {
	case "list_windows":
		s.listed = true
		return fmt.Sprintf("%q (%s) — chosen", s.Win.Title, s.Win.AppName) + s.savedFor()
	case "snapshot":
		if !s.listed && s.mode != "plain" {
			s.listed = true
			return s.snapshotText() + strings.TrimPrefix(s.savedFor(), "\n")
		}
		return s.snapshotText()
	case "act":
		var a struct {
			ID, Action, Text, Key string
		}
		json.Unmarshal(args, &a)
		if a.Action == "press" {
			if s.br != nil {
				s.br.ModelPresses++
			}
			out := s.press(a.Key)
			if strings.HasPrefix(out, "ok") {
				s.did(Step{Do: "press", Key: a.Key})
			}
			return out
		}
		ref, ok := s.ids[a.ID]
		if !ok {
			s.errors = append(s.errors, "act: unknown id "+a.ID)
			return "refused: " + a.ID + " is not an id from your snapshots. Take a snapshot and use an id from it."
		}
		n := s.nodes[a.ID]
		if out := s.guard(a.Action, n); out != "" {
			return out
		}
		var cmd []string
		switch a.Action {
		case "click", "toggle", "expand", "collapse":
			cmd = []string{a.Action, ref}
		case "set_value":
			cmd = []string{"set-value", ref, a.Text}
		case "select":
			cmd = []string{"select", ref, a.Text}
		default:
			return "refused: unknown action " + a.Action
		}
		env, _, _, err := s.cli.Run(15*time.Second, cmd...)
		if err != nil {
			s.errors = append(s.errors, a.Action+": "+err.Error())
			msg := "error: " + err.Error()
			if e, ok := err.(*EnvError); ok && e.Suggestion != "" {
				msg += " (" + clip(e.Suggestion, 200) + ")"
			}
			return msg
		}
		if s.br != nil && a.Action == "click" && reRejectBtn.MatchString(n.Name) {
			s.br.RejectClicked = true
		}
		s.did(Step{Do: a.Action, Role: n.Role, Name: n.Name, Text: a.Text, Optional: a.Action == "click" && reRejectBtn.MatchString(n.Name)})
		return actSummary(env)
	case "wait":
		var w struct {
			Text    string
			Gone    bool
			Seconds float64
		}
		json.Unmarshal(args, &w)
		out := s.waitText(w.Text, w.Gone, w.Seconds)
		if strings.HasPrefix(out, "done") {
			s.did(Step{Do: map[bool]string{true: "wait_gone", false: "wait"}[w.Gone], Text: w.Text})
		}
		return out
	case "run_macro":
		var a struct {
			Name   string   `json:"name"`
			Params []string `json:"params"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return "error: " + err.Error()
		}
		m := s.store.macro(s.Win.AppName, a.Name)
		if m == nil {
			return fmt.Sprintf("error: no macro %q for this app", a.Name)
		}
		params := map[string]string{}
		for _, p := range a.Params {
			k, v, ok := strings.Cut(p, "=")
			if !ok {
				return fmt.Sprintf("error: params are name=value, like %q", "query=some words")
			}
			params[strings.Trim(strings.TrimSpace(k), "{}")] = v
		}
		out, rec := s.replay(m, params, true)
		s.replays = append(s.replays, rec)
		s.store.save()
		return out
	case "save_macro":
		if s.record {
			return s.saveRecorded(args)
		}
		var m Macro
		if err := json.Unmarshal(args, &m); err != nil {
			return "error: " + err.Error()
		}
		err := m.validate()
		if err == nil && s.checkSteps {
			err = s.seenCheck(&m)
		}
		if err != nil {
			s.errors = append(s.errors, "save_macro: "+err.Error())
			return "not saved: " + err.Error()
		}
		for i, p := range m.Params {
			m.Params[i] = strings.Trim(p, "{}")
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
		return fmt.Sprintf("saved macro %s, version %d. The user sees a notice and can undo it.", m.Name, m.Version)
	case "save_recipe":
		var r Recipe
		if err := json.Unmarshal(args, &r); err != nil {
			return "error: " + err.Error()
		}
		if !reName.MatchString(r.Name) || strings.TrimSpace(r.Text) == "" || len([]rune(r.Text)) > 1200 {
			return "not saved: a recipe needs a lower_snake_case name and a text of at most 1,200 characters"
		}
		r.App, r.MadeBy, r.Source, r.Untrusted, r.Made = s.Win.AppName, s.model, "agent", true, time.Now()
		if s.br != nil {
			r.Site = s.lastSite
		}
		s.store.putRecipe(&r)
		s.store.save()
		s.saved = append(s.saved, "recipe:"+r.Name)
		notice := fmt.Sprintf("Recipe saved: %s [Undo] [Edit]", r.Name)
		s.notices = append(s.notices, notice)
		fmt.Println("   notice:", notice)
		return "saved recipe " + r.Name + ". The user sees a notice and can undo it."
	}
	return "error: unknown tool " + name
}

// actSummary keeps what the model needs from an action envelope: the delivery and the state after.
func actSummary(env Envelope) string {
	var d struct {
		Disposition struct {
			Delivery string `json:"delivery"`
		} `json:"disposition"`
		PostState json.RawMessage `json:"post_state"`
	}
	json.Unmarshal(env.Data, &d)
	out := "ok: " + d.Disposition.Delivery
	if len(d.PostState) > 0 && string(d.PostState) != "null" {
		out += "; after: " + clip(string(d.PostState), 300)
	}
	return out
}

// seenCheck: each action step must name an element this session's snapshots showed, by role
// and exact name (case-insensitive, as agent-desktop's --exact). Steps whose name holds a
// {param} are only checked for the role. The error goes back to the model.
func (s *Session) seenCheck(m *Macro) error {
	roles := map[string]bool{}
	names := map[string]bool{}
	for _, n := range s.nodes {
		roles[n.Role] = true
		names[n.Role+"\x00"+strings.ToLower(n.Name)] = true
	}
	for i, st := range m.Steps {
		if (st.Do == "wait" || st.Do == "wait_gone") && !reParam.MatchString(st.Text) && !strings.Contains(strings.ToLower(s.seen.String()), strings.ToLower(st.Text)) {
			return fmt.Errorf("step %d: %q never showed in this session (a wait for it timed out, or no snapshot had it). Wait for a text you saw", i+1, st.Text)
		}
		if !actions[st.Do] && st.Do != "click_each" {
			continue
		}
		if !roles[st.Role] {
			return fmt.Errorf("step %d: your snapshots showed no %s; use the role as the snapshot shows it", i+1, st.Role)
		}
		if st.Do == "click_each" {
			for _, kv := range st.Map {
				_, v, _ := strings.Cut(kv, "=")
				if !names[st.Role+"\x00"+strings.ToLower(v)] {
					return fmt.Errorf("step %d: your snapshots showed no %s named %q", i+1, st.Role, v)
				}
			}
			continue
		}
		if reParam.MatchString(st.Name) {
			continue
		}
		if !names[st.Role+"\x00"+strings.ToLower(st.Name)] {
			if st.Optional {
				continue // a dialog that may not appear may not have been seen either
			}
			return fmt.Errorf("step %d: your snapshots showed no %s named %q. Use the name exactly as the snapshot shows it, or leave name empty when the element has none", i+1, st.Role, st.Name)
		}
	}
	return nil
}
