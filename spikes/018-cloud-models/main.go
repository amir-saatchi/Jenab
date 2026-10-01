package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/openai/openai-go/v3"
)

var (
	flagProvider = flag.String("provider", "", "comma-separated providers to run: groq,ollama,gemini,zai (default all)")
	flagQuick    = flag.Bool("quick", false, "basic test, 3 tool tasks, 2 cache turns; no 32k and no burst tests")
	flagList     = flag.Bool("list", false, "only list the models each provider reports")
	flagEnv      = flag.String("env", "../../.env", "path of the .env file with the keys")
	flagScan     = flag.Bool("scan", false, "only run the secret scan of this folder")

	out     *redactWriter
	errOut  *redactWriter
	inTok   atomic.Int64 // prompt tokens reported by the providers
	started = time.Now()
)

const tokenBudget = 1_400_000

func logf(format string, a ...any) {
	fmt.Fprintf(errOut, "[%5.0fs] "+format+"\n", append([]any{time.Since(started).Seconds()}, a...)...)
}

func main() {
	flag.Parse()
	out = &redactWriter{w: os.Stdout}
	errOut = &redactWriter{w: os.Stderr}
	defer out.Flush()
	if err := loadEnv(*flagEnv); err != nil {
		fmt.Fprintln(os.Stderr, "cannot read the .env file:", err)
		os.Exit(1)
	}
	if *flagScan {
		fmt.Println(scanLine())
		return
	}
	initTools()
	want := map[string]bool{}
	for _, p := range strings.Split(*flagProvider, ",") {
		if p = strings.TrimSpace(p); p != "" {
			want[p] = true
		}
	}
	var provs []*Prov
	for _, p := range providers {
		if len(want) == 0 || want[p.Name] {
			provs = append(provs, p)
		}
	}
	ctx := context.Background()

	fmt.Fprintf(out, "# SPIKE-018 results\n\nRun %s, %s %s/%s, openai-go/v3 v3.66.0. Flags: provider=%q quick=%v.\n\n",
		started.Format("2006-01-02 15:04"), runtime.Version(), runtime.GOOS, runtime.GOARCH, *flagProvider, *flagQuick)
	fmt.Fprintf(out, "Rules used: SDK retries 0, our loop retries %d times, retry-after capped at %s (above the cap: give up). "+
		"A stream is complete only with a finish_reason. Tool list: %d tools (SPEC 8.1), %d bytes of definitions.\n\n",
		maxRetries, retryAfterCap, len(toolSpecs), toolJSON)

	// ---- 0. models ----
	fmt.Fprintf(out, "## 0. Providers and models\n\n")
	for _, p := range provs {
		if err := p.setup(ctx); err != nil && p.client == nil {
			fmt.Fprintf(out, "- **%s**: %s\n", p.Name, err)
			continue
		} else if err != nil {
			fmt.Fprintf(out, "- **%s** `%s`: /models failed: %s\n", p.Name, p.BaseURL, short(redact(err.Error()), 200))
			continue
		}
		var ids []string
		for id := range p.Listed {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		fmt.Fprintf(out, "- **%s** `%s`: /models lists %d models: %s\n", p.Name, p.BaseURL, len(ids), "`"+strings.Join(ids, "`, `")+"`")
		if p.HostNote != "" {
			fmt.Fprintf(out, "  - host: %s\n", p.HostNote)
		}
		resolve(p)
	}
	if *flagList {
		return
	}
	fmt.Fprintf(out, "\n| Provider | Wanted | Model ID tested | In /models | Context limit (provider) | reasoning_effort sent |\n|---|---|---|---|---|---|\n")
	for _, p := range provs {
		for _, m := range p.Models {
			if m.Listed || p.Name == "zai" {
				m.Context = contextLimit(m)
			}
			fmt.Fprintf(out, "| %s | %s | `%s` | %v | %s | %s |\n", p.Name, m.Want, m.ID, yn(m.Listed), m.Context, orDash(m.Effort))
		}
	}
	fmt.Fprintln(out)
	var probes []string
	for _, p := range provs {
		for _, id := range p.Probes {
			if p.client == nil {
				continue
			}
			m := &Model{P: p, ID: id, Want: id}
			_, m.Listed = p.Listed[id]
			r := do(ctx, m, Req{Label: "probe", Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("Say ok.")}, MaxTokens: 50})
			res := "works"
			if r.Err != nil {
				res = short(r.ErrText, 260)
			}
			probes = append(probes, fmt.Sprintf("| %s | `%s` | %s | %s |", p.Name, id, yn(m.Listed), cell(res)))
		}
	}
	if len(probes) > 0 {
		fmt.Fprintf(out, "Availability probes (one tiny request each):\n\n| Provider | Model | In /models | Result |\n|---|---|---|---|\n%s\n\n", strings.Join(probes, "\n"))
	}

	// ---- run ----
	var mu sync.Mutex
	results := map[*Model]*ModelResult{}
	var bursts []string
	var wg sync.WaitGroup
	for _, p := range provs {
		if p.client == nil {
			continue
		}
		wg.Add(1)
		go func(p *Prov) {
			defer wg.Done()
			var pw sync.WaitGroup
			for _, m := range p.Models {
				run := func(m *Model) {
					r := runModel(ctx, m)
					mu.Lock()
					results[m] = r
					mu.Unlock()
				}
				if p.Parallel {
					pw.Add(1)
					go func(m *Model) { defer pw.Done(); run(m) }(m)
				} else {
					run(m)
				}
			}
			pw.Wait()
			if !*flagQuick {
				b := burst(ctx, p)
				mu.Lock()
				bursts = append(bursts, b)
				mu.Unlock()
			}
		}(p)
	}
	wg.Wait()
	var all []*ModelResult
	for _, p := range provs {
		for _, m := range p.Models {
			if r := results[m]; r != nil {
				all = append(all, r)
			}
		}
	}
	report(all, bursts)
	fmt.Fprintf(out, "\nTotal prompt tokens reported by the providers: %d. Run time: %.1f min.\n", inTok.Load(), time.Since(started).Minutes())
	out.Flush()

	// ---- secret scan ----
	line := scanLine()
	fmt.Fprintf(out, "\n%s\n", line)
	fmt.Fprintln(errOut, line)
	errOut.Flush()
}

// scanLine scans every file in the spike folder for the key values and
// returns only "secret scan: clean" or the file names.
func scanLine() string {
	dir, _ := os.Getwd()
	if exe, err := os.Executable(); err == nil && !strings.Contains(exe, "go-build") {
		dir = filepath.Dir(exe)
	}
	if hits := secretScan(dir); len(hits) > 0 {
		return "secret scan: key value found in " + strings.Join(hits, ", ")
	}
	return "secret scan: clean"
}

func yn(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func orDash(s string) string {
	if s == "" {
		return "–"
	}
	return s
}

// resolve maps the wanted names to listed IDs.
func resolve(p *Prov) {
	for _, w := range p.Want {
		id, listed := "", false
		for _, alt := range strings.Split(w, "|") {
			if _, ok := p.Listed[alt]; ok {
				id, listed = alt, true
				break
			}
			if strings.Contains(alt, "*") { // groq qwen*27b
				a, b, _ := strings.Cut(alt, "*")
				var cands []string
				for l := range p.Listed {
					if strings.Contains(strings.ToLower(l), a) && strings.HasSuffix(strings.ToLower(l), b) {
						cands = append(cands, l)
					}
				}
				sort.Strings(cands)
				if len(cands) > 0 {
					id, listed = cands[len(cands)-1], true
					break
				}
			}
			if !strings.Contains(alt, ":") { // ollama: "nemotron-3-super" may be listed with a tag
				var cands []string
				for l := range p.Listed {
					if strings.HasPrefix(l, alt+":") {
						cands = append(cands, l)
					}
				}
				sort.Strings(cands)
				if len(cands) > 0 {
					id, listed = cands[0], true
					break
				}
			}
		}
		if id == "" {
			id = strings.Split(w, "|")[0]
		}
		m := &Model{P: p, Want: w, ID: id, Listed: listed}
		if strings.Contains(id, "gpt-oss") || strings.HasPrefix(id, "gemini-") {
			m.Effort = "low"
		}
		p.Models = append(p.Models, m)
	}
}

// ---- per-model tests ----

type TaskResult struct {
	ID, Kind string
	Pass     bool
	Why      string
	Steps    int
	Time     time.Duration
	Signed   bool // a tool call carried extra_content (thought signature)
}

type CtxResult struct {
	Label  string
	Target int
	R      Result
}

type ModelResult struct {
	M                      *Model
	Basic                  Result
	BasicPass              bool
	BasicWhy               string
	Tasks                  []TaskResult
	CallsValid, CallsTotal int
	BadArgs                []string
	Cache                  []Result
	Ctx                    []CtxResult
	T1Chars, T1Tok         int         // size of the t1 tool request, for calibration
	NoEcho                 *TaskResult // Gemini t9 without sending extra_content back
	IndexReuse             int         // responses where parallel calls reused a tool-call index
}

func estTok(msgs []openai.ChatCompletionMessageParamUnion, tools bool) int {
	n := 0
	for _, m := range msgs {
		n += msgChars(m)
	}
	if tools {
		n += toolJSON
	}
	return n / 3
}

func do(ctx context.Context, m *Model, r Req) Result {
	if inTok.Load() > tokenBudget {
		return Result{Err: fmt.Errorf("token budget used up"), ErrText: "skipped: token budget used up"}
	}
	if r.EstTokens == 0 {
		r.EstTokens = estTok(r.Messages, len(r.Tools) > 0)
	}
	res := stream(ctx, m, r)
	inTok.Add(int64(res.Prompt))
	m.mu.Lock()
	if res.Err == nil {
		m.calls++
		m.failStreak = 0
	} else if !strings.HasPrefix(res.ErrText, "skipped") {
		m.failStreak++
		if m.failStreak >= 4 && m.stopped == "" {
			m.stopped = "4 failed requests in a row"
		}
	}
	m.mu.Unlock()
	st := "ok"
	if res.Err != nil {
		st = short(res.ErrText, 140)
	}
	logf("%-8s %-28s %-12s %s (%.1fs, in %d, cached %d)", m.P.Name, m.ID, r.Label, st, res.Total.Seconds(), res.Prompt, res.Cached)
	return res
}

func runModel(ctx context.Context, m *Model) *ModelResult {
	mr := &ModelResult{M: m}
	if !m.Listed {
		logf("%s: %s not in /models, trying anyway", m.P.Name, m.ID)
	}
	// 1. basic
	msgs := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage("You are a test assistant. Answer briefly."),
		openai.UserMessage("Reply with the word pong, then list three colors separated by commas."),
	}
	b := do(ctx, m, Req{Label: "basic", Messages: msgs, MaxTokens: 400})
	mr.Basic = b
	switch {
	case b.Err != nil:
		mr.BasicWhy = short(b.ErrText, 160)
	case !has(b.Text, "pong"):
		mr.BasicWhy = fmt.Sprintf("no pong in text (finish %s, %d chars)", b.Finish, len(b.Text))
	case !b.UsageSeen:
		mr.BasicWhy = "no usage chunk"
	default:
		mr.BasicPass = true
	}
	if b.Err != nil && m.Stopped() != "" {
		return mr
	}
	// 2. tools
	for _, t := range tasks {
		if *flagQuick && !quickTasks[t.ID] {
			continue
		}
		mr.Tasks = append(mr.Tasks, runTask(ctx, m, t, mr, true))
	}
	// 4. cache (before context, so we can calibrate chars per token)
	nonce := fmt.Sprintf("%x", time.Now().UnixNano())
	turns := 3
	if *flagQuick {
		turns = 2
	}
	cmsgs, cchars := burrowRequest(18, nonce, 9000, "In one sentence: which coins appear in the query results above?")
	ctok := 0
	for i := 0; i < turns; i++ {
		r := do(ctx, m, Req{Label: fmt.Sprintf("cache/t%d", i+1), Messages: cmsgs, Tools: oaiTools, MaxTokens: 300})
		mr.Cache = append(mr.Cache, r)
		if r.Err != nil {
			break
		}
		if i == 0 {
			ctok = r.Prompt
		}
		am := openai.ChatCompletionAssistantMessageParam{}
		am.Content.OfString = openai.String(firstNonEmpty(r.Text, "(no text)"))
		cmsgs = append(cmsgs, openai.ChatCompletionMessageParamUnion{OfAssistant: &am},
			openai.UserMessage([]string{"And which coin had the highest price? One sentence.", "Thanks. Which coin had the lowest price? One sentence."}[i%2]))
	}
	// chars for a target size. The history (number-heavy rows) has fewer chars per token than the
	// fixed part, so the slope comes from cache turn 1 minus the fixed part: the t1 tool request
	// (tools + short system prompt) plus the longer Burrow system prompt counted at 4 chars/token.
	extraSys := msgChars(openai.SystemMessage(systemBlocks(nonce))) - msgChars(openai.SystemMessage(toolSystem))
	slope := 0.0
	if ctok > 0 {
		slope = float64(cchars) / float64(ctok)
		fixedTok := float64(mr.T1Tok) + float64(extraSys)/4
		if mr.T1Tok > 0 && float64(ctok) > fixedTok+200 {
			slope = float64(cchars-mr.T1Chars-extraSys) / (float64(ctok) - fixedTok)
		}
	}
	baseChars, baseTok := cchars, ctok
	charsFor := func(target int) int {
		if baseTok == 0 {
			return target * 2
		}
		return baseChars + int(float64(target-baseTok)*slope)
	} // 3. context
	sizes := []int{8000, 32000}
	if *flagQuick {
		sizes = sizes[:1]
	}
	q := "In one sentence: what was the most recent date in the last query result above?"
	for i, sz := range sizes {
		cm, chars := burrowRequest(int64(100+i), fmt.Sprintf("%x", time.Now().UnixNano()), charsFor(sz), q)
		r := do(ctx, m, Req{Label: fmt.Sprintf("ctx/%dk", sz/1000), Messages: cm, Tools: oaiTools, MaxTokens: 400, EstTokens: sz})
		mr.Ctx = append(mr.Ctx, CtxResult{fmt.Sprintf("%dk", sz/1000), sz, r})
		if r.Err == nil && baseTok > 0 && r.Prompt > baseTok+500 {
			// refine with a history-only slope for the next size
			slope = float64(chars-baseChars) / float64(r.Prompt-baseTok)
			baseChars, baseTok = chars, r.Prompt
		}
		if r.Err != nil && r.Status == 413 && sz == 8000 {
			// too large for the tier: also measure the largest size that should fit
			cm, _ := burrowRequest(int64(150), fmt.Sprintf("%x", time.Now().UnixNano()), charsFor(5500), q)
			r2 := do(ctx, m, Req{Label: "ctx/5.5k", Messages: cm, Tools: oaiTools, MaxTokens: 400, EstTokens: 7500})
			mr.Ctx = append(mr.Ctx, CtxResult{"5.5k (fallback)", 5500, r2})
		}
	}
	// Gemini: the same 2-step task without sending the thought signature back
	if m.P.Name == "gemini" && !*flagQuick {
		signed := false
		for _, t := range mr.Tasks {
			signed = signed || t.Signed
		}
		if signed {
			tr := runTask(ctx, m, tasks[8], &ModelResult{}, false)
			mr.NoEcho = &tr
		}
	}
	return mr
}
func runTask(ctx context.Context, m *Model, t task, mr *ModelResult, echo bool) TaskResult {
	tr := TaskResult{ID: t.ID, Kind: t.Kind}
	t0 := time.Now()
	defer func() { tr.Time = time.Since(t0) }()
	msgs := []openai.ChatCompletionMessageParamUnion{openai.SystemMessage(toolSystem), openai.UserMessage(t.Prompt)}
	for si, st := range t.Steps {
		tr.Steps = si + 1
		r := do(ctx, m, Req{Label: fmt.Sprintf("%s/s%d", t.ID, si+1), Messages: msgs, Tools: oaiTools, MaxTokens: 1024, Parallel: t.Parallel})
		if r.Err != nil {
			tr.Why = fmt.Sprintf("s%d error: %s", si+1, short(r.ErrText, 300))
			return tr
		}
		if t.ID == "t1" && si == 0 {
			mr.T1Tok, mr.T1Chars = r.Prompt, toolJSON
			for _, x := range msgs {
				mr.T1Chars += msgChars(x)
			}
		}
		for _, n := range r.Notes {
			if strings.Contains(n, "reused an index") {
				mr.IndexReuse++
			}
		}
		var names []string
		for _, c := range r.Calls {
			names = append(names, c.Name)
			mr.CallsTotal++
			if err := validArgs(c); err != nil {
				mr.BadArgs = append(mr.BadArgs, fmt.Sprintf("%s %s: %s", t.ID, c.Name, err))
			} else {
				mr.CallsValid++
			}
		}
		if len(st.Tools) == 0 {
			if len(r.Calls) > 0 {
				tr.Why = fmt.Sprintf("s%d: called %v, expected an answer", si+1, names)
				return tr
			}
			if w := st.Answer(r.Text); w != "" {
				tr.Why = fmt.Sprintf("s%d: %s (finish %s: %q)", si+1, w, r.Finish, short(r.Text, 60))
				return tr
			}
			tr.Pass = true
			return tr
		}
		exp := append([]string(nil), st.Tools...)
		sort.Strings(exp)
		got := append([]string(nil), names...)
		sort.Strings(got)
		if strings.Join(exp, ",") != strings.Join(got, ",") {
			tr.Why = fmt.Sprintf("s%d: called %v, expected %v (finish %s)", si+1, names, st.Tools, r.Finish)
			if len(names) == 0 {
				tr.Why += fmt.Sprintf(" text %q", short(r.Text, 60))
			}
			return tr
		}
		for _, c := range r.Calls {
			if err := validArgs(c); err != nil {
				tr.Why = fmt.Sprintf("s%d: %s args invalid: %s", si+1, c.Name, err)
				return tr
			}
		}
		if w := st.Check(r.Calls); w != "" {
			tr.Why = fmt.Sprintf("s%d: %s", si+1, w)
			return tr
		}
		if si == len(t.Steps)-1 {
			tr.Pass = true
			return tr
		}
		am := openai.ChatCompletionAssistantMessageParam{}
		if r.Text != "" {
			am.Content.OfString = openai.String(r.Text)
		}
		for _, c := range r.Calls {
			fc := &openai.ChatCompletionMessageFunctionToolCallParam{
				ID: c.ID, Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{Name: c.Name, Arguments: c.Args}}
			if c.Extra != "" {
				tr.Signed = true
				if echo {
					// Gemini: send extra_content (thought_signature) back unchanged.
					fc.SetExtraFields(map[string]any{"extra_content": json.RawMessage(c.Extra)})
				}
			}
			am.ToolCalls = append(am.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{OfFunction: fc})
		}
		msgs = append(msgs, openai.ChatCompletionMessageParamUnion{OfAssistant: &am})
		for _, c := range r.Calls {
			msgs = append(msgs, openai.ToolMessage(st.Result(c), c.ID))
		}
	}
	return tr
}

// ---- 5. burst tests ----

func burst(ctx context.Context, p *Prov) string {
	var m *Model
	pick := map[string]string{"groq": "gpt-oss-20b", "gemini": "lite", "ollama": "gpt-oss:20b", "zai": "glm-4.7-flash"}[p.Name]
	for _, x := range p.Models {
		if strings.Contains(x.ID, pick) && x.Stopped() == "" {
			m = x
		}
	}
	if m == nil {
		return fmt.Sprintf("| %s | – | skipped (model stopped or missing) |", p.Name)
	}
	var lines []string
	switch p.Name {
	case "groq":
		// 4 requests of about 3.5k tokens back to back: 14k tokens against an 8k TPM limit.
		for i := 0; i < 4; i++ {
			msgs, _ := burrowRequest(int64(300+i), fmt.Sprintf("%x", time.Now().UnixNano()), 9000, "Say ok.")
			r := do(ctx, m, Req{Label: fmt.Sprintf("burst/%d", i+1), Messages: msgs, Tools: oaiTools, MaxTokens: 50, NoPace: true})
			lines = append(lines, burstLine(i+1, r))
		}
	case "gemini":
		for i := 0; i < 16; i++ {
			msgs := []openai.ChatCompletionMessageParamUnion{openai.UserMessage("Say ok.")}
			r := do(ctx, m, Req{Label: fmt.Sprintf("burst/%d", i+1), Messages: msgs, MaxTokens: 50, NoPace: true})
			lines = append(lines, burstLine(i+1, r))
			if r.Err != nil {
				break
			}
		}
	case "ollama", "zai":
		// Ollama free plan: 1 concurrent request; Z.ai free models: concurrency limit. Send 2 at once.
		var wg sync.WaitGroup
		rs := make([]Result, 2)
		for i := range rs {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				msgs := []openai.ChatCompletionMessageParamUnion{openai.UserMessage("Count from 1 to 20, separated by spaces.")}
				rs[i] = do(ctx, m, Req{Label: fmt.Sprintf("burst/%d", i+1), Messages: msgs, MaxTokens: 200, NoPace: true})
			}(i)
		}
		wg.Wait()
		for i, r := range rs {
			lines = append(lines, burstLine(i+1, r)+fmt.Sprintf(" total %.1fs", r.Total.Seconds()))
		}
	}
	return fmt.Sprintf("| %s | `%s` | %s |", p.Name, m.ID, strings.Join(lines, "; "))
}

func burstLine(i int, r Result) string {
	if r.Err != nil {
		return fmt.Sprintf("#%d failed after %d attempt(s): %s", i, r.Attempts, short(r.ErrText, 90))
	}
	return fmt.Sprintf("#%d ok after %d attempt(s)", i, r.Attempts)
}
