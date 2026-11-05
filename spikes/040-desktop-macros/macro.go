package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// A macro is a list of steps Jenab replays without the model. Each step finds its element by
// role and name (never by a snapshot id), through agent-desktop's find, and every 8.10 guard
// runs as it would for the model's own action. A recipe is a short text the model reads.

type Step struct {
	Do       string   `json:"do"`                 // click set_value toggle expand collapse select press click_each wait wait_gone log
	Role     string   `json:"role,omitempty"`     // as in the snapshot: button, textfield, link, statictext ...
	Name     string   `json:"name,omitempty"`     // exact name for actions; part of the name for log
	Text     string   `json:"text,omitempty"`     // set_value/select value, wait text, click_each characters
	Key      string   `json:"key,omitempty"`      // press: return, escape, tab, backspace
	Map      []string `json:"map,omitempty"`      // click_each: "4=Four", one per character
	Optional bool     `json:"optional,omitempty"` // skipped when its element isn't there
	Seconds  float64  `json:"seconds,omitempty"`  // wait time (find or wait)
	Limit    int      `json:"limit,omitempty"`    // log: elements to record
}

type Macro struct {
	Name        string    `json:"name"`
	App         string    `json:"app"`
	Site        string    `json:"site,omitempty"` // browser: the origin the macro works on
	Description string    `json:"description"`
	Params      []string  `json:"params"`
	Steps       []Step    `json:"steps"`
	Made        time.Time `json:"made"`
	LastWorked  time.Time `json:"last_worked,omitzero"`
	Runs        int       `json:"runs"`
	Failures    int       `json:"failures"`
	FailStreak  int       `json:"fail_streak"`
	Version     int       `json:"version"`
	Untrusted   bool      `json:"untrusted"` // made in a turn that read screen text (every desktop turn)
	MadeBy      string    `json:"made_by"`
}

type Recipe struct {
	Name      string    `json:"name"`
	App       string    `json:"app"`
	Site      string    `json:"site,omitempty"`
	Text      string    `json:"text"`
	Source    string    `json:"source"` // "app" (ships with Jenab) or "agent"
	Made      time.Time `json:"made"`
	LastUsed  time.Time `json:"last_used,omitzero"`
	Uses      int       `json:"uses"`
	Untrusted bool      `json:"untrusted"`
	MadeBy    string    `json:"made_by"`
}

// Change is one save, kept so the notice's Undo can put the old version back.
type Change struct {
	At     time.Time       `json:"at"`
	Kind   string          `json:"kind"` // macro or recipe
	App    string          `json:"app"`
	Name   string          `json:"name"`
	Before json.RawMessage `json:"before,omitempty"` // null: it was new
}

// Store stands in for the registry.db tables (see results.md for the schema).
type Store struct {
	path    string
	mu      sync.Mutex
	Macros  []*Macro  `json:"macros"`
	Recipes []*Recipe `json:"recipes"`
	Changes []Change  `json:"changes"`
}

func openStore(path string) (*Store, error) {
	s := &Store{path: path}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	return s, json.Unmarshal(b, s)
}

func (s *Store) save() error { return writeJSON(s.path, s) }

func (s *Store) macro(app, name string) *Macro {
	for _, m := range s.Macros {
		if m.App == app && m.Name == name {
			return m
		}
	}
	return nil
}

func (s *Store) putMacro(m *Macro) {
	before, _ := json.Marshal(s.macro(m.App, m.Name))
	s.Changes = append(s.Changes, Change{At: time.Now(), Kind: "macro", App: m.App, Name: m.Name, Before: before})
	for i, old := range s.Macros {
		if old.App == m.App && old.Name == m.Name {
			m.Version = old.Version + 1
			m.Made = old.Made
			m.Runs, m.Failures = old.Runs, old.Failures
			s.Macros[i] = m
			return
		}
	}
	m.Version = 1
	s.Macros = append(s.Macros, m)
}

func (s *Store) putRecipe(r *Recipe) {
	var old *Recipe
	for _, o := range s.Recipes {
		if o.App == r.App && o.Name == r.Name {
			old = o
		}
	}
	before, _ := json.Marshal(old)
	s.Changes = append(s.Changes, Change{At: time.Now(), Kind: "recipe", App: r.App, Name: r.Name, Before: before})
	for i, o := range s.Recipes {
		if o == old {
			s.Recipes[i] = r
			return
		}
	}
	s.Recipes = append(s.Recipes, r)
}

// undo puts back what the last save of that macro or recipe replaced (or removes a new one).
func (s *Store) undo(kind, app, name string) error {
	for i := len(s.Changes) - 1; i >= 0; i-- {
		c := s.Changes[i]
		if c.Kind != kind || c.App != app || c.Name != name {
			continue
		}
		s.Changes = append(s.Changes[:i], s.Changes[i+1:]...)
		if kind == "macro" {
			var before *Macro
			json.Unmarshal(c.Before, &before)
			for j, m := range s.Macros {
				if m.App == app && m.Name == name {
					if before == nil {
						s.Macros = append(s.Macros[:j], s.Macros[j+1:]...)
					} else {
						s.Macros[j] = before
					}
					return nil
				}
			}
		} else {
			var before *Recipe
			json.Unmarshal(c.Before, &before)
			for j, r := range s.Recipes {
				if r.App == app && r.Name == name {
					if before == nil {
						s.Recipes = append(s.Recipes[:j], s.Recipes[j+1:]...)
					} else {
						s.Recipes[j] = before
					}
					return nil
				}
			}
		}
	}
	return fmt.Errorf("nothing to undo for %s %s/%s", kind, app, name)
}

var (
	reParam   = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	reRefLike = regexp.MustCompile(`^@?s?\w*:?e\d+$|^\d+:e\d+$`)
	reName    = regexp.MustCompile(`^[a-z][a-z0-9_]{1,40}$`)
)

var actions = map[string]bool{"click": true, "set_value": true, "toggle": true, "expand": true, "collapse": true, "select": true}

// validate checks a macro the model wants to save; the error goes back to the model.
func (m *Macro) validate() error {
	if !reName.MatchString(m.Name) {
		return fmt.Errorf("name must be lower_snake_case, like google_search")
	}
	if len(m.Steps) == 0 || len(m.Steps) > 40 {
		return fmt.Errorf("a macro has 1 to 40 steps")
	}
	declared := map[string]bool{}
	for _, p := range m.Params {
		declared[strings.Trim(p, "{}")] = true
	}
	used := map[string]bool{}
	hasLog := false
	for i, st := range m.Steps {
		n := i + 1
		for _, f := range []string{st.Name, st.Text} {
			for _, mm := range reParam.FindAllStringSubmatch(f, -1) {
				if !declared[mm[1]] {
					return fmt.Errorf("step %d uses {%s}, which is not in params", n, mm[1])
				}
				used[mm[1]] = true
			}
		}
		if reRefLike.MatchString(st.Name) {
			return fmt.Errorf("step %d: name %q looks like a snapshot id; give the element's role and its name as the snapshot shows it", n, st.Name)
		}
		switch {
		case actions[st.Do]:
			if st.Role == "" {
				return fmt.Errorf("step %d (%s) needs the element's role", n, st.Do)
			}
			if (st.Do == "set_value" || st.Do == "select") && st.Text == "" {
				return fmt.Errorf("step %d (%s) needs text", n, st.Do)
			}
		case st.Do == "press":
			switch st.Key {
			case "return", "escape", "tab", "backspace":
			default:
				return fmt.Errorf("step %d: press takes key return, escape, tab or backspace", n)
			}
		case st.Do == "click_each":
			if st.Text == "" || len(st.Map) == 0 || st.Role == "" {
				return fmt.Errorf("step %d (click_each) needs role, text (like \"{a}\") and map (like [\"1=One\"])", n)
			}
			for _, kv := range st.Map {
				if k, _, ok := strings.Cut(kv, "="); !ok || len([]rune(k)) != 1 {
					return fmt.Errorf("step %d: map entries are one character, =, a name: %q", n, kv)
				}
			}
		case st.Do == "wait" || st.Do == "wait_gone":
			if st.Text == "" {
				return fmt.Errorf("step %d (%s) needs text", n, st.Do)
			}
		case st.Do == "log":
			if st.Role == "" {
				return fmt.Errorf("step %d (log) needs the role of the elements to record", n)
			}
			hasLog = true
		default:
			return fmt.Errorf("step %d: unknown do %q", n, st.Do)
		}
	}
	for p := range declared {
		if !used[p] {
			return fmt.Errorf("param %q is not used in any step", p)
		}
	}
	if !hasLog {
		return fmt.Errorf("add at least one log step, so a replay shows what was on screen at the end")
	}
	return nil
}

func fill(s string, params map[string]string) string {
	return reParam.ReplaceAllStringFunc(s, func(m string) string {
		if v, ok := params[m[1:len(m)-1]]; ok {
			return v
		}
		return m
	})
}

func (st Step) String() string {
	var b strings.Builder
	b.WriteString(st.Do)
	if st.Do == "press" {
		b.WriteString(" " + st.Key)
	}
	if st.Role != "" {
		b.WriteString(" " + st.Role)
	}
	if st.Name != "" {
		fmt.Fprintf(&b, " %q", clip(st.Name, 60))
	}
	if st.Text != "" {
		fmt.Fprintf(&b, " text %q", clip(st.Text, 60))
	}
	if st.Optional {
		b.WriteString(" (optional)")
	}
	return b.String()
}

func describeMacro(m *Macro, now time.Time) string {
	ps := make([]string, len(m.Params))
	for i, p := range m.Params {
		ps[i] = strings.Trim(p, "{}")
	}
	when := "never worked yet"
	if !m.LastWorked.IsZero() {
		when = "last worked " + ago(now.Sub(m.LastWorked))
	}
	return fmt.Sprintf("macro %s(%s): %s [%d steps, %s, %d runs, %d failed]", m.Name, strings.Join(ps, ", "),
		clip(m.Description, 200), len(m.Steps), when, m.Runs, m.Failures)
}

func ago(d time.Duration) string {
	switch {
	case d < 24*time.Hour:
		return "today"
	case d < 48*time.Hour:
		return "1 day ago"
	default:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	}
}
