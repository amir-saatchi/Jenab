package app

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/secret"
)

// A provider on this computer's token is replaced, then removed, through
// Connect; the view tells which local providers have one (TASK-074).
func TestALocalProvidersKeyIsReplacedAndRemoved(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	ss := e.svc.Settings
	e.fp.Listed = []provider.ModelInfo{{ID: "qwen", Context: 8000}}
	req := ConnectRequest{Name: "studio", Kind: string(provider.KindCompatible), BaseURL: "http://localhost:1234/v1/"}
	stored := func() bool {
		_, err := e.secrets.Get(provider.KeyName("studio"))
		if err != nil && !errors.Is(err, secret.ErrNotFound) {
			t.Fatal(err)
		}
		return err == nil
	}
	keyed := func(v SettingsView) []string { return v.LocalKeys }

	r, err := ss.Connect(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if stored() || len(keyed(r.View)) != 0 {
		t.Errorf("connected without a token: stored %v, local keys %v", stored(), r.View.LocalKeys)
	}
	models := r.View.Settings.LLM.Providers["studio"].Models

	for _, key := range []string{"tok-first-0123", "tok-second-0123"} {
		req.Key = key
		r, err = ss.Connect(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		v, err := e.secrets.Get(provider.KeyName("studio"))
		if err != nil || v.Reveal() != key {
			t.Errorf("the token isn't the new one in the keychain: %v", err)
		}
		if !slices.Equal(keyed(r.View), []string{"studio"}) {
			t.Errorf("local keys = %v", r.View.LocalKeys)
		}
	}
	got, err := ss.Get(ctx)
	if err != nil || !slices.Equal(got.LocalKeys, []string{"studio"}) {
		t.Errorf("Get: local keys = %v, %v", got.LocalKeys, err)
	}

	req.Key = ""
	r, err = ss.Connect(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if stored() || len(keyed(r.View)) != 0 {
		t.Errorf("removed: stored %v, local keys %v", stored(), r.View.LocalKeys)
	}
	if got := r.View.Settings.LLM.Providers["studio"].Models; !slices.Equal(got, models) || len(got) == 0 {
		t.Errorf("models = %v, want %v", r.View.Settings.LLM.Providers["studio"].Models, models)
	}
	// "p" isn't on this computer: it always has a key and isn't listed.
	if _, ok := r.View.Settings.LLM.Providers["p"]; !ok || slices.Contains(r.View.LocalKeys, "p") {
		t.Errorf("p: %v", r.View.LocalKeys)
	}
	saved, err := ss.Save(ctx, r.View.Settings)
	if err != nil || saved.LocalKeys == nil || len(saved.LocalKeys) != 0 {
		t.Errorf("Save: local keys = %#v, %v", saved.LocalKeys, err)
	}
}
