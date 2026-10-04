package provider_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
	"github.com/amir-saatchi/jenab/internal/secret"
)

// memKeyring is a keychain in memory.
type memKeyring struct {
	mu sync.Mutex
	m  map[string]string
}

func (k *memKeyring) Get(name string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	v, ok := k.m[name]
	if !ok {
		return "", secret.ErrNotFound
	}
	return v, nil
}

func (k *memKeyring) Set(name, v string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.m[name] = v
	return nil
}

func (k *memKeyring) Delete(name string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.m, name)
	return nil
}

const testKey = "sk-test-0123456789abcdef"

type setup struct {
	reg  *provider.Registry
	fp   *fake.Provider
	gate *limit.Gate
}

// newSetup makes a Registry with one fake provider "p" of kind kind and a
// background limit of perProvider.
func newSetup(t *testing.T, kind provider.Kind, perProvider int, replies ...fake.Reply) setup {
	t.Helper()
	fp := fake.New(replies...)
	sec := secret.New(&memKeyring{m: map[string]string{provider.KeyName("p"): testKey}})
	gate := limit.NewGate(4)
	reg := provider.NewRegistry(provider.Deps{
		Settings: config.LLMSettings{
			MaxParallelCalls:         4,
			ProviderMaxParallelCalls: map[string]int{"p": perProvider},
			Models:                   map[string]string{"default": "p/m1"},
			Providers: map[string]config.ProviderSettings{
				"p": {Kind: string(kind), BaseURL: "https://example.test/v1/", Models: []config.ModelSettings{{ID: "m1", Context: 32000}}},
			},
		},
		Secrets:  sec,
		Gate:     gate,
		Backends: map[provider.Kind]provider.Factory{kind: fp.Factory()},
	})
	return setup{reg: reg, fp: fp, gate: gate}
}

// collect reads a whole stream.
func collect(reg *provider.Registry, p limit.Priority, req provider.Request) ([]provider.Event, error) {
	var evs []provider.Event
	for ev, err := range reg.Stream(context.Background(), p, req) {
		if err != nil {
			return evs, err
		}
		evs = append(evs, ev)
	}
	return evs, nil
}

func kindOf(err error) provider.ErrorKind {
	var pe *provider.Error
	if errors.As(err, &pe) {
		return pe.Kind
	}
	return ""
}

func status(t *testing.T, reg *provider.Registry) provider.Status {
	t.Helper()
	st := reg.Status()
	if len(st) != 1 {
		t.Fatalf("Status: %d providers, want 1", len(st))
	}
	return st[0]
}

func rateLimited(wait time.Duration) error {
	return &provider.Error{Kind: provider.RateLimited, Status: 429, RetryAfter: wait, Message: "too many requests"}
}

func TestStreamComplete(t *testing.T) {
	s := newSetup(t, provider.KindCompatible, 2, fake.Text("hello"))
	evs, err := collect(s.reg, limit.Interactive, provider.Request{Model: "default"})
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 3 || evs[2].Kind != provider.EventDone || evs[1].Part.Text.Text != "hello" {
		t.Fatalf("events = %+v", evs)
	}
	calls := s.fp.Calls()
	if calls[0].Model != "m1" || calls[0].Context != 32000 {
		t.Errorf("backend got model %q context %d, want m1 and 32000", calls[0].Model, calls[0].Context)
	}
	for _, bad := range []string{"m1", "q/m1", "", "p/"} {
		if _, err := collect(s.reg, limit.Interactive, provider.Request{Model: bad}); err == nil {
			t.Errorf("model %q: no error", bad)
		}
	}
}

func TestModelIDWithSlash(t *testing.T) {
	s := newSetup(t, provider.KindCompatible, 2)
	if _, err := collect(s.reg, limit.Interactive, provider.Request{Model: "p/openai/gpt-oss-120b"}); err != nil {
		t.Fatal(err)
	}
	if got := s.fp.Calls()[0].Model; got != "openai/gpt-oss-120b" {
		t.Errorf("model = %q", got)
	}
}

func TestCutOffStream(t *testing.T) {
	s := newSetup(t, provider.KindCompatible, 2, fake.Reply{Events: []provider.Event{{Kind: provider.EventDelta, Text: "par"}}})
	_, err := collect(s.reg, limit.Interactive, provider.Request{Model: "p/m1"})
	if kindOf(err) != provider.Transport || !errors.Is(err, provider.ErrCutOff) {
		t.Fatalf("err = %v, want a transport cut-off", err)
	}
}

func TestRateLimitPausesProvider(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newSetup(t, provider.KindCompatible, 4, fake.Fail(rateLimited(30*time.Second)))
		start := time.Now()
		_, err := collect(s.reg, limit.Background, provider.Request{Model: "p/m1"})
		var pe *provider.Error
		if !errors.As(err, &pe) || pe.Kind != provider.RateLimited || pe.RetryAfter != 30*time.Second {
			t.Fatalf("err = %v, want rate_limited with a 30 s wait", err)
		}
		if got := status(t, s.reg).PausedFor; got != 30*time.Second {
			t.Errorf("PausedFor = %s", got)
		}

		// Every call to the provider waits, chat calls too, and is told so.
		var wg sync.WaitGroup
		for _, p := range []limit.Priority{limit.Interactive, limit.Background} {
			wg.Go(func() {
				evs, err := collect(s.reg, p, provider.Request{Model: "p/m1"})
				if err != nil {
					t.Errorf("after the pause: %v", err)
					return
				}
				if len(evs) < 2 || evs[0].Kind != provider.EventWait || evs[0].Wait != 30*time.Second || evs[0].Paused != provider.RateLimited {
					t.Fatalf("events %+v, want a 30 s wait for a rate limit first", evs)
				}
				if evs[1].Kind != provider.EventWait || evs[1].Wait != 0 {
					t.Errorf("second event = %+v, want the end of the wait", evs[1])
				}
				if d := time.Since(start); d < 30*time.Second {
					t.Errorf("sent after %s, before the pause ended", d)
				}
			})
		}
		synctest.Wait()
		if n := len(s.fp.Calls()); n != 1 {
			t.Errorf("%d calls reached the provider during the pause, want only the first", n-1)
		}
		wg.Wait()
		if n := len(s.fp.Calls()); n != 3 {
			t.Errorf("%d calls, want 3", n)
		}
	})
}

func TestResumeEndsThePause(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newSetup(t, provider.KindCompatible, 4, fake.Fail(rateLimited(30*time.Second)))
		start := time.Now()
		collect(s.reg, limit.Interactive, provider.Request{Model: "p/m1"})
		s.reg.Resume("p")
		s.reg.Resume("unknown") // no such provider: nothing happens
		if got := status(t, s.reg).PausedFor; got != 0 {
			t.Errorf("PausedFor = %s after Resume", got)
		}
		evs, err := collect(s.reg, limit.Interactive, provider.Request{Model: "p/m1"})
		if err != nil || len(evs) == 0 || evs[0].Kind == provider.EventWait || time.Since(start) != 0 {
			t.Errorf("after Resume: %+v, %v, %s later; want sent at once", evs, err, time.Since(start))
		}
	})
}

func TestResumeWakesWaitingCalls(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newSetup(t, provider.KindCompatible, 4, fake.Fail(&provider.Error{Kind: provider.Overloaded, Status: 529, RetryAfter: time.Minute}))
		collect(s.reg, limit.Interactive, provider.Request{Model: "p/m1"})
		start := time.Now()
		done := make(chan []provider.Event)
		go func() {
			evs, _ := collect(s.reg, limit.Background, provider.Request{Model: "p/m1"})
			done <- evs
		}()
		synctest.Wait() // the call waits out the pause
		s.reg.Resume("p")
		evs := <-done
		if time.Since(start) != 0 {
			t.Errorf("the waiting call went after %s, want at once", time.Since(start))
		}
		if len(evs) < 2 || evs[0].Paused != provider.Overloaded || evs[1].Kind != provider.EventWait || evs[1].Wait != 0 {
			t.Errorf("events %+v, want a wait for an overload, then its end", evs)
		}
	})
}

func TestQuotaIsNotPaused(t *testing.T) {
	s := newSetup(t, provider.KindCompatible, 4, fake.Fail(provider.Classify("x", 429, nil, `{"error":{"message":"Quota exceeded for metric: GenerateRequestsPerDayPerProjectPerModel-FreeTier"}}`)))
	_, err := collect(s.reg, limit.Background, provider.Request{Model: "p/m1"})
	var pe *provider.Error
	if !errors.As(err, &pe) || pe.Kind != provider.Quota || pe.Retryable() {
		t.Fatalf("err = %v, want a quota error that is not retryable", err)
	}
	if pe.Provider != "p" {
		t.Errorf("Provider = %q, want p", pe.Provider)
	}
	st := status(t, s.reg)
	if st.PausedFor != 0 || st.Current != 4 || !strings.Contains(st.LastProblem, "quota") {
		t.Errorf("status = %+v, want no pause and the quota shown", st)
	}
	if len(s.fp.Calls()) != 1 {
		t.Errorf("the provider got %d calls, want 1: the registry never retries", len(s.fp.Calls()))
	}
}

func TestLimitHalvesAndComesBack(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Two calls in flight both get a 429: one pause, halved once.
		inFlight := fake.Reply{Wait: time.Second, Events: []provider.Event{{Kind: provider.EventDelta, Text: "x"}}, Err: rateLimited(time.Second)}
		s := newSetup(t, provider.KindCompatible, 8, inFlight, inFlight)
		var wg sync.WaitGroup
		for range 2 {
			wg.Go(func() { collect(s.reg, limit.Background, provider.Request{Model: "p/m1"}) })
		}
		wg.Wait()
		if st := status(t, s.reg); st.Current != 4 || st.Calls.Size != 4 {
			t.Fatalf("after one pause: %+v, want 4", st)
		}
		time.Sleep(2 * time.Second)
		s.fp.Push(fake.Fail(rateLimited(time.Second)))
		collect(s.reg, limit.Background, provider.Request{Model: "p/m1"})
		if st := status(t, s.reg); st.Current != 2 {
			t.Fatalf("after a second pause: Current = %d, want 2", st.Current)
		}
		time.Sleep(2 * time.Second)
		for want := 3; want <= 8; want++ {
			for range 19 {
				collect(s.reg, limit.Background, provider.Request{Model: "p/m1"})
			}
			if st := status(t, s.reg); st.Current != want-1 {
				t.Fatalf("after 19 more: Current = %d, want %d", st.Current, want-1)
			}
			collect(s.reg, limit.Background, provider.Request{Model: "p/m1"})
			if st := status(t, s.reg); st.Current != want || st.Calls.Size != want {
				t.Fatalf("after 20 in a row: %+v, want %d", st, want)
			}
			if want == 4 {
				break // the rest is the same
			}
		}
	})
}

func TestStallTimeouts(t *testing.T) {
	tests := []struct {
		name  string
		kind  provider.Kind
		reply fake.Reply
		want  time.Duration
	}{
		{"no first event", provider.KindCompatible, fake.Reply{Hang: true}, provider.FirstEvent},
		{"no first event, Ollama", provider.KindOllama, fake.Reply{Hang: true}, provider.FirstEventOllama},
		{"silent mid-stream", provider.KindCompatible, fake.Reply{Wait: 50 * time.Second, Events: []provider.Event{
			{Kind: provider.EventDelta, Text: "a"}, {Kind: provider.EventDelta, Text: "b"}}, Hang: true}, 100*time.Second + provider.Between},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s := newSetup(t, tt.kind, 1, tt.reply)
				start := time.Now()
				_, err := collect(s.reg, limit.Interactive, provider.Request{Model: "p/m1"})
				if kindOf(err) != provider.Transport || !strings.Contains(err.Error(), "stalled") {
					t.Fatalf("err = %v, want a stall", err)
				}
				if d := time.Since(start); d != tt.want {
					t.Errorf("failed after %s, want %s", d, tt.want)
				}
			})
		})
	}
}

func TestSlowCallerIsNotAStall(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newSetup(t, provider.KindCompatible, 1, fake.Text("hi"))
		var err error
		for _, e := range s.reg.Stream(context.Background(), limit.Interactive, provider.Request{Model: "p/m1"}) {
			if e != nil {
				err = e
			}
			time.Sleep(5 * time.Minute) // e.g. a slow disk
		}
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestCallerCancelIsNotAProviderError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newSetup(t, provider.KindCompatible, 1, fake.Reply{Hang: true})
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var err error
		for _, e := range s.reg.Stream(ctx, limit.Interactive, provider.Request{Model: "p/m1"}) {
			err = e
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v, want the caller's deadline", err)
		}
		if st := status(t, s.reg); st.PausedFor != 0 {
			t.Errorf("paused after the caller's cancel: %+v", st)
		}
	})
}

func TestSilentTruncation(t *testing.T) {
	big := strings.Repeat("BTC closed higher today. ", 2000) // 50,000 bytes: about 12,500 tokens
	req := provider.Request{Model: "p/m1", Messages: []chat.Message{{Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: big}}}}}}
	for _, tt := range []struct {
		usage chat.Usage
		ok    bool
	}{
		{chat.Usage{Input: 4096}, false},                  // Ollama's default context
		{chat.Usage{Input: 1000, CacheRead: 10000}, true}, // cached tokens count
		{chat.Usage{Input: 7000}, true},
	} {
		s := newSetup(t, provider.KindCompatible, 1, fake.Reply{Events: []provider.Event{fake.Done(provider.StopEnd, tt.usage)}})
		_, err := collect(s.reg, limit.Interactive, req)
		if tt.ok != (err == nil) || (!tt.ok && kindOf(err) != provider.TooLarge) {
			t.Errorf("usage %+v: err = %v", tt.usage, err)
		}
	}
}

func TestBackgroundTakesProviderSlotFirst(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newSetup(t, provider.KindCompatible, 1,
			fake.Reply{Wait: time.Minute, Events: []provider.Event{fake.Done(provider.StopEnd, chat.Usage{Input: 1})}})
		var wg sync.WaitGroup
		for range 3 {
			wg.Go(func() {
				if _, err := collect(s.reg, limit.Background, provider.Request{Model: "p/m1"}); err != nil {
					t.Error(err)
				}
			})
		}
		synctest.Wait()
		if g := s.gate.Stats(); g.InUse != 1 || g.Waiting != 0 {
			t.Errorf("global gate = %+v: calls waiting for their provider must not hold or wait for a global slot", g)
		}
		if p := status(t, s.reg).Calls; p.InUse != 1 || p.Waiting != 2 {
			t.Errorf("provider gate = %+v, want 1 in use and 2 waiting", p)
		}
		// A chat call never waits.
		if _, err := collect(s.reg, limit.Interactive, provider.Request{Model: "p/m1"}); err != nil {
			t.Fatal(err)
		}
		wg.Wait()
	})
}

func TestMissingKey(t *testing.T) {
	s := newSetup(t, provider.KindCompatible, 1)
	s.reg.Apply(config.LLMSettings{Providers: map[string]config.ProviderSettings{"other": {Kind: "openai_compatible", BaseURL: "https://example.test/"}}})
	_, err := collect(s.reg, limit.Interactive, provider.Request{Model: "other/m"})
	if kindOf(err) != provider.BadRequest {
		t.Fatalf("err = %v, want a request error for the missing key", err)
	}
}
