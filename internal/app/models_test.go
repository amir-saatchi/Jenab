package app

import (
	"context"
	"errors"
	"math"
	"slices"
	"testing"
	"time"
	_ "time/tzdata" // Europe/Berlin on any system

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/secret"
)

func ids(ms []ModelOption, on bool) []string {
	var out []string
	for _, m := range ms {
		if m.On == on {
			out = append(out, m.ID)
		}
	}
	return out
}

// Settings → Models: catalog and other models, a new key, and Remove
// (SPEC 3.9).
func TestProviderModelsRekeyAndRemove(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	ss := e.svc.Settings
	e.fp.Listed = []provider.ModelInfo{{ID: "gemini-3.8-flash"}, {ID: "gemini-3.5-flash-lite"}, {ID: "gemini-next", Context: 4000}}
	if _, err := ss.Connect(ctx, ConnectRequest{Name: "g", Kind: string(provider.KindGemini), Key: "g-key-0123456789"}); err != nil {
		t.Fatal(err)
	}
	pm, err := ss.ProviderModels(ctx, "g")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids(pm.Models, true), []string{"gemini-3.8-flash", "gemini-3.5-flash-lite"}) || len(pm.Other) != 1 ||
		pm.Other[0].On || pm.Other[0].Known || pm.Other[0].Context != 4000 || pm.Models[0].Name != "Gemini 3.8 Flash" {
		t.Fatalf("ProviderModels = %+v", pm)
	}

	// The user turns one off and an other model on; a new key keeps that.
	next := e.settings.Get()
	next.LLM.Providers = map[string]config.ProviderSettings{"p": next.LLM.Providers["p"],
		"g": {Kind: "gemini", Models: []config.ModelSettings{{ID: "gemini-3.8-flash"}, {ID: "gemini-next", Context: 4000}}}}
	if _, err := ss.Save(ctx, next); err != nil {
		t.Fatal(err)
	}
	r, err := ss.Connect(ctx, ConnectRequest{Name: "g", Kind: string(provider.KindGemini), Key: "g-key-new-0123456789"})
	if err != nil || r.Models != 2 {
		t.Fatalf("Connect again = %+v, %v", r, err)
	}
	if v, _ := e.secrets.Get(provider.KeyName("g")); v.Reveal() != "g-key-new-0123456789" {
		t.Error("the new key is not stored")
	}
	if pm, _ = ss.ProviderModels(ctx, "g"); !slices.Equal(ids(pm.Models, true), []string{"gemini-3.8-flash"}) || !pm.Other[0].On {
		t.Fatalf("after a new key = %+v", pm)
	}

	// Removing p moves the aliases to g, and deletes p's key.
	v, err := ss.Remove(ctx, "p")
	if err != nil {
		t.Fatal(err)
	}
	llm := v.Settings.LLM
	if _, ok := llm.Providers["p"]; ok || llm.Models["default"] != "g/gemini-3.8-flash" || llm.Models["fast"] != "g/gemini-3.8-flash" {
		t.Fatalf("after Remove = %+v", llm)
	}
	if _, err := e.secrets.Get(provider.KeyName("p")); !errors.Is(err, secret.ErrNotFound) {
		t.Errorf("p's key: %v", err)
	}
	if _, err := ss.Remove(ctx, "g"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.secrets.Get(provider.KeyName("g")); !errors.Is(err, secret.ErrNotFound) {
		t.Errorf("g's key: %v", err)
	}
	if llm := e.settings.Get().LLM; len(llm.Models) != 0 || len(llm.Providers) != 0 {
		t.Errorf("after removing all = %+v", llm)
	}
	var ue *UIError
	if _, err := ss.Remove(ctx, "g"); !errors.As(err, &ue) || ue.Kind != KindNotFound {
		t.Errorf("Remove(gone) = %v", err)
	}
}

func TestUsage(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	ss := e.svc.Settings
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.Local)
	ss.now = func() time.Time { return now }
	next := e.settings.Get()
	next.LLM.Providers["g"] = config.ProviderSettings{Kind: "gemini", Models: []config.ModelSettings{{ID: "gemini-3.8-flash"}}}
	if _, err := ss.Save(ctx, next); err != nil {
		t.Fatal(err)
	}
	op, err := e.svc.Project.Create(ctx, "Coins")
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.pm.Open(ctx, op.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []chat.Message{
		{Model: "g/gemini-3.8-flash", CreatedAt: now, Usage: chat.Usage{Input: 1_000_000, Output: 1_000_000, CacheRead: 1_000_000}},
		{Model: "p/m1", CreatedAt: now.AddDate(0, 0, -1), Usage: chat.Usage{Output: 10}},
		{Model: "g/gemini-3.8-flash", CreatedAt: now.AddDate(0, 0, -40), Usage: chat.Usage{Output: 99}},
	} {
		m.Chat, m.Turn, m.Role = op.Mother, 1, chat.RoleAssistant
		m.Parts = []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: "hi"}}}
		if _, _, err := p.Chats.AppendMessage(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	p.Release()

	r, err := ss.Usage(ctx, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Days) != 30 || r.Days[29].Day != "2026-10-05" || r.Days[29].Output != 1_000_000 || r.Days[28].Output != 10 {
		t.Fatalf("days = %+v", r.Days[28:])
	}
	// 0.75 input + 3.75 output + 0.075 cache read, per million tokens.
	if math.Abs(r.Total.Cost-4.575) > 1e-9 || r.Total.Unpriced != 10 || r.Total.Output != 1_000_010 {
		t.Errorf("total = %+v", r.Total)
	}
	if len(r.Models) != 2 || r.Models[0].Model != "Gemini 3.8 Flash" || r.Models[0].Provider != "g" || r.Models[1].Model != "m1" {
		t.Errorf("models = %+v", r.Models)
	}
	if len(r.Chats) != 1 || r.Chats[0].Project != "Coins" || len(r.Skipped) != 0 {
		t.Errorf("chats = %+v, skipped %v", r.Chats, r.Skipped)
	}
}

func TestCheckFolder(t *testing.T) {
	ss := newEnv(t, t.TempDir()).svc.Settings
	var ue *UIError
	if _, err := ss.CheckFolder(context.Background(), "Jenab"); !errors.As(err, &ue) || ue.Kind != KindInvalid {
		t.Errorf("CheckFolder(relative) = %v", err)
	}
	if w, err := ss.CheckFolder(context.Background(), t.TempDir()); w != nil || err != nil {
		t.Errorf("CheckFolder(temp) = %v, %v", w, err)
	}
}

// Each message counts on its own local day, on both sides of a daylight
// saving change, and the days add up to the total.
func TestUsageAcrossDST(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip(err)
	}
	at := func(m time.Month, d, h, min int) time.Time { return time.Date(2026, m, d, h, min, 0, 0, berlin) }
	cases := []struct {
		name string
		now  time.Time
		msgs []time.Time
		want map[string]int // day → output tokens
	}{
		{"march", at(time.April, 2, 12, 0), // summer time from March 29
			[]time.Time{at(time.March, 27, 0, 30), at(time.March, 27, 23, 30), at(time.March, 29, 1, 30), at(time.March, 29, 3, 30), at(time.April, 2, 0, 15)},
			map[string]int{"2026-03-27": 2, "2026-03-29": 2, "2026-04-02": 1}},
		{"october", at(time.October, 28, 12, 0), // winter time from October 25
			[]time.Time{at(time.October, 22, 0, 30), at(time.October, 24, 23, 45), at(time.October, 25, 2, 30), at(time.October, 28, 0, 15), at(time.October, 28, 23, 59)},
			map[string]int{"2026-10-22": 1, "2026-10-24": 1, "2026-10-25": 1, "2026-10-28": 2}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			e := newEnv(t, t.TempDir())
			ss := e.svc.Settings
			ss.now = func() time.Time { return c.now }
			op, err := e.svc.Project.Create(ctx, "Coins")
			if err != nil {
				t.Fatal(err)
			}
			p, err := e.pm.Open(ctx, op.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, at := range c.msgs {
				m := chat.Message{Chat: op.Mother, Turn: 1, Role: chat.RoleAssistant, Model: "p/m1", CreatedAt: at, Usage: chat.Usage{Output: 1},
					Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: "hi"}}}}
				if _, _, err := p.Chats.AppendMessage(ctx, m); err != nil {
					t.Fatal(err)
				}
			}
			p.Release()

			r, err := ss.Usage(ctx, 7)
			if err != nil {
				t.Fatal(err)
			}
			sum := 0
			for _, d := range r.Days {
				sum += d.Output
				if d.Output != c.want[d.Day] {
					t.Errorf("%s: %d tokens, want %d", d.Day, d.Output, c.want[d.Day])
				}
			}
			if sum != r.Total.Output || sum != len(c.msgs) {
				t.Errorf("days add up to %d, total %d, want %d", sum, r.Total.Output, len(c.msgs))
			}
		})
	}
}
