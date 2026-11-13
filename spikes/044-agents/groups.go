package main

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// group is a tool group (the ticket's first draft, with save_page,
// save_pipeline and search_ref added where SPEC 8.1 has them).
type group struct {
	Name  string
	Title string // as the refusal names it
	Line  string // its one line in create_chat's schema
	Tools []string
}

// groups are the groups a chat can be given, in create_chat's order.
var groups = []group{
	{"read_data", "Read data", "read the project's tables", []string{"query", "describe_table"}},
	{"change_data", "Change data", "add rows and ask the schema agent for tables or columns", []string{"insert", "request_schema_change"}},
	{"views_pipelines", "Views and pipelines", "build, check and run views, pages and pipelines",
		[]string{"save_view", "save_page", "save_pipeline", "read_config", "open_page", "validate", "dry_run", "run_pipeline", "run_status"}},
	{"files", "Files", "the project's files (the bucket) and the linked folder",
		[]string{"bucket_list", "bucket_read", "bucket_put", "bucket_delete", "list_files", "read_file", "search_code"}},
	{"web", "Web", "search and read the web, feeds and APIs", []string{"web_search", "fetch_page", "read_feed", "save_link", "call_api", "connect_api"}},
	{"memory", "Memory", "edit user and project memory", []string{"update_memory"}},
}

// alwaysOn are the tools every chat has, whatever its groups. subagent is
// left out: the app has no subagent tool yet.
var alwaysOn = []string{"ask_user", "update_session_notes", "search_history", "read_messages", "read_ref", "search_ref", "list_chats", "load_skill", "task_status"}

// motherGroups are Mother's own groups in every condition; she also has
// create_chat and send_to_chat, and no group that writes data.
var (
	motherGroups = []string{"read_data", "files", "memory"}
	motherTools  = []string{"create_chat", "send_to_chat"}
)

func groupByName(name string) (group, bool) {
	i := slices.IndexFunc(groups, func(g group) bool { return g.Name == name })
	if i < 0 {
		return group{}, false
	}
	return groups[i], true
}

// groupOf is the group of a tool, or "" for an always-on or Mother tool.
func groupOf(tool string) string {
	for _, g := range groups {
		if slices.Contains(g.Tools, tool) {
			return g.Name
		}
	}
	return ""
}

// groupTools are the tools of the groups, in the groups' order.
func groupTools() []string {
	var out []string
	for _, g := range groups {
		out = append(out, g.Tools...)
	}
	return out
}

// toolsOf expands a chat's picks, group names or single tools, into the
// tools it has: the picks plus the always-on tools. Unknown names are
// dropped (create_chat's schema doesn't allow them).
func toolsOf(picks []string) []string {
	out := slices.Clone(alwaysOn)
	for _, p := range picks {
		if g, ok := groupByName(p); ok {
			out = append(out, g.Tools...)
		} else if groupOf(p) != "" {
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// groupsOf are the groups a chat's tools touch: a group counts when the
// chat has any tool of it (condition 3 picks single tools).
func groupsOf(tools []string) []string {
	var out []string
	for _, g := range groups {
		if slices.ContainsFunc(g.Tools, func(t string) bool { return slices.Contains(tools, t) }) {
			out = append(out, g.Name)
		}
	}
	return out
}

// motherToolList is Mother's tools.
func motherToolList() []string {
	return append(toolsOf(motherGroups), motherTools...)
}

// refusal is the error for a call to a tool outside a chat's tools. It
// names the group, and says what to do next as the prompt's "when no tool
// fits" line does.
func refusal(mother bool) func(string) string {
	return func(name string) string {
		g, ok := groupByName(groupOf(name))
		if !ok {
			return "there is no tool named " + name
		}
		next := "Tell the user what you can't do here, or ask them."
		if mother {
			next = "Hand the work to a chat that has it, with send_to_chat or create_chat."
		}
		return fmt.Sprintf("%s isn't one of this chat's tools: it is in the %s group (%s), which this chat doesn't have. A chat's tools are fixed when it starts. %s",
			name, g.Title, g.Line, next)
	}
}

// toolLines are one line per single tool, for condition 3's schema
// (SPEC 8.1's purposes, shortened).
var toolLines = map[string]string{
	"query":                 "read-only SQL on the project's tables",
	"describe_table":        "a table's columns, keys and row count",
	"insert":                "add up to 50 rows to a table",
	"request_schema_change": "ask the schema agent for new tables or columns",
	"save_view":             "validate and save a view",
	"save_page":             "validate and save a page of views",
	"save_pipeline":         "validate and save a pipeline",
	"read_config":           "a stored view, page or pipeline as YAML",
	"open_page":             "open a page in the chat",
	"validate":              "validate a config without saving it",
	"dry_run":               "dry-run a pipeline",
	"run_pipeline":          "start a pipeline run",
	"run_status":            "a run's status and steps",
	"bucket_list":           "list the project's files",
	"bucket_read":           "read a project file",
	"bucket_put":            "save a project file",
	"bucket_delete":         "delete a project file",
	"list_files":            "list files in the linked folder",
	"read_file":             "read a file in the linked folder",
	"search_code":           "search the linked folder",
	"web_search":            "search the web",
	"fetch_page":            "read a web page",
	"read_feed":             "read an RSS or Atom feed",
	"save_link":             "save a link to the Resources panel",
	"call_api":              "one GET through a saved API connection",
	"connect_api":           "propose an API connection",
	"update_memory":         "edit user or project memory",
}

// createChatSchema is create_chat's schema: tools as groups (condition 2,
// and condition 4 and the key's chats) or as single tools (condition 3).
func createChatSchema(single bool) json.RawMessage {
	var desc strings.Builder
	var enum []string
	always := "Every chat also has " + strings.Join(alwaysOn, ", ") + "."
	if single {
		desc.WriteString("The tools the chat can use, by name. It can't use any other tool, and its tools are fixed once it starts. " + always + "\nTools:")
		for _, t := range groupTools() {
			enum = append(enum, t)
			fmt.Fprintf(&desc, "\n- %s: %s", t, toolLines[t])
		}
	} else {
		desc.WriteString("The tool groups the chat can use. It can't use a tool outside them, and its groups are fixed once it starts. " + always + "\nGroups:")
		for _, g := range groups {
			enum = append(enum, g.Name)
			fmt.Fprintf(&desc, "\n- %s: %s (%s)", g.Name, g.Line, strings.Join(g.Tools, ", "))
		}
	}
	s := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title": map[string]any{"type": "string", "description": "The chat's title, unique in the project."},
			"role": map[string]any{"type": "string", "description": "The chat's custom prompt, at most 500 tokens, added after its system prompt. " +
				"Say what the chat is for, how it works and what it reports. The system prompt already has the general rules."},
			"message": map[string]any{"type": "string", "description": "The first message to the chat: what to do now, with the context it needs."},
			"tools":   map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": enum}, "description": desc.String()},
		},
		"required": []string{"title", "role", "message", "tools"},
	}
	b, _ := json.Marshal(s)
	return b
}
