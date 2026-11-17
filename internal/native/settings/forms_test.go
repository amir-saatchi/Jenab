package settings_test

import (
	"slices"
	"testing"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/native/kit"
)

// TASK-068's fixes to Settings' forms.

// disabled tells whether the element named name takes no input.
func disabled(t *testing.T, tt *kit.Tester, name string) bool {
	t.Helper()
	for _, a := range tt.AccessAll() {
		if a.Name == name {
			return a.Disabled
		}
	}
	t.Fatalf("no %q in the window", name)
	return false
}

// connects answers each connect as the backend does: the provider is
// saved with one model. wait, when not nil, holds each answer until it is
// closed.
func connects(x *fixture, wait chan struct{}) {
	x.set.Lock(func() {
		x.set.ConnectOK = func(req app.ConnectRequest) (app.ConnectResult, error) {
			if wait != nil {
				<-wait
			}
			var v app.SettingsView
			x.set.Lock(func() { v = x.set.View })
			ps := map[string]config.ProviderSettings{}
			for k, p := range v.Settings.LLM.Providers {
				ps[k] = p
			}
			ps[req.Name] = config.ProviderSettings{Kind: req.Kind, BaseURL: req.BaseURL, Models: []config.ModelSettings{{ID: "m"}}}
			v.Settings.LLM.Providers = ps
			return app.ConnectResult{Provider: req.Name, Models: 1, View: v}, nil
		}
	})
}

// requests are the connects sent so far.
func requests(x *fixture) []app.ConnectRequest {
	var out []app.ConnectRequest
	x.set.Lock(func() { out = slices.Clone(x.set.Connects) })
	return out
}

const addText = "The key is checked with the provider and stored in the OS keychain."

// pick opens Add provider, unless it shows, and chooses provider in it.
func (x *fixture) pick(t *testing.T, provider string) {
	t.Helper()
	if !kit.Shows(x.tt, addText) {
		x.click(t, "Add provider")
	}
	x.settle(func() bool {
		x.tt.Frame()
		return kit.Shows(x.tt, "Provider") && !kit.Shows(x.tt, "Loading the providers")
	})
	x.focus(t, "Provider")
	x.tt.Key(0, kit.KeyEnter)
	x.tt.Frame()
	if err := x.tt.ChooseMenuItem(provider); err != nil {
		t.Fatalf("%v; menu %q", err, x.tt.Menu())
	}
	x.settle(nil)
}

// 1. A key typed for one provider is dropped when another is chosen, so
// it never goes to the other's URL, and when the dialog closes. So is a
// base-URL placeholder value, such as Cloudflare's account ID.
func TestAKeyIsDroppedWhenTheProviderChanges(t *testing.T) {
	both(t, func(t *testing.T, x *fixture) {
		connects(x, nil)
		x.set.Lock(func() {
			x.set.PresetList = append(x.set.PresetList, app.PresetItem{ID: "cloudflare", Name: "Cloudflare Workers AI", Kind: "openai_compatible",
				BaseURL: "https://api.cloudflare.com/client/v4/accounts/{account_id}/ai/v1/", Fields: []string{"account_id"}, Group: "cloud"})
		})
		x.pick(t, "Cloudflare Workers AI")
		x.focus(t, "Account ID")
		x.tt.Type("acct-test")
		x.focus(t, "API key")
		x.tt.Type("sk-test")
		x.settle(nil)
		if disabled(t, x.tt, "Connect") {
			t.Fatal("Connect is off with the account ID and key typed")
		}
		x.pick(t, "Anthropic")
		if !disabled(t, x.tt, "Connect") {
			t.Error("the key typed for Cloudflare stayed for Anthropic")
		}
		x.pick(t, "Cloudflare Workers AI")
		for _, a := range x.tt.AccessAll() {
			if a.Name == "Account ID" && a.Value != "" {
				t.Errorf("the account ID stayed: %q", a.Value)
			}
		}
		x.pick(t, "Anthropic")
		x.focus(t, "API key")
		x.tt.Type("sk-test")
		x.settle(nil)
		if disabled(t, x.tt, "Connect") {
			t.Fatal("Connect is off with a key typed")
		}
		x.pick(t, "Google Gemini")
		if !disabled(t, x.tt, "Connect") {
			t.Error("the key typed for Anthropic stayed for Google Gemini")
		}
		// Closed and opened again, the form starts empty.
		x.pick(t, "Anthropic")
		x.focus(t, "API key")
		x.tt.Type("sk-test")
		x.settle(nil)
		x.tt.Key(0, kit.KeyEscape)
		x.settle(nil)
		hides(t, x.tt, addText)
		x.pick(t, "Anthropic")
		if !disabled(t, x.tt, "Connect") {
			t.Error("the key stayed after the dialog closed")
		}
		if n := len(requests(x)); n != 0 {
			t.Errorf("%d connects sent", n)
		}
	})
}

// 2. Presets that can't be read show the error and Try again, not a
// spinner for good.
func TestThePresetsErrorOffersTryAgain(t *testing.T) {
	both(t, func(t *testing.T, x *fixture) {
		x.set.Lock(func() { x.set.PresetsBad = 1 })
		x.click(t, "Add provider")
		x.settle(func() bool { x.tt.Frame(); return kit.Shows(x.tt, "Try again") })
		shows(t, x.tt, "The providers couldn't be read", "the presets couldn't be read", "Try again")
		hides(t, x.tt, "Loading the providers")
		x.click(t, "Try again")
		x.settle(func() bool { x.tt.Frame(); return kit.Shows(x.tt, "Choose a provider") })
		shows(t, x.tt, "Choose a provider")
		hides(t, x.tt, "Try again")
	})
}

// 3. A connect that ends after its dialog was closed doesn't close the
// dialog opened since.
func TestALateConnectLeavesANewDialogOpen(t *testing.T) {
	x := start(t, false, seed())
	wait := make(chan struct{})
	connects(x, wait)
	x.pick(t, "Anthropic")
	x.focus(t, "API key")
	x.tt.Type("sk-test")
	x.settle(nil)
	x.click(t, "Connect")
	x.settle(func() bool { return len(requests(x)) == 1 })
	x.tt.Key(0, kit.KeyEscape)
	x.settle(nil)
	hides(t, x.tt, addText)
	x.click(t, "Add provider")
	x.settle(func() bool { x.tt.Frame(); return kit.Shows(x.tt, "Choose a provider") })
	close(wait)
	x.settle(func() bool { _, ok := x.saved().LLM.Providers["anthropic"]; return ok })
	x.settle(nil)
	shows(t, x.tt, "Anthropic connected · 1 model on", addText, "Choose a provider")
}

// 4. Save waits for the folder check, and a folder the check refuses
// can't be saved.
func TestTheDataFolderWaitsForItsCheck(t *testing.T) {
	both(t, func(t *testing.T, x *fixture) {
		x.set.Lock(func() { x.set.Refuse[`D:\Jenab\projects\p1`] = "The data folder can't be inside the projects folder." })
		x.click(t, "Change…")
		typed := func(dir string) {
			x.focus(t, "Folder")
			x.tt.Key(kit.Ctrl, kit.KeyA)
			x.tt.Type(dir)
			x.settle(nil)
		}
		checked := func(text string) {
			t.Helper()
			x.settle(func() bool { x.tt.Frame(); return kit.Shows(x.tt, text) })
			shows(t, x.tt, text)
			if !disabled(t, x.tt, "Save") || !invalid(x.tt, "Folder") {
				t.Errorf("%q: Save is on, or the field isn't marked", text)
			}
		}
		passes := func(dir string) {
			t.Helper()
			typed(dir)
			if !disabled(t, x.tt, "Save") {
				t.Fatalf("%s: Save is on before the check", dir)
			}
			x.settle(func() bool { x.tt.Frame(); return !disabled(t, x.tt, "Save") })
			if disabled(t, x.tt, "Save") {
				t.Fatalf("%s: Save stayed off after the check passed", dir)
			}
		}
		passes(`E:\Sync`)
		typed("Jenab")
		checked(`"Jenab" is not a full path. Choose a folder.`)
		typed(`D:\Jenab\projects\p1`)
		checked("The data folder can't be inside the projects folder.")
		passes(`E:\Data`)
		x.click(t, "Save")
		x.settle(func() bool { return x.saved().DataFolder != "" })
		if got := x.saved().DataFolder; got != `E:\Data` {
			t.Errorf("data folder = %q", got)
		}
	})
}

// 5. A provider on this computer needs no key, LM Studio too, but Add
// provider takes an optional one for a server that wants a token.
func TestAProviderOnThisComputerNeedsNoKey(t *testing.T) {
	both(t, func(t *testing.T, x *fixture) {
		x.set.Lock(func() {
			ps := x.set.View.Settings.LLM.Providers
			ps["lmstudio"] = config.ProviderSettings{Kind: "openai_compatible", BaseURL: "http://localhost:1234/v1/", Models: []config.ModelSettings{{ID: "qwen"}}}
			x.set.PresetList = append(x.set.PresetList, app.PresetItem{ID: "lmstudio", Name: "LM Studio", Kind: "openai_compatible",
				BaseURL: "http://localhost:1234/v1/", Group: "local", Host: "localhost:1234", NoKey: true})
		})
		x.st.Settings.Load()
		x.settle(func() bool { x.tt.Frame(); return kit.Shows(x.tt, "lmstudio") })
		shows(t, x.tt, "On this computer · no key needed · 1 model on", "Replace the key of gemini")
		connects(x, nil)

		// No key: Connect works with the field empty.
		x.pick(t, "LM Studio")
		shows(t, x.tt, "API key (optional)", "Only if the server asks for a token · stored in the OS keychain")
		x.click(t, "Connect")
		x.settle(func() bool { return len(requests(x)) == 1 })
		// A token: it goes with the connect, to the keychain.
		x.pick(t, "LM Studio")
		x.focus(t, "API key (optional)")
		x.tt.Type("tok-test")
		x.settle(nil)
		x.click(t, "Connect")
		x.settle(func() bool { return len(requests(x)) == 2 })
		// Any OpenAI-compatible URL on this computer: the key is optional.
		x.pick(t, "OpenAI-compatible…")
		x.focus(t, "Name")
		x.tt.Type("vllm")
		x.focus(t, "Base URL")
		x.tt.Type("http://127.0.0.1:8000/v1/")
		x.settle(nil)
		shows(t, x.tt, "API key (optional)")
		x.click(t, "Connect")
		x.settle(func() bool { return len(requests(x)) == 3 })
		r := requests(x)
		if r[0].Kind != "openai_compatible" || r[0].Key != "" || r[0].BaseURL != "http://localhost:1234/v1/" {
			t.Errorf("first = %+v", r[0])
		}
		if r[1].Key != "tok-test" {
			t.Error("the token didn't go with the second connect")
		}
		if r[2].Name != "vllm" || r[2].Key != "" || r[2].BaseURL != "http://127.0.0.1:8000/v1/" {
			t.Errorf("third = %+v", r[2])
		}
	})
}
