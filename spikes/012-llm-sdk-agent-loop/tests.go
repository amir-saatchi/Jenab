package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"time"
)

type Cell struct {
	Status string // pass, partial, fail, n/a
	Note   string
}

type check struct {
	fails, partial []string
}

func (c *check) must(ok bool, format string, a ...any) {
	if !ok {
		c.fails = append(c.fails, fmt.Sprintf(format, a...))
	}
}
func (c *check) should(ok bool, format string, a ...any) {
	if !ok {
		c.partial = append(c.partial, fmt.Sprintf(format, a...))
	}
}
func (c *check) cell(okNote string) Cell {
	if len(c.fails) > 0 {
		return Cell{"fail", strings.Join(append(c.fails, c.partial...), "; ")}
	}
	if len(c.partial) > 0 {
		return Cell{"partial", strings.Join(c.partial, "; ")}
	}
	return Cell{"pass", okNote}
}

// ---- shared fixtures ----

var pngBytes = func() []byte {
	b, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
	return b
}()

const thinkSig = "EqQBCgIYAhIM1gbcDa9GJwZA2b3hGgxBdjrkzLoky3dl1pkiMOYds+/Zq9w1Mg=="

var weatherTool = ToolDef{Name: "get_weather", Description: "Current weather for a city", Schema: map[string]any{
	"type": "object", "properties": map[string]any{"city": map[string]any{"type": "string"}, "unit": map[string]any{"type": "string"}}, "required": []string{"city"}}}
var timeTool = ToolDef{Name: "get_time", Description: "Current time in a time zone", Schema: map[string]any{
	"type": "object", "properties": map[string]any{"tz": map[string]any{"type": "string"}}, "required": []string{"tz"}}}
var shotTool = ToolDef{Name: "screenshot", Description: "Screenshot of the page", Schema: map[string]any{
	"type": "object", "properties": map[string]any{}}}

var toolFuncs = map[string]ToolFunc{
	"get_weather": func(json.RawMessage) []Part { return []Part{{Type: PText, Text: "18 C, sunny"}} },
	"get_time":    func(json.RawMessage) []Part { return []Part{{Type: PText, Text: "14:05"}} },
	"screenshot": func(json.RawMessage) []Part {
		return []Part{{Type: PText, Text: "screenshot of the page"}, {Type: PImage, MIME: "image/png", Data: pngBytes}}
	},
}

func baseRequest(k Kind, tools ...ToolDef) *Request {
	model := mockModel(k)
	return &Request{Model: model, MaxTokens: 1024, Tools: tools,
		System:   []Part{{Type: PText, Text: "You are Burrow."}},
		Messages: []Message{{Role: "user", Parts: []Part{{Type: PText, Text: "Weather and time in Berlin?"}}}}}
}

func contextBlocks() []Part {
	names := []string{"System prompt", "User memory", "Project memory", "Project card", "Session notes"}
	var out []Part
	for i, n := range names {
		out = append(out, Part{Type: PText, Text: fmt.Sprintf("[%d] %s: %s", i+1, n, strings.Repeat("lorem ipsum ", 20))})
	}
	return out
}

func newProv(opt Option, k Kind, m *Mock, retries int) (Provider, error) {
	return opt.New(Options{Kind: k, BaseURL: m.URL(), APIKey: "test-key-not-real", Model: mockModel(k), MaxRetries: retries})
}

func countEv(evs []Event, k EventKind) int {
	n := 0
	for _, e := range evs {
		if e.Kind == k {
			n++
		}
	}
	return n
}

func jsonEq(a, b string) bool {
	var x, y any
	if json.Unmarshal([]byte(a), &x) != nil || json.Unmarshal([]byte(b), &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

func finalText(r *Response) string {
	if r == nil {
		return ""
	}
	var s []string
	for _, p := range r.Message.Parts {
		if p.Type == PText {
			s = append(s, p.Text)
		}
	}
	return strings.Join(s, "")
}

// ---- T1: streamed text, two parallel tool calls with partial JSON, both results sent back ----

func t1Script(m *Mock, n int, w http.ResponseWriter, r *http.Request, body map[string]any) {
	if isAnth(r) {
		a := anthStart(w, Usage{Input: 120})
		if n == 0 {
			a.text("Checking ", "both ", "now.")
			a.tool("toolu_A", "get_weather", `{"city":`, ` "Ber`, `lin", "unit"`, `: "C"}`)
			a.tool("toolu_B", "get_time", `{"tz"`, `: "Europe/Berlin"}`)
			a.end("tool_use", 61)
		} else {
			a.text("Berlin: 18 C ", "at 14:05.")
			a.end("end_turn", 12)
		}
		return
	}
	o := oaiStart(w, body)
	if n == 0 {
		o.text("Checking ", "both ", "now.")
		o.tool(0, "call_A", "get_weather", `{"city":`, ` "Ber`, `lin", "unit"`, `: "C"}`)
		o.tool(1, "call_B", "get_time", `{"tz"`, `: "Europe/Berlin"}`)
		o.end("tool_calls", Usage{Input: 120, Output: 61})
	} else {
		o.text("Berlin: 18 C ", "at 14:05.")
		o.end("stop", Usage{Input: 200, Output: 12})
	}
}

func testT1(opt Option, k Kind) Cell {
	m := newMock(t1Script)
	defer m.Close()
	p, err := newProv(opt, k, m, 0)
	if err != nil {
		return errCell(err)
	}
	before := libraryRanTool.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := runLoop(ctx, p, baseRequest(k, weatherTool, timeTool), toolFuncs, false, 4)
	if err != nil {
		return Cell{"fail", "loop error: " + firstLine(err.Error())}
	}
	c := &check{}
	c.must(libraryRanTool.Load() == before, "library ran a tool itself")
	c.must(messageBeforeChange(res.DB), "tool ran before assistant message was written")
	c.must(res.Requests == 2, "%d requests, want 2", res.Requests)
	c.must(finalText(res.Final) == "Berlin: 18 C at 14:05.", "final text %q", finalText(res.Final))
	c.should(countEv(res.Events, EvText) >= 3, "text not streamed as deltas (%d events)", countEv(res.Events, EvText))
	c.should(countEv(res.Events, EvToolStart) == 2, "tool-call start events: %d", countEv(res.Events, EvToolStart))
	c.should(countEv(res.Events, EvToolDelta) >= 6, "argument deltas: %d", countEv(res.Events, EvToolDelta))
	c.should(countEv(res.Events, EvUsage) >= 1, "no usage event")
	b := m.Body(1)
	msgs := arr(b["messages"])
	if k == Anthropic {
		var uses, results []string
		userMsgs := 0
		for _, mm := range msgs {
			x := mp(mm)
			has := false
			for _, blk := range arr(x["content"]) {
				switch str(mp(blk)["type"]) {
				case "tool_use":
					in, _ := json.Marshal(mp(blk)["input"])
					uses = append(uses, str(mp(blk)["id"])+string(in))
				case "tool_result":
					results = append(results, str(mp(blk)["tool_use_id"]))
					has = true
				}
			}
			if has {
				userMsgs++
			}
		}
		c.must(len(uses) == 2 && uses[0] == `toolu_A{"city":"Berlin","unit":"C"}` && uses[1] == `toolu_B{"tz":"Europe/Berlin"}`, "tool_use echoed back: %v", uses)
		c.must(len(results) == 2 && results[0] == "toolu_A" && results[1] == "toolu_B", "tool_result ids: %v", results)
		c.should(userMsgs == 1, "the 2 tool_results are split over %d user messages (the API merges them; docs ask for one)", userMsgs)
	} else {
		var calls, results []string
		for _, mm := range msgs {
			x := mp(mm)
			for _, tc := range arr(x["tool_calls"]) {
				f := mp(mp(tc)["function"])
				calls = append(calls, str(mp(tc)["id"])+"="+str(f["name"]))
				if str(mp(tc)["id"]) == "call_A" {
					c.must(jsonEq(str(f["arguments"]), `{"city":"Berlin","unit":"C"}`), "call_A arguments %q", str(f["arguments"]))
				}
			}
			if str(x["role"]) == "tool" {
				results = append(results, str(x["tool_call_id"]))
			}
		}
		c.must(len(calls) == 2 && calls[0] == "call_A=get_weather" && calls[1] == "call_B=get_time", "assistant tool_calls echoed: %v", calls)
		c.must(len(results) == 2 && results[0] == "call_A" && results[1] == "call_B", "tool messages: %v", results)
	}
	return c.cell(fmt.Sprintf("%d text, %d tool-start, %d arg-delta events", countEv(res.Events, EvText), countEv(res.Events, EvToolStart), countEv(res.Events, EvToolDelta)))
}

// ---- T2: tool result with an image ----

func t2Script(m *Mock, n int, w http.ResponseWriter, r *http.Request, body map[string]any) {
	if isAnth(r) {
		a := anthStart(w, Usage{Input: 50})
		if n == 0 {
			a.tool("toolu_S", "screenshot", `{}`)
			a.end("tool_use", 10)
		} else {
			a.text("I see a white pixel.")
			a.end("end_turn", 6)
		}
		return
	}
	if n == 1 {
		// The real API accepts only text parts in tool messages.
		for _, mm := range arr(body["messages"]) {
			if str(mp(mm)["role"]) == "tool" {
				for _, pt := range arr(mp(mm)["content"]) {
					if str(mp(pt)["type"]) != "text" {
						oaiError(w, 400, "invalid_request_error", "invalid_value", "Invalid 'messages[].content': tool messages only support text content parts.", nil)
						return
					}
				}
			}
		}
	}
	o := oaiStart(w, body)
	if n == 0 {
		o.tool(0, "call_S", "screenshot", `{}`)
		o.end("tool_calls", Usage{Input: 50, Output: 10})
	} else {
		o.text("I see a white pixel.")
		o.end("stop", Usage{Input: 80, Output: 6})
	}
}

func testT2(opt Option, k Kind) Cell {
	m := newMock(t2Script)
	defer m.Close()
	p, err := newProv(opt, k, m, 0)
	if err != nil {
		return errCell(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, lerr := runLoop(ctx, p, baseRequest(k, shotTool), toolFuncs, false, 3)
	b64 := base64.StdEncoding.EncodeToString(pngBytes)
	raw := m.Raw(1)
	if raw == "" {
		return Cell{"fail", "no second request: " + errText(lerr)}
	}
	if !strings.Contains(raw, b64) {
		return Cell{"fail", "image dropped from the tool result"}
	}
	msgs := arr(m.Body(1)["messages"])
	if k == Anthropic {
		last := mp(msgs[len(msgs)-1])
		for _, blk := range arr(last["content"]) {
			if str(mp(blk)["type"]) != "tool_result" {
				continue
			}
			for _, cc := range arr(mp(blk)["content"]) {
				src := mp(mp(cc)["source"])
				if str(mp(cc)["type"]) == "image" && str(src["type"]) == "base64" && str(src["media_type"]) == "image/png" && str(src["data"]) == b64 {
					if lerr != nil {
						return Cell{"partial", "image block sent, but loop error: " + errText(lerr)}
					}
					return Cell{"pass", "image block inside tool_result"}
				}
			}
		}
		return Cell{"fail", "image sent, but not as an image block inside tool_result"}
	}
	for _, mm := range msgs {
		if str(mp(mm)["role"]) == "tool" && strings.Contains(js(mm), b64) {
			return Cell{"fail", "image put inside the tool message; the API rejects it (400)"}
		}
	}
	if lerr != nil {
		return Cell{"fail", errText(lerr)}
	}
	return Cell{"pass", "tool text + image in a user message after it (our rule)"}
}

// ---- T3: cache points and cache usage ----

func t3Script(m *Mock, n int, w http.ResponseWriter, r *http.Request, body map[string]any) {
	if isAnth(r) {
		if n == 0 {
			a := anthStart(w, Usage{Input: 20, CacheWrite: 1500})
			a.tool("toolu_C", "get_weather", `{"city":"Berlin"}`)
			a.end("tool_use", 15)
		} else {
			a := anthStart(w, Usage{Input: 30, CacheWrite: 200, CacheRead: 1500})
			a.text("18 C.")
			a.end("end_turn", 4)
		}
		return
	}
	o := oaiStart(w, body)
	o.text("18 C.")
	o.end("stop", Usage{Input: 3100, Output: 4, CacheRead: 3000})
}

// cachePaths lists every place in the JSON body that has a cache_control key.
func cachePaths(v any, path string, out *[]string) {
	switch x := v.(type) {
	case map[string]any:
		if _, ok := x["cache_control"]; ok {
			*out = append(*out, path)
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if k != "cache_control" {
				cachePaths(x[k], path+"."+k, out)
			}
		}
	case []any:
		for i, e := range x {
			cachePaths(e, fmt.Sprintf("%s[%d]", path, i), out)
		}
	}
}

func testT3(opt Option, k Kind) Cell {
	m := newMock(t3Script)
	defer m.Close()
	p, err := newProv(opt, k, m, 0)
	if err != nil {
		return errCell(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req := baseRequest(k, weatherTool)
	req.System = contextBlocks()
	req.Messages = []Message{
		{Role: "user", Parts: []Part{{Type: PText, Text: "hi"}}},
		{Role: "assistant", Parts: []Part{{Type: PText, Text: "hello"}}},
		{Role: "user", Parts: []Part{{Type: PText, Text: "Weather in Berlin?"}}},
	}
	res, lerr := runLoop(ctx, p, req, toolFuncs, true, 3)
	c := &check{}
	if k == OpenAI {
		if lerr != nil {
			return Cell{"fail", errText(lerr)}
		}
		c.must(len(res.Usage) == 1 && res.Usage[0].CacheRead == 3000, "cached_tokens not surfaced (%+v)", res.Usage)
		return c.cell("no cache points in this API; cached_tokens=3000 surfaced")
	}
	joined := 0
	block5 := contextBlocks()[4].Text
	for i := 0; i < m.Count(); i++ {
		b := m.Body(i)
		sys := arr(b["system"])
		// The cache point must sit right after block 5. Joining the blocks into fewer system
		// blocks is fine as long as the point ends where block 5 ends.
		sysIdx := -1
		for j, s := range sys {
			if strings.HasSuffix(str(mp(s)["text"]), block5) {
				sysIdx = j
			}
		}
		if len(sys) != 5 {
			joined = len(sys)
		}
		c.must(sysIdx >= 0, "req %d: block 5 not found as the end of a system block", i+1)
		msgs := arr(b["messages"])
		want := []string{fmt.Sprintf(".system[%d]", sysIdx)}
		if len(msgs) > 0 {
			lm := mp(msgs[len(msgs)-1])
			want = append(want, fmt.Sprintf(".messages[%d].content[%d]", len(msgs)-1, len(arr(lm["content"]))-1))
		}
		var got []string
		cachePaths(b, "", &got)
		sort.Strings(got)
		sort.Strings(want)
		c.must(reflect.DeepEqual(got, want), "req %d: cache_control at %v, want %v", i+1, got, want)
	}
	c.must(m.Count() == 2, "%d requests: %s", m.Count(), errText(lerr))
	if lerr == nil && len(res.Usage) == 2 {
		c.must(res.Usage[0].CacheWrite == 1500, "cache write not surfaced (got %d)", res.Usage[0].CacheWrite)
		c.must(res.Usage[1].CacheRead == 1500, "cache read not surfaced (got %d)", res.Usage[1].CacheRead)
		c.must(res.Usage[1].CacheWrite == 200, "cache write on turn 2 (got %d)", res.Usage[1].CacheWrite)
	}
	note := "2 points: end of block 5 and last block, both requests; write/read tokens surfaced"
	if joined > 0 {
		note += fmt.Sprintf(" (5 blocks joined into %d system block)", joined)
	}
	return c.cell(note)
}

// ---- T4: extended thinking blocks with signature passed back unchanged ----

func t4Script(m *Mock, n int, w http.ResponseWriter, r *http.Request, body map[string]any) {
	a := anthStart(w, Usage{Input: 40})
	if n == 0 {
		a.thinking(thinkSig, "Let me think", " about Berlin.")
		a.tool("toolu_T", "get_weather", `{"city":"Berlin"}`)
		a.end("tool_use", 30)
	} else {
		a.text("18 C.")
		a.end("end_turn", 4)
	}
}

func testT4(opt Option, k Kind) Cell {
	if k != Anthropic {
		return Cell{"n/a", ""}
	}
	m := newMock(t4Script)
	defer m.Close()
	p, err := newProv(opt, k, m, 0)
	if err != nil {
		return errCell(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req := baseRequest(k, weatherTool)
	req.ThinkingBudget = 2048
	req.MaxTokens = 4096
	res, lerr := runLoop(ctx, p, req, toolFuncs, false, 3)
	c := &check{}
	th := mp(m.Body(0)["thinking"])
	c.must(str(th["type"]) == "enabled" && fmt.Sprint(th["budget_tokens"]) == "2048", "thinking config sent: %v", th)
	if m.Count() < 2 {
		c.must(false, "no second request: %s", errText(lerr))
		return c.cell("")
	}
	msgs := arr(m.Body(1)["messages"])
	var asst map[string]any
	for _, mm := range msgs {
		if str(mp(mm)["role"]) == "assistant" {
			asst = mp(mm)
		}
	}
	blocks := arr(asst["content"])
	want := map[string]any{"type": "thinking", "thinking": "Let me think about Berlin.", "signature": thinkSig}
	c.must(len(blocks) >= 2 && reflect.DeepEqual(mp(blocks[0]), want), "assistant block 0 = %s", js(firstOr(blocks)))
	c.must(len(blocks) >= 2 && str(mp(blocks[len(blocks)-1])["type"]) == "tool_use", "tool_use not after thinking")
	if res != nil {
		c.should(countEv(res.Events, EvThinking) >= 2, "thinking not streamed (%d events)", countEv(res.Events, EvThinking))
	}
	c.must(lerr == nil, "loop: %s", errText(lerr))
	return c.cell("thinking + signature byte-identical, before tool_use")
}

func firstOr(a []any) any {
	if len(a) == 0 {
		return nil
	}
	return a[0]
}

// ---- T5: 429 / 529 / 500 ----

type errCase struct {
	status int
	typ    string
	hdr    map[string]string
}

var anthErrCases = []errCase{
	{429, "rate_limit_error", map[string]string{"retry-after": "1"}},
	{529, "overloaded_error", nil},
	{500, "api_error", nil},
}
var oaiErrCases = []errCase{
	{429, "rate_limit_exceeded", map[string]string{"retry-after": "1"}},
	{500, "server_error", nil},
}

type ErrRow struct {
	Option, Kind, Case    string
	DefAttempts, Attempts0 int
	DefTime               time.Duration
	Visible, Err          string
}

func testT5(opt Option, k Kind) ([]ErrRow, Cell) {
	cases := anthErrCases
	if k == OpenAI {
		cases = oaiErrCases
	}
	var rows []ErrRow
	allVisible, controllable := true, true
	for _, ec := range cases {
		row := ErrRow{Option: opt.Name, Kind: string(k), Case: fmt.Sprint(ec.status)}
		for _, retries := range []int{-1, 0} {
			ec := ec
			m := newMock(func(m *Mock, n int, w http.ResponseWriter, r *http.Request, body map[string]any) {
				if isAnth(r) {
					anthError(w, ec.status, ec.typ, "mock "+ec.typ, ec.hdr)
				} else {
					oaiError(w, ec.status, ec.typ, ec.typ, "mock "+ec.typ, ec.hdr)
				}
			})
			p, err := newProv(opt, k, m, retries)
			if err != nil {
				m.Close()
				return nil, errCell(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			t0 := time.Now()
			_, err = p.Stream(ctx, baseRequest(k, weatherTool), func(Event) {})
			el := time.Since(t0)
			cancel()
			m.Close()
			if retries < 0 {
				row.DefAttempts, row.DefTime = m.Count(), el
				row.Err = firstLine(errText(err))
				pe := opt.Classify(err)
				if pe != nil && pe.Status == ec.status {
					row.Visible = fmt.Sprintf("status %d, type %q", pe.Status, pe.Type)
					if ec.hdr != nil {
						if pe.HasRetry {
							row.Visible += fmt.Sprintf(", retry-after %s", pe.RetryAfter)
						} else {
							row.Visible += ", retry-after not reachable"
							allVisible = false
						}
					}
				} else {
					row.Visible = "status not reachable"
					allVisible = false
				}
			} else {
				row.Attempts0 = m.Count()
				if row.Attempts0 != 1 {
					controllable = false
				}
			}
		}
		rows = append(rows, row)
	}
	var parts []string
	for _, r := range rows {
		parts = append(parts, fmt.Sprintf("%s: %d tries/%.1fs", r.Case, r.DefAttempts, r.DefTime.Seconds()))
	}
	st, note := "pass", strings.Join(parts, ", ")
	if !controllable {
		st, note = "partial", note+"; retries cannot be set to 0"
	}
	if !allVisible {
		st, note = "partial", note+"; status/retry-after not all reachable"
	}
	return rows, Cell{st, note}
}

// ---- T6: cancel mid-stream (10 runs) ----

const t6Runs = 10

type cancelRun struct {
	ret, closedAfter time.Duration
	closed           bool
	err              error
	fail             string
}

func cancelOnce(opt Option, k Kind) cancelRun {
	m := newMock(func(m *Mock, n int, w http.ResponseWriter, r *http.Request, body map[string]any) {
		if isAnth(r) {
			a := anthStart(w, Usage{Input: 10})
			a.s.ev("content_block_start", obj{"type": "content_block_start", "index": 0, "content_block": obj{"type": "text", "text": ""}})
			a.s.ev("content_block_delta", obj{"type": "content_block_delta", "index": 0, "delta": obj{"type": "text_delta", "text": "Hello"}})
		} else {
			o := oaiStart(w, body)
			o.text("Hello")
		}
		select {
		case <-r.Context().Done():
			m.closed <- clock()
		case <-time.After(10 * time.Second):
		}
	})
	defer m.Close()
	p, err := newProv(opt, k, m, 0)
	if err != nil {
		return cancelRun{fail: "setup: " + err.Error()}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var cancelAt stamp
	done := make(chan error, 1)
	go func() {
		_, err := p.Stream(ctx, baseRequest(k), func(e Event) {
			if e.Kind == EvText && cancelAt == 0 {
				cancelAt = clock()
				cancel()
			}
		})
		done <- err
	}()
	var r cancelRun
	select {
	case r.err = <-done:
	case <-time.After(8 * time.Second):
		return cancelRun{fail: "Stream did not return 8 s after cancel"}
	}
	if cancelAt == 0 {
		return cancelRun{fail: "no text event before end: " + errText(r.err)}
	}
	r.ret = since(cancelAt)
	select {
	case t := <-m.closed:
		r.closed, r.closedAfter = true, between(cancelAt, t)
	case <-time.After(2 * time.Second):
	}
	return r
}

func testT6(opt Option, k Kind) Cell {
	var maxRet, maxClose time.Duration
	nilErr, notCancel, notClosed := 0, 0, 0
	var sample error
	for i := 0; i < t6Runs; i++ {
		r := cancelOnce(opt, k)
		if r.fail != "" {
			return Cell{"fail", r.fail}
		}
		maxRet = max(maxRet, r.ret)
		if r.closed {
			maxClose = max(maxClose, r.closedAfter)
		} else {
			notClosed++
		}
		switch {
		case r.err == nil:
			nilErr++
		case !errors.Is(r.err, context.Canceled):
			notCancel++
		default:
			sample = r.err
		}
	}
	note := fmt.Sprintf("%d runs: returned within %s, server saw close within %s", t6Runs, fmtDur(maxRet), fmtDur(maxClose))
	if sample != nil {
		note += "; err " + shortErr(sample)
	}
	st := "pass"
	if notCancel > 0 {
		st, note = "partial", note+fmt.Sprintf("; %d runs not context.Canceled", notCancel)
	}
	if nilErr > 0 || notClosed > 0 {
		st, note = "fail", note+fmt.Sprintf("; nil error (partial message looks complete) in %d runs; connection left open in %d runs", nilErr, notClosed)
	}
	return Cell{st, note}
}

// ---- T7: truncated / malformed streams ----

var t7Variants = []string{"EOF without end", "connection reset", "malformed JSON", "error event"}

func t7Script(variant string) func(m *Mock, n int, w http.ResponseWriter, r *http.Request, body map[string]any) {
	return func(m *Mock, n int, w http.ResponseWriter, r *http.Request, body map[string]any) {
		if isAnth(r) {
			a := anthStart(w, Usage{Input: 10})
			a.s.ev("content_block_start", obj{"type": "content_block_start", "index": 0, "content_block": obj{"type": "text", "text": ""}})
			a.s.ev("content_block_delta", obj{"type": "content_block_delta", "index": 0, "delta": obj{"type": "text_delta", "text": "Hel"}})
			switch variant {
			case "EOF without end":
				return
			case "connection reset":
				fmt.Fprint(w, "event: content_block_delta\ndata: {\"type\":\"content_block_del")
				a.s.flush()
				panic(http.ErrAbortHandler)
			case "malformed JSON":
				a.s.ev("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"lo"`)
				a.s.ev("content_block_stop", obj{"type": "content_block_stop", "index": 0})
				a.end("end_turn", 3)
			case "error event":
				a.s.ev("error", obj{"type": "error", "error": obj{"type": "overloaded_error", "message": "Overloaded"}})
			}
			return
		}
		o := oaiStart(w, body)
		o.text("Hel")
		switch variant {
		case "EOF without end":
			return
		case "connection reset":
			fmt.Fprint(w, "data: {\"id\":\"chatcmpl-mock\",\"object\":\"chat.compl")
			o.s.flush()
			panic(http.ErrAbortHandler)
		case "malformed JSON":
			o.s.data(`{"id":"chatcmpl-mock","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"lo"}`)
			o.end("stop", Usage{Input: 10, Output: 3})
		case "error event":
			o.s.data(obj{"error": obj{"message": "The server is overloaded", "type": "server_error", "param": nil, "code": nil}})
		}
	}
}

type TruncRow struct {
	Option, Kind, Variant, Result string
}

func testT7(opt Option, k Kind) ([]TruncRow, Cell) {
	var rows []TruncRow
	okAll := true
	var bad []string
	run := func(v string) (string, bool) {
		m := newMock(t7Script(v))
		defer m.Close()
		p, err := newProv(opt, k, m, 0)
		if err != nil {
			return "setup: " + err.Error(), false
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		t0 := clock()
		resp, err := p.Stream(ctx, baseRequest(k), func(Event) {})
		el := since(t0)
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return "hang (5 s timeout)", false
		case err != nil:
			return fmt.Sprintf("error after %s: %s", fmtDur(el), shortErr(err)), true
		default:
			return fmt.Sprintf("no error, text %q", finalText(resp)), false
		}
	}
	for _, v := range t7Variants {
		res, ok := run(v)
		if !ok {
			okAll = false
			if strings.HasPrefix(res, "hang") {
				bad = append(bad, v+": hang")
			} else {
				bad = append(bad, v+": silent")
			}
		}
		rows = append(rows, TruncRow{opt.Name, string(k), v, res})
	}
	if opt.Name == optSDK.Name {
		sdkRawMode = true
		res, _ := run("EOF without end")
		sdkRawMode = false
		rows = append(rows, TruncRow{opt.Name + " without our end check", string(k), "EOF without end", res})
	}
	if okAll {
		return rows, Cell{"pass", "all 4 variants return an error"}
	}
	return rows, Cell{"fail", strings.Join(bad, ", ")}
}

// ---- helpers ----

func errCell(err error) Cell {
	if errors.Is(err, errUnsupported) {
		return Cell{"n/a", "not supported"}
	}
	return Cell{"fail", "setup: " + firstLine(err.Error())}
}

func errText(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}

func shortErr(err error) string {
	s := firstLine(errText(err))
	if len(s) > 90 {
		s = s[:90] + "..."
	}
	return "`" + strings.ReplaceAll(s, "|", "/") + "`"
}

func firstLine(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func fmtDur(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return fmt.Sprintf("%.0f µs", float64(d.Microseconds()))
	case d < time.Second:
		return fmt.Sprintf("%.1f ms", float64(d.Microseconds())/1000)
	default:
		return fmt.Sprintf("%.2f s", d.Seconds())
	}
}

func parseRetryAfter(v string) (time.Duration, bool) {
	if v == "" {
		return 0, false
	}
	var f float64
	if _, err := fmt.Sscanf(v, "%g", &f); err == nil {
		return time.Duration(f * float64(time.Second)), true
	}
	if t, err := http.ParseTime(v); err == nil {
		return time.Until(t), true
	}
	return 0, false
}

// mockModel returns a real model name: some frameworks look names up in a registry.
func mockModel(k Kind) string {
	if k == Anthropic {
		return "claude-sonnet-4-5"
	}
	return "gpt-4o"
}

// genkitDefault runs the T1 script once through genkit.Generate without WithReturnToolRequests(true)
// and reports how often genkit called the registered Go tool functions itself.
func genkitDefault() string {
	m := newMock(t1Script)
	defer m.Close()
	p, err := newProv(optGenkit, Anthropic, m, 0)
	if err != nil {
		return "setup: " + err.Error()
	}
	genkitDefaultLoop = true
	defer func() { genkitDefaultLoop = false }()
	before := libraryRanTool.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = p.Stream(ctx, baseRequest(Anthropic, weatherTool, timeTool), func(Event) {})
	return fmt.Sprintf("genkit.Generate without WithReturnToolRequests(true): genkit called the Go tool functions %d times itself and sent %d requests (err %s)",
		libraryRanTool.Load()-before, m.Count(), shortErr(err))
}
