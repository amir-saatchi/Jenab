package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Every request goes through send: HTTPS to the provider's own host only, the key only in the
// Authorization header, and errors redacted. Cloudflare's account ID is part of its URL path,
// so URLs are never printed or written.

var httpClient = &http.Client{Timeout: 120 * time.Second}

type reply struct {
	Status  int
	Ms      int64
	Body    []byte
	Headers map[string]string
	Err     string
}

func keepHeader(k string) bool {
	k = strings.ToLower(k)
	return k == "retry-after" || strings.HasPrefix(k, "x-ratelimit") || strings.HasPrefix(k, "ratelimit")
}

func send(ctx context.Context, host, url, keyName string, body any) reply {
	if !strings.HasPrefix(url, "https://"+host+"/") {
		return reply{Err: "refusing request to an unexpected host"}
	}
	b, err := json.Marshal(body)
	if err != nil {
		return reply{Err: "encoding the request: " + err.Error()}
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(b))
	if err != nil {
		return reply{Err: "building the request failed"}
	}
	req.Header.Set("Authorization", "Bearer "+secrets[keyName])
	req.Header.Set("Content-Type", "application/json")
	start := time.Now()
	resp, err := httpClient.Do(req)
	r := reply{Ms: time.Since(start).Milliseconds()}
	if err != nil {
		r.Err = "transport: " + redact(err.Error())
		return r
	}
	defer resp.Body.Close()
	r.Status = resp.StatusCode
	r.Body, err = io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	r.Ms = time.Since(start).Milliseconds()
	if err != nil {
		r.Err = "reading the body: " + redact(err.Error())
	}
	for k, v := range resp.Header {
		if keepHeader(k) {
			if r.Headers == nil {
				r.Headers = map[string]string{}
			}
			r.Headers[strings.ToLower(k)] = strings.Join(v, ", ")
		}
	}
	return r
}

func retryable(r reply) bool {
	return r.Err != "" && r.Status == 0 || r.Status == 429 || r.Status >= 500
}

// wait before the next try: the provider's retry-after, else 5, 15, 30 s.
func backoff(r reply, try int) time.Duration {
	if s, ok := r.Headers["retry-after"]; ok {
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n > 0 && n <= 120 {
			return time.Duration(n) * time.Second
		}
	}
	return []time.Duration{5 * time.Second, 15 * time.Second, 30 * time.Second}[min(try, 2)]
}

// stopped providers: a quota or daily-limit error, or a 429 that outlives the retries, stops
// every later call to that provider.
var (
	stopMu  sync.Mutex
	stopped = map[string]string{}
)

func stopProvider(p, why string) {
	stopMu.Lock()
	defer stopMu.Unlock()
	if stopped[p] == "" {
		stopped[p] = why
		logf("STOP %s: %s", p, why)
	}
}

func isStopped(p string) string {
	stopMu.Lock()
	defer stopMu.Unlock()
	return stopped[p]
}

func quotaLike(body string) bool {
	s := strings.ToLower(body)
	for _, w := range []string{"quota", "daily", "neuron", "usage limit", "weekly", "exceeded your", "insufficient", "billing"} {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// withRetries runs call up to 3 times on 429, 5xx and transport errors.
func withRetries(ctx context.Context, provider string, call func() reply) (reply, int) {
	var r reply
	for try := 0; try < 3; try++ {
		if why := isStopped(provider); why != "" {
			return reply{Err: "provider stopped: " + why}, try
		}
		r = call()
		if r.Status >= 400 && quotaLike(string(r.Body)) {
			stopProvider(provider, fmt.Sprintf("HTTP %d: %s", r.Status, short(redact(string(r.Body)), 300)))
			return r, try + 1
		}
		if !retryable(r) {
			return r, try + 1
		}
		if try < 2 {
			d := backoff(r, try)
			logf("%s: HTTP %d %s, retry in %s", provider, r.Status, short(r.Err, 80), d)
			select {
			case <-ctx.Done():
				return r, try + 1
			case <-time.After(d):
			}
		}
	}
	if r.Status == 429 {
		stopProvider(provider, "429 after 3 tries: "+short(redact(string(r.Body)), 300))
	}
	return r, 3
}

// ---- Cloudflare Workers AI (Clef) ----

const cfHost = "api.cloudflare.com"

func clefURL(model string) string {
	return "https://" + cfHost + "/client/v4/accounts/" + secrets["CLOUDFLARE_ID"] + "/ai/run/@cf/cloudflare/" + model
}

type clefBody struct {
	Model     string       `json:"model"`
	State     any          `json:"state"`
	Questions map[string]Q `json:"questions"`
	Images    []string     `json:"images,omitempty"`
}

type cfEnvelope struct {
	Result struct {
		Model   string          `json:"model"`
		Answers json.RawMessage `json:"answers"`
		Usage   struct {
			In  int `json:"input_tokens"`
			Out int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"result"`
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

// askClef sends a request (any body, so the error tests can send broken ones).
func askClef(ctx context.Context, model string, body any, retries bool) (Rec, map[string]Ans) {
	return askClefWith(ctx, model, "CLOUDFLARE_TOKEN", body, retries)
}

func askClefWith(ctx context.Context, model, keyName string, body any, retries bool) (Rec, map[string]Ans) {
	call := func() reply { return send(ctx, cfHost, clefURL(model), keyName, body) }
	var r reply
	tries := 1
	if retries {
		r, tries = withRetries(ctx, "cloudflare", call)
	} else {
		r = call()
	}
	rec := Rec{Model: model, Status: r.Status, Ms: r.Ms, Tries: tries, Err: r.Err, Headers: r.Headers}
	if r.Err != "" && r.Status == 0 {
		return rec, nil
	}
	var env cfEnvelope
	if err := json.Unmarshal(r.Body, &env); err != nil || r.Status != 200 || !env.Success {
		rec.Body = short(redact(string(r.Body)), 2000)
		if rec.Err == "" {
			rec.Err = fmt.Sprintf("HTTP %d", r.Status)
		}
		return rec, nil
	}
	rec.In, rec.Out = env.Result.Usage.In, env.Result.Usage.Out
	ans, err := parseClef(env.Result.Answers)
	if err != nil {
		rec.Err = "reading answers: " + err.Error()
		rec.Body = short(redact(string(r.Body)), 2000)
	}
	return rec, ans
}

// ---- OpenAI-compatible LLMs (baseline) ----

type llmProv struct {
	Name, Host, URL, Key string
	Model                string        // the API's model ID when it differs from the label
	Gap                  time.Duration // pause between calls (the lane runs one call at a time)
	In, Out              float64       // USD per million tokens, for the spending cap; 0 = free
}

// Workers AI LLMs, through its OpenAI-compatible endpoint. Prices are Cloudflare's (2026-10-04).
var llmProvs = map[string]llmProv{
	"gemma4:31b":    {Name: "ollama", Host: "ollama.com", URL: "https://ollama.com/v1/chat/completions", Key: "OLLAMA_API_KEY", Gap: time.Second},
	"glm-4.5-flash": {Name: "zai", Host: "api.z.ai", URL: "https://api.z.ai/api/paas/v4/chat/completions", Key: "Z_API_KEY", Gap: 2 * time.Second},
	"gemma-4-26b":   {Name: "workers-ai", Host: cfHost, Key: "CLOUDFLARE_TOKEN", Model: "@cf/google/gemma-4-26b-a4b-it", In: 0.1, Out: 0.3},
	"gpt-oss-120b":  {Name: "workers-ai", Host: cfHost, Key: "CLOUDFLARE_TOKEN", Model: "@cf/openai/gpt-oss-120b", In: 0.35, Out: 0.75},
	"qwen3-30b-a3b": {Name: "workers-ai", Host: cfHost, Key: "CLOUDFLARE_TOKEN", Model: "@cf/qwen/qwen3-30b-a3b-fp8", In: 0.0509, Out: 0.335},
}

func (p llmProv) url() string {
	if p.URL != "" {
		return p.URL
	}
	return "https://" + cfHost + "/client/v4/accounts/" + secrets["CLOUDFLARE_ID"] + "/ai/v1/chat/completions"
}

// The spending cap: Workers AI gives 10,000 free neurons a day ($0.011 per 1,000). Spend is
// counted from each reply's usage; past the cap, the provider stops.
var (
	spendMu   sync.Mutex
	spent     = map[string]float64{}
	budgetUSD = 0.088 // 8,000 neurons
	llmEffort string  // reasoning_effort for every LLM, "" = not sent
)

func charge(p llmProv, in, out int) {
	if p.In == 0 && p.Out == 0 {
		return
	}
	spendMu.Lock()
	spent[p.Name] += (p.In*float64(in) + p.Out*float64(out)) / 1e6
	s := spent[p.Name]
	spendMu.Unlock()
	if s > budgetUSD {
		stopProvider(p.Name, fmt.Sprintf("spending cap: $%.4f of $%.4f", s, budgetUSD))
	}
}

const llmSystem = `You answer typed questions about a state. The user message is a JSON object with "state" and "questions". Each question has an id, a type, instructions and criteria.
- noul: a yes/no question. Answer true or false.
- choice: pick one option. Answer with the option's id, exactly as written.
- score: rate on the ordered levels in criteria, lowest first, numbered from 0. Answer with the level number.
Reply with one JSON object only, mapping every question id to its answer, e.g. {"urgent": true, "team": "billing", "severity": 2}. No other text.`

func askLLM(ctx context.Context, model string, it *Item) (Rec, map[string]Ans) {
	p := llmProvs[model]
	user, _ := json.Marshal(map[string]any{"state": it.State, "questions": it.Qs})
	id := model
	if p.Model != "" {
		id = p.Model
	}
	body := map[string]any{
		"model": id,
		"messages": []map[string]string{
			{"role": "system", "content": llmSystem},
			{"role": "user", "content": string(user)},
		},
		"temperature":     0,
		"max_tokens":      4000,
		"response_format": map[string]string{"type": "json_object"},
	}
	if llmEffort != "" {
		body["reasoning_effort"] = llmEffort
	}
	r, tries := withRetries(ctx, p.Name, func() reply { return send(ctx, p.Host, p.url(), p.Key, body) })
	rec := Rec{Model: model, Status: r.Status, Ms: r.Ms, Tries: tries, Err: r.Err, Headers: r.Headers}
	if r.Status != 200 {
		rec.Body = short(redact(string(r.Body)), 1000)
		if rec.Err == "" {
			rec.Err = fmt.Sprintf("HTTP %d", r.Status)
		}
		return rec, nil
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			Prompt     int `json:"prompt_tokens"`
			Completion int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(r.Body, &out); err != nil || len(out.Choices) == 0 {
		rec.Err = "reading the reply"
		rec.Body = short(redact(string(r.Body)), 1000)
		return rec, nil
	}
	rec.In, rec.Out = out.Usage.Prompt, out.Usage.Completion
	charge(p, rec.In, rec.Out)
	text := out.Choices[0].Message.Content
	ans := parseLLM(text, it.Qs)
	for _, a := range ans {
		if a.Bad != "" {
			rec.Note = "raw: " + short(redact(text), 600)
			break
		}
	}
	return rec, ans
}

func short(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
