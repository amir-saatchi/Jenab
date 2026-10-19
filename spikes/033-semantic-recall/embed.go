package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/store"
)

// embedder is the shape every piece of SPIKE-033 shares, with no shared
// package: texts go in as given (the caller adds any prefix), and vectors
// come back as the model gives them (the caller normalizes them).
type embedder interface {
	Name() string
	Dims() int
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// errStopped marks a quota or rate-limit error: that model stops, and the
// error is recorded. Free tiers aren't worked around.
var errStopped = errors.New("model stopped")

func newEmbedder(m modelSpec, k keys) (embedder, error) {
	key := ""
	if m.KeyEnv != "" {
		if key = k.get(m.KeyEnv); key == "" {
			return nil, fmt.Errorf("%s: %s is not set in .env or the environment", m.Name, m.KeyEnv)
		}
	}
	h := httpEmbedder{spec: m, key: key, client: &http.Client{Timeout: 5 * time.Minute}}
	switch m.Kind {
	case "ollama":
		return &ollamaEmbedder{h}, nil
	case "openai_compatible":
		return &openAIEmbedder{h}, nil
	case "fake":
		return newFake(m.Name, m.Dims), nil
	case "builtin":
		// The built-in embedder is the nested module in builtin/, with the
		// same shape. It is wired in here after the pieces are merged.
		return nil, fmt.Errorf("%s: the built-in embedder is not wired yet", m.Name)
	}
	return nil, fmt.Errorf("%s: unknown kind %q", m.Name, m.Kind)
}

type httpEmbedder struct {
	spec   modelSpec
	key    string
	client *http.Client
	dims   int // seen in the first answer
}

func (h *httpEmbedder) Name() string { return h.spec.Name }
func (h *httpEmbedder) Dims() int    { return h.dims }

func (h *httpEmbedder) url(path string) string {
	return strings.TrimRight(h.spec.BaseURL, "/") + path
}

// post sends one request. Overloads and lost connections are tried again,
// up to 3 tries; a quota or rate limit stops the model at once.
func (h *httpEmbedder) post(ctx context.Context, url string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	for try := 0; ; try++ {
		err := h.once(ctx, url, b, out)
		var pe *provider.Error
		if err == nil || !errors.As(err, &pe) {
			return err
		}
		if pe.Kind == provider.Quota || pe.Kind == provider.RateLimited {
			return fmt.Errorf("%w: %v", errStopped, err)
		}
		if !pe.Retryable() || try == 2 {
			return err
		}
		select {
		case <-time.After(max(pe.RetryAfter, provider.Backoff(try))):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (h *httpEmbedder) once(ctx context.Context, url string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if h.key != "" {
		req.Header.Set("Authorization", "Bearer "+h.key)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return provider.TransportError(h.spec.Name, "the request failed", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 256<<20))
	if err != nil {
		return provider.TransportError(h.spec.Name, "the answer was cut off", err)
	}
	if resp.StatusCode/100 != 2 {
		text := string(data)
		if h.key != "" {
			text = strings.ReplaceAll(text, h.key, "[key]")
		}
		return provider.Classify(h.spec.Name, resp.StatusCode, resp.Header, text)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%s: the answer isn't the expected JSON: %w", h.spec.Name, err)
	}
	return nil
}

func (h *httpEmbedder) check(texts []string, vecs [][]float32) error {
	if len(vecs) != len(texts) {
		return fmt.Errorf("%s: %d vectors for %d texts", h.spec.Name, len(vecs), len(texts))
	}
	for _, v := range vecs {
		if len(v) == 0 || (h.Dims() != 0 && len(v) != h.Dims()) {
			return fmt.Errorf("%s: a vector of width %d, expected %d", h.spec.Name, len(v), h.Dims())
		}
		h.dims = len(v)
	}
	return nil
}

// ollamaEmbedder calls Ollama's native /api/embed.
type ollamaEmbedder struct{ httpEmbedder }

func (o *ollamaEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	var out struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	req := map[string]any{"model": o.spec.Model, "input": texts}
	if err := o.post(ctx, o.url("/api/embed"), req, &out); err != nil {
		return nil, err
	}
	return out.Embeddings, o.check(texts, out.Embeddings)
}

// openAIEmbedder calls an OpenAI-compatible /embeddings.
type openAIEmbedder struct{ httpEmbedder }

func (o *openAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	var out struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	req := map[string]any{"model": o.spec.Model, "input": texts, "encoding_format": "float"}
	if err := o.post(ctx, o.url("/embeddings"), req, &out); err != nil {
		return nil, err
	}
	vecs := make([][]float32, len(texts))
	for _, d := range out.Data {
		if d.Index < 0 || d.Index >= len(vecs) || vecs[d.Index] != nil {
			return nil, fmt.Errorf("%s: a vector with index %d", o.spec.Name, d.Index)
		}
		vecs[d.Index] = d.Embedding
	}
	return vecs, o.check(texts, vecs)
}

// fakeEmbedder tests the harness: each word, and each word's first four
// letters, adds ±1 at a hashed place. Texts with words in common are near;
// it knows nothing of meaning or language.
type fakeEmbedder struct {
	name  string
	dims  int
	calls atomic.Int64
	texts atomic.Int64
}

func newFake(name string, dims int) *fakeEmbedder {
	if dims <= 0 {
		dims = 512
	}
	return &fakeEmbedder{name: name, dims: dims}
}

func (f *fakeEmbedder) Name() string { return f.name }
func (f *fakeEmbedder) Dims() int    { return f.dims }

func (f *fakeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	f.calls.Add(1)
	f.texts.Add(int64(len(texts)))
	out := make([][]float32, len(texts))
	for i, t := range texts {
		v := make([]float32, f.dims)
		words := strings.FieldsFunc(strings.ToLower(store.Normalize(t)), func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsNumber(r)
		})
		for _, w := range words {
			add(v, w, 1)
			if r := []rune(w); len(r) > 4 {
				add(v, string(r[:4]), 0.5)
			}
		}
		out[i] = v
	}
	return out, ctx.Err()
}

func add(v []float32, w string, x float32) {
	h := fnv.New64a()
	h.Write([]byte(w))
	s := h.Sum64()
	if s>>63 == 1 {
		x = -x
	}
	v[s%uint64(len(v))] += x
}
