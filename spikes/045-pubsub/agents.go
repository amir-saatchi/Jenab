package main

import (
	"fmt"
	"slices"
)

// group is a tool group as SPEC 8.6 has it (SPIKE-044).
type group struct {
	Name  string
	Title string // as the refusal names it
	Line  string
	Tools []string
}

var groups = []group{
	{"read", "Read", "read the project's tables, views, pages, pipelines and runs", []string{"query", "describe_table", "read_config", "run_status", "open_page"}},
	{"change_data", "Change data", "add rows and ask the schema agent for tables or columns", []string{"insert", "request_schema_change"}},
	{"build_run", "Build and run", "build, check and run views, pages and pipelines", []string{"save_view", "save_page", "save_pipeline", "validate", "dry_run", "run_pipeline"}},
	{"files", "Files", "the project's files (the bucket) and the linked folder", []string{"bucket_list", "bucket_read", "bucket_put", "bucket_delete", "list_files", "read_file", "search_code"}},
	{"web", "Web", "search and read the web, feeds and APIs", []string{"web_search", "fetch_page", "read_feed", "save_link", "call_api", "connect_api"}},
	{"memory", "Memory", "edit user and project memory", []string{"update_memory"}},
}

// alwaysOn are SPEC 8.6's always-on tools, without subagent: the app has
// no subagent tool yet.
var alwaysOn = []string{"ask_user", "update_session_notes", "search_history", "read_messages", "read_ref", "search_ref", "list_chats", "load_skill", "task_status"}

// The spike's tools, by condition: subscribe and unsubscribe in condition
// 3, publish and read_report in 4 and 5.
var (
	subTools = []string{"subscribe", "unsubscribe"}
	pubTools = []string{"publish", "read_report"}
)

func groupOf(tool string) (group, bool) {
	for _, g := range groups {
		if slices.Contains(g.Tools, tool) {
			return g, true
		}
	}
	return group{}, false
}

// toolsOf are a chat's tools: its groups, the always-on tools and the
// condition's spike tools.
func toolsOf(gs []string, cond int) []string {
	out := slices.Clone(alwaysOn)
	for _, n := range gs {
		for _, g := range groups {
			if g.Name == n {
				out = append(out, g.Tools...)
			}
		}
	}
	switch {
	case cond == condAgent:
		out = append(out, subTools...)
	case part2(cond):
		out = append(out, pubTools...)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// refusal is SPEC 8.6's refusal for a tool outside the chat's groups.
func refusal(name string) string {
	g, ok := groupOf(name)
	if !ok {
		return "there is no tool named " + name
	}
	return fmt.Sprintf("%s isn't one of this chat's tools: it is in the %s group (%s), which this chat doesn't have. A chat's tools are fixed when it starts. Tell the user what you can't do here, or ask them.",
		name, g.Title, g.Line)
}

// sub is a subscription's event or channel and filter: what an answer
// key lists and what subscribe makes.
type sub struct {
	Event    string `json:"event"` // schema_changed, pipeline_run or a channel's name
	Table    string `json:"table,omitempty"`
	Pipeline string `json:"pipeline,omitempty"`
	Outcome  string `json:"outcome,omitempty"`
}

func (s sub) String() string {
	out := s.Event
	var fs []string
	if s.Table != "" {
		fs = append(fs, "table = "+s.Table)
	}
	if s.Pipeline != "" {
		fs = append(fs, "pipeline = "+s.Pipeline)
	}
	if s.Outcome != "" {
		fs = append(fs, "outcome = "+s.Outcome)
	}
	if len(fs) == 0 {
		if s.Event == evSchema || s.Event == evRun {
			return out + " (any)"
		}
		return out
	}
	for i, f := range fs {
		if i == 0 {
			out += " where " + f
		} else {
			out += " and " + f
		}
	}
	return out
}

// agentDef is one of the scenario's agents, set up the way SPEC 8.6 says:
// a title, a role (its custom prompt) and its tool groups. Its answer key
// says which subscriptions its job needs.
type agentDef struct {
	Key    string // short name in the script and the results
	Title  string
	Role   string
	Groups []string
	// Need are the subscriptions the job needs (conditions 2, 4 and 5
	// set them up; condition 3 scores against them). A made subscription
	// counts as one of them when its event matches; Right says whether
	// its filter is right too.
	Need []sub
	// Allowed are subscriptions that are reasonable but not needed: they
	// count as neither needed nor unneeded.
	Allowed []sub
	// Channels are the channels the agent subscribes to in Part 2.
	Channels []string
}

// The scenario's six agents. Their roles state the job, not the tools:
// the price watcher's says the other chats need a big move, not how to
// tell them.
var agents = []agentDef{
	{
		Key: "schema", Title: "Schema agent", Groups: []string{"read", "change_data"},
		Role: "You make the schema changes the user asks for in this project: new tables and new columns. Ask for each with request_schema_change, " +
			"with a clear goal, then check the table with describe_table. Reply with what changed: the table, the column and its type.",
	},
	{
		Key: "docs", Title: "Docs agent", Groups: []string{"read", "files"},
		Role: "You keep docs/schema.md in line with the project's tables. The file has one section per table, with each column, its type and a short line on what it holds. " +
			"Whenever a table or a column is added, changed or removed, the file must be updated to match. Reply with what you changed in the file.",
		Need: []sub{{Event: evSchema}},
	},
	{
		Key: "reviewer", Title: "Reviewer", Groups: []string{"read", "build_run"},
		Role: "You look into every pipeline run that fails in this project. For a failed run, read its status and the pipeline's config, find the cause and suggest a fix. " +
			"Don't change pipelines yourself. Reply with the run, the cause and the fix.",
		Need: []sub{{Event: evRun, Outcome: "failed"}},
	},
	{
		Key: "watcher", Title: "Price watcher", Groups: []string{"read", "web"},
		Role: "You watch the Bitcoin price. When asked, compare the newest price in btc_prices with the one the day before. " +
			"A move of more than 5% from one day to the next matters to other chats in this project, such as the weekly report. " +
			"Reply with the move in percent and whether it crossed 5%.",
		Allowed: []sub{{Event: evRun, Pipeline: "daily_btc"}, {Event: evRun, Pipeline: "daily_btc", Outcome: "success"}},
	},
	{
		Key: "reporter", Title: "Reporter", Groups: []string{"read", "files"},
		Role: "You write the weekly Bitcoin summary in summaries/draft.md: the week's prices, the big moves and the news that mattered, in short sections. " +
			"Update the draft when asked. Reply with what you added.",
		Allowed:  []sub{{Event: evRun, Pipeline: "weekly_summary"}, {Event: evRun, Pipeline: "weekly_summary", Outcome: "success"}},
		Channels: []string{"alerts"},
	},
	{
		Key: "entry", Title: "Data entry", Groups: []string{"read", "change_data"},
		Role: "You add the rows the user gives you to the project's tables: prices, news and keywords. Check the table's columns with describe_table first, " +
			"and don't make up values. Reply with the rows you added.",
	},
}

func agentByKey(k string) agentDef {
	i := slices.IndexFunc(agents, func(a agentDef) bool { return a.Key == k })
	return agents[i]
}

// keySubs are the subscriptions conditions 2, 4 and 5 set up for an
// agent, as if the user had made them in the Customize panel: the
// needed app events, and in Part 2 the channels.
func keySubs(a agentDef, cond int) []sub {
	if cond == condNone || cond == condAgent {
		return nil
	}
	out := slices.Clone(a.Need)
	if part2(cond) {
		for _, c := range a.Channels {
			out = append(out, sub{Event: c})
		}
	}
	return out
}

// matchesKey says whether a made subscription is one of the needed ones
// (by event), and whether its filter is right: the needed filter fields
// match, and it adds none that would miss needed events.
func matchesKey(s sub, need sub) (match, right bool) {
	if s.Event != need.Event {
		return false, false
	}
	return true, s.Table == need.Table && s.Pipeline == need.Pipeline && s.Outcome == need.Outcome
}

// allowedSub: one of the agent's reasonable but unneeded subscriptions.
func allowedSub(a agentDef, s sub) bool {
	return slices.Contains(a.Allowed, s)
}
