package agent

import (
	"slices"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// The inspector's record is what the provider got.
func TestTracesRecordWhatWasSent(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings(), big())
	defer h.stop()
	h.o.d.Traces = NewTraces()
	c := h.newChat(chat.Chat{Title: "Prices"})
	h.fp.Push(fake.ToolCall("c1", "big", map[string]int{"n": 20000}), fake.Text("read it"))
	h.turn(c.ID, "hello")

	list := h.o.d.Traces.List()
	if len(list) != 1 || list[0].Chat != c.ID || list[0].Turn != 1 || list[0].Title != "Prices" || list[0].Model != "p/m1" || list[0].Ended.IsZero() {
		t.Fatalf("List = %+v", list)
	}
	if r := list[0].Requests; len(r) != 2 || r[0].Req.Messages != nil {
		t.Errorf("List keeps the texts: %+v", r)
	}
	tr, ok := h.o.d.Traces.Turn(TraceKey{h.pid, c.ID, 1})
	if !ok {
		t.Fatal("turn not recorded")
	}
	calls := h.fp.Calls()
	if len(calls) != len(tr.Requests) {
		t.Fatalf("%d requests recorded, the provider got %d", len(tr.Requests), len(calls))
	}
	for i, r := range tr.Requests {
		got, sent := r.Req, calls[i]
		if jsonOf(t, got.System) != jsonOf(t, sent.System) || jsonOf(t, got.Tools) != jsonOf(t, sent.Tools) || jsonOf(t, got.Messages) != jsonOf(t, sent.Messages) {
			t.Errorf("request %d differs from what the provider got", i)
		}
		if jsonOf(t, r.Blocks) != jsonOf(t, Blocks(sent, 1)) {
			t.Errorf("request %d blocks %+v", i, r.Blocks)
		}
		if !r.Done || r.Usage.Input == 0 || r.Err != "" || r.Last {
			t.Errorf("request %d: %+v", i, r)
		}
	}
	names := []string{}
	for _, b := range tr.Requests[1].Blocks {
		names = append(names, b.Name)
	}
	if want := []string{"Tools", "System prompt, role and skills", "This turn"}; !slices.Equal(names, want) {
		t.Errorf("blocks %q, want %q", names, want)
	}
	if !tr.Requests[1].Blocks[1].Cache {
		t.Error("the cache point after the system block is missing")
	}
	if k := tr.Requests[0].Parts; !slices.Equal(k, []chat.PartKind{chat.PartToolCall}) {
		t.Errorf("parts %v", k)
	}
	if len(tr.Tools) != 1 {
		t.Fatalf("tools %+v", tr.Tools)
	}
	if tl := tr.Tools[0]; tl.Name != "big" || tl.CallID != "c1" || tl.Request != 0 || tl.Bytes < 20000 || tl.Ref == "" || tl.Error || tl.Started.Before(tr.Requests[0].Started) {
		t.Errorf("tool %+v", tl)
	}

	// The next turn of the chat is a new record, the history its own block.
	h.fp.Push(fake.Text("ok"))
	h.turn(c.ID, "again")
	tr2, _ := h.o.d.Traces.Turn(TraceKey{h.pid, c.ID, 2})
	if bs := tr2.Requests[0].Blocks; len(bs) != 4 || bs[2].Name != "History window" || bs[3].Name != "This turn" {
		t.Errorf("turn 2 blocks %+v", bs)
	}
}

// Over the budget the oldest requests lose their texts; past the turn
// limit the oldest turns go.
func TestTracesBudget(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings())
	defer h.stop()
	h.o.d.Traces = &Traces{max: 2, budget: 1}
	c := h.newChat(chat.Chat{Title: "Prices"})
	for _, m := range []string{"one", "two", "three"} {
		h.fp.Push(fake.Text("ok"))
		h.turn(c.ID, m)
	}
	list := h.o.d.Traces.List()
	if len(list) != 2 || list[0].Turn != 3 || list[1].Turn != 2 {
		t.Fatalf("List = %+v", list)
	}
	old, _ := h.o.d.Traces.Turn(TraceKey{h.pid, c.ID, 2})
	last, _ := h.o.d.Traces.Turn(TraceKey{h.pid, c.ID, 3})
	if !old.Requests[0].Dropped || old.Requests[0].Req.System != nil || len(old.Requests[0].Blocks) == 0 {
		t.Errorf("old request %+v", old.Requests[0])
	}
	if last.Requests[0].Dropped || last.Requests[0].Req.System == nil {
		t.Error("the newest request lost its texts")
	}
	if s := h.o.d.Traces; s.size != textSize(last.Requests[0]) {
		t.Errorf("size %d, want %d", s.size, textSize(last.Requests[0]))
	}
}

// A running tool reports the tool's timeout as its limit, and the chat's
// title; the manager lists the open project.
func TestRunningToolHasItsLimit(t *testing.T) {
	started, release := make(chan string, 1), make(chan struct{})
	h := newHarness(t, t.TempDir(), "", testSettings(), blocker(started, release))
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Prices"})
	h.fp.Push(fake.ToolCall("c1", "block", struct{}{}), fake.Text("done"))
	h.send(c.ID, "go")
	<-started
	as := h.pm.Activities()
	if len(as) != 1 || as[0].Name != "Test" || len(as[0].Work) != 1 {
		t.Fatalf("Activities = %+v", as)
	}
	if w := as[0].Work[0]; w.Title != "Prices" || w.Limit != tool.DefaultTimeout || w.LastActivity.Before(w.Started) {
		t.Errorf("work %+v", w)
	}
	close(release)
	h.wait()
	if h.o.d.Models.FirstEvent("p") != provider.FirstEvent {
		t.Error("FirstEvent")
	}
}
