//go:build windows

package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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
	"github.com/amir-saatchi/jenab/internal/secret"
	"github.com/amir-saatchi/jenab/internal/skill"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// TestDesktopLive runs desktop control end to end (SPEC 8.10, TASK-035):
// a real model, the real agent-desktop and a test editor the test opens
// and closes itself. It never acts on another window: the pick card is
// answered with the editor's option only.
//
//	JENAB_DESKTOP_LIVE    the .env file with the key
//	JENAB_AGENT_DESKTOP   agent-desktop.exe (the pinned one)
//	JENAB_TEST_EDITOR     SPIKE-039's harness, which has the editor
//	JENAB_DESKTOP_MODEL   kind/base-url/model/key-variable, default
//	                      gemini//gemini-3.5-flash-lite/GEMINI_API_KEY
//	JENAB_DESKTOP_REPS    runs, default 3
//	JENAB_DESKTOP_STOP    "1": one more run, stopped after the pick
//	JENAB_DESKTOP_APP     the task names this program ("Notepad") instead
//	                      of the window's title; run a copy of the editor
//	                      by that name, so its process name matches
func TestDesktopLive(t *testing.T) {
	envFile, prog, editor := os.Getenv("JENAB_DESKTOP_LIVE"), os.Getenv("JENAB_AGENT_DESKTOP"), os.Getenv("JENAB_TEST_EDITOR")
	if envFile == "" || prog == "" || editor == "" {
		t.Skip("set JENAB_DESKTOP_LIVE, JENAB_AGENT_DESKTOP and JENAB_TEST_EDITOR to run it")
	}
	spec := strings.Split(cmpOr(os.Getenv("JENAB_DESKTOP_MODEL"), "gemini//gemini-3.5-flash-lite/GEMINI_API_KEY"), "/")
	if len(spec) < 4 {
		t.Fatal("JENAB_DESKTOP_MODEL: kind/base-url/model/key-variable")
	}
	kind, model, keyVar := spec[0], spec[len(spec)-2], spec[len(spec)-1]
	base := strings.Join(spec[1:len(spec)-2], "/")
	key := readEnv(t, envFile, keyVar)
	reps, _ := strconv.Atoi(cmpOr(os.Getenv("JENAB_DESKTOP_REPS"), "3"))

	root := t.TempDir()
	set := config.Defaults()
	set.Approvals.DefaultLevel = "standard"
	set.LLM.Models = map[string]string{"default": "live/" + model}
	set.LLM.Providers = map[string]config.ProviderSettings{"live": {Kind: kind, BaseURL: base, Models: []config.ModelSettings{{ID: model}}}}
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
	pid, err := pm.Create(ctx, "Desktop")
	if err != nil {
		t.Fatal(err)
	}
	var requests int
	var reqMu sync.Mutex
	bk := backends.All()
	inner := bk[provider.Kind(kind)]
	bk[provider.Kind(kind)] = func(c provider.Connection) (provider.Provider, error) {
		p, err := inner(c)
		return countingProvider{p, func() { reqMu.Lock(); requests++; reqMu.Unlock() }}, err
	}
	models := provider.NewRegistry(provider.Deps{Settings: set.LLM, Gate: limit.NewGate(2), Backends: bk, Catalog: provider.MustCatalog(),
		Secrets: secret.New(&memKeyring{m: map[string]string{provider.KeyName("live"): key}})})
	cli := &desktop.CLI{Path: prog, Home: filepath.Join(root, "agent-desktop")}
	dm := desktop.NewManager(desktop.Deps{Runner: cli, On: func() bool { return true }, Self: os.Getpid(), Class: desktop.WindowClass, Log: log})
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
	defer func() { // the Q30 order, so the temp folder can go
		o.Refuse()
		cancel()
		wctx, done := context.WithTimeout(ctx, 30*time.Second)
		defer done()
		_ = o.Wait(wctx)
		_ = pm.CloseAll(wctx)
		_ = reg.Close()
	}()

	const sentence = "Meet me at the north gate at 7:30, and bring the blue folder."
	run := func(n int, stop bool) {
		title := fmt.Sprintf("TASK035 Live Editor %d", n)
		ed := exec.Command(editor, "editor", "-title", title)
		if err := ed.Start(); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = ed.Process.Kill(); _, _ = ed.Process.Wait() }() // only the process this test started
		win := waitWindow(t, cli, ed.Process.Pid)

		var c chat.Chat
		p, err := pm.Open(ctx, pid)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Release()
		if c, err = p.Chats.CreateChat(ctx, id.SourceUser, chat.Chat{Title: title}); err != nil {
			t.Fatal(err)
		}
		reqMu.Lock()
		requests = 0
		reqMu.Unlock()
		start := time.Now()
		task := fmt.Sprintf("In the open text editor %q, write this sentence: %s", title, sentence)
		if app := os.Getenv("JENAB_DESKTOP_APP"); app != "" {
			task = fmt.Sprintf("In %s, write this sentence: %s", app, sentence)
		}
		if _, err := o.Send(ctx, pid, c.ID, UserMessage{Text: task}); err != nil {
			t.Fatal(err)
		}
		var kinds []string
		deadline := time.Now().Add(5 * time.Minute)
		for o.State(pid, c.ID) != chat.StateIdle && time.Now().Before(deadline) {
			l := o.Live(pid, c.ID)
			if l.Waiting == nil {
				time.Sleep(200 * time.Millisecond)
				continue
			}
			a := approvalAt(t, p, c.ID, l.Waiting.Message, l.Waiting.Index)
			kinds = append(kinds, a.Kind)
			ans := Answer{Message: l.Waiting.Message, Index: l.Waiting.Index, Grant: chat.GrantOnce}
			if a.Kind == desktop.KindWindow {
				ans.Grant = chat.GrantDeny
				for _, op := range a.Options {
					if strings.HasPrefix(op.Label, title+" — ") && op.Grant == chat.Grant(chat.GrantPick+win) {
						ans.Grant = op.Grant
					}
					for _, bad := range []string{"powershell", "windowsterminal", "cmd", "jenab", "agent.test"} {
						if strings.HasSuffix(op.Label, " — "+bad) {
							t.Errorf("run %d: the card offers %s", n, bad)
						}
					}
				}
				// The Ask tells the card: one window of the app the model
				// named, several, none, or no app (the full list).
				group := ""
				for _, op := range a.Options {
					if op.Grant == ans.Grant {
						group = op.Group
					}
				}
				t.Logf("run %d: pick card with %d windows, ask %q, picked group %q, why %q", n, len(a.Options)-1, a.Ask, group, a.Why)
			}
			if err := o.Answer(pid, c.ID, ans); err != nil {
				t.Fatal(err)
			}
			if stop && a.Kind == desktop.KindWindow {
				time.Sleep(time.Second)
				o.Stop(pid, c.ID)
			}
		}
		took := time.Since(start)
		ms, _, err := p.Chats.Messages(ctx, c.ID, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		var in, out, cached, calls int
		var steps []string
		for _, m := range ms {
			in += m.Usage.Input
			out += m.Usage.Output
			cached += m.Usage.CacheRead
			for _, pt := range m.Parts {
				if pt.ToolCall != nil {
					calls++
					steps = append(steps, pt.ToolCall.Name)
				}
			}
		}
		text := editorText(t, cli, win)
		ok := strings.Contains(text, sentence)
		reqMu.Lock()
		r := requests
		reqMu.Unlock()
		t.Logf("run %d: ok %v, %d requests, %d tool calls %v, cards %v, tokens in %d (+%d cached) out %d, %s",
			n, ok, r, calls, steps, kinds, in, cached, out, took.Round(100*time.Millisecond))
		if stop {
			if !logs.has("desktop: session ended", "the turn ended") {
				t.Errorf("Stop: the session didn't end with the turn:\n%s", logs)
			}
			return
		}
		if !ok {
			t.Errorf("run %d: the editor has %q", n, clipText(text, 200))
			for _, m := range ms { // the steps, for the report: only the test editor's text
				for _, pt := range m.Parts {
					switch {
					case pt.ToolCall != nil:
						t.Logf("  call %s %s", pt.ToolCall.Name, clipText(string(pt.ToolCall.Args), 200))
					case pt.ToolResult != nil:
						t.Logf("  result %s", clipText(pt.ToolResult.Text, 400))
					case pt.Text != nil && m.Role == chat.RoleAssistant:
						t.Logf("  says %s", clipText(pt.Text.Text, 300))
					}
				}
			}
		}
	}
	for n := 1; n <= reps; n++ {
		run(n, false)
	}
	if os.Getenv("JENAB_DESKTOP_STOP") == "1" {
		run(reps+1, true)
	}
	t.Logf("log lines with text lengths only:\n%s", logs.grep("desktop:"))
}

type countingProvider struct {
	provider.Provider
	count func()
}

func (c countingProvider) Stream(ctx context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	c.count()
	return c.Provider.Stream(ctx, req)
}

func approvalAt(t *testing.T, p *project.Project, c id.Chat, m id.Message, i int) chat.Approval {
	t.Helper()
	ms, _, err := p.Chats.Messages(context.Background(), c, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, msg := range ms {
		if msg.ID == m && i < len(msg.Parts) && msg.Parts[i].Approval != nil {
			return *msg.Parts[i].Approval
		}
	}
	t.Fatalf("no card at %s:%d", m, i)
	return chat.Approval{}
}

// waitWindow is the window of process pid, once agent-desktop lists it.
func waitWindow(t *testing.T, cli *desktop.CLI, pid int) string {
	t.Helper()
	for range 50 {
		data, err := cli.Run(context.Background(), 15*time.Second, "list-windows")
		if err != nil {
			t.Fatal(err)
		}
		var ws []desktop.Window
		_ = json.Unmarshal(data, &ws)
		for _, w := range ws {
			if w.PID == pid {
				return w.ID
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("the editor's window didn't show")
	return ""
}

// editorText is all the text of the editor's window.
func editorText(t *testing.T, cli *desktop.CLI, win string) string {
	t.Helper()
	data, err := cli.Run(context.Background(), 15*time.Second, "snapshot", "--window-id", win)
	if err != nil {
		return "(" + err.Error() + ")"
	}
	return string(data)
}

func readEnv(t *testing.T, path, name string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if ok && strings.TrimSpace(k) == name {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	t.Fatalf("%s isn't in the env file", name)
	return ""
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func clipText(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// lines collects log output.
type lines struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *lines) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lines) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func (l *lines) has(all ...string) bool {
	for _, line := range strings.Split(l.String(), "\n") {
		ok := true
		for _, s := range all {
			ok = ok && strings.Contains(line, s)
		}
		if ok {
			return true
		}
	}
	return false
}

func (l *lines) grep(s string) string {
	var out []string
	for _, line := range strings.Split(l.String(), "\n") {
		if strings.Contains(line, s) {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}
