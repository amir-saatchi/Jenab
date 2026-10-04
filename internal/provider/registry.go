package provider

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"maps"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/secret"
)

// Stall timeouts (SPEC 3.8): the first event must come within FirstEvent
// (FirstEventOllama for a local Ollama, which queues requests and loads
// models), and then no more than Between may pass between two events.
// Starting values, tuned with the benchmark.
const (
	FirstEvent       = 2 * time.Minute
	FirstEventOllama = 10 * time.Minute
	Between          = 60 * time.Second
)

// raiseAfter is how many calls in a row must succeed before a halved
// background limit goes up by one (SPEC 3.8).
const raiseAfter = 20

// truncation check (SPEC 3.8, SPIKE-017): a reported input below half of
// Jenab's estimate is a truncation, for requests of at least minEstimate
// tokens.
const minEstimate = 2000

var (
	ErrUnknownModel    = errors.New("provider: unknown model")
	ErrUnknownProvider = errors.New("provider: no provider by that name")
)

// KeyName is the keychain name of a provider's key.
func KeyName(provider string) string { return "provider:" + provider }

// Deps are the Registry's dependencies.
type Deps struct {
	Settings config.LLMSettings
	Secrets  *secret.Store
	Gate     *limit.Gate // the global max_parallel_calls gate
	Backends map[Kind]Factory
	Catalog  *Catalog
	Log      *slog.Logger
}

// Registry picks the backend for a model and adds the shared rules: the
// global Gate and one Gate per provider, the pause per provider and its
// halved limit, the stall timeouts, the truncation check and redaction.
type Registry struct {
	d Deps

	mu       sync.Mutex
	settings config.LLMSettings
	conns    map[string]*conn
}

// conn is one provider's state.
type conn struct {
	name string
	set  config.ProviderSettings
	kind Kind

	mu      sync.Mutex
	backend Provider // built on first use
	gate    *limit.Gate
	limit   int // the configured background limit
	current int // the limit now, halved after a rate limit
	until   time.Time
	why     ErrorKind     // the kind of error that started the pause
	wake    chan struct{} // closed by Resume, for the calls waiting out the pause
	fails   int           // rate limits in a row, for the waits
	streak  int           // calls in a row that succeeded
	last    *Error
	reports map[string]ModelInfo // from the last Models call, by ID
}

// NewRegistry returns a Registry for the providers in d.Settings.
func NewRegistry(d Deps) *Registry {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.Catalog == nil {
		d.Catalog = MustCatalog()
	}
	r := &Registry{d: d, conns: map[string]*conn{}}
	r.Apply(d.Settings)
	return r
}

// Apply takes changed settings at once (SPEC 7.6). Providers whose
// connection changed are rebuilt at their next call; their pause stays.
func (r *Registry) Apply(s config.LLMSettings) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.settings = s
	for name, ps := range s.Providers {
		lim := s.MaxParallelCalls
		if n, ok := s.ProviderMaxParallelCalls[name]; ok {
			lim = n
		} else if Kind(ps.Kind) == KindOllama && localURL(ps.BaseURL) {
			lim = 1 // a local model answers one request at a time (SPIKE-017)
		}
		lim = max(lim, 1)
		c := r.conns[name]
		if c == nil {
			c = &conn{name: name, gate: limit.NewGate(lim), current: lim}
			r.conns[name] = c
		}
		c.mu.Lock()
		if c.set.Kind != ps.Kind || c.set.BaseURL != ps.BaseURL {
			c.backend = nil
		}
		c.set, c.kind = ps, Kind(ps.Kind)
		if c.current == c.limit || c.current > lim {
			c.current = lim // not halved, or above the new limit
		}
		c.limit = lim
		c.gate.SetSize(c.current)
		c.mu.Unlock()
	}
	for name := range r.conns {
		if _, ok := s.Providers[name]; !ok {
			delete(r.conns, name)
		}
	}
}

// Forget drops a provider's built backend, so the next call reads its key
// again, e.g. after the key changed.
func (r *Registry) Forget(name string) {
	if c := r.conn(name); c != nil {
		c.mu.Lock()
		c.backend = nil
		c.mu.Unlock()
	}
}

func (r *Registry) conn(name string) *conn {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.conns[name]
}

// Resolve turns an alias ("default", "fast") or "provider/model" into the
// provider's name and its model ID. Model IDs may hold slashes
// ("groq/openai/gpt-oss-120b").
func (r *Registry) Resolve(model string) (provider, id string, err error) {
	r.mu.Lock()
	if a, ok := r.settings.Models[model]; ok {
		model = a
	}
	r.mu.Unlock()
	provider, id, ok := strings.Cut(model, "/")
	if !ok || provider == "" || id == "" {
		return "", "", fmt.Errorf("%w: %q", ErrUnknownModel, model)
	}
	if r.conn(provider) == nil {
		return "", "", fmt.Errorf("%w: %q", ErrUnknownProvider, provider)
	}
	return provider, id, nil
}

// ContextWindow is the model's context window: from the settings, the
// catalog, or what the provider reported, in that order; 0 if unknown.
func (r *Registry) ContextWindow(provider, id string) int {
	c := r.conn(provider)
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, m := range c.set.Models {
		if m.ID == id && m.Context > 0 {
			return m.Context
		}
	}
	if m, ok := r.d.Catalog.Find(c.kind, id); ok {
		return m.Context
	}
	return c.reports[id].Context
}

// backendOf builds c's backend if needed.
func (r *Registry) backendOf(c *conn) (Provider, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.backend != nil {
		return c.backend, nil
	}
	f := r.d.Backends[c.kind]
	if f == nil {
		return nil, fmt.Errorf("provider %s: no backend for kind %q", c.name, c.kind)
	}
	conn := Connection{Name: c.name, Kind: c.kind, BaseURL: c.set.BaseURL}
	if conn.BaseURL == "" {
		conn.BaseURL = DefaultBaseURL(c.kind)
	}
	if len(Placeholders(conn.BaseURL)) > 0 {
		u, err := fill(conn.BaseURL, func(field string) (string, error) {
			if r.d.Secrets == nil {
				return "", secret.ErrNotFound
			}
			v, err := r.d.Secrets.Get(FieldName(c.name, field)) // Get makes Redact cover it
			return v.Reveal(), err
		})
		if err != nil {
			return nil, &Error{Kind: BadRequest, Provider: c.name, Message: err.Error(), Err: err}
		}
		conn.BaseURL = u
	}
	if r.d.Secrets != nil {
		conn.Redact = r.d.Secrets.Redact
		key, err := r.d.Secrets.Get(KeyName(c.name))
		switch {
		case err == nil:
			conn.Key = key
		case errors.Is(err, secret.ErrNotFound) && c.kind == KindOllama:
			// a local Ollama needs no key
		default:
			return nil, &Error{Kind: BadRequest, Provider: c.name, Message: "no key is stored for this provider", Err: err}
		}
	}
	b, err := f(conn)
	if err != nil {
		return nil, fmt.Errorf("provider %s: %w", c.name, err)
	}
	c.backend = b
	return b, nil
}

// Stream sends req to the model named in req.Model and yields its events.
//
// Background calls wait for their provider's slot, then for a global one.
// Interactive calls never wait for a slot (SPEC 7.6). Every call waits
// while its provider is paused after a rate limit, and gets an EventWait
// first so the chat can show the wait (SPEC 8.3).
//
// Stream does not retry. A rate limit or overload pauses the provider, and
// the error's RetryAfter says when the pause ends.
func (r *Registry) Stream(ctx context.Context, p limit.Priority, req Request) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		name, id, err := r.Resolve(req.Model)
		if err != nil {
			yield(Event{}, err)
			return
		}
		c := r.conn(name)
		b, err := r.backendOf(c)
		if err != nil {
			yield(Event{}, err)
			return
		}
		req.Model = id
		if m, ok := r.d.Catalog.Find(c.kind, id); ok {
			req.Known = &m
		}
		if req.Context == 0 {
			req.Context = r.ContextWindow(name, id)
		}
		release, err := r.admit(ctx, c, p, yield)
		if err != nil {
			if err != errStopped {
				yield(Event{}, err)
			}
			return
		}
		defer release()
		r.run(ctx, c, b, req, yield)
	}
}

var errStopped = errors.New("the caller stopped reading")

// admit waits out the provider's pause and takes the slots. A pause that
// starts while the call waits for a slot is waited out too.
func (r *Registry) admit(ctx context.Context, c *conn, p limit.Priority, yield func(Event, error) bool) (func(), error) {
	waited := false
	for {
		if wait, why, wake := c.pause(); wait > 0 {
			waited = true
			if !yield(Event{Kind: EventWait, Wait: wait, Paused: why}, nil) {
				return nil, errStopped
			}
			t := time.NewTimer(wait)
			select {
			case <-t.C:
			case <-wake:
				t.Stop()
			case <-ctx.Done():
				t.Stop()
				return nil, ctx.Err()
			}
		}
		// The provider's slot first, so a background call never holds a
		// global slot while its provider is busy (SPEC 7.6).
		relP, err := c.gate.Acquire(ctx, p)
		if err != nil {
			return nil, err
		}
		relG := func() {}
		if r.d.Gate != nil {
			if relG, err = r.d.Gate.Acquire(ctx, p); err != nil {
				relP()
				return nil, err
			}
		}
		release := func() { relG(); relP() }
		if wait, _, _ := c.pause(); wait <= 0 {
			if waited && !yield(Event{Kind: EventWait}, nil) { // the wait is over
				release()
				return nil, errStopped
			}
			return release, nil
		}
		release()
	}
}

// run streams one request with the stall timeouts and the end checks.
func (r *Registry) run(ctx context.Context, c *conn, b Provider, req Request, yield func(Event, error) bool) {
	ictx, cancel := context.WithCancel(ctx)
	defer cancel()
	first := FirstEvent
	if c.kind == KindOllama {
		first = FirstEventOllama
	}
	// The watchdog cancels the request when the provider is silent too
	// long. It is stopped while the caller handles an event, so a slow
	// caller never looks like a stalled provider.
	var mu sync.Mutex
	window, stalled := first, false
	timer := time.AfterFunc(first, func() {
		mu.Lock()
		stalled = true
		mu.Unlock()
		cancel()
	})
	defer timer.Stop()
	stallErr := func(err error) error {
		mu.Lock()
		defer mu.Unlock()
		if !stalled {
			return nil
		}
		return TransportError(c.name, fmt.Sprintf("the stream stalled: no event for %s", window), err)
	}

	for ev, err := range b.Stream(ictx, req) {
		timer.Stop()
		if err != nil {
			if serr := stallErr(err); serr != nil {
				err = serr
			} else if ctx.Err() != nil {
				err = ctx.Err() // the caller cancelled: not the provider's fault
			}
			yield(Event{}, r.failed(c, err))
			return
		}
		if ev.Kind == EventDone {
			if terr := truncated(c.name, req, ev.Usage); terr != nil {
				yield(Event{}, r.failed(c, terr))
				return
			}
			c.succeeded()
			yield(ev, nil)
			return
		}
		if !yield(ev, nil) {
			return
		}
		mu.Lock()
		window = Between
		mu.Unlock()
		timer.Reset(Between)
	}
	if serr := stallErr(ErrCutOff); serr != nil {
		yield(Event{}, r.failed(c, serr))
		return
	}
	if ctx.Err() != nil {
		yield(Event{}, ctx.Err())
		return
	}
	yield(Event{}, r.failed(c, TransportError(c.name, "the stream ended without a done event", ErrCutOff)))
}

// failed records err against c and returns it, redacted, as an *Error
// where it is one.
func (r *Registry) failed(c *conn, err error) error {
	var pe *Error
	if !errors.As(err, &pe) {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		pe = TransportError(c.name, err.Error(), err)
	}
	pe.Provider = c.name
	if r.d.Secrets != nil {
		pe.Message = r.d.Secrets.Redact(pe.Message)
	}
	switch pe.Kind {
	case RateLimited, Overloaded:
		pe.RetryAfter = c.paused(pe.Kind, pe.RetryAfter)
	}
	c.mu.Lock()
	c.streak = 0
	c.last = pe
	c.mu.Unlock()
	r.d.Log.Warn("provider: call failed", "provider", c.name, "kind", pe.Kind, "status", pe.Status, "retry_after", pe.RetryAfter, "err", pe.Message)
	return pe
}

// paused starts or extends the provider's pause after a rate limit or an
// overload, and halves its background limit (at least 1). It returns the
// pause left.
func (c *conn) paused(why ErrorKind, wait time.Duration) time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	if wait <= 0 {
		wait = Backoff(c.fails)
	}
	c.fails++
	now := time.Now()
	if !now.Before(c.until) {
		// A new pause halves the limit once, however many calls the
		// provider turns away during it.
		c.current = max(1, c.current/2)
		c.gate.SetSize(c.current)
	}
	if until := now.Add(wait); until.After(c.until) {
		c.until, c.why = until, why
	}
	c.streak = 0
	return time.Until(c.until)
}

// Resume ends a provider's pause, for *Retry now* (SPEC 8.3): the user
// chose to try at once. Every call waiting out the pause goes now, from
// any chat or pipeline. Another rate limit starts a new pause.
func (r *Registry) Resume(provider string) {
	if c := r.conn(provider); c != nil {
		c.mu.Lock()
		c.until = time.Time{}
		if c.wake != nil {
			close(c.wake)
			c.wake = nil
		}
		c.mu.Unlock()
	}
}

// pause is the pause left (0 if none), why it started, and a channel
// Resume closes.
func (c *conn) pause() (time.Duration, ErrorKind, <-chan struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	wait := max(time.Until(c.until), 0)
	if wait == 0 {
		return 0, "", nil
	}
	if c.wake == nil {
		c.wake = make(chan struct{})
	}
	return wait, c.why, c.wake
}

// succeeded counts a complete answer; after raiseAfter in a row a halved
// limit goes up by one.
func (c *conn) succeeded() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fails = 0
	c.last = nil
	c.streak++
	if c.current < c.limit && c.streak >= raiseAfter {
		c.current++
		c.gate.SetSize(c.current)
		c.streak = 0
	}
}

// truncated reports a silent truncation: the provider read far fewer input
// tokens than the request holds (SPEC 3.8, SPIKE-017).
func truncated(provider string, req Request, u *chat.Usage) *Error {
	if u == nil {
		return nil
	}
	got := u.Input + u.CacheRead + u.CacheWrite
	est := EstimateTokens(req)
	if got == 0 || est < minEstimate || got*2 >= est {
		return nil
	}
	return &Error{Kind: TooLarge, Provider: provider,
		Message: fmt.Sprintf("the provider read %d input tokens of about %d; the rest was dropped without an error", got, est)}
}

// EstimateTokens is a rough count of a request's input tokens: 4 bytes a
// token over every text the model reads. Images are not counted.
func EstimateTokens(req Request) int {
	n := 0
	for _, b := range req.System {
		n += len(b.Text)
	}
	for _, t := range req.Tools {
		n += len(t.Name) + len(t.Description) + len(t.Schema)
	}
	for _, m := range req.Messages {
		for _, p := range m.Parts {
			switch {
			case p.Text != nil:
				n += len(p.Text.Text)
			case p.Thinking != nil:
				n += len(p.Thinking.Text)
			case p.ToolCall != nil:
				n += len(p.ToolCall.Name) + len(p.ToolCall.Args)
			case p.ToolResult != nil:
				n += len(p.ToolResult.Text)
			case p.Notice != nil:
				n += len(p.Notice.Text)
			}
		}
	}
	return n / 4
}

// Status is a provider's state for the bottom bar and the runtime panel.
type Status struct {
	Name        string
	Kind        Kind
	PausedFor   time.Duration // 0 if not paused
	Limit       int           // the configured background limit
	Current     int           // the limit now
	Calls       limit.GateStats
	LastProblem string // the last error's kind and message, "" after a success
}

// Status lists every provider, by name.
func (r *Registry) Status() []Status {
	r.mu.Lock()
	cs := slices.Sorted(maps.Keys(r.conns))
	conns := make([]*conn, len(cs))
	for i, n := range cs {
		conns[i] = r.conns[n]
	}
	r.mu.Unlock()
	out := make([]Status, 0, len(conns))
	for _, c := range conns {
		c.mu.Lock()
		s := Status{Name: c.name, Kind: c.kind, PausedFor: max(time.Until(c.until), 0), Limit: c.limit, Current: c.current, Calls: c.gate.Stats()}
		if c.last != nil {
			s.LastProblem = c.last.Error()
		}
		c.mu.Unlock()
		out = append(out, s)
	}
	return out
}

// Models asks one provider for its models and remembers what it reports
// (context windows, for ContextWindow).
func (r *Registry) Models(ctx context.Context, provider string) ([]ModelInfo, error) {
	c := r.conn(provider)
	if c == nil {
		return nil, fmt.Errorf("%w: %q", ErrUnknownProvider, provider)
	}
	b, err := r.backendOf(c)
	if err != nil {
		return nil, err
	}
	ms, err := b.Models(ctx)
	if err != nil {
		return nil, r.failed(c, err)
	}
	c.mu.Lock()
	c.reports = map[string]ModelInfo{}
	for _, m := range ms {
		c.reports[m.ID] = m
	}
	c.mu.Unlock()
	return ms, nil
}

// Connected is what Connect found.
type Connected struct {
	// Settings are the provider's settings with its models turned on: the
	// catalog models the key can use (Anthropic, OpenAI, Gemini), or every
	// listed model (OpenAI-compatible, Ollama). The caller saves them and
	// calls Apply.
	Settings config.ProviderSettings
	// Other are listed models the catalog doesn't know, off by default
	// (*Other models*). Turning one on needs its context window.
	Other []ModelInfo
	// NoModelList: the provider has no model list (Cloudflare Workers AI's
	// OpenAI-compatible endpoint), so the user adds model IDs and their
	// context windows by hand. The key is checked at the first call.
	NoModelList bool
}

// Connect tries a new connection (SPEC 3.9): it lists the models with key,
// and only if that works stores key in the keychain. An empty key is
// allowed for a local Ollama. baseURL "" means the kind's default; fields
// are the values for its placeholders, stored in the keychain with the key.
func (r *Registry) Connect(ctx context.Context, name string, kind Kind, baseURL, key string, fields map[string]string) (Connected, error) {
	if name == "" || strings.ContainsAny(name, "/: ") {
		return Connected{}, fmt.Errorf("provider: %q is not a usable name", name)
	}
	f := r.d.Backends[kind]
	if f == nil {
		return Connected{}, fmt.Errorf("provider: no backend for kind %q", kind)
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL(kind)
	}
	if baseURL == "" {
		return Connected{}, fmt.Errorf("provider: %s needs a base URL", kind)
	}
	var hide []secret.Value // the field values, redacted like the key
	for _, f := range Placeholders(baseURL) {
		hide = append(hide, secret.NewValue(FieldName(name, f), fields[f]))
	}
	resolved, err := fill(baseURL, func(field string) (string, error) {
		if v := fields[field]; v != "" {
			return v, nil
		}
		return "", secret.ErrNotFound
	})
	if err != nil {
		return Connected{}, fmt.Errorf("provider %s: %w", name, err)
	}
	if _, err := CheckBaseURL(resolved); err != nil {
		return Connected{}, errors.New(redactAll(err.Error(), hide))
	}
	conn := Connection{Name: name, Kind: kind, BaseURL: resolved, Key: secret.NewValue(KeyName(name), key)}
	conn.Redact = func(s string) string {
		if r.d.Secrets != nil {
			s = r.d.Secrets.Redact(s)
		}
		return redactAll(s, hide)
	}
	b, err := f(conn)
	if err != nil {
		return Connected{}, fmt.Errorf("provider %s: %w", name, err)
	}
	out := Connected{Settings: config.ProviderSettings{Kind: string(kind)}}
	listed, err := b.Models(ctx)
	var pe *Error
	switch {
	case err == nil:
	case errors.As(err, &pe) && (pe.Status == 404 || pe.Status == 405):
		out.NoModelList = true
	default:
		if pe != nil {
			pe.Provider = name
			pe.Message = conn.Redact(conn.Key.Redact(pe.Message))
		}
		return Connected{}, err
	}
	if r.d.Secrets != nil {
		if key != "" {
			if err := r.d.Secrets.Set(KeyName(name), key); err != nil {
				return Connected{}, fmt.Errorf("provider %s: storing the key: %w", name, err)
			}
		}
		for _, f := range Placeholders(baseURL) {
			if err := r.d.Secrets.Set(FieldName(name, f), fields[f]); err != nil {
				return Connected{}, fmt.Errorf("provider %s: storing the %s: %w", name, f, err)
			}
		}
	}

	if baseURL != DefaultBaseURL(kind) {
		out.Settings.BaseURL = baseURL
	}
	known := r.d.Catalog.Models(kind)
	if len(known) == 0 {
		for _, m := range listed {
			out.Settings.Models = append(out.Settings.Models, config.ModelSettings{ID: m.ID, Context: m.Context})
		}
		return out, nil
	}
	have := map[string]bool{}
	for _, m := range listed {
		have[m.ID] = true
		if _, ok := r.d.Catalog.Find(kind, m.ID); !ok {
			out.Other = append(out.Other, m)
		}
	}
	for _, m := range known {
		if have[m.ID] {
			out.Settings.Models = append(out.Settings.Models, config.ModelSettings{ID: m.ID})
		}
	}
	return out, nil
}

func redactAll(s string, vs []secret.Value) string {
	for _, v := range vs {
		s = v.Redact(s)
	}
	return s
}

// localURL tells whether a base URL is this machine; "" is a kind's
// default, which for Ollama is localhost.
func localURL(baseURL string) bool {
	if baseURL == "" {
		return true
	}
	u, err := url.Parse(baseURL)
	return err == nil && isLoopback(u.Hostname())
}
