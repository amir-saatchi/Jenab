package provider

import (
	"net/http"
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	hdr := func(kv ...string) http.Header {
		h := http.Header{}
		for i := 0; i < len(kv); i += 2 {
			h.Set(kv[i], kv[i+1])
		}
		return h
	}
	// Bodies as the providers sent them in SPIKE-018, shortened.
	tests := []struct {
		name   string
		status int
		header http.Header
		body   string
		kind   ErrorKind
		wait   time.Duration
	}{
		{"gemini per minute", 429, nil,
			`[{"error":{"code":429,"message":"You exceeded your current quota, please check your plan and billing details. Quota exceeded for metric: generativelanguage.googleapis.com/generate_content_free_tier_input_token_count","status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.QuotaFailure","violations":[{"quotaId":"GenerateContentInputTokensPerModelPerMinute-FreeTier"}]},{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"52s"}]}}]`,
			RateLimited, 52 * time.Second},
		{"gemini per day", 429, nil,
			`[{"error":{"code":429,"message":"You exceeded your current quota. limit: 20, model: gemini-3.8-flash","details":[{"violations":[{"quotaId":"GenerateRequestsPerDayPerProjectPerModel-FreeTier"}]},{"retryDelay":"59s"}]}}]`,
			Quota, 0},
		{"groq tokens per minute", 429, hdr("retry-after", "12"),
			`{"error":{"message":"Rate limit reached for model openai/gpt-oss-120b on tokens per minute (TPM): Limit 8000, Used 7000. Please try again in 11.5s.","type":"tokens","code":"rate_limit_exceeded"}}`,
			RateLimited, 12 * time.Second},
		{"groq try again in, no header", 429, nil,
			`{"error":{"message":"Please try again in 850ms."}}`, RateLimited, 850 * time.Millisecond},
		{"groq tokens per day", 429, nil,
			`{"error":{"message":"Rate limit reached on tokens per day (TPD): Limit 200000"}}`, Quota, 0},
		{"zai overloaded", 429, nil, `{"code":"1305","message":"The service may be temporarily overloaded, please try again later"}`, RateLimited, 0},
		{"zai balance", 429, nil, `{"error":{"code":"1113","message":"Insufficient balance or no resource package."}}`, Quota, 0},
		{"openai insufficient quota", 429, nil, `{"error":{"type":"insufficient_quota","code":"insufficient_quota"}}`, Quota, 0},
		{"anthropic credits", 400, nil, `{"type":"error","error":{"type":"invalid_request_error","message":"Your credit balance is too low to access the Anthropic API."}}`, Quota, 0},
		{"anthropic overloaded", 529, hdr("retry-after-ms", "1500"), `{"type":"error","error":{"type":"overloaded_error"}}`, Overloaded, 1500 * time.Millisecond},
		{"too large", 413, nil, `{"error":{"message":"Request too large for model"}}`, TooLarge, 0},
		{"server error", 500, nil, `[{"error":{"code":500,"message":"Internal error encountered.","status":"INTERNAL"}}]`, Overloaded, 0},
		{"timeout", 408, nil, ``, Transport, 0},
		{"bad key", 401, nil, `{"error":{"message":"Incorrect API key provided"}}`, BadRequest, 0},
		{"not found", 404, nil, `[{"error":{"code":404,"message":"This model models/gemini-2.5-pro is no longer available to new users."}}]`, BadRequest, 0},
		{"wait too long", 429, hdr("retry-after", "3600"), `{"error":{"message":"slow down"}}`, Quota, 0},
		{"wait as a date", 503, hdr("retry-after", time.Now().Add(time.Hour).UTC().Format(http.TimeFormat)), ``, Quota, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := Classify("p", tt.status, tt.header, tt.body)
			if e.Kind != tt.kind || e.RetryAfter != tt.wait {
				t.Errorf("got %s wait %s, want %s wait %s", e.Kind, e.RetryAfter, tt.kind, tt.wait)
			}
			if e.Status != tt.status || e.Provider != "p" {
				t.Errorf("status %d provider %q", e.Status, e.Provider)
			}
			if want := tt.kind == RateLimited || tt.kind == Overloaded || tt.kind == Transport; e.Retryable() != want {
				t.Errorf("Retryable = %v", e.Retryable())
			}
		})
	}
}

func TestBackoff(t *testing.T) {
	for i, base := range []time.Duration{10, 20, 40, 80, 100, 100, 100} {
		base *= time.Second
		for range 50 {
			if d := Backoff(i); d < base || d > base+base/5 {
				t.Fatalf("Backoff(%d) = %s, want %s to %s", i, d, base, base+base/5)
			}
		}
	}
}

func TestShortenKeepsRunes(t *testing.T) {
	s := shorten("سلام دنیا، این یک پیام خطای طولانی است", 9)
	for _, r := range s {
		if r == '�' {
			t.Fatalf("cut inside a rune: %q", s)
		}
	}
}

func TestCatalog(t *testing.T) {
	c := MustCatalog()
	for _, k := range []Kind{KindAnthropic, KindOpenAI, KindGemini} {
		def, fast := c.Suggested(k)
		if def == "" || fast == "" {
			t.Errorf("%s: default %q, fast %q", k, def, fast)
		}
		for _, m := range c.Models(k) {
			if m.Context <= 0 || m.Output <= 0 || m.Output > m.Context {
				t.Errorf("%s/%s: context %d, output %d", k, m.ID, m.Context, m.Output)
			}
		}
	}
	if _, err := ParseCatalog([]byte(`{"models":[{"kind":"openai","id":"a","name":"A"},{"kind":"openai","id":"a","name":"A"}]}`)); err == nil {
		t.Error("a model listed twice was accepted")
	}
	if _, err := ParseCatalog([]byte(`{"models":[{"kind":"nope","id":"a","name":"A"}]}`)); err == nil {
		t.Error("an unknown kind was accepted")
	}
}
