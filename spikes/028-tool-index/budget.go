package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"os"
	"sync"
	"time"
	_ "time/tzdata" // the Pacific day of Gemini's quota, on Windows too

	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/backends"
)

// budget stops a provider's runs at its first quota error: a daily or
// monthly quota, not a per-minute limit, which the app waits out and
// retries. Gemini's free tier is also shared with other work, so its
// requests are capped a day (by Main-Session's rule) and counted in a file
// across runs of the harness; the day is Pacific time, when the quota
// resets.
type budget struct {
	mu      sync.Mutex
	path    string
	max     int
	day     string
	used    map[string]int             // Gemini requests by day
	stopped map[provider.Kind]string   // why a kind stopped
	stops   map[provider.Kind][]func() // cancel the kind's runs
}

var errBudget = errors.New("spike budget: no more requests")

func pacificDay(t time.Time) string {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		loc = time.FixedZone("PT", -8*3600)
	}
	return t.In(loc).Format(time.DateOnly)
}

func openBudget(path string, max int) (*budget, error) {
	b := &budget{path: path, max: max, day: pacificDay(time.Now()), used: map[string]int{},
		stopped: map[provider.Kind]string{}, stops: map[provider.Kind][]func(){}}
	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal(raw, &b.used); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	return b, nil
}

// onStop registers a cancel for a kind's runs.
func (b *budget) onStop(k provider.Kind, cancel func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.stops[k] = append(b.stops[k], cancel)
}

// left is what today's Gemini cap still allows.
func (b *budget) left() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.max - b.used[b.day]
}

func (b *budget) take(k provider.Kind) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if why := b.stopped[k]; why != "" {
		return fmt.Errorf("%w (%s)", errBudget, why)
	}
	if k != provider.KindGemini {
		return nil
	}
	if b.used[b.day] >= b.max {
		b.halt(k, fmt.Sprintf("the cap of %d Gemini requests for %s is used", b.max, b.day))
		return fmt.Errorf("%w (%s)", errBudget, b.stopped[k])
	}
	b.used[b.day]++
	raw, _ := json.MarshalIndent(b.used, "", "  ")
	return os.WriteFile(b.path, raw, 0o644)
}

// halt stops a kind's runs. b.mu is held.
func (b *budget) halt(k provider.Kind, why string) {
	if b.stopped[k] != "" {
		return
	}
	b.stopped[k] = why
	for _, stop := range b.stops[k] {
		go stop()
	}
}

func (b *budget) quota(k provider.Kind, err error) {
	// The app's own reading of the error.
	var pe *provider.Error
	if errors.As(err, &pe) && pe.Kind == provider.Quota {
		b.mu.Lock()
		b.halt(k, "the provider said: "+clip(err.Error(), 200))
		b.mu.Unlock()
	}
}

// why is why a kind stopped; "" while it runs.
func (b *budget) why(k provider.Kind) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.stopped[k]
}

// all is every kind that stopped, and why.
func (b *budget) all() map[provider.Kind]string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := map[provider.Kind]string{}
	for k, v := range b.stopped {
		out[k] = v
	}
	return out
}

// counted are the app's backends with each request checked against the
// budget and each error read for a quota.
func counted(b *budget) map[provider.Kind]provider.Factory {
	all := backends.All()
	if b == nil {
		return all
	}
	for k, inner := range all {
		all[k] = func(c provider.Connection) (provider.Provider, error) {
			p, err := inner(c)
			if err != nil {
				return nil, err
			}
			return &countedProvider{Provider: p, b: b, kind: k}, nil
		}
	}
	return all
}

type countedProvider struct {
	provider.Provider
	b    *budget
	kind provider.Kind
}

func (p *countedProvider) Stream(ctx context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	return func(yield func(provider.Event, error) bool) {
		if err := p.b.take(p.kind); err != nil {
			yield(provider.Event{}, err)
			return
		}
		for ev, err := range p.Provider.Stream(ctx, redirect(req)) {
			p.b.quota(p.kind, err)
			if !yield(ev, err) {
				return
			}
		}
	}
}

// clip shortens s to n bytes at a rune boundary.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n] + "…"
}
