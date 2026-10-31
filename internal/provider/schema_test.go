package provider_test

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
)

var testSchema = json.RawMessage(`{"type":"object","properties":{"a":{"type":"string"}},"required":["a"]}`)

func badRequest(status int) error {
	return &provider.Error{Kind: provider.BadRequest, Status: status, Message: "response_format is not supported"}
}

// schemas lists, per call the backend got, whether it carried a schema.
func asError(err error, pe **provider.Error) bool { return errors.As(err, pe) }

func schemas(fp *fake.Provider) []bool {
	var out []bool
	for _, c := range fp.Calls() {
		out = append(out, c.Schema != nil)
	}
	return out
}

func equal(a, b []bool) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSchemaRejectedIsDropped(t *testing.T) {
	s := newSetup(t, provider.KindCompatible, 2, fake.Fail(badRequest(400)))
	req := provider.Request{Model: "default", Schema: testSchema}
	evs, err := collect(s.reg, limit.Background, req)
	if err != nil {
		t.Fatalf("the request without the schema should succeed: %v", err)
	}
	if len(evs) == 0 || evs[len(evs)-1].Kind != provider.EventDone {
		t.Fatalf("events = %+v", evs)
	}
	calls := s.fp.Calls()
	if got := schemas(s.fp); !equal(got, []bool{true, false}) {
		t.Fatalf("schema per call = %v, want [true false]", got)
	}
	// The resend is the same request without the schema.
	a, b := calls[0], calls[1]
	a.Schema = nil
	if !reflect.DeepEqual(a, b) {
		t.Errorf("the resend differs beyond the schema:\n%+v\n%+v", a, b)
	}
	if p := status(t, s.reg).LastProblem; p != "" {
		t.Errorf("the rejected schema was recorded as the provider's problem: %q", p)
	}

	// Remembered for the model: one call, no schema.
	if _, err := collect(s.reg, limit.Background, req); err != nil {
		t.Fatal(err)
	}
	if got := schemas(s.fp); !equal(got, []bool{true, false, false}) {
		t.Fatalf("schema per call = %v, want the third without", got)
	}
	// Not for another model, nor for the same model with tools.
	if _, err := collect(s.reg, limit.Background, provider.Request{Model: "p/m2", Schema: testSchema}); err != nil {
		t.Fatal(err)
	}
	tools := provider.Request{Model: "default", Schema: testSchema, Tools: []provider.ToolDef{{Name: "t", Schema: json.RawMessage(`{"type":"object"}`)}}}
	if _, err := collect(s.reg, limit.Background, tools); err != nil {
		t.Fatal(err)
	}
	if got := schemas(s.fp); !equal(got, []bool{true, false, false, true, true}) {
		t.Fatalf("schema per call = %v, want another model and tools to keep it", got)
	}
}

func TestSchemaKeptOnOtherErrors(t *testing.T) {
	cases := []struct {
		name    string
		replies []fake.Reply
		calls   int
		kind    provider.ErrorKind
	}{
		{"401", []fake.Reply{fake.Fail(badRequest(401))}, 1, provider.BadRequest},
		{"404", []fake.Reply{fake.Fail(badRequest(404))}, 1, provider.BadRequest},
		{"rate limit", []fake.Reply{fake.Fail(rateLimited(0))}, 1, provider.RateLimited},
		{"transport", []fake.Reply{fake.Fail(provider.TransportError("p", "lost", nil))}, 1, provider.Transport},
		{"too large", []fake.Reply{fake.Fail(&provider.Error{Kind: provider.TooLarge, Status: 400})}, 1, provider.TooLarge},
		// A 400 that comes back without the schema too wasn't the schema.
		{"400 twice", []fake.Reply{fake.Fail(badRequest(400)), fake.Fail(badRequest(400))}, 2, provider.BadRequest},
		// The resend failed before the API took it: nothing is known.
		{"400 then overload", []fake.Reply{fake.Fail(badRequest(400)), fake.Fail(&provider.Error{Kind: provider.Overloaded, Status: 503})}, 2, provider.Overloaded},
		// A 400 after the answer started is not about the request.
		{"400 mid-stream", []fake.Reply{{Events: fake.Text("x").Events[:1], Err: badRequest(400)}}, 1, provider.BadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newSetup(t, provider.KindCompatible, 2, c.replies...)
			req := provider.Request{Model: "default", Schema: testSchema}
			_, err := collect(s.reg, limit.Background, req)
			if kindOf(err) != c.kind {
				t.Fatalf("error %v, want kind %s", err, c.kind)
			}
			if n := len(s.fp.Calls()); n != c.calls {
				t.Fatalf("%d calls, want %d", n, c.calls)
			}
			s.fp.Replace()    // "ok" from now on
			s.reg.Resume("p") // no wait after a rate limit or overload
			if _, err := collect(s.reg, limit.Background, req); err != nil {
				t.Fatal(err)
			}
			if calls := s.fp.Calls(); calls[len(calls)-1].Schema == nil {
				t.Error("the schema was dropped after an error that isn't about it")
			}
		})
	}
	t.Run("no schema", func(t *testing.T) {
		s := newSetup(t, provider.KindCompatible, 2, fake.Fail(badRequest(400)))
		if _, err := collect(s.reg, limit.Background, provider.Request{Model: "default"}); kindOf(err) != provider.BadRequest {
			t.Fatalf("error %v", err)
		}
		if n := len(s.fp.Calls()); n != 1 {
			t.Fatalf("%d calls, want 1", n)
		}
	})
}

func TestSchemaKeptWhenCancelled(t *testing.T) {
	s := newSetup(t, provider.KindCompatible, 2, fake.Reply{Hang: true})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(50*time.Millisecond, cancel)
	var err error
	for _, e := range s.reg.Stream(ctx, limit.Background, provider.Request{Model: "default", Schema: testSchema}) {
		if e != nil {
			err = e
		}
	}
	if err == nil {
		t.Fatal("no error")
	}
	if n := len(s.fp.Calls()); n != 1 {
		t.Fatalf("%d calls, want 1: a cancelled request is not sent again", n)
	}
}

func TestSchemaForgottenOnNewEndpoint(t *testing.T) {
	s := newSetup(t, provider.KindCompatible, 2, fake.Fail(badRequest(400)))
	req := provider.Request{Model: "default", Schema: testSchema}
	if _, err := collect(s.reg, limit.Background, req); err != nil {
		t.Fatal(err)
	}
	set := func(url string) {
		s.reg.Apply(config.LLMSettings{
			MaxParallelCalls: 4,
			Models:           map[string]string{"default": "p/m1"},
			Providers: map[string]config.ProviderSettings{
				"p": {Kind: string(provider.KindCompatible), BaseURL: url, Models: []config.ModelSettings{{ID: "m1", Context: 32000}}},
			},
		})
	}
	set("https://example.test/v1/") // unchanged: still dropped
	if _, err := collect(s.reg, limit.Background, req); err != nil {
		t.Fatal(err)
	}
	set("https://other.test/v1/")
	if _, err := collect(s.reg, limit.Background, req); err != nil {
		t.Fatal(err)
	}
	if got := schemas(s.fp); !equal(got, []bool{true, false, false, true}) {
		t.Fatalf("schema per call = %v, want it back for the new base URL", got)
	}
}

// rejecter answers 400 to any request with a schema, and "ok" otherwise.
type rejecter struct {
	mu   sync.Mutex
	with int
	all  int
}

func (r *rejecter) Stream(_ context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	r.mu.Lock()
	r.all++
	if req.Schema != nil {
		r.with++
	}
	r.mu.Unlock()
	if req.Schema != nil {
		return func(yield func(provider.Event, error) bool) { yield(provider.Event{}, badRequest(400)) }
	}
	return fake.New().Stream(context.Background(), req)
}

func (r *rejecter) Models(context.Context) ([]provider.ModelInfo, error) { return nil, nil }
func (r *rejecter) SendsSchema() bool                                    { return true }

func TestSchemaDropUnderParallelCalls(t *testing.T) {
	s := newSetup(t, provider.KindCompatible, 4)
	rj := &rejecter{}
	s.reg = provider.NewRegistry(provider.Deps{
		Settings: config.LLMSettings{
			MaxParallelCalls: 4,
			Models:           map[string]string{"default": "p/m1"},
			Providers: map[string]config.ProviderSettings{
				"p": {Kind: string(provider.KindCompatible), BaseURL: "https://example.test/v1/"},
			},
		},
		Backends: map[provider.Kind]provider.Factory{provider.KindCompatible: func(provider.Connection) (provider.Provider, error) { return rj, nil }},
	})
	const n = 8
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for range n {
		wg.Go(func() {
			if _, err := collect(s.reg, limit.Background, provider.Request{Model: "default", Schema: testSchema}); err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if rj.with > n || rj.all != rj.with+n {
		t.Fatalf("%d calls, %d with the schema; want each of %d answered once without it", rj.all, rj.with, n)
	}
	before := rj.with
	if _, err := collect(s.reg, limit.Background, provider.Request{Model: "default", Schema: testSchema}); err != nil {
		t.Fatal(err)
	}
	if rj.with != before {
		t.Error("the schema was sent again after it was dropped")
	}
}

// hooked is a backend around a shared fake. Each build is a new one, as
// with the real backends; before runs at the start of each call.
type hooked struct {
	fp     *fake.Provider
	before func(provider.Request)
	json   bool // whether it has a JSON mode
}

func (h *hooked) Stream(ctx context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	if h.before != nil {
		h.before(req)
	}
	return h.fp.Stream(ctx, req)
}

func (h *hooked) Models(ctx context.Context) ([]provider.ModelInfo, error) { return h.fp.Models(ctx) }

// plain is a backend without a JSON mode.
type plain struct{ *hooked }

func (p plain) Stream(ctx context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	return p.hooked.Stream(ctx, req)
}

type withJSON struct{ *hooked }

func (w withJSON) Stream(ctx context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	return w.hooked.Stream(ctx, req)
}
func (w withJSON) SendsSchema() bool { return true }

func hookSettings(url string) config.LLMSettings {
	return config.LLMSettings{
		MaxParallelCalls:         4,
		ProviderMaxParallelCalls: map[string]int{"p": 2},
		Models:                   map[string]string{"default": "p/m1"},
		Providers: map[string]config.ProviderSettings{
			"p": {Kind: string(provider.KindCompatible), BaseURL: url, Models: []config.ModelSettings{{ID: "m1", Context: 32000}}},
		},
	}
}

// hookSetup is a Registry whose provider "p" is built as a new hooked
// backend each time. before may use the Registry through *reg.
func hookSetup(json bool, before func(reg *provider.Registry, req provider.Request), replies ...fake.Reply) (*provider.Registry, *fake.Provider) {
	fp := fake.New(replies...)
	var reg *provider.Registry
	factory := func(provider.Connection) (provider.Provider, error) {
		h := &hooked{fp: fp, json: json}
		if before != nil {
			h.before = func(req provider.Request) { before(reg, req) }
		}
		if json {
			return withJSON{h}, nil
		}
		return plain{h}, nil
	}
	reg = provider.NewRegistry(provider.Deps{
		Settings: hookSettings("https://example.test/v1/"),
		Backends: map[provider.Kind]provider.Factory{provider.KindCompatible: factory},
	})
	return reg, fp
}

// A backend that doesn't send the schema: its 400 is not about it.
func TestSchemaIgnoredBackendNotResent(t *testing.T) {
	reg, fp := hookSetup(false, nil, fake.Fail(badRequest(400)))
	if _, err := collect(reg, limit.Background, provider.Request{Model: "default", Schema: testSchema}); kindOf(err) != provider.BadRequest {
		t.Fatalf("error %v, want the 400", err)
	}
	if n := len(fp.Calls()); n != 1 {
		t.Fatalf("%d calls, want 1", n)
	}
}

// The drop is for that schema: another schema for the same model is
// still sent.
func TestSchemaDropIsPerSchema(t *testing.T) {
	s := newSetup(t, provider.KindCompatible, 2, fake.Fail(badRequest(400)))
	other := json.RawMessage(`{"type":"object","properties":{"b":{"type":"integer"}}}`)
	for _, sch := range []json.RawMessage{testSchema, other, testSchema} {
		if _, err := collect(s.reg, limit.Background, provider.Request{Model: "default", Schema: sch}); err != nil {
			t.Fatal(err)
		}
	}
	calls := s.fp.Calls()
	var got []string
	for _, c := range calls {
		got = append(got, string(c.Schema))
	}
	want := []string{string(testSchema), "", string(other), ""}
	if len(got) != len(want) {
		t.Fatalf("schemas %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("schemas %q, want %q", got, want)
		}
	}
}

// A provider paused by another call between the two tries: no resend, the
// first error is returned, and nothing is dropped.
func TestSchemaNotResentWhilePaused(t *testing.T) {
	var other atomic.Bool // the nested call runs the hook too
	reg, fp := hookSetup(true, func(reg *provider.Registry, req provider.Request) {
		if other.CompareAndSwap(false, true) {
			// Another call gets a rate limit while this one is out.
			if _, err := collect(reg, limit.Background, provider.Request{Model: "p/m2"}); kindOf(err) != provider.RateLimited {
				t.Errorf("the other call: %v", err)
			}
		}
	}, fake.Fail(rateLimited(time.Minute)), fake.Fail(badRequest(400)))
	// The other call runs first inside the hook, so it plays the first
	// reply; this call plays the 400.
	_, err := collect(reg, limit.Background, provider.Request{Model: "default", Schema: testSchema})
	var pe *provider.Error
	if kindOf(err) != provider.BadRequest || !asError(err, &pe) || pe.Status != 400 {
		t.Fatalf("error %v, want the first 400", err)
	}
	if n := len(fp.Calls()); n != 2 {
		t.Fatalf("%d calls, want 2: no resend while paused", n)
	}
	reg.Resume("p")
	if _, err := collect(reg, limit.Background, provider.Request{Model: "default", Schema: testSchema}); err != nil {
		t.Fatal(err)
	}
	if calls := fp.Calls(); calls[len(calls)-1].Schema == nil {
		t.Error("the schema was dropped")
	}
}

// The connection rebuilt while a call is out: the old backend's resend
// records nothing for the new one.
func TestSchemaBackendRebuiltMidCall(t *testing.T) {
	var once sync.Once
	reg, fp := hookSetup(true, func(reg *provider.Registry, req provider.Request) {
		once.Do(func() { reg.Apply(hookSettings("https://other.test/v1/")) })
	}, fake.Fail(badRequest(400)))
	req := provider.Request{Model: "default", Schema: testSchema}
	for range 2 {
		if _, err := collect(reg, limit.Background, req); err != nil {
			t.Fatal(err)
		}
	}
	if got := schemas(fp); !equal(got, []bool{true, false, true}) {
		t.Fatalf("schema per call = %v, want the new backend to send it", got)
	}
}
