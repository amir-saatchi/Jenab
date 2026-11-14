package main

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"strings"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
)

// dryModel is a rule-based stand-in for a model, for -dry: each agent
// calls the tools its message needs, subscribes and publishes where it
// can (and once to a channel that doesn't exist), so every path of the
// harness runs without a key. Its answers mean nothing.
type dryModel struct{}

func (dryModel) Models(context.Context) ([]provider.ModelInfo, error) { return nil, nil }

func (dryModel) Stream(ctx context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	return fake.New(dryReply(req)).Stream(ctx, req)
}

func dryReply(req provider.Request) fake.Reply {
	usage := chat.Usage{Input: provider.EstimateTokens(req), Output: 20}
	has := map[string]bool{}
	for _, t := range req.Tools {
		has[t.Name] = true
	}
	var sys strings.Builder
	for _, b := range req.System {
		sys.WriteString(b.Text + "\n")
	}
	role := sys.String()
	ms := req.Messages
	last := ms[len(ms)-1]
	var user, notices, history string
	called := false // a call since the user's last message
	for _, m := range ms {
		for _, p := range m.Parts {
			switch {
			case p.Notice != nil:
				notices += p.Notice.Text + "\n"
			case p.Text != nil:
				history += p.Text.Text + "\n"
				if m.Role == chat.RoleUser && !strings.HasPrefix(p.Text.Text, "[") {
					user, called = p.Text.Text, false
				}
			case p.ToolCall != nil:
				history += p.ToolCall.Name + "\n"
				called = true
			}
		}
	}
	text := func(s string) fake.Reply {
		return fake.Reply{Events: []provider.Event{
			{Kind: provider.EventPart, Part: &chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: s}}},
			fake.Done(provider.StopEnd, usage),
		}}
	}
	if req.ToolChoice != "" || last.Role == chat.RoleTool || called {
		return text("Done (dry run). " + strings.TrimSpace(notices))
	}
	var cs [][2]any
	add := func(name string, args any) {
		if has[name] {
			cs = append(cs, [2]any{name, args})
		}
	}
	first := !strings.Contains(history, "subscribe")
	switch {
	case strings.Contains(role, "keep docs/schema.md"):
		if first {
			add("subscribe", map[string]any{"event": "schema_changed"})
		}
		content := "# Schema\n\nbtc_prices(date, price), btc_news, watch_keywords."
		if strings.Contains(notices, "volume") || strings.Contains(sys.String(), "volume") {
			content += "\nbtc_prices.volume REAL."
		}
		if strings.Contains(notices, "eth_prices") || strings.Contains(sys.String(), "eth_prices") {
			content += "\neth_prices(date, price)."
		}
		add("bucket_put", map[string]any{"key": "docs/schema.md", "content": content})
	case strings.Contains(role, "pipeline run that fails"):
		if first {
			add("subscribe", map[string]any{"event": "pipeline_run", "filter": map[string]any{"outcome": "failed"}})
		}
		if strings.Contains(notices, "r_q7m2") {
			add("run_status", map[string]any{"run_id": "r_q7m2"})
		} else {
			add("read_config", map[string]any{"id": "weekly_summary"})
		}
	case strings.Contains(role, "schema changes the user asks"):
		add("request_schema_change", map[string]any{"goal": user})
		add("publish", map[string]any{"channel": "data", "title": "Schema changed", "body": user})
	case strings.Contains(role, "watch the Bitcoin price"):
		add("query", map[string]any{"sql": "SELECT date, price FROM btc_prices ORDER BY date DESC LIMIT 2"})
		switch {
		case strings.Contains(user, "Today's price"):
			add("publish", map[string]any{"channel": "alerts", "title": "BTC fell 6.4% today", "body": "From 71,260 to 66,700."})
		case strings.Contains(user, "Any change"):
			add("publish", map[string]any{"channel": "general", "title": "No change", "body": "Nothing new."})
		}
	case strings.Contains(role, "weekly Bitcoin summary"):
		if first {
			add("subscribe", map[string]any{"event": "pipeline_run", "filter": map[string]any{"pipeline": "weekly_summary"}})
		}
		draft := "# Week\n\nPrices so far."
		if i := strings.Index(notices, "report rp_"); i >= 0 {
			id := strings.Fields(notices[i+len("report "):])[0]
			add("read_report", map[string]any{"id": strings.TrimRight(id, ",")})
			draft += "\nBTC fell 6.4% today."
		}
		add("bucket_put", map[string]any{"key": "summaries/draft.md", "content": draft})
	case strings.Contains(role, "add the rows the user gives you"):
		if first {
			add("subscribe", map[string]any{"event": "schema_changed", "filter": map[string]any{"table": "watch_keywords"}})
		}
		if strings.Contains(user, "keywords") {
			add("insert", map[string]any{"table": "watch_keywords", "rows": []map[string]any{{"keyword": "halving"}, {"keyword": "MiCA"}}})
		} else {
			add("insert", map[string]any{"table": "btc_news", "rows": []map[string]any{{"url": "https://news.example.com/crypto/1010", "date": "2026-11-12",
				"title": "Bitcoin falls 6% as ETF outflows grow", "reason": "خروج پول از صندوق‌ها"}}})
		}
	}
	if len(cs) == 0 {
		return text("OK (dry run).")
	}
	var evs []provider.Event
	for i, c := range cs {
		b, _ := json.Marshal(c[1])
		evs = append(evs, provider.Event{Kind: provider.EventPart, Part: &chat.Part{Kind: chat.PartToolCall,
			ToolCall: &chat.ToolCall{ID: fmt.Sprintf("dry%d_%d", len(ms), i), Name: c[0].(string), Args: b}}})
	}
	return fake.Reply{Events: append(evs, fake.Done(provider.StopToolUse, usage))}
}
