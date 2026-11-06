package desktop

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// Store keeps macros and recipes (8.11): registry.db in the app.
type Store interface {
	DesktopMacros(ctx context.Context, app string) ([]store.DesktopMacro, error)
	SaveDesktopMacro(ctx context.Context, m store.DesktopMacro, now time.Time) (store.DesktopSave, error)
	RecordDesktopRun(ctx context.Context, run store.DesktopRun) (bool, error)
	DesktopRecipes(ctx context.Context, apps ...string) ([]store.DesktopRecipe, error)
	SaveDesktopRecipe(ctx context.Context, d store.DesktopRecipe, now time.Time) (store.DesktopSave, error)
	UseDesktopRecipes(ctx context.Context, ids []int64, now time.Time) error
	ShipDesktopRecipes(ctx context.Context, ds []store.DesktopRecipe, version string, now time.Time) error
	PruneDesktop(ctx context.Context, now time.Time) error
}

// KindMacro is a replay's card: at Strict only, once per replay (8.11).
const KindMacro = "desktop_macro"

// BrowserApp is the app of recipes for any browser.
const BrowserApp = "browser"

// Shipped are the recipes that come with Jenab (SPIKE-040): only where
// models failed without one. ShippedVersion changes with their texts.
var Shipped = []store.DesktopRecipe{
	{App: "calculatorapp", Name: "calculator", Text: "The digits are separate buttons named Zero to Nine, with Plus, Minus and Equals. " +
		"The result is the statictext 'Display is …'. For another mode, click Open Navigation and select the mode's option (a click doesn't switch it). " +
		"If there is no button named One, the window is in another mode: switch it to Standard that way first. " +
		"Don't use Keep on top: the window drops out of the list."},
	{App: BrowserApp, Name: "web_search", Text: "Set the address bar to the site and press Return. If a cookie dialog appears, decline it. " +
		"Type the words into the page's search box and press Return or its search button, wait for the results, and read titles and sites from the result links; " +
		"skip ads and don't open them."},
}

const ShippedVersion = "2"

// reminder is the one reminder per session (8.11), the same for every
// model.
const reminder = "Before you answer: you did a task in the window by hand. If it worked, save it now as the desktop skill says " +
	"(desktop_save_macro, then desktop_save_recipe); the user sees a notice and can undo it. Then answer. If it didn't work, answer without saving."

func (m *Manager) macroTools(spec func(tool.Spec) tool.Spec) []tool.Tool {
	return []tool.Tool{
		tool.Func(spec(tool.Spec{Name: "desktop_run_macro", Timeout: 5 * time.Minute,
			Description: "Replay a saved macro in the picked window, with the task's values as params. Jenab runs its steps without you and returns its log. If a step fails, the result has the window's snapshot, and you carry on by hand.",
			Schema: json.RawMessage(`{"type":"object","properties":{` +
				`"name":{"type":"string","maxLength":41,"description":"The macro's name, from desktop_start's list"},` +
				`"params":{"type":"array","maxItems":10,"items":{"type":"string","maxLength":2000},"description":"Each param as name=value with this task's value, like [\"query=blue folders\"]"}` +
				`},"required":["name"],"additionalProperties":false}`),
		}), m.runMacro),
		tool.Func(spec(tool.Spec{Name: "desktop_save_macro", AtLimit: true,
			Description: "Save the task you did in the picked window as a macro, once it worked, so it can be replayed without you. Jenab takes the steps from your actions that worked, in order; you name the macro, give the values that change as params and say which elements to log. The same name makes a new version.",
			Schema: json.RawMessage(`{"type":"object","properties":{` +
				`"name":{"type":"string","maxLength":41,"description":"lower_snake_case, like site_search"},` +
				`"description":{"type":"string","maxLength":300,"description":"What it does, in one sentence"},` +
				`"params":{"type":"array","maxItems":10,"items":{"type":"string","maxLength":2000},"description":"The values that change from task to task, as name=value with the value you used this time, like [\"query=blue folders\"]"},` +
				`"log":{"type":"array","maxItems":5,"items":{"type":"object","properties":{` +
				`"role":{"type":"string","maxLength":40,"description":"The role of the elements that hold the result, as in the snapshot: statictext, link, textfield …"},` +
				`"name":{"type":"string","maxLength":200,"description":"A part of their name; empty for all of that role"},` +
				`"limit":{"type":"integer","minimum":1,"maximum":50,"description":"How many to record, 10 by default"}` +
				`},"required":["role"],"additionalProperties":false},"description":"What a replay records and returns"},` +
				`"skip":{"type":"array","maxItems":40,"items":{"type":"integer","minimum":1},"description":"Numbers of recorded steps to leave out, from a save's result"}` +
				`},"required":["name","description"],"additionalProperties":false}`),
		}), m.saveMacro),
		tool.Func(spec(tool.Spec{Name: "desktop_save_recipe", AtLimit: true,
			Description: "Save a recipe for the picked app: two to five sentences on how to do a task in it, for when a macro breaks. The same name replaces your earlier one.",
			Schema: json.RawMessage(`{"type":"object","properties":{` +
				`"name":{"type":"string","maxLength":41,"description":"lower_snake_case, like site_search"},` +
				`"text":{"type":"string","maxLength":1200}` +
				`},"required":["name","text"],"additionalProperties":false}`),
		}), m.saveRecipe),
	}
}

// recorded is the chat's session of this turn for a save: the open one,
// or the one that ended in this turn.
func (m *Manager) recorded(env *tool.Env) (*Session, error) {
	k := key{env.Project.ID, env.Chat}
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.sessions[k]; s != nil && s.turn == env.Turn {
		return s, nil
	}
	if s := m.last[k]; s != nil && s.turn == env.Turn {
		return s, nil
	}
	return nil, tool.Errorf("no desktop session in this turn: a macro or recipe is saved from the session it was done in")
}

func (s *Session) alive() bool {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	return s.m.sessions[s.key] == s
}

// app is the session's app as macros are stored: the program's name.
func (s *Session) app() string { return program(s.win.App) }

// site is a browser session's locked site, "" for other apps.
func (s *Session) site() string {
	if s.browser && !blank(s.origin) {
		return s.origin
	}
	return ""
}

// prepare puts the shipped recipes in the store once, and applies the
// removal rules at each session's start.
func (m *Manager) prepare(ctx context.Context) {
	if m.d.Store == nil {
		return
	}
	m.shipped.Do(func() {
		if err := m.d.Store.ShipDesktopRecipes(ctx, Shipped, ShippedVersion, m.d.Now()); err != nil {
			m.d.Log.Warn("desktop: shipping recipes", "err", err)
		}
	})
	if err := m.d.Store.PruneDesktop(ctx, m.d.Now()); err != nil {
		m.d.Log.Warn("desktop: removing old macros", "err", err)
	}
}

// savedList is the end of desktop_start's result (8.11): the macros and
// recipes for the picked app, and for a browser the browser recipes,
// without their age (SPIKE-040: a model skipped old ones).
func (m *Manager) savedList(ctx context.Context, s *Session) string {
	if m.d.Store == nil {
		return ""
	}
	app := s.app()
	ms, err := m.d.Store.DesktopMacros(ctx, app)
	if err != nil {
		m.d.Log.Warn("desktop: listing macros", "err", err)
	}
	apps := []string{app}
	if s.browser {
		apps = append(apps, BrowserApp)
	}
	rs, err := m.d.Store.DesktopRecipes(ctx, apps...)
	if err != nil {
		m.d.Log.Warn("desktop: listing recipes", "err", err)
	}
	// One recipe per name: the user's, else the agent's, else Jenab's.
	rank := map[string]int{store.RecipeUser: 0, store.RecipeAgent: 1, store.RecipeApp: 2}
	slices.SortStableFunc(rs, func(a, b store.DesktopRecipe) int {
		return cmp.Or(cmp.Compare(a.Site+"\x00"+a.Name, b.Site+"\x00"+b.Name), cmp.Compare(rank[a.Source], rank[b.Source]))
	})
	rs = slices.CompactFunc(rs, func(a, b store.DesktopRecipe) bool { return a.Site == b.Site && a.Name == b.Name })
	if len(ms) == 0 && len(rs) == 0 {
		return "\nNo macros or recipes are saved for " + app + " yet."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\nSaved for %s (run a macro with desktop_run_macro; a recipe is a hint: follow it and check each step):\n", app)
	for _, mc := range ms[:min(len(ms), 20)] {
		fmt.Fprintf(&b, "- macro %s(%s)", mc.Name, strings.Join(mc.Params, ", "))
		if mc.Site != "" {
			b.WriteString(", on " + mc.Site)
		}
		b.WriteString(": " + clip(mc.Description, 200) + "\n")
	}
	var used []int64
	for _, r := range rs[:min(len(rs), 10)] {
		fmt.Fprintf(&b, "- recipe %s", r.Name)
		if r.Site != "" {
			b.WriteString(", on " + r.Site)
		}
		b.WriteString(": " + strings.ReplaceAll(r.Text, "\n", " ") + "\n")
		used = append(used, r.ID)
	}
	if err := m.d.Store.UseDesktopRecipes(ctx, used, m.d.Now()); err != nil {
		m.d.Log.Warn("desktop: marking recipes used", "err", err)
	}
	return strings.TrimRight(b.String(), "\n")
}

// remind asks for the one reminder per session (8.11): when the model
// acted by hand, saved nothing and is about to answer.
func (m *Manager) remind(env *tool.Env, s *Session) {
	if env.BeforeAnswer == nil || m.d.Store == nil {
		return
	}
	env.BeforeAnswer(func() string {
		if s.byHand == 0 || s.saved || s.reminded {
			return ""
		}
		s.reminded = true
		m.d.Log.Info("desktop: reminder to save", "app", s.win.App)
		return reminder
	})
}

type logArg struct {
	Role  string `json:"role"`
	Name  string `json:"name"`
	Limit int    `json:"limit"`
}

type saveMacroArgs struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Params      []string `json:"params"`
	Log         []logArg `json:"log"`
	Skip        []int    `json:"skip"`
}

func (m *Manager) saveMacro(ctx context.Context, env *tool.Env, a saveMacroArgs) (tool.Result, error) {
	if m.d.Store == nil {
		return tool.Result{}, tool.Errorf("macros can't be saved here")
	}
	s, err := m.recorded(env)
	if err != nil {
		return tool.Result{}, err
	}
	a.Name, a.Description = strings.TrimSpace(a.Name), strings.TrimSpace(a.Description)
	if !reMacroName.MatchString(a.Name) {
		return tool.Result{}, tool.Errorf("not saved: a macro's name is lower_snake_case, 2 to 41 characters, like site_search")
	}
	if a.Description == "" {
		return tool.Result{}, tool.Errorf("not saved: give a one-sentence description")
	}
	params, err := parseParams(a.Params)
	if err != nil {
		return tool.Result{}, tool.Errorf("not saved: %v", err)
	}
	skip := map[int]bool{}
	for _, k := range a.Skip {
		if k < 1 || k > len(s.done) {
			return tool.Result{}, tool.Errorf("not saved: skip %d is not a recorded step; there are %d:\n%s", k, len(s.done), listing(s.done))
		}
		skip[k] = true
	}
	var steps []Step
	for i, st := range s.done {
		if !skip[i+1] {
			steps = append(steps, st)
		}
	}
	if len(steps) == 0 {
		return tool.Result{}, tool.Errorf("not saved: no action of this session worked yet, so there is nothing to record")
	}
	gen, unused := generalize(steps, params)
	if len(unused) > 0 {
		p := unused[0]
		return tool.Result{}, tool.Errorf("not saved: the value of %s (%q) is in none of the steps that worked:\n%sGive the value as you typed or clicked it, or leave the param out.", p.name, p.value, listing(steps))
	}
	logs, notes := s.logSteps(ctx, a.Log, params)
	if len(logs) == 0 {
		if logs = logOf(s.changedBy, s.before); len(logs) > 0 {
			notes = append(notes, "Jenab logs what your last action changed.")
		} else {
			notes = append(notes, "Nothing to log: check the window with a snapshot after a replay.")
		}
	}
	app, site := s.app(), s.site()
	ms, err := m.d.Store.DesktopMacros(ctx, app)
	if err != nil {
		return tool.Result{}, err
	}
	var old *store.DesktopMacro
	for i := range ms {
		if ms[i].Site == site && ms[i].Name == a.Name {
			old = &ms[i]
		}
	}
	names := make([]string, len(params))
	for i, p := range params {
		names[i] = p.name
	}
	slices.Sort(names)
	if old != nil && old.Worked() {
		// A fix can't lose the params (SPIKE-040: a 38-step version without
		// params replaced a working one).
		for _, p := range old.Params {
			if !slices.Contains(names, p) {
				return tool.Result{}, tool.Errorf("not saved: macro %s has worked with the params %s; a new version must keep them. Give each with the value you used this time, or save under another name.",
					a.Name, strings.Join(old.Params, ", "))
			}
		}
	}
	if old != nil && s.failed[a.Name] {
		// A fix after a failed replay keeps the old names as alternatives.
		if prev, err := ParseSteps(old.Steps, old.Params); err == nil {
			gen = align(prev, gen)
		}
	}
	all := append(gen, logs...)
	if len(all) > maxSteps {
		return tool.Result{}, tool.Errorf("not saved: %d steps, more than %d. Leave out the ones you don't need with skip:\n%s", len(all), maxSteps, listing(steps))
	}
	if err := Validate(all, names); err != nil {
		return tool.Result{}, tool.Errorf("not saved: %v", err)
	}
	raw, err := json.Marshal(all)
	if err != nil {
		return tool.Result{}, err
	}
	if old != nil && string(old.Steps) == string(raw) && slices.Equal(old.Params, names) {
		// The task done by hand again: no new version and no notice.
		s.saved = true
		return tool.Result{Text: fmt.Sprintf("Not saved again: macro %s (version %d) already has these steps. Next time, run it with desktop_run_macro instead of doing the task by hand.",
			a.Name, old.Version)}, nil
	}
	saved, err := m.d.Store.SaveDesktopMacro(ctx, store.DesktopMacro{App: app, Site: site, Name: a.Name, Description: clip(a.Description, 300),
		Params: names, Steps: raw, MadeByModel: env.Model, MadeInChat: string(env.Chat), Untrusted: true}, m.d.Now())
	if err != nil {
		return tool.Result{}, err
	}
	s.saved = true
	mc := saved.Macro
	m.d.Log.Info("desktop: macro saved", "app", app, "steps", len(all), "params", len(names), "version", mc.Version)
	var b strings.Builder
	fmt.Fprintf(&b, "Saved macro %s, version %d, with these steps:\n%s", mc.Name, mc.Version, listing(all))
	for _, n := range notes {
		b.WriteString(n + "\n")
	}
	if len(a.Skip) > 0 || len(gen) != len(steps) {
		b.WriteString("Your actions as recorded (the numbers for skip):\n" + listing(s.done))
	}
	b.WriteString("The user sees a notice and can undo it.")
	return tool.Result{Text: b.String(), Notices: []chat.Notice{{Kind: chat.NoticeDesktopSaved,
		Text:    fmt.Sprintf("Macro saved: %s (%d steps)", mc.Name, len(all)),
		Target:  fmt.Sprintf("desktop:macro:%d", mc.ID),
		Change:  fmt.Sprint(saved.Change),
		Details: listing(all)}}}, nil
}

// logSteps checks the model's log (8.11): each must find an element in
// the session's last snapshot, or in the window now; duplicates go. notes
// say what was left out.
func (s *Session) logSteps(ctx context.Context, args []logArg, params []param) (logs []Step, notes []string) {
	for _, l := range args {
		role, part := strings.ToLower(strings.TrimSpace(l.Role)), strings.TrimSpace(l.Name)
		if role == "" {
			continue
		}
		st := Step{Do: "log", Role: role, Limit: min(max(l.Limit, 0), maxLog)}
		if st.Limit == 0 {
			st.Limit = defaultLog
		}
		if part != "" {
			st.Name = Names{part}
		}
		if part == "" {
			// A role alone logs every element of it, such as the window's
			// buttons (TASK-038's live runs): it must be one whose text
			// the last action changed, as the result's.
			if !slices.ContainsFunc(s.changedBy, func(n Node) bool { return n.Role == role && newText(n, s.before) }) {
				notes = append(notes, fmt.Sprintf("Left out the step %s: with no name, a log step must be an element whose text your last action changed. Give a part of the name of the element that holds the result.", st))
				continue
			}
		}
		found := slices.ContainsFunc(s.lastAll, st.matches)
		if !found && s.alive() {
			ms, err := s.findAll(ctx, role, part, false, 1)
			found = err == nil && len(ms) > 0
		}
		if !found {
			notes = append(notes, fmt.Sprintf("Left out the step %s: no such element in the window.", st))
			continue
		}
		for _, p := range params {
			if len(st.Name) > 0 {
				st.Name[0], _ = replaceValue(st.Name[0], p)
			}
		}
		if slices.ContainsFunc(logs, func(o Step) bool { return o.Role == st.Role && slices.Equal(o.Name, st.Name) }) {
			continue
		}
		logs = append(logs, st)
	}
	return logs, notes
}

type saveRecipeArgs struct {
	Name string `json:"name"`
	Text string `json:"text"`
}

func (m *Manager) saveRecipe(ctx context.Context, env *tool.Env, a saveRecipeArgs) (tool.Result, error) {
	if m.d.Store == nil {
		return tool.Result{}, tool.Errorf("recipes can't be saved here")
	}
	s, err := m.recorded(env)
	if err != nil {
		return tool.Result{}, err
	}
	a.Name, a.Text = strings.TrimSpace(a.Name), strings.TrimSpace(a.Text)
	switch {
	case !reMacroName.MatchString(a.Name):
		return tool.Result{}, tool.Errorf("not saved: a recipe's name is lower_snake_case, 2 to 41 characters, like site_search")
	case a.Text == "":
		return tool.Result{}, tool.Errorf("not saved: the text is empty")
	case len([]rune(a.Text)) > maxRecipe:
		return tool.Result{}, tool.Errorf("not saved: a recipe has at most %d characters; this one has %d", maxRecipe, len([]rune(a.Text)))
	}
	rs, err := m.d.Store.DesktopRecipes(ctx, s.app())
	if err != nil {
		return tool.Result{}, err
	}
	if slices.ContainsFunc(rs, func(r store.DesktopRecipe) bool {
		return r.Source == store.RecipeAgent && r.Site == s.site() && r.Name == a.Name && r.Text == a.Text
	}) {
		s.saved = true
		return tool.Result{Text: fmt.Sprintf("Not saved again: recipe %s already has this text.", a.Name)}, nil
	}
	saved, err := m.d.Store.SaveDesktopRecipe(ctx, store.DesktopRecipe{App: s.app(), Site: s.site(), Name: a.Name, Text: a.Text,
		MadeByModel: env.Model, MadeInChat: string(env.Chat), Untrusted: true}, m.d.Now())
	if err != nil {
		return tool.Result{}, err
	}
	s.saved = true
	m.d.Log.Info("desktop: recipe saved", "app", s.app(), "text_len", len([]rune(a.Text)))
	return tool.Result{Text: fmt.Sprintf("Saved recipe %s for %s. The user sees a notice and can undo it.", a.Name, s.app()),
		Notices: []chat.Notice{{Kind: chat.NoticeDesktopSaved, Text: "Recipe saved: " + a.Name,
			Target: fmt.Sprintf("desktop:recipe:%d", saved.Recipe.ID), Change: fmt.Sprint(saved.Change), Details: a.Text}}}, nil
}

type runMacroArgs struct {
	Name   string   `json:"name"`
	Params []string `json:"params"`
}

func (m *Manager) runMacro(ctx context.Context, env *tool.Env, a runMacroArgs) (tool.Result, error) {
	if m.d.Store == nil {
		return tool.Result{}, tool.Errorf("no macros are saved here")
	}
	s, err := m.session(env)
	if err != nil {
		return tool.Result{}, err
	}
	if err := s.check(ctx); err != nil {
		return tool.Result{}, err
	}
	ms, err := m.d.Store.DesktopMacros(ctx, s.app())
	if err != nil {
		return tool.Result{}, err
	}
	var mc *store.DesktopMacro
	var names []string
	for i := range ms {
		names = append(names, ms[i].Name)
		if ms[i].Name != strings.TrimSpace(a.Name) {
			continue
		}
		switch {
		case ms[i].Site == s.site():
			mc = &ms[i]
		case mc == nil && s.site() == "":
			mc = &ms[i] // a blank window takes the macro's site
		}
	}
	if mc == nil {
		if len(names) == 0 {
			return tool.Result{}, tool.Errorf("no macro is saved for %s", s.app())
		}
		return tool.Result{}, tool.Errorf("no macro %q for %s on this site; the saved ones are: %s", a.Name, s.app(), strings.Join(slices.Compact(names), ", "))
	}
	steps, err := ParseSteps(mc.Steps, mc.Params)
	if err != nil {
		return tool.Result{}, tool.Errorf("macro %s can't be read (%v): do the task by hand, then save it again", mc.Name, err)
	}
	given, err := parseParams(a.Params)
	if err != nil {
		return tool.Result{}, tool.Errorf("%v", err)
	}
	vals := map[string]string{}
	for _, p := range given {
		if !slices.Contains(mc.Params, p.name) {
			return tool.Result{}, tool.Errorf("macro %s has no param %q; its params are: %s", mc.Name, p.name, strings.Join(mc.Params, ", "))
		}
		vals[p.name] = p.value
	}
	for _, p := range mc.Params {
		if _, ok := vals[p]; !ok {
			return tool.Result{}, tool.Errorf("give macro %s every param, as name=value: %s (%q is missing)", mc.Name, strings.Join(mc.Params, ", "), p)
		}
	}
	filled := make([]Step, len(steps))
	for i, st := range steps {
		filled[i] = fill(st, vals)
	}
	if err := s.approveMacro(ctx, env, mc, filled); err != nil {
		return tool.Result{}, err
	}
	start := m.d.Now()
	o, err := s.replay(ctx, env, filled)
	run := store.DesktopRun{MacroID: mc.ID, Version: mc.Version, At: m.d.Now(), OK: err == nil && o.failedAt == 0,
		FailedAt: o.failedAt, Reason: o.reason, Skipped: o.skipped, Seconds: m.d.Now().Sub(start).Seconds(), LogChars: len([]rune(strings.Join(o.log, "\n")))}
	if err != nil && run.FailedAt == 0 {
		run.FailedAt, run.Reason = o.step+1, "the session ended"
	}
	if !run.OK {
		if s.failed == nil {
			s.failed = map[string]bool{}
		}
		s.failed[mc.Name] = true
	}
	removed, rerr := m.d.Store.RecordDesktopRun(context.WithoutCancel(ctx), run)
	if rerr != nil {
		m.d.Log.Warn("desktop: recording a replay", "err", rerr)
	}
	m.d.Log.Info("desktop: replay", "app", s.win.App, "steps", len(steps), "ok", run.OK, "failed_at", run.FailedAt, "skipped", o.skipped, "log_len", run.LogChars,
		"took", m.d.Now().Sub(start).Milliseconds())
	if err != nil {
		return tool.Result{}, err
	}
	var b strings.Builder
	if run.OK {
		fmt.Fprintf(&b, "Done: macro %s (version %d) ran %d steps in %.0f s", mc.Name, mc.Version, len(steps), run.Seconds)
		if o.skipped > 0 {
			fmt.Fprintf(&b, "; %d optional step(s) skipped", o.skipped)
		}
		b.WriteString(".\n")
		if age := run.At.Sub(mc.LastWorkedAt); age > 24*time.Hour {
			fmt.Fprintf(&b, "It last worked %s: check the result in the window with a snapshot before you trust it.\n", ago(age))
		}
		b.WriteString(logText(o.log))
		b.WriteString("Check the log against the task before you answer.")
		return tool.Result{Text: b.String()}, nil
	}
	fmt.Fprintf(&b, "Failed at step %d of %d (%s): %s\n", o.failedAt, len(steps), filled[o.failedAt-1], o.reason)
	if len(o.done) > 0 {
		b.WriteString("Steps before it:\n" + strings.Join(o.done, "\n") + "\n")
	}
	if len(o.log) > 0 {
		b.WriteString("Log so far:\n" + strings.Join(o.log, "\n") + "\n")
	}
	if removed {
		fmt.Fprintf(&b, "It failed %d replays in a row, so it was removed; save the task again once it works.\n", store.MacroFailLimit)
	}
	if !s.alive() {
		b.WriteString("The session ended, so the window can't be read now.")
		return tool.Result{Text: b.String()}, nil
	}
	view, err := s.look(ctx, false)
	if err != nil {
		return tool.Result{}, err
	}
	b.WriteString("The window now:\n" + view)
	b.WriteString("Carry on by hand from here: the ids above work with desktop_act. When the task worked, save the macro again under the same name; its old names stay as alternatives.")
	return tool.Result{Text: b.String(), Whole: true, Series: "desktop_snapshot",
		Stub: fmt.Sprintf("[replay of %s, failed at step %d, and its snapshot — replaced by a newer snapshot]", mc.Name, o.failedAt)}, nil
}

func logText(log []string) string {
	if len(log) == 0 {
		return "No log.\n"
	}
	return "Log:\n" + strings.Join(log, "\n") + "\n"
}

// approveMacro asks once for a replay at Strict (8.11), with the steps
// and the params filled in.
func (s *Session) approveMacro(ctx context.Context, env *tool.Env, mc *store.DesktopMacro, steps []Step) error {
	if env.Approve == nil {
		return tool.Errorf("desktop control works only in a chat turn the user started")
	}
	card := chat.Approval{ID: id.Approval(id.New()), Kind: KindMacro, Target: "desktop:" + s.win.ID,
		Ask:     fmt.Sprintf("Run the macro %s in %q?", mc.Name, title(s.win)),
		Risk:    "Jenab replays these steps in the window without asking for each. Every step keeps the rules of desktop control; the first key press still asks.",
		Details: fmt.Sprintf("Window: %s (%s)\n%s\nSteps:\n%s", title(s.win), strings.ToLower(s.win.App), clip(mc.Description, 300), listing(steps)),
		Options: []chat.ApprovalOption{{Label: "Once", Grant: chat.GrantOnce}, {Label: "Deny", Grant: chat.GrantDeny}}}
	got, err := env.Approve(ctx, card)
	if err != nil {
		return err
	}
	if got.Answer == chat.GrantDeny {
		msg := "not run: the user denied it"
		if got.Note != "" {
			msg += "; their note: " + got.Note
		}
		return tool.Errorf("%s", msg)
	}
	return nil
}

// outcome is how a replay went.
type outcome struct {
	step     int // the step it was at, from 0
	failedAt int // the step that failed, from 1; 0 when none did
	reason   string
	skipped  int
	log      []string
	done     []string // the steps before the failed one, with how each went
}

// replay runs a macro's steps, with the params filled in, in the locked
// window (8.11). A step that fails ends it with the outcome's failedAt;
// an error is the session's end or a stop.
func (s *Session) replay(ctx context.Context, env *tool.Env, steps []Step) (o outcome, err error) {
	if s.browser {
		// The site and CAPTCHA checks, before the first step.
		if _, _, err := s.snapshot(ctx, false, 0); err != nil {
			return o, err
		}
	}
	fail := func(i int, why string) (outcome, error) {
		o.failedAt, o.reason = i+1, why
		return o, nil
	}
	// stepErr is a step's error: the session's end stops the replay, a
	// refusal fails the step.
	stepErr := func(i int, err error) (outcome, error) {
		var te *tool.Error
		if !s.alive() || !errors.As(err, &te) {
			return o, err
		}
		return fail(i, te.Msg)
	}
	for i, st := range steps {
		o.step = i
		line := fmt.Sprintf("%d %s", i+1, st)
		if err := s.check(ctx); err != nil {
			return o, err
		}
		wait := findWait
		if st.Optional {
			wait = optionalWait
		}
		switch {
		case elementDo[st.Do]:
			el, why, err := s.findOne(ctx, st.Role, st.Name, wait)
			if err != nil {
				return stepErr(i, err)
			}
			if el == nil {
				if st.Optional {
					o.skipped++
					s.did(st)
					o.done = append(o.done, line+" — skipped, not there")
					continue
				}
				return fail(i, why)
			}
			if _, err := s.do(ctx, env, actArgs{Action: st.Do, Text: st.Text}, el.Ref, el.node(), false); err != nil {
				return stepErr(i, err)
			}
			s.keep(st, el.Name)
		case st.Do == "press":
			if _, err := s.do(ctx, env, actArgs{Action: "press", Key: st.Key}, "", Node{}, false); err != nil {
				return stepErr(i, err)
			}
		case st.Do == "click_each":
			for _, r := range st.Text {
				name := string(r)
				if len(st.Map) > 0 {
					name = ""
					for _, kv := range st.Map {
						if k, v, _ := strings.Cut(kv, "="); k == string(r) {
							name = v
						}
					}
					if name == "" {
						return fail(i, fmt.Sprintf("the macro has no button for %q", string(r)))
					}
				}
				el, why, err := s.findOne(ctx, st.Role, Names{name}, wait)
				if err != nil {
					return stepErr(i, err)
				}
				if el == nil {
					return fail(i, why)
				}
				if _, err := s.do(ctx, env, actArgs{Action: "click"}, el.Ref, el.node(), false); err != nil {
					return stepErr(i, err)
				}
			}
		case st.Do == "wait" || st.Do == "wait_gone":
			gone := st.Do == "wait_gone"
			limit := defaultWaitFor
			if st.Optional {
				limit = optionalWait
			}
			ok, err := s.waitFor(ctx, st.Name, gone, limit)
			if err != nil {
				return stepErr(i, err)
			}
			if !ok {
				if st.Optional {
					o.skipped++
					s.did(st)
					o.done = append(o.done, line+" — skipped, not there")
					continue
				}
				state := "didn't appear"
				if gone {
					state = "is still there"
				}
				return fail(i, fmt.Sprintf("the text %s after %.0f s", state, limit.Seconds()))
			}
			s.did(Step{Do: st.Do, Name: st.Name, Optional: st.Optional})
		case st.Do == "log":
			lines, err := s.logged(ctx, st)
			if err != nil {
				return stepErr(i, err)
			}
			o.log = append(o.log, lines...)
			o.done = append(o.done, fmt.Sprintf("%s — %d recorded", line, max(len(lines)-1, 0)))
			continue
		}
		o.done = append(o.done, line+" — ok")
	}
	return o, nil
}

// keep puts a replayed step's names on the step do recorded, the one that
// was there first, and its optional mark, so a save after it keeps them.
func (s *Session) keep(st Step, found string) {
	if len(s.done) == 0 {
		return
	}
	last := &s.done[len(s.done)-1]
	names := Names{found}
	for _, n := range st.Name {
		if !strings.EqualFold(n, found) {
			names = append(names, n)
		}
	}
	if found == "" {
		names = nil
	}
	last.Name, last.Optional = names, st.Optional
}

// found is an element agent-desktop's find returned.
type found struct {
	Ref    string   `json:"ref_id"`
	Role   string   `json:"role"`
	Name   string   `json:"name"`
	Value  *string  `json:"value"`
	States []string `json:"states"`
	Path   []string `json:"path"`
}

func (f found) node() Node {
	n := Node{Role: f.Role, Name: f.Name, States: f.States, Ref: f.Ref}
	if f.Value != nil {
		n.Value = *f.Value
	}
	return n
}

// findAll asks agent-desktop for elements of the locked window by role
// and name: exact is the whole name without case, else a part of it.
func (s *Session) findAll(ctx context.Context, role, name string, exact bool, limit int) ([]found, error) {
	args := []string{"find", "--window-id", s.win.ID, "--timeout-ms", "10000"}
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
	data, err := s.m.d.Runner.Run(ctx, callTimeout, args...)
	if err != nil {
		var de *Error
		if errors.As(err, &de) && de.Code == CodeElementNotFound {
			return nil, nil
		}
		return nil, s.fail(err)
	}
	var d struct {
		Matches []found `json:"matches"`
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("desktop: reading find: %w", err)
	}
	if s.browser {
		// Only the page and the address bar, as for the model (8.10).
		d.Matches = slices.DeleteFunc(d.Matches, func(f found) bool { return !inPage(f) && !reAddress.MatchString(f.Name) })
	}
	return d.Matches, nil
}

// inPage reports whether a browser's element is in the page.
func inPage(f found) bool {
	if f.Role == "document" {
		return true
	}
	return slices.ContainsFunc(f.Path, func(p string) bool { return strings.HasPrefix(p, "document") })
}

// findOne waits up to wait for an element with a ref, by role and any of
// its names in order. Without one it says why.
func (s *Session) findOne(ctx context.Context, role string, names Names, wait time.Duration) (*found, string, error) {
	if len(names) == 0 {
		names = Names{""}
	}
	for try := 0; ; try++ {
		for _, n := range names {
			ms, err := s.findAll(ctx, role, n, n != "", 5)
			if err != nil {
				return nil, "", err
			}
			for _, f := range ms {
				if f.Ref != "" {
					return &f, "", nil
				}
			}
		}
		if try >= tries(wait) {
			what := "no " + role
			if names[0] != "" {
				what += " named " + names.String()
			}
			return nil, fmt.Sprintf("found %s in %.0f s", what, wait.Seconds()), nil
		}
		if err := sleep(ctx, s.m.d.Poll); err != nil {
			return nil, "", err
		}
	}
}

// tries is how many more times a wait of d looks: once a second, which is
// the poll the app uses.
func tries(d time.Duration) int { return max(1, int(d/time.Second)) }

// waitFor waits until any of texts is in the window, or with gone until
// none is.
func (s *Session) waitFor(ctx context.Context, texts Names, gone bool, limit time.Duration) (bool, error) {
	for try := 0; ; try++ {
		there := false
		for _, t := range texts {
			data, err := s.m.d.Runner.Run(ctx, callTimeout, "find", "--window-id", s.win.ID, "--text", t, "--count")
			if err != nil {
				return false, s.fail(err)
			}
			var c struct {
				Count int `json:"count"`
			}
			_ = json.Unmarshal(data, &c)
			there = there || c.Count > 0
		}
		if there != gone {
			return true, nil
		}
		if try >= tries(limit) {
			return false, nil
		}
		if err := sleep(ctx, s.m.d.Poll); err != nil {
			return false, err
		}
	}
}

// logged records what a log step finds: a line per element, link
// addresses and a document's path left out.
func (s *Session) logged(ctx context.Context, st Step) ([]string, error) {
	limit := cmp.Or(st.Limit, defaultLog)
	var all []found
	parts := st.Name
	if len(parts) == 0 {
		parts = Names{""}
	}
	for _, p := range parts {
		ms, err := s.findAll(ctx, st.Role, p, false, limit)
		if err != nil {
			return nil, err
		}
		if all = ms; len(ms) > 0 {
			break
		}
	}
	lines := []string{fmt.Sprintf("%s: %d found", st, len(all))}
	for _, f := range all {
		n := f.node()
		if n.Role == "document" {
			n.Value = origin(n.Value)
		}
		lines = append(lines, "- "+describe(n))
	}
	return lines, nil
}

// look is a snapshot for the model, as desktop_snapshot gives it.
func (s *Session) look(ctx context.Context, full bool) (string, error) {
	n := s.snaps + 1
	v, snap, err := s.snapshot(ctx, full, n)
	if err != nil {
		return "", err
	}
	s.snaps, s.full, s.last, s.lastAll = n, full, v.lines, v.all
	s.remember(v)
	var b strings.Builder
	if s.browser {
		site := s.origin
		if site == "" {
			site = "no site yet"
		}
		fmt.Fprintf(&b, "Browser window %q (snapshot %d), page: %s\n", title(s.win), n, site)
	} else {
		fmt.Fprintf(&b, "Window %q (snapshot %d)\n", title(s.win), n)
	}
	b.WriteString(v.text)
	switch {
	case snap.Unproven:
		b.WriteString("[the tree is incomplete: the app didn't answer in time, and its ids may be refused. Take a new snapshot before you act]\n")
	case !snap.Complete:
		b.WriteString("[the tree is incomplete: the app didn't answer in time]\n")
	}
	return b.String(), nil
}
