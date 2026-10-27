package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/amir-saatchi/jenab/internal/agent"
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

// event is one emitted event.
type event struct {
	name string
	data any
}

// sink records what a Publisher emits.
type sink struct {
	mu     sync.Mutex
	events []event
}

func (s *sink) emit(name string, data any) {
	s.mu.Lock()
	s.events = append(s.events, event{name, data})
	s.mu.Unlock()
}

func (s *sink) names() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ns []string
	for _, e := range s.events {
		ns = append(ns, e.name)
	}
	return ns
}

func (s *sink) notices() []project.NoticeKind {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ks []project.NoticeKind
	for _, e := range s.events {
		if n, ok := e.data.(project.Notice); ok {
			ks = append(ks, n.Kind)
		}
	}
	return ks
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

const testKey = "sk-test-0123456789abcdef"

// env is the app's packages on a temp folder, wired as in cmd/desktop,
// with a fake provider "p" and no Wails.
type env struct {
	t        *testing.T
	root     string
	paths    config.Paths
	fp       *fake.Provider
	sink     *sink
	reg      *store.Registry
	pm       *project.Manager
	orch     *agent.Orchestrator
	settings *Settings
	secrets  *secret.Store
	traces   *agent.Traces
	svc      Bound
	cancel   context.CancelFunc
	stopped  bool
}

func newEnv(t *testing.T, root string) *env {
	t.Helper()
	ctx := context.Background()
	e := &env{t: t, root: root, fp: fake.New(), sink: &sink{}}
	e.paths = config.Paths{Root: root, Settings: filepath.Join(root, "config.yaml"), Registry: filepath.Join(root, "registry.db"),
		Logs: filepath.Join(root, "logs")}.WithDataFolder(filepath.Join(root, "data"))
	s := config.Defaults()
	s.LLM.Models = map[string]string{"default": "p/m1", "fast": "p/m1"}
	s.LLM.Providers = map[string]config.ProviderSettings{
		"p": {Kind: string(provider.KindCompatible), BaseURL: "https://example.test/v1/", Models: []config.ModelSettings{{ID: "m1", Context: 32000}}},
	}
	if err := config.SaveSettings(e.paths.Settings, s); err != nil {
		t.Fatal(err)
	}
	s, problems, err := config.LoadSettings(e.paths.Settings)
	if err != nil {
		t.Fatal(err)
	}
	e.settings = NewSettings(e.paths.Settings, s, problems)
	if e.reg, err = store.OpenRegistry(ctx, e.paths.Registry); err != nil {
		t.Fatal(err)
	}
	secrets := secret.New(&memKeyring{m: map[string]string{provider.KeyName("p"): testKey}})
	e.secrets = secrets
	gate := limit.NewGate(s.LLM.MaxParallelCalls)
	models := provider.NewRegistry(provider.Deps{Settings: s.LLM, Secrets: secrets, Gate: gate,
		Backends: map[provider.Kind]provider.Factory{provider.KindCompatible: e.fp.Factory(), provider.KindGemini: e.fp.Factory()}})
	e.settings.OnChange(func(s config.Settings) { gate.SetSize(s.LLM.MaxParallelCalls); models.Apply(s.LLM) })
	pub := &Publisher{emit: e.sink.emit}
	e.pm = project.NewManager(project.Deps{Paths: e.paths, Registry: e.reg, Events: pub})
	tools := tool.NewRegistry()
	tools.Add(agent.Tools()...)
	appCtx, cancel := context.WithCancel(ctx)
	e.cancel = cancel
	e.traces = agent.NewTraces()
	e.orch = agent.New(agent.Deps{Context: appCtx, Projects: e.pm, Models: models, Tools: tools, Settings: e.settings.Get, Events: pub, Traces: e.traces,
		Card: func(ctx context.Context, p *project.Project) (string, error) { return p.DB.Card(ctx, time.Now()) }})
	e.svc = NewServices(Services{Orchestrator: e.orch, Projects: e.pm, Registry: e.reg, Models: models, Secrets: secrets,
		Settings: e.settings, Traces: e.traces, Logs: e.paths.Logs})
	t.Cleanup(e.stop)
	return e
}

// quitTimeout and quitWait are cmd/desktop's ShutdownTimeout and
// WaitTimeout.
const (
	quitTimeout = 10 * time.Second
	quitWait    = 5 * time.Second
)

// shutdown runs the app's quit order (Q30) as cmd/desktop's Shutdown does:
// refuse, cancel, wait, then close the projects and the registry.
func (e *env) shutdown() error {
	e.stopped = true
	e.orch.Refuse()
	e.cancel()
	ctx, cancel := context.WithTimeout(context.Background(), quitTimeout)
	defer cancel()
	wctx, wcancel := context.WithTimeout(ctx, quitWait)
	err := e.orch.Wait(wctx)
	wcancel()
	return errors.Join(err, e.pm.CloseAll(ctx), e.reg.Close())
}

func (e *env) stop() {
	if !e.stopped {
		if err := e.shutdown(); err != nil {
			e.t.Error(err)
		}
	}
}

// idle waits until the chat has no turn.
func (e *env) idle(c id.Chat) {
	e.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for e.orch.ChatStatus(c) != "" {
		if time.Now().After(deadline) {
			e.t.Fatalf("chat %s is still %q", c, e.orch.ChatStatus(c))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func uiErr(t *testing.T, err error, kind string) {
	t.Helper()
	var ue *UIError
	if !errors.As(err, &ue) || ue.Kind != kind {
		t.Fatalf("err = %#v; want a UIError of kind %s", err, kind)
	}
}

// The app quits within 10 s while a turn is running, and the next start
// needs no recovery (P1-13).
func TestShutdownDuringTurn(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	e := newEnv(t, root)
	op, err := e.svc.Project.Create(ctx, "Busy")
	if err != nil {
		t.Fatal(err)
	}
	e.fp.Push(fake.Reply{Events: []provider.Event{{Kind: provider.EventDelta, PartKind: chat.PartText, Text: "Thinking"}}, Hang: true})
	if _, err := e.svc.Chat.Send(ctx, op.ID, op.Mother, "Work hard"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for e.orch.Live(op.ID, op.Mother).Text == "" {
		if time.Now().After(deadline) {
			t.Fatal("the turn never streamed")
		}
		time.Sleep(5 * time.Millisecond)
	}
	start := time.Now()
	if err := e.shutdown(); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > quitTimeout {
		t.Fatalf("shutdown took %v", took)
	}
	dir := filepath.Join(e.paths.Projects, string(op.ID))
	if entries, _ := os.ReadDir(e.paths.Projects); len(entries) == 1 {
		dir = filepath.Join(e.paths.Projects, entries[0].Name())
	}
	if _, err := os.Stat(filepath.Join(dir, "jenab.lock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the lock file is still there: %v", err)
	}
	if _, err := e.svc.Chat.Send(ctx, op.ID, op.Mother, "More"); err == nil {
		t.Fatal("a message after shutdown")
	} else {
		uiErr(t, err, KindClosing)
	}

	// The next start opens the project without recovery, and keeps what
	// the stopped turn wrote.
	e2 := newEnv(t, root)
	snap, err := e2.svc.Chat.Snapshot(ctx, op.ID, op.Mother)
	if err != nil {
		t.Fatal(err)
	}
	if kinds := e2.sink.notices(); len(kinds) != 0 {
		t.Fatalf("notices after a clean quit: %v", kinds)
	}
	if len(snap.Messages) == 0 || snap.Messages[0].Parts[0].Text.Text != "Work hard" {
		t.Fatalf("messages after restart: %+v", snap.Messages)
	}
}

func TestWindowTheme(t *testing.T) {
	for _, c := range []struct {
		theme string
		dark  bool
		bg    application.RGBA
		frame application.Theme
	}{
		{"light", true, lightBackground, application.Light},
		{"dark", false, darkBackground, application.Dark},
		{"system", true, darkBackground, application.SystemDefault},
		{"system", false, lightBackground, application.SystemDefault},
	} {
		if bg, frame := windowTheme(c.theme, c.dark); bg != c.bg || frame != c.frame {
			t.Errorf("windowTheme(%s, %t) = %v, %v", c.theme, c.dark, bg, frame)
		}
	}
}
