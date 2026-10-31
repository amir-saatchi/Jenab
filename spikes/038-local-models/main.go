// Command 038-local-models is SPIKE-038's speed probe: how long a local
// Ollama model takes to read prompts of about 2k, 8k and 16k tokens and
// how fast it writes, from Ollama's own timings on a direct /api/chat.
//
//	go run ./spikes/038-local-models -model granite4:350m -out spikes/038-local-models/results/speed.jsonl
//
// The prompt is synthetic text, seeded per request so no two share a
// prefix Ollama could reuse, then a request to summarize it. Thinking is
// off where the model has it. The first request is a tiny warm-up, so the
// load time is apart. The model is unloaded at the end (keep_alive 0).
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

func main() {
	var (
		base    = flag.String("base", "http://127.0.0.1:11434", "the Ollama server")
		model   = flag.String("model", "", "the model")
		sizes   = flag.String("sizes", "2000,8000,16000", "prompt sizes in tokens, comma-separated, smallest first")
		numCtx  = flag.Int("ctx", 32768, "num_ctx")
		predict = flag.Int("predict", 200, "num_predict")
		limit   = flag.Duration("timeout", 10*time.Minute, "the longest one request may take; larger sizes are skipped after it")
		out     = flag.String("out", "", "a JSONL file the rows are added to")
	)
	flag.Parse()
	if *model == "" {
		fmt.Fprintln(os.Stderr, "-model is needed")
		os.Exit(2)
	}
	var targets []int
	for _, s := range strings.Split(*sizes, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			fmt.Fprintln(os.Stderr, "-sizes:", err)
			os.Exit(2)
		}
		targets = append(targets, n)
	}
	p := &probe{base: strings.TrimSuffix(*base, "/"), model: *model, ctx: *numCtx, predict: *predict, limit: *limit}
	if err := p.run(targets, *out); err != nil {
		fmt.Fprintln(os.Stderr, "probe:", err)
		os.Exit(1)
	}
}

type probe struct {
	base, model  string
	ctx, predict int
	limit        time.Duration
	think        *bool
}

// Row is one request's result.
type Row struct {
	Time            string  `json:"time"`
	Model           string  `json:"model"`
	Target          int     `json:"target"` // 0 for the warm-up
	Words           int     `json:"words"`
	Think           *bool   `json:"think,omitempty"`
	NumCtx          int     `json:"num_ctx"`
	PromptTokens    int     `json:"prompt_tokens"`
	PromptSeconds   float64 `json:"prompt_seconds"`
	PromptPerSecond float64 `json:"prompt_per_second"`
	OutputTokens    int     `json:"output_tokens"`
	OutputSeconds   float64 `json:"output_seconds"`
	OutputPerSecond float64 `json:"output_per_second"`
	LoadSeconds     float64 `json:"load_seconds"`
	TotalSeconds    float64 `json:"total_seconds"` // Ollama's total_duration
	WallSeconds     float64 `json:"wall_seconds"`
	DoneReason      string  `json:"done_reason,omitempty"`
	Error           string  `json:"error,omitempty"`
}

func (p *probe) run(targets []int, out string) error {
	caps, err := p.capabilities()
	if err != nil {
		return err
	}
	if slices.Contains(caps, "thinking") {
		f := false
		p.think = &f
	}
	defer p.unload()
	rows := []Row{p.ask(0, 0, "Say hi in three words.", 8)}
	wordsPerToken := 0.75 // a first guess; the first size measures it
	for _, t := range targets {
		n := int(float64(t) * wordsPerToken)
		r := p.ask(t, n, text(n, uint64(t)*7919+uint64(time.Now().UnixNano()))+"\n\nSummarize the text above in about 150 words.", p.predict)
		rows = append(rows, r)
		if r.Error != "" {
			break // the larger sizes take longer still
		}
		if r.PromptTokens > 0 {
			wordsPerToken = float64(n) / float64(r.PromptTokens)
		}
	}
	for _, r := range rows {
		fmt.Printf("%-16s target %5d  prompt %6d tok in %7.1fs (%6.1f tok/s)  output %4d tok in %6.1fs (%5.1f tok/s)  load %5.1fs  wall %7.1fs  %s%s\n",
			r.Model, r.Target, r.PromptTokens, r.PromptSeconds, r.PromptPerSecond, r.OutputTokens, r.OutputSeconds, r.OutputPerSecond, r.LoadSeconds, r.WallSeconds, r.DoneReason, r.Error)
	}
	if out == "" {
		return nil
	}
	f, err := os.OpenFile(out, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, r := range rows {
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	return nil
}

func (p *probe) capabilities() ([]string, error) {
	var s struct {
		Capabilities []string `json:"capabilities"`
	}
	err := p.post(context.Background(), "/api/show", map[string]any{"model": p.model}, &s)
	return s.Capabilities, err
}

// ask sends one prompt and reads Ollama's timings.
func (p *probe) ask(target, words int, prompt string, predict int) Row {
	r := Row{Time: time.Now().Format(time.RFC3339), Model: p.model, Target: target, Words: words, Think: p.think, NumCtx: p.ctx}
	body := map[string]any{
		"model":      p.model,
		"messages":   []map[string]string{{"role": "user", "content": prompt}},
		"stream":     false,
		"keep_alive": "10m",
		"options":    map[string]int{"num_ctx": p.ctx, "num_predict": predict},
	}
	if p.think != nil {
		body["think"] = *p.think
	}
	var a struct {
		PromptEvalCount    int    `json:"prompt_eval_count"`
		PromptEvalDuration int64  `json:"prompt_eval_duration"`
		EvalCount          int    `json:"eval_count"`
		EvalDuration       int64  `json:"eval_duration"`
		LoadDuration       int64  `json:"load_duration"`
		TotalDuration      int64  `json:"total_duration"`
		DoneReason         string `json:"done_reason"`
	}
	ctx, cancel := context.WithTimeout(context.Background(), p.limit)
	defer cancel()
	start := time.Now()
	err := p.post(ctx, "/api/chat", body, &a)
	r.WallSeconds = time.Since(start).Seconds()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			r.Error = fmt.Sprintf("over %s, stopped", p.limit)
		} else {
			r.Error = err.Error()
		}
		return r
	}
	sec := func(ns int64) float64 { return float64(ns) / 1e9 }
	r.PromptTokens, r.PromptSeconds = a.PromptEvalCount, sec(a.PromptEvalDuration)
	r.OutputTokens, r.OutputSeconds = a.EvalCount, sec(a.EvalDuration)
	r.LoadSeconds, r.TotalSeconds, r.DoneReason = sec(a.LoadDuration), sec(a.TotalDuration), a.DoneReason
	if r.PromptSeconds > 0 {
		r.PromptPerSecond = float64(r.PromptTokens) / r.PromptSeconds
	}
	if r.OutputSeconds > 0 {
		r.OutputPerSecond = float64(r.OutputTokens) / r.OutputSeconds
	}
	return r
}

// unload asks Ollama to drop the model from memory now.
func (p *probe) unload() {
	if err := p.post(context.Background(), "/api/generate", map[string]any{"model": p.model, "keep_alive": 0}, nil); err != nil {
		fmt.Fprintln(os.Stderr, "unload:", err)
	}
}

func (p *probe) post(ctx context.Context, path string, body, v any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", p.base+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("%s: HTTP %d: %s", path, resp.StatusCode, bytes.TrimSpace(data))
	}
	if v == nil {
		return nil
	}
	return json.Unmarshal(data, v)
}

// text is n words of plain synthetic English in sentences, from seed.
func text(n int, seed uint64) string {
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	var b strings.Builder
	for i, left := 0, 0; i < n; i++ {
		w := vocabulary[rng.IntN(len(vocabulary))]
		if left == 0 {
			if i > 0 {
				b.WriteString(". ")
			}
			left = 8 + rng.IntN(10)
			w = strings.ToUpper(w[:1]) + w[1:]
		} else {
			b.WriteByte(' ')
		}
		b.WriteString(w)
		left--
	}
	b.WriteByte('.')
	return b.String()
}

var vocabulary = strings.Fields(`
the a garden school report water tomato basil carrot bed soil team week
morning evening children parents group plan harvest weather rain sun leaf
root seed plant grow small large green early late every each many few
market price coin table query answer project chat memory note list day
month year number value total change rule decision goal open question
because while after before during under over between through about
river mountain city village road bridge house window door kitchen street
writes reads keeps finds moves holds shows turns brings takes gives makes
quiet bright cold warm heavy light quick slow simple clear careful useful
teacher student farmer driver doctor baker painter writer reader neighbor
book letter paper story picture song game lesson message letter idea
`)
