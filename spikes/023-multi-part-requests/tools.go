package main

// Tool definitions (a subset of SPEC 8.1) sent to the models. The same definitions go to every
// model and to both conditions; Mother gets three more tools (SPEC 8.6).

import (
	"encoding/json"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
)

type toolSpec struct{ Name, Desc, Schema string }

var mainTools = []toolSpec{
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
}

var motherTools = []toolSpec{
	{"list_chats", "IDs, titles, roles and status of the project's chats.",
		`{"type":"object","properties":{}}`},
	{"create_chat", "Mother chat only: create a chat with a title, a role (its lasting job) and a first message. Returns the chat ID and a task ID right away; a notice with the chat's reply follows when its turn ends.",
		`{"type":"object","properties":{"title":{"type":"string"},"role":{"type":"string"},"message":{"type":"string"}},"required":["title","role","message"]}`},
	{"send_to_chat", "Mother chat only: send a message to an existing chat. Returns a task ID right away; a notice with the chat's reply follows when its turn ends.",
		`{"type":"object","properties":{"chat_id":{"type":"string"},"message":{"type":"string"}},"required":["chat_id","message"]}`},
}

var toolSets = map[string][]openai.ChatCompletionToolUnionParam{}

func buildTools(specs []toolSpec) []openai.ChatCompletionToolUnionParam {
	var out []openai.ChatCompletionToolUnionParam
	for _, t := range specs {
		var m map[string]any
		if err := json.Unmarshal([]byte(t.Schema), &m); err != nil {
			panic(t.Name + ": " + err.Error())
		}
		out = append(out, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name: t.Name, Description: openai.String(t.Desc), Parameters: shared.FunctionParameters(m)}))
	}
	return out
}

func initTools() {
	toolSets["main"] = buildTools(mainTools)
	toolSets["mother"] = buildTools(append(append([]toolSpec{}, mainTools...), motherTools...))
}

// background tools (SPEC 8.3)
func isBgCall(name string, args map[string]any) bool {
	switch name {
	case "run_pipeline", "create_chat", "send_to_chat":
		return true
	case "subagent":
		b, _ := args["background"].(bool)
		if s, ok := args["background"].(string); ok && s == "true" {
			b = true
		}
		return b
	}
	return false
}

var delegationTools = map[string]bool{"send_to_chat": true, "create_chat": true, "subagent": true}
