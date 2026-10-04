package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
	"github.com/amir-saatchi/jenab/internal/secret"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// recorder is a Publisher that keeps what it gets. With panics set, it
// panics after each status.
type recorder struct {
	mu       sync.Mutex
	deltas   []chat.Delta
	parts    []chat.PartDone
	statuses []chat.Status
	panics   bool
}

func (r *recorder) Delta(d chat.Delta) {
	r.mu.Lock()
	r.deltas = append(r.deltas, d)
	r.mu.Unlock()
}

func (r *recorder) Part(p chat.PartDone) {
	r.mu.Lock()
	r.parts = append(r.parts, p)
	r.mu.Unlock()
}

func (r *recorder) Status(s chat.Status) {
	r.mu.Lock()
	r.statuses = append(r.statuses, s)
	panics := r.panics
	r.mu.Unlock()
	if panics {
		panic("the frontend is gone")
	}
}

func (r *recorder) allStatuses() []chat.Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.statuses)
}

func (r *recorder) allDeltas() []chat.Delta {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.deltas)
}

// retries are the statuses that show a wait.
func (r *recorder) retries() []chat.Retry {
	var out []chat.Retry
	for _, s := range r.allStatuses() {
		if s.Retry != nil {
			out = append(out, *s.Retry)
		}
	}
	return out
}

type memKeyring struct {
	mu sync.Mutex
	m  map[string]string
}

func (k *memKeyring) Get(name string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	v, ok := k.m[name]
	if !ok {
		return "", secret.ErrNotFound
	}
	return v, nil
}

func (k *memKeyring) Set(name, v string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.m[name] = v
	return nil
}

func (k *memKeyring) Delete(name string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.m, name)
	return nil
}

// harness is an Orchestrator on a real project with a fake provider "p".
type harness struct {
	t      *testing.T
	root   string
	set    config.Settings
	fp     *fake.Provider
	gate   *limit.Gate
	models *provider.Registry
	tools  *tool.Registry
	reg    *store.Registry
	pm     *project.Manager
	pub    *recorder
	o      *Orchestrator
	cancel context.CancelFunc
	pid    id.Project
}

func testSettings() config.Settings {
	s := config.Defaults()
	s.LLM.MaxParallelCalls = 2
	s.LLM.ProviderMaxParallelCalls = map[string]int{"p": 1}
	s.LLM.Models = map[string]string{"default": "p/m1", "fast": "p/small"}
	s.LLM.Providers = map[string]config.ProviderSettings{
		"p": {Kind: string(provider.KindCompatible), BaseURL: "https://example.test/v1/", Models: []config.ModelSettings{{ID: "m1", Context: 32000}, {ID: "small", Context: 8000}}},
	}
	return s
}

// newHarness starts on root; pid is the project to use, or "" for a new
// one. tools are added to the test registry.
func newHarness(t *testing.T, root string, pid id.Project, set config.Settings, tools ...tool.Tool) *harness {
	t.Helper()
	ctx := context.Background()
	h := &harness{t: t, root: root, set: set, fp: fake.New(), gate: limit.NewGate(set.LLM.MaxParallelCalls), pub: &recorder{}, pid: pid}
	paths := config.Paths{Root: root, Registry: filepath.Join(root, "registry.db")}.WithDataFolder(filepath.Join(root, "data"))
	var err error
	if h.reg, err = store.OpenRegistry(ctx, paths.Registry); err != nil {
		t.Fatal(err)
	}
	h.pm = project.NewManager(project.Deps{Paths: paths, Registry: h.reg})
	if h.pid == "" {
		if h.pid, err = h.pm.Create(ctx, "Test"); err != nil {
			t.Fatal(err)
		}
	}
	h.models = provider.NewRegistry(provider.Deps{
		Settings: set.LLM,
		Secrets:  secret.New(&memKeyring{m: map[string]string{provider.KeyName("p"): "sk-test-0123456789abcdef"}}),
		Gate:     h.gate,
		Backends: map[provider.Kind]provider.Factory{provider.KindCompatible: h.fp.Factory()},
	})
	h.tools = tool.NewRegistry()
	h.tools.Add(tools...)
	appCtx, cancel := context.WithCancel(ctx)
	h.cancel = cancel
	h.o = New(Deps{Context: appCtx, Projects: h.pm, Models: h.models, Tools: h.tools,
		Settings: func() config.Settings { return h.set }, Events: h.pub})
	return h
}

// stop shuts down in the Q30 order.
func (h *harness) stop() {
	h.t.Helper()
	h.o.Refuse()
	h.cancel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := h.o.Wait(ctx); err != nil {
		h.t.Errorf("Wait: %v", err)
	}
	if err := h.pm.CloseAll(ctx); err != nil {
		h.t.Error(err)
	}
	if err := h.reg.Close(); err != nil {
		h.t.Error(err)
	}
}

// open runs fn with the project open.
func (h *harness) open(fn func(p *project.Project)) {
	h.t.Helper()
	p, err := h.pm.Open(context.Background(), h.pid)
	if err != nil {
		h.t.Fatal(err)
	}
	defer p.Release()
	fn(p)
}

// newChat makes a chat; a title is fixed, so no title request is made.
func (h *harness) newChat(c chat.Chat) chat.Chat {
	h.t.Helper()
	h.open(func(p *project.Project) {
		var err error
		if c, err = p.Chats.CreateChat(context.Background(), id.SourceUser, c); err != nil {
			h.t.Fatal(err)
		}
	})
	return c
}

func (h *harness) mother() chat.Chat {
	h.t.Helper()
	var m chat.Chat
	h.open(func(p *project.Project) {
		cs, err := p.Chats.Chats(context.Background())
		if err != nil {
			h.t.Fatal(err)
		}
		m = cs[0]
	})
	return m
}

func (h *harness) send(c id.Chat, text string) id.Message {
	h.t.Helper()
	m, err := h.o.Send(context.Background(), h.pid, c, UserMessage{Text: text})
	if err != nil {
		h.t.Fatal(err)
	}
	return m
}

// wait waits until every turn has ended. In a synctest bubble, whose
// clock starts in 2000, time is fake and a hang is caught by -timeout.
func (h *harness) wait() {
	h.t.Helper()
	patience := 30 * time.Second
	if time.Now().Year() == 2000 {
		patience = time.Hour
	}
	ctx, cancel := context.WithTimeout(context.Background(), patience)
	defer cancel()
	if err := h.o.Wait(ctx); err != nil {
		h.t.Fatalf("turns still running: %v", err)
	}
}

// turn sends text and waits for the turn to end.
func (h *harness) turn(c id.Chat, text string) {
	h.t.Helper()
	h.send(c, text)
	h.wait()
}

func (h *harness) messages(c id.Chat) []chat.Message {
	h.t.Helper()
	var ms []chat.Message
	h.open(func(p *project.Project) {
		var err error
		if ms, _, err = p.Chats.Messages(context.Background(), c, 1, 0); err != nil {
			h.t.Fatal(err)
		}
	})
	return ms
}

// validHistory checks the rules both protocol shapes share (SPEC 3.8):
// every tool call is answered by a result in the tool message right after
// its assistant message, before any other message, and every result
// answers a call. OpenAI wants the tool messages right after the call;
// Anthropic wants the results in the next user message, which a tool
// message becomes. A tool message has results, and no text is blank.
func validHistory(t *testing.T, ms []chat.Message) {
	t.Helper()
	var open []string
	for i, m := range ms {
		for _, p := range m.Parts {
			if p.Text != nil && strings.TrimSpace(p.Text.Text) == "" {
				t.Errorf("message %d: blank text", i)
			}
		}
		switch m.Role {
		case chat.RoleTool:
			if !slices.ContainsFunc(m.Parts, func(p chat.Part) bool { return p.ToolResult != nil }) {
				t.Errorf("message %d: a tool message without results", i)
			}
			for _, p := range m.Parts {
				if p.ToolResult == nil {
					continue
				}
				if !slices.Contains(open, p.ToolResult.CallID) {
					t.Errorf("message %d: result for %s, which is not an open call", i, p.ToolResult.CallID)
				}
				open = without(open, p.ToolResult.CallID)
			}
			continue
		}
		if len(open) > 0 {
			t.Errorf("message %d (%s) comes before the results of %v", i, m.Role, open)
			open = nil
		}
		if m.Role == chat.RoleAssistant {
			if len(m.Parts) == 0 {
				t.Errorf("message %d: an assistant message without parts", i)
			}
			for _, p := range m.Parts {
				if p.ToolCall != nil {
					open = append(open, p.ToolCall.ID)
				}
			}
		}
	}
	if len(open) > 0 {
		t.Errorf("calls without results at the end: %v", open)
	}
}

// texts lists the text of every part, for messages compared in tests.
func texts(ms []chat.Message) []string {
	var out []string
	for _, m := range ms {
		for _, p := range m.Parts {
			switch {
			case p.Text != nil:
				out = append(out, string(m.Role)+": "+p.Text.Text)
			case p.Thinking != nil:
				out = append(out, string(m.Role)+": thinking "+p.Thinking.Text)
			case p.Image != nil:
				out = append(out, string(m.Role)+": image "+p.Image.Ref)
			case p.ToolCall != nil:
				out = append(out, string(m.Role)+": call "+p.ToolCall.Name)
			case p.ToolResult != nil:
				out = append(out, string(m.Role)+": result "+p.ToolResult.Text)
			case p.Notice != nil:
				out = append(out, string(m.Role)+": "+p.Notice.Text)
			}
		}
	}
	return out
}

func turnsOf(ms []chat.Message) []int {
	var out []int
	for _, m := range ms {
		if len(out) == 0 || out[len(out)-1] != m.Turn {
			out = append(out, m.Turn)
		}
	}
	return out
}

func jsonOf(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// echo returns its text argument.
func echo() tool.Tool {
	return tool.Func(tool.Spec{Name: "echo", Description: "Echo text.",
		Schema: json.RawMessage(`{"type": "object", "properties": {"text": {"type": "string"}}}`)},
		func(_ context.Context, _ *tool.Env, a struct{ Text string }) (tool.Result, error) {
			return tool.Result{Text: a.Text}, nil
		})
}

// blocker blocks until release is closed or ctx ends; started gets a
// value when it runs.
func blocker(started chan<- string, release <-chan struct{}) tool.Tool {
	return tool.Func(tool.Spec{Name: "block", Description: "Wait.",
		Schema: json.RawMessage(`{"type": "object"}`)},
		func(ctx context.Context, env *tool.Env, _ struct{}) (tool.Result, error) {
			select {
			case started <- string(env.Message):
			case <-ctx.Done():
				return tool.Result{}, ctx.Err()
			}
			select {
			case <-release:
				return tool.Result{Text: "released"}, nil
			case <-ctx.Done():
				return tool.Result{}, ctx.Err()
			}
		})
}

// big returns n bytes of text, so it is stored with a preview.
func big() tool.Tool {
	return tool.Func(tool.Spec{Name: "big", Description: "A long result.",
		Schema: json.RawMessage(`{"type": "object", "properties": {"n": {"type": "integer"}}}`)},
		func(_ context.Context, _ *tool.Env, a struct{ N int }) (tool.Result, error) {
			return tool.Result{Text: strings.Repeat("row of data\n", a.N/12+1)}, nil
		})
}

// read makes a reply report n input tokens, so the Registry's truncation
// check passes for a large request.
func read(r fake.Reply, n int) fake.Reply {
	r.Events = slices.Clone(r.Events)
	for i, ev := range r.Events {
		if ev.Kind == provider.EventDone {
			var u chat.Usage
			if ev.Usage != nil {
				u = *ev.Usage
			}
			u.Input = n
			r.Events[i].Usage = &u
		}
	}
	return r
}
