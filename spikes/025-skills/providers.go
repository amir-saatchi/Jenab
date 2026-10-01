package main

// Provider setup, trimmed from SPIKE-018: Ollama Cloud and Z.ai only, same host lock,
// outgoing-header allow-list, error-body capture and redaction.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type Prov struct {
	Name    string
	BaseURL string
	KeyName string
	Host    string // the only host the key may go to
	MinGap  time.Duration
	Models  []*Model
	client  *openai.Client
	Listed  map[string]bool
}

type Model struct {
	P       *Prov
	ID      string
	Effort  string // reasoning_effort we send ("" = provider default)
	pace    pacer
	stopped string
	mu      sync.Mutex
	calls   int
	// cached_tokens reports over the whole run
	cachedSeen, cachedHits, cachedMax int
}

func (m *Model) Name() string { return m.P.Name + " " + m.ID }

func (m *Model) stop(why string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopped == "" {
		m.stopped = why
	}
}

func (m *Model) noteCached(n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cachedSeen++
	if n > m.cachedMax {
		m.cachedMax = n
	}
	if n > 0 {
		m.cachedHits++
	}
}

// stopAll stops every model of the provider (same key, same quota).
func (p *Prov) stopAll(why string) {
	for _, m := range p.Models {
		m.stop(why)
	}
}

func (m *Model) Stopped() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopped
}

// Base URLs from the providers' docs (same as SPIKE-018). Ollama's free plan allows one
// concurrent request, so Ollama models run one after another; Z.ai runs in its own lane.
var providers = []*Prov{
	{Name: "ollama", BaseURL: "https://ollama.com/v1/", KeyName: "OLLAMA_API_KEY", Host: "ollama.com", MinGap: 1 * time.Second},
	{Name: "zai", BaseURL: "https://api.z.ai/api/paas/v4/", KeyName: "Z_API_KEY", Host: "api.z.ai", MinGap: 2 * time.Second},
}

var modelIDs = map[string][]string{
	"ollama": {"nemotron-3-ultra", "gemma4:31b", "gpt-oss:120b"},
	"zai":    {"glm-4.5-flash"},
}

func findModel(name string) *Model {
	for _, p := range providers {
		for _, m := range p.Models {
			if m.ID == name || m.Name() == name {
				return m
			}
		}
	}
	return nil
}

// ---- per-call capture (status, rate-limit headers) ----

type ctxKey struct{}

type capture struct {
	mu      sync.Mutex
	status  int
	headers map[string]string
	body    string // error body, redacted
}

func withCapture(ctx context.Context) (context.Context, *capture) {
	c := &capture{}
	return context.WithValue(ctx, ctxKey{}, c), c
}

func keepHeader(k string) bool {
	k = strings.ToLower(k)
	return k == "retry-after" || strings.HasPrefix(k, "x-ratelimit") || strings.HasPrefix(k, "ratelimit")
}

// allowed outgoing headers. The SDK also reads OPENAI_ORG_ID, OPENAI_PROJECT_ID and
// OPENAI_CUSTOM_HEADERS from the environment; those must never reach a non-OpenAI host.
func allowedOut(k string) bool {
	k = strings.ToLower(k)
	switch k {
	case "authorization", "content-type", "accept", "user-agent", "content-length", "idempotency-key":
		return true
	}
	return strings.HasPrefix(k, "x-stainless-")
}

func (p *Prov) middleware(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
	if req.URL.Scheme != "https" || req.URL.Host != p.Host {
		return nil, fmt.Errorf("refusing request to unexpected host %q", req.URL.Host)
	}
	for k := range req.Header {
		if !allowedOut(k) {
			req.Header.Del(k)
		}
	}
	resp, err := next(req)
	if c, ok := req.Context().Value(ctxKey{}).(*capture); ok && resp != nil {
		c.mu.Lock()
		c.status = resp.StatusCode
		c.headers = map[string]string{}
		for k, v := range resp.Header {
			if keepHeader(k) {
				c.headers[strings.ToLower(k)] = strings.Join(v, ",")
			}
		}
		if resp.StatusCode >= 400 {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
			resp.Body.Close()
			resp.Body = io.NopCloser(bytes.NewReader(b))
			c.body = redact(string(b))
		}
		c.mu.Unlock()
	}
	return resp, err
}

func (c *capture) hdrString() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var ks []string
	for k := range c.headers {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	var parts []string
	for _, k := range ks {
		parts = append(parts, k+"="+c.headers[k])
	}
	return strings.Join(parts, " ")
}

func (c *capture) get(k string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.headers[k]
}

// stripOpenAIEnv removes OPENAI_* variables from this process, so the SDK cannot pick up a
// base URL, org, project or custom headers from the environment (SPIKE-018 rule).
func stripOpenAIEnv() []string {
	var names []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(strings.ToUpper(k), "OPENAI_") {
			os.Unsetenv(k)
			names = append(names, k)
		}
	}
	return names
}

func (p *Prov) init() error {
	key := secrets[p.KeyName]
	if key == "" {
		return fmt.Errorf("%s not set in .env", p.KeyName)
	}
	c := openai.NewClient(
		option.WithBaseURL(p.BaseURL),
		option.WithAPIKey(key),
		option.WithMaxRetries(0),
		option.WithMiddleware(p.middleware),
		option.WithRequestTimeout(6*time.Minute),
	)
	p.client = &c
	return nil
}

func (p *Prov) setup(ctx context.Context) error {
	if err := p.init(); err != nil {
		return err
	}
	pg, err := p.client.Models.List(ctx)
	for i := 0; i < 2 && err != nil && statusOf(err) == 0; i++ {
		time.Sleep(2 * time.Second)
		pg, err = p.client.Models.List(ctx)
	}
	p.Listed = map[string]bool{}
	if err != nil {
		return fmt.Errorf("/models: %s", statusText(err))
	}
	for _, m := range pg.Data {
		p.Listed[strings.TrimPrefix(m.ID, "models/")] = true
	}
	return nil
}

func statusOf(err error) int {
	var oe *openai.Error
	if errors.As(err, &oe) {
		return oe.StatusCode
	}
	return 0
}

func statusText(err error) string {
	if err == nil {
		return "200"
	}
	if s := statusOf(err); s != 0 {
		return fmt.Sprint(s)
	}
	return "error: " + short(redact(err.Error()), 80)
}

func short(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
