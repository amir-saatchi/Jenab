package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// withStore gives h a registry for macros and recipes, and a calculator
// window, w-8, whose display follows its buttons.
func (h *harness) withStore() *store.Registry {
	h.t.Helper()
	reg, err := store.OpenRegistry(context.Background(), filepath.Join(h.t.TempDir(), "registry.db"))
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { reg.Close() })
	h.build(reg)
	h.f.windows = append(h.f.windows, Window{ID: "w-8", Title: "Calculator", App: "CalculatorApp.exe", PID: 80, Instance: "i80", Visible: true})
	h.f.trees["w-8"] = calcTree("0")
	h.f.onAct = func(f *fake, cmd, ref string) {
		if !strings.HasPrefix(ref, "@c:") || cmd != "click" {
			return
		}
		n := f.node("w-8", ref)
		d := f.node("w-8", "@c:d")
		shown := strings.TrimPrefix(d.Name, "Display is ")
		if shown == "0" {
			shown = ""
		}
		switch {
		case n.Name == "Plus":
			shown += "+"
		case n.Name == "Equals":
			sum := 0
			for _, p := range strings.Split(shown, "+") {
				v, _ := strconv.Atoi(p)
				sum += v
			}
			shown = strconv.Itoa(sum)
		default:
			shown += string(keypad[n.Name])
		}
		d.Name = "Display is " + shown
	}
	return reg
}

func calcTree(display string) Node {
	kids := []Node{{Role: "statictext", Name: "Display is " + display, Ref: "@c:d"}}
	for _, k := range []string{"Zero", "One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine", "Plus", "Equals"} {
		kids = append(kids, Node{Role: "button", Name: k, Ref: "@c:" + strings.ToLower(k)})
	}
	return Node{Role: "window", Name: "Calculator", Children: kids}
}

// nextTurn ends the turn and starts the next one.
func (h *harness) nextTurn() {
	h.endTurn()
	h.env.Turn++
	h.cards = nil
}

// clicks takes a snapshot and clicks the buttons named, by hand.
func (h *harness) clicks(names ...string) {
	h.t.Helper()
	snap := h.ok("desktop_snapshot", map[string]any{})
	for _, n := range names {
		h.ok("desktop_act", map[string]string{"id": idOf(h.t, snap, "button", n), "action": "click"})
	}
}

// idOf is the id of the element with role and name in a snapshot's text.
func idOf(t *testing.T, snap, role, name string) string {
	t.Helper()
	for _, l := range strings.Split(snap, "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, fmt.Sprintf("- %s %q [", role, name)) {
			return l[strings.Index(l, "[")+1 : strings.Index(l, "]")]
		}
	}
	t.Fatalf("no %s %q in:\n%s", role, name, snap)
	return ""
}

func macroSteps(t *testing.T, reg *store.Registry, app, name string) (store.DesktopMacro, []Step) {
	t.Helper()
	ms, err := reg.DesktopMacros(context.Background(), app)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ms {
		if m.Name == name {
			var steps []Step
			if err := json.Unmarshal(m.Steps, &steps); err != nil {
				t.Fatal(err)
			}
			return m, steps
		}
	}
	t.Fatalf("no macro %s for %s", name, app)
	return store.DesktopMacro{}, nil
}

// adds is the calculator task: a+b by hand, saved as the macro add.
func (h *harness) adds(a, b string) {
	h.t.Helper()
	h.pick = "w-8"
	h.ok("desktop_start", map[string]string{"app": "calculatorapp", "why": "add"})
	var names []string
	for _, r := range a + "+" + b {
		names = append(names, map[rune]string{'0': "Zero", '1': "One", '2': "Two", '3': "Three", '4': "Four", '5': "Five", '6': "Six", '7': "Seven", '8': "Eight", '9': "Nine", '+': "Plus"}[r])
	}
	h.clicks(append(names, "Equals")...)
}

func TestRecorderKeepsWhatWorked(t *testing.T) {
	h := newHarness(t)
	h.withStore()
	h.ok("desktop_start", map[string]string{"why": "x"})
	snap := h.ok("desktop_snapshot", map[string]any{})
	h.ok("desktop_act", map[string]string{"id": idOf(t, snap, "textfield", "Text"), "action": "set_value", "text": "Hel"})
	h.ok("desktop_act", map[string]string{"id": idOf(t, snap, "textfield", "Text"), "action": "set_value", "text": "Hello world"})
	h.fails("desktop_act", map[string]string{"id": idOf(t, snap, "textfield", "Password"), "action": "set_value", "text": "x"}, "password")
	h.fails("desktop_act", map[string]string{"id": idOf(t, snap, "button", "Accept all cookies"), "action": "click"}, "")
	h.ok("desktop_act", map[string]string{"id": idOf(t, snap, "button", "Save"), "action": "click"})
	h.ok("desktop_wait", map[string]any{"text": "Hello"})
	s := h.m.sessions[key{h.env.Project.ID, h.env.Chat}]
	got := listing(s.done)
	want := "1 set_value textfield \"Text\" text \"Hello world\"\n2 click button \"Save\"\n3 wait for \"Hello\"\n"
	if got != want {
		t.Fatalf("recorded:\n%s\nwant:\n%s", got, want)
	}
	if s.byHand != 3 {
		t.Errorf("by hand %d, want 3", s.byHand)
	}
}

func TestSaveFoldsKeypadAndPutsParams(t *testing.T) {
	h := newHarness(t)
	reg := h.withStore()
	h.adds("12", "5")
	out := h.ok("desktop_save_macro", map[string]any{"name": "add", "description": "Adds two numbers.", "params": []string{"a=12", "b=5"},
		"log": []map[string]any{{"role": "statictext", "name": "Display is"}}})
	if !strings.Contains(out, "Saved macro add, version 1") {
		t.Fatalf("save: %s", out)
	}
	m, steps := macroSteps(t, reg, "calculatorapp", "add")
	got := listing(steps)
	want := "1 click_each button text \"{a}\"\n2 click button \"Plus\"\n3 click_each button text \"{b}\"\n4 click button \"Equals\"\n5 log statictext \"Display is\"\n"
	if got != want {
		t.Fatalf("steps:\n%s\nwant:\n%s", got, want)
	}
	if !slices.Contains(steps[0].Map, "1=One") || !slices.Equal(m.Params, []string{"a", "b"}) || !m.Untrusted || m.MadeInChat != string(h.env.Chat) {
		t.Fatalf("macro %+v, map %v", m, steps[0].Map)
	}
}

// The same steps saved again are no new version.
func TestSaveTheSameStepsAgain(t *testing.T) {
	h := newHarness(t)
	reg := h.withStore()
	h.adds("12", "5")
	save := map[string]any{"name": "add", "description": "Adds two numbers.", "params": []string{"a=12", "b=5"},
		"log": []map[string]any{{"role": "statictext", "name": "Display is"}}}
	h.ok("desktop_save_macro", save)
	h.nextTurn()
	h.adds("30", "4")
	save["params"] = []string{"a=30", "b=4"}
	if out := h.ok("desktop_save_macro", save); !strings.Contains(out, "Not saved again: macro add (version 1)") {
		t.Fatalf("second save: %s", out)
	}
	if m, _ := macroSteps(t, reg, "calculatorapp", "add"); m.Version != 1 {
		t.Fatalf("version %d", m.Version)
	}
	recipe := map[string]any{"name": "adding", "text": "Click the digits, Plus, the digits, Equals."}
	h.ok("desktop_save_recipe", recipe)
	if out := h.ok("desktop_save_recipe", recipe); !strings.Contains(out, "Not saved again: recipe adding") {
		t.Fatalf("second recipe save: %s", out)
	}
}

func TestSaveRefusesAParamInNoStep(t *testing.T) {
	h := newHarness(t)
	h.withStore()
	h.adds("12", "5")
	h.fails("desktop_save_macro", map[string]any{"name": "add", "description": "d", "params": []string{"a=12", "b=7"}},
		`the value of b ("7") is in none of the steps`)
	h.fails("desktop_save_macro", map[string]any{"name": "Add Numbers", "description": "d"}, "lower_snake_case")
	h.fails("desktop_save_macro", map[string]any{"name": "add", "description": "d", "skip": []int{9}}, "skip 9 is not a recorded step")
}

func TestSaveMakesADeclineOptional(t *testing.T) {
	h := newHarness(t)
	reg := h.withStore()
	h.ok("desktop_start", map[string]string{"why": "x"})
	snap := h.ok("desktop_snapshot", map[string]any{})
	h.ok("desktop_act", map[string]string{"id": idOf(t, snap, "button", "Reject all"), "action": "click"})
	h.ok("desktop_act", map[string]string{"id": idOf(t, snap, "textfield", "Text"), "action": "set_value", "text": "blue folders"})
	h.ok("desktop_save_macro", map[string]any{"name": "write", "description": "Writes.", "params": []string{"words=blue folders"},
		"log": []map[string]any{{"role": "textfield", "name": "Text"}}})
	_, steps := macroSteps(t, reg, "notes", "write")
	if !steps[0].Optional || steps[1].Text != "{words}" || steps[1].Optional {
		t.Fatalf("steps:\n%s", listing(steps))
	}
}

func TestSaveChecksTheLog(t *testing.T) {
	h := newHarness(t)
	reg := h.withStore()
	h.adds("3", "4")
	out := h.ok("desktop_save_macro", map[string]any{"name": "add", "description": "Adds.", "params": []string{"a=3", "b=4"},
		"log": []map[string]any{{"role": "link", "name": "Result"}, {"role": "statictext", "name": "Display"}, {"role": "statictext", "name": "Display"}}})
	if !strings.Contains(out, `Left out the step log link "Result"`) {
		t.Fatalf("save: %s", out)
	}
	_, steps := macroSteps(t, reg, "calculatorapp", "add")
	if n := len(slices.DeleteFunc(slices.Clone(steps), func(s Step) bool { return s.Do != "log" })); n != 1 {
		t.Fatalf("%d log steps, want 1 (no duplicate):\n%s", n, listing(steps))
	}

	// No usable log: Jenab logs what the last action changed.
	h.nextTurn()
	h.f.trees["w-8"] = calcTree("0")
	h.adds("2", "6")
	out = h.ok("desktop_save_macro", map[string]any{"name": "add_more", "description": "Adds.", "params": []string{"a=2", "b=6"},
		"log": []map[string]any{{"role": "link", "name": "Nothing"}, {"role": "button"}}})
	if !strings.Contains(out, "Jenab logs what your last action changed") || !strings.Contains(out, "Left out the step log button: with no name") {
		t.Fatalf("save: %s", out)
	}
	_, steps = macroSteps(t, reg, "calculatorapp", "add_more")
	if last := steps[len(steps)-1]; last.Do != "log" || last.Role != "statictext" || last.Name.first() != "Display is" {
		t.Fatalf("automatic log: %s", listing(steps))
	}
}

func TestReplayWithOtherParams(t *testing.T) {
	h := newHarness(t)
	reg := h.withStore()
	h.adds("12", "5")
	h.ok("desktop_save_macro", map[string]any{"name": "add", "description": "Adds two numbers.", "params": []string{"a=12", "b=5"},
		"log": []map[string]any{{"role": "statictext", "name": "Display is"}}})
	for i, c := range []struct{ a, b, sum string }{{"7", "8", "15"}, {"40", "2", "42"}, {"123", "0", "123"}} {
		h.nextTurn()
		h.f.trees["w-8"] = calcTree("0")
		start := h.ok("desktop_start", map[string]string{"app": "calculatorapp", "why": "add"})
		if !strings.Contains(start, "- macro add(a, b): Adds two numbers.") || strings.Contains(start, "ago") {
			t.Fatalf("start lists the macro without its age:\n%s", start)
		}
		if !strings.Contains(start, "- recipe calculator: The digits are separate buttons") {
			t.Fatalf("start lists the shipped recipe:\n%s", start)
		}
		before := len(h.f.calls)
		out := h.ok("desktop_run_macro", map[string]any{"name": "add", "params": []string{"a=" + c.a, "b=" + c.b}})
		if !strings.Contains(out, "Done: macro add (version 1) ran 5 steps") || !strings.Contains(out, `statictext "Display is `+c.sum+`"`) {
			t.Fatalf("run %d: %s", i, out)
		}
		if strings.Contains(out, "last worked") {
			t.Fatalf("a fresh macro has no age note: %s", out)
		}
		// Each step went through the CLI: no snapshot ids, the refs find gave.
		var clicked int
		for _, cl := range h.f.calls[before:] {
			if cl[0] == "click" {
				clicked++
			}
		}
		if want := len(c.a) + len(c.b) + 2; clicked != want {
			t.Fatalf("run %d: %d clicks, want %d", i, clicked, want)
		}
	}
	m, _ := macroSteps(t, reg, "calculatorapp", "add")
	runs, _ := reg.DesktopRuns(context.Background(), m.ID)
	if m.Runs != 3 || m.Failures != 0 || len(runs) != 3 || !runs[0].OK {
		t.Fatalf("macro %+v runs %+v", m, runs)
	}
}

func TestReplayAgeNote(t *testing.T) {
	h := newHarness(t)
	h.withStore()
	h.adds("1", "2")
	h.ok("desktop_save_macro", map[string]any{"name": "add", "description": "Adds.", "params": []string{"a=1", "b=2"}})
	h.nextTurn()
	h.now = h.now.Add(3*24*time.Hour + time.Hour)
	h.f.trees["w-8"] = calcTree("0")
	start := h.ok("desktop_start", map[string]string{"app": "calculatorapp", "why": "add"})
	if strings.Contains(start, "days") {
		t.Fatalf("the list has no age: %s", start)
	}
	out := h.ok("desktop_run_macro", map[string]any{"name": "add", "params": []string{"a=3", "b=4"}})
	if !strings.Contains(out, "It last worked 3 days ago: check the result in the window with a snapshot") {
		t.Fatalf("run: %s", out)
	}
	h.nextTurn()
	h.now = h.now.Add(time.Hour)
	h.f.trees["w-8"] = calcTree("0")
	h.ok("desktop_start", map[string]string{"app": "calculatorapp", "why": "add"})
	if out := h.ok("desktop_run_macro", map[string]any{"name": "add", "params": []string{"a=3", "b=4"}}); strings.Contains(out, "last worked") {
		t.Fatalf("worked an hour ago: no note: %s", out)
	}
}

func TestReplayFailureHandsOver(t *testing.T) {
	h := newHarness(t)
	reg := h.withStore()
	h.adds("1", "2")
	h.ok("desktop_save_macro", map[string]any{"name": "add", "description": "Adds.", "params": []string{"a=1", "b=2"}})
	h.nextTurn()
	// Another language: the Plus button is now Plus-Taste.
	tr := calcTree("0")
	for i := range tr.Children {
		if tr.Children[i].Name == "Plus" {
			tr.Children[i].Name = "Addieren"
		}
	}
	h.f.trees["w-8"] = tr
	h.ok("desktop_start", map[string]string{"app": "calculatorapp", "why": "add"})
	out := h.ok("desktop_run_macro", map[string]any{"name": "add", "params": []string{"a=4", "b=5"}})
	for _, want := range []string{
		`Failed at step 2 of`, `(click button "Plus"): found no button named "Plus" in 10 s`,
		"Steps before it:\n1 click_each button", "The window now:", `button "Addieren" [`, "Carry on by hand from here",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("failure lacks %q:\n%s", want, out)
		}
	}
	// By hand from there, then the fix: the old name stays an alternative.
	snap := h.ok("desktop_snapshot", map[string]any{})
	for _, n := range []string{"Addieren", "Five", "Equals"} {
		h.ok("desktop_act", map[string]string{"id": idOf(t, snap, "button", n), "action": "click"})
	}
	out = h.ok("desktop_save_macro", map[string]any{"name": "add", "description": "Adds.", "params": []string{"a=4", "b=5"}})
	m, steps := macroSteps(t, reg, "calculatorapp", "add")
	if m.Version != 2 || !slices.Equal(steps[1].Name, Names{"Addieren", "Plus"}) {
		t.Fatalf("fix:\n%s\n%s", out, listing(steps))
	}
	// The next replay takes whichever name is there.
	h.nextTurn()
	h.f.trees["w-8"] = calcTree("0")
	h.ok("desktop_start", map[string]string{"app": "calculatorapp", "why": "add"})
	if out := h.ok("desktop_run_macro", map[string]any{"name": "add", "params": []string{"a=6", "b=3"}}); !strings.Contains(out, `"Display is 9"`) {
		t.Fatalf("replay after the fix: %s", out)
	}
}

func TestFixKeepsParams(t *testing.T) {
	h := newHarness(t)
	h.withStore()
	h.adds("1", "2")
	h.ok("desktop_save_macro", map[string]any{"name": "add", "description": "Adds.", "params": []string{"a=1", "b=2"}})
	h.nextTurn()
	h.f.trees["w-8"] = calcTree("0")
	h.ok("desktop_start", map[string]string{"app": "calculatorapp", "why": "add"})
	h.ok("desktop_run_macro", map[string]any{"name": "add", "params": []string{"a=3", "b=4"}})
	h.nextTurn()
	h.adds("3", "4")
	h.fails("desktop_save_macro", map[string]any{"name": "add", "description": "Adds 3 and 4.", "params": []string{"a=3"}},
		"has worked with the params a, b; a new version must keep them")
	h.ok("desktop_save_macro", map[string]any{"name": "add_three", "description": "Adds 3 and something.", "params": []string{"b=4"}})
}

func TestReplayKeepsTheGuards(t *testing.T) {
	h := newHarness(t)
	reg := h.withStore()
	ctx := context.Background()
	for _, c := range []struct {
		name string
		step Step
		want string
	}{
		{"consent", Step{Do: "click", Role: "button", Name: Names{"Accept all cookies"}}, "consent"},
		{"password", Step{Do: "set_value", Role: "textfield", Name: Names{"Password"}, Text: "{x}"}, "password"},
	} {
		steps := []Step{c.step}
		if c.step.Text == "" {
			steps = append(steps, Step{Do: "set_value", Role: "textfield", Name: Names{"Text"}, Text: "{x}"})
		}
		raw, _ := json.Marshal(steps)
		if _, err := reg.SaveDesktopMacro(ctx, store.DesktopMacro{App: "notes", Name: c.name, Description: "d", Params: []string{"x"}, Steps: raw}, h.now); err != nil {
			t.Fatal(err)
		}
		h.nextTurn()
		h.pick = "w-1"
		h.ok("desktop_start", map[string]string{"why": "x"})
		out := h.ok("desktop_run_macro", map[string]any{"name": c.name, "params": []string{"x=hello"}})
		if !strings.Contains(out, "Failed at step 1") || !strings.Contains(strings.ToLower(out), c.want) {
			t.Fatalf("%s: %s", c.name, out)
		}
	}
	if len(h.f.ran("set-value")) != 0 {
		t.Fatalf("a guarded step reached the app: %v", h.f.ran("set-value"))
	}
	for _, cl := range h.f.ran("click") {
		if cl[1] == "@s:e4" {
			t.Fatal("the consent click reached the app")
		}
	}
}

func TestStrictCardListsTheFilledSteps(t *testing.T) {
	h := newHarness(t)
	h.withStore()
	h.adds("1", "2")
	h.ok("desktop_save_macro", map[string]any{"name": "add", "description": "Adds.", "params": []string{"a=1", "b=2"}})
	h.nextTurn()
	h.f.trees["w-8"] = calcTree("0")
	h.ok("desktop_start", map[string]string{"app": "calculatorapp", "why": "add"})
	h.cards = nil
	h.ok("desktop_run_macro", map[string]any{"name": "add", "params": []string{"a=34", "b=5"}})
	var macro []chat.Approval
	for _, c := range h.cards {
		if c.Kind == KindMacro {
			macro = append(macro, c)
		} else if c.Kind == KindAction {
			t.Fatalf("a replay asks once, not per step: %+v", c)
		}
	}
	if len(macro) != 1 || !strings.Contains(macro[0].Details, `1 click_each button text "34"`) || !strings.Contains(macro[0].Ask, "Run the macro add") {
		t.Fatalf("cards: %+v", h.cards)
	}
	// Denied: nothing runs.
	h.nextTurn()
	h.f.trees["w-8"] = calcTree("0")
	h.ok("desktop_start", map[string]string{"app": "calculatorapp", "why": "add"})
	h.answer = chat.GrantDeny
	clicks := len(h.f.ran("click"))
	h.fails("desktop_run_macro", map[string]any{"name": "add", "params": []string{"a=3", "b=5"}}, "the user denied it")
	if len(h.f.ran("click")) != clicks {
		t.Fatal("a denied replay clicked")
	}
}

func TestReplayParamsAreChecked(t *testing.T) {
	h := newHarness(t)
	h.withStore()
	h.adds("1", "2")
	h.ok("desktop_save_macro", map[string]any{"name": "add", "description": "Adds.", "params": []string{"a=1", "b=2"}})
	h.nextTurn()
	h.ok("desktop_start", map[string]string{"app": "calculatorapp", "why": "add"})
	h.fails("desktop_run_macro", map[string]any{"name": "add", "params": []string{"a=3"}}, `"b" is missing`)
	h.fails("desktop_run_macro", map[string]any{"name": "add", "params": []string{"a=3", "b=1", "c=2"}}, `no param "c"`)
	h.fails("desktop_run_macro", map[string]any{"name": "sub"}, "saved ones are: add")
}

func TestThreeFailuresRemoveTheMacro(t *testing.T) {
	h := newHarness(t)
	reg := h.withStore()
	h.adds("1", "2")
	h.ok("desktop_save_macro", map[string]any{"name": "add", "description": "Adds.", "params": []string{"a=1", "b=2"}})
	var out string
	for range 3 {
		h.nextTurn()
		tr := calcTree("0")
		tr.Children = slices.DeleteFunc(tr.Children, func(n Node) bool { return n.Name == "Equals" })
		h.f.trees["w-8"] = tr
		h.ok("desktop_start", map[string]string{"app": "calculatorapp", "why": "add"})
		out = h.ok("desktop_run_macro", map[string]any{"name": "add", "params": []string{"a=3", "b=4"}})
	}
	if !strings.Contains(out, "It failed 3 replays in a row, so it was removed") {
		t.Fatalf("third failure: %s", out)
	}
	if ms, _ := reg.DesktopMacros(context.Background(), "calculatorapp"); len(ms) != 0 {
		t.Fatalf("still listed: %+v", ms)
	}
	h.nextTurn()
	if start := h.ok("desktop_start", map[string]string{"app": "calculatorapp", "why": "add"}); strings.Contains(start, "macro add") {
		t.Fatalf("start still lists it: %s", start)
	}
}

func TestSaveNoticeUndo(t *testing.T) {
	h := newHarness(t)
	reg := h.withStore()
	ctx := context.Background()
	h.adds("1", "2")
	b, _ := json.Marshal(map[string]any{"name": "add", "description": "Adds.", "params": []string{"a=1", "b=2"}})
	r, err := tool.Run(ctx, h.tools["desktop_save_macro"], tool.Call{ID: "s", Args: b, Env: h.env})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Notices) != 1 || r.Notices[0].Kind != chat.NoticeDesktopSaved || !strings.HasPrefix(r.Notices[0].Target, "desktop:macro:") || r.Notices[0].Change == "" {
		t.Fatalf("notice: %+v", r.Notices)
	}
	n, _ := strconv.ParseInt(r.Notices[0].Change, 10, 64)
	u, err := reg.UndoDesktopSave(ctx, n, false)
	if err != nil || !u.Done || !u.Undone {
		t.Fatalf("undo %+v %v", u, err)
	}
	if ms, _ := reg.DesktopMacros(ctx, "calculatorapp"); len(ms) != 0 {
		t.Fatal("an undone new macro is gone")
	}
	if u, err = reg.UndoDesktopSave(ctx, n, true); err != nil || !u.Done || u.Undone {
		t.Fatalf("redo %+v %v", u, err)
	}
	macroSteps(t, reg, "calculatorapp", "add")

	// A recipe's notice too, after desktop_end in the same turn.
	h.ok("desktop_end", map[string]any{})
	b, _ = json.Marshal(map[string]any{"name": "adding", "text": "Click the digits, then Plus, then Equals."})
	r, err = tool.Run(ctx, h.tools["desktop_save_recipe"], tool.Call{ID: "r", Args: b, Env: h.env})
	if err != nil || len(r.Notices) != 1 || !strings.HasPrefix(r.Notices[0].Target, "desktop:recipe:") {
		t.Fatalf("recipe: %+v %v", r, err)
	}
	h.fails("desktop_save_recipe", map[string]any{"name": "long", "text": strings.Repeat("a", 1201)}, "maxLength")
	h.nextTurn()
	h.fails("desktop_save_recipe", map[string]any{"name": "late", "text": "t"}, "no desktop session in this turn")
}

func TestReminderOncePerSession(t *testing.T) {
	h := newHarness(t)
	h.withStore()
	var checks []func() string
	h.env.BeforeAnswer = func(f func() string) { checks = append(checks, f) }
	remind := func() string {
		for _, c := range checks {
			if s := c(); s != "" {
				return s
			}
		}
		return ""
	}
	h.ok("desktop_start", map[string]string{"why": "x"})
	if s := remind(); s != "" {
		t.Fatalf("nothing done by hand, but: %s", s)
	}
	snap := h.ok("desktop_snapshot", map[string]any{})
	h.ok("desktop_act", map[string]string{"id": idOf(t, snap, "button", "Save"), "action": "click"})
	if s := remind(); s != reminder {
		t.Fatalf("reminder %q", s)
	}
	if s := remind(); s != "" {
		t.Fatalf("a second reminder: %s", s)
	}
	// Saved: no reminder.
	h.nextTurn()
	checks = nil
	h.adds("1", "2")
	h.ok("desktop_save_macro", map[string]any{"name": "add", "description": "Adds.", "params": []string{"a=1", "b=2"}})
	if s := remind(); s != "" {
		t.Fatalf("saved, but: %s", s)
	}
	// Without a store, the tools save nothing and there is no reminder.
	h2 := newHarness(t)
	h2.env.BeforeAnswer = func(f func() string) { t.Fatal("no store, no reminder") }
	h2.ok("desktop_start", map[string]string{"why": "x"})
}

func TestStepsFormat(t *testing.T) {
	raw := []byte(`[{"do":"click","role":"button","name":["Search","Suche"]},{"do":"set_value","role":"textfield","name":"Query","text":"{q}"},` +
		`{"do":"press","key":"return"},{"do":"wait","name":"results"},{"do":"log","role":"link","limit":5}]`)
	steps, err := ParseSteps(raw, []string{"q"})
	if err != nil {
		t.Fatal(err)
	}
	back, _ := json.Marshal(steps)
	if !strings.Contains(string(back), `"name":"Query"`) || !strings.Contains(string(back), `"name":["Search","Suche"]`) {
		t.Fatalf("one name is a string, more a list: %s", back)
	}
	for _, c := range []struct {
		raw, params, want string
	}{
		{`[{"do":"click","role":"button","name":"{x}"}]`, "", "not a param"},
		{`[{"do":"click","role":"button","name":"A"}]`, "x", `"x" is in no step`},
		{`[{"do":"click","name":"A"}]`, "", "needs the element's role"},
		{`[{"do":"press","key":"f4"}]`, "", "press takes key return"},
		{`[{"do":"click","role":"button","name":"3:e12"}]`, "", "snapshot id"},
		{`[{"do":"run","role":"button"}]`, "", "unknown do"},
		{`[]`, "", "1 to 40 steps"},
	} {
		var ps []string
		if c.params != "" {
			ps = []string{c.params}
		}
		if _, err := ParseSteps([]byte(c.raw), ps); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", c.raw, err, c.want)
		}
	}
}

func TestAlign(t *testing.T) {
	old := []Step{
		{Do: "click", Role: "button", Name: Names{"Accept"}, Optional: true},
		{Do: "set_value", Role: "textfield", Name: Names{"Search"}, Text: "{q}"},
		{Do: "click", Role: "button", Name: Names{"Search"}},
		{Do: "log", Role: "link"},
	}
	cur := []Step{
		{Do: "set_value", Role: "textfield", Name: Names{"Suche"}, Text: "{q}"},
		{Do: "click", Role: "button", Name: Names{"Suchen"}},
		{Do: "log", Role: "link"},
	}
	got := align(old, cur)
	if !slices.Equal(got[0].Name, Names{"Suche", "Search"}) || !slices.Equal(got[1].Name, Names{"Suchen", "Search"}) || len(got) != 3 || got[2].Name != nil {
		t.Fatalf("aligned:\n%s", listing(got))
	}
}

// The automatic log leaves the window out and keeps the part of a name
// that stayed.
func TestAutomaticLog(t *testing.T) {
	before := []Node{{Role: "window", Name: "Calc"}, {Role: "statictext", Name: "Display is 0"}}
	before = append(before, Node{Role: "button", Name: "Equals"})
	changed := []Node{{Role: "window", Name: "Calc"}, {Role: "button", Name: "Equals", States: []string{"focused"}}, {Role: "statictext", Name: "Display is 72"}}
	got := logOf(changed, before)
	if len(got) != 1 || got[0].Role != "statictext" || !slices.Equal(got[0].Name, Names{"Display is"}) {
		t.Fatalf("log: %+v", got)
	}
}

// A Clear or a rewind starts the turn numbers again: EndChat drops the
// chat's session and the steps kept for its saves.
func TestEndChat(t *testing.T) {
	h := newHarness(t)
	h.withStore()
	h.adds("1", "2")
	h.ok("desktop_end", map[string]any{})
	h.m.EndChat(h.env.Project.ID, h.env.Chat)
	h.fails("desktop_save_macro", map[string]any{"name": "add", "description": "Adds.", "params": []string{"a=1", "b=2"}}, "no desktop session in this turn")
	h.adds("3", "4")
	h.m.EndChat(h.env.Project.ID, h.env.Chat)
	h.fails("desktop_snapshot", map[string]any{}, "no window is in use")
}
