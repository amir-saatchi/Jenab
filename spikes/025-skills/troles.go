package main

// T-roles: Mother requests that should create a role chat with create_chat(title, role,
// message, skills). Mother's context is the SPIKE-023 fixture ("Crypto watch" card and chat
// list), its base, Mother and prompt-rule texts, the delegation skill always loaded (SPEC 8.9),
// and the skill list. The run stops at the first create_chat, send_to_chat or subagent call, or
// at the final answer. Other tools get short canned results (no network, no database).

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
)

type RoleReq struct {
	ID         string
	Prompt     string
	Expected   []string
	Acceptable []string // superset of Expected
}

var roleReqs = []RoleReq{
	{ID: "R1_redesign_tables", Expected: []string{"database-design", "migrations"}, Acceptable: []string{"database-design", "migrations", "sql-queries", "pipelines", "config-guide"},
		Prompt: "Start a new chat that redesigns our tables so that prices and news work for many coins and many news sources."},
	{ID: "R2_dashboards", Expected: []string{"config-guide"}, Acceptable: []string{"config-guide", "sql-queries"},
		Prompt: "Start a new chat that builds dashboards for me: charts and tables of our data, whenever I ask for one."},
	{ID: "R3_gold_news_daily", Expected: []string{"pipelines", "web-research"}, Acceptable: []string{"pipelines", "web-research", "database-design", "migrations", "sql-queries"},
		Prompt: "Start a new chat that watches gold news every day and keeps a short log of what matters."},
	{ID: "R4_sql_questions", Expected: []string{"sql-queries"}, Acceptable: []string{"sql-queries", "config-guide"},
		Prompt: "Start a new chat that answers my questions about our data with SQL, e.g. monthly averages or the best and worst days."},
	{ID: "R5_pipeline_care", Expected: []string{"pipelines"}, Acceptable: []string{"pipelines", "sql-queries"},
		Prompt: "Start a new chat that looks after our pipelines: when one fails it finds out why and fixes it, and it changes schedules when I ask."},
	{ID: "R6_exchange_trades", Expected: []string{"database-design", "migrations", "pipelines", "config-guide"}, Acceptable: []string{"database-design", "migrations", "pipelines", "config-guide", "sql-queries"},
		Prompt: "Start a new chat for my exchange trades: it should design a table for them, fill it every night from the exchange's API, and show my trades in a table view."},
}

func roleReqByID(id string) *RoleReq {
	for i := range roleReqs {
		if roleReqs[i].ID == id || strings.HasPrefix(roleReqs[i].ID, id+"_") {
			return &roleReqs[i]
		}
	}
	return nil
}

type RoleRec struct {
	Route   string // create_chat, send_to_chat, subagent, answer, none
	Title   string `json:",omitempty"`
	RoleTxt string `json:",omitempty"`
	Message string `json:",omitempty"`
	Skills  []string
	Unknown []string `json:",omitempty"` // skill names not in the list
	Score   string   // exact, acceptable, wrong
	Missing []string `json:",omitempty"`
	Extra   []string `json:",omitempty"`
}

// Mother's tools: the SPIKE-023 main tools, the three Mother tools (create_chat gains `skills`,
// SPEC 8.6), and load_skill.
var motherSpecs = []toolSpec{
	{"query", "Read-only SQL SELECT on the project database (SQLite); returns up to 200 rows and the row count.",
		`{"type":"object","properties":{"sql":{"type":"string"}},"required":["sql"]}`},
	{"describe_table", "Columns, types, keys and row count of a table.",
		`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`},
	{"read_config", "The stored pipeline or view config as YAML.",
		`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`},
	{"run_pipeline", "Start a pipeline run. Returns the run ID right away; the run goes on in the background and a notice follows when it finishes.",
		`{"type":"object","properties":{"id":{"type":"string"},"inputs":{"type":"object","description":"input values of the pipeline"}},"required":["id"]}`},
	{"run_status", "Status and step summaries of a pipeline run.",
		`{"type":"object","properties":{"run_id":{"type":"string"}},"required":["run_id"]}`},
	{"web_search", "Search the web or the news; returns titles, URLs, dates and snippets.",
		`{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"integer"},"source":{"type":"string","enum":["web","news"]}},"required":["query"]}`},
	{"fetch_page", "Fetch a web page as readable text; returns a preview.",
		`{"type":"object","properties":{"url":{"type":"string"}},"required":["url"]}`},
	{"call_api", "One GET request through a saved connection; returns the response.",
		`{"type":"object","properties":{"connection":{"type":"string"},"path":{"type":"string","description":"path with query string, e.g. /simple/price?ids=bitcoin&vs_currencies=usd"}},"required":["connection","path"]}`},
	{"subagent", "Run a task in an isolated context with its own tools (web_search, fetch_page, query); returns only the result. Subagents cannot change memory or the schema. With background: true it returns a task ID right away and a notice follows when it finishes; otherwise the call returns when the task is done, which can take minutes.",
		`{"type":"object","properties":{"task":{"type":"string"},"inputs":{"type":"object"},"background":{"type":"boolean"}},"required":["task"]}`},
	{"task_status", "Status of a background task (subagent or chat).",
		`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`},
	{"list_chats", "IDs, titles, roles and status of the project's chats.",
		`{"type":"object","properties":{}}`},
	{"create_chat", "Mother chat only: create a chat with a title, a role (its lasting job), the skills it starts with, and a first message. Returns the chat ID and a task ID right away; a notice with the chat's reply follows when its turn ends.",
		`{"type":"object","properties":{"title":{"type":"string"},"role":{"type":"string"},"message":{"type":"string"},"skills":{"type":"array","items":{"type":"string"},"description":"names from the skill list that the new chat starts with; it can load more later"}},"required":["title","role","message","skills"]}`},
	{"send_to_chat", "Mother chat only: send a message to an existing chat. Returns a task ID right away; a notice with the chat's reply follows when its turn ends.",
		`{"type":"object","properties":{"chat_id":{"type":"string"},"message":{"type":"string"}},"required":["chat_id","message"]}`},
	loadSkillSpec,
}

var oaiMotherTools []openai.ChatCompletionToolUnionParam

func initMotherTools() { oaiMotherTools, _ = buildTools(motherSpecs) }

// motherSystem is block 1 (base, Mother text, the SPEC 8.3 prompt rule, the loaded delegation
// skill, the skill list) and block 4 (card, chat list).
func motherSystem() string {
	parts := []string{prompts["base"], prompts["mother"], prompts["rule"],
		"## Loaded skill: delegation\n" + skills["delegation"].Body,
		strings.TrimRight(skillList(map[string]bool{"delegation": true}), "\n"),
		prompts["card"], prompts["chats"]}
	return strings.Join(parts, "\n\n")
}

func anyStrList(v any) []string {
	var out []string
	switch t := v.(type) {
	case []any:
		for _, x := range t {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
	case string: // some models send a JSON string or a comma list
		var l []string
		if json.Unmarshal([]byte(t), &l) == nil {
			return l
		}
		for _, x := range strings.Split(t, ",") {
			if x = strings.TrimSpace(x); x != "" {
				out = append(out, x)
			}
		}
	}
	return out
}

func scoreRole(rec *RoleRec, rr *RoleReq) {
	if rec.Route != "create_chat" {
		rec.Score = "wrong"
		return
	}
	got := map[string]bool{}
	for _, s := range rec.Skills {
		s = strings.ToLower(strings.TrimSpace(s))
		if skills[s] == nil {
			rec.Unknown = append(rec.Unknown, s)
			continue
		}
		got[s] = true
	}
	exp := map[string]bool{}
	for _, s := range rr.Expected {
		exp[s] = true
		if !got[s] {
			rec.Missing = append(rec.Missing, s)
		}
	}
	acc := map[string]bool{}
	for _, s := range rr.Acceptable {
		acc[s] = true
	}
	for s := range got {
		if !exp[s] {
			rec.Extra = append(rec.Extra, s)
		}
	}
	sort.Strings(rec.Extra)
	outside := false
	for _, s := range rec.Extra {
		if !acc[s] {
			outside = true
		}
	}
	switch {
	case len(rec.Missing) == 0 && len(rec.Extra) == 0 && len(rec.Unknown) == 0:
		rec.Score = "exact"
	case len(rec.Missing) == 0 && !outside && len(rec.Unknown) == 0:
		rec.Score = "acceptable"
	default:
		rec.Score = "wrong"
	}
}

func runRole(ctx context.Context, m *Model, rr *RoleReq, rep int) *RunResult {
	res := newResult(m, rr.ID, "v2", rep, false)
	res.Test, res.Cond, res.Item, res.Rep = "troles", "M", rr.ID, rep
	rec := &RoleRec{Route: "none"}
	res.Role = rec
	a := &Agent{M: m, Cond: "B"}
	a.markLoaded("delegation", "role")
	res.Transcript = a.openTranscript(fmt.Sprintf("troles_%s_%s_r%d", m.ID, rr.ID, rep))
	defer a.closeTranscript()
	t0 := time.Now()
	sys := motherSystem()
	a.logf("===== %s %s rep %d\n----- system (%d chars)\n%s\n----- user\n%s\n", m.ID, rr.ID, rep, len(sys), sys, rr.Prompt)
	cur := []msg{{Role: "user", Text: rr.Prompt}}
	done := false
	for req := 0; req < probeMaxReq && !done; req++ {
		if overBudget() {
			res.Stop = "budget"
			break
		}
		msgs := []openai.ChatCompletionMessageParamUnion{openai.SystemMessage(sys)}
		for _, x := range cur {
			msgs = append(msgs, toOAI(x))
		}
		r := stream(ctx, m, Req{Label: fmt.Sprintf("%s/r%d/q%d", rr.ID, rep, req+1), Messages: msgs, Tools: oaiMotherTools, MaxTokens: maxOutTokens})
		res.Requests++
		res.Prompt += r.Prompt
		res.Output += r.Output
		res.ReqPrompts = append(res.ReqPrompts, r.Prompt)
		if r.Prompt > res.PeakPrompt {
			res.PeakPrompt = r.Prompt
		}
		spend(r.Prompt, r.Output)
		if r.Err != nil {
			res.Err = short(r.ErrText, 600)
			res.Stop = "error"
			if m.Stopped() != "" {
				res.Stop = "quota"
			}
			a.logf("----- error\n%s\n", res.Err)
			break
		}
		a.logf("----- assistant (finish %s, prompt %d, output %d, %.1fs)\n", r.Finish, r.Prompt, r.Output, r.Total.Seconds())
		if r.Text != "" {
			a.logf("%s\n", r.Text)
		}
		for _, c := range r.Calls {
			a.logf("> %s %s\n", c.Name, c.Args)
		}
		cur = append(cur, msg{Role: "assistant", Text: r.Text, Calls: r.Calls})
		if len(r.Calls) == 0 {
			res.Events = append(res.Events, SkEvent{Req: req + 1, Kind: "answer"})
			rec.Route, res.Stop = "answer", "answer"
			break
		}
		for _, c := range r.Calls {
			var args map[string]any
			json.Unmarshal([]byte(c.Args), &args)
			out := ""
			switch c.Name {
			case "create_chat", "send_to_chat", "subagent":
				res.Events = append(res.Events, SkEvent{Req: req + 1, Kind: "write", Name: c.Name})
				if !done {
					rec.Route = c.Name
					if c.Name == "create_chat" {
						rec.Title, _ = args["title"].(string)
						rec.RoleTxt, _ = args["role"].(string)
						rec.Message, _ = args["message"].(string)
						rec.Skills = anyStrList(args["skills"])
					}
				}
				done = true
				out = "started: task t_1 (a notice follows when it finishes)"
			case "load_skill":
				out = a.loadSkill(c.Args, req+1, res)
			case "list_chats":
				out = prompts["chats"]
				res.Events = append(res.Events, SkEvent{Req: req + 1, Kind: "read", Name: c.Name})
			case "web_search":
				out = fakeSearch(c.Args)
				res.Events = append(res.Events, SkEvent{Req: req + 1, Kind: "search", Name: c.Name})
			default:
				out = "(not available in this test; use the project card and the chat list)"
				res.Events = append(res.Events, SkEvent{Req: req + 1, Kind: "read", Name: c.Name})
			}
			a.logf("< %s\n%s\n", c.Name, out)
			cur = append(cur, msg{Role: "tool", Text: out, CallID: c.ID})
		}
		if done {
			res.Stop = "delegated"
		}
	}
	if res.Stop == "" {
		res.Stop = "request cap"
	}
	for _, n := range a.order {
		res.Loaded = append(res.Loaded, n+":"+a.loaded[n])
	}
	scoreRole(rec, rr)
	res.Seconds = time.Since(t0).Seconds()
	a.logf("----- result: route %s, skills %v, score %s (missing %v, extra %v, unknown %v)\n", rec.Route, rec.Skills, rec.Score, rec.Missing, rec.Extra, rec.Unknown)
	return res
}
