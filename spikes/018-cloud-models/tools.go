package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// The SPEC 8.1 main agent tool list (v1), written as JSON schemas for this spike.

type toolSpec struct {
	Name, Desc, Schema string
}

const cfgObj = `{"type":"object","description":"Config object as YAML-equivalent JSON"}`

var toolSpecs = []toolSpec{
	{"query", "Read-only SQL query on the project database. Returns up to 200 rows or 4,000 tokens, plus the total count.",
		`{"type":"object","properties":{"sql":{"type":"string","description":"One SELECT statement"},"params":{"type":"array","items":{},"description":"Positional parameters for ? placeholders"}},"required":["sql"],"additionalProperties":false}`},
	{"describe_table", "Columns, types, keys, annotations and row count of one table.",
		`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"],"additionalProperties":false}`},
	{"insert", "Insert a few rows by hand (at most 50 per call). Recurring or bulk data goes through pipelines.",
		`{"type":"object","properties":{"table":{"type":"string"},"rows":{"type":"array","minItems":1,"maxItems":50,"items":{"type":"object"},"description":"Each row is an object of column: value"}},"required":["table","rows"],"additionalProperties":false}`},
	{"request_schema_change", "Hand a schema change request (new table, new column) to the schema agent.",
		`{"type":"object","properties":{"goal":{"type":"string","description":"What the user wants, in plain words"}},"required":["goal"],"additionalProperties":false}`},
	{"save_view", "Validate and save a view config (table, chart or form). Returns all errors together.",
		`{"type":"object","properties":{"config":` + cfgObj + `},"required":["config"],"additionalProperties":false}`},
	{"save_pipeline", "Validate and save a pipeline config. Returns all errors together.",
		`{"type":"object","properties":{"config":` + cfgObj + `},"required":["config"],"additionalProperties":false}`},
	{"validate", "Validate a view or pipeline config without saving.",
		`{"type":"object","properties":{"config":` + cfgObj + `},"required":["config"],"additionalProperties":false}`},
	{"dry_run", "Dry run a pipeline: runs the steps without writing to tables.",
		`{"type":"object","properties":{"pipeline":{"type":"string","description":"Pipeline ID"},"inputs":{"type":"object"}},"required":["pipeline"],"additionalProperties":false}`},
	{"run_pipeline", "Start a pipeline run. Returns the run ID.",
		`{"type":"object","properties":{"id":{"type":"string","description":"Pipeline ID"},"inputs":{"type":"object"}},"required":["id"],"additionalProperties":false}`},
	{"run_status", "Status and step summaries of a pipeline run.",
		`{"type":"object","properties":{"run_id":{"type":"string"}},"required":["run_id"],"additionalProperties":false}`},
	{"update_memory", "Edit one section of user or project memory. Empty content deletes the section. Rejected if the revision changed.",
		`{"type":"object","properties":{"scope":{"type":"string","enum":["user","project"]},"section":{"type":"string"},"content":{"type":"string"},"expected_revision":{"type":"integer"}},"required":["scope","section","content","expected_revision"],"additionalProperties":false}`},
	{"update_session_notes", "Replace this chat's session notes. Rejected if the revision changed.",
		`{"type":"object","properties":{"content":{"type":"string"},"expected_revision":{"type":"integer"}},"required":["content","expected_revision"],"additionalProperties":false}`},
	{"search_history", "Full-text search over this chat or all chats in the project. Returns snippets with message IDs.",
		`{"type":"object","properties":{"query":{"type":"string"},"scope":{"type":"string","enum":["chat","project"]},"substring":{"type":"boolean","description":"Also find words inside words (slower)"}},"required":["query","scope"],"additionalProperties":false}`},
	{"read_messages", "Read specific messages of a chat by ID range.",
		`{"type":"object","properties":{"chat_id":{"type":"string"},"from":{"type":"string","description":"First message ID"},"to":{"type":"string","description":"Last message ID"}},"required":["chat_id","from","to"],"additionalProperties":false}`},
	{"read_ref", "Read a stored tool output or page by ref, from a token offset.",
		`{"type":"object","properties":{"ref":{"type":"string"},"offset":{"type":"integer","minimum":0}},"required":["ref"],"additionalProperties":false}`},
	{"search_ref", "Search inside a stored tool output or page.",
		`{"type":"object","properties":{"ref":{"type":"string"},"query":{"type":"string"}},"required":["ref","query"],"additionalProperties":false}`},
	{"web_search", "Search the web through the configured search provider.",
		`{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":20},"freshness":{"type":"string","enum":["any","day","week","month","year"]}},"required":["query"],"additionalProperties":false}`},
	{"fetch_page", "Fetch a web page as readable text. Returns a preview and a ref.",
		`{"type":"object","properties":{"url":{"type":"string","format":"uri"}},"required":["url"],"additionalProperties":false}`},
	{"bucket_list", "List bucket objects under a prefix.",
		`{"type":"object","properties":{"prefix":{"type":"string"}},"required":["prefix"],"additionalProperties":false}`},
	{"bucket_read", "Read a bucket object: text as preview and ref, images to vision-capable models.",
		`{"type":"object","properties":{"key":{"type":"string"}},"required":["key"],"additionalProperties":false}`},
	{"bucket_put", "Store content under a key, or keep a cached object (from_ref) under a permanent key. Give content or from_ref.",
		`{"type":"object","properties":{"key":{"type":"string"},"content":{"type":"string"},"from_ref":{"type":"string"}},"required":["key"],"additionalProperties":false}`},
	{"bucket_delete", "Delete a bucket key.",
		`{"type":"object","properties":{"key":{"type":"string"}},"required":["key"],"additionalProperties":false}`},
	{"save_link", "Save a link to the Resources panel.",
		`{"type":"object","properties":{"url":{"type":"string","format":"uri"},"title":{"type":"string"},"note":{"type":"string"},"tags":{"type":"array","items":{"type":"string"}}},"required":["url","title"],"additionalProperties":false}`},
	{"subagent", "Run a task in an isolated context; returns only the result. Subagents cannot change memory or the schema.",
		`{"type":"object","properties":{"task":{"type":"string"},"inputs":{"type":"object"}},"required":["task"],"additionalProperties":false}`},
}

var (
	oaiTools []openai.ChatCompletionToolUnionParam
	schemas  = map[string]*jsonschema.Schema{}
	toolJSON int // bytes of all tool definitions
)

func initTools() {
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	for _, t := range toolSpecs {
		var m map[string]any
		if err := json.Unmarshal([]byte(t.Schema), &m); err != nil {
			panic(t.Name + ": " + err.Error())
		}
		toolJSON += len(t.Name) + len(t.Desc) + len(t.Schema)
		oaiTools = append(oaiTools, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name: t.Name, Description: openai.String(t.Desc), Parameters: shared.FunctionParameters(m)}))
		doc, _ := jsonschema.UnmarshalJSON(strings.NewReader(t.Schema))
		url := "mem://" + t.Name + ".json"
		if err := c.AddResource(url, doc); err != nil {
			panic(err)
		}
		s, err := c.Compile(url)
		if err != nil {
			panic(t.Name + ": " + err.Error())
		}
		schemas[t.Name] = s
	}
}

// validArgs checks call arguments against the tool's schema.
func validArgs(c Call) error {
	s, ok := schemas[c.Name]
	if !ok {
		return fmt.Errorf("unknown tool %q", c.Name)
	}
	v, err := jsonschema.UnmarshalJSON(strings.NewReader(c.Args))
	if err != nil {
		return fmt.Errorf("not JSON")
	}
	if err := s.Validate(v); err != nil {
		return fmt.Errorf("schema: %s", short(err.Error(), 120))
	}
	return nil
}

func args(c Call) map[string]any {
	var m map[string]any
	json.Unmarshal([]byte(c.Args), &m)
	return m
}

func str(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return s
}

func has(s string, subs ...string) bool {
	s = strings.ToLower(s)
	for _, x := range subs {
		if !strings.Contains(s, strings.ToLower(x)) {
			return false
		}
	}
	return true
}

// ---- tool tasks ----

// step: what the model must do at this point. Tools empty = answer without tools.
type step struct {
	Tools  []string                 // expected tool names (any order); parallel if more than one
	Check  func(cs []Call) string   // "" = ok
	Result func(c Call) string      // fake tool result for the next step
	Answer func(text string) string // for the final no-tool step
}

type task struct {
	ID, Kind, Prompt string
	Parallel         bool
	Steps            []step
}

const toolSystem = `You are the agent inside Burrow, a desktop app for personal data projects.
Project: "Crypto tracker". Tables: prices (daily coin prices in EUR) and coins (coin list). Use describe_table to see columns.
Pipelines: daily_btc (cron 08:00). Views: btc_chart (chart).
Use a tool when the request needs data, an action or the web. Answer directly, without tools, when no tool is needed.
When the user asks for several independent things at once, call the tools in parallel in one response.
Keep answers to one or two sentences.`

func ansHas(subs ...string) func(string) string {
	return func(t string) string {
		for _, s := range subs {
			if has(t, s) {
				return ""
			}
		}
		return fmt.Sprintf("answer lacks %q", subs)
	}
}

func fixed(s string) func(Call) string { return func(Call) string { return s } }

var tasks = []task{
	{ID: "t1", Kind: "right tool", Prompt: "What columns does the prices table have?",
		Steps: []step{{Tools: []string{"describe_table"}, Check: func(cs []Call) string {
			if str(args(cs[0]), "name") != "prices" {
				return "name != prices"
			}
			return ""
		}}}},
	{ID: "t2", Kind: "right tool", Prompt: "How many rows in prices are for the coin BTC? Count them with SQL.",
		Steps: []step{{Tools: []string{"query"}, Check: func(cs []Call) string {
			if s := str(args(cs[0]), "sql"); !has(s, "count") || !has(s, "prices") {
				return "sql lacks count/prices"
			}
			return ""
		}}}},
	{ID: "t3", Kind: "right tool", Prompt: "Search the web for news about the Ethereum Fusaka upgrade from the past week. I want 5 results.",
		Steps: []step{{Tools: []string{"web_search"}, Check: func(cs []Call) string {
			a := args(cs[0])
			if !has(str(a, "query"), "fusaka") {
				return "query lacks fusaka"
			}
			if l, _ := a["limit"].(float64); l != 5 {
				return "limit != 5"
			}
			if str(a, "freshness") != "week" {
				return "freshness != week"
			}
			return ""
		}}}},
	{ID: "t4", Kind: "right tool", Prompt: "Remember for this project, in the preferences section, that prices are shown in EUR. The section's current revision is 7.",
		Steps: []step{{Tools: []string{"update_memory"}, Check: func(cs []Call) string {
			a := args(cs[0])
			if str(a, "scope") != "project" || str(a, "section") != "preferences" || !has(str(a, "content"), "eur") {
				return "wrong scope/section/content"
			}
			if r, _ := a["expected_revision"].(float64); r != 7 {
				return "expected_revision != 7"
			}
			return ""
		}}}},
	{ID: "t5", Kind: "no tool", Prompt: "What is 17 times 3? Just tell me.",
		Steps: []step{{Answer: ansHas("51")}}},
	{ID: "t6", Kind: "no tool", Prompt: "How do you say 'good morning' in German?",
		Steps: []step{{Answer: ansHas("guten morgen")}}},
	{ID: "t7", Kind: "parallel", Parallel: true, Prompt: "Describe both tables, prices and coins. Call describe_table for both at once, in parallel.",
		Steps: []step{{Tools: []string{"describe_table", "describe_table"}, Check: func(cs []Call) string {
			n := map[string]bool{str(args(cs[0]), "name"): true, str(args(cs[1]), "name"): true}
			if !n["prices"] || !n["coins"] {
				return "names != {prices, coins}"
			}
			return ""
		}}}},
	{ID: "t8", Kind: "parallel", Parallel: true, Prompt: "Two independent things, do both at once in parallel: list the bucket objects under reports/ and get the status of pipeline run run_81.",
		Steps: []step{{Tools: []string{"bucket_list", "run_status"}, Check: func(cs []Call) string {
			for _, c := range cs {
				a := args(c)
				if c.Name == "bucket_list" && !strings.HasPrefix(str(a, "prefix"), "reports") {
					return "prefix != reports/"
				}
				if c.Name == "run_status" && str(a, "run_id") != "run_81" {
					return "run_id != run_81"
				}
			}
			return ""
		}}}},
	{ID: "t9", Kind: "2-step", Prompt: "Start the daily_btc pipeline now, then check the run's status and tell me how it went.",
		Steps: []step{
			{Tools: []string{"run_pipeline"}, Check: func(cs []Call) string {
				if str(args(cs[0]), "id") != "daily_btc" {
					return "id != daily_btc"
				}
				return ""
			}, Result: fixed(`{"run_id":"run_93","status":"started"}`)},
			{Tools: []string{"run_status"}, Check: func(cs []Call) string {
				if str(args(cs[0]), "run_id") != "run_93" {
					return "run_id != run_93"
				}
				return ""
			}, Result: fixed(`{"run_id":"run_93","status":"success","steps":[{"id":"fetch","status":"ok"},{"id":"store","status":"ok","rows_inserted":6}]}`)},
			{Answer: ansHas("success", "succeeded", "successful", "6 rows", "six")},
		}},
	{ID: "t10", Kind: "3-step", Prompt: "First check the columns of the prices table, then add today's BTC price: 58,000 EUR on 2026-09-28.",
		Steps: []step{
			{Tools: []string{"describe_table"}, Check: func(cs []Call) string {
				if str(args(cs[0]), "name") != "prices" {
					return "name != prices"
				}
				return ""
			}, Result: fixed(`{"table":"prices","columns":[{"name":"date","type":"DATE"},{"name":"coin","type":"TEXT"},{"name":"price_eur","type":"REAL"}],"primary_key":["date","coin"],"rows":412}`)},
			{Tools: []string{"insert"}, Check: func(cs []Call) string {
				a := args(cs[0])
				rows, _ := a["rows"].([]any)
				if str(a, "table") != "prices" || len(rows) != 1 {
					return "table/rows wrong"
				}
				r, _ := rows[0].(map[string]any)
				if fmt.Sprint(r["date"]) != "2026-09-28" || !has(fmt.Sprint(r["coin"]), "btc") || fmt.Sprint(r["price_eur"]) != "58000" {
					return "row values wrong: " + short(cs[0].Args, 80)
				}
				return ""
			}, Result: fixed(`{"inserted":1}`)},
			{Answer: ansHas("added", "inserted", "58", "saved", "recorded", "done")},
		}},
	{ID: "t11", Kind: "3-step", Prompt: "Fetch https://example.com/btc-weekly and save it to my resources as a link tagged btc.",
		Steps: []step{
			{Tools: []string{"fetch_page"}, Check: func(cs []Call) string {
				if !has(str(args(cs[0]), "url"), "example.com/btc-weekly") {
					return "url wrong"
				}
				return ""
			}, Result: fixed(`[page "BTC weekly report" — example.com — 900 tokens, showing first 60 — ref: cache/pages/example.com/ab12f9.txt]
Bitcoin closed the week at 58,000 EUR, up 3 percent. Volume was flat.`)},
			{Tools: []string{"save_link"}, Check: func(cs []Call) string {
				a := args(cs[0])
				tags, _ := a["tags"].([]any)
				ok := false
				for _, t := range tags {
					if has(fmt.Sprint(t), "btc") {
						ok = true
					}
				}
				if !has(str(a, "url"), "example.com/btc-weekly") || !ok {
					return "url/tags wrong"
				}
				return ""
			}, Result: fixed(`{"saved":true,"id":"link_12"}`)},
			{Answer: ansHas("saved", "added", "link")},
		}},
}

var quickTasks = map[string]bool{"t1": true, "t5": true, "t9": true}
