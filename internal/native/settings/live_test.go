package settings_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/native/chat"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/settings"
	"github.com/amir-saatchi/jenab/internal/native/shell"
	"github.com/amir-saatchi/jenab/internal/native/state"
	"github.com/amir-saatchi/jenab/internal/native/state/statetest"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/backends"
	"github.com/amir-saatchi/jenab/internal/secret"
	"github.com/amir-saatchi/jenab/internal/store"
)

// memKeyring keeps the key in the test's memory, as the bench does; it
// never reaches the OS keychain.
type memKeyring struct {
	mu sync.Mutex
	m  map[string]string
}

func (k *memKeyring) Get(name string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if v, ok := k.m[name]; ok {
		return v, nil
	}
	return "", secret.ErrNotFound
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
	if _, ok := k.m[name]; !ok {
		return secret.ErrNotFound
	}
	delete(k.m, name)
	return nil
}

// TestLiveConnectAndPickAModel is TASK-055's live check: on the real
// settings service, Settings › Models connects Gemini with the user's key
// and makes gemini-3.5-flash-lite the default and fast model. It runs
// with JENAB_LIVE_SETTINGS=1 and GEMINI_API_KEY set; shots go to
// JENAB_SETTINGS_SHOTS when set.
func TestLiveConnectAndPickAModel(t *testing.T) {
	key := os.Getenv("GEMINI_API_KEY")
	if os.Getenv("JENAB_LIVE_SETTINGS") != "1" || key == "" {
		t.Skip("set JENAB_LIVE_SETTINGS=1 and GEMINI_API_KEY for the live check")
	}
	const model = "gemini-3.5-flash-lite"
	ctx := context.Background()
	root := t.TempDir()
	paths := config.Paths{Root: root, Settings: filepath.Join(root, "config.yaml"), Registry: filepath.Join(root, "registry.db"),
		Logs: filepath.Join(root, "logs")}.WithDataFolder(filepath.Join(root, "data"))
	if err := config.SaveSettings(paths.Settings, config.Defaults()); err != nil {
		t.Fatal(err)
	}
	s, problems, err := config.LoadSettings(paths.Settings)
	if err != nil {
		t.Fatal(err)
	}
	cfg := app.NewSettings(paths.Settings, s, problems)
	reg, err := store.OpenRegistry(ctx, paths.Registry)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reg.Close() })
	secrets := secret.New(&memKeyring{m: map[string]string{}})
	gate := limit.NewGate(s.LLM.MaxParallelCalls)
	models := provider.NewRegistry(provider.Deps{Settings: s.LLM, Secrets: secrets, Gate: gate, Backends: backends.All()})
	cfg.OnChange(func(s config.Settings) { gate.SetSize(s.LLM.MaxParallelCalls); models.Apply(s.LLM) })
	pm := project.NewManager(project.Deps{Paths: paths, Registry: reg})
	b := app.NewServices(app.Services{Projects: pm, Registry: reg, Models: models, Secrets: secrets, Settings: cfg,
		Started: app.Started{DataFolder: paths.DataFolder, DefaultDataFolder: paths.DataFolder}})

	loop := &statetest.Loop{}
	st := state.NewStores(state.API{Projects: b.Project, Chats: b.Chat, Settings: b.Settings, System: b.System}, loop.Post)
	t.Cleanup(st.Close)
	sh := shell.New(st, chat.New(st), loop.Post)
	sh.Settings = settings.New(st, loop.Post)
	sh.Start()
	tt, err := kit.NewTester(sh.View, 1280, 800, false)
	if err != nil {
		t.Fatal(err)
	}
	x := &fixture{loop: loop, st: st, sh: sh, tt: tt}
	x.settle(func() bool { return st.Projects.Loaded })
	x.open(state.Place{View: state.ViewSettings})
	wait := func(what string, cond func() bool) {
		t.Helper()
		for end := time.Now().Add(90 * time.Second); time.Now().Before(end); {
			if x.settle(cond) {
				return
			}
		}
		t.Fatalf("%s: no answer in 90 s; window shows %q", what, tt.Texts())
	}
	shot := func(name string) {
		if dir := os.Getenv("JENAB_SETTINGS_SHOTS"); dir != "" {
			saveShot(t, tt, filepath.Join(dir, name+".png"))
		}
	}

	x.click(t, "Add provider")
	wait("the presets", func() bool { return kit.Shows(tt, "Choose a provider") })
	x.reveal("Choose a provider")
	if err := tt.Click("Choose a provider"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	i := slices.IndexFunc(tt.Menu(), func(s string) bool { return strings.Contains(s, "Gemini") })
	if i < 0 {
		t.Fatalf("no Gemini in %q", tt.Menu())
	}
	if err := tt.ChooseMenuItem(tt.Menu()[i]); err != nil {
		t.Fatal(err)
	}
	x.settle(nil)
	x.focus(t, "API key")
	tt.Type(key)
	x.settle(nil)
	shot("live-1-connect")
	x.click(t, "Connect")
	wait("the connect", func() bool {
		v := st.Settings.View
		return v != nil && len(v.Settings.LLM.Providers) > 0
	})
	x.settle(nil)
	var name string
	for n := range st.Settings.View.Settings.LLM.Providers {
		name = n
	}
	t.Logf("connected %q; window says %q", name, strings.Join(slices.DeleteFunc(tt.Texts(), func(s string) bool { return !strings.Contains(s, "connected") }), " | "))

	// The model is on, or turned on in the provider's models.
	ref := name + "/" + model
	on := func() bool {
		return slices.ContainsFunc(st.Settings.View.Settings.LLM.Providers[name].Models, func(m config.ModelSettings) bool { return m.ID == model })
	}
	if !on() {
		x.click(t, "Models of "+name)
		wait("the model list", func() bool { return kit.Shows(tt, model) })
		x.focus(t, modelName(st, b, name, model))
		tt.Key(0, kit.KeySpace)
		wait("the model on", on)
		tt.Key(0, kit.KeyEscape)
		x.settle(nil)
	}
	wait("the model list", func() bool {
		return slices.ContainsFunc(st.Settings.Models, func(g app.ModelGroup) bool { return g.Provider == name })
	})
	for _, alias := range []string{"Default model", "Fast model"} {
		x.focus(t, alias)
		tt.Key(0, kit.KeyEnter)
		tt.Frame()
		label := modelName(st, b, name, model)
		if err := tt.ChooseMenuItem(label); err != nil {
			t.Fatalf("%s: %v; menu %q", alias, err, tt.Menu())
		}
		x.settle(nil)
	}
	wait("the aliases", func() bool {
		m := cfg.Get().LLM.Models
		return m["default"] == ref && m["fast"] == ref
	})
	x.click(t, "Models")
	shot("live-2-picked")
	t.Logf("default and fast are %s", ref)
}

// modelName is the name the pickers show for a provider's model.
func modelName(st *state.Stores, b app.Bound, provider, model string) string {
	for _, g := range st.Settings.Models {
		for _, m := range g.Models {
			if m.Ref == provider+"/"+model {
				return m.Name
			}
		}
	}
	pm, err := b.Settings.ProviderModels(context.Background(), provider)
	if err == nil {
		for _, m := range append(pm.Models, pm.Other...) {
			if m.ID == model {
				return m.Name
			}
		}
	}
	return model
}
