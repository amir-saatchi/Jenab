package app

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/secret"
)

func TestModels(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	gs, err := e.svc.Settings.Models(ctx)
	if err != nil || len(gs) != 1 || gs[0].Provider != "p" || len(gs[0].Models) != 1 {
		t.Fatalf("Models = %+v, %v", gs, err)
	}
	m := gs[0].Models[0]
	if m.Ref != "p/m1" || m.Name != "m1" || !slices.Equal(m.Aliases, []string{"default", "fast"}) {
		t.Errorf("model = %+v", m)
	}
}

func TestPresets(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	ss := e.svc.Settings
	for _, up := range []bool{false, true} {
		ss.ollamaUp = func(context.Context) bool { return up }
		ps, err := ss.Presets(ctx)
		if err != nil || len(ps) != len(provider.Presets) {
			t.Fatalf("Presets = %+v, %v", ps, err)
		}
		i := slices.IndexFunc(ps, func(p PresetItem) bool { return p.ID == "ollama" })
		if !ps[i].NoKey || ps[i].Running != up || (up && i != 0) || (!up && i == 0) {
			t.Errorf("up %v: Ollama at %d: %+v", up, i, ps[i])
		}
		oc := ps[slices.IndexFunc(ps, func(p PresetItem) bool { return p.ID == "ollama-cloud" })]
		if oc.Kind != string(provider.KindOllama) || oc.NoKey || oc.Running {
			t.Errorf("up %v: Ollama Cloud = %+v", up, oc)
		}
		cf := ps[slices.IndexFunc(ps, func(p PresetItem) bool { return p.ID == "cloudflare" })]
		if !slices.Equal(cf.Fields, []string{"account_id"}) || cf.NoKey {
			t.Errorf("cloudflare = %+v", cf)
		}
	}
}

// A message sent with no provider is kept: the turn fails, and after
// Connect a Retry sends it (SPEC 3.9).
func TestConnectSendsKeptMessage(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	ss, cs := e.svc.Settings, e.svc.Chat
	none := e.settings.Get()
	none.LLM.Providers = map[string]config.ProviderSettings{}
	none.LLM.Models = map[string]string{}
	if _, err := ss.Save(ctx, none); err != nil {
		t.Fatal(err)
	}
	op, err := e.svc.Project.Create(ctx, "Coins")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cs.Send(ctx, op.ID, op.Mother, "track a price every day"); err != nil {
		t.Fatal(err)
	}
	e.idle(op.Mother)
	snap, err := cs.Snapshot(ctx, op.ID, op.Mother)
	if err != nil {
		t.Fatal(err)
	}
	last := snap.Messages[len(snap.Messages)-1].Parts[0]
	if last.Notice == nil || last.Notice.Kind != chat.NoticeTurnFailed {
		t.Fatalf("last part = %+v", last)
	}
	if len(e.fp.Calls()) != 0 {
		t.Fatalf("calls without a provider: %d", len(e.fp.Calls()))
	}

	e.fp.Listed = []provider.ModelInfo{{ID: "m2", Context: 8000}, {ID: "m3"}}
	r, err := ss.Connect(ctx, ConnectRequest{Name: " p2 ", Kind: string(provider.KindCompatible),
		BaseURL: "https://example.test/v1/", Key: "sk-new-0123456789"})
	if err != nil {
		t.Fatal(err)
	}
	llm := r.View.Settings.LLM
	if r.Provider != "p2" || r.Models != 2 || llm.Models["default"] != "p2/m2" || llm.Models["fast"] != "p2/m2" ||
		len(llm.Providers["p2"].Models) != 2 {
		t.Fatalf("Connect = %+v", r)
	}
	if v, err := e.secrets.Get(provider.KeyName("p2")); err != nil || v.Reveal() != "sk-new-0123456789" {
		t.Errorf("key in the keychain: %v", err)
	}
	if err := cs.Retry(ctx, op.ID, op.Mother); err != nil {
		t.Fatal(err)
	}
	e.idle(op.Mother)
	calls := e.fp.Calls()
	if len(calls) == 0 || calls[0].Model != "m2" || !sent(calls[0], "track a price every day") {
		t.Fatalf("calls = %+v", calls)
	}
	snap, err = cs.Snapshot(ctx, op.ID, op.Mother)
	if err != nil {
		t.Fatal(err)
	}
	if m := snap.Messages[len(snap.Messages)-1]; m.Role != chat.RoleAssistant || m.Parts[0].Text == nil {
		t.Fatalf("last message = %+v", m)
	}
}

func sent(req provider.Request, text string) bool {
	for _, m := range req.Messages {
		for _, p := range m.Parts {
			if m.Role == chat.RoleUser && p.Text != nil && p.Text.Text == text {
				return true
			}
		}
	}
	return false
}

func TestSetAliases(t *testing.T) {
	cat := provider.MustCatalog()
	ps := config.ProviderSettings{Kind: "openai_compatible", Models: []config.ModelSettings{{ID: "a"}, {ID: "b"}}}
	llm := config.LLMSettings{Models: map[string]string{"default": "old/x"},
		Providers: map[string]config.ProviderSettings{"keep": {}, "new": ps}}
	setAliases(&llm, cat, "new", ps)
	if llm.Models["default"] != "new/a" || llm.Models["fast"] != "new/a" {
		t.Errorf("aliases = %v", llm.Models)
	}
	// Aliases that point at a connected provider stay.
	llm.Models = map[string]string{"default": "keep/x", "fast": "keep/y"}
	setAliases(&llm, cat, "new", ps)
	if llm.Models["default"] != "keep/x" || llm.Models["fast"] != "keep/y" {
		t.Errorf("aliases = %v", llm.Models)
	}
	// The catalog's suggestions win when they are on.
	var def, fast string
	for _, k := range provider.Kinds {
		if def, fast = cat.Suggested(k); def != "" && fast != "" && def != fast {
			ps = config.ProviderSettings{Kind: string(k), Models: []config.ModelSettings{{ID: "z"}, {ID: fast}, {ID: def}}}
			break
		}
	}
	if def == "" {
		t.Skip("no kind with both suggestions")
	}
	llm.Models = map[string]string{}
	setAliases(&llm, cat, "new", ps)
	if llm.Models["default"] != "new/"+def || llm.Models["fast"] != "new/"+fast {
		t.Errorf("aliases = %v; want %s and %s", llm.Models, def, fast)
	}
}

func TestNotifyWithoutNotifier(t *testing.T) {
	e := newEnv(t, t.TempDir())
	if err := e.svc.System.Notify(context.Background(), Notification{Title: "t"}); err != nil {
		t.Fatal(err)
	}
	var got Notification
	e.svc.System.notify = func(n Notification) error { got = n; return nil }
	if err := e.svc.System.Notify(context.Background(), Notification{Chat: "c", Title: "Mother is waiting"}); err != nil || got.Title != "Mother is waiting" {
		t.Fatalf("Notify = %v, %+v", err, got)
	}
}

// A secret typed into the base URL reaches neither the error nor the log.
func TestConnectBaseURLSecret(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	var buf bytes.Buffer
	e.svc.Settings.log = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	for _, u := range []string{"https://x.test/v1/?key=sk-live-SECRET", "https://bob:pa55word@x.test/v1/"} {
		_, err := e.svc.Settings.Connect(ctx, ConnectRequest{Name: "x", Kind: string(provider.KindCompatible), BaseURL: u, Key: "k-0123456789"})
		uiErr(t, err, KindInvalid)
		ue := err.(*UIError)
		if s := ue.Message + ue.Details + buf.String(); strings.Contains(s, "SECRET") || strings.Contains(s, "55word") {
			t.Errorf("%s: a secret shows: %#v, log %q", u, ue, buf.String())
		}
	}
}

// Mistakes in the Connect form are invalid input with a useful message.
func TestConnectInvalid(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	for _, req := range []ConnectRequest{
		{Name: "lan", Kind: string(provider.KindCompatible), BaseURL: "http://192.168.1.5:8080/v1/", Key: "k-0123456789"},
		{Name: "nokey", Kind: string(provider.KindCompatible), BaseURL: "https://x.test/v1/"},
		{Name: "a/b", Kind: string(provider.KindCompatible), BaseURL: "https://x.test/v1/", Key: "k-0123456789"},
		{Name: "long", Kind: string(provider.KindCompatible), BaseURL: "https://x.test/v1/", Key: strings.Repeat("k", secret.MaxSize+1)},
		{Name: "cf", Kind: string(provider.KindCompatible), BaseURL: "https://x.test/{account_id}/v1/", Key: "k-0123456789"},
	} {
		_, err := e.svc.Settings.Connect(ctx, req)
		var ue *UIError
		if !errors.As(err, &ue) || ue.Kind != KindInvalid || ue.Message == "Something went wrong." {
			t.Errorf("%s: %#v", req.Name, err)
		}
	}
}

// A name in use for another kind or base URL is refused before its key is
// replaced; the same connection gets the new key and keeps its models.
func TestConnectNameTaken(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	ss := e.svc.Settings
	before := e.settings.Get().LLM.Providers["p"]
	for _, req := range []ConnectRequest{
		{Name: "p", Kind: string(provider.KindGemini), Key: "g-key-0123456789"},
		{Name: "p", Kind: string(provider.KindCompatible), BaseURL: "https://other.test/v1/", Key: "o-key-0123456789"},
	} {
		_, err := ss.Connect(ctx, req)
		uiErr(t, err, KindInvalid)
	}
	if v, err := e.secrets.Get(provider.KeyName("p")); err != nil || v.Reveal() != testKey {
		t.Errorf("the key changed: %v", err)
	}
	if got := e.settings.Get().LLM.Providers["p"]; !reflect.DeepEqual(got, before) {
		t.Errorf("settings changed: %+v", got)
	}

	r, err := ss.Connect(ctx, ConnectRequest{Name: "p", Kind: string(provider.KindCompatible), BaseURL: "https://example.test/v1/", Key: "new-key-0123456789"})
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := e.secrets.Get(provider.KeyName("p")); v.Reveal() != "new-key-0123456789" {
		t.Error("the key was not replaced")
	}
	if got := r.View.Settings.LLM.Providers["p"]; !reflect.DeepEqual(got.Models, before.Models) {
		t.Errorf("models = %+v, want %+v", got.Models, before.Models)
	}
}
