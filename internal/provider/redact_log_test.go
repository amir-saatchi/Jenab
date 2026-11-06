package provider_test

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
	"github.com/amir-saatchi/jenab/internal/secret"
)

// syncBuffer is a log sink that a test reads while calls may write.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// Every "provider:" log line carries the provider's error through the
// Store's Redact: an error that echoes the key, in its message or in a
// transport error's URL, is logged with [secret:...] in its place.
func TestProviderLogsAreRedacted(t *testing.T) {
	withKey := func(status int) error {
		return &provider.Error{Kind: provider.BadRequest, Status: status, Message: "invalid argument; key " + testKey + " in the request"}
	}
	fp := fake.New(
		fake.Fail(withKey(400)), // the schema is dropped: the first log line
		fake.Text("ok"),         // the request without the schema
		fake.Fail(withKey(400)), // another schema, failing twice: the second log line
		fake.Fail(withKey(400)), // and "call failed"
		fake.Fail(withKey(401)), // "call failed" for a provider Error
		fake.Fail(errors.New(`Post "https://example.test/v1/?key=`+testKey+`": connection reset`)), // a transport error
	)
	logs := &syncBuffer{}
	reg := provider.NewRegistry(provider.Deps{
		Settings: config.LLMSettings{
			MaxParallelCalls: 4,
			Models:           map[string]string{"default": "p/m1"},
			Providers: map[string]config.ProviderSettings{
				"p": {Kind: string(provider.KindCompatible), BaseURL: "https://example.test/v1/", Models: []config.ModelSettings{{ID: "m1", Context: 32000}}},
			},
		},
		Secrets:  secret.New(&memKeyring{m: map[string]string{provider.KeyName("p"): testKey}}),
		Gate:     limit.NewGate(4),
		Backends: map[provider.Kind]provider.Factory{provider.KindCompatible: fp.Factory()},
		Log:      slog.New(slog.NewTextHandler(logs, nil)),
	})
	other := []byte(`{"type":"object","properties":{"b":{"type":"string"}}}`)
	for i, req := range []provider.Request{
		{Model: "default", Schema: testSchema},
		{Model: "default", Schema: other},
		{Model: "default"},
		{Model: "default"},
	} {
		_, _ = collect(reg, limit.Background, req)
		if i == 0 && len(fp.Calls()) != 2 {
			t.Fatalf("the first request made %d calls, want 2", len(fp.Calls()))
		}
	}
	out := logs.String()
	for _, want := range []string{
		"provider: the API rejected the answer schema",
		"provider: a request with an answer schema failed, and failed again without it",
		"provider: call failed",
		"connection reset",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the log has no %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, testKey) {
		t.Errorf("the log holds the key:\n%s", out)
	}
	if n := strings.Count(out, "[secret:"); n < 5 {
		t.Errorf("%d redacted keys in the log, want at least 5:\n%s", n, out)
	}
}
