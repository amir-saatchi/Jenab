package scenario

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// TestDepsAndCallHooks: Options.Deps changes the orchestrator's deps, and
// Options.Call makes the checks read a wrapped call as the tool it runs,
// while the transcript shows both (SPIKE-028, spike-only). Without them a
// run is as before.
func TestDepsAndCallHooks(t *testing.T) {
	s, err := Load(writeSet(t, map[string]string{
		"_set.yaml": `title: Prices
tools:
  - name: web_search
    description: Search the web.
    schema: {type: object, properties: {query: {type: string}}, required: [query]}
rules:
  web_search:
    - result: gold is 2641 USD
`,
		"wrapped.yaml": `id: wrapped
context: main
messages: [{text: What is gold at today}]
asserts:
  - {type: tool_called, test: x, tool: web_search, args_match: '(?i)gold'}
`,
	}))
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "_plays.yaml")
	if err := os.WriteFile(p, []byte(`scenarios:
  wrapped:
    - calls: [{name: wrap, args: {name: web_search, args: {query: gold}}}]
    - text: Gold is 2641 USD.
`), 0o644); err != nil {
		t.Fatal(err)
	}
	sc, err := ReadScript(p)
	if err != nil {
		t.Fatal(err)
	}
	type wrapArgs struct {
		Name string          `json:"name"`
		Args json.RawMessage `json:"args"`
	}
	unwrap := func(name, args string) (string, string) {
		var a wrapArgs
		if name != "wrap" || json.Unmarshal([]byte(args), &a) != nil {
			return name, args
		}
		return a.Name, string(a.Args)
	}
	run := func(hooks bool) Run {
		opt := Options{Models: ScriptedModels(sc), Script: sc, Scale: 0.01, Timeout: time.Minute, Temp: t.TempDir()}
		if hooks {
			opt.Deps = func(d *agent.Deps) {
				reg := d.Tools
				reg.Add(tool.Func(tool.Spec{Name: "wrap", Description: "Run a tool.", Schema: json.RawMessage(`{"type":"object"}`)},
					func(ctx context.Context, env *tool.Env, a wrapArgs) (tool.Result, error) {
						tl, _ := reg.Get(a.Name)
						return tool.Run(ctx, tl, tool.Call{Args: a.Args, Env: env})
					}))
			}
			opt.Call = unwrap
		}
		rep, err := RunSet(context.Background(), s, opt)
		if err != nil {
			t.Fatal(err)
		}
		return rep.Runs[0]
	}
	with := run(true)
	if with.Asserts[0].Status != Pass || !strings.Contains(with.Transcript, "> call wrap→web_search {") ||
		!strings.Contains(with.Transcript, "> result web_search:\n> gold is 2641 USD") {
		t.Errorf("with the hooks: %+v\n%s", with.Asserts, with.Transcript)
	}
	without := run(false)
	if without.Asserts[0].Status != Fail || !strings.Contains(without.Transcript, "> result wrap (error):\n> there is no tool named wrap") {
		t.Errorf("without the hooks: %+v\n%s", without.Asserts, without.Transcript)
	}
}

// TestSetupHook: Options.Setup can add earlier turns to the chat under
// test, and the scripted message then comes after them (SPIKE-028,
// spike-only). Without it the message is the chat's first turn.
func TestSetupHook(t *testing.T) {
	s, err := Load(writeSet(t, map[string]string{
		"_set.yaml": "title: Notes\n",
		"later.yaml": `id: later
context: main
messages: [{text: What was the code word}]
asserts:
  - {type: answer_contains, test: x, match: heron}
`,
	}))
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "_plays.yaml")
	if err := os.WriteFile(p, []byte("scenarios:\n  later:\n    - text: The code word was heron.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sc, err := ReadScript(p)
	if err != nil {
		t.Fatal(err)
	}
	text := func(s string) []chat.Part { return []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: s}}} }
	run := func(hook bool) Run {
		opt := Options{Models: ScriptedModels(sc), Script: sc, Scale: 0.01, Timeout: time.Minute, Temp: t.TempDir()}
		if hook {
			opt.Setup = func(ctx context.Context, got *Scenario, p *project.Project, c id.Chat) error {
				if got.ID != "later" {
					t.Errorf("Setup got scenario %q", got.ID)
				}
				for _, m := range []chat.Message{
					{Chat: c, Turn: 1, Role: chat.RoleUser, Parts: text("Remember the code word heron.")},
					{Chat: c, Turn: 1, Role: chat.RoleAssistant, Parts: text("Noted.")},
				} {
					if _, _, err := p.Chats.AppendMessage(ctx, m); err != nil {
						return err
					}
				}
				return nil
			}
		}
		rep, err := RunSet(context.Background(), s, opt)
		if err != nil {
			t.Fatal(err)
		}
		return rep.Runs[0]
	}
	with := run(true)
	if with.Error != "" || !strings.Contains(with.Transcript, "## Turn 1\n") ||
		!strings.Contains(with.Transcript, "Remember the code word heron.") || !strings.Contains(with.Transcript, "## Turn 2\n") {
		t.Errorf("with the hook: %s\n%s", with.Error, with.Transcript)
	}
	without := run(false)
	if strings.Contains(without.Transcript, "## Turn 2\n") || strings.Contains(without.Transcript, "Remember") {
		t.Errorf("without the hook:\n%s", without.Transcript)
	}
}
