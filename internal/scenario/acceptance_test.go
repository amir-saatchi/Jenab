package scenario

import (
	"context"
	"encoding/json"
	"iter"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
)

// historyModel is a scripted model for the acceptance set. It answers "ok",
// except for three messages: the sum after the restart, from the list it
// still sees in its window; the question about the purchase, with a
// search_history call; and the search's result, with what it found.
type historyModel struct {
	mu    sync.Mutex
	calls []provider.Request
}

func (m *historyModel) Models(context.Context) ([]provider.ModelInfo, error) { return nil, nil }

func (m *historyModel) Stream(ctx context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	m.mu.Lock()
	m.calls = append(m.calls, req)
	m.mu.Unlock()
	last := req.Messages[len(req.Messages)-1]
	r := fake.Text("ok")
	switch {
	case last.Role == chat.RoleTool:
		var found strings.Builder
		for _, p := range last.Parts {
			if p.ToolResult != nil {
				found.WriteString(p.ToolResult.Text)
			}
		}
		r = fake.Text("From the history: " + found.String())
	case strings.Contains(textOf(last), "Add those five"):
		if strings.Contains(textOfAll(req.Messages), "List the prices from 2026-10-01") {
			r = fake.Text("They add up to 301,500.")
		} else {
			r = fake.Text("I don't see the list any more.")
		}
	case strings.Contains(textOf(last), "my first Bitcoin purchase"):
		r = fake.ToolCall("h1", "search_history", map[string]any{"query": "bought"})
	}
	// The usage reports the whole prompt, so the truncation guard is calm.
	n := r.Events[len(r.Events)-1]
	n.Usage = &chat.Usage{Input: provider.EstimateTokens(req), Output: n.Usage.Output}
	r.Events[len(r.Events)-1] = n
	return fake.New(r).Stream(ctx, req)
}

func textOf(m chat.Message) string {
	var b strings.Builder
	for _, p := range m.Parts {
		if p.Text != nil {
			b.WriteString(p.Text.Text)
		}
	}
	return b.String()
}

func textOfAll(ms []chat.Message) string {
	var b strings.Builder
	for _, m := range ms {
		b.WriteString(textOf(m) + "\n")
		for _, p := range m.Parts {
			if p.ToolResult != nil {
				b.WriteString(p.ToolResult.Text + "\n")
			}
		}
	}
	return b.String()
}

// TestAcceptanceRestart runs P1-18's scenario on a scripted model: a chat
// of 25 turns with a restart before turn 13. The chat goes on after the
// restart, and at turn 25 the purchase from turn 3 is found only through
// search_history: it is no longer in the window.
func TestAcceptanceRestart(t *testing.T) {
	s, err := Load(filepath.Join("..", "..", "scenarios", "acceptance"))
	if err != nil {
		t.Fatal(err)
	}
	m := &historyModel{}
	models := Static([]ProviderSpec{{Name: "p", Kind: "openai_compatible", BaseURL: "https://example.test/v1/", Models: []config.ModelSettings{{ID: "m", Context: 32000}}}},
		map[string]string{"p": "test-key-0123456789"}, "p/m")
	rep, err := RunSet(context.Background(), s, Options{
		Models: models, Backends: map[provider.Kind]provider.Factory{provider.KindCompatible: func(provider.Connection) (provider.Provider, error) { return m, nil }},
		Scale: 0.01, Timeout: 2 * time.Minute, Temp: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	r := rep.Runs[0]
	if r.Error != "" {
		t.Fatalf("error: %s", r.Error)
	}
	for _, a := range r.Asserts {
		if a.Status != Pass {
			t.Errorf("%s (%s): %s, %s", a.Type, a.Test, a.Status, a.Why)
		}
	}
	if r.Turns != 25 {
		t.Errorf("%d turns, want 25", r.Turns)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	asking := func(s string) *provider.Request {
		for i, c := range m.calls {
			if strings.Contains(textOf(c.Messages[len(c.Messages)-1]), s) {
				return &m.calls[i]
			}
		}
		t.Fatalf("no request asks %q", s)
		return nil
	}

	// The restart cut the window: turn 13 sees turns 10 to 12. Without
	// it, the window would reach back to turn 7.
	if all := textOfAll(asking("Add those five").Messages); strings.Contains(all, "Which keywords are we watching") || !strings.Contains(all, "How many rows") {
		t.Errorf("turn 13's window after the restart:\n%s", all)
	}

	// The purchase was out of the window when turn 25 asked about it, and
	// the search found it on disk after the restart.
	ask := asking("my first Bitcoin purchase")
	if strings.Contains(textOfAll(ask.Messages), "Bitstamp") {
		t.Error("turn 3 is still in the window at turn 25")
	}
	var args json.RawMessage
	for _, c := range m.calls {
		for _, msg := range c.Messages {
			for _, p := range msg.Parts {
				if p.ToolCall != nil && p.ToolCall.Name == "search_history" {
					args = p.ToolCall.Args
				}
			}
		}
	}
	if string(args) != `{"query":"bought"}` {
		t.Errorf("search_history args %s", args)
	}
}
