package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Keys are read from the repo-root .env by name, into memory. They are never printed, logged,
// written or put on a command line; errors are redacted. Gemini requests are counted per Pacific
// day in out/requests-<date>.txt, and the spike refuses to pass 380 (the rule is under 400 a day).

const keyName = "GEMINI_API_KEY"
const dailyCap = 380

var geminiKey string

// loaded holds every key read, so redact can hide all of them.
var loaded []string

func loadKey(envPath string) error {
	v, err := readEnvKey(envPath, keyName)
	geminiKey = v
	return err
}

func readEnvKey(envPath, name string) (string, error) {
	f, err := os.Open(envPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	val := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimPrefix(strings.TrimSpace(sc.Text()), "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(k) != name {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		}
		val = v
	}
	if val == "" {
		return "", fmt.Errorf("%s not found in %s", name, envPath)
	}
	loaded = append(loaded, val)
	return val, sc.Err()
}

func redact(s string) string {
	for _, k := range loaded {
		if len(k) >= 8 {
			s = strings.ReplaceAll(s, k, "[REDACTED]")
		}
	}
	return s
}

type Gemini struct {
	Model   string
	OutDir  string
	http    http.Client
	lastReq time.Time
}

func pacificDay() string {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		loc = time.FixedZone("PST", -8*3600)
	}
	return time.Now().In(loc).Format("2006-01-02")
}

func (g *Gemini) countFile() string { return filepath.Join(g.OutDir, "requests-"+pacificDay()+".txt") }

func (g *Gemini) used() int {
	b, _ := os.ReadFile(g.countFile())
	n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return n
}

func (g *Gemini) bump() error {
	n := g.used()
	if n >= dailyCap {
		return fmt.Errorf("daily cap reached: %d requests today (Pacific)", n)
	}
	os.MkdirAll(g.OutDir, 0o755)
	return os.WriteFile(g.countFile(), []byte(strconv.Itoa(n+1)), 0o644)
}

var reRetry = regexp.MustCompile(`"retryDelay":\s*"(\d+)(\.\d+)?s"`)

// post sends one request, retrying 429 and 5xx up to 4 times. Every attempt counts.
func (g *Gemini) post(method string, body any) ([]byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	u := "https://generativelanguage.googleapis.com/v1beta/models/" + g.Model + ":" + method
	for attempt := 0; ; attempt++ {
		if err := g.bump(); err != nil {
			return nil, err
		}
		if gap := 4*time.Second - time.Since(g.lastReq); gap > 0 { // free tier: keep under ~15 a minute
			time.Sleep(gap)
		}
		g.lastReq = time.Now()
		req, _ := http.NewRequest("POST", u, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-goog-api-key", geminiKey)
		g.http.Timeout = 120 * time.Second
		resp, err := g.http.Do(req)
		if err != nil {
			if attempt < 4 {
				time.Sleep(5 * time.Second)
				continue
			}
			return nil, fmt.Errorf("%s", redact(err.Error()))
		}
		rb, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == 200 {
			return rb, nil
		}
		if (resp.StatusCode == 429 || resp.StatusCode >= 500) && attempt < 4 {
			wait := 10 * time.Second
			if m := reRetry.FindSubmatch(rb); m != nil {
				s, _ := strconv.Atoi(string(m[1]))
				wait = time.Duration(s+1) * time.Second
			}
			fmt.Fprintf(os.Stderr, "  gemini %d, retry in %s\n", resp.StatusCode, wait)
			time.Sleep(wait)
			continue
		}
		return nil, fmt.Errorf("gemini %d: %s", resp.StatusCode, redact(clip(string(rb), 600)))
	}
}

func (g *Gemini) CountTokens(text string) (int, error) {
	rb, err := g.post("countTokens", map[string]any{"contents": []any{map[string]any{"role": "user",
		"parts": []any{map[string]any{"text": text}}}}})
	if err != nil {
		return 0, err
	}
	var r struct {
		TotalTokens int `json:"totalTokens"`
	}
	err = json.Unmarshal(rb, &r)
	return r.TotalTokens, err
}

// Content and Part keep the model's parts as raw JSON, so thought signatures go back unchanged.
type Content struct {
	Role  string            `json:"role"`
	Parts []json.RawMessage `json:"parts"`
}

type FunctionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
	ID   string          `json:"id,omitempty"`
}

type Usage struct {
	Prompt   int `json:"promptTokenCount"`
	Output   int `json:"candidatesTokenCount"`
	Thoughts int `json:"thoughtsTokenCount"`
	Cached   int `json:"cachedContentTokenCount"`
	Total    int `json:"totalTokenCount"`
}

type GenResult struct {
	Content Content
	Calls   []FunctionCall
	Text    string
	Usage   Usage
	Finish  string
}

func (g *Gemini) Generate(system string, contents []Content, tools []any) (*GenResult, error) {
	rb, err := g.post("generateContent", map[string]any{
		"systemInstruction": map[string]any{"parts": []any{map[string]any{"text": system}}},
		"contents":          contents,
		"tools":             []any{map[string]any{"functionDeclarations": tools}},
	})
	if err != nil {
		return nil, err
	}
	var r struct {
		Candidates []struct {
			Content      Content `json:"content"`
			FinishReason string  `json:"finishReason"`
		} `json:"candidates"`
		Usage Usage `json:"usageMetadata"`
	}
	if err := json.Unmarshal(rb, &r); err != nil {
		return nil, err
	}
	if len(r.Candidates) == 0 {
		return nil, fmt.Errorf("no candidates: %s", clip(string(rb), 300))
	}
	c := r.Candidates[0]
	res := &GenResult{Content: c.Content, Usage: r.Usage, Finish: c.FinishReason}
	if res.Content.Role == "" {
		res.Content.Role = "model"
	}
	for _, p := range c.Content.Parts {
		var part struct {
			Text         string        `json:"text"`
			Thought      bool          `json:"thought"`
			FunctionCall *FunctionCall `json:"functionCall"`
		}
		json.Unmarshal(p, &part)
		if part.FunctionCall != nil {
			res.Calls = append(res.Calls, *part.FunctionCall)
		} else if part.Text != "" && !part.Thought {
			res.Text += part.Text
		}
	}
	return res, nil
}

func textPart(s string) json.RawMessage {
	b, _ := json.Marshal(map[string]string{"text": s})
	return b
}

func responsePart(call FunctionCall, result string) json.RawMessage {
	fr := map[string]any{"name": call.Name, "response": map[string]any{"result": result}}
	if call.ID != "" {
		fr["id"] = call.ID
	}
	b, _ := json.Marshal(map[string]any{"functionResponse": fr})
	return b
}
