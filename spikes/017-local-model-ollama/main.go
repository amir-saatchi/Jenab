// SPIKE-017: can Burrow be developed and tested against qwen3.5:4b on Ollama on the dev laptop?
//
// Usage: go run . > results.md          (about an hour of model time)
//
//	go run . -quick > results.md   (a smaller set, about 15 minutes)
//
// Needs a running Ollama server with qwen3.5:4b pulled. With -serverlog pointing at the log of an
// `ollama serve` started with OLLAMA_DEBUG=1, the prefix-reuse table also shows Ollama's own
// "loading cache slot" counts.
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/openai/openai-go/v3"
)

var (
	host      = flag.String("host", "http://127.0.0.1:11434", "Ollama base URL")
	model     = flag.String("model", "qwen3.5:4b", "base model")
	quick     = flag.Bool("quick", false, "smaller run")
	serverLog = flag.String("serverlog", "", "path of the ollama serve log (OLLAMA_DEBUG=1) for cache-slot lines")
	threads   = flag.Int("threads", 10, "num_thread for the derived models")
	keep      = flag.Bool("keep", false, "keep the derived models")
	maxReq    = flag.Duration("maxreq", 6*time.Minute, "timeout for one request")
	only_     = flag.String("sections", "facts,think,threads,trunc,speed,reuse,tools", "sections to run (for development)")
	taskIDs   = flag.String("tasks", "", "comma list of task IDs to run (for development)")
	sizesFlag = flag.String("sizes", "", "comma list of speed sizes in tokens (for development)")
)

func on(name string) bool { return contains(strings.Split(*only_, ","), name) }

var slog *srvLog
var start = time.Now()

// progress goes to stderr so stdout stays the report.
func progress(f string, a ...any) {
	fmt.Fprintf(os.Stderr, "[%5.1f min] %s\n", time.Since(start).Minutes(), fmt.Sprintf(f, a...))
}

type section struct {
	strings.Builder
}

func (s *section) p(f string, a ...any) { fmt.Fprintf(s, f+"\n", a...) }

var mems []memRow

type memRow struct {
	Label   string
	Ctx     int
	PsSize  int64
	Mem     memSample
	KvCache string
	Total   string
}

func sampleMem(label, mdl string, li logInfo) {
	m := loaded(mdl)
	r := memRow{Label: label, Mem: memNow(), KvCache: li.KvCache, Total: li.TotalMem}
	if m != nil {
		r.Ctx, r.PsSize = m.ContextLength, m.Size
	}
	mems = append(mems, r)
}

func derived(ctx int) string { return fmt.Sprintf("burrow-qwen3.5-4b-%dk", ctx/1024) }

func main() {
	flag.Parse()
	initClient()
	loadTools()
	if *serverLog != "" {
		slog = &srvLog{path: *serverLog}
	}
	var ver struct{ Version string }
	if err := apiGet("/api/version", &ver); err != nil {
		fmt.Fprintln(os.Stderr, "Ollama is not answering at", *host, ":", err)
		os.Exit(1)
	}
	m0 := memNow()
	var head, facts, think, thr, trunc, speed, reuse, tool section

	mdl12, mdl20, mdl36 := derived(12288), derived(20480), derived(36864)
	defer func() {
		if !*keep {
			for _, m := range []string{mdl12, mdl20, mdl36} {
				unload(m)
				_ = deleteModel(m)
			}
		}
	}()

	head.p("# SPIKE-017 results\n")
	head.p("Ollama %s, model `%s`, openai-go v3.66.0, Go %s, %s. %s, %s logical CPUs, %s RAM (%s available at start).",
		ver.Version, *model, runtime.Version(), time.Now().Format("2006-01-02 15:04"), strings.TrimSpace(os.Getenv("PROCESSOR_IDENTIFIER")), os.Getenv("NUMBER_OF_PROCESSORS"), gibu(m0.TotalPhys), gibu(m0.AvailPhys))
	if *quick {
		head.p("\n**Quick run** (-quick): fewer sizes and tasks.")
	}
	head.p("\nOther programs were running on the laptop (editors, browsers, other agents); timings include that contention.")

	if on("facts") {
		modelFacts(&facts)
	}
	if on("think") {
		thinkSwitches(&think)
	}
	if !*quick && on("threads") {
		threadTest(&thr)
	}
	if on("trunc") {
		truncation(&trunc)
	}

	for _, c := range []int{12288, 20480, 36864} {
		t0 := time.Now()
		err := createDerived(derived(c), map[string]any{"num_ctx": c, "num_thread": *threads})
		progress("created %s in %.1f s (err %v)", derived(c), time.Since(t0).Seconds(), err)
		if err != nil {
			facts.p("\nCreating %s failed: %v", derived(c), err)
		}
	}
	facts.p("\n**Derived models** (`POST /api/create` with `{\"model\": \"%s\", \"from\": \"%s\", \"parameters\": {\"num_ctx\": 12288, \"num_thread\": %d}}`, same for %s and %s). They share the weights blob, so creating one takes about a second and no disk space.",
		mdl12, *model, *threads, mdl20, mdl36)

	speed.p("## 4. Speed\n")
	speed.p("Burrow-shaped requests (SPEC 3.1: system prompt, user and project memory, project card, session notes, history turns with query/page previews of up to 1,500 tokens, current turn). No tool list in these requests (see section 6 for its size). Streaming over /v1. Each thinking-off request starts with a new request id, so nothing is reused. max_tokens = 200 with thinking off, 400 with thinking on (thinking uses the same budget).")
	speed.p("\n- TTFT = time to the first streamed token (reasoning or answer). With thinking off this is almost all prompt processing, so prompt tok/s = prompt tokens / TTFT.\n- The thinking-on request repeats the same prompt right after the thinking-off one, so its TTFT shows a reused prefix (section 5), not a cold start.\n- \"code / ticket\" = the answer contains the code word (start of the system prompt) / the ticket number (first history message).")
	speed.p("\n| size | model (num_ctx) | thinking | prompt tokens | TTFT | prompt tok/s | output tokens | output tok/s | first answer token | total | code / ticket | finish |\n|---|---|---|---|---|---|---|---|---|---|---|---|")
	sizes := []int{2000, 8000, 16000, 32000}
	if *quick {
		sizes = []int{2000}
	}
	if *sizesFlag != "" {
		sizes = nil
		for _, f := range strings.Split(*sizesFlag, ",") {
			var n int
			fmt.Sscan(f, &n)
			sizes = append(sizes, n)
		}
	}
	if !on("speed") {
		sizes = nil
	}
	var lastRate float64
	for _, size := range sizes {
		mdl := mdl12
		if size > 10000 {
			mdl = mdl20
		}
		if size > 18000 {
			mdl = mdl36
		}
		if lastRate > 0 && float64(size)/lastRate > maxReq.Seconds()*1.15 {
			est := float64(size) / lastRate / 60
			speed.p("| %dk | %s | off | – | – | – | – | – | – | – | – | not run: at %.0f tok/s it needs about %.0f min |", size/1000, mdl, lastRate, est)
			memOnly(mdl, fmt.Sprintf("%s loaded, no request", mdl))
			continue
		}
		slog.mark()
		warm(mdl)
		msgs := burrowRequest(shape{Target: size, Nonce: fmt.Sprintf("speed-%d-%d", size, time.Now().UnixNano())}, checkQuestion)
		progress("speed %d off", size)
		r := stream(req{Model: mdl, Msgs: msgs, MaxTokens: 200, Think: thinkOff, Timeout: *maxReq})
		li := slog.info()
		speed.p(speedRow(size, mdl, "off", r))
		sampleMem(fmt.Sprintf("after %dk request", size/1000), mdl, li)
		if r.Err != nil {
			if r.timedOut() {
				lastRate = float64(size) / maxReq.Seconds() * 0.9
			}
			continue
		}
		lastRate = float64(r.PromptTok) / r.TTFT.Seconds()
		if size <= 8000 {
			progress("speed %d on", size)
			r2 := stream(req{Model: mdl, Msgs: msgs, MaxTokens: 400, Timeout: *maxReq})
			speed.p(speedRow(size, mdl, "on", r2))
		}
	}
	speed.p("\nThinking on was only run up to 8k: prompt processing is the same, and the time budget does not allow it at 16k.")

	// Reuse and tools run on the 12k model; reuse's num_ctx case uses the 20k model.
	if on("reuse") {
		prefixReuse(&reuse, mdl12, mdl20)
	}
	if on("tools") {
		toolCalling(&tool, mdl12)
	}
	for _, m := range []string{mdl12, mdl20, mdl36} {
		unload(m)
	}

	fmt.Print(head.String(), "\n", facts.String(), "\n", think.String(), "\n", trunc.String(), "\n")
	if !*quick {
		fmt.Print(thr.String(), "\n")
	}
	fmt.Print(speed.String(), "\n", reuse.String(), "\n", tool.String(), "\n")
	memTable()
	fmt.Printf("\nTotal run time: %.1f min.\n", time.Since(start).Minutes())
}

func speedRow(size int, mdl, th string, r *res) string {
	code, ticket := checkAnswer(r.Content)
	yn := func(b bool) string {
		if b {
			return "yes"
		}
		return "no"
	}
	if r.Err != nil {
		return fmt.Sprintf("| %dk | %s | %s | – | %s | – | – | – | – | %s | – | %s |", size/1000, mdl, th, sec(r.TTFT), sec(r.Total), oneLine(r.Err.Error()))
	}
	gen := r.Total - r.TTFT
	return fmt.Sprintf("| %dk | %s | %s | %d | %s | %s | %d | %s | %s | %s | %s / %s | %s |", size/1000, mdl, th, r.PromptTok, sec(r.TTFT), rate(r.PromptTok, r.TTFT),
		r.OutTok, rate(r.OutTok, gen), sec(r.FirstContent), sec(r.Total), yn(code), yn(ticket), r.Finish)
}

// warm loads a model (and unloads any other) with a tiny request, so load time is not in the timings.
func warm(mdl string) time.Duration {
	only(mdl)
	if loaded(mdl) != nil {
		return 0
	}
	t0 := time.Now()
	r := stream(req{Model: mdl, Msgs: []openai.ChatCompletionMessageParamUnion{user("Hi")}, MaxTokens: 1, Think: thinkOff, Timeout: *maxReq})
	progress("loaded %s in %.1f s (err %v)", mdl, time.Since(t0).Seconds(), r.Err)
	return time.Since(t0)
}

func memOnly(mdl, label string) {
	slog.mark()
	warm(mdl)
	sampleMem(label, mdl, slog.info())
}

// ---------------- 1. model facts and default context ----------------

func modelFacts(s *section) {
	var show struct {
		Capabilities []string       `json:"capabilities"`
		Details      map[string]any `json:"details"`
		ModelInfo    map[string]any `json:"model_info"`
		Parameters   string         `json:"parameters"`
	}
	must(apiPost("/api/show", map[string]any{"model": *model}, &show))
	mi := show.ModelInfo
	s.p("## 1. Model facts\n")
	s.p("| field | value |\n|---|---|")
	s.p("| architecture | %v (%v layers; every %vth layer is full attention, the others are linear-attention/SSM layers) |", mi["general.architecture"], mi["qwen35.block_count"], mi["qwen35.full_attention_interval"])
	s.p("| parameters | %v (%v) |", show.Details["parameter_size"], mi["general.parameter_count"])
	s.p("| quantization | %v |", show.Details["quantization_level"])
	s.p("| model context length | %v tokens |", mi["qwen35.context_length"])
	s.p("| capabilities | %s |", strings.Join(show.Capabilities, ", "))
	s.p("| default parameters | %s |", strings.Join(strings.Fields(show.Parameters), " "))

	// Default context: load the base model with a /v1 request and read /api/ps.
	only(*model)
	unload(*model)
	slog.mark()
	r := stream(req{Model: *model, Msgs: []openai.ChatCompletionMessageParamUnion{user("Hi")}, MaxTokens: 1, Think: thinkOff})
	li := slog.info()
	ctx := 0
	if m := loaded(*model); m != nil {
		ctx = m.ContextLength
	}
	progress("base model loaded, ctx %d (err %v)", ctx, r.Err)
	s.p("\n**Default context.** A plain /v1 request loads `%s` with a context of **%d tokens** (`/api/ps` context_length%s). Ollama picks this default from VRAM; this laptop has none.", *model, ctx, kvNote(li))
	sampleMem("base model, default context", *model, li)
	if app, kv := appServerConfig(); app != "" {
		s.p("\nThe Ollama tray app starts its own server with `OLLAMA_CONTEXT_LENGTH=%s`; its last runner used KvSize %s. So the context a /v1 request gets depends on how the server was started, not on the request.", app, kv)
	}
}

func kvNote(li logInfo) string {
	if li.KvSize == 0 {
		return ""
	}
	return fmt.Sprintf("; server log: KvSize %d, NumThreads %d", li.KvSize, li.Threads)
}

// ---------------- 2. thinking switches and /v1 settings ----------------

func thinkSwitches(s *section) {
	s.p("## 2. Thinking and other settings over /v1\n")
	s.p("Prompt \"Say hi in three words.\", max_tokens 60, base model.\n")
	s.p("| how | reasoning streamed | answer | completion tokens | finish |\n|---|---|---|---|---|")
	q := "Say hi in three words."
	cases := []struct {
		name  string
		msgs  []openai.ChatCompletionMessageParamUnion
		eff   string
		extra map[string]any
	}{
		{"nothing set (model default)", []openai.ChatCompletionMessageParamUnion{user(q)}, "", nil},
		{"`reasoning_effort: \"none\"`", []openai.ChatCompletionMessageParamUnion{user(q)}, "none", nil},
		{"`reasoning_effort: \"low\"`", []openai.ChatCompletionMessageParamUnion{user(q)}, "low", nil},
		{"extra field `think: false`", []openai.ChatCompletionMessageParamUnion{user(q)}, "", map[string]any{"think": false}},
		{"`/no_think` at the end of the user message", []openai.ChatCompletionMessageParamUnion{user(q + " /no_think")}, "", nil},
		{"`/no_think` in the system prompt", []openai.ChatCompletionMessageParamUnion{sys("/no_think"), user(q)}, "", nil},
	}
	for _, c := range cases {
		progress("think switch: %s", c.name)
		r := stream(req{Model: *model, Msgs: c.msgs, MaxTokens: 60, Effort: c.eff, Extra: c.extra})
		if r.Err != nil {
			s.p("| %s | error: %s | | | |", c.name, oneLine(r.Err.Error()))
			continue
		}
		s.p("| %s | %s | %q | %d | %s |", c.name, yesNo(r.Reasoning != "", fmt.Sprintf("yes (%d chars)", len(r.Reasoning))), trunc(r.Content, 40), r.OutTok, r.Finish)
	}
	nr, err := nativeChat(map[string]any{"model": *model, "messages": []map[string]string{{"role": "user", "content": q}}, "think": false, "options": map[string]any{"num_predict": 60}})
	if err == nil {
		s.p("| native `/api/chat` with `think: false` | %s | %q | %d | %s |", yesNo(nr.Message.Thinking != "", "yes"), trunc(nr.Message.Content, 40), nr.EvalCount, nr.DoneReason)
	}

	// num_ctx and keep_alive as extra /v1 fields.
	s.p("\n| extra /v1 field | effect |\n|---|---|")
	r := stream(req{Model: *model, Msgs: []openai.ChatCompletionMessageParamUnion{user("Hi")}, MaxTokens: 1, Think: thinkOff, Extra: map[string]any{"options": map[string]any{"num_ctx": 8192}, "num_ctx": 8192}})
	ctx := 0
	if m := loaded(*model); m != nil {
		ctx = m.ContextLength
	}
	ctxErr := r.Err
	r = stream(req{Model: *model, Msgs: []openai.ChatCompletionMessageParamUnion{user("Count from 1 to 50, separated by commas.")}, MaxTokens: 20, MaxCompletion: true, Think: thinkOff})
	s.p("| `max_completion_tokens: 20` (what openai-go's MaxCompletionTokens sends) | %d completion tokens, finish %s |", r.OutTok, r.Finish)
	r = stream(req{Model: *model, Msgs: []openai.ChatCompletionMessageParamUnion{user("Count from 1 to 50, separated by commas.")}, MaxTokens: 20, Think: thinkOff})
	s.p("| `max_tokens: 20` (openai-go's deprecated MaxTokens) | %d completion tokens, finish %s |", r.OutTok, r.Finish)
	s.p("| `options: {num_ctx: 8192}` and `num_ctx: 8192` | loaded context after the request: %d (err %v) |", ctx, ctxErr)
	r = stream(req{Model: *model, Msgs: []openai.ChatCompletionMessageParamUnion{user("Hi")}, MaxTokens: 1, Think: thinkOff, Extra: map[string]any{"keep_alive": "30m"}})
	if m := loaded(*model); m != nil {
		s.p("| `keep_alive: \"30m\"` | model expires in %.0f min after the request (server default is 5 min) (err %v) |", time.Until(m.ExpiresAt).Minutes(), r.Err)
	}
}

func unloadNote(gone bool, d time.Duration) string {
	if gone {
		return fmt.Sprintf("model unloaded %.0f s after turn 5; TTFT includes the reload", d.Seconds())
	}
	return fmt.Sprintf("model still loaded %.0f s after turn 5: /v1 ignored keep_alive", d.Seconds())
}

func yesNo(b bool, yes string) string {
	if b {
		return yes
	}
	return "no"
}

// ---------------- threads ----------------

func threadTest(s *section) {
	s.p("## 3b. CPU threads\n")
	s.p("Native /api/chat, base model, a ~700-token prompt, `think: false`, num_predict 16; each num_thread value reloads the model, so nothing is cached. Timings are Ollama's own (prompt_eval_duration, eval_duration).\n")
	s.p("| num_thread | NumThreads in log | load | prompt tokens | prompt tok/s | output tok/s |\n|---|---|---|---|---|---|")
	txt := burrowRequest(shape{Target: 700, Nonce: "threads"}, "What is the session code word? One word.")
	var msgs []map[string]string
	for _, m := range txt {
		if m.OfSystem != nil {
			msgs = append(msgs, map[string]string{"role": "system", "content": m.OfSystem.Content.OfString.Value})
		}
	}
	msgs = append(msgs, map[string]string{"role": "user", "content": "What is the session code word? One word."})
	for _, n := range []int{0, 4, 8, 10, 12} {
		opts := map[string]any{"num_predict": 16}
		label := "not set (Ollama default)"
		if n > 0 {
			opts["num_thread"] = n
			label = fmt.Sprint(n)
		}
		unload(*model)
		slog.mark()
		progress("threads %s", label)
		r, err := nativeChat(map[string]any{"model": *model, "messages": msgs, "think": false, "options": opts})
		li := slog.info()
		if err != nil {
			s.p("| %s | | error %s | | | |", label, oneLine(err.Error()))
			continue
		}
		th := "–"
		if li.Threads > 0 {
			th = fmt.Sprint(li.Threads)
		}
		s.p("| %s | %s | %.1f s | %d | %.1f | %.1f |", label, th, float64(r.LoadDuration)/1e9, r.PromptEvalCount,
			float64(r.PromptEvalCount)/(float64(r.PromptEvalDuration)/1e9), float64(r.EvalCount)/(float64(r.EvalDuration)/1e9))
	}
	unload(*model)
}

// ---------------- 3. truncation ----------------

func truncation(s *section) {
	s.p("## 3. Default context and truncation\n")
	s.p("Both requests go to the base model over /v1 with the default context (4,096 unless the server sets another), thinking off.\n")
	s.p("| prompt | built for (estimate) | prompt_tokens reported | TTFT | answer | code word | ticket | server log |\n|---|---|---|---|---|---|---|---|")
	warm(*model)
	cases := []struct {
		name string
		msgs []openai.ChatCompletionMessageParamUnion
		est  int
	}{
		{"Burrow-shaped, ~8k: code word at the start of the system prompt, ticket in the first history message", burrowRequest(shape{Target: 8000, Nonce: "trunc"}, checkQuestion), 8000},
	}
	if !*quick {
		cases = append(cases, struct {
			name string
			msgs []openai.ChatCompletionMessageParamUnion
			est  int
		}{"one ~12k system message, code word only at its start", bigSystem(12000), 12000})
	}
	for _, c := range cases {
		slog.mark()
		progress("truncation: %s", c.name)
		r := stream(req{Model: *model, Msgs: c.msgs, MaxTokens: 60, Think: thinkOff, Timeout: *maxReq})
		li := slog.info()
		code, ticket := checkAnswer(r.Content)
		lg := "(no -serverlog)"
		if li.Have {
			lg = "no truncation line"
			if len(li.Trunc) > 0 {
				var parts []string
				for _, t := range li.Trunc {
					parts = append(parts, "`"+trunc(t, 110)+"`")
				}
				lg = strings.Join(parts, "; ")
			}
		}
		if r.Err != nil {
			s.p("| %s | ~%d | – | – | error: %s | | | %s |", c.name, c.est, oneLine(r.Err.Error()), lg)
			continue
		}
		s.p("| %s | ~%d | %d | %s | %q | %s | %s | %s |", c.name, c.est, r.PromptTok, sec(r.TTFT), trunc(r.Content, 60), yesNo(code, "yes"), yesNo(ticket, "yes"), lg)
	}
	s.p("\nThe response has `finish_reason: stop` and no error or warning field; only prompt_tokens being lower than the prompt shows the cut. The same ~8k prompt on %s (num_ctx 12288) is the 8k row of section 4.", derived(12288))
}

// ---------------- 5. prefix reuse ----------------

func prefixReuse(s *section, mdl, other string) {
	s.p("## 5. Prefix reuse between turns\n")
	s.p("A ~1.3k-token Burrow-shaped chat on %s, thinking off, max 60 tokens. Turn 2 = turn 1 + the assistant answer + a short user message, as SPEC 3.1 intends. \"cache slot\" is Ollama's debug log line `loading cache slot … used=N` (tokens taken from the cache).\n", mdl)
	s.p("| step | prompt tokens | TTFT | cache slot (server log) | note |\n|---|---|---|---|---|")
	warm(mdl)
	row := func(step string, r *res, li logInfo, note string) {
		if r.Err != nil {
			note += " error: " + oneLine(r.Err.Error())
		}
		s.p("| %s | %d | %s | %s | %s |", step, r.PromptTok, sec(r.TTFT), li.slot(), note)
	}
	run := func(step string, m string, msgs []openai.ChatCompletionMessageParamUnion, note string, extra map[string]any) *res {
		slog.mark()
		progress("reuse: %s", step)
		r := stream(req{Model: m, Msgs: msgs, MaxTokens: 60, Think: thinkOff, Extra: extra})
		row(step, r, slog.info(), note)
		return r
	}
	a := burrowRequest(shape{Target: 1300, Nonce: "chatA"}, "Summarize the last tool result in one sentence.")
	r1 := run("A turn 1 (cold)", mdl, a, "", nil)
	a = append(a, asst(r1.Content), user("Now only the highest value_usd, one number."))
	r2 := run("A turn 2 (turn 1 + answer + short message)", mdl, a, "", nil)
	run("A turn 2 again (identical request)", mdl, a, "", nil)
	changed := append([]openai.ChatCompletionMessageParamUnion{}, a...)
	sysText := changed[0].OfSystem.Content.OfString.Value
	changed[0] = sys(strings.Replace(sysText, "Session code word", "[memory refreshed] Session code word", 1))
	run("A turn 2 with a changed first line of the system prompt", mdl, changed, "", nil)
	a = append(a, asst(r2.Content), user("And the lowest value_usd?"))
	b := burrowRequest(shape{Target: 1300, Nonce: "chatB"}, "Which coin appears most often in the last tool result?")
	rb := run("B turn 1 (another chat, in between)", mdl, b, "", nil)
	r3 := run("A turn 3 (after B)", mdl, a, "", nil)

	// Two requests at the same time.
	a = append(a, asst(r3.Content), user("Thanks. Which source had it?"))
	b = append(b, asst(rb.Content), user("And the second most common?"))
	slog.mark()
	progress("reuse: parallel")
	var wg sync.WaitGroup
	var pa, pb *res
	wg.Add(2)
	go func() { defer wg.Done(); pa = stream(req{Model: mdl, Msgs: a, MaxTokens: 60, Think: thinkOff}) }()
	go func() { time.Sleep(200 * time.Millisecond); defer wg.Done(); pb = stream(req{Model: mdl, Msgs: b, MaxTokens: 60, Think: thinkOff}) }()
	wg.Wait()
	li := slog.info()
	row("A turn 4 and B turn 2 sent together: A", pa, li, fmt.Sprintf("total %s", sec(pa.Total)))
	row("… B (sent 0.2 s later)", pb, li, fmt.Sprintf("total %s; OLLAMA_NUM_PARALLEL=1, so B waits for A", sec(pb.Total)))

	// keep_alive expiry.
	a = append(a, asst(pa.Content), user("OK."))
	r5 := run("A turn 5 with `keep_alive: \"5s\"`", mdl, a, "", map[string]any{"keep_alive": "5s"})
	t0 := time.Now()
	for loaded(mdl) != nil && time.Since(t0) < 90*time.Second {
		time.Sleep(time.Second)
	}
	gone := loaded(mdl) == nil
	a = append(a, asst(r5.Content), user("One more: what was the first day in the table?"))
	run("A turn 6 after the model unloaded", mdl, a, unloadNote(gone, time.Since(t0)), nil)

	// Different num_ctx = different runner.
	a = append(a, asst(""), user("Thanks."))
	only(other)
	run(fmt.Sprintf("A turn 7 on %s (other num_ctx)", other), other, a, "different num_ctx → model reload, TTFT includes it", nil)
	only(mdl)
}

// ---------------- 6. tool calling ----------------

func toolCalling(s *section, mdl string) {
	s.p("## 6. Tool calling\n")
	warm(mdl)
	// Size of the tool list: same request with and without tools.
	base := []openai.ChatCompletionMessageParamUnion{sys(toolSystem), user(tasks[0].Prompt)}
	progress("tools: size without tools")
	r0 := stream(req{Model: mdl, Msgs: base, MaxTokens: 1, Think: thinkOff})
	s.p("All %d tools of SPEC 8.1 with JSON schemas, on %s over /v1 (streaming). Arguments are checked with santhosh-tekuri/jsonschema/v6 against the tool's schema, then against what the task needs (table name, values, enum choice, revision). Single-tool tasks check the first response only; multi-step tasks run the loop with simulated tool results up to the final answer.\n", len(tools), mdl)
	s.p("System prompt + one user message: %d prompt tokens without tools (TTFT %s).", r0.PromptTok, sec(r0.TTFT))

	type mode struct {
		name   string
		think  thinkMode
		maxTok int
		ids    []string
		budget time.Duration
	}
	modes := []mode{{"thinking off (`reasoning_effort: none`)", thinkOff, 400, nil, 0}}
	if *quick {
		modes[0].ids = []string{"T01", "T06", "T10", "T12", "T14"}
	} else if *taskIDs != "" {
		modes[0].ids = strings.Split(*taskIDs, ",")
	} else {
		modes = append(modes, mode{"thinking on (model default)", thinkDefault, 800, []string{"T01", "T10", "T12", "T14", "T05", "T03", "T13", "T11", "T02", "T04", "T06", "T07", "T08", "T09", "T15"}, 15 * time.Minute})
	}
	var summary []string
	for mi, m := range modes {
		s.p("\n### %s, max_tokens %d\n", m.name, m.maxTok)
		s.p("| task | kind | prompt | pass | right tool | args valid (schema) | args right | requests | time | first TTFT | prompt tokens | completion tokens | notes |\n|---|---|---|---|---|---|---|---|---|---|---|---|---|")
		t0 := time.Now()
		var n, pass, right, valid, argsOK int
		for _, t := range ordered(m.ids) {
			if m.budget > 0 && time.Since(t0) > m.budget {
				s.p("| %s | %s | %s | not run (time budget) | | | | | | | | | |", t.ID, t.Kind, trunc(t.Prompt, 50))
				continue
			}
			progress("tools %d: %s", mi, t.ID)
			tr := runTask(mdl, t, m.think, m.maxTok)
			n++
			if tr.Pass {
				pass++
			}
			if tr.RightTool {
				right++
			}
			if tr.ArgsValid {
				valid++
			}
			if tr.ArgsRight {
				argsOK++
			}
			s.p("| %s | %s | %s | %s | %s | %s | %s | %d | %s | %s | %d | %d | %s |", t.ID, t.Kind, trunc(t.Prompt, 50), yn(tr.Pass), yn(tr.RightTool), yn(tr.ArgsValid), yn(tr.ArgsRight),
				tr.Requests, sec(tr.Time), sec(tr.FirstTTFT), tr.PromptTok, tr.ReasonTok, strings.ReplaceAll(tr.Note, "|", "/"))
		}
		s.p("\n**%s:** pass %d/%d, right tool (or rightly no tool) %d/%d, schema-valid args %d/%d, right args %d/%d, %.1f min.", m.name, pass, n, right, n, valid, n, argsOK, n, time.Since(t0).Minutes())
		summary = append(summary, fmt.Sprintf("| %s | %d/%d | %d/%d | %d/%d | %d/%d | %.1f min |", m.name, pass, n, right, n, valid, n, argsOK, n, time.Since(t0).Minutes()))
	}
	s.p("\n| mode | pass | right tool | schema-valid args | right args | time |\n|---|---|---|---|---|---|")
	for _, l := range summary {
		s.p("%s", l)
	}
	s.p("\n\"args valid\" and \"args right\" count as no when no call was made for a task that needs one.")
}

// ordered returns the tasks in the order of ids (all tasks if ids is nil).
func ordered(ids []string) []task {
	if ids == nil {
		return tasks
	}
	var out []task
	for _, id := range ids {
		for _, t := range tasks {
			if t.ID == id {
				out = append(out, t)
			}
		}
	}
	return out
}

func yn(b bool) string { return yesNo(b, "yes") }

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// ---------------- memory ----------------

func memTable() {
	fmt.Print("## 7. Memory\n\n")
	fmt.Print("Runner working set = the largest ollama.exe process (tasklist). /api/ps size is Ollama's own estimate. Ollama log values are from the runner's load lines.\n\n")
	fmt.Println("| when | num_ctx | /api/ps size | runner working set | system available | KV cache (log) | total (log) |\n|---|---|---|---|---|---|---|")
	for _, m := range mems {
		kv, tot := m.KvCache, m.Total
		if kv == "" {
			kv = "–"
		}
		if tot == "" {
			tot = "–"
		}
		fmt.Printf("| %s | %d | %s | %s | %s | %s | %s |\n", m.Label, m.Ctx, gib(m.PsSize), gib(m.Mem.RunnerWS), gibu(m.Mem.AvailPhys), kv, tot)
	}
}
