package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
	"github.com/amir-saatchi/jenab/internal/tool"
)

func TestRequestCap(t *testing.T) {
	set := testSettings()
	set.LLM.TurnMaxRequests = 3
	h := newHarness(t, t.TempDir(), "", set, echo())
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Prices"})
	for i := range 5 {
		h.fp.Push(fake.ToolCall(fmt.Sprint("c", i), "echo", map[string]string{"text": "again"}))
	}
	h.turn(c.ID, "loop")
	calls := h.fp.Calls()
	if len(calls) != 3 {
		t.Fatalf("%d requests, want the cap of 3", len(calls))
	}
	if len(calls[1].Tools) != 1 || calls[2].Tools != nil {
		t.Errorf("tools %d, %d; want the last request without tools", len(calls[1].Tools), len(calls[2].Tools))
	}
	got := texts(calls[2].Messages)
	if got[len(got)-1] != "user: "+lastRequest {
		t.Errorf("last request ends with %q", got[len(got)-1])
	}
	ms := h.messages(c.ID)
	for _, s := range texts(ms) {
		if strings.Contains(s, lastRequest) {
			t.Error("the last-request line is stored")
		}
	}
	// The model answered the last request with a call anyway: it is
	// stored, closed and never run.
	if got := texts(ms); got[len(got)-1] != "tool: result not run: the turn reached its request limit" {
		t.Errorf("last call: %q", got[len(got)-1])
	}
	validHistory(t, ms)
}

func TestTitle(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings())
	defer h.stop()
	c := h.newChat(chat.Chat{})
	h.fp.Push(fake.Text("BTC is at 60k."), fake.Text("\"Bitcoin price today.\"\nmore text"))
	h.turn(c.ID, "What is the bitcoin price?")
	calls := h.fp.Calls()
	if len(calls) != 2 {
		t.Fatalf("%d requests, want the turn and the title", len(calls))
	}
	req := calls[1]
	if req.System[0].Text != titlePrompt || req.Tools != nil || req.Model != "small" {
		t.Errorf("title request %+v", req)
	}
	if got := texts(req.Messages); !slices.Equal(got, []string{"user: User: What is the bitcoin price?\n\nAssistant: BTC is at 60k."}) {
		t.Errorf("title input %q", got)
	}
	h.open(func(p *project.Project) {
		ch, err := p.Chats.Chat(context.Background(), c.ID)
		if err != nil || ch.Title != "Bitcoin price today" || ch.TitleFixed {
			t.Errorf("chat %+v, %v", ch, err)
		}
	})
	// A titled chat gets no more title requests.
	h.turn(c.ID, "and ETH?")
	if n := len(h.fp.Calls()); n != 3 {
		t.Errorf("%d requests, want 3", n)
	}
}

func TestCleanTitle(t *testing.T) {
	for in, want := range map[string]string{
		"Bitcoin price":              "Bitcoin price",
		"  \"Bitcoin  price.\"  ":    "Bitcoin price",
		"**Prices**\nexplanation":    "Prices",
		"\n\n«قیمت بیت\u200cکوین»\n": "قیمت بیت\u200cکوین",
		"\u200f‘قیمت طلا’۔\u200c":    "قیمت طلا",
		"„Price“…":                   "Price",
		"Cafe\u0301 prices":          "Caf\u00e9 prices",
		"":                           "",
		strings.Repeat("a", 150):     strings.Repeat("a", 100),
	} {
		if got := cleanTitle(in); got != want {
			t.Errorf("cleanTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMotherContext(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings(),
		tool.Func(tool.Spec{Name: "mother_only", Description: "Only Mother.", Schema: json.RawMessage(`{"type": "object"}`), Mother: true},
			func(context.Context, *tool.Env, struct{}) (tool.Result, error) { return tool.Result{Text: "ran"}, nil }))
	defer h.stop()
	m := h.mother()
	eth := h.newChat(chat.Chat{Title: "ETH tracker", Role: "Track the ETH price.\nKeep a weekly log."})
	h.newChat(chat.Chat{Title: "Old"})
	h.open(func(p *project.Project) {
		ctx := context.Background()
		p.Chats.Archive(ctx, eth.ID, false)
		cs, _ := p.Chats.Chats(ctx)
		p.Chats.Archive(ctx, cs[2].ID, true)
		p.Chats.SaveNotes(ctx, chat.SessionNote{Chat: eth.ID, Content: "Weekly log for weeks 36-39.\nMore."})
	})
	h.turn(eth.ID, "hi") // last activity
	h.fp.Push(fake.ToolCall("x", "mother_only", struct{}{}))
	h.turn(m.ID, "what runs?")
	req := h.fp.Calls()[1]
	if len(req.System) != 2 || req.System[0].Cache || !req.System[1].Cache {
		t.Fatalf("system blocks %+v", req.System)
	}
	if !strings.Contains(req.System[0].Text, strings.TrimSpace(motherPrompt)) || !strings.Contains(req.System[0].Text, "Requests with several parts") {
		t.Errorf("block 1: %s", req.System[0].Text)
	}
	list := req.System[1].Text
	want := fmt.Sprintf("Chat list\n- %s \"ETH tracker\". Role: Track the ETH price. Status: idle. Last activity: ", eth.ID)
	if !strings.HasPrefix(list, want) || !strings.HasSuffix(list, ". Notes: Weekly log for weeks 36-39.") || strings.Contains(list, "Old") || strings.Count(list, "\n") != 1 {
		t.Errorf("chat list:\n%s", list)
	}
	if !slices.ContainsFunc(req.Tools, func(d provider.ToolDef) bool { return d.Name == "mother_only" }) {
		t.Error("Mother lacks her tool")
	}
	if got := texts(h.messages(m.ID)); got[2] != "tool: result ran" {
		t.Errorf("Mother's call: %q", got)
	}

	// Another chat neither sees the tool nor can call it.
	h.fp.Push(fake.ToolCall("y", "mother_only", struct{}{}))
	h.turn(eth.ID, "try it")
	calls := h.fp.Calls()
	req = calls[len(calls)-2]
	if slices.ContainsFunc(req.Tools, func(d provider.ToolDef) bool { return d.Name == "mother_only" }) {
		t.Error("a chat sees Mother's tool")
	}
	if !strings.Contains(req.System[0].Text, "This chat's role:\nTrack the ETH price.\nKeep a weekly log.") || len(req.System) != 1 {
		t.Errorf("role block %+v", req.System)
	}
	ms := h.messages(eth.ID)
	if got := texts(ms); !slices.Contains(got, "tool: result there is no tool named mother_only") {
		t.Errorf("chat's call: %q", got)
	}
}

func TestToolFailures(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings(),
		tool.Func(tool.Spec{Name: "boom", Description: "Panics.", Schema: json.RawMessage(`{"type": "object"}`)},
			func(context.Context, *tool.Env, struct{}) (tool.Result, error) { panic("oops") }),
		tool.Func(tool.Spec{Name: "broken", Description: "Fails.", Schema: json.RawMessage(`{"type": "object"}`)},
			func(context.Context, *tool.Env, struct{}) (tool.Result, error) {
				return tool.Result{}, errors.New("disk full")
			}),
		tool.Func(tool.Spec{Name: "picky", Description: "Wants text.", Schema: json.RawMessage(`{"type": "object", "properties": {"text": {"type": "string"}}, "required": ["text"]}`)},
			func(context.Context, *tool.Env, struct{}) (tool.Result, error) { return tool.Result{Text: "ok"}, nil }),
		tool.Func(tool.Spec{Name: "picture", Description: "An image.", Schema: json.RawMessage(`{"type": "object"}`)},
			func(context.Context, *tool.Env, struct{}) (tool.Result, error) {
				return tool.Result{Text: "an image", Images: []chat.Image{{Ref: "img/a.png", MIME: "image/png"}}}, nil
			}))
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Prices"})
	r := fake.ToolCall("a", "boom", struct{}{})
	for _, call := range []*chat.ToolCall{{ID: "b", Name: "broken"}, {ID: "c", Name: "picky"}, {ID: "d", Name: "nothing"}, {ID: "e", Name: "picture"}} {
		call.Args = []byte(`{}`)
		r.Events = slices.Insert(r.Events, len(r.Events)-1, provider.Event{Kind: provider.EventPart, Part: &chat.Part{Kind: chat.PartToolCall, ToolCall: call}})
	}
	h.fp.Push(r)
	h.turn(c.ID, "go")
	ms := h.messages(c.ID)
	res := ms[2].Parts
	want := []string{"boom failed: the tool crashed: oops", "broken failed: disk full", "wrong arguments:", "there is no tool named nothing", "an image"}
	for i, w := range want {
		if !strings.HasPrefix(res[i].ToolResult.Text, w) || res[i].ToolResult.IsError != (i < 4) {
			t.Errorf("result %d: %+v, want %q", i, res[i].ToolResult, w)
		}
	}
	if len(res) != 6 || res[5].Image == nil || res[5].Image.Ref != "img/a.png" {
		t.Errorf("parts %+v, want the image after its result", res)
	}
	validHistory(t, ms)
}

// say returns its text, which it needs.
func say() tool.Tool {
	return tool.Func(tool.Spec{Name: "say", Description: "Says it back.", Schema: json.RawMessage(`{"type": "object", "properties": {"text": {"type": "string"}}, "required": ["text"]}`)},
		func(_ context.Context, _ *tool.Env, a struct{ Text string }) (tool.Result, error) {
			return tool.Result{Text: a.Text}, nil
		})
}

// badJSON is an answer with one call whose arguments are not valid JSON.
func badJSON(id string, stop provider.StopReason) fake.Reply {
	return fake.Reply{Events: []provider.Event{
		{Kind: provider.EventPart, Part: &chat.Part{Kind: chat.PartToolCall, ToolCall: provider.ToolCall(id, "say", `{"text":`, "")}},
		fake.Done(stop, chat.Usage{Input: 10, Output: 5}),
	}}
}

// TestBadToolJSON: a call with arguments that are not valid JSON is not
// run; the model gets the parser's error and the next call runs (SPEC 3.8).
func TestBadToolJSON(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings(), say())
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Echo"})
	h.fp.Push(badJSON("a", provider.StopToolUse), fake.ToolCall("b", "say", map[string]string{"text": "hi"}), fake.Text("done"))
	h.turn(c.ID, "go")
	ms := h.messages(c.ID)
	if len(ms) != 6 {
		t.Fatalf("%d messages, want 6", len(ms))
	}
	if r := ms[2].Parts[0].ToolResult; !r.IsError || r.Text != "the arguments are not valid JSON: unexpected end of JSON input, at byte 8 of 8. Send the call again with valid JSON." {
		t.Errorf("result = %+v", r)
	}
	if r := ms[4].Parts[0].ToolResult; r.IsError || r.Text != "hi" {
		t.Errorf("second result = %+v", r)
	}
	sent := h.fp.Calls()[1].Messages[1].Parts[0].ToolCall
	if sent == nil || string(sent.Args) != "{}" || sent.Invalid != `{"text":` {
		t.Errorf("the bad call sent back as %+v, want Args {} and the raw text kept", sent)
	}
	validHistory(t, ms)
}

// TestCutToolCall: a call cut off at the output limit is dropped, and the
// turn shows its cut notice.
func TestCutToolCall(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings(), say())
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Echo"})
	h.fp.Push(badJSON("a", provider.StopMaxTokens), fake.Text("unused"))
	h.turn(c.ID, "go")
	ms := h.messages(c.ID)
	if len(ms) != 2 || ms[1].Parts[0].Notice == nil || ms[1].Parts[0].Notice.Kind != chat.NoticeAnswerCut {
		t.Fatalf("messages %+v, want the question and the cut notice", ms)
	}
	if n := len(h.fp.Calls()); n != 1 {
		t.Errorf("%d requests, want 1", n)
	}
}

// TestBadCallsStop: 3 requests in a row whose calls all fail to run stop
// the turn; a call that runs resets the count (8.3).
func TestBadCallsStop(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings(), say())
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Echo"})
	unknown := fake.ToolCall("u", "nothing", struct{}{})
	wrong := fake.ToolCall("w", "say", struct{}{})
	good := fake.ToolCall("g", "say", map[string]string{"text": "hi"})
	h.fp.Push(badJSON("a", provider.StopToolUse), unknown, good, wrong, badJSON("b", provider.StopToolUse), unknown, fake.Text("unused"))
	h.turn(c.ID, "go")
	if n := len(h.fp.Calls()); n != 6 {
		t.Errorf("%d requests, want 6", n)
	}
	ms := h.messages(c.ID)
	n := ms[len(ms)-1].Parts[0].Notice
	if n == nil || n.Kind != chat.NoticeTurnFailed || !strings.Contains(n.Text, "couldn't run, 3 times in a row") {
		t.Errorf("last message = %+v, want the turn_failed notice", ms[len(ms)-1])
	}
	validHistory(t, ms)
}

func TestInTurnTrimming(t *testing.T) {
	set := testSettings()
	set.LLM.Providers["p"].Models[0].Context = 7600 // trims past 3,800 tokens
	h := newHarness(t, t.TempDir(), "", set, big())
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Prices"})
	for i := range 4 {
		h.fp.Push(read(fake.ToolCall(fmt.Sprint("c", i), "big", map[string]int{"n": 20000}), 10000))
	}
	h.fp.Push(read(fake.Text("done"), 10000), read(fake.Text("ok"), 10000))
	h.turn(c.ID, "read four")
	calls := h.fp.Calls()
	previews := func(req provider.Request) (n, stubs int) {
		for _, m := range req.Messages {
			for _, p := range m.Parts {
				if p.ToolResult != nil {
					n++
					if !strings.Contains(p.ToolResult.Text, "\n") {
						stubs++
					}
				}
			}
		}
		return n, stubs
	}
	// Each preview is about 1,500 tokens: the third result passes the
	// ratio, so all but the two newest become stubs, and stay stubs.
	for i, want := range [][2]int{{0, 0}, {1, 0}, {2, 0}, {3, 1}, {4, 2}} {
		if n, stubs := previews(calls[i]); n != want[0] || stubs != want[1] {
			t.Errorf("request %d: %d results, %d stubs; want %v", i+1, n, stubs, want)
		}
	}
	// The next turn changes nothing before its end: no cut is due.
	h.turn(c.ID, "next")
	if n, stubs := previews(h.fp.Calls()[5]); n != 4 || stubs != 2 {
		t.Errorf("next turn: %d results, %d stubs; want 4 and 2", n, stubs)
	}
}

func TestCutAfterIdle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings(), big())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Prices"})
		for n := 1; n <= 5; n++ {
			h.turn(c.ID, fmt.Sprint("message ", n))
		}
		time.Sleep(cacheIdle - time.Second)
		h.turn(c.ID, "six")
		if got := turnsOf(h.fp.Calls()[5].Messages); !slices.Equal(got, []int{1, 2, 3, 4, 5, 6}) {
			t.Errorf("before the cache expired: turns %v", got)
		}
		time.Sleep(cacheIdle + time.Second)
		h.turn(c.ID, "seven")
		if got := turnsOf(h.fp.Calls()[6].Messages); !slices.Equal(got, []int{4, 5, 6, 7}) {
			t.Errorf("after the cache expired: turns %v", got)
		}
	})
}

func TestSendAndEvents(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings())
	c := h.newChat(chat.Chat{Title: "Prices"})
	if _, err := h.o.Send(context.Background(), h.pid, c.ID, UserMessage{Text: "  \n"}); !errors.Is(err, ErrEmpty) {
		t.Errorf("empty message: %v", err)
	}
	h.turn(c.ID, "hello")
	st := h.pub.allStatuses()
	if len(st) != 3 || st[0].State != chat.StateWorking || st[0].Streaming != "" ||
		st[1].State != chat.StateWorking || st[1].Streaming == "" ||
		st[2].State != chat.StateIdle || st[2].Streaming != "" || st[2].Seq <= st[0].Seq {
		t.Fatalf("statuses %+v", st)
	}
	ds := h.pub.allDeltas()
	if len(ds) != 1 || ds[0].Text != "ok" || ds[0].Seq != st[0].Seq {
		t.Errorf("deltas %+v", ds)
	}
	h.pub.mu.Lock()
	parts := slices.Clone(h.pub.parts)
	h.pub.mu.Unlock()
	if len(parts) != 2 || parts[1].Seq != st[2].Seq || parts[1].Message != st[1].Streaming || ds[0].Message != st[1].Streaming || parts[1].Part.Text.Text != "ok" {
		t.Errorf("parts %+v", parts)
	}
	h.stop()
	if _, err := h.o.Send(context.Background(), h.pid, c.ID, UserMessage{Text: "late"}); !errors.Is(err, ErrRefused) {
		t.Errorf("after Refuse: %v", err)
	}
}

func TestMessageDuringLastRequest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		set := testSettings()
		set.LLM.TurnMaxRequests = 2
		h := newHarness(t, t.TempDir(), "", set, echo())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Prices"})
		slow := fake.Text("first answer")
		slow.Wait = time.Second
		h.fp.Push(fake.ToolCall("c1", "echo", map[string]string{"text": "hi"}), slow, fake.Text("second answer"))
		h.send(c.ID, "first")
		time.Sleep(time.Second + time.Millisecond) // the last request is streaming
		h.send(c.ID, "second")
		h.wait()
		calls := h.fp.Calls()
		if len(calls) != 3 || calls[2].Tools != nil {
			t.Fatalf("%d requests; want one more without tools for the message", len(calls))
		}
		want := []string{"user: first", "assistant: call echo", "tool: result hi", "assistant: first answer", "user: second", "assistant: second answer"}
		if got := texts(h.messages(c.ID)); !slices.Equal(got, want) {
			t.Errorf("stored %q\nwant %q", got, want)
		}
	})
}

func TestCutBytes(t *testing.T) {
	for _, c := range []struct {
		s    string
		n    int
		want string
	}{
		{"abc", 5, "abc"},
		{"abcdef", 3, "abc"},
		{"abبی", 3, "ab"}, // never half a character
		{"abبی", 4, "abب"},
		{"ب", 1, ""},
	} {
		if got := cutBytes(c.s, c.n); got != c.want {
			t.Errorf("cutBytes(%q, %d) = %q, want %q", c.s, c.n, got, c.want)
		}
	}
}
