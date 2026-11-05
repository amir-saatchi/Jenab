package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ReplayRecord is what the harness keeps of one replay.
type ReplayRecord struct {
	Macro    string   `json:"macro"`
	Version  int      `json:"version"`
	OK       bool     `json:"ok"`
	FailedAt int      `json:"failed_at,omitempty"`
	Reason   string   `json:"reason,omitempty"`
	Skipped  int      `json:"skipped"`
	Steps    int      `json:"steps"`
	Seconds  float64  `json:"seconds"`
	OldNote  bool     `json:"old_note"` // the result said the macro was old
	Log      []string `json:"log"`
	ByModel  bool     `json:"by_model"` // run_macro from the model (false: the harness's own check)
}

type found struct {
	Ref    string   `json:"ref_id"`
	Role   string   `json:"role"`
	Name   string   `json:"name"`
	Value  *string  `json:"value"`
	States []string `json:"states"`
}

func (f found) node() SnapNode {
	n := SnapNode{Role: f.Role, Name: f.Name, States: f.States, Ref: f.Ref}
	if f.Value != nil {
		n.Value = *f.Value
	}
	return n
}

// find asks agent-desktop for elements of the locked window by role and name. exact: the whole
// name (case-insensitive, agent-desktop's --exact); otherwise a part of it.
func (s *Session) find(role, name string, exact bool, limit int) ([]found, error) {
	args := []string{"find", "--window-id", s.Win.ID, "--timeout-ms", "15000"}
	if role != "" {
		args = append(args, "--role", role)
	}
	if name != "" {
		args = append(args, "--name", name)
		if exact {
			args = append(args, "--exact")
		}
	}
	args = append(args, "--limit", fmt.Sprint(limit))
	env, _, _, err := s.cli.Run(20*time.Second, args...)
	if err != nil {
		if e, ok := err.(*EnvError); ok && (e.Code == "ELEMENT_NOT_FOUND" || e.Code == "NOT_FOUND") {
			return nil, nil
		}
		return nil, err
	}
	var d struct {
		Matches []found `json:"matches"`
	}
	json.Unmarshal(env.Data, &d)
	return d.Matches, nil
}

// findOne waits up to wait for an element with a ref.
func (s *Session) findOne(role, name string, wait time.Duration) (*found, error) {
	end := time.Now().Add(wait)
	var lastErr error
	for {
		ms, err := s.find(role, name, true, 5)
		lastErr = err
		for _, m := range ms {
			if m.Ref != "" {
				return &m, nil
			}
		}
		if time.Now().After(end) {
			return nil, lastErr
		}
		time.Sleep(700 * time.Millisecond)
	}
}

// pageOrigin is a browser window's page origin, read from its document's value.
func (s *Session) pageOrigin() (origin_ string, path string) {
	ms, err := s.find("document", "", false, 10)
	if err != nil {
		return "", ""
	}
	for _, m := range ms {
		if m.Value != nil && (strings.HasPrefix(*m.Value, "http") || strings.HasPrefix(*m.Value, "about:")) {
			v := *m.Value
			o := origin(v)
			return o, strings.TrimPrefix(v, o)
		}
	}
	return "", ""
}

func (s *Session) locked() bool {
	h := hwndOf(s.Win.ID)
	return isWindow(h) && windowPID(h) == s.owner
}

// replay runs a macro's steps in the locked window. It returns the text for the model.
func (s *Session) replay(m *Macro, params map[string]string, byModel bool) (string, ReplayRecord) {
	start := time.Now()
	rec := ReplayRecord{Macro: m.Name, Version: m.Version, ByModel: byModel}
	var done []string
	fail := func(i int, st Step, reason string) (string, ReplayRecord) {
		rec.FailedAt, rec.Reason, rec.Seconds = i+1, reason, time.Since(start).Seconds()
		m.Runs++
		m.Failures++
		m.FailStreak++
		var b strings.Builder
		fmt.Fprintf(&b, "failed at step %d of %d (%s): %s\n", i+1, len(m.Steps), st, reason)
		if len(done) > 0 {
			b.WriteString("steps before it:\n" + strings.Join(done, "\n") + "\n")
		}
		if len(rec.Log) > 0 {
			b.WriteString("log so far:\n" + strings.Join(rec.Log, "\n") + "\n")
		}
		if s.stop == "" && s.locked() {
			b.WriteString("The window now:\n" + s.snapshotText())
			b.WriteString("Carry on by hand from here; the ids above work with act. When the task worked, save the fixed macro under the same name.")
		}
		return b.String(), rec
	}
	for _, p := range m.Params {
		p = strings.Trim(p, "{}")
		if _, ok := params[p]; !ok {
			return fmt.Sprintf("refused: give params %v (missing %q)", m.Params, p), rec
		}
	}
	site := m.Site
	for i, st := range m.Steps {
		rec.Steps = i + 1
		if !s.locked() {
			return fail(i, st, "the chosen window is gone. Stop and tell the user.")
		}
		name, text := fill(st.Name, params), fill(st.Text, params)
		wait := time.Duration(st.Seconds * float64(time.Second))
		if wait <= 0 {
			wait = 10 * time.Second
			if st.Optional {
				wait = 5 * time.Second
			}
		}
		wait = min(wait, 30*time.Second)
		line := fmt.Sprintf("%d %s", i+1, st)
		switch {
		case actions[st.Do]:
			el, err := s.findOne(st.Role, name, wait)
			if el == nil {
				if st.Optional {
					rec.Skipped++
					s.did(Step{Do: st.Do, Role: st.Role, Name: name, Text: text, Optional: true})
					done = append(done, line+" — skipped, not there")
					continue
				}
				why := fmt.Sprintf("no %s named %q after %.0f s", st.Role, name, wait.Seconds())
				if err != nil {
					why += " (" + err.Error() + ")"
				}
				return fail(i, st, why)
			}
			if out := s.guard(st.Do, el.node()); out != "" {
				return fail(i, st, out)
			}
			var cmd []string
			switch st.Do {
			case "set_value":
				cmd = []string{"set-value", el.Ref, text}
			case "select":
				cmd = []string{"select", el.Ref, text}
			default:
				cmd = []string{st.Do, el.Ref}
			}
			if _, _, _, err := s.cli.Run(15*time.Second, cmd...); err != nil {
				return fail(i, st, err.Error())
			}
			s.did(Step{Do: st.Do, Role: st.Role, Name: name, Text: text, Optional: st.Optional})
			done = append(done, line+" — ok")
		case st.Do == "press":
			out := s.press(st.Key)
			if strings.HasPrefix(out, "refused") || strings.HasPrefix(out, "error") {
				return fail(i, st, out)
			}
			s.did(Step{Do: "press", Key: st.Key})
			done = append(done, line+" — "+out)
		case st.Do == "click_each":
			names := map[rune]string{}
			for _, kv := range st.Map {
				k, v, _ := strings.Cut(kv, "=")
				names[[]rune(k)[0]] = v
			}
			for _, r := range text {
				n, ok := names[r]
				if !ok {
					return fail(i, st, fmt.Sprintf("the map has no entry for %q", string(r)))
				}
				el, err := s.findOne(st.Role, n, wait)
				if el == nil {
					why := fmt.Sprintf("no %s named %q for %q", st.Role, n, string(r))
					if err != nil {
						why += " (" + err.Error() + ")"
					}
					return fail(i, st, why)
				}
				if out := s.guard("click", el.node()); out != "" {
					return fail(i, st, out)
				}
				if _, _, _, err := s.cli.Run(15*time.Second, "click", el.Ref); err != nil {
					return fail(i, st, err.Error())
				}
				s.did(Step{Do: "click", Role: st.Role, Name: n})
			}
			done = append(done, line+" — ok")
		case st.Do == "wait" || st.Do == "wait_gone":
			if out := s.waitText(text, st.Do == "wait_gone", wait.Seconds()); !strings.HasPrefix(out, "done") {
				if st.Optional {
					rec.Skipped++
					s.did(Step{Do: st.Do, Text: text, Optional: true})
					done = append(done, line+" — skipped: "+out)
					continue
				}
				return fail(i, st, out)
			}
			s.did(Step{Do: st.Do, Text: text, Optional: st.Optional})
			done = append(done, line+" — ok")
		case st.Do == "log":
			limit := st.Limit
			if limit <= 0 {
				limit = 10
			}
			limit = min(limit, 50)
			ms, err := s.find(st.Role, name, false, limit)
			if err != nil {
				return fail(i, st, err.Error())
			}
			rec.Log = append(rec.Log, fmt.Sprintf("step %d, log %s %q: %d found", i+1, st.Role, name, len(ms)))
			for _, f := range ms {
				l := "- " + f.Role
				if f.Name != "" {
					l += fmt.Sprintf(" %q", clip(f.Name, 200))
				}
				if f.Value != nil && *f.Value != "" && *f.Value != f.Name && f.Role != "link" && f.Role != "document" {
					l += fmt.Sprintf(" = %q", clip(*f.Value, 200))
				}
				rec.Log = append(rec.Log, l)
			}
			done = append(done, line+fmt.Sprintf(" — %d recorded", len(ms)))
		}
		// In a browser, the site lock and the CAPTCHA check run after every action.
		if s.br != nil && st.Do != "log" && st.Do != "wait" && st.Do != "wait_gone" {
			o, path := s.pageOrigin()
			if strings.Contains(path, "/sorry/") {
				s.br.Captcha = true
				s.br.CaptchaWhy = o + clip(path, 40)
				s.stop = "captcha"
				return fail(i, st, "the page asks to prove you are not a robot. Stop: the user has to handle this.")
			}
			if strings.HasPrefix(o, "http") {
				if site == "" {
					site = o
				} else if o != site {
					return fail(i, st, fmt.Sprintf("the page left %s (now %s)", site, o))
				}
			}
		}
	}
	rec.OK = true
	rec.Seconds = time.Since(start).Seconds()
	var b strings.Builder
	fmt.Fprintf(&b, "done: macro %s (version %d) ran %d steps in %.0f s", m.Name, m.Version, len(m.Steps), rec.Seconds)
	if rec.Skipped > 0 {
		fmt.Fprintf(&b, "; %d optional step(s) skipped", rec.Skipped)
	}
	b.WriteString(".\n")
	if age := time.Since(m.LastWorked); m.LastWorked.IsZero() || age > 24*time.Hour {
		rec.OldNote = true
		fmt.Fprintf(&b, "It last worked %s: check the result in the window before you trust it.\n", ago(age))
	}
	b.WriteString("log:\n" + strings.Join(rec.Log, "\n") + "\n")
	m.Runs++
	m.FailStreak = 0
	m.LastWorked = time.Now()
	if m.Site == "" {
		m.Site = site
	}
	return b.String(), rec
}
