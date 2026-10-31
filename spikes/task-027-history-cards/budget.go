package main

// SPIKE-028's budget, with Main-Session's rules for TASK-027: a model stops
// at its provider's first quota error; a rate limit slows its requests
// down (the app also waits and retries); and Gemini's requests are capped
// a Pacific day (shared with other work), counted in a file across runs
// of the harness.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"log"
	"os"
	"sync"
	"time"
	_ "time/tzdata" // the Pacific day of Gemini's quota, on Windows too

	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/backends"
)

type budget struct {
	mu      sync.Mutex
	path    string
	max     int
	day     string
	used    map[string]int             // Gemini requests by day
	stopped map[provider.Kind]string   // why a kind stopped
	stops   map[provider.Kind][]func() // cancel the kind's runs
	sent    map[provider.Kind]int      // requests sent in this process
	limited map[provider.Kind]int      // rate-limit errors seen
	gap     map[provider.Kind]time.Duration
	last    map[provider.Kind]time.Time
}

// The gap between a kind's requests after a rate limit: it starts at
// gapStart, doubles at each one, up to gapMax.
const (
	gapStart = 2 * time.Second
	gapMax   = 30 * time.Second
)

var errBudget = errors.New("harness budget: no more requests")

func pacificDay(t time.Time) string {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		loc = time.FixedZone("PT", -8*3600)
	}
	return t.In(loc).Format(time.DateOnly)
}

func openBudget(path string, max int) (*budget, error) {
	b := &budget{path: path, max: max, day: pacificDay(time.Now()), used: map[string]int{},
		stopped: map[provider.Kind]string{}, stops: map[provider.Kind][]func(){}, sent: map[provider.Kind]int{},
		limited: map[provider.Kind]int{}, gap: map[provider.Kind]time.Duration{}, last: map[provider.Kind]time.Time{}}
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
	return b.max - b.used[pacificDay(time.Now())]
}

// wait keeps a rate-limited kind's requests gap apart.
func (b *budget) wait(ctx context.Context, k provider.Kind) error {
	b.mu.Lock()
	gap, last := b.gap[k], b.last[k]
	b.mu.Unlock()
	if d := time.Until(last.Add(gap)); d > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(d):
		}
	}
	return nil
}

func (b *budget) take(k provider.Kind) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if why := b.stopped[k]; why != "" {
		return fmt.Errorf("%w (%s)", errBudget, why)
	}
	if k == provider.KindGemini {
		day := pacificDay(time.Now())
		if b.used[day] >= b.max {
			b.halt(k, fmt.Sprintf("the cap of %d Gemini requests for %s is used", b.max, day))
			return fmt.Errorf("%w (%s)", errBudget, b.stopped[k])
		}
		b.used[day]++
	}
	b.sent[k]++
	b.last[k] = time.Now()
	if k != provider.KindGemini {
		return nil
	}
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

// check reads an error as the app does: a quota stops the kind, a rate
// limit widens the gap between its requests.
func (b *budget) check(k provider.Kind, err error) {
	var pe *provider.Error
	if !errors.As(err, &pe) {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	switch pe.Kind {
	case provider.Quota:
		b.halt(k, fmt.Sprintf("the provider said (%s): %s", pe.Kind, clip(err.Error(), 200)))
	case provider.RateLimited:
		b.limited[k]++
		b.gap[k] = min(gapMax, max(gapStart, 2*b.gap[k]))
		log.Printf("%s: rate limited (%d so far); requests now %s apart", k, b.limited[k], b.gap[k])
	}
}

// rateLimits is the rate-limit errors a kind has had.
func (b *budget) rateLimits(k provider.Kind) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.limited[k]
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

// requests is what this process sent, by kind.
func (b *budget) requests() map[provider.Kind]int {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := map[provider.Kind]int{}
	for k, v := range b.sent {
		out[k] = v
	}
	return out
}

// counted are the app's backends with each request checked against the
// budget and each error read for a quota or rate limit.
func counted(b *budget) map[provider.Kind]provider.Factory {
	all := backends.All()
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
		if err := p.b.wait(ctx, p.kind); err != nil {
			yield(provider.Event{}, err)
			return
		}
		if err := p.b.take(p.kind); err != nil {
			yield(provider.Event{}, err)
			return
		}
		for ev, err := range p.Provider.Stream(ctx, req) {
			p.b.check(p.kind, err)
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
