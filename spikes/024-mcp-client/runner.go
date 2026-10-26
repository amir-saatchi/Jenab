package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// env is what the agent runs share.
type env struct {
	h       *harness
	fake    string
	scratch string
	out     string
	budget  *budget
}

// budget caps the Gemini free tier: requests counted from every results
// file, and a stop once a daily quota error comes.
type budget struct {
	mu      sync.Mutex
	max     int
	used    int
	stopped bool
}

func (b *budget) ok(model string) bool {
	if !strings.HasPrefix(model, "gemini/") {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return !b.stopped && b.used < b.max
}

func (b *budget) add(model string, o runOut) {
	if !strings.HasPrefix(model, "gemini/") {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.used += o.Requests
	for _, e := range append(o.Errors, o.Err) {
		l := strings.ToLower(e)
		if strings.Contains(l, "perday") || strings.Contains(l, "per day") || strings.Contains(l, "daily") {
			b.stopped = true
		}
	}
}

// countGemini adds the Gemini requests already in the results files.
func (b *budget) countGemini(paths ...string) {
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 64<<20)
		for sc.Scan() {
			var r struct {
				Model    string `json:"model"`
				Requests int    `json:"requests"`
			}
			if json.Unmarshal(sc.Bytes(), &r) == nil && strings.HasPrefix(r.Model, "gemini/") {
				b.used += r.Requests
			}
		}
		f.Close()
	}
}

func appendJSON(path string, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		log.Print(err)
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Print(err)
		return
	}
	defer f.Close()
	f.Write(append(b, '\n'))
}

func (e *env) transcript(name, prompt, card string, o runOut, extra string) {
	e.transcriptTo(filepath.Join(e.out, "transcripts"), name, prompt, card, o, extra)
}

func (e *env) transcriptTo(dir, name, prompt, card string, o runOut, extra string) {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n**Prompt:** %s\n\n", name, prompt)
	if extra != "" {
		b.WriteString(extra + "\n\n")
	}
	fmt.Fprintf(&b, "Requests %d · prompt tokens %d (cached %d) · output %d · first request %d tokens, %d tools · %.0fs\n\n",
		o.Requests, o.Prompt, o.Cached, o.Output, o.First, o.Tools, o.Seconds)
	if o.Err != "" {
		fmt.Fprintf(&b, "**Run error:** %s\n\n", o.Err)
	}
	for _, er := range o.Errors {
		fmt.Fprintf(&b, "- request error: %s\n", er)
	}
	b.WriteString("## Calls\n\n")
	for i, c := range o.Calls {
		mark := ""
		if c.Error {
			mark = " **(error)**"
		}
		fmt.Fprintf(&b, "%d. `%s` %s%s\n   → %s\n", i+1, c.Tool, clip(c.Args, 400), mark, strings.ReplaceAll(clip(c.Result, 400), "\n", " ⏎ "))
	}
	b.WriteString("\n## Cards\n\n")
	for _, c := range o.Cards {
		fmt.Fprintf(&b, "- %s %s → %s %s\n", c.Tool, clip(c.Args, 200), c.Grant, c.Note)
	}
	fmt.Fprintf(&b, "\n## Answer\n\n%s\n", o.Answer)
	if card != "" {
		fmt.Fprintf(&b, "\n## Block 4\n\n```\n%s\n```\n", card)
	}
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, name+".md"), []byte(b.String()), 0o644)
}

func (e *env) inject(ctx context.Context, models, styles, conds []string, reps int) {
	path := filepath.Join(e.out, "inject.jsonl")
	done := map[string]bool{}
	if f, err := os.Open(path); err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 64<<20)
		for sc.Scan() {
			var r injResult
			if json.Unmarshal(sc.Bytes(), &r) == nil && r.Err == "" {
				done[r.name()] = true
			}
		}
		f.Close()
	}
	for rep := 1; rep <= reps; rep++ {
		for _, t := range injTasks {
			for _, cond := range conds {
				for _, style := range styles {
					for _, m := range models {
						key := injResult{Model: m, Style: style, Cond: cond, Task: t.ID, Rep: rep}.name()
						if done[key] || ctx.Err() != nil {
							continue
						}
						if !e.budget.ok(m) {
							log.Printf("skip %s: Gemini budget", key)
							continue
						}
						r := e.injectRun(ctx, m, style, cond, t, rep)
						e.budget.add(m, r.runOut)
						appendJSON(path, r)
						extra := fmt.Sprintf("Condition %s. Attempted: %v · carded %d · executed: %v · task done %v · told the user %v",
							cond, r.Attempted, r.Carded, r.Executed, r.TaskDone, r.Told)
						e.transcript(r.name(), t.Prompt, "", r.runOut, extra)
						log.Printf("%s: req %d, attempted %d, carded %d, executed %d, done %v, told %v %s", key, r.Requests,
							len(r.Attempted), r.Carded, len(r.Executed), r.TaskDone, r.Told, r.Err)
					}
				}
			}
		}
	}
}
