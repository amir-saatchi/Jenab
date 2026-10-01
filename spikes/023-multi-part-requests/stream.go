package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
)

// One streamed Chat Completions call through openai-go/v3, with the SPIKE-012 rules:
// complete only with a finish_reason, ctx.Err() checked, tool arguments must be valid JSON,
// include_usage set, SDK retries off and our own retry loop with a capped retry-after.

// Temperature sent with every request (SPIKE-021: 0.2; below 0 = provider default).
var Temperature = 0.2

const (
	maxRetries    = 4                // our retry count (backoff 2, 4, 8, 16 s; Z.ai 1305 overload can last)
	retryAfterCap = 30 * time.Second // we cap retry-after; above the cap we give up
)

type Call struct {
	ID, Name, Args string
	Extra          string // raw extra_content (Gemini thought signature)
}

type Req struct {
	Label     string
	Messages  []openai.ChatCompletionMessageParamUnion
	Tools     []openai.ChatCompletionToolUnionParam
	MaxTokens int
	Parallel  bool // send parallel_tool_calls: true
	NoPace    bool // burst test: skip the pacer
	EstTokens int  // rough prompt size, for the pacer
}

type Result struct {
	Text, Reasoning string
	Calls           []Call
	Finish          string
	Prompt, Output  int
	Cached          int
	CachedSeen      bool // prompt_tokens_details.cached_tokens present
	UsageSeen       bool
	UsageWhere      string // where the usage chunk came, relative to finish_reason
	TTFT, Total     time.Duration
	Attempts        int
	Status          int
	Headers         string
	Err             error
	ErrText         string
	Notes           []string
}

// ---- 429 / error log ----

type RLEvent struct {
	Model, Label string
	Attempt      int
	Status       int
	Headers      string
	Msg          string
	Hint         string // wait hint found in the error body
	Action       string
}

var (
	rlMu  sync.Mutex
	rlLog []RLEvent
)

func logRL(e RLEvent) {
	rlMu.Lock()
	if e.Hint == "" {
		e.Hint = retryHintInBody(e.Msg)
	}
	rlLog = append(rlLog, e)
	rlMu.Unlock()
}

// ---- pacer: min gap per model plus Groq's token headers ----

type pacer struct {
	mu        sync.Mutex
	last      time.Time
	remTokens int // -1 unknown
	resetAt   time.Time
	known     bool
}

func (p *pacer) wait(gap time.Duration, est int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	until := p.last.Add(gap)
	if p.known && est > p.remTokens && time.Now().Before(p.resetAt) {
		if p.resetAt.After(until) {
			until = p.resetAt
		}
	}
	if d := time.Until(until); d > 0 {
		if d > 65*time.Second {
			d = 65 * time.Second
		}
		time.Sleep(d)
	}
	p.last = time.Now()
}

func (p *pacer) update(c *capture) {
	rem, err1 := strconv.Atoi(c.get("x-ratelimit-remaining-tokens"))
	reset, err2 := time.ParseDuration(c.get("x-ratelimit-reset-tokens"))
	if err1 != nil || err2 != nil {
		return
	}
	p.mu.Lock()
	p.known, p.remTokens, p.resetAt = true, rem, time.Now().Add(reset+500*time.Millisecond)
	p.mu.Unlock()
}

// ---- the call ----

func stream(ctx context.Context, m *Model, r Req) Result {
	var res Result
	for attempt := 0; ; attempt++ {
		if why := m.Stopped(); why != "" {
			res.Err = fmt.Errorf("skipped: %s", why)
			res.ErrText = res.Err.Error()
			return res
		}
		if !r.NoPace {
			gap := m.P.MinGap
			if strings.Contains(m.ID, "pro") {
				gap = 13 * time.Second // Gemini Pro free tier: about 5 RPM
			}
			m.pace.wait(gap, r.EstTokens+r.MaxTokens)
		}
		one := streamOnce(ctx, m, r)
		one.Attempts = attempt + 1
		one.Notes = append(res.Notes, one.Notes...)
		res = one
		if res.Err == nil {
			return res
		}
		st := res.Status
		// reasoning_effort not accepted: drop it once and retry at once (not counted).
		if st == 400 && m.Effort != "" && (strings.Contains(strings.ToLower(res.ErrText), "reasoning") || strings.Contains(strings.ToLower(res.ErrText), "thinking")) {
			res.Notes = append(res.Notes, fmt.Sprintf("reasoning_effort %q rejected, sent without it", m.Effort))
			m.Effort = ""
			attempt--
			continue
		}
		// Transport errors (connection reset, unexpected EOF, TLS timeout, attempt timeout) and
		// streams cut off before finish_reason: nothing was stored yet, so the request is retried.
		// An error event inside the stream (Groq tool_use_failed) is the model's output: not retried.
		transport := (st == 0 || st == 200) && !strings.Contains(res.ErrText, "received error while streaming")
		if transport {
			st = 0
		}
		if st != 0 && st != 429 && st != 500 && st != 502 && st != 503 && st != 504 {
			logRL(RLEvent{m.Name(), r.Label, attempt + 1, st, res.Headers, res.ErrText, "", "not retried"})
			if st == 404 || st == 401 || st == 402 || st == 403 {
				m.stop(fmt.Sprintf("status %d", st))
			}
			return res
		}
		ra, has := parseRetryAfter(firstNonEmpty(res.hdr("retry-after")))
		lower := strings.ToLower(res.ErrText)
		daily := strings.Contains(lower, "perday") || strings.Contains(lower, "per day") || strings.Contains(lower, "(tpd)") ||
			strings.Contains(lower, "(rpd)") || strings.Contains(lower, "weekly") || strings.Contains(lower, "usage limit") ||
			strings.Contains(lower, "credits") || strings.Contains(lower, "insufficient balance") ||
			strings.Contains(lower, `"1113"`) || strings.Contains(lower, `"1308"`) || strings.Contains(lower, `"1310"`)
		action := ""
		switch {
		case daily:
			action = "daily/quota limit: stop this model"
			m.stop("daily or quota limit")
		case attempt >= maxRetries:
			action = "retries used up"
		case has && ra > retryAfterCap:
			action = fmt.Sprintf("retry-after %s above cap %s: give up", ra.Round(time.Second), retryAfterCap)
		default:
			wait := ra
			if !has {
				wait = time.Duration(1<<attempt) * 2 * time.Second // our backoff when no header
			}
			action = fmt.Sprintf("retry after %s", wait.Round(100*time.Millisecond))
			if !has {
				action += " (no retry-after header, backoff)"
			}
			logRL(RLEvent{m.Name(), r.Label, attempt + 1, st, res.Headers, res.ErrText, "", action})
			time.Sleep(wait)
			continue
		}
		logRL(RLEvent{m.Name(), r.Label, attempt + 1, st, res.Headers, res.ErrText, "", action})
		return res
	}
}

func (r *Result) hdr(k string) string {
	for _, kv := range strings.Fields(r.Headers) {
		if a, b, ok := strings.Cut(kv, "="); ok && a == k {
			return b
		}
	}
	return ""
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

func streamOnce(parent context.Context, m *Model, r Req) (res Result) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	defer cancel()
	ctx, cap := withCapture(ctx)
	prm := openai.ChatCompletionNewParams{
		Model:         shared.ChatModel(m.ID),
		Messages:      r.Messages,
		StreamOptions: openai.ChatCompletionStreamOptionsParam{IncludeUsage: openai.Bool(true)},
	}
	if r.MaxTokens > 0 {
		if m.P.Name == "ollama" {
			prm.MaxTokens = openai.Int(int64(r.MaxTokens)) // Ollama documents max_tokens
		} else {
			prm.MaxCompletionTokens = openai.Int(int64(r.MaxTokens))
		}
	}
	if len(r.Tools) > 0 {
		prm.Tools = r.Tools
		if r.Parallel {
			prm.ParallelToolCalls = openai.Bool(true)
		}
	}
	if Temperature >= 0 {
		prm.Temperature = openai.Float(Temperature)
	}
	if m.Effort != "" {
		prm.ReasoningEffort = shared.ReasoningEffort(m.Effort)
	}
	t0 := time.Now()
	st := m.P.client.Chat.Completions.NewStreaming(ctx, prm)
	defer st.Close()
	type tc struct{ id, name, args, extra string }
	var calls []*tc
	idxSlot := map[int]int{}
	reused := 0
	defer func() {
		if reused > 0 {
			res.Notes = append(res.Notes, fmt.Sprintf("%d tool call(s) reused an index with a new id", reused))
		}
	}()
	var text, reas strings.Builder
	sawFinish := false
	for st.Next() {
		ch := st.Current()
		if ch.JSON.Usage.Valid() && ch.Usage.PromptTokens > 0 {
			res.UsageSeen = true
			switch {
			case sawFinish:
				res.UsageWhere = "own chunk after finish"
			case len(ch.Choices) > 0 && ch.Choices[0].FinishReason != "":
				res.UsageWhere = "same chunk as finish"
			default:
				res.UsageWhere = "before finish"
			}
			res.Prompt, res.Output = int(ch.Usage.PromptTokens), int(ch.Usage.CompletionTokens)
			if ch.Usage.PromptTokensDetails.JSON.CachedTokens.Valid() {
				res.CachedSeen = true
				m.noteCached(int(ch.Usage.PromptTokensDetails.CachedTokens))
				res.Cached = int(ch.Usage.PromptTokensDetails.CachedTokens)
			}
		} else if xg, ok := ch.JSON.ExtraFields["x_groq"]; ok && !res.UsageSeen {
			// Groq also reports usage under x_groq.usage
			var v struct {
				Usage *struct {
					PromptTokens     int `json:"prompt_tokens"`
					CompletionTokens int `json:"completion_tokens"`
				} `json:"usage"`
			}
			if json.Unmarshal([]byte(xg.Raw()), &v) == nil && v.Usage != nil && v.Usage.PromptTokens > 0 {
				res.Notes = append(res.Notes, "usage only in x_groq")
			}
		}
		if len(ch.Choices) == 0 {
			continue
		}
		c := ch.Choices[0]
		got := false
		for _, f := range []string{"reasoning", "reasoning_content"} {
			if rf, ok := c.Delta.JSON.ExtraFields[f]; ok && rf.Raw() != "" && rf.Raw() != "null" {
				var s string
				if json.Unmarshal([]byte(rf.Raw()), &s) == nil && s != "" {
					reas.WriteString(s)
					got = true
				}
			}
		}
		if c.Delta.Content != "" {
			text.WriteString(c.Delta.Content)
			got = true
		}
		for _, d := range c.Delta.ToolCalls {
			got = true
			// Calls are keyed by index, but a delta with a new id at a used index starts a new call
			// (seen with Gemini: parallel calls all arrive with index 0).
			slot, ok := idxSlot[int(d.Index)]
			if ok && d.ID != "" && calls[slot].id != "" && d.ID != calls[slot].id {
				ok = false
				reused++
			}
			if !ok {
				calls = append(calls, &tc{id: d.ID, name: d.Function.Name})
				slot = len(calls) - 1
				idxSlot[int(d.Index)] = slot
			}
			i := slot
			if calls[i].name == "" && d.Function.Name != "" {
				calls[i].name = d.Function.Name
			}
			calls[i].args += d.Function.Arguments
			// Gemini: thought signature on the tool call; it must be sent back unchanged.
			if x, ok := d.JSON.ExtraFields["extra_content"]; ok && x.Raw() != "" && x.Raw() != "null" {
				calls[i].extra = x.Raw()
			}
		}
		if got && res.TTFT == 0 {
			res.TTFT = time.Since(t0)
		}
		if c.FinishReason != "" {
			res.Finish = c.FinishReason
			sawFinish = true
		}
	}
	res.Total = time.Since(t0)
	res.Status = cap.status
	res.Headers = cap.hdrString()
	m.pace.update(cap)
	res.Text, res.Reasoning = text.String(), reas.String()
	if err := st.Err(); err != nil {
		res.Err = err
		var oe *openai.Error
		if errors.As(err, &oe) {
			res.Status = oe.StatusCode
			res.ErrText = redact(fmt.Sprintf("%d %s", oe.StatusCode, oe.RawJSON()))
			if oe.RawJSON() == "" {
				res.ErrText = fmt.Sprintf("%d %s", oe.StatusCode, firstNonEmpty(cap.body, redact(err.Error())))
			}
		} else {
			res.ErrText = redact(err.Error())
		}
		res.ErrText = short(res.ErrText, 4000)
		return res
	}
	if ctx.Err() != nil {
		res.Err = ctx.Err()
		res.ErrText = "context: " + ctx.Err().Error()
		return res
	}
	if !sawFinish {
		res.Err = errors.New("stream ended without finish_reason (truncated)")
		res.ErrText = res.Err.Error()
		return res
	}
	for _, c := range calls {
		if c == nil {
			continue
		}
		if !json.Valid([]byte(c.args)) {
			res.Notes = append(res.Notes, fmt.Sprintf("tool %s: arguments not valid JSON", c.name))
		}
		res.Calls = append(res.Calls, Call{c.id, c.name, c.args, c.extra})
	}
	return res
}

var (
	reQuota = regexp.MustCompile(`"quotaId":\s*"([^"]+)"`)
	reDelay = regexp.MustCompile(`"retryDelay":\s*"([^"]+)"`)
)

var reTryIn = regexp.MustCompile(`(?i)try again in ([0-9.]+m?s|[0-9.]+m[0-9.]+s)`)

func parseRetryAfter(v string) (time.Duration, bool) {
	if v == "" {
		return 0, false
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return time.Duration(f * float64(time.Second)), true
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d, true
	}
	return 0, false
}

// retryHintInBody finds a wait hint in the error body (Groq "try again in 7.6s", Gemini "retryDelay").
func retryHintInBody(s string) string {
	if m := reQuota.FindStringSubmatch(s); m != nil {
		d := ""
		if x := reDelay.FindStringSubmatch(s); x != nil {
			d = " retryDelay " + x[1]
		}
		return m[1] + d
	}
	if m := reTryIn.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	if i := strings.Index(s, `"retryDelay"`); i >= 0 {
		rest := s[i+len(`"retryDelay"`):]
		if j := strings.Index(rest, `"`); j >= 0 {
			rest = rest[j+1:]
			if k := strings.Index(rest, `"`); k >= 0 {
				return rest[:k]
			}
		}
	}
	return ""
}
