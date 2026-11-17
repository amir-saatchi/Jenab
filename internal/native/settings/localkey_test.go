package settings_test

import (
	"slices"
	"testing"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/native/kit"
)

// keys answers each connect as the backend does for a key: a typed one is
// stored, an empty one removes the stored one, and the models stay.
func keys(x *fixture) {
	x.set.Lock(func() {
		x.set.ConnectOK = func(req app.ConnectRequest) (app.ConnectResult, error) {
			var v app.SettingsView
			x.set.Lock(func() { v = x.set.View })
			v.LocalKeys = slices.DeleteFunc(slices.Clone(v.LocalKeys), func(n string) bool { return n == req.Name })
			if req.Key != "" {
				v.LocalKeys = append(v.LocalKeys, req.Name)
			}
			return app.ConnectResult{Provider: req.Name, Models: 1, View: v}, nil
		}
	})
}

// TASK-074: a provider on this computer gets Replace key too, with the key
// optional, so its token can be changed or removed.
func TestALocalProvidersTokenIsReplacedAndRemoved(t *testing.T) {
	both(t, func(t *testing.T, x *fixture) {
		x.set.Lock(func() {
			x.set.View.Settings.LLM.Providers["lmstudio"] = config.ProviderSettings{Kind: "openai_compatible", BaseURL: "http://localhost:1234/v1/",
				Models: []config.ModelSettings{{ID: "qwen"}}}
			x.set.View.LocalKeys = []string{"lmstudio"}
		})
		x.st.Settings.Load()
		x.settle(func() bool {
			x.tt.Frame()
			return kit.Shows(x.tt, "On this computer · key in the keychain · 1 model on")
		})
		keys(x)
		shows(t, x.tt, "On this computer · key in the keychain · 1 model on", "On this computer · no key needed · 1 model on")

		// Replaced: the new token goes with the check, to the keychain.
		x.click(t, "Replace the key of lmstudio")
		x.settle(nil)
		shows(t, x.tt, "New key for lmstudio", "API key (optional)",
			"The saved key is never shown. A new one replaces it, and an empty field removes it; the models stay as they are.",
			"Leave it empty to remove the saved key · checked with the server first", "Remove key")
		x.focus(t, "API key (optional)")
		x.tt.Type("tok-new")
		x.settle(nil)
		hides(t, x.tt, "Remove key")
		x.click(t, "Save key")
		x.settle(func() bool { return len(requests(x)) == 1 })
		x.settle(nil)
		shows(t, x.tt, "lmstudio: the new key works and is saved", "On this computer · key in the keychain · 1 model on")
		hides(t, x.tt, "New key for lmstudio")

		// Removed: the empty field is sent, and the row says no key.
		x.click(t, "Replace the key of lmstudio")
		x.settle(nil)
		x.click(t, "Remove key")
		x.settle(func() bool { return len(requests(x)) == 2 })
		x.settle(func() bool {
			x.tt.Frame()
			return kit.Shows(x.tt, "lmstudio: the key is removed; the server answers without one")
		})
		hides(t, x.tt, "On this computer · key in the keychain · 1 model on", "New key for lmstudio")

		r := requests(x)
		want := app.ConnectRequest{Name: "lmstudio", Kind: "openai_compatible", BaseURL: "http://localhost:1234/v1/"}
		for i, key := range []string{"tok-new", ""} {
			got := r[i]
			if got.Name != want.Name || got.Kind != want.Kind || got.BaseURL != want.BaseURL || got.Key != key {
				t.Errorf("connect %d = %+v, key %q", i, got, key)
			}
		}
		if ms := x.saved().LLM.Providers["lmstudio"].Models; len(ms) != 1 || ms[0].ID != "qwen" {
			t.Errorf("models = %v", ms)
		}

		// With no key, the empty field has nothing to remove.
		x.click(t, "Replace the key of lmstudio")
		x.settle(nil)
		shows(t, x.tt, "This server is on this computer and has no key. Add one only if it asks for a token; the models stay as they are.",
			"Only if the server asks for a token · stored in the OS keychain")
		hides(t, x.tt, "Remove key")
		if !disabled(t, x.tt, "Save key") {
			t.Error("Save key is on with nothing typed and no key to remove")
		}
	})
}

// A provider that isn't on this computer still needs its key.
func TestARemoteKeyStaysNeeded(t *testing.T) {
	x := start(t, false, seed())
	x.click(t, "Replace the key of gemini")
	x.settle(nil)
	shows(t, x.tt, "API key", "The saved key is never shown. A new one replaces it; the models stay as they are.")
	hides(t, x.tt, "API key (optional)", "Remove key")
	if !disabled(t, x.tt, "Save key") {
		t.Error("Save key is on with no key typed")
	}
}
