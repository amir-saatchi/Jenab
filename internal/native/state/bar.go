package state

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/sysmem"
)

// The bottom bar (SPEC 5.12): the React app's shell/bottom-bar.tsx's texts
// and hooks/use-poll.ts.

// StatusAPI is what the bottom bar reads: SystemService.Memory and
// SettingsService.Providers, which the native app joins.
type StatusAPI interface {
	Memory(ctx context.Context) (sysmem.Reading, error)
	Providers(ctx context.Context) ([]app.ProviderStatus, error)
}

// BarEvery is how often the bottom bar reads the memory and the
// providers while the window shows.
var BarEvery = 2 * time.Second

// Bar is the bottom bar's readings. A failed read clears its value, so
// nothing stale stays on screen.
type Bar struct {
	Memory    *sysmem.Reading
	Providers []app.ProviderStatus
	// Visible tells whether the window shows; the bar reads nothing while
	// it doesn't. nil reads always.
	Visible func() bool

	api  StatusAPI
	post Post
	once sync.Once
	stop chan struct{}
}

// NewBar makes the bar's store; Start starts its reads.
func NewBar(api StatusAPI, post Post) *Bar {
	return &Bar{api: api, post: post, stop: make(chan struct{})}
}

// Start reads at once, then every BarEvery while the window shows, one
// read at a time, until Stop. It does nothing without a StatusAPI.
func (b *Bar) Start() {
	if b.api == nil {
		return
	}
	b.once.Do(func() {
		visible := b.Visible
		go func() {
			t := time.NewTicker(BarEvery)
			defer t.Stop()
			for {
				if visible == nil || visible() {
					b.read()
				}
				select {
				case <-t.C:
				case <-b.stop:
					return
				}
			}
		}()
	})
}

func (b *Bar) read() {
	m, merr := b.api.Memory(background)
	ps, perr := b.api.Providers(background)
	b.post(func() {
		b.Memory = nil
		if merr == nil {
			b.Memory = &m
		}
		b.Providers = nil
		if perr == nil {
			b.Providers = ps
		}
	})
}

// Stop ends the reads.
func (b *Bar) Stop() {
	select {
	case <-b.stop:
	default:
		close(b.stop)
	}
}

// Paused is the providers paused now (3.8).
func (b *Bar) Paused() []app.ProviderStatus {
	var out []app.ProviderStatus
	for _, p := range b.Providers {
		if p.PausedMS > 0 {
			out = append(out, p)
		}
	}
	return out
}

// WorkText is the project's work on the bar: "2 working · 1 waiting".
func WorkText(items []app.ChatItem) string {
	working, waiting := 0, 0
	for _, it := range items {
		switch it.State {
		case chat.StateWorking:
			working++
		case chat.StateWaiting:
			waiting++
		}
	}
	var parts []string
	if working > 0 {
		parts = append(parts, fmt.Sprintf("%d working", working))
	}
	if waiting > 0 {
		parts = append(parts, fmt.Sprintf("%d waiting", waiting))
	}
	return strings.Join(parts, " · ")
}

// WorkChat is the chat the work text opens: one that waits, else one that
// works; ok is false when none does.
func WorkChat(items []app.ChatItem) (app.ChatItem, bool) {
	for _, st := range []chat.State{chat.StateWaiting, chat.StateWorking} {
		for _, it := range items {
			if it.State == st {
				return it, true
			}
		}
	}
	return app.ChatItem{}, false
}

const gib = 1 << 30

// GB is bytes as gigabytes with one decimal.
func GB(bytes uint64) string { return fmt.Sprintf("%.1f", float64(bytes)/gib) }

func wholeGB(bytes uint64) int64 { return int64(math.Round(float64(bytes) / gib)) }

// MemoryText is what the OS can still hand out. No total: on Windows it
// is RAM plus the page file, which reads as the wrong RAM size.
func MemoryText(r sysmem.Reading) string {
	switch {
	case r.Free > 0:
		return "Memory " + GB(r.Free) + " GB free"
	case r.Level == sysmem.OK:
		return fmt.Sprintf("Memory %d GB", wholeGB(r.RAM))
	}
	return "Memory low"
}

// MemoryTip says what the free memory is out of: the RAM, and on Windows
// the page file too.
func MemoryTip(r sysmem.Reading) string {
	if r.Level == sysmem.Critical {
		return "Memory is almost full: new commands wait until some is free."
	}
	var paged uint64
	if r.Total > r.RAM {
		paged = r.Total - r.RAM
	}
	switch {
	case r.Free > 0 && paged >= 1<<29:
		return fmt.Sprintf("What the system can still hand out: %d GB RAM + %d GB page file", wholeGB(r.RAM), wholeGB(paged))
	case r.Free > 0:
		return fmt.Sprintf("Free memory, of %d GB RAM", wholeGB(r.RAM))
	}
	return fmt.Sprintf("%d GB RAM", wholeGB(r.RAM))
}

// ProblemText is a provider paused while it lasts (3.8).
func ProblemText(p app.ProviderStatus) string {
	return fmt.Sprintf("%s paused, retry in %d s", p.Name, int64(math.Ceil(float64(p.PausedMS)/1000)))
}
