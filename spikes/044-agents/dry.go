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

// dryModel is a rule-based stand-in for a model, for -dry: Mother creates
// a chat for any request, and an agent calls the tools its task needs,
// also ones it lacks, so every path of the harness runs without a key.
// Its answers mean nothing.
type dryModel struct{}

func (dryModel) Models(context.Context) ([]provider.ModelInfo, error) { return nil, nil }

func (dryModel) Stream(ctx context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	return fake.New(dryReply(req)).Stream(ctx, req)
}

func dryReply(req provider.Request) fake.Reply {
	usage := chat.Usage{Input: provider.EstimateTokens(req), Output: 20}
	has := map[string]string{}
	for _, t := range req.Tools {
		has[t.Name] = string(t.Schema)
	}
	ms := req.Messages
	last := ms[len(ms)-1]
	var user string
	for i := len(ms) - 1; i >= 0 && user == ""; i-- {
		if ms[i].Role == chat.RoleUser {
			for _, p := range ms[i].Parts {
				if p.Text != nil {
					user = p.Text.Text
				}
			}
		}
	}
	text := func(s string) fake.Reply {
		return fake.Reply{Events: []provider.Event{
			{Kind: provider.EventPart, Part: &chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: s}}},
			fake.Done(provider.StopEnd, usage),
		}}
	}
	calls := func(cs ...[2]any) fake.Reply {
		var evs []provider.Event
		for i, c := range cs {
			b, _ := json.Marshal(c[1])
			evs = append(evs, provider.Event{Kind: provider.EventPart, Part: &chat.Part{Kind: chat.PartToolCall,
				ToolCall: &chat.ToolCall{ID: fmt.Sprintf("dry%d_%d", len(ms), i), Name: c[0].(string), Args: b}}})
		}
		return fake.Reply{Events: append(evs, fake.Done(provider.StopToolUse, usage))}
	}
	if req.ToolChoice != "" {
		return text("Done (dry run).")
	}
	if last.Role == chat.RoleTool {
		for _, p := range last.Parts {
			if r := p.ToolResult; r != nil && r.IsError && strings.Contains(r.Text, refusedMark) {
				return text("I can't do that here: the tool isn't one of this chat's tools.")
			}
		}
		return text("Done (dry run).")
	}
	low := strings.ToLower(user)
	if s, ok := has["create_chat"]; ok {
		if strings.Contains(low, "which") || strings.Contains(low, "what") || strings.Contains(low, "how many") || strings.Contains(user, "چند") {
			return text("The card has it (dry run).")
		}
		tools := []string{groups[0].Name, "change_data", "files"}
		if strings.Contains(s, `"query"`) && !strings.Contains(s, `"`+groups[0].Name+`"`) {
			tools = []string{"query", "insert", "bucket_put"}
		}
		return calls([2]any{"create_chat", map[string]any{"title": "Dry agent " + fmt.Sprint(len(user)%97), "role": "You do the job the user asked for. Report what you did.",
			"message": user, "tools": tools}})
	}
	if strings.Contains(user, "[Message from Mother]") {
		if strings.Contains(low, "stablecoin") {
			return calls([2]any{"insert", map[string]any{"table": "watch_keywords", "rows": []map[string]any{{"keyword": "stablecoin"}, {"keyword": "MiCA"}}}})
		}
		return text("Ready (dry run).")
	}
	switch {
	case strings.Contains(low, "weekly_summary") && len(ms) < 3: // one form, so the harness's answer runs
		return calls([2]any{"ask_user", map[string]any{"questions": []map[string]any{{"header": "Depth", "question": "How deep a review?",
			"options": []map[string]any{{"label": "Quick"}, {"label": "Full"}}}}}})
	case strings.Contains(low, "weekly_summary"):
		return calls([2]any{"read_config", map[string]any{"id": "weekly_summary"}})
	case strings.Contains(low, "eth price"):
		return calls([2]any{"request_schema_change", map[string]any{"goal": "a table for daily ETH prices"}},
			[2]any{"save_pipeline", map[string]any{"config": strings.ReplaceAll(strings.ReplaceAll(dailyBTC, "daily_btc", "daily_eth"), "btc_prices", "eth_prices")}})
	case strings.Contains(low, "closing prices"):
		return calls([2]any{"insert", map[string]any{"table": "btc_prices", "rows": []map[string]any{
			{"date": "2026-09-01", "price": 57310.20}, {"date": "2026-09-02", "price": 57902.75}, {"date": "2026-09-03", "price": 56880.10},
			{"date": "2026-09-04", "price": 56115.00}, {"date": "2026-09-05", "price": 56740.55}}}})
	case strings.Contains(low, "docs/schema.md"):
		return calls([2]any{"bucket_put", map[string]any{"key": "docs/schema.md", "content": "# Schema\n\nbtc_prices, btc_news, watch_keywords."}})
	case strings.Contains(low, "coingecko"):
		return calls([2]any{"web_search", map[string]any{"query": "bitcoin price api"}},
			[2]any{"bucket_put", map[string]any{"key": "notes/apis.md", "content": "CoinGecko, CoinCap, Binance."}})
	case strings.Contains(user, "digest/"):
		return calls([2]any{"query", map[string]any{"sql": "SELECT title FROM btc_news"}},
			[2]any{"bucket_put", map[string]any{"key": "digest/2026-11-12.md", "content": strings.Repeat("خلاصه اخبار هفته: صندوق‌های بیت‌کوین ورودی بزرگی داشتند. ", 5)}})
	case strings.Contains(low, "halving"):
		return calls([2]any{"insert", map[string]any{"table": "watch_keywords", "rows": []map[string]any{{"keyword": "halving"}}}})
	}
	return text("OK (dry run).")
}
