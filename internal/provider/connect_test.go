package provider_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
	"github.com/amir-saatchi/jenab/internal/secret"
)

const (
	cfBase    = "https://api.cloudflare.com/client/v4/accounts/{account_id}/ai/v1/"
	accountID = "acct0123456789abcdef"
)

// recorder is a fake backend that remembers what it was built from.
type recorder struct {
	mu    sync.Mutex
	fp    *fake.Provider
	conns []provider.Connection
}

func (r *recorder) factory(provider.Kind) provider.Factory {
	return func(c provider.Connection) (provider.Provider, error) {
		r.mu.Lock()
		r.conns = append(r.conns, c)
		r.mu.Unlock()
		return r.fp, nil
	}
}

func (r *recorder) built() []provider.Connection {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]provider.Connection(nil), r.conns...)
}

type connectSetup struct {
	reg *provider.Registry
	rec *recorder
	kr  *memKeyring
}

// newConnectSetup makes a Registry with no providers and a recording
// backend for every kind.
func newConnectSetup(fp *fake.Provider) connectSetup {
	rec := &recorder{fp: fp}
	kr := &memKeyring{m: map[string]string{}}
	backends := map[provider.Kind]provider.Factory{}
	for _, k := range []provider.Kind{provider.KindAnthropic, provider.KindOpenAI, provider.KindGemini, provider.KindCompatible, provider.KindOllama} {
		backends[k] = rec.factory(k)
	}
	reg := provider.NewRegistry(provider.Deps{
		Settings: config.LLMSettings{MaxParallelCalls: 4},
		Secrets:  secret.New(kr),
		Gate:     limit.NewGate(4),
		Backends: backends,
	})
	return connectSetup{reg: reg, rec: rec, kr: kr}
}

func TestConnectFillsPlaceholders(t *testing.T) {
	fp := fake.New()
	fp.ListErr = &provider.Error{Kind: provider.BadRequest, Status: 405, Message: "method not allowed"}
	s := newConnectSetup(fp)

	got, err := s.reg.Connect(context.Background(), "cf", provider.KindCompatible, cfBase, testKey, map[string]string{"account_id": accountID})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if !got.NoModelList {
		t.Error("NoModelList = false for a 405 model list")
	}
	if got.Settings.BaseURL != cfBase {
		t.Errorf("settings base URL = %q, want the template %q", got.Settings.BaseURL, cfBase)
	}
	if v := s.kr.m[provider.KeyName("cf")]; v != testKey {
		t.Errorf("stored key = %q", v)
	}
	if v := s.kr.m[provider.FieldName("cf", "account_id")]; v != accountID {
		t.Errorf("stored account_id = %q", v)
	}
	want := "https://api.cloudflare.com/client/v4/accounts/" + accountID + "/ai/v1/"
	if b := s.rec.built(); len(b) != 1 || b[0].BaseURL != want {
		t.Fatalf("Connect built %+v, want one backend on %s", b, want)
	}

	// The saved settings work: the backend gets the filled URL from the keychain.
	s.reg.Apply(config.LLMSettings{MaxParallelCalls: 4, Providers: map[string]config.ProviderSettings{
		"cf": {Kind: string(provider.KindCompatible), BaseURL: cfBase, Models: []config.ModelSettings{{ID: "m", Context: 8000}}},
	}})
	if _, err := collect(s.reg, limit.Interactive, provider.Request{Model: "cf/m"}); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	b := s.rec.built()
	if len(b) != 2 || b[1].BaseURL != want {
		t.Fatalf("Stream built %+v, want a backend on %s", b, want)
	}
	if r := b[1].Redact("account " + accountID); strings.Contains(r, accountID) {
		t.Errorf("the backend's Redact leaves the account ID: %q", r)
	}
}

func TestConnectNotFoundListIsNoModelList(t *testing.T) {
	fp := fake.New()
	fp.ListErr = &provider.Error{Kind: provider.BadRequest, Status: 404, Message: "not found"}
	s := newConnectSetup(fp)
	got, err := s.reg.Connect(context.Background(), "p", provider.KindCompatible, "https://example.test/v1/", testKey, nil)
	if err != nil || !got.NoModelList {
		t.Fatalf("Connect = %+v, %v; want NoModelList", got, err)
	}
	if s.kr.m[provider.KeyName("p")] != testKey {
		t.Error("the key was not stored")
	}
}

func TestConnectFailureRedactsAndStoresNothing(t *testing.T) {
	fp := fake.New()
	fp.ListErr = &provider.Error{Kind: provider.BadRequest, Status: 401, Message: "bad key " + testKey + " for account " + accountID}
	s := newConnectSetup(fp)
	_, err := s.reg.Connect(context.Background(), "cf", provider.KindCompatible, cfBase, testKey, map[string]string{"account_id": accountID})
	if err == nil {
		t.Fatal("Connect worked with a 401 model list")
	}
	if msg := err.Error(); strings.Contains(msg, testKey) || strings.Contains(msg, accountID) {
		t.Errorf("error shows a secret: %q", msg)
	}
	if kindOf(err) != provider.BadRequest || !strings.HasPrefix(err.Error(), "cf: ") {
		t.Errorf("err = %v, want a request error from cf", err)
	}
	if len(s.kr.m) != 0 {
		t.Errorf("stored %v after a failed Connect", s.kr.m)
	}
}

func TestConnectRejectsBadFields(t *testing.T) {
	for _, v := range []string{"", "../other", "a/b", "a?b=c", "a.b", "a b", "a#b", "a@evil.test", strings.Repeat("a", 129)} {
		s := newConnectSetup(fake.New())
		_, err := s.reg.Connect(context.Background(), "cf", provider.KindCompatible, cfBase, testKey, map[string]string{"account_id": v})
		if err == nil {
			t.Errorf("account_id %q: Connect worked", v)
		} else if v != "" && strings.Contains(err.Error(), v) {
			t.Errorf("account_id %q: the error shows it: %v", v, err)
		}
		if len(s.rec.built()) != 0 || len(s.kr.m) != 0 {
			t.Errorf("account_id %q: built %d backends, stored %v", v, len(s.rec.built()), s.kr.m)
		}
	}
	// Missing.
	s := newConnectSetup(fake.New())
	if _, err := s.reg.Connect(context.Background(), "cf", provider.KindCompatible, cfBase, testKey, nil); err == nil {
		t.Error("Connect worked with no account_id")
	}
}

func TestStreamNeedsStoredField(t *testing.T) {
	for name, stored := range map[string]map[string]string{
		"missing": {provider.KeyName("cf"): testKey},
		"bad":     {provider.KeyName("cf"): testKey, provider.FieldName("cf", "account_id"): "x/../y"},
	} {
		t.Run(name, func(t *testing.T) {
			s := newConnectSetup(fake.New())
			for k, v := range stored {
				s.kr.m[k] = v
			}
			s.reg.Apply(config.LLMSettings{MaxParallelCalls: 4, Providers: map[string]config.ProviderSettings{
				"cf": {Kind: string(provider.KindCompatible), BaseURL: cfBase, Models: []config.ModelSettings{{ID: "m", Context: 8000}}},
			}})
			_, err := collect(s.reg, limit.Interactive, provider.Request{Model: "cf/m"})
			if kindOf(err) != provider.BadRequest {
				t.Fatalf("err = %v, want a request error", err)
			}
			if len(s.rec.built()) != 0 {
				t.Error("a backend was built")
			}
		})
	}
}

func TestLocalOllamaLimit(t *testing.T) {
	cases := []struct {
		kind  provider.Kind
		base  string
		set   map[string]int
		limit int
	}{
		{provider.KindOllama, "", nil, 1},
		{provider.KindOllama, "http://127.0.0.1:11434/", nil, 1},
		{provider.KindOllama, "http://localhost:11434/", map[string]int{"p": 3}, 3},
		{provider.KindOllama, "https://ollama.com/", nil, 4},
		{provider.KindCompatible, "http://localhost:8080/v1/", nil, 4},
	}
	for _, c := range cases {
		s := newConnectSetup(fake.New())
		s.reg.Apply(config.LLMSettings{MaxParallelCalls: 4, ProviderMaxParallelCalls: c.set,
			Providers: map[string]config.ProviderSettings{"p": {Kind: string(c.kind), BaseURL: c.base}}})
		if got := status(t, s.reg).Limit; got != c.limit {
			t.Errorf("%s %q %v: limit %d, want %d", c.kind, c.base, c.set, got, c.limit)
		}
	}
}

func TestPresetsConnect(t *testing.T) {
	ids := map[string]bool{}
	for _, p := range provider.Presets {
		if ids[p.ID] || p.Name == "" {
			t.Errorf("preset %q: repeated ID or no name", p.ID)
		}
		ids[p.ID] = true
		fields := map[string]string{}
		for _, f := range provider.Placeholders(p.BaseURL) {
			fields[f] = "abc123"
		}
		s := newConnectSetup(fake.New())
		if _, err := s.reg.Connect(context.Background(), p.ID, p.Kind, p.BaseURL, testKey, fields); err != nil {
			t.Errorf("preset %s: %v", p.ID, err)
		}
	}
}

func TestConnectNeedsKey(t *testing.T) {
	s := newConnectSetup(fake.New())
	if _, err := s.reg.Connect(context.Background(), "p", provider.KindCompatible, "https://example.test/v1/", "", nil); err == nil {
		t.Error("Connect worked with no key")
	}
	if len(s.rec.built()) != 0 || len(s.kr.m) != 0 {
		t.Errorf("built %d backends, stored %v", len(s.rec.built()), s.kr.m)
	}
}
