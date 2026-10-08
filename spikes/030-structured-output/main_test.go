package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
	"github.com/amir-saatchi/jenab/internal/secret"
)

func tasksByID(t *testing.T) map[string]*Task {
	t.Helper()
	ts, err := loadTasks("data")
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]*Task{}
	for _, x := range ts {
		m[x.ID] = x
	}
	return m
}

func TestTasksLoad(t *testing.T) {
	m := tasksByID(t)
	want := map[string]int{"select-en": 30, "select-de": 30, "select-fa": 30, "long-en": 700, "decide-en": 20, "decide-de": 20, "decide-fa": 20}
	for id, n := range want {
		if m[id] == nil || m[id].Items != n {
			t.Errorf("%s: %+v", id, m[id])
		}
	}
	if len(m) != 13 {
		t.Errorf("%d tasks: %v", len(m), taskIDs(slices.Collect(func(yield func(*Task) bool) {
			for _, x := range m {
				yield(x)
			}
		})))
	}
	// The long task's five Ethereum lines are the only ones naming it.
	n := 0
	for _, l := range strings.Split(m["long-en"].Input, "\n") {
		if strings.Contains(l, "Ethereum") || strings.Contains(l, "ETH ") || strings.Contains(l, "Ether ") {
			n++
		}
	}
	if n != 5 {
		t.Errorf("long-en has %d Ethereum lines", n)
	}
}

// answer builds the right answer from a task's expectation, taking the
// first of each $any.
func answer(t *Task) any {
	switch t.Kind {
	case "select":
		var picks []any
		for _, e := range t.Expect.([]any) {
			picks = append(picks, map[string]any{"index": e, "reason": "about ETH"})
		}
		return map[string]any{"picks": picks}
	case "decide":
		var as []any
		for i, e := range t.Expect.([]any) {
			w := e.([]any)
			as = append(as, map[string]any{"index": i, "complaint": w[0], "topic": w[1], "urgency": w[2]})
		}
		return map[string]any{"answers": as}
	}
	return firstAlt(t.Expect)
}

func firstAlt(v any) any {
	switch x := v.(type) {
	case map[string]any:
		if alts, ok := x["$any"].([]any); ok {
			return firstAlt(alts[0])
		}
		out := map[string]any{}
		for k, e := range x {
			out[k] = firstAlt(e)
		}
		return out
	case []any:
		out := []any{}
		for _, e := range x {
			out = append(out, firstAlt(e))
		}
		return out
	}
	return v
}

func TestRightAnswersPass(t *testing.T) {
	for _, task := range tasksByID(t) {
		sch, err := compileSchema(task.ID, task.Schema)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(answer(task))
		v, _, err := parseText(string(b))
		if err != nil {
			t.Fatal(err)
		}
		c := check(task, sch, v)
		if !c.valid() {
			t.Errorf("%s: %v %v", task.ID, c.kinds, c.msgs)
			continue
		}
		if s := scoreAnswer(task, v); !s.Right || s.Total == 0 {
			t.Errorf("%s: %+v", task.ID, s)
		}
	}
}

func TestScoreWrongAnswers(t *testing.T) {
	m := tasksByID(t)
	ex := m["extract-en"]
	a := answer(ex).(map[string]any)
	a["capacity"] = 500                                                             // made up
	a["speakers"].([]any)[1].(map[string]any)["company"] = "independent consultant" // copied
	a["city"] = "  berlin "
	b, _ := json.Marshal(a)
	v, _, _ := parseText(string(b))
	s := scoreAnswer(ex, v)
	if s.Right || s.Wrong != 2 || s.MadeUp != 1 || s.Copied != 1 {
		t.Errorf("extract: %+v", s)
	}

	fa := m["extract-fa"]
	a = answer(fa).(map[string]any)
	a["organizer"] = "داده پردازان پارس" // a space instead of the ZWNJ
	a["city"] = "Tehran"
	b, _ = json.Marshal(a)
	v, _, _ = parseText(string(b))
	if s := scoreAnswer(fa, v); !s.Right {
		t.Errorf("extract-fa: %+v", s)
	}

	sel := m["select-en"]
	v, _, _ = parseText(`{"picks":[{"index":5,"reason":""},{"index":12,"reason":""},{"index":18,"reason":""},{"index":8,"reason":""},{"index":2,"reason":""}]}`)
	if s := scoreAnswer(sel, v); s.Right || s.Wrong != 0 || !strings.Contains(s.Detail, "order") {
		t.Errorf("select order: %+v", s)
	}

	dec := m["decide-en"]
	da := answer(dec).(map[string]any)
	da["answers"].([]any)[6].(map[string]any)["urgency"] = 4 // within one
	da["answers"].([]any)[3].(map[string]any)["topic"] = "other"
	b, _ = json.Marshal(da)
	v, _, _ = parseText(string(b))
	if s := scoreAnswer(dec, v); s.Right || s.Wrong != 1 {
		t.Errorf("decide: %+v", s)
	}
}

func TestCheckKinds(t *testing.T) {
	m := tasksByID(t)
	sel, dec, ex := m["select-en"], m["decide-en"], m["extract-en"]
	cases := []struct {
		task *Task
		json string
		want string
	}{
		{sel, `{"picks":[{"index":1,"reason":"a"},{"index":1,"reason":"a"},{"index":2,"reason":"a"},{"index":3,"reason":"a"},{"index":4,"reason":"a"}]}`, failRepeated},
		{sel, `{"picks":[{"index":1},{"index":2,"reason":"a"},{"index":3,"reason":"a"},{"index":4,"reason":"a"},{"index":5,"reason":"a"}]}`, failMissing},
		{sel, `{"picks":[{"index":"1","reason":"a"},{"index":2,"reason":"a"},{"index":3,"reason":"a"},{"index":4,"reason":"a"},{"index":5,"reason":"a"}]}`, failType},
		{sel, `{"picks":[{"index":30,"reason":"a"},{"index":2,"reason":"a"},{"index":3,"reason":"a"},{"index":4,"reason":"a"},{"index":5,"reason":"a"}]}`, failRange},
		{sel, `{"picks":[{"index":1,"reason":"a"}]}`, failCount},
		{sel, `{"picks":[{"index":1,"reason":"a","x":1},{"index":2,"reason":"a"},{"index":3,"reason":"a"},{"index":4,"reason":"a"},{"index":5,"reason":"a"}]}`, failExtra},
	}
	for i, c := range cases {
		sch, _ := compileSchema(c.task.ID, c.task.Schema)
		v, _, err := parseText(c.json)
		if err != nil {
			t.Fatal(err)
		}
		got := check(c.task, sch, v)
		t.Logf("%d: %v %q", i, got.kinds, got.msgs)
		if !slices.Contains(got.kinds, c.want) || len(got.msgs) == 0 {
			t.Errorf("%d: kinds %v, msgs %v, want %s", i, got.kinds, got.msgs, c.want)
		}
	}
	// decide: an answer list with one message twice and one missing.
	da := answer(dec).(map[string]any)
	da["answers"].([]any)[19].(map[string]any)["index"] = 0
	b, _ := json.Marshal(da)
	v, _, _ := parseText(string(b))
	sch, _ := compileSchema(dec.ID, dec.Schema)
	if got := check(dec, sch, v); !slices.Contains(got.kinds, failRepeated) || !slices.Contains(got.kinds, failGap) {
		t.Errorf("decide: %v", got.kinds)
	}
	// extract: a bad date and a value outside the enum.
	ea := answer(ex).(map[string]any)
	ea["date"], ea["format"] = "15 April 2027", "offline"
	b, _ = json.Marshal(ea)
	v, _, _ = parseText(string(b))
	sch, _ = compileSchema(ex.ID, ex.Schema)
	if got := check(ex, sch, v); !slices.Contains(got.kinds, failFormat) || !slices.Contains(got.kinds, failEnum) {
		t.Errorf("extract: %v %v", got.kinds, got.msgs)
	}
}

func TestParseText(t *testing.T) {
	for s, wrapped := range map[string]bool{
		`{"a":1}`:                       false,
		"  {\"a\":1}\n":                 false,
		"```json\n{\"a\":1}\n```":       true,
		"Here is the result: {\"a\":1}": true,
		"{\"a\":1}\nI hope this helps.": true,
		"<think>x</think>\n{\"a\":1}":   true,
	} {
		v, w, err := parseText(s)
		if err != nil || w != wrapped || v.(map[string]any)["a"] == nil {
			t.Errorf("%q: %v %v %v", s, v, w, err)
		}
	}
	for _, s := range []string{"", "no json here", `{"a":`, "I can't do that."} {
		if _, _, err := parseText(s); err == nil {
			t.Errorf("%q: no error", s)
		}
	}
}

func TestInject(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if r.ContentLength != int64(len(b)) {
			t.Errorf("content length %d, body %d", r.ContentLength, len(b))
		}
		json.Unmarshal(b, &got)
	}))
	defer srv.Close()
	c := &http.Client{Transport: inject{http.DefaultTransport}}
	ctx := withExtra(context.Background(), map[string]any{"tool_choice": "required"})
	req, _ := http.NewRequestWithContext(ctx, "POST", srv.URL, strings.NewReader(`{"model":"m","messages":[]}`))
	if _, err := c.Do(req); err != nil {
		t.Fatal(err)
	}
	if got["tool_choice"] != "required" || got["model"] != "m" {
		t.Errorf("body = %v", got)
	}
	// Without fields the body is sent as is.
	got = nil
	req, _ = http.NewRequest("POST", srv.URL, strings.NewReader(`{"model":"n"}`))
	c.Do(req)
	if got["model"] != "n" || got["tool_choice"] != nil {
		t.Errorf("body = %v", got)
	}
}

func TestExtra(t *testing.T) {
	s := json.RawMessage(`{"type":"object"}`)
	ol := &Model{Kind: provider.KindOllama, Support: Support{JSONSchema: true}}
	if _, ok := ol.extra(mForced, s); ok {
		t.Error("Ollama forced")
	}
	if f, ok := ol.extra(mNative, s); !ok || f["format"] == nil {
		t.Errorf("Ollama native: %v", f)
	}
	cm := &Model{Kind: provider.KindCompatible, Support: Support{ForcedRequired: true, JSONObject: true}}
	if f, _ := cm.extra(mForced, s); f["tool_choice"] != "required" {
		t.Errorf("forced: %v", f)
	}
	if f, _ := cm.extra(mNative, s); f["response_format"].(map[string]any)["type"] != "json_object" {
		t.Errorf("native: %v", f)
	}
	if _, ok := (&Model{Kind: provider.KindCompatible}).extra(mNative, s); ok {
		t.Error("native without support")
	}
	if f, ok := cm.extra(mTool, s); !ok || f != nil {
		t.Errorf("tool: %v", f)
	}
}

// The methods run through the app's registry on the fake provider: a
// wrong first answer is sent back with its errors, and the retry counts.
func TestAskRetry(t *testing.T) {
	m := tasksByID(t)
	sel := m["select-en"]
	right, _ := json.Marshal(answer(sel))
	wrong := `{"picks":[{"index":12,"reason":"a"},{"index":12,"reason":"a"},{"index":18,"reason":"a"},{"index":8,"reason":"a"},{"index":2,"reason":"a"}]}`

	cases := []struct {
		method  string
		replies []fake.Reply
		valid1  bool
		valid   bool
		fail1   string
		last    string // what the retry message says
	}{
		{mText, []fake.Reply{fake.Text("```json\n" + string(right) + "\n```")}, true, true, "", ""},
		{mText, []fake.Reply{fake.Text(wrong), fake.Text(string(right))}, false, true, failRepeated, "picked twice"},
		{mText, []fake.Reply{fake.Text("Sure! Here are the items."), fake.Text("still no json")}, false, false, failBadJSON, "isn't valid"},
		{mTool, []fake.Reply{fake.Text("I picked 12, 5, 18, 8 and 2."), fake.ToolCall("c1", submitName, json.RawMessage(right))}, false, true, failNoCall, "didn't call"},
		{mTool, []fake.Reply{fake.ToolCall("c1", submitName, json.RawMessage(wrong)), fake.ToolCall("c2", submitName, json.RawMessage(right))}, false, true, failRepeated, "Call submit again"},
	}
	for i, c := range cases {
		for j := range c.replies {
			c.replies[j].Wait = 20 * time.Millisecond // Windows' clock is coarser than the fake
		}
		fp := fake.New(c.replies...)
		kr := &memKeyring{m: map[string]string{provider.KeyName("p"): "k"}}
		sec := secret.New(kr)
		settings := config.LLMSettings{MaxParallelCalls: 2, Providers: map[string]config.ProviderSettings{
			"p": {Kind: string(provider.KindCompatible), BaseURL: "https://x.example/v1/", Models: []config.ModelSettings{{ID: "m", Context: 64000}}},
		}}
		reg := provider.NewRegistry(provider.Deps{Settings: settings, Secrets: sec, Gate: limit.NewGate(2),
			Backends: map[provider.Kind]provider.Factory{provider.KindCompatible: fp.Factory()}})
		a := &asker{reg: reg, redact: sec.Redact, maxTokens: 100}
		r := a.ask(context.Background(), &Model{Ref: "p/m", Kind: provider.KindCompatible}, sel, c.method, false)
		if r.Error != "" || r.Seconds <= 0 || r.Valid1 != c.valid1 || r.Valid != c.valid || (c.fail1 != "" && !slices.Contains(r.Fail1, c.fail1)) {
			t.Errorf("%d: %+v", i, r)
		}
		if c.valid && (r.Score == nil || !r.Score.Right) {
			t.Errorf("%d: score %+v", i, r.Score)
		}
		calls := fp.Calls()
		if c.last != "" {
			last := calls[len(calls)-1].Messages
			msg := last[len(last)-1]
			text := ""
			for _, p := range msg.Parts {
				if p.Text != nil {
					text += p.Text.Text
				}
				if p.ToolResult != nil {
					text += p.ToolResult.Text
				}
			}
			if !strings.Contains(text, c.last) {
				t.Errorf("%d: retry message %q", i, text)
			}
		}
		req := calls[0]
		tools := c.method == mTool
		if (len(req.Tools) == 1) != tools || strings.Contains(req.System[0].Text, `"picks"`) == tools {
			t.Errorf("%d: request tools %d, system %q", i, len(req.Tools), req.System[0].Text[:80])
		}
	}
}
