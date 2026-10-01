package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
)

// Tool-calling tasks against the SPEC 8.1 tool list.

const toolSystem = `You are Burrow, a desktop assistant for a user's local project data (SQLite tables, views, pipelines, a file bucket).
Rules:
- Use a tool when the request needs project data, the web, memory, the bucket or a pipeline. Do not invent data.
- Answer directly without tools when no tool is needed (small talk, arithmetic, general knowledge).
- Memory holds intent (goals, preferences, decisions, conventions), never table values. User memory applies to all projects; project memory to this project only.
- Schema changes go through request_schema_change.
- When several independent calls are needed, make them in parallel in one response.
- Keep answers short.

# User memory (revision 3)
## preferences
Short answers. Dates as YYYY-MM-DD.

# Project memory (revision 5)
## decisions
Daily closes come from the exchange API at 08:00 UTC (pipeline daily_btc).

# Project card (as of 2026-09-28 08:05)
Tables:
- prices (2,410 rows): coin TEXT, day DATE [2021-01-01..2026-09-27], close_usd REAL, _run_id TEXT, _fetched_at TEXT; PK (coin, day)
- trades (58 rows): id INTEGER PK, day DATE, coin TEXT, side TEXT, amount REAL, price_usd REAL, note TEXT
Views: v_prices (chart), v_trades (table)
Pipelines: daily_btc (cron 0 8 * * *, last run success 2026-09-28 08:00)
Bucket: reports/ (14 objects), images/ (32 objects); 9 saved links
Chat: chat_7`

type check func(a map[string]any) string

type task struct {
	ID, Kind, Prompt string
	Want             []string         // tools that must be called, in order (multi-step) or in one response (parallel); empty = no tool
	Check            map[string]check // extra argument checks per tool
	Results          map[string]string
	Final            func(string) string // check on the final text (no-tool and multi-step tasks)
}

func str(a map[string]any, k string) string { s, _ := a[k].(string); return s }
func lc(s string) string                    { return strings.ToLower(s) }
func need(ok bool, msg string) string {
	if ok {
		return ""
	}
	return msg
}

var tasks = []task{
	{ID: "T01", Kind: "single", Prompt: "What columns does the trades table have?", Want: []string{"describe_table"},
		Check: map[string]check{"describe_table": func(a map[string]any) string { return need(str(a, "name") == "trades", "name != trades") }}},
	{ID: "T02", Kind: "single", Prompt: "How many ETH rows are in the prices table?", Want: []string{"query"},
		Check: map[string]check{"query": func(a map[string]any) string {
			s := lc(str(a, "sql"))
			p := lc(fmt.Sprint(a["params"]))
			return need(strings.Contains(s, "count") && strings.Contains(s, "prices") && (strings.Contains(s, "eth") || strings.Contains(p, "eth")), "sql lacks count/prices/eth")
		}}},
	{ID: "T03", Kind: "single", Prompt: "Add my trade: I bought 0.1 BTC on 2026-09-01 at 57,000 USD.", Want: []string{"insert"},
		Check: map[string]check{"insert": func(a map[string]any) string {
			rows, _ := a["rows"].([]any)
			r := lc(fmt.Sprint(rows))
			return need(str(a, "table") == "trades" && len(rows) == 1 && strings.Contains(r, "0.1") && strings.Contains(r, "57000") && strings.Contains(r, "2026-09-01"), "table/row values wrong")
		}}},
	{ID: "T04", Kind: "single", Prompt: "Search the web for news about Ethereum ETF flows from the past week.", Want: []string{"web_search"},
		Check: map[string]check{"web_search": func(a map[string]any) string {
			q := lc(str(a, "query"))
			return need((strings.Contains(q, "ethereum") || strings.Contains(q, "eth")) && str(a, "freshness") == "week", "query/freshness wrong")
		}}},
	{ID: "T05", Kind: "single", Prompt: "Please remember this for all my projects: I want prices shown in EUR.", Want: []string{"update_memory"},
		Check: map[string]check{"update_memory": func(a map[string]any) string {
			rev := fmt.Sprint(a["expected_revision"])
			return need(str(a, "scope") == "user" && strings.Contains(str(a, "content"), "EUR") && rev == "3", "scope/content/revision wrong")
		}}},
	{ID: "T06", Kind: "single", Prompt: "Run the daily_btc pipeline now.", Want: []string{"run_pipeline"},
		Check: map[string]check{"run_pipeline": func(a map[string]any) string { return need(str(a, "id") == "daily_btc", "id wrong") }}},
	{ID: "T07", Kind: "single", Prompt: "I also want to track the daily trading volume in the prices table. Add a column for it.", Want: []string{"request_schema_change"},
		Check: map[string]check{"request_schema_change": func(a map[string]any) string {
			return need(strings.Contains(lc(str(a, "goal")), "volume"), "goal lacks volume")
		}}},
	{ID: "T08", Kind: "single", Prompt: "The page preview stopped early. Read more of cache/pages/example.com/ab12f9.txt, starting at offset 1500.", Want: []string{"read_ref"},
		Check: map[string]check{"read_ref": func(a map[string]any) string {
			return need(str(a, "ref") == "cache/pages/example.com/ab12f9.txt" && fmt.Sprint(a["offset"]) == "1500", "ref/offset wrong")
		}}},
	{ID: "T09", Kind: "single", Prompt: "Did we talk about exchange fees in any other chat of this project?", Want: []string{"search_history"},
		Check: map[string]check{"search_history": func(a map[string]any) string {
			return need(str(a, "scope") == "project" && strings.Contains(lc(str(a, "query")), "fee"), "scope/query wrong")
		}}},
	{ID: "T10", Kind: "no tool", Prompt: "What is 17 times 23?", Final: func(s string) string { return need(strings.Contains(s, "391"), "answer lacks 391") }},
	{ID: "T11", Kind: "no tool", Prompt: "Thanks, that's all for today!", Final: func(s string) string { return need(strings.TrimSpace(s) != "", "empty answer") }},
	{ID: "T12", Kind: "multi-step", Prompt: "How many BTC trades did I make in September 2026? First check the columns of the trades table, then count.",
		Want: []string{"describe_table", "query"},
		Check: map[string]check{
			"describe_table": func(a map[string]any) string { return need(str(a, "name") == "trades", "name != trades") },
			"query": func(a map[string]any) string {
				s := lc(str(a, "sql"))
				return need(strings.Contains(s, "count") && strings.Contains(s, "trades"), "sql lacks count/trades")
			}},
		Results: map[string]string{
			"describe_table": "trades (58 rows): id INTEGER PK, day DATE, coin TEXT ('BTC' or 'ETH'), side TEXT ('buy' or 'sell'), amount REAL, price_usd REAL, note TEXT",
			"query":          "| count |\n|---|\n| 7 |\n(1 row)"},
		Final: func(s string) string { return need(strings.Contains(s, "7"), "answer lacks 7") }},
	{ID: "T13", Kind: "multi-step", Prompt: "Start the daily_btc pipeline and tell me whether the run succeeded.",
		Want: []string{"run_pipeline", "run_status"},
		Check: map[string]check{
			"run_pipeline": func(a map[string]any) string { return need(str(a, "id") == "daily_btc", "id wrong") },
			"run_status":   func(a map[string]any) string { return need(str(a, "run_id") == "run_981", "run_id wrong") }},
		Results: map[string]string{
			"run_pipeline": `{"run_id":"run_981","status":"started"}`,
			"run_status":   `{"run_id":"run_981","status":"failed","steps":[{"id":"fetch","status":"failed","error":"HTTP 429 Too Many Requests"},{"id":"store","status":"skipped"}]}`},
		Final: func(s string) string {
			l := lc(s)
			return need(strings.Contains(l, "fail") || strings.Contains(l, "429") || strings.Contains(l, "not succeed") || strings.Contains(l, "did not"), "answer does not say it failed")
		}},
	{ID: "T14", Kind: "parallel", Prompt: "Describe both the prices table and the trades table. Make both tool calls in parallel.",
		Want: []string{"describe_table", "describe_table"}},
	{ID: "T15", Kind: "parallel", Prompt: "List the bucket objects under reports/ and under images/, in parallel.",
		Want: []string{"bucket_list", "bucket_list"}},
}

type taskResult struct {
	Task                            task
	RightTool, ArgsValid, ArgsRight bool
	Pass                            bool
	Note                            string
	Requests                        int
	Time                            time.Duration
	FirstTTFT                       time.Duration
	PromptTok                       int
	ReasonTok                       int // completion tokens spent (thinking + answer)
	Skipped                         bool
}

func runTask(mdl string, t task, think thinkMode, maxTok int) taskResult {
	tr := taskResult{Task: t}
	msgs := []openai.ChatCompletionMessageParamUnion{sys(toolSystem), user(t.Prompt)}
	tp := toolParams()
	var called []string
	argsValid, argsRight := true, true
	var notes []string
	t0 := time.Now()
	maxReq := 1
	if t.Kind == "multi-step" {
		maxReq = len(t.Want) + 2
	}
	var final string
	for i := 0; i < maxReq; i++ {
		r := stream(req{Model: mdl, Msgs: msgs, Tools: tp, MaxTokens: maxTok, Think: think})
		tr.Requests++
		tr.ReasonTok += r.OutTok
		if i == 0 {
			tr.FirstTTFT, tr.PromptTok = r.TTFT, r.PromptTok
		}
		if r.Err != nil {
			notes = append(notes, "error: "+oneLine(r.Err.Error()))
			break
		}
		if r.Finish == "length" && len(r.Calls) == 0 {
			notes = append(notes, fmt.Sprintf("hit max tokens (%d)", maxTok))
		}
		if len(r.Calls) == 0 {
			final = r.Content
			break
		}
		var results []openai.ChatCompletionMessageParamUnion
		for _, c := range r.Calls {
			called = append(called, c.Name)
			a, err := validArgs(c)
			if err != nil {
				argsValid = false
				notes = append(notes, c.Name+": "+err.Error())
			} else if chk := t.Check[c.Name]; chk != nil {
				if p := chk(a); p != "" {
					argsRight = false
					notes = append(notes, c.Name+": "+p+" "+trunc(c.Args, 90))
				}
			}
			out := t.Results[c.Name]
			if out == "" {
				out = `{"ok":true}`
			}
			results = append(results, toolMsg(c.ID, out))
		}
		if i == 0 && t.Kind == "parallel" {
			notes = append(notes, fmt.Sprintf("%d call(s) in one response", len(r.Calls)))
		}
		msgs = append(msgs, asstCalls(r.Content, r.Calls))
		msgs = append(msgs, results...)
	}
	tr.Time = time.Since(t0)
	switch t.Kind {
	case "no tool":
		tr.RightTool = len(called) == 0
		if !tr.RightTool {
			notes = append(notes, "called "+strings.Join(called, ","))
		}
		if tr.RightTool && t.Final != nil {
			if p := t.Final(final); p != "" {
				notes = append(notes, p)
				argsRight = false
			}
		}
	case "parallel":
		tr.RightTool = len(called) == 2 && called[0] == t.Want[0] && called[1] == t.Want[1]
		if len(called) != 2 {
			notes = append(notes, "called ["+strings.Join(called, ",")+"]")
		}
	case "multi-step":
		tr.RightTool = subseq(t.Want, called)
		if !tr.RightTool {
			notes = append(notes, "called ["+strings.Join(called, ",")+"]")
		}
		if final == "" {
			notes = append(notes, "no final answer")
			argsRight = false
		} else if p := t.Final(final); p != "" {
			notes = append(notes, p+": "+trunc(final, 80))
			argsRight = false
		}
	default:
		tr.RightTool = len(called) >= 1 && called[0] == t.Want[0]
		if !tr.RightTool {
			if len(called) == 0 {
				notes = append(notes, "no tool; said "+trunc(final, 60))
			} else {
				notes = append(notes, "called "+strings.Join(called, ","))
			}
		}
	}
	if len(called) == 0 && t.Kind != "no tool" {
		argsValid, argsRight = false, false
	}
	tr.ArgsValid, tr.ArgsRight = argsValid, argsRight && argsValid
	tr.Pass = tr.RightTool && tr.ArgsValid && tr.ArgsRight
	tr.Note = strings.Join(notes, "; ")
	return tr
}

func subseq(want, got []string) bool {
	j := 0
	for _, g := range got {
		if j < len(want) && g == want[j] {
			j++
		}
	}
	return j == len(want)
}

func trunc(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) > n {
		return string([]rune(s)[:n]) + "…"
	}
	return s
}
