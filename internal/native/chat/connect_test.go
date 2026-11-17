package chat_test

import (
	"slices"
	"testing"

	"github.com/amir-saatchi/jenab/internal/app"
	jc "github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state/statetest"
)

// failed is the notice a turn that stopped leaves: a user message of one
// notice, which the model never reads.
func failed(turn int, text string) jc.Message {
	m := statetest.Text(turn, jc.RoleUser, "")
	m.Parts = []jc.Part{notice(jc.Notice{Kind: jc.NoticeTurnFailed, Text: text})}
	return m
}

// A turn a provider error stopped shows the Retry card, which continues
// it (SPEC 8.3).
func TestTheRetryCardRetries(t *testing.T) {
	both(t, func(t *testing.T, dark bool) {
		x := open(t, dark, statetest.Text(1, jc.RoleUser, persian), failed(1, "gemini is overloaded. Try again in a minute."))
		shows(t, x.tt, "The turn stopped", "gemini is overloaded. Try again in a minute.", "Retry")
		click(t, x, "Retry")
		x.until(t, "the retry", func() bool { return x.f.Retries() == 1 })
		// Once the turn goes on, the card is gone.
		x.status(jc.Status{State: jc.StateWorking})
		x.draw()
		if kit.Shows(x.tt, "The turn stopped") {
			t.Error("the Retry card stayed while the turn runs")
		}
	})
}

// With no provider set up, the turn that stopped shows the Connect card:
// Settings' form with Ollama, running here, picked first. Connecting
// sends the kept message (SPEC 3.9).
func TestTheConnectCardSendsTheKeptMessage(t *testing.T) {
	both(t, func(t *testing.T, dark bool) {
		set := statetest.NewSettings()
		set.PresetList = []app.PresetItem{
			{ID: "ollama", Name: "Ollama", Kind: "ollama", Group: "local", Host: "localhost:11434", NoKey: true, Running: true},
			{ID: "gemini", Name: "Google Gemini", Kind: "gemini", Group: "cloud", Host: "generativelanguage.googleapis.com"},
		}
		set.ConnectOK = func(req app.ConnectRequest) (app.ConnectResult, error) {
			v := app.SettingsView{Settings: config.Defaults()}
			v.Settings.LLM.Providers = map[string]config.ProviderSettings{
				req.Name: {Kind: req.Kind, Models: []config.ModelSettings{{ID: "qwen3:8b"}}},
			}
			return app.ConnectResult{Provider: req.Name, Models: 1, View: v}, nil
		}
		f := statetest.New()
		x := openWith(t, dark, f, set.Use(f.API()), statetest.Text(1, jc.RoleUser, persian),
			failed(1, "No AI model is set up. Add a provider in Settings › Models."))
		x.st.Settings.Load()
		x.until(t, "the Connect card", func() bool { return kit.Shows(x.tt, "Ollama runs on this computer; no key is needed.") })
		shows(t, x.tt, "Notes needs an AI model to answer",
			"Add a provider with your own key. Your message is kept and sent once it is connected.", "Ollama", "Connect", persian)
		if kit.Shows(x.tt, "The turn stopped") {
			t.Error("the Retry card shows with no provider")
		}
		shot(t, x, "connect-card")
		click(t, x, "Connect")
		x.until(t, "the kept message sent", func() bool { return x.f.Retries() == 1 })
		var reqs []app.ConnectRequest
		set.Lock(func() { reqs = slices.Clone(set.Connects) })
		if len(reqs) != 1 || reqs[0].Kind != "ollama" || reqs[0].Key != "" {
			t.Errorf("connects %+v", reqs)
		}
		if got := toasts(x); len(got) != 1 {
			t.Errorf("toasts %q", got)
		}
		// A provider is set up now: the card is gone.
		if kit.Shows(x.tt, "Notes needs an AI model to answer") {
			t.Error("the Connect card stayed once connected")
		}
	})
}

// The new provider's settings arrive before its model list: the card stops
// showing in between, and the kept message is still sent once the connect
// is done (TASK-071).
func TestAConnectWhoseModelsAnswerLateSendsTheKeptMessage(t *testing.T) {
	both(t, func(t *testing.T, dark bool) {
		set := statetest.NewSettings()
		set.PresetList = []app.PresetItem{
			{ID: "ollama", Name: "Ollama", Kind: "ollama", Group: "local", Host: "localhost:11434", NoKey: true, Running: true},
		}
		set.ConnectOK = func(req app.ConnectRequest) (app.ConnectResult, error) {
			v := app.SettingsView{Settings: config.Defaults()}
			v.Settings.LLM.Providers = map[string]config.ProviderSettings{
				req.Name: {Kind: req.Kind, Models: []config.ModelSettings{{ID: "qwen3:8b"}}},
			}
			return app.ConnectResult{Provider: req.Name, Models: 1, View: v}, nil
		}
		f := statetest.New()
		x := openWith(t, dark, f, set.Use(f.API()), statetest.Text(1, jc.RoleUser, persian),
			failed(1, "No AI model is set up. Add a provider in Settings › Models."))
		x.st.Settings.Load()
		x.until(t, "the Connect card", func() bool { return kit.Shows(x.tt, "Ollama runs on this computer; no key is needed.") })
		wait := make(chan struct{})
		set.Lock(func() { set.HoldModels = wait })
		click(t, x, "Connect")
		// The settings with the provider show while its models load: the
		// card is gone.
		x.until(t, "the card gone", func() bool { return !kit.Shows(x.tt, "Notes needs an AI model to answer") })
		if n := x.f.Retries(); n != 0 {
			t.Fatalf("sent before the connect was done: %d", n)
		}
		close(wait)
		x.until(t, "the kept message sent", func() bool { return x.f.Retries() == 1 })
		if got := toasts(x); len(got) != 1 {
			t.Errorf("toasts %q", got)
		}
	})
}
