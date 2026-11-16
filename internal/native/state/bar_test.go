package state

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/sysmem"
)

// The bottom bar's texts: the React app's shell/format.test.ts, ported.

func barItem(st chat.State) app.ChatItem { return app.ChatItem{State: st} }

func TestWorkText(t *testing.T) {
	same(t, WorkText([]app.ChatItem{barItem(chat.StateWorking), barItem(chat.StateWorking), barItem(chat.StateWaiting), barItem(chat.StateIdle)}), "2 working · 1 waiting")
	same(t, WorkText([]app.ChatItem{barItem(chat.StateWaiting)}), "1 waiting")
	same(t, WorkText([]app.ChatItem{barItem(chat.StateIdle)}), "")
	// Clicking opens the chat that waits, else one that works.
	w := barItem(chat.StateWaiting)
	w.Chat.ID = "w"
	it, ok := WorkChat([]app.ChatItem{barItem(chat.StateWorking), w})
	if !ok || it.Chat.ID != "w" {
		t.Fatalf("WorkChat = %+v %v", it, ok)
	}
	if _, ok := WorkChat([]app.ChatItem{barItem(chat.StateIdle)}); ok {
		t.Fatal("an idle chat is work")
	}
}

func TestMemoryText(t *testing.T) {
	g := func(v float64) uint64 { return uint64(v * gib) }
	win := sysmem.Reading{Free: g(5.4), Total: g(29), RAM: g(15.7), Level: sysmem.OK}
	same(t, MemoryText(win), "Memory 5.4 GB free")
	same(t, MemoryTip(win), "What the system can still hand out: 16 GB RAM + 13 GB page file")
	linux := sysmem.Reading{Free: g(2.1), Total: g(16), RAM: g(16), Level: sysmem.OK}
	same(t, MemoryTip(linux), "Free memory, of 16 GB RAM")
	crit := win
	crit.Free, crit.Level = g(0.3), sysmem.Critical
	if !strings.Contains(MemoryTip(crit), "almost full") {
		t.Errorf("critical tip = %q", MemoryTip(crit))
	}
	mac := sysmem.Reading{Total: g(16), RAM: g(16), Level: sysmem.OK}
	same(t, MemoryText(mac), "Memory 16 GB")
	low := mac
	low.Level = sysmem.Low
	same(t, MemoryText(low), "Memory low")
	same(t, MemoryTip(mac), "16 GB RAM")
}

func TestProblemText(t *testing.T) {
	same(t, ProblemText(app.ProviderStatus{Name: "Z.ai", PausedMS: 29_100}), "Z.ai paused, retry in 30 s")
}

// statusFake reads memory and providers, failing while fail is set.
type statusFake struct {
	reads atomic.Int32
	fail  atomic.Bool
}

func (f *statusFake) Memory(context.Context) (sysmem.Reading, error) {
	f.reads.Add(1)
	if f.fail.Load() {
		return sysmem.Reading{}, errors.New("no reading")
	}
	return sysmem.Reading{Free: 3 * gib, Level: sysmem.OK}, nil
}

func (f *statusFake) Providers(context.Context) ([]app.ProviderStatus, error) {
	return []app.ProviderStatus{{Name: "a"}, {Name: "b", PausedMS: 1000}}, nil
}

// The bar reads at once and then on a timer, only while the window
// shows; a failed read clears the value (use-poll.test.ts).
func TestTheBarReadsWhileTheWindowShows(t *testing.T) {
	defer func(d time.Duration) { BarEvery = d }(BarEvery)
	BarEvery = 5 * time.Millisecond
	l := newLoop()
	f := &statusFake{}
	b := NewBar(f, l.post)
	var shown atomic.Bool
	b.Visible = shown.Load
	b.Start()
	defer b.Stop()
	time.Sleep(30 * time.Millisecond)
	l.drain()
	if b.Memory != nil || f.reads.Load() != 0 {
		t.Fatal("the bar read while the window was hidden")
	}
	shown.Store(true)
	until(t, l, func() bool { return b.Memory != nil })
	same(t, b.Memory.Free, uint64(3*gib))
	same(t, len(b.Paused()), 1)
	f.fail.Store(true)
	until(t, l, func() bool { return b.Memory == nil })
}
