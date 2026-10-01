package main

import (
	"fmt"
	"math/rand"
	"strings"

	"github.com/openai/openai-go/v3"
)

// A request shaped like Burrow's main agent request (SPEC 3.1): system prompt, user memory,
// project memory, project card, session notes, history turns with tool previews (3.7), current turn.
// Two facts are planted for the checks: a code word at the very start of the system prompt and a
// ticket number in the first user message of the history.

const codeWord = "PELICAN-47"
const ticketNo = "T-8812"

// estTok estimates Qwen 3.5 tokens: digits are one token each, other text about 4 characters per
// token; the factor 1.23 is calibrated against Ollama's prompt_tokens for this generator (it covers
// the chat template and JSON of tool calls).
func estTok(s string) int {
	d, o := 0, 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			d++
		} else {
			o++
		}
	}
	return int((float64(d) + float64(o)/4) * 1.23)
}

var systemRules = []string{
	"You are Burrow, a desktop assistant that keeps a user's project data in local SQLite tables, views and pipelines.",
	"Answer in the user's language. Keep answers short unless the user asks for detail.",
	"Use tools when they are needed and never invent table contents; read them with query or describe_table.",
	"Tool results larger than the preview limit are stored in the bucket; use read_ref or search_ref to read more.",
	"Memory holds intent: goals, preferences, decisions and conventions. Never store values that live in tables.",
	"Schema changes go through request_schema_change. Never drop tables or columns without approval.",
	"Recurring or bulk data goes through pipelines, not insert. insert is for small manual entries (at most 50 rows).",
	"Prices are stored in USD. Convert only for display, using the rate in the fx table.",
	"When a pipeline fails, read run_status first and explain the failing step in one sentence.",
	"Quote SQL only when the user asks for it. Prefer a table or a short list for results.",
	"If a question needs data you cannot reach with the tools, say so plainly.",
	"Web pages are fetched as readable text; cite the host when you use a page.",
	"Do not repeat the project card back to the user.",
}

var words = strings.Fields(`bitcoin price halving miner fee block chain exchange volume market order
wallet ledger report analyst supply demand rally drop support resistance trend week month quarter
investor fund inflow outflow custody spot futures option hash rate difficulty network node upgrade
policy rate inflation dollar euro index close open high low average daily weekly signal risk`)

type shape struct {
	Target int // wanted prompt tokens
	Nonce  string
}

// burrowRequest builds the messages for about sh.Target tokens.
func burrowRequest(sh shape, current string) []openai.ChatCompletionMessageParamUnion {
	rnd := rand.New(rand.NewSource(int64(sh.Target)))
	header := sh.Target * 35 / 100
	if header < 500 {
		header = 500
	}
	if header > 3200 {
		header = 3200
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Session code word: %s. Request id: %s.\n\n", codeWord, sh.Nonce)
	b.WriteString("# Rules\n")
	for _, r := range systemRules {
		b.WriteString("- " + r + "\n")
	}
	b.WriteString("\n# User memory\n## preferences\nShort answers. Dates as YYYY-MM-DD. Charts with a log scale for prices.\n## goals\nTrack crypto prices and my own trades; learn what moves the market.\n")
	if header > 900 {
		b.WriteString("\n# Project memory\n## decisions\nDaily closes come from the exchange API at 08:00 UTC (pipeline daily_btc). Chosen because the free plan allows one call per minute.\nA general prices(coin, day, …) table replaced btc_prices when Ethereum was added.\n## conventions\nTables for pipeline data have _run_id and _fetched_at. Bucket keys for reports: reports/<year>/<name>.pdf.\n")
	}
	b.WriteString("\n# Project card (as of 2026-09-28 08:05)\n## Tables\n")
	tables := []string{"prices", "trades", "fx", "news", "wallets", "fees", "miners", "etf_flows", "notes", "alerts", "reports", "sources", "funding", "onchain", "orders", "tags"}
	for i := 0; estTok(b.String()) < header-250; i++ {
		t := tables[i%len(tables)]
		if i >= len(tables) {
			t = fmt.Sprintf("%s_%d", t, i/len(tables))
		}
		fmt.Fprintf(&b, "- %s (%d rows): id INTEGER PK, day DATE [2021-01-01..2026-09-27], coin TEXT, value_usd REAL, source TEXT, note TEXT, _run_id TEXT, _fetched_at TEXT\n", t, 200+rnd.Intn(9000))
	}
	b.WriteString("## Views\n- v_prices: Daily closes (chart)\n- v_trades: My trades (table)\n## Pipelines\n- daily_btc: cron 0 8 * * *, last run success 2026-09-28 08:00\n## Bucket\n- reports/ (14 objects), images/ (32 objects); 9 saved links\n")
	b.WriteString("\n# Session notes\nTask: compare September 2026 with September 2025. Plan: query monthly closes, then fees, then news. Open: which fee source to trust.\n")
	msgs := []openai.ChatCompletionMessageParamUnion{sys(b.String())}
	used := estTok(b.String()) + estTok(current) + 20

	for turn := 0; used < sh.Target-150; turn++ {
		room := sh.Target - used - 200
		preview := 1500
		if room < preview+150 {
			preview = room - 150
		}
		if preview < 100 {
			break
		}
		q := fmt.Sprintf("Show me the %s for %s %d.", []string{"daily closes", "fee report", "news summary", "trade list"}[turn%4],
			[]string{"September", "August", "July", "June"}[turn%4], 2026-turn%2)
		if turn == 0 {
			q = "My ticket number is " + ticketNo + ", keep it for later. " + q
		}
		id := fmt.Sprintf("call_%d", turn+1)
		var tc call
		var body string
		if turn%2 == 0 {
			tc = call{ID: id, Name: "query", Args: fmt.Sprintf(`{"sql":"SELECT day, coin, value_usd, source FROM prices WHERE day >= '2026-%02d-01' ORDER BY day","params":[]}`, 9-turn%4)}
			body = tablePreview(rnd, preview, turn)
		} else {
			tc = call{ID: id, Name: "fetch_page", Args: fmt.Sprintf(`{"url":"https://news.example.com/markets/%d"}`, 1000+turn)}
			body = pagePreview(rnd, preview, turn)
		}
		ans := fmt.Sprintf("Here is the summary for turn %d: prices moved between %d and %d USD, with the largest daily change on day %d. The data is in the preview above; ask if you want a chart.", turn+1, 50000+rnd.Intn(5000), 60000+rnd.Intn(5000), 1+rnd.Intn(28))
		msgs = append(msgs, user(q), asstCalls("", []call{tc}), toolMsg(id, body), asst(ans))
		used += estTok(q) + estTok(tc.Args) + estTok(body) + estTok(ans) + 30
	}
	msgs = append(msgs, user(current))
	return msgs
}

func tablePreview(rnd *rand.Rand, tok, turn int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[query result — %d rows, %d tokens, showing first %d — ref: cache/tool/chat_7/msg_%d-1]\n| day | coin | value_usd | source |\n|---|---|---|---|\n", 400+turn*13, tok*4, tok, 30+turn)
	for d := 0; estTok(b.String()) < tok-30; d++ {
		fmt.Fprintf(&b, "| 2026-%02d-%02d | %s | %d.%02d | %s |\n", 1+(d/28+turn)%12, 1+d%28, []string{"BTC", "ETH"}[d%2], 30000+rnd.Intn(40000), rnd.Intn(100), []string{"binance", "kraken", "coinbase"}[d%3])
	}
	b.WriteString("[use read_ref(ref, offset) or search_ref(ref, query) for more]")
	return b.String()
}

func pagePreview(rnd *rand.Rand, tok, turn int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[page \"Market report %d\" — news.example.com — %d tokens, showing first %d — ref: cache/pages/news.example.com/%06x.txt]\n", turn, tok*5, tok, rnd.Intn(1<<24))
	for estTok(b.String()) < tok-30 {
		n := 12 + rnd.Intn(14)
		for i := 0; i < n; i++ {
			w := words[rnd.Intn(len(words))]
			if i == 0 {
				w = strings.ToUpper(w[:1]) + w[1:]
			}
			b.WriteString(w)
			if rnd.Intn(9) == 0 {
				fmt.Fprintf(&b, " %d", rnd.Intn(100000))
			}
			if i < n-1 {
				b.WriteString(" ")
			}
		}
		b.WriteString(". ")
		if rnd.Intn(5) == 0 {
			b.WriteString("\n\n")
		}
	}
	b.WriteString("\n[use read_ref(ref, offset) or search_ref(ref, query) for more]")
	return b.String()
}

const checkQuestion = "Two quick checks before we continue: what is the session code word from the system prompt, and what ticket number did I give in my first message? Answer in one line as: code word, ticket."

func checkAnswer(s string) (code, ticket bool) {
	return strings.Contains(s, codeWord), strings.Contains(s, ticketNo)
}

// bigSystem is one system message of about tok tokens with the code word only at its start.
func bigSystem(tok int) []openai.ChatCompletionMessageParamUnion {
	rnd := rand.New(rand.NewSource(99))
	var b strings.Builder
	fmt.Fprintf(&b, "The secret code word is %s. Remember it.\n\nReference material follows.\n\n", codeWord)
	b.WriteString(pagePreview(rnd, tok-60, 0))
	return []openai.ChatCompletionMessageParamUnion{sys(b.String()), user("What is the secret code word given at the very start of the system prompt? Reply with the code word only.")}
}
