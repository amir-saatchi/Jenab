package main

import (
	"fmt"
	"math/rand"
	"strings"

	"github.com/openai/openai-go/v3"
)

// burrowRequest builds a synthetic request shaped like SPEC 3.1: system prompt, user memory,
// project memory, project card, session notes (one system message, most stable first), then a
// history window of previous turns with tool calls and tool result previews, then the current turn.
// All content is generated here; nothing comes from the repo or the user.

var coins = []string{"BTC", "ETH", "SOL", "ADA", "DOT", "XRP", "LTC", "AVAX"}

func systemBlocks(nonce string) string {
	var b strings.Builder
	b.WriteString("# System prompt\n")
	if nonce != "" {
		b.WriteString("Session " + nonce + ".\n")
	}
	b.WriteString(`You are the agent inside Burrow, a desktop app for personal data projects. You help the user collect,
store, query and show data. You work only through the tools you are given. You never invent data: if a value is
not in a tool result, say so. Tool results larger than the preview size are stored under a ref; use read_ref or
search_ref to read more. Memory holds intent (goals, preferences, decisions, conventions), never values that live in
tables. Keep answers short and plain. When a request needs several independent tool calls, make them in parallel.
Pipelines are the way to collect recurring data; insert is for a few manual rows only.
`)
	b.WriteString("\n# User memory\n## preferences\n- Answers in English, short sentences.\n- Currency: EUR. Dates as YYYY-MM-DD.\n## goals\n- Track a small crypto portfolio and see weekly trends.\n")
	b.WriteString("\n# Project memory\n## decisions\n- Prices come from the daily_btc pipeline at 08:00 (reason: one source, one time).\n- ETH was added on 2026-06-01; older ETH rows do not exist.\n## conventions\n- Coin symbols in upper case.\n- Charts use a 90-day window unless asked otherwise.\n")
	b.WriteString("\n# Project card (as of 2026-09-28 07:00)\nTables:\n")
	b.WriteString("- prices(date DATE, coin TEXT, price_eur REAL, source TEXT) PK(date, coin); 2,912 rows; date 2025-06-01..2026-09-27\n")
	b.WriteString("- coins(symbol TEXT PK, name TEXT, added DATE); 8 rows\n- holdings(coin TEXT PK, amount REAL, note TEXT); 5 rows\n")
	b.WriteString("Views: btc_chart (chart), holdings_table (table), add_holding (form)\nPipelines: daily_btc (cron 08:00, last run success 2026-09-27 08:00), weekly_report (cron Mon 09:00, last run success)\nBucket: reports/ (14 objects), cache/ (31 objects); 6 saved links\n")
	b.WriteString("\n# Session notes\nTask: review price trends for the portfolio. Plan: look at the last weeks per coin, then decide on a chart. Open question: include staking rewards?\n")
	return b.String()
}

// oneTurn is a previous turn: user question, assistant tool call, tool result preview, assistant answer.
func oneTurn(r *rand.Rand, i int, day int) []openai.ChatCompletionMessageParamUnion {
	coin := coins[i%len(coins)]
	var rows strings.Builder
	rows.WriteString("date|coin|price_eur|source\n")
	base := 100.0 + r.Float64()*900
	if coin == "BTC" {
		base = 55000
	} else if coin == "ETH" {
		base = 2300
	}
	n := 40
	for k := 0; k < n; k++ {
		d := day - n + k
		p := base * (1 + (r.Float64()-0.5)*0.08)
		rows.WriteString(fmt.Sprintf("2026-%02d-%02d|%s|%.2f|daily_btc\n", 1+(d/28)%12, 1+d%28, coin, p))
	}
	callID := fmt.Sprintf("call_h%03d", i)
	sql := fmt.Sprintf("SELECT date, coin, price_eur, source FROM prices WHERE coin = '%s' ORDER BY date DESC LIMIT %d", coin, n)
	am := openai.ChatCompletionAssistantMessageParam{}
	am.ToolCalls = []openai.ChatCompletionMessageToolCallUnionParam{{OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
		ID: callID, Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{Name: "query", Arguments: fmt.Sprintf(`{"sql":%q}`, sql)}}}}
	final := openai.ChatCompletionAssistantMessageParam{}
	final.Content.OfString = openai.String(fmt.Sprintf("Here are the last %d %s prices. The range was about %.0f to %.0f EUR, with no gaps.", n, coin, base*0.96, base*1.04))
	return []openai.ChatCompletionMessageParamUnion{
		openai.UserMessage(fmt.Sprintf("Show me the last %d days of %s prices (turn %d).", n, coin, i+1)),
		{OfAssistant: &am},
		openai.ToolMessage(fmt.Sprintf("[query result — %d of %d rows]\n%s", n, n, rows.String()), callID),
		{OfAssistant: &final},
	}
}

// burrowRequest returns messages of about targetChars (JSON chars, tools included) and the
// character count. The last user message is the current turn.
func burrowRequest(seed int64, nonce string, targetChars int, current string) ([]openai.ChatCompletionMessageParamUnion, int) {
	r := rand.New(rand.NewSource(seed))
	sys := systemBlocks(nonce)
	msgs := []openai.ChatCompletionMessageParamUnion{openai.SystemMessage(sys)}
	chars := msgChars(msgs[0]) + toolJSON + msgChars(openai.UserMessage(current))
	for i := 0; chars < targetChars; i++ {
		t := oneTurn(r, i, 200+i*3)
		for _, m := range t {
			chars += msgChars(m)
		}
		msgs = append(msgs, t...)
	}
	msgs = append(msgs, openai.UserMessage(current))
	return msgs, chars
}

func msgChars(m openai.ChatCompletionMessageParamUnion) int {
	b, _ := m.MarshalJSON()
	return len(b)
}
