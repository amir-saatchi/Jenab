package main

import (
	"context"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/tool"
)

func TestMatches(t *testing.T) {
	failed := event{Kind: evRun, Pipeline: "daily_btc", Outcome: "failed"}
	for _, c := range []struct {
		s    sub
		want bool
	}{
		{sub{Event: evRun}, true},
		{sub{Event: evRun, Outcome: "failed"}, true},
		{sub{Event: evRun, Outcome: "success"}, false},
		{sub{Event: evRun, Pipeline: "weekly_summary"}, false},
		{sub{Event: evRun, Pipeline: "*", Outcome: "failed"}, true},
		{sub{Event: evSchema}, false},
		{sub{Event: "alerts"}, false},
	} {
		if got := matches(c.s, failed); got != c.want {
			t.Errorf("%s: %v, want %v", c.s, got, c.want)
		}
	}
	if !matches(sub{Event: evSchema, Table: "BTC_prices"}, event{Kind: evSchema, Table: "btc_prices"}) {
		t.Error("table names compare without case")
	}
}

// An event reaches each subscribed chat once, at its next turn, never
// the chat that caused it, and not through an ended subscription.
func TestDelivery(t *testing.T) {
	b := newBus(condUser)
	docs, schema, rev := id.Chat("c_docs"), id.Chat("c_schema"), id.Chat("c_rev")
	b.addChat("docs", docs)
	b.addChat("schema", schema)
	b.addChat("reviewer", rev)
	b.mu.Lock()
	b.subscribe("docs", sub{Event: evSchema}, "user")
	b.subscribe("docs", sub{Event: evSchema, Table: "btc_prices"}, "user") // two matching subscriptions: one notice
	b.subscribe("schema", sub{Event: evSchema}, "user")                    // its own events
	old, _ := b.subscribe("reviewer", sub{Event: evSchema}, "user")
	b.subs[3].Ended = true
	b.mu.Unlock()
	if old.ID != "s_4" {
		t.Fatalf("id %s", old.ID)
	}
	b.emit(event{Kind: evSchema, By: "schema", Table: "btc_prices", Text: "[schema_changed: btc_prices + column volume REAL]"})
	if b.own != 1 {
		t.Errorf("own events skipped %d, want 1", b.own)
	}
	ns, _ := b.notices(context.Background(), nil, chat.Chat{ID: docs})
	if len(ns) != 1 || ns[0].Kind != noticeEvent || !strings.Contains(ns[0].Text, "volume") {
		t.Fatalf("docs got %v", ns)
	}
	if again, _ := b.notices(context.Background(), nil, chat.Chat{ID: docs}); len(again) != 0 {
		t.Errorf("a notice was given twice: %v", again)
	}
	for _, c := range []id.Chat{schema, rev} {
		if ns, _ := b.notices(context.Background(), nil, chat.Chat{ID: c}); len(ns) != 0 {
			t.Errorf("%s got %v", c, ns)
		}
	}
	b.emit(event{Kind: evSchema, By: "app", Table: "x", Text: strings.Repeat("a", 3000)})
	ns, _ = b.notices(context.Background(), nil, chat.Chat{ID: docs})
	if len(ns) != 1 || len([]rune(ns[0].Text)) != noticeChars {
		t.Errorf("a long notice is %d runes, want %d", len([]rune(ns[0].Text)), noticeChars)
	}
}

func toolByName(ts []tool.Tool, name string) tool.Tool {
	for _, t := range ts {
		if t.Spec().Name == name {
			return t
		}
	}
	return nil
}

// publish refuses a channel that doesn't exist and counts it; a report
// reaches the channel's subscribers, and read_report returns its body.
func TestPublish(t *testing.T) {
	b := newBus(condSection)
	watcher, reporter := id.Chat("c_w"), id.Chat("c_r")
	b.addChat("watcher", watcher)
	b.addChat("reporter", reporter)
	b.mu.Lock()
	b.subscribe("reporter", sub{Event: "alerts"}, "user")
	b.subscribe("watcher", sub{Event: "alerts"}, "user")
	b.mu.Unlock()
	ts := b.tools()
	pub, read := toolByName(ts, "publish"), toolByName(ts, "read_report")
	ctx := context.Background()
	env := &tool.Env{Chat: watcher}
	if _, err := pub.Run(ctx, tool.Call{Env: env, Args: []byte(`{"channel":"general","title":"x","body":"y"}`)}); err == nil || !strings.Contains(err.Error(), "alerts") {
		t.Fatalf("an unknown channel: %v", err)
	}
	res, err := pub.Run(ctx, tool.Call{Env: env, Args: []byte(`{"channel":"alerts","title":"BTC fell 6.4% today","body":"From 71,260 to 66,700."}`)})
	if err != nil || !strings.Contains(res.Text, "rp_2") {
		t.Fatalf("publish: %v %v", res, err)
	}
	_, _, _, pubs, own, _ := b.snapshot()
	if len(pubs) != 2 || !pubs[0].Refused || pubs[1].Refused || own != 1 {
		t.Errorf("pubs %+v own %d", pubs, own)
	}
	ns, _ := b.notices(ctx, nil, chat.Chat{ID: reporter})
	if len(ns) != 1 || !strings.Contains(ns[0].Text, `alerts by chat "Price watcher": BTC fell 6.4% today`) || !strings.Contains(ns[0].Text, "rp_2") {
		t.Fatalf("reporter got %v", ns)
	}
	res, err = read.Run(ctx, tool.Call{Env: &tool.Env{Chat: reporter}, Args: []byte(`{"id":"rp_2"}`)})
	if err != nil || !strings.Contains(res.Text, "66,700") {
		t.Errorf("read_report: %v %v", res, err)
	}
	if s := b.block5("reporter"); !strings.Contains(s, "- s_1: alerts") || !strings.Contains(s, `"BTC fell 6.4% today" (Price watcher, rp_2)`) || !strings.Contains(s, "- reviews:") {
		t.Errorf("block 5:\n%s", s)
	}
}

// subscribe makes a subscription once, refuses a wrong filter, and
// unsubscribe ends only the chat's own.
func TestSubscribeTool(t *testing.T) {
	b := newBus(condAgent)
	docs, rev := id.Chat("c_docs"), id.Chat("c_rev")
	b.addChat("docs", docs)
	b.addChat("reviewer", rev)
	ts := b.tools()
	subT, unsub := toolByName(ts, "subscribe"), toolByName(ts, "unsubscribe")
	ctx := context.Background()
	if _, err := subT.Run(ctx, tool.Call{Env: &tool.Env{Chat: docs}, Args: []byte(`{"event":"schema_changed","filter":{"outcome":"failed"}}`)}); err == nil {
		t.Error("schema_changed took an outcome")
	}
	for range 2 {
		if _, err := subT.Run(ctx, tool.Call{Env: &tool.Env{Chat: rev}, Args: []byte(`{"event":"pipeline_run","filter":{"outcome":"failed","pipeline":"*"}}`)}); err != nil {
			t.Fatal(err)
		}
	}
	subs, _, _, _, _, dups := b.snapshot()
	if len(subs) != 1 || dups != 1 || subs[0].sub != (sub{Event: evRun, Outcome: "failed"}) || subs[0].By != "agent" {
		t.Fatalf("subs %+v dups %d", subs, dups)
	}
	if _, err := unsub.Run(ctx, tool.Call{Env: &tool.Env{Chat: docs}, Args: []byte(`{"id":"s_1"}`)}); err == nil {
		t.Error("a chat ended another chat's subscription")
	}
	if _, err := unsub.Run(ctx, tool.Call{Env: &tool.Env{Chat: rev}, Args: []byte(`{"id":"s_1"}`)}); err != nil {
		t.Fatal(err)
	}
	if s := b.block5("reviewer"); !strings.Contains(s, "- none") || strings.Contains(s, "channels") {
		t.Errorf("block 5:\n%s", s)
	}
	if s := newBus(condNone).block5("reviewer"); s != "" {
		t.Errorf("condition 1 has block 5 text: %q", s)
	}
}

// The key scores: a needed event with the wrong filter is made but not
// right; each agent's tools follow its groups and the condition.
func TestKey(t *testing.T) {
	need := agentByKey("reviewer").Need[0]
	if m, r := matchesKey(sub{Event: evRun}, need); !m || r {
		t.Errorf("no outcome filter: match %v right %v", m, r)
	}
	if m, r := matchesKey(sub{Event: evRun, Outcome: "failed"}, need); !m || !r {
		t.Errorf("the key's filter: match %v right %v", m, r)
	}
	ts := toolsOf(agentByKey("docs").Groups, condAgent)
	for _, want := range []string{"query", "bucket_put", "subscribe", "ask_user"} {
		if !contains(ts, want) {
			t.Errorf("docs lacks %s", want)
		}
	}
	for _, not := range []string{"insert", "publish", "web_search"} {
		if contains(ts, not) {
			t.Errorf("docs has %s", not)
		}
	}
	if ts := toolsOf(agentByKey("watcher").Groups, condTool); !contains(ts, "publish") || !contains(ts, "read_report") || contains(ts, "subscribe") {
		t.Errorf("watcher in condition 4: %v", ts)
	}
	if got := keySubs(agentByKey("reporter"), condSection); len(got) != 1 || got[0].Event != "alerts" {
		t.Errorf("reporter's key in condition 5: %v", got)
	}
	if got := keySubs(agentByKey("docs"), condAgent); got != nil {
		t.Errorf("condition 3 sets up %v", got)
	}
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}
