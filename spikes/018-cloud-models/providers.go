package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// Base URLs come from each provider's official docs:
//   Groq:         https://console.groq.com/docs/openai            -> https://api.groq.com/openai/v1
//   Ollama Cloud: https://docs.ollama.com/api/openai-compatibility -> https://ollama.com/v1
//   Gemini:       https://ai.google.dev/gemini-api/docs/openai    -> https://generativelanguage.googleapis.com/v1beta/openai/

type Prov struct {
	Name    string
	BaseURL string
	KeyName string
	Host    string // the only host the key may go to
	// second host, tried only when the first answers 401 (Z.ai: keys may be platform-specific)
	AltBaseURL, AltHost string
	HostNote            string
	// Parallel: models of this provider may run at the same time (limits are per model).
	// Ollama's free plan allows 1 concurrent request, so its models run one after another.
	Parallel bool
	// MinGap between two requests to the same model (from the published RPM).
	MinGap time.Duration
	// Wanted models: the user's names; resolved against /models at runtime.
	Want   []string
	Probes []string // IDs tried with one tiny request only (availability on this account)
	Models []*Model
	client *openai.Client
	// listed model IDs and raw /models entries
	Listed map[string]string
}

type Model struct {
	P       *Prov
	Want    string
	ID      string
	Listed  bool
	Context string // documented / reported context limit
	Effort  string // reasoning_effort we send ("" = none)
	pace    pacer
	stopped string // non-empty: daily quota or unavailable; skip the rest
	mu      sync.Mutex
	calls   int // successful calls
	// failed requests in a row (after our retries); 4 stop the model
	failStreak int
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

// noteCached records that the provider reported cached_tokens in any call of this model.
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

func (m *Model) Stopped() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopped
}

var providers = []*Prov{
	{Name: "groq", BaseURL: "https://api.groq.com/openai/v1/", KeyName: "GROQ_API_KEY", Host: "api.groq.com",
		Parallel: true, MinGap: 2500 * time.Millisecond, // 30 RPM
		Want: []string{"openai/gpt-oss-120b", "openai/gpt-oss-20b", "qwen*27b"}},
	{Name: "ollama", BaseURL: "https://ollama.com/v1/", KeyName: "OLLAMA_API_KEY", Host: "ollama.com",
		Parallel: false, MinGap: 1 * time.Second,
		Want: []string{"gemma4:31b", "gpt-oss:120b", "gpt-oss:20b", "nemotron-3-nano:30b", "nemotron-3-super", "nemotron-3-ultra"}},
	{Name: "gemini", BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai/", KeyName: "GEMINI_API_KEY", Host: "generativelanguage.googleapis.com",
		Parallel: true, MinGap: 7 * time.Second, // free tier Flash: about 10 RPM; Pro is set to 13 s below
		Want:   []string{"gemini-3.8-flash", "gemini-3.5-flash-lite|gemini-3.1-flash-lite", "gemma-4-31b-it"},
		Probes: []string{"gemini-2.5-pro", "gemini-3.1-pro-preview"}}, // Pro tiers: one tiny request each
	// Z.ai: https://docs.z.ai/guides/overview/quick-start (OpenAI-compatible base URL). Free models per
	// https://docs.z.ai/guides/overview/pricing: GLM-4.7-Flash and GLM-4.5-Flash. glm-4-flash is not on
	// that page, so it is not called. The mainland platform is tried only if the key is unknown (401).
	{Name: "zai", BaseURL: "https://api.z.ai/api/paas/v4/", KeyName: "Z_API_KEY", Host: "api.z.ai",
		AltBaseURL: "https://open.bigmodel.cn/api/paas/v4/", AltHost: "open.bigmodel.cn",
		Parallel: false, MinGap: 2 * time.Second,
		Want: []string{"glm-4.7-flash", "glm-4.5-flash"}},
}

// ---- per-call capture (status, rate-limit headers, time to headers) ----

type ctxKey struct{}

type capture struct {
	mu      sync.Mutex
	status  int
	headers map[string]string
	hdrAt   time.Time
	body    string // error body, redacted
}

func withCapture(ctx context.Context) (context.Context, *capture) {
	c := &capture{}
	return context.WithValue(ctx, ctxKey{}, c), c
}

// keepHeader: only rate-limit headers are recorded. Auth headers are never read.
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
		c.hdrAt = time.Now()
		c.headers = map[string]string{}
		for k, v := range resp.Header {
			if keepHeader(k) {
				c.headers[strings.ToLower(k)] = strings.Join(v, ",")
			}
		}
		if resp.StatusCode >= 400 {
			// Keep the error body (Gemini sends a JSON array the SDK does not parse). Redacted at once.
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

func (p *Prov) init() error {
	key := secrets[p.KeyName]
	if key == "" {
		return fmt.Errorf("%s not set in .env", p.KeyName)
	}
	c := openai.NewClient(
		option.WithBaseURL(p.BaseURL),
		option.WithAPIKey(key),
		option.WithMaxRetries(0), // our retry loop (SPIKE-012: the retry count is ours)
		option.WithMiddleware(p.middleware),
		option.WithRequestTimeout(4*time.Minute),
	)
	p.client = &c
	return nil
}

// setup builds the client and lists models. For a provider with a second host, a 401 on the
// first host switches to the second one.
func (p *Prov) setup(ctx context.Context) error {
	if err := p.init(); err != nil {
		return err
	}
	err := p.listModels(ctx)
	if p.AltBaseURL != "" {
		if statusOf(err) == 401 {
			p.HostNote = fmt.Sprintf("%s answered 401 on /models; switched to %s", p.Host, p.AltHost)
			p.BaseURL, p.Host = p.AltBaseURL, p.AltHost
			p.init()
			err = p.listModels(ctx)
		} else {
			p.HostNote = fmt.Sprintf("%s: /models answered %s, so %s was not tried", p.Host, statusText(err), p.AltHost)
		}
	}
	if statusOf(err) == 404 {
		p.Listed = map[string]string{}
		p.HostNote += "; no /models endpoint (404)"
		return nil
	}
	return err
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

// listModels returns model IDs from /models and the raw JSON per entry.
func (p *Prov) listModels(ctx context.Context) error {
	pg, err := p.client.Models.List(ctx)
	for i := 0; i < 2 && err != nil && statusOf(err) == 0; i++ { // transport error: retry
		time.Sleep(2 * time.Second)
		pg, err = p.client.Models.List(ctx)
	}
	if err != nil {
		return err
	}
	p.Listed = map[string]string{}
	for _, m := range pg.Data {
		p.Listed[strings.TrimPrefix(m.ID, "models/")] = m.RawJSON()
	}
	return nil
}

// ---- native endpoints for the documented context limit (same host, documented auth header) ----

var nativeHTTP = &http.Client{Timeout: 30 * time.Second}

func nativeGet(p *Prov, method, url string, body any, hdr map[string]string) (map[string]any, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rd)
	if err != nil {
		return nil, err
	}
	if req.URL.Host != p.Host {
		return nil, fmt.Errorf("unexpected host")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := nativeHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s", redact(err.Error()))
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, short(redact(string(b)), 160))
	}
	var out map[string]any
	err = json.Unmarshal(b, &out)
	return out, err
}

// contextLimit asks the provider for the model's context size.
func contextLimit(m *Model) string {
	p := m.P
	key := secrets[p.KeyName]
	switch p.Name {
	case "groq":
		var raw map[string]any
		if json.Unmarshal([]byte(p.Listed[m.ID]), &raw) == nil {
			if v, ok := raw["context_window"]; ok {
				s := fmt.Sprintf("%v", v)
				if mc, ok := raw["max_completion_tokens"]; ok {
					s += fmt.Sprintf(" (max output %v)", mc)
				}
				return s + " (/models context_window)"
			}
		}
	case "gemini":
		// Native models.get, key in the documented x-goog-api-key header (never in the URL).
		out, err := nativeGet(p, "GET", "https://generativelanguage.googleapis.com/v1beta/models/"+m.ID, nil, map[string]string{"x-goog-api-key": key})
		if err != nil {
			return "? (" + err.Error() + ")"
		}
		return fmt.Sprintf("%v in / %v out (models.get)", num(out["inputTokenLimit"]), num(out["outputTokenLimit"]))
	case "ollama":
		out, err := nativeGet(p, "POST", "https://ollama.com/api/show", map[string]any{"model": m.ID}, map[string]string{"Authorization": "Bearer " + key})
		if err != nil {
			return "? (" + err.Error() + ")"
		}
		if mi, ok := out["model_info"].(map[string]any); ok {
			for k, v := range mi {
				if strings.HasSuffix(k, ".context_length") {
					return fmt.Sprintf("%v (/api/show %s)", num(v), k)
				}
			}
		}
		return "? (/api/show has no context_length)"
	case "zai":
		// not reported by the API; from the model pages in the docs
		return map[string]string{"glm-4.7-flash": "200K in / 128K out (docs)", "glm-4.5-flash": "128K in / 96K out (docs)"}[m.ID]
	}
	return "?"
}

func num(v any) string {
	if f, ok := v.(float64); ok {
		return fmt.Sprintf("%.0f", f)
	}
	return fmt.Sprint(v)
}

func short(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
