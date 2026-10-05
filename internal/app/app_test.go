package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
	e.orch = agent.New(agent.Deps{Context: appCtx, Projects: e.pm, Models: models, Tools: tools, Settings: e.settings.Get, Events: pub, Traces: e.traces})
	e.svc = NewServices(Services{Orchestrator: e.orch, Projects: e.pm, Registry: e.reg, Models: models, Secrets: secrets,
		Settings: e.settings, Traces: e.traces, Logs: e.paths.Logs})
	t.Cleanup(e.stop)
	return e
}

// shutdown runs the app's quit order.
func (e *env) shutdown() error {
	e.stopped = true
	err := Shutdown{Refuse: []func(){e.orch.Refuse}, Cancel: e.cancel, Wait: []func(context.Context) error{e.orch.Wait}, Close: e.pm.CloseAll}.Run()
	if cerr := e.reg.Close(); cerr != nil {
		err = errors.Join(err, cerr)
	}
	return err
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

func TestServices(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	ps, cs := e.svc.Project, e.svc.Chat

	if _, err := ps.Create(ctx, "  "); err == nil {
		t.Fatal("a project without a name")
	} else {
		uiErr(t, err, KindInvalid)
	}
	op, err := ps.Create(ctx, "Coins")
	if err != nil {
		t.Fatal(err)
	}
	if op.Name != "Coins" || op.Mother == "" || op.Level != "standard" || op.Damage != nil {
		t.Fatalf("created %+v", op)
	}
	list, err := ps.List(ctx)
	if err != nil || len(list) != 1 || list[0].ID != op.ID || list[0].LastOpened == nil {
		t.Fatalf("List = %+v, %v", list, err)
	}
	if again, err := ps.Open(ctx, op.ID); err != nil || again != op {
		t.Fatalf("Open = %+v, %v", again, err)
	}
	if _, err := ps.Open(ctx, id.Project(id.New())); err == nil {
		t.Fatal("opened a project that doesn't exist")
	} else {
		uiErr(t, err, KindNotFound)
	}

	// A message to the Mother chat runs a turn; the snapshot has it.
	e.fp.Push(fake.Text("Hello."))
	if _, err := cs.Send(ctx, op.ID, op.Mother, "  "); err == nil {
		t.Fatal("sent an empty message")
	} else {
		uiErr(t, err, KindInvalid)
	}
	if _, err := cs.Send(ctx, op.ID, op.Mother, "Hi"); err != nil {
		t.Fatal(err)
	}
	e.idle(op.Mother)
	snap, err := cs.Snapshot(ctx, op.ID, op.Mother)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Chat.Kind != chat.KindMother || snap.From != 1 || len(snap.Messages) != 2 || snap.Seq == 0 || snap.Live.Running {
		t.Fatalf("snapshot %+v", snap)
	}
	if got := snap.Messages[1].Parts[0].Text.Text; got != "Hello." {
		t.Errorf("answer %q", got)
	}
	if !slices.Contains(e.sink.names(), EventPart) || !slices.Contains(e.sink.names(), EventStatus) {
		t.Errorf("events %v", e.sink.names())
	}
	if ms, err := cs.Messages(ctx, op.ID, op.Mother, 1, 1); err != nil || len(ms) != 2 {
		t.Errorf("Messages = %d, %v", len(ms), err)
	}
	if ms, err := cs.Messages(ctx, op.ID, op.Mother, 0, 0); err != nil || len(ms) != 2 {
		t.Errorf("Messages from 0 = %d, %v", len(ms), err)
	}

	// A chat of the user's, renamed, with a role and a model.
	c, err := cs.Create(ctx, op.ID, "", "You track prices.")
	if err != nil {
		t.Fatal(err)
	}
	if title, err := cs.Rename(ctx, op.ID, c.ID, "  Prices "); err != nil || title != "Prices" {
		t.Fatalf("Rename = %q, %v", title, err)
	}
	if err := cs.SetRole(ctx, op.ID, op.Mother, "x"); err == nil {
		t.Fatal("Mother got a role")
	} else {
		uiErr(t, err, KindInvalid)
	}
	if err := cs.SetModel(ctx, op.ID, c.ID, "fast"); err != nil {
		t.Fatal(err)
	}
	items, err := cs.List(ctx, op.ID)
	if err != nil || len(items) != 2 || items[0].Chat.Kind != chat.KindMother || items[0].LastActivity == nil ||
		items[1].Chat.Title != "Prices" || !items[1].Chat.TitleFixed || items[1].Chat.Model != "fast" || items[1].LastActivity != nil {
		t.Fatalf("List = %+v, %v", items, err)
	}
	if err := cs.Archive(ctx, op.ID, c.ID, true); err != nil {
		t.Fatal(err)
	}

	// Errors the frontend can act on.
	if err := cs.SetLevel(ctx, op.ID, c.ID, "loose"); err == nil {
		t.Fatal("a bad level")
	} else {
		uiErr(t, err, KindInvalid)
	}
	if err := cs.SetLevel(ctx, op.ID, c.ID, "strict"); err != nil {
		t.Fatal(err)
	}
	if op2, _ := ps.Open(ctx, op.ID); op2.Level != "strict" {
		t.Errorf("level %s", op2.Level)
	}
	if err := cs.Answer(ctx, op.ID, c.ID, agent.Answer{Message: "m", Grant: chat.GrantOnce}); err == nil {
		t.Fatal("an answer with nothing waiting")
	} else {
		uiErr(t, err, KindBusy)
	}
	if err := cs.Retry(ctx, op.ID, c.ID); err == nil {
		t.Fatal("a retry with nothing to retry")
	} else {
		uiErr(t, err, KindBusy)
	}
	if _, err := cs.Snapshot(ctx, op.ID, id.Chat(id.New())); err == nil {
		t.Fatal("a snapshot of no chat")
	} else {
		uiErr(t, err, KindNotFound)
	}
	if err := cs.Stop(ctx, op.ID, c.ID); err != nil {
		t.Fatal(err)
	}

	r, err := cs.Search(ctx, op.ID, SearchRequest{Query: "hello"})
	if err != nil || len(r.Hits) != 1 || r.Hits[0].Chat != op.Mother {
		t.Fatalf("Search = %+v, %v", r, err)
	}
	if err := cs.Clear(ctx, op.ID, op.Mother); err != nil {
		t.Fatal(err)
	}
	if snap, err := cs.Snapshot(ctx, op.ID, op.Mother); err != nil || len(snap.Messages) != 0 || snap.Messages == nil {
		t.Fatalf("after Clear: %+v, %v", snap, err)
	}
	if err := cs.Delete(ctx, op.ID, c.ID); err != nil {
		t.Fatal(err)
	}

	if a, err := ps.Activity(ctx, op.ID); err != nil || a.Project != op.ID || !a.Open {
		t.Errorf("Activity = %+v, %v", a, err)
	}
	if _, err := ps.FolderWarning(ctx); err != nil {
		t.Error(err)
	}
}

func TestSettingsService(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	ss := e.svc.Settings
	v, err := ss.Get(ctx)
	if err != nil || v.Problems == nil || len(v.Problems) != 0 || v.Settings.LLM.Models["default"] != "p/m1" {
		t.Fatalf("Get = %+v, %v", v, err)
	}
	next := v.Settings
	next.UI.Theme = "dark"
	next.LLM.MaxParallelCalls = 3
	next.Context.HistoryMinTurns = -5 // out of range: the default is used
	v, err = ss.Save(ctx, next)
	if err != nil {
		t.Fatal(err)
	}
	if v.Settings.UI.Theme != "dark" || v.Settings.LLM.MaxParallelCalls != 3 || len(v.Problems) != 1 ||
		v.Problems[0].Path != "context.history_min_turns" || v.Settings.Context.HistoryMinTurns != config.Defaults().Context.HistoryMinTurns {
		t.Fatalf("Save = %+v", v)
	}
	if e.settings.Get().UI.Theme != "dark" {
		t.Error("Get after Save has the old settings")
	}
	src, err := os.ReadFile(e.paths.Settings)
	if err != nil || !strings.Contains(string(src), "theme: dark") {
		t.Errorf("config.yaml: %v\n%s", err, src)
	}
	ps, err := ss.Providers(ctx)
	if err != nil || len(ps) != 1 || ps[0].Name != "p" || ps[0].Limit != 3 {
		t.Fatalf("Providers = %+v, %v", ps, err)
	}
	// The JSON names are config.yaml's.
	b, _ := json.Marshal(v.Settings)
	if !strings.Contains(string(b), `"max_parallel_calls":3`) || !strings.Contains(string(b), `"theme":"dark"`) {
		t.Errorf("JSON %s", b)
	}
}

func TestBucket(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	op, err := e.svc.Project.Create(ctx, "Files")
	if err != nil {
		t.Fatal(err)
	}
	proj, err := e.pm.Open(ctx, op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := proj.DB.PutObject(ctx, limit.Interactive, id.SourceUser, "notes/a.txt", strings.NewReader("one"), store.PutOptions{Source: "upload"}); err != nil {
		t.Fatal(err)
	}
	if _, err := proj.DB.PutObject(ctx, limit.Interactive, id.SourceUser, "notes/a.txt", strings.NewReader("two"), store.PutOptions{Source: "upload"}); err != nil {
		t.Fatal(err)
	}
	proj.Release()

	pg, err := e.svc.Bucket.List(ctx, op.ID, "", "")
	if err != nil || len(pg.Objects) != 0 || !slices.Equal(pg.Folders, []string{"notes/"}) {
		t.Fatalf("List = %+v, %v", pg, err)
	}
	pg, err = e.svc.Bucket.List(ctx, op.ID, "notes/", "")
	if err != nil || len(pg.Objects) != 1 || pg.Objects[0].Key != "notes/a.txt" || pg.Objects[0].Version != 2 || pg.Objects[0].Expires != nil {
		t.Fatalf("List notes/ = %+v, %v", pg, err)
	}
	if vs, err := e.svc.Bucket.Versions(ctx, op.ID, "notes/a.txt"); err != nil || len(vs) != 2 {
		t.Fatalf("Versions = %+v, %v", vs, err)
	}
	if _, err := e.svc.Bucket.List(ctx, op.ID, "/bad", ""); err == nil {
		t.Fatal("a bad prefix")
	} else {
		uiErr(t, err, KindInvalid)
	}

	// The objects route, as Wails mounts it: the route cut off the path.
	h := objectHandler(e.pm)
	for _, path := range []string{"/" + string(op.ID) + "/notes/a.txt", objectPath(op.ID, "notes/a.txt")} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		body, _ := io.ReadAll(rec.Body)
		if rec.Code != http.StatusOK || string(body) != "two" || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: %d %q", path, rec.Code, body)
		}
	}
	for _, path := range []string{"/" + string(op.ID) + "/notes/none.txt", "/" + id.New() + "/notes/a.txt"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: %d", path, rec.Code)
		}
	}
	if e.pm.Busy() {
		t.Error("the objects route kept a lease")
	}
}

func objectPath(p id.Project, key string) string { return "/objects/" + string(p) + "/" + key }

func TestDevLog(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	if err := os.MkdirAll(e.paths.Logs, 0o755); err != nil {
		t.Fatal(err)
	}
	line := "time=x level=ERROR msg=failed key=" + testKey + "\n"
	if err := os.WriteFile(filepath.Join(e.paths.Logs, "jenab.log"), []byte("first\n"+line), 0o644); err != nil {
		t.Fatal(err)
	}
	// The store redacts the keys it has read, as a provider call does.
	if _, err := e.secrets.Get(provider.KeyName("p")); err != nil {
		t.Fatal(err)
	}
	lines, err := e.svc.Dev.Log(ctx, "failed")
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || strings.Contains(lines[0], testKey) {
		t.Errorf("Log = %q", lines)
	}
	if ps, err := e.svc.Dev.Providers(ctx); err != nil || len(ps) != 1 {
		t.Errorf("Providers = %+v, %v", ps, err)
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
	if took := time.Since(start); took > ShutdownTimeout {
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

func TestShutdownDeadline(t *testing.T) {
	var order []string
	start := time.Now()
	err := Shutdown{
		Refuse: []func(){func() { order = append(order, "refuse") }},
		Cancel: func() { order = append(order, "cancel") },
		Wait: []func(context.Context) error{func(ctx context.Context) error {
			<-ctx.Done() // a turn that never ends
			order = append(order, "wait")
			return ctx.Err()
		}},
		Close: func(ctx context.Context) error {
			if dl, ok := ctx.Deadline(); !ok || time.Until(dl) < ShutdownTimeout-WaitTimeout-time.Second {
				t.Errorf("close has until %v", dl)
			}
			order = append(order, "close")
			return nil
		},
	}.Run()
	if !errors.Is(err, context.DeadlineExceeded) || !slices.Equal(order, []string{"refuse", "cancel", "wait", "close"}) {
		t.Fatalf("err %v, order %v", err, order)
	}
	if took := time.Since(start); took < WaitTimeout || took > WaitTimeout+time.Second {
		t.Errorf("took %v", took)
	}
}

func TestUIError(t *testing.T) {
	b := base{log: slog.New(slog.DiscardHandler), redact: func(s string) string { return strings.ReplaceAll(s, testKey, "[key]") }}
	cases := []struct {
		err  error
		kind string
	}{
		{agent.ErrInTurn, KindBusy},
		{store.ErrTitleTaken, KindInvalid},
		{store.ErrReadOnly, KindReadOnly},
		{project.ErrClosed, KindClosing},
		{errors.Join(errors.New("chat 01J"), store.ErrNotFound), KindNotFound},
		{&provider.Error{Provider: "p", Kind: provider.BadRequest, Message: "bad key " + testKey}, KindProvider},
		{errors.New("disk full near " + testKey), KindInternal},
	}
	for _, c := range cases {
		err := c.err
		b.guard("test", &err)
		var ue *UIError
		if !errors.As(err, &ue) || ue.Kind != c.kind || strings.Contains(ue.Message+ue.Details, testKey) {
			t.Errorf("%v: %#v", c.err, err)
		}
	}
	var ui error = &UIError{Kind: KindBusy, Message: "Wait."}
	b.guard("test", &ui)
	uiErr(t, ui, KindBusy)
	var none error
	b.guard("test", &none)
	if none != nil {
		t.Errorf("nil became %#v", none)
	}

	// A panic in a service method is an internal error, not a crash.
	err := func() (err error) {
		defer b.guard("panics", &err)
		panic("boom")
	}()
	uiErr(t, err, KindInternal)

	j, _ := json.Marshal(&UIError{Kind: KindBusy, Message: "Wait.", Details: "d"})
	if string(j) != `{"kind":"busy","message":"Wait.","details":"d"}` {
		t.Errorf("JSON %s", j)
	}
	// As Wails marshals a returned error: through the error interface.
	var asErr error = &UIError{Kind: KindInternal, Message: "Something went wrong."}
	j, _ = json.Marshal(&asErr)
	if string(j) != `{"kind":"internal","message":"Something went wrong."}` {
		t.Errorf("JSON through error %s", j)
	}
}

func TestPublisher(t *testing.T) {
	s := &sink{}
	p := &Publisher{emit: s.emit}
	p.Delta(chat.Delta{})
	p.Part(chat.PartDone{})
	p.Status(chat.Status{})
	p.Notice(project.Notice{})
	p.Activity(project.Activity{})
	want := []string{"chat:delta", "chat:part", "chat:status", "project:notice", "project:activity"}
	if got := s.names(); !slices.Equal(got, want) {
		t.Errorf("events %v", got)
	}
	// The payloads go out as the registered types, which Wails checks.
	if _, ok := s.events[0].data.(chat.Delta); !ok {
		t.Errorf("delta payload %T", s.events[0].data)
	}
}

// A snapshot holds the last SnapshotTurns turns; older ones come with
// Messages.
func TestSnapshotWindow(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	op, err := e.svc.Project.Create(ctx, "Long")
	if err != nil {
		t.Fatal(err)
	}
	for i := range SnapshotTurns + 1 {
		e.fp.Push(fake.Text("ok"))
		if _, err := e.svc.Chat.Send(ctx, op.ID, op.Mother, fmt.Sprint("turn ", i+1)); err != nil {
			t.Fatal(err)
		}
		e.idle(op.Mother)
	}
	snap, err := e.svc.Chat.Snapshot(ctx, op.ID, op.Mother)
	if err != nil {
		t.Fatal(err)
	}
	if snap.From != 2 || len(snap.Messages) != 2*SnapshotTurns || snap.Messages[0].Parts[0].Text.Text != "turn 2" {
		t.Fatalf("From %d, %d messages", snap.From, len(snap.Messages))
	}
	ms, err := e.svc.Chat.Messages(ctx, op.ID, op.Mother, 1, 1)
	if err != nil || len(ms) != 2 || ms[0].Parts[0].Text.Text != "turn 1" {
		t.Fatalf("older turn: %+v, %v", ms, err)
	}
}

// A rename during a turn moves the turn's later events past the
// snapshot's sequence number, so the frontend keeps them (Q32).
func TestWriteDuringTurn(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	op, err := e.svc.Project.Create(ctx, "Live")
	if err != nil {
		t.Fatal(err)
	}
	delta := func(text string) provider.Event {
		return provider.Event{Kind: provider.EventDelta, PartKind: chat.PartText, Text: text}
	}
	e.fp.Push(fake.Reply{Wait: 400 * time.Millisecond, Events: []provider.Event{delta("a"), delta("b")}, Hang: true})
	if _, err := e.svc.Chat.Send(ctx, op.ID, op.Mother, "Go"); err != nil {
		t.Fatal(err)
	}
	deltas := func() []chat.Delta {
		e.sink.mu.Lock()
		defer e.sink.mu.Unlock()
		var ds []chat.Delta
		for _, ev := range e.sink.events {
			if d, ok := ev.data.(chat.Delta); ok {
				ds = append(ds, d)
			}
		}
		return ds
	}
	wait := func(n int) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for len(deltas()) < n {
			if time.Now().After(deadline) {
				t.Fatalf("%d deltas; want %d", len(deltas()), n)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	wait(1)
	if _, err := e.svc.Chat.Rename(ctx, op.ID, op.Mother, "Renamed"); err != nil {
		t.Fatal(err)
	}
	snap, err := e.svc.Chat.Snapshot(ctx, op.ID, op.Mother)
	if err != nil {
		t.Fatal(err)
	}
	ps, err := e.svc.Settings.Providers(ctx)
	if err != nil || len(ps) != 1 || ps[0].Running != 1 {
		t.Errorf("Providers during the turn = %+v, %v", ps, err)
	}
	wait(2)
	if d := deltas()[1]; d.Seq < snap.Seq {
		t.Errorf("delta after the rename has seq %d; the snapshot has %d", d.Seq, snap.Seq)
	}
}

// A chat that asks the user is in Waiting and shows as waiting in List.
func TestWaiting(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	cs := e.svc.Chat
	if ws, err := cs.Waiting(ctx); err != nil || ws == nil || len(ws) != 0 {
		t.Fatalf("Waiting = %+v, %v", ws, err)
	}
	op, err := e.svc.Project.Create(ctx, "Coins")
	if err != nil {
		t.Fatal(err)
	}
	form := map[string]any{"questions": []map[string]any{
		{"header": "Currency", "question": "Which currency?", "options": []map[string]any{{"label": "EUR"}, {"label": "USD"}}},
	}}
	e.fp.Push(fake.ToolCall("c1", "ask_user", form), fake.Text("ok"))
	if _, err := cs.Send(ctx, op.ID, op.Mother, "set up prices"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for e.orch.State(op.ID, op.Mother) != chat.StateWaiting {
		if time.Now().After(deadline) {
			t.Fatal("the chat never waited")
		}
		time.Sleep(5 * time.Millisecond)
	}
	ws, err := cs.Waiting(ctx)
	if err != nil || len(ws) != 1 || ws[0].Project != op.ID || ws[0].Chat != op.Mother || ws[0].Title == "" ||
		ws[0].Waiting.Kind != chat.PartQuestion || ws[0].Waiting.Text != "Which currency?" {
		t.Fatalf("Waiting = %+v, %v", ws, err)
	}
	items, err := cs.List(ctx, op.ID)
	if err != nil || items[0].State != chat.StateWaiting {
		t.Fatalf("List = %+v, %v", items, err)
	}
	if err := cs.Stop(ctx, op.ID, op.Mother); err != nil {
		t.Fatal(err)
	}
	e.idle(op.Mother)
	if items, err := cs.List(ctx, op.ID); err != nil || items[0].State != chat.StateIdle {
		t.Fatalf("List after Stop = %+v, %v", items, err)
	}
}

func TestMemory(t *testing.T) {
	e := newEnv(t, t.TempDir())
	r, err := e.svc.System.Memory(context.Background())
	if err != nil || r.Total == 0 || r.Level == "" {
		t.Fatalf("Memory = %+v, %v", r, err)
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
