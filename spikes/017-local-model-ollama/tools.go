package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// The main agent tool list of SPEC 8.1 as JSON schemas.

type toolDef struct {
	Name, Desc string
	Schema     map[string]any
	compiled   *jsonschema.Schema
}

const toolsJSON = `[
{"name":"query","desc":"Run a read-only SQL query on the project database. Returns up to 200 rows or 4,000 tokens, plus the total count.",
 "schema":{"type":"object","properties":{"sql":{"type":"string","description":"One SELECT statement. Use ? for parameters."},"params":{"type":"array","items":{},"description":"Values for the ? placeholders"}},"required":["sql"],"additionalProperties":false}},
{"name":"describe_table","desc":"Columns, types, keys, annotations and row count of one table.",
 "schema":{"type":"object","properties":{"name":{"type":"string","description":"Table name"}},"required":["name"],"additionalProperties":false}},
{"name":"insert","desc":"Insert a few manual rows into a table (at most 50 rows per call). Recurring or bulk data goes through pipelines.",
 "schema":{"type":"object","properties":{"table":{"type":"string"},"rows":{"type":"array","minItems":1,"maxItems":50,"items":{"type":"object","description":"Column name to value"}}},"required":["table","rows"],"additionalProperties":false}},
{"name":"request_schema_change","desc":"Hand a schema change request (new table, new column, rename, ...) to the schema agent. Describe the goal in plain language.",
 "schema":{"type":"object","properties":{"goal":{"type":"string"}},"required":["goal"],"additionalProperties":false}},
{"name":"save_view","desc":"Validate and save a view config (table, chart or form). Returns all validation errors together.",
 "schema":{"type":"object","properties":{"config":{"type":"object","description":"The full view config"}},"required":["config"],"additionalProperties":false}},
{"name":"save_pipeline","desc":"Validate and save a pipeline config. Returns all validation errors together.",
 "schema":{"type":"object","properties":{"config":{"type":"object","description":"The full pipeline config"}},"required":["config"],"additionalProperties":false}},
{"name":"validate","desc":"Validate a view or pipeline config without saving it.",
 "schema":{"type":"object","properties":{"config":{"type":"object"}},"required":["config"],"additionalProperties":false}},
{"name":"dry_run","desc":"Dry-run a pipeline: runs the steps without writing to tables.",
 "schema":{"type":"object","properties":{"pipeline":{"type":"string","description":"Pipeline ID"},"inputs":{"type":"object"}},"required":["pipeline"],"additionalProperties":false}},
{"name":"run_pipeline","desc":"Start a pipeline run now. Returns the run ID.",
 "schema":{"type":"object","properties":{"id":{"type":"string","description":"Pipeline ID"},"inputs":{"type":"object"}},"required":["id"],"additionalProperties":false}},
{"name":"run_status","desc":"Status and step summaries of a pipeline run.",
 "schema":{"type":"object","properties":{"run_id":{"type":"string"}},"required":["run_id"],"additionalProperties":false}},
{"name":"update_memory","desc":"Edit one section of user memory (all projects) or project memory (this project). Memory holds goals, preferences, decisions and conventions, never table values. Empty content deletes the section.",
 "schema":{"type":"object","properties":{"scope":{"type":"string","enum":["user","project"]},"section":{"type":"string","description":"e.g. goals, preferences, decisions, conventions"},"content":{"type":"string","description":"The full new content of the section"},"expected_revision":{"type":"integer","description":"Revision of the section as last read"}},"required":["scope","section","content","expected_revision"],"additionalProperties":false}},
{"name":"update_session_notes","desc":"Replace this chat's session notes (current task, plan, open questions).",
 "schema":{"type":"object","properties":{"content":{"type":"string"},"expected_revision":{"type":"integer"}},"required":["content","expected_revision"],"additionalProperties":false}},
{"name":"search_history","desc":"Full-text search over this chat or all chats in the project. Returns snippets with message IDs.",
 "schema":{"type":"object","properties":{"query":{"type":"string"},"scope":{"type":"string","enum":["chat","project"]},"substring":{"type":"boolean","description":"Also find words inside words (slower)"}},"required":["query","scope"],"additionalProperties":false}},
{"name":"read_messages","desc":"Read specific messages of a chat by message ID range.",
 "schema":{"type":"object","properties":{"chat_id":{"type":"string"},"from":{"type":"string","description":"First message ID"},"to":{"type":"string","description":"Last message ID"}},"required":["chat_id","from","to"],"additionalProperties":false}},
{"name":"read_ref","desc":"Read a stored tool output or page (a ref from a preview) starting at a token offset.",
 "schema":{"type":"object","properties":{"ref":{"type":"string"},"offset":{"type":"integer","minimum":0}},"required":["ref"],"additionalProperties":false}},
{"name":"search_ref","desc":"Search inside a stored tool output or page (a ref from a preview).",
 "schema":{"type":"object","properties":{"ref":{"type":"string"},"query":{"type":"string"}},"required":["ref","query"],"additionalProperties":false}},
{"name":"web_search","desc":"Search the web through the configured search provider.",
 "schema":{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":20},"freshness":{"type":"string","enum":["day","week","month","year","any"]}},"required":["query"],"additionalProperties":false}},
{"name":"fetch_page","desc":"Fetch a web page as readable text. Returns a preview and a ref.",
 "schema":{"type":"object","properties":{"url":{"type":"string"}},"required":["url"],"additionalProperties":false}},
{"name":"bucket_list","desc":"List objects in the project bucket under a prefix.",
 "schema":{"type":"object","properties":{"prefix":{"type":"string"}},"required":["prefix"],"additionalProperties":false}},
{"name":"bucket_read","desc":"Read an object from the bucket (text as preview and ref).",
 "schema":{"type":"object","properties":{"key":{"type":"string"}},"required":["key"],"additionalProperties":false}},
{"name":"bucket_put","desc":"Store content in the bucket under a key, or keep a cached object (from_ref) under a permanent key.",
 "schema":{"type":"object","properties":{"key":{"type":"string"},"content":{"type":"string"},"from_ref":{"type":"string"}},"required":["key"],"additionalProperties":false}},
{"name":"bucket_delete","desc":"Delete a key from the bucket.",
 "schema":{"type":"object","properties":{"key":{"type":"string"}},"required":["key"],"additionalProperties":false}},
{"name":"save_link","desc":"Save a link to the Resources panel.",
 "schema":{"type":"object","properties":{"url":{"type":"string"},"title":{"type":"string"},"note":{"type":"string"},"tags":{"type":"array","items":{"type":"string"}}},"required":["url","title"],"additionalProperties":false}},
{"name":"subagent","desc":"Run a task in an isolated context (for heavy reading); returns only the result. Subagents cannot change memory or the schema.",
 "schema":{"type":"object","properties":{"task":{"type":"string"},"inputs":{"type":"object"}},"required":["task"],"additionalProperties":false}}
]`

var tools []*toolDef
var toolByName = map[string]*toolDef{}

func loadTools() {
	var raw []struct {
		Name, Desc string
		Schema     map[string]any
	}
	must(json.Unmarshal([]byte(toolsJSON), &raw))
	for _, r := range raw {
		c := jsonschema.NewCompiler()
		must(c.AddResource(r.Name+".json", r.Schema))
		s, err := c.Compile(r.Name + ".json")
		must(err)
		t := &toolDef{Name: r.Name, Desc: r.Desc, Schema: r.Schema, compiled: s}
		tools = append(tools, t)
		toolByName[t.Name] = t
	}
}

func toolParams() []openai.ChatCompletionToolUnionParam {
	var out []openai.ChatCompletionToolUnionParam
	for _, t := range tools {
		out = append(out, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name: t.Name, Description: openai.String(t.Desc), Parameters: shared.FunctionParameters(t.Schema)}))
	}
	return out
}

// validArgs checks a tool call against its schema.
func validArgs(c call) (map[string]any, error) {
	t := toolByName[c.Name]
	if t == nil {
		return nil, fmt.Errorf("unknown tool %q", c.Name)
	}
	inst, err := jsonschema.UnmarshalJSON(strings.NewReader(c.Args))
	if err != nil {
		return nil, fmt.Errorf("args not JSON: %v", err)
	}
	if err := t.compiled.Validate(inst); err != nil {
		return nil, fmt.Errorf("schema: %s", oneLine(err.Error()))
	}
	m, _ := inst.(map[string]any)
	return m, nil
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 140 {
		s = s[:140] + "…"
	}
	return s
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
