package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/provider"
)

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

// Concurrent changes all land, and an OnChange function may read the
// settings: it runs without the lock.
func TestSettingsUpdate(t *testing.T) {
	e := newEnv(t, t.TempDir())
	var applied atomic.Int32
	e.settings.OnChange(func(config.Settings) {
		_ = e.settings.Get()
		applied.Add(1)
	})
	const n = 10
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			_, _, err := e.settings.update(func(s *config.Settings) error {
				s.LLM.Providers[fmt.Sprintf("c%d", i)] = config.ProviderSettings{Kind: string(provider.KindCompatible), BaseURL: "https://example.test/v1/"}
				return nil
			})
			if err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if got := len(e.settings.Get().LLM.Providers); got != n+1 {
		t.Errorf("%d providers, want %d", got, n+1)
	}
	if applied.Load() != n {
		t.Errorf("OnChange ran %d times, want %d", applied.Load(), n)
	}
	// An error from fn saves nothing.
	before := e.settings.Get()
	if _, _, err := e.settings.update(func(s *config.Settings) error {
		delete(s.LLM.Providers, "p")
		return errors.New("no")
	}); err == nil {
		t.Error("update ignored fn's error")
	}
	if _, ok := e.settings.Get().LLM.Providers["p"]; !ok || len(before.LLM.Providers) != n+1 {
		t.Error("a failed update changed the settings")
	}
}
