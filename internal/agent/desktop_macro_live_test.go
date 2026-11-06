//go:build windows

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/desktop"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/backends"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
	"github.com/amir-saatchi/jenab/internal/secret"
	"github.com/amir-saatchi/jenab/internal/skill"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// TestDesktopMacroLive runs macros end to end (SPEC 8.11, TASK-038): per
// app, a run by hand that should save a macro, three replays with other
// params (the last with the clock 3 days on, for the age note), a break
// (another language or mode) the model should carry on from and fix, and
// a replay after the fix in the first language or mode. It drives only
// windows it opens: the test editor, a new Calculator window and Chrome
// with a new profile on a stand-in search site on 127.0.0.1.
//
//	JENAB_DESKTOP_LIVE, JENAB_AGENT_DESKTOP, JENAB_TEST_EDITOR and
//	JENAB_DESKTOP_MODEL as for TestDesktopLive
//	JENAB_DESKTOP_SCRATCH  the folder for the registry, logs and Chrome's
//	                       profiles (kept); required
//	JENAB_DESKTOP_APPS     default editor,calculator,site
//	JENAB_DESKTOP_MAX_REQUESTS  a run's cap, default 30
//	JENAB_DESKTOP_BUDGET   the test's cap; runs left are skipped, default 250
//	JENAB_DESKTOP_PACE_MS  the least time between two requests, default 0
//	JENAB_CHROME           chrome.exe, default Program Files
//	JENAB_DESKTOP_DRY      "1": open, grade and close each window, no model
//	JENAB_DESKTOP_ONLY_REPLAY  a folder of an earlier run: no model runs,
//	                       only the replays without the model of its macros
//	JENAB_DESKTOP_SITE_PORT  the stand-in site's port, default any (a
//	                       replay of an earlier run's macro needs its port)
//	JENAB_DESKTOP_TRACE    "1": every run's calls and results, not only
//	                       the failed, make and break runs
func TestDesktopMacroLive(t *testing.T) {
	envFile, prog, editor, scratch := os.Getenv("JENAB_DESKTOP_LIVE"), os.Getenv("JENAB_AGENT_DESKTOP"), os.Getenv("JENAB_TEST_EDITOR"), os.Getenv("JENAB_DESKTOP_SCRATCH")
	if envFile == "" || prog == "" || editor == "" || scratch == "" {
		t.Skip("set JENAB_DESKTOP_LIVE, JENAB_AGENT_DESKTOP, JENAB_TEST_EDITOR and JENAB_DESKTOP_SCRATCH to run it")
	}
	spec := strings.Split(cmpOr(os.Getenv("JENAB_DESKTOP_MODEL"), "gemini//gemini-3.5-flash-lite/GEMINI_API_KEY"), "/")
	if len(spec) < 4 {
		t.Fatal("JENAB_DESKTOP_MODEL: kind/base-url/model/key-variable")
	}
	kind, model, keyVar := spec[0], spec[len(spec)-2], spec[len(spec)-1]
	base := strings.Join(spec[1:len(spec)-2], "/")
	key := readEnv(t, envFile, keyVar)
	maxRequests, _ := strconv.Atoi(cmpOr(os.Getenv("JENAB_DESKTOP_MAX_REQUESTS"), "30"))
	budget, _ := strconv.Atoi(cmpOr(os.Getenv("JENAB_DESKTOP_BUDGET"), "250"))
	pace, _ := strconv.Atoi(os.Getenv("JENAB_DESKTOP_PACE_MS"))
	apps := strings.Split(cmpOr(os.Getenv("JENAB_DESKTOP_APPS"), "editor,calculator,site"), ",")

	only := os.Getenv("JENAB_DESKTOP_ONLY_REPLAY")
	root := cmpOr(only, filepath.Join(scratch, "macro-"+model+"-"+time.Now().Format("0102-150405")))
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	set := config.Defaults()
	set.Approvals.DefaultLevel = "standard"
	set.LLM.Models = map[string]string{"default": "live/" + model}
	// "replay" is a scripted model for the replays without the model.
	set.LLM.Providers = map[string]config.ProviderSettings{"live": {Kind: kind, BaseURL: base, Models: []config.ModelSettings{{ID: model}}},
		"replay": {Kind: "replay", Models: []config.ModelSettings{{ID: "script", Context: 1_000_000}}}}
	set.DesktopControl = true
	logs := &lines{}
	log := slog.New(slog.NewTextHandler(logs, nil))
	paths := config.Paths{Root: root, Registry: filepath.Join(root, "registry.db")}.WithDataFolder(filepath.Join(root, "data"))
	ctx := context.Background()
	reg, err := store.OpenRegistry(ctx, paths.Registry)
	if err != nil {
		t.Fatal(err)
	}
	pm := project.NewManager(project.Deps{Paths: paths, Registry: reg})
	pid, err := pm.Create(ctx, "Desktop macros "+time.Now().Format("150405"))
	if err != nil {
		t.Fatal(err)
	}
	count := &requestCount{pace: time.Duration(pace) * time.Millisecond}
	bk := backends.All()
	inner := bk[provider.Kind(kind)]
	script := fake.New()
	bk["replay"] = script.Factory()
	bk[provider.Kind(kind)] = func(c provider.Connection) (provider.Provider, error) {
		p, err := inner(c)
		return countingProvider{p, count}, err
	}
	models := provider.NewRegistry(provider.Deps{Settings: set.LLM, Gate: limit.NewGate(2), Backends: bk, Catalog: provider.MustCatalog(), Log: log,
		Secrets: secret.New(&memKeyring{m: map[string]string{provider.KeyName("live"): key, provider.KeyName("replay"): "none"}})})
	cli := &desktop.CLI{Path: prog, Home: filepath.Join(root, "agent-desktop")}
	var shift atomic.Int64 // the clock's offset, for the age note
	dm := desktop.NewManager(desktop.Deps{Runner: cli, On: func() bool { return true }, Self: os.Getpid(), Class: desktop.WindowClass, Log: log, Store: reg,
		Now: func() time.Time { return time.Now().Add(time.Duration(shift.Load())) }})
	tools := tool.NewRegistry()
	tools.Add(tool.Builtin(tool.Deps{})...)
	tools.Add(Tools()...)
	tools.Add(dm.Tools()...)
	skills, err := skill.Builtin()
	if err != nil {
		t.Fatal(err)
	}
	appCtx, cancel := context.WithCancel(ctx)
	o := New(Deps{Context: appCtx, Projects: pm, Models: models, Tools: tools, Skills: skills, Log: log,
		Settings: func() config.Settings { return set }, Events: &recorder{}})
	defer func() {
		o.Refuse()
		cancel()
		wctx, done := context.WithTimeout(ctx, 30*time.Second)
		defer done()
		_ = o.Wait(wctx)
		_ = pm.CloseAll(wctx)
		_ = reg.Close()
		_ = os.WriteFile(filepath.Join(root, "log.txt"), []byte(logs.grep("desktop:", "provider:", "agent:")), 0o644)
	}()

	port, _ := strconv.Atoi(os.Getenv("JENAB_DESKTOP_SITE_PORT"))
	site := startStandIn(t, port)
	total := 0
	var report []string
	stopped := ""

	type phase struct {
		label string
		set   int
		brk   bool // another language or mode
		old   bool // the clock 3 days on
	}
	for _, name := range apps {
		a := liveApps(t, name, editor, cli, site, root)
		phases := []phase{{"make", 0, false, false}, {"replay", 1, false, false}, {"replay", 2, false, false}, {"replay, 3 days old", 3, false, true}}
		if a.breaks {
			phases = append(phases, phase{"break", 4, true, false}, phase{"after the fix", 5, false, false})
		}
		if only != "" {
			phases = nil
		}
		for _, ph := range phases {
			if stopped != "" {
				break
			}
			if total >= budget {
				stopped = fmt.Sprintf("the budget of %d requests is used", budget)
				break
			}
			params := a.params[ph.set]
			goal := a.goal
			for k, v := range params {
				goal = strings.ReplaceAll(goal, "{"+k+"}", v)
			}
			win, title, closeApp := a.open(ph.brk)
			if ph.old {
				shift.Store(int64(72 * time.Hour))
			}
			var r liveRun
			if os.Getenv("JENAB_DESKTOP_DRY") == "1" {
				t.Logf("dry: window %s %q; display or page: %s", win, title, clipText(editorText(t, cli, win), 1500))
			} else {
				r = liveTurn(t, o, pm, pid, count, goal, win, title, maxRequests)
			}
			shift.Store(0)
			ok, why := a.check(win, params, r.answer+"\n"+r.results)
			closeApp()
			total += r.requests
			line := fmt.Sprintf("%s %s (set %d): ok %v (%s), %d requests (%d failed, %d empty), tool calls %d, tokens in %d (+%d cached) out %d, %s; macro: %s; saved: %s",
				name, ph.label, ph.set, ok, why, r.requests, r.failed, r.empty, len(r.calls), r.in, r.cached, r.out, r.took.Round(time.Second), cmpOr(r.macro, "not run"), cmpOr(strings.Join(r.saved, "; "), "nothing"))
			t.Log(line)
			t.Logf("  calls: %s", strings.Join(r.calls, " "))
			t.Logf("  cards: %v; answer: %s", r.cards, clipText(r.answer, 300))
			report = append(report, line)
			if !ok || ph.brk || ph.label == "make" || os.Getenv("JENAB_DESKTOP_TRACE") == "1" {
				for _, s := range r.trace {
					t.Log("   ", s)
				}
			}
			if r.quota {
				stopped = "the provider says the quota is used"
			}
		}
		// Replays without the model, of the macro saved last for the app:
		// in the first language or mode, and in the break's.
		replays := []phase{{"no-model replay", 1, false, false}}
		if a.breaks {
			replays = append(replays, phase{"no-model replay, break", 4, true, false})
		}
		for _, ph := range replays {
			ms, _ := reg.DesktopMacros(ctx, a.app)
			if len(ms) == 0 || stopped != "" {
				t.Logf("%s %s: no macro to replay", name, ph.label)
				break
			}
			mc := ms[0]
			for _, m := range ms {
				if m.LastWorkedAt.After(mc.LastWorkedAt) {
					mc = m
				}
			}
			keys := slices.Sorted(maps.Keys(a.params[ph.set]))
			if len(keys) != len(mc.Params) {
				t.Logf("%s %s: macro %s has params %v, the task %d values: not replayed", name, ph.label, mc.Name, mc.Params, len(keys))
				break
			}
			var ps []string
			for i, k := range keys { // in name order; the sum doesn't mind
				ps = append(ps, mc.Params[i]+"="+a.params[ph.set][k])
			}
			script.Replace(scripted(fake.ToolCall("r1", "desktop_start", map[string]string{"app": a.app, "why": "Replay the saved macro."})),
				scripted(fake.ToolCall("r2", "desktop_run_macro", map[string]any{"name": mc.Name, "params": ps})), scripted(fake.Text("Replayed.")))
			win, title, closeApp := a.open(ph.brk)
			set.LLM.Models["default"] = "replay/script"
			r := liveTurn(t, o, pm, pid, count, "Replay the saved macro.", win, title, 0)
			set.LLM.Models["default"] = "live/" + model
			ok, why := a.check(win, a.params[ph.set], r.results)
			closeApp()
			line := fmt.Sprintf("%s %s (set %d) of %s v%d: ok %v (%s), %s; macro: %s", name, ph.label, ph.set, mc.Name, mc.Version, ok, why, r.took.Round(time.Second), cmpOr(r.macro, "not run"))
			t.Log(line)
			report = append(report, line)
			for _, s := range r.trace {
				t.Log("   ", s)
			}
		}
		ms, _ := reg.DesktopMacros(ctx, "")
		for _, m := range ms {
			t.Logf("macro %s/%s %s v%d params %v runs %d failures %d made by %s:\n%s", m.App, m.Site, m.Name, m.Version, m.Params, m.Runs, m.Failures, m.MadeByModel, desktop.Listing(m.Steps))
		}
	}
	rs, _ := reg.DesktopRecipes(ctx)
	for _, d := range rs {
		t.Logf("recipe %s/%s %s (%s, %d uses): %s", d.App, d.Site, d.Name, d.Source, d.Uses, clipText(d.Text, 300))
	}
	t.Logf("%d requests in all; folder %s", total, root)
	_ = os.WriteFile(filepath.Join(root, "report.txt"), []byte(strings.Join(report, "\n")+"\n"), 0o644)
	if stopped != "" {
		t.Errorf("stopped: %s", stopped)
	}
	if site.accepted.Load() > 0 {
		t.Errorf("a consent dialog was accepted %d times", site.accepted.Load())
	}
}

// scripted says the scripted model read the whole request, so the
// Registry doesn't take it for a silent truncation.
func scripted(r fake.Reply) fake.Reply {
	for i, ev := range r.Events {
		if ev.Usage != nil {
			u := *ev.Usage
			u.Input = 1_000_000
			r.Events[i].Usage = &u
		}
	}
	return r
}

// liveRun is one turn's numbers and what it did.
type liveRun struct {
	requests, failed, empty int
	in, out, cached         int
	took                    time.Duration
	calls, cards, saved     []string
	trace                   []string
	macro, answer           string
	results                 string // the full results of desktop_run_macro
	quota                   bool
}

// liveTurn sends goal in a new chat and answers the cards as the user
// would: the pick card with the test's own window only, the others Once.
func liveTurn(t *testing.T, o *Orchestrator, pm *project.Manager, pid id.Project, count *requestCount, goal, win, title string, maxRequests int) liveRun {
	t.Helper()
	ctx := context.Background()
	p, err := pm.Open(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Release()
	c, err := p.Chats.CreateChat(ctx, id.SourceUser, chat.Chat{Title: fmt.Sprintf("%s %d", title, time.Now().UnixNano())})
	if err != nil {
		t.Fatal(err)
	}
	count.reset()
	start := time.Now()
	if _, err := o.Send(ctx, pid, c.ID, UserMessage{Text: goal}); err != nil {
		t.Fatal(err)
	}
	var r liveRun
	capped := false
	deadline := time.Now().Add(8 * time.Minute)
	for o.State(pid, c.ID) != chat.StateIdle {
		if time.Now().After(deadline) && !capped {
			capped = true
			t.Logf("stopped after 8 minutes")
			o.Stop(pid, c.ID)
		}
		if n := count.get().requests; maxRequests > 0 && n >= maxRequests && !capped {
			capped = true
			t.Logf("stopped at %d requests", n)
			o.Stop(pid, c.ID)
		}
		l := o.Live(pid, c.ID)
		if l.Waiting == nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		a := approvalAt(t, p, c.ID, l.Waiting.Message, l.Waiting.Index)
		r.cards = append(r.cards, a.Kind)
		ans := Answer{Message: l.Waiting.Message, Index: l.Waiting.Index, Grant: chat.GrantOnce}
		if a.Kind == desktop.KindWindow {
			ans.Grant = chat.GrantDeny
			for _, op := range a.Options {
				if op.Grant == chat.Grant(chat.GrantPick+win) {
					ans.Grant = op.Grant
				}
			}
		}
		if err := o.Answer(pid, c.ID, ans); err != nil {
			t.Fatal(err)
		}
	}
	r.took = time.Since(start)
	n := count.get()
	r.requests, r.failed, r.empty = n.requests, n.failed, n.empty
	ms, _, err := p.Chats.Messages(ctx, c.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{} // call id: tool
	for _, m := range ms {
		r.in += m.Usage.Input
		r.out += m.Usage.Output
		r.cached += m.Usage.CacheRead
		var text []string
		for _, pt := range m.Parts {
			switch {
			case pt.ToolCall != nil:
				names[pt.ToolCall.ID] = pt.ToolCall.Name
				r.calls = append(r.calls, strings.TrimPrefix(pt.ToolCall.Name, "desktop_"))
				r.trace = append(r.trace, "call "+pt.ToolCall.Name+" "+clipText(string(pt.ToolCall.Args), 400))
			case pt.ToolResult != nil:
				first, _, _ := strings.Cut(pt.ToolResult.Text, "\n")
				switch names[pt.ToolResult.CallID] {
				case "desktop_run_macro":
					r.results += pt.ToolResult.Text + "\n"
					if r.macro != "" {
						r.macro += " | "
					}
					r.macro += clipText(first, 160)
				case "desktop_save_macro", "desktop_save_recipe":
					r.saved = append(r.saved, clipText(first, 160))
				}
				r.trace = append(r.trace, "result "+clipText(pt.ToolResult.Text, 600))
			case pt.Notice != nil:
				r.trace = append(r.trace, fmt.Sprintf("notice %s: %s", pt.Notice.Kind, clipText(pt.Notice.Text, 300)))
				if strings.Contains(strings.ToLower(pt.Notice.Text), "quota") {
					r.quota = true
				}
			case pt.Text != nil && m.Role == chat.RoleAssistant:
				text = append(text, pt.Text.Text)
			}
		}
		if m.Role == chat.RoleAssistant && len(text) > 0 {
			// All the turn's text: the user reads it all, and a model may
			// give the result before the reminder and only "done" after.
			said := strings.Join(text, "\n")
			r.answer = strings.TrimSpace(r.answer + "\n" + said)
			r.trace = append(r.trace, "says "+clipText(said, 300))
		}
	}
	return r
}

// liveApp is one app of the live runs: its task, its six param sets, how
// to open its window (in another language or mode for the break) and how
// to grade a run.
type liveApp struct {
	app    string // the program, as macros are saved under it
	goal   string
	params []map[string]string
	breaks bool
	open   func(brk bool) (win, title string, close func())
	check  func(win string, p map[string]string, answer string) (bool, string)
}

func liveApps(t *testing.T, name, editor string, cli *desktop.CLI, site *standIn, root string) liveApp {
	t.Helper()
	switch name {
	case "editor":
		const title = "TASK038 Live Editor"
		// The editor has no language or mode to switch: no break.
		return liveApp{
			app:  "spike039",
			goal: `In the open text editor "` + title + `", write this sentence: {text}`,
			params: []map[string]string{
				{"text": "Meet me at the north gate at 7:30, and bring the blue folder."},
				{"text": "The train leaves at 9:15 from platform 4."},
				{"text": "Please water the plants on Tuesday and Friday."},
				{"text": "Our budget for March is 2,400 euros."},
			},
			open: func(bool) (string, string, func()) {
				ed := exec.Command(editor, "editor", "-title", title)
				if err := ed.Start(); err != nil {
					t.Fatal(err)
				}
				win := waitWindow(t, cli, ed.Process.Pid)
				return win, title, func() { _ = ed.Process.Kill(); _, _ = ed.Process.Wait() } // only the process this test started
			},
			check: func(win string, p map[string]string, _ string) (bool, string) {
				text := editorText(t, cli, win)
				n := strings.Count(text, p["text"])
				return n == 1, fmt.Sprintf("the sentence %d times in the editor", n)
			},
		}
	case "calculator":
		return liveApp{
			app:  "calculatorapp",
			goal: "Use Calculator to work out {a} + {b}, and tell me the result.",
			params: []map[string]string{
				{"a": "4827", "b": "9356"}, {"a": "315", "b": "2048"}, {"a": "7777", "b": "1234"},
				{"a": "66", "b": "905"}, {"a": "12345", "b": "678"}, {"a": "4096", "b": "8192"},
			},
			breaks: true,
			open: func(brk bool) (string, string, func()) {
				win := openCalculator(t, cli)
				if brk {
					time.Sleep(1500 * time.Millisecond)
					if err := calcMode(cli, win, "Date calculation"); err != nil {
						closeWindow(win)
						t.Fatalf("Calculator's mode: %v", err)
					}
				}
				return win, "Calculator", func() {
					// Calculator keeps its mode between starts: Standard back.
					if err := calcMode(cli, win, "Standard"); err != nil {
						t.Logf("Calculator back to Standard: %v", err)
					}
					closeWindow(win)
				}
			},
			check: func(win string, p map[string]string, answer string) (bool, string) {
				a, _ := strconv.Atoi(p["a"])
				b, _ := strconv.Atoi(p["b"])
				want := strconv.Itoa(a + b)
				display := ""
				if ms := liveFind(cli, win, "statictext", "Display is"); len(ms) > 0 {
					display = ms[0].Name
				}
				// Right only when the window shows the sum too: a model can
				// work it out itself (SPIKE-040).
				strip := strings.NewReplacer(",", "", ".", "", " ", "", "\u202f", "", "\u00a0", "")
				ok := strings.Contains(strip.Replace(answer), want) && strip.Replace(display) == "Displayis"+want
				return ok, "want " + want + ", display: " + display
			},
		}
	case "site":
		return liveApp{
			app:  "chrome",
			goal: `In the open Chrome window, go to ` + site.url + `, search for "{query}", and tell me the first five results (title and URL), in order. Reading them is enough; don't open them.`,
			params: []map[string]string{
				{"query": "Wails v3 Go desktop framework"}, {"query": "SQLite WAL mode"}, {"query": "Go generics tutorial"},
				{"query": "Tauri vs Electron"}, {"query": "Windows UI Automation API"}, {"query": "Rust async runtime comparison"},
			},
			breaks: true,
			open: func(brk bool) (string, string, func()) {
				site.lang.Store("en")
				if brk {
					site.lang.Store("de")
				}
				profile := filepath.Join(root, "chrome-"+time.Now().Format("150405.000"))
				return openChrome(t, cli, profile)
			},
			check: func(_ string, p map[string]string, answer string) (bool, string) {
				at, n := 0, 0
				for _, r := range standInResults(p["query"])[:5] {
					i := strings.Index(answer[at:], r)
					if i < 0 {
						break
					}
					at += i + len(r)
					n++
				}
				return n == 5, fmt.Sprintf("titles %d/5 in order", n)
			},
		}
	}
	t.Fatalf("unknown app %q", name)
	return liveApp{}
}

type liveMatch struct {
	Ref  string `json:"ref_id"`
	Role string `json:"role"`
	Name string `json:"name"`
}

func liveFind(cli *desktop.CLI, win, role, name string) []liveMatch {
	data, err := cli.Run(context.Background(), 20*time.Second, "find", "--window-id", win, "--role", role, "--name", name, "--limit", "5")
	if err != nil {
		return nil
	}
	var d struct {
		Matches []liveMatch `json:"matches"`
	}
	_ = json.Unmarshal(data, &d)
	return d.Matches
}

func liveWindows(t *testing.T, cli *desktop.CLI) []desktop.Window {
	t.Helper()
	data, err := cli.Run(context.Background(), 15*time.Second, "list-windows")
	if err != nil {
		t.Fatal(err)
	}
	var ws []desktop.Window
	_ = json.Unmarshal(data, &ws)
	return ws
}

// openCalculator opens a new Calculator window: the one not there before.
func openCalculator(t *testing.T, cli *desktop.CLI) string {
	t.Helper()
	seen := map[string]bool{}
	for _, w := range liveWindows(t, cli) {
		seen[w.ID] = true
	}
	cmd := exec.Command("calc.exe")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go cmd.Wait()
	for range 100 {
		for _, w := range liveWindows(t, cli) {
			if !seen[w.ID] && strings.Contains(strings.ToLower(w.App), "calculator") {
				return w.ID
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("Calculator's window didn't show")
	return ""
}

// calcMode switches Calculator's mode from its navigation: a click on a
// mode doesn't switch it, select does (SPIKE-040).
func calcMode(cli *desktop.CLI, win, mode string) error {
	nav := liveFind(cli, win, "button", "Open Navigation")
	if len(nav) == 0 {
		return fmt.Errorf("no navigation button")
	}
	ctx := context.Background()
	if _, err := cli.Run(ctx, 15*time.Second, "click", nav[0].Ref); err != nil {
		return err
	}
	time.Sleep(time.Second)
	opt := liveFind(cli, win, "option", mode+" Calculator")
	if len(opt) == 0 {
		return fmt.Errorf("no option %q", mode+" Calculator")
	}
	if _, err := cli.Run(ctx, 15*time.Second, "select", opt[0].Ref, mode+" Calculator"); err != nil {
		return err
	}
	time.Sleep(1500 * time.Millisecond)
	return nil
}

var (
	user32      = syscall.NewLazyDLL("user32.dll")
	postMessage = user32.NewProc("PostMessageW")
	isWindow    = user32.NewProc("IsWindow")
)

// closeWindow asks the window the test opened to close (WM_CLOSE).
func closeWindow(win string) {
	h, err := strconv.ParseUint(strings.TrimPrefix(win, "w-"), 10, 64)
	if err != nil {
		return
	}
	postMessage.Call(uintptr(h), 0x0010, 0, 0)
	for range 50 {
		if r, _, _ := isWindow.Call(uintptr(h)); r == 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// openChrome starts Chrome with its own profile folder, its UI in
// English, on about:blank; the close kills that process tree only.
func openChrome(t *testing.T, cli *desktop.CLI, profile string) (string, string, func()) {
	t.Helper()
	exe := cmpOr(os.Getenv("JENAB_CHROME"), `C:\Program Files\Google\Chrome\Application\chrome.exe`)
	cmd := exec.Command(exe, "--user-data-dir="+profile, "--no-first-run", "--no-default-browser-check",
		"--disable-search-engine-choice-screen", "--disable-sync", "--lang=en-US", "--new-window", "about:blank")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	go cmd.Wait()
	kill := func() { _ = exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run() }
	for range 150 {
		for _, w := range liveWindows(t, cli) {
			if w.PID == pid {
				time.Sleep(2 * time.Second)
				return w.ID, w.Title, func() { closeWindow(w.ID); time.Sleep(time.Second); kill() }
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	kill()
	t.Fatal("Chrome's window didn't show")
	return "", "", nil
}

// standIn is SPIKE-040's stand-in search site: a cookie dialog on the
// page until it is answered, a search box, results made from the query,
// in English or German.
type standIn struct {
	url      string
	lang     atomic.Value // "en", "de"
	accepted atomic.Int32
}

var standInWords = map[string]map[string]string{
	"en": {"search": "Search", "button": "Search", "results": "Results", "about": "About %d results",
		"dialog": "Before you continue to Findit", "reject": "Reject all", "accept": "Accept all", "cookies": "We use cookies to run this site."},
	"de": {"search": "Suche", "button": "Suchen", "results": "Ergebnisse", "about": "Ungefähr %d Ergebnisse",
		"dialog": "Bevor Sie zu Findit weitergehen", "reject": "Alle ablehnen", "accept": "Alle akzeptieren", "cookies": "Wir verwenden Cookies."},
}

func standInResults(q string) []string {
	words := []string{"guide", "tutorial", "reference", "notes", "benchmarks", "FAQ", "examples", "overview"}
	k := 0
	for _, c := range strings.ToLower(strings.TrimSpace(q)) {
		k = (k*31 + int(c)) % 1000003
	}
	var out []string
	for i := range 8 {
		out = append(out, q+" "+words[(k+i*3)%len(words)])
	}
	return out
}

func startStandIn(t *testing.T, port int) *standIn {
	t.Helper()
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	s := &standIn{url: "http://" + ln.Addr().String() + "/"}
	s.lang.Store("en")
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lang := s.lang.Load().(string)
		tx := standInWords[lang]
		if r.URL.Path == "/consent" {
			if r.URL.Query().Get("choice") == "accept" {
				s.accepted.Add(1)
			}
			http.SetCookie(w, &http.Cookie{Name: "c", Value: r.URL.Query().Get("choice"), Path: "/"})
			back := r.URL.Query().Get("back")
			if !strings.HasPrefix(back, "/") {
				back = "/"
			}
			http.Redirect(w, r, back, http.StatusSeeOther)
			return
		}
		if r.URL.Path != "/" && r.URL.Path != "/search" {
			http.NotFound(w, r)
			return
		}
		_, noCookie := r.Cookie("c") // an error: no answer yet
		q := r.URL.Query().Get("q")
		var b strings.Builder
		fmt.Fprintf(&b, `<!doctype html><html lang="%s"><head><meta charset="utf-8"><title>Findit</title></head><body>`, lang)
		if noCookie != nil {
			back := html.EscapeString(r.URL.RequestURI())
			fmt.Fprintf(&b, `<div role="dialog" aria-modal="true" aria-label="%s" style="position:fixed;inset:0;background:#fff;padding:2em">
<h1>%s</h1><p>%s</p>
<form action="/consent" method="get"><input type="hidden" name="back" value="%s"><input type="hidden" name="choice" value="reject"><button>%s</button></form>
<form action="/consent" method="get"><input type="hidden" name="back" value="%s"><input type="hidden" name="choice" value="accept"><button>%s</button></form></div>`,
				tx["dialog"], tx["dialog"], tx["cookies"], back, tx["reject"], back, tx["accept"])
		}
		fmt.Fprintf(&b, `<header><h1>Findit</h1><form action="/search" method="get" role="search">
<input type="search" name="q" aria-label="%s" value="%s"><button>%s</button></form></header>`, tx["search"], html.EscapeString(q), tx["button"])
		if r.URL.Path == "/search" && q != "" {
			fmt.Fprintf(&b, `<main><h2>%s</h2><p>%s</p><ol>`, tx["results"], fmt.Sprintf(tx["about"], 1000+len(q)*37))
			hosts := []string{"docs.example.org", "blog.example.net", "wiki.example.com", "dev.example.org", "forum.example.net", "learn.example.com", "news.example.org", "code.example.net"}
			for i, title := range standInResults(q) {
				h := hosts[(len(q)+i)%len(hosts)]
				fmt.Fprintf(&b, `<li><a href="https://%s/%d"><h3>%s</h3><cite>https://%s</cite></a></li>`, h, i, html.EscapeString(title), h)
			}
			b.WriteString(`</ol></main>`)
		}
		b.WriteString(`</body></html>`)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(b.String()))
	})}
	go srv.Serve(ln)
	t.Cleanup(func() { _ = srv.Close() })
	return s
}
