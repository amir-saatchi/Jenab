package desktop

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Macros and recipes (8.11, SPIKE-040). A macro is steps Jenab replays in
// the picked window without the model. Jenab records them from what
// worked in the session, by role and exact name; the model only names
// the macro, its params and its log. A step finds its element through
// agent-desktop's find, never by a snapshot id, and every 8.10 guard runs
// as for the model's own action.

// Step is one step of a macro.
type Step struct {
	// Do is click, set_value, toggle, expand, collapse, select (an element
	// by role and name), press (a key of 8.10's list), click_each (one
	// click per character of Text), wait or wait_gone (a text, in Name),
	// or log (the elements of Role whose name holds Name).
	Do   string `json:"do"`
	Role string `json:"role,omitempty"`
	// Name is the element's exact name, or for wait the text; alternatives
	// are tried in order ("Search", "Suche"). Empty finds by role alone.
	Name Names  `json:"name,omitempty"`
	Text string `json:"text,omitempty"` // set_value's text, select's option, click_each's characters
	Key  string `json:"key,omitempty"`  // press
	// Map gives click_each a button's name for each character ("4=Four");
	// without it, each character is the button's name.
	Map      []string `json:"map,omitempty"`
	Optional bool     `json:"optional,omitempty"` // skipped when its element or text isn't there
	Limit    int      `json:"limit,omitempty"`    // log: elements to record, 10 by default, at most 50
}

// Names is a name with its alternatives; one name is written as a string.
type Names []string

func (n Names) MarshalJSON() ([]byte, error) {
	if len(n) == 1 {
		return json.Marshal(n[0])
	}
	return json.Marshal([]string(n))
}

func (n *Names) UnmarshalJSON(b []byte) error {
	var one string
	if err := json.Unmarshal(b, &one); err == nil {
		*n = Names{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return fmt.Errorf("a name is a string or a list of strings")
	}
	*n = many
	return nil
}

func (n Names) first() string {
	if len(n) == 0 {
		return ""
	}
	return n[0]
}

func (n Names) String() string {
	q := make([]string, len(n))
	for i, s := range n {
		q[i] = fmt.Sprintf("%q", clip(s, 60))
	}
	return strings.Join(q, " or ")
}

// overlaps reports whether two names share one, without case.
func (n Names) overlaps(o Names) bool {
	return slices.ContainsFunc(n, func(a string) bool {
		return slices.ContainsFunc(o, func(b string) bool { return strings.EqualFold(a, b) })
	})
}

// maxAlternatives bounds a step's names.
const maxAlternatives = 4

const (
	maxSteps       = 40
	maxRecipe      = 1200
	defaultLog     = 10
	maxLog         = 50
	findWait       = 10 * time.Second
	optionalWait   = 5 * time.Second
	defaultWaitFor = 10 * time.Second
)

var (
	reParam     = regexp.MustCompile(`\{([a-z][a-z0-9_]*)\}`)
	reMacroName = regexp.MustCompile(`^[a-z][a-z0-9_]{1,40}$`)
	reParamName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,40}$`)
	reRefLike   = regexp.MustCompile(`^@?\w*:?e\d+$|^\d+:e\d+$`)
	keys        = []string{"return", "escape", "tab", "backspace"}
	elementDo   = map[string]bool{"click": true, "set_value": true, "toggle": true, "expand": true, "collapse": true, "select": true}
)

// String is a step as the model and Settings read it.
func (st Step) String() string {
	var b strings.Builder
	switch st.Do {
	case "press":
		b.WriteString("press " + st.Key)
	case "wait", "wait_gone":
		b.WriteString(map[string]string{"wait": "wait for ", "wait_gone": "wait until gone: "}[st.Do] + st.Name.String())
	default:
		b.WriteString(st.Do)
		if st.Role != "" {
			b.WriteString(" " + st.Role)
		}
		if len(st.Name) > 0 && st.Name.first() != "" {
			b.WriteString(" " + st.Name.String())
		}
		if st.Text != "" {
			fmt.Fprintf(&b, " text %q", clip(st.Text, 80))
		}
	}
	if st.Optional {
		b.WriteString(" (optional)")
	}
	return b.String()
}

// Listing is a macro's stored steps as Settings shows them, numbered.
func Listing(raw []byte) string {
	var steps []Step
	if err := json.Unmarshal(raw, &steps); err != nil {
		return string(raw)
	}
	return listing(steps)
}

// listing numbers steps from 1, one per line.
func listing(steps []Step) string {
	var b strings.Builder
	for i, st := range steps {
		fmt.Fprintf(&b, "%d %s\n", i+1, st)
	}
	return b.String()
}

// ParseSteps reads a macro's steps and checks them against its params.
func ParseSteps(raw []byte, params []string) ([]Step, error) {
	var steps []Step
	if err := json.Unmarshal(raw, &steps); err != nil {
		return nil, fmt.Errorf("the steps are not a list of steps: %w", err)
	}
	return steps, Validate(steps, params)
}

// Validate checks a macro's steps and params: the step format of 8.11,
// 1 to 40 steps, and each param used in a step and each {param} declared.
func Validate(steps []Step, params []string) error {
	if len(steps) == 0 || len(steps) > maxSteps {
		return fmt.Errorf("a macro has 1 to %d steps", maxSteps)
	}
	declared := map[string]bool{}
	for _, p := range params {
		if !reParamName.MatchString(p) {
			return fmt.Errorf("param %q: a param's name is lower_snake_case, like query", p)
		}
		declared[p] = true
	}
	used := map[string]bool{}
	for i, st := range steps {
		n := i + 1
		fields := append([]string{st.Text}, st.Name...)
		for _, f := range fields {
			for _, m := range reParam.FindAllStringSubmatch(f, -1) {
				if !declared[m[1]] {
					return fmt.Errorf("step %d uses {%s}, which is not a param", n, m[1])
				}
				used[m[1]] = true
			}
		}
		if len(st.Name) > maxAlternatives {
			return fmt.Errorf("step %d has more than %d names", n, maxAlternatives)
		}
		for _, nm := range st.Name {
			if reRefLike.MatchString(nm) {
				return fmt.Errorf("step %d: %q looks like a snapshot id, not a name", n, nm)
			}
		}
		switch {
		case elementDo[st.Do]:
			if st.Role == "" {
				return fmt.Errorf("step %d (%s) needs the element's role", n, st.Do)
			}
			if st.Do == "select" && st.Text == "" {
				return fmt.Errorf("step %d (select) needs the option in text", n)
			}
		case st.Do == "press":
			if !slices.Contains(keys, st.Key) {
				return fmt.Errorf("step %d: press takes key return, escape, tab or backspace", n)
			}
		case st.Do == "click_each":
			if st.Role == "" || st.Text == "" {
				return fmt.Errorf("step %d (click_each) needs a role and text", n)
			}
			for _, kv := range st.Map {
				if k, v, ok := strings.Cut(kv, "="); !ok || utf8.RuneCountInString(k) != 1 || v == "" {
					return fmt.Errorf("step %d: a map entry is one character, =, a name: %q", n, kv)
				}
			}
		case st.Do == "wait" || st.Do == "wait_gone":
			if st.Name.first() == "" {
				return fmt.Errorf("step %d (%s) needs the text in name", n, st.Do)
			}
		case st.Do == "log":
			if st.Role == "" {
				return fmt.Errorf("step %d (log) needs the role of the elements to record", n)
			}
			if st.Limit < 0 || st.Limit > maxLog {
				return fmt.Errorf("step %d (log): limit is 1 to %d", n, maxLog)
			}
		default:
			return fmt.Errorf("step %d: unknown do %q", n, st.Do)
		}
	}
	for p := range declared {
		if !used[p] {
			return fmt.Errorf("param %q is in no step", p)
		}
	}
	return nil
}

// fill puts the params' values in a step's names and text.
func fill(st Step, params map[string]string) Step {
	f := func(s string) string {
		return reParam.ReplaceAllStringFunc(s, func(m string) string {
			if v, ok := params[m[1:len(m)-1]]; ok {
				return v
			}
			return m
		})
	}
	out := st
	out.Text = f(st.Text)
	out.Name = make(Names, len(st.Name))
	for i, n := range st.Name {
		out.Name[i] = f(n)
	}
	return out
}

// ---- the recorder ----

// did keeps one step of the session that worked: an action the guards let
// through and the app got, a wait that found its text, a step of a replay.
// A set_value right after one on the same element replaces it.
func (s *Session) did(st Step) {
	if n := len(s.done); n > 0 && st.Do == "set_value" {
		last := s.done[n-1]
		if last.Do == "set_value" && last.Role == st.Role && slices.Equal(last.Name, st.Name) {
			s.done[n-1] = st
			return
		}
	}
	s.done = append(s.done, st)
}

// keypad are the names of digit buttons that a run of clicks may spell a
// value with (Calculator's). A button named by one character spells that
// character.
var keypad = map[string]rune{"Zero": '0', "One": '1', "Two": '2', "Three": '3', "Four": '4', "Five": '5', "Six": '6', "Seven": '7', "Eight": '8', "Nine": '9'}

// spells is the character a button's name stands for.
func spells(name string) (rune, bool) {
	if r, ok := keypad[name]; ok {
		return r, true
	}
	if utf8.RuneCountInString(name) == 1 {
		r, _ := utf8.DecodeRuneInString(name)
		return r, true
	}
	return 0, false
}

type param struct{ name, value string }

// parseParams reads name=value pairs. A value is the one used this time.
func parseParams(ps []string) ([]param, error) {
	var out []param
	seen := map[string]bool{}
	for _, p := range ps {
		k, v, ok := strings.Cut(p, "=")
		k = strings.Trim(strings.TrimSpace(k), "{}")
		if !ok || !reParamName.MatchString(k) || v == "" {
			return nil, fmt.Errorf("params are name=value, with a lower_snake_case name and the value you used this time, like %q", "query=blue folders")
		}
		if seen[k] {
			return nil, fmt.Errorf("param %q is given twice", k)
		}
		seen[k] = true
		out = append(out, param{k, v})
	}
	return out, nil
}

// replaceValue puts {name} where value is in s. A value of one or two
// characters only stands for a whole text, so "a" doesn't take the a out
// of every word.
func replaceValue(s string, p param) (string, bool) {
	switch {
	case s == "" || !strings.Contains(s, p.value):
		return s, false
	case utf8.RuneCountInString(p.value) < 3 && s != p.value:
		return s, false
	}
	return strings.ReplaceAll(s, p.value, "{"+p.name+"}"), true
}

// generalize turns the recorded steps into a macro's: {param} where each
// value was, a run of keypad clicks that spells a value folded into one
// click_each, and a click that declines made optional. It returns the
// params that are in no step.
func generalize(steps []Step, params []param) ([]Step, []param) {
	// Longer values first, so a value that holds another one is replaced whole.
	ps := slices.Clone(params)
	sort.SliceStable(ps, func(i, j int) bool { return len(ps[i].value) > len(ps[j].value) })
	used := map[string]bool{}
	var out []Step
	for i := 0; i < len(steps); i++ {
		st := steps[i]
		if st.Do == "click" && len(st.Name) == 1 {
			if folded, n, p := fold(steps[i:], ps); n > 0 {
				out = append(out, folded)
				used[p] = true
				i += n - 1
				continue
			}
		}
		st.Name = slices.Clone(st.Name)
		for _, p := range ps {
			var ok bool
			if st.Text, ok = replaceValue(st.Text, p); ok {
				used[p.name] = true
			}
			for k := range st.Name {
				if st.Name[k], ok = replaceValue(st.Name[k], p); ok {
					used[p.name] = true
				}
			}
		}
		if (st.Do == "click" || st.Do == "select") && isDecline(st) {
			st.Optional = true
		}
		out = append(out, st)
	}
	var unused []param
	for _, p := range params {
		if !used[p.name] {
			unused = append(unused, p)
		}
	}
	return out, unused
}

// fold reads a run of clicks on buttons of one role that spell a param's
// value: the click_each step, the clicks it stands for and the param.
func fold(steps []Step, ps []param) (Step, int, string) {
	role := steps[0].Role
	var word []rune
	words := false // the buttons are named by words, as Calculator's
	n := 0
	for n < len(steps) && steps[n].Do == "click" && steps[n].Role == role && len(steps[n].Name) == 1 {
		r, ok := spells(steps[n].Name[0])
		if !ok {
			break
		}
		if _, w := keypad[steps[n].Name[0]]; w {
			words = true
		}
		word = append(word, r)
		n++
	}
	for _, p := range ps {
		for k := n; k > 0; k-- { // the longest run from here that spells a value
			if string(word[:k]) == p.value {
				st := Step{Do: "click_each", Role: role, Text: "{" + p.name + "}"}
				if words {
					for name, r := range keypad {
						st.Map = append(st.Map, string(r)+"="+name)
					}
					sort.Strings(st.Map)
				}
				return st, k, p.name
			}
		}
	}
	return Step{}, 0, ""
}

func isDecline(st Step) bool {
	return slices.ContainsFunc(st.Name, func(n string) bool { return len([]rune(n)) < 60 && reDecline.MatchString(n) })
}

// sameKind: two steps that do the same to the same kind of element.
func sameKind(a, b Step) bool {
	return a.Do == b.Do && strings.EqualFold(a.Role, b.Role) && a.Key == b.Key && (a.Do != "click_each" || a.Text == b.Text)
}

// align keeps the old version's names as alternatives (8.11): the new
// steps are lined up with the old ones by what they do and to what kind
// of element, in order, preferring pairs with a name in common, and each
// pair keeps both names, the new first. Log steps are the new version's.
func align(old, cur []Step) []Step {
	o := slices.DeleteFunc(slices.Clone(old), func(s Step) bool { return s.Do == "log" })
	n, m := len(o), len(cur)
	// best[i][j]: the best score lining up o[i:] with cur[j:].
	best := make([][]int, n+1)
	for i := range best {
		best[i] = make([]int, m+1)
	}
	score := func(a, b Step) int {
		switch {
		case b.Do == "log" || !sameKind(a, b):
			return 0
		case a.Name.overlaps(b.Name) || len(a.Name) == 0 && len(b.Name) == 0:
			return 3
		}
		return 2
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			best[i][j] = max(best[i+1][j], best[i][j+1])
			if s := score(o[i], cur[j]); s > 0 {
				best[i][j] = max(best[i][j], best[i+1][j+1]+s)
			}
		}
	}
	out := slices.Clone(cur)
	for i, j := 0, 0; i < n && j < m; {
		s := score(o[i], cur[j])
		switch {
		case s > 0 && best[i][j] == best[i+1][j+1]+s:
			st := out[j]
			st.Name = slices.Clone(st.Name)
			for _, nm := range o[i].Name {
				if len(st.Name) < maxAlternatives && !slices.ContainsFunc(st.Name, func(x string) bool { return strings.EqualFold(x, nm) }) {
					st.Name = append(st.Name, nm)
				}
			}
			st.Optional = st.Optional || o[i].Optional
			out[j] = st
			i, j = i+1, j+1
		case best[i][j] == best[i+1][j]:
			i++
		default:
			j++
		}
	}
	return out
}

// ---- the automatic log ----

// logOf is the log Jenab picks when the model gave none that matches: the
// elements the last recorded action changed, by role and the part of the
// name they kept ("Display is" of "Display is 4,827"). The window itself
// is left out: its title is no result; so is an element whose text stayed
// (only its focus or another state changed).
func logOf(changed, before []Node) []Step {
	var out []Step
	for _, c := range changed {
		if c.Role == "window" || !newText(c, before) {
			continue
		}
		st := Step{Do: "log", Role: c.Role, Limit: defaultLog}
		name := ""
		switch {
		case c.Name == "":
		case slices.ContainsFunc(before, func(b Node) bool { return b.Role == c.Role && b.Name == c.Name }):
			name = c.Name // its value changed
		default:
			for _, b := range before {
				if b.Role != c.Role {
					continue
				}
				if p := commonPrefix(b.Name, c.Name); utf8.RuneCountInString(p) > utf8.RuneCountInString(name) {
					name = p
				}
			}
			if utf8.RuneCountInString(name) < 4 {
				name = ""
			}
		}
		if name != "" {
			st.Name = Names{name}
		}
		if !slices.ContainsFunc(out, func(o Step) bool { return o.Role == st.Role && slices.Equal(o.Name, st.Name) }) {
			out = append(out, st)
		}
		if len(out) == 3 {
			break
		}
	}
	return out
}

// newText reports whether n's name or value is new: no element of its role
// had both before.
func newText(n Node, before []Node) bool {
	return !slices.ContainsFunc(before, func(b Node) bool { return b.Role == n.Role && b.Name == n.Name && b.Value == n.Value })
}

// commonPrefix is the start two names share, in whole words: "Display
// is" of "Display is 72" and "Display is 78", not "Display is 7".
func commonPrefix(a, b string) string {
	ra, rb := []rune(a), []rune(b)
	n := 0
	for n < len(ra) && n < len(rb) && ra[n] == rb[n] {
		n++
	}
	if n < len(ra) || n < len(rb) {
		for n > 0 && (unicode.IsLetter(ra[n-1]) || unicode.IsDigit(ra[n-1])) {
			n--
		}
	}
	return strings.TrimSpace(string(ra[:n]))
}

// matches reports whether a log step finds n: its role, and a part of
// its name without case.
func (st Step) matches(n Node) bool {
	if !strings.EqualFold(n.Role, st.Role) {
		return false
	}
	if len(st.Name) == 0 {
		return true
	}
	return slices.ContainsFunc(st.Name, func(part string) bool {
		return strings.Contains(strings.ToLower(n.Name), strings.ToLower(part))
	})
}

// ago says how long ago a macro last worked, in days.
func ago(d time.Duration) string {
	switch days := int(d.Hours() / 24); {
	case days < 1:
		return "today"
	case days == 1:
		return "1 day ago"
	default:
		return fmt.Sprintf("%d days ago", days)
	}
}
