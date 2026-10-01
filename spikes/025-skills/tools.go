package main

// The agent tools (subset of SPEC 8.1/8.2): apply_migration, save_pipeline, save_view, save_form,
// describe_table, query, get_config. Write tools return the complete error list with paths.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
)

type toolSpec struct{ Name, Desc, Schema string }

var toolSpecs = []toolSpec{
	{"apply_migration", "Change the database schema with structured migration steps (never SQL DDL). Runs as one transaction: the steps, then every view and pipeline is re-validated against the new schema. Views and pipelines that must change because of the migration are sent in the same call as full YAML configs in `views` and `pipelines`; if any check fails, nothing is applied and all errors are returned.",
		`{"type":"object","properties":{"steps":{"type":"array","items":{"type":"object"},"description":"migration steps, each with op"},"pipelines":{"type":"array","items":{"type":"string"},"description":"full YAML of pipelines updated in the same change"},"views":{"type":"array","items":{"type":"string"},"description":"full YAML of views and forms updated in the same change"}},"required":["steps"]}`},
	{"save_pipeline", "Validate and save a pipeline config (full YAML). Returns all errors with paths, or success.",
		`{"type":"object","properties":{"yaml":{"type":"string","description":"the complete pipeline config as YAML"}},"required":["yaml"]}`},
	{"save_view", "Validate and save a table or chart view config (full YAML). Returns all errors with paths, or success.",
		`{"type":"object","properties":{"yaml":{"type":"string","description":"the complete view config as YAML"}},"required":["yaml"]}`},
	{"save_form", "Validate and save a form view config (full YAML, type: form). Returns all errors with paths, or success.",
		`{"type":"object","properties":{"yaml":{"type":"string","description":"the complete form config as YAML"}},"required":["yaml"]}`},
	{"describe_table", "Columns, types, keys, indexes and row count of a table.",
		`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`},
	{"query", "Run one read-only SQL SELECT on the project database; returns up to 200 rows and the total count.",
		`{"type":"object","properties":{"sql":{"type":"string"},"params":{"type":"object","description":"values for named parameters (:name)"}},"required":["sql"]}`},
	{"get_config", "Read the saved YAML of a view or pipeline by id.",
		`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`},
}

// SPIKE-025 additions: web_search (fake results; SPEC 8.1) for every condition, load_skill
// (SPEC 8.9) for conditions B and C.
var webSearchSpec = toolSpec{"web_search", "Search the web or the news; returns titles, URLs, dates and snippets.",
	`{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"integer"},"freshness":{"type":"string","enum":["day","week","month","any"]},"source":{"type":"string","enum":["web","news","wikipedia","hn"]}},"required":["query"]}`}

var loadSkillSpec = toolSpec{"load_skill", "Load a skill from the skill list; its instructions and examples arrive as the result.",
	`{"type":"object","properties":{"name":{"type":"string","description":"skill name from the skill list"}},"required":["name"]}`}

var (
	oaiTools   []openai.ChatCompletionToolUnionParam // builder tools, condition A (021's 7 tools + web_search)
	oaiToolsBC []openai.ChatCompletionToolUnionParam // condition B/C: + load_skill
	toolBytes  int
)

func buildTools(specs []toolSpec) ([]openai.ChatCompletionToolUnionParam, int) {
	var out []openai.ChatCompletionToolUnionParam
	n := 0
	for _, t := range specs {
		var m map[string]any
		if err := json.Unmarshal([]byte(t.Schema), &m); err != nil {
			panic(t.Name + ": " + err.Error())
		}
		n += len(t.Name) + len(t.Desc) + len(t.Schema)
		out = append(out, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name: t.Name, Description: openai.String(t.Desc), Parameters: shared.FunctionParameters(m)}))
	}
	return out, n
}

func initTools() {
	a := append(append([]toolSpec{}, toolSpecs...), webSearchSpec)
	oaiTools, toolBytes = buildTools(a)
	oaiToolsBC, _ = buildTools(append(a, loadSkillSpec))
	initMotherTools()
}

var writeTools = map[string]bool{"apply_migration": true, "save_pipeline": true, "save_view": true, "save_form": true}

// WriteCall is one call of a write tool, for the metrics.
type WriteCall struct {
	Tool     string
	Artifact string // config id or "migration"
	OK       bool
	Issues   []Issue
	Updated  []string // dependents sent with a migration
}

// callTool runs one tool call and returns its text result.
func (p *Project) callTool(name, args string) (string, *WriteCall) {
	var a map[string]any
	d := json.NewDecoder(strings.NewReader(args))
	d.UseNumber()
	if err := d.Decode(&a); err != nil {
		wc := &WriteCall{Tool: name, Artifact: "?"}
		if writeTools[name] {
			wc.Issues = []Issue{{Cat: CatSchema, Msg: "tool arguments are not valid JSON: " + err.Error()}}
		}
		return "error: tool arguments are not valid JSON: " + short(err.Error(), 200), map[bool]*WriteCall{true: wc, false: nil}[writeTools[name]]
	}
	a = normJSON(a).(map[string]any)
	switch name {
	case "describe_table":
		return p.describeTable(asStr(a["name"])), nil
	case "query":
		return p.runQuery(asStr(a["sql"]), asMap(a["params"])), nil
	case "get_config":
		c := p.Configs[asStr(a["id"])]
		if c == nil {
			return fmt.Sprintf("error: no view or pipeline %q (saved: %s)", asStr(a["id"]), strings.Join(p.configIDs(), ", ")), nil
		}
		return fmt.Sprintf("%s %s (revision %d):\n%s", c.Kind, c.ID, c.Rev, c.Src), nil
	case "save_pipeline":
		return p.saveConfig(name, "pipeline", yamlArg(a))
	case "save_view", "save_form":
		return p.saveConfig(name, "view", yamlArg(a))
	case "apply_migration":
		var pipes, views []string
		for _, x := range asList(a["pipelines"]) {
			pipes = append(pipes, cfgText(x))
		}
		for _, x := range asList(a["views"]) {
			views = append(views, cfgText(x))
		}
		steps := a["steps"]
		if s, ok := steps.(string); ok { // some models send the list as a JSON string
			var v any
			if json.Unmarshal([]byte(s), &v) == nil {
				steps = normJSON(v)
			}
		}
		r := p.applyMigration(steps, pipes, views)
		wc := &WriteCall{Tool: name, Artifact: "migration", OK: r.OK, Issues: r.Issues, Updated: r.Updated}
		if !r.OK {
			return invalidText(r.Issues, "apply_migration"), wc
		}
		var b strings.Builder
		fmt.Fprintf(&b, "applied: migration committed, schema version %d.\n", p.SchemaVersion)
		for _, s := range r.Summary {
			b.WriteString("- " + s + "\n")
		}
		if len(r.Destructive) > 0 {
			b.WriteString("destructive steps (approved by the user): " + strings.Join(r.Destructive, "; ") + "\n")
		}
		if len(r.Updated) > 0 {
			b.WriteString("dependents updated: " + strings.Join(r.Updated, ", ") + "\n")
		}
		b.WriteString(warnText(r.Issues))
		return b.String(), wc
	}
	return fmt.Sprintf("error: unknown tool %q", name), nil
}

// yamlArg reads the config argument; models sometimes send an object instead of YAML text.
func yamlArg(a map[string]any) string {
	for _, k := range []string{"yaml", "config", "content"} {
		if v, ok := a[k]; ok {
			return cfgText(v)
		}
	}
	return ""
}

func cfgText(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

// normJSON turns json.Number into int64 or float64.
func normJSON(v any) any {
	switch t := v.(type) {
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i
		}
		f, _ := t.Float64()
		return f
	case map[string]any:
		for k, x := range t {
			t[k] = normJSON(x)
		}
		return t
	case []any:
		for i, x := range t {
			t[i] = normJSON(x)
		}
		return t
	}
	return v
}

func (p *Project) saveConfig(tool, kind, src string) (string, *WriteCall) {
	wc := &WriteCall{Tool: tool, Artifact: "?"}
	d, err := loadYAML(src)
	if err != nil {
		wc.Issues = []Issue{loadIssue(err, "")}
		return invalidText(wc.Issues, tool), wc
	}
	v := asMap(d.Value)
	if v == nil {
		wc.Issues = []Issue{{Cat: CatSchema, Msg: "the config must be a YAML mapping"}}
		return invalidText(wc.Issues, tool), wc
	}
	id := asStr(v["id"])
	if id != "" {
		wc.Artifact = id
	}
	schema, err := loadSchema(p.R)
	if err != nil {
		return "error: " + err.Error(), nil
	}
	next := map[string]*Config{}
	for k, c := range p.Configs {
		next[k] = c
	}
	typ := asStr(v["type"])
	if kind == "view" && typ == "" && v["steps"] != nil {
		// a pipeline sent to save_view
		wc.Issues = []Issue{{Cat: CatStepType, Msg: "this looks like a pipeline; use save_pipeline"}}
		return invalidText(wc.Issues, tool), wc
	}
	c := &Config{ID: id, Kind: kind, Type: typ, Src: src, Val: v, Rev: revOf(p.Configs[id]) + 1}
	if id != "" {
		next[id] = c
	}
	is := &issues{doc: d}
	var hosts []string
	if kind == "pipeline" {
		hosts = checkPipeline(p.R, schema, next, d, is)
	} else {
		checkView(p.R, schema, next, p.Annot, d, is)
	}
	wc.Issues = is.list
	if is.errCount() > 0 {
		return invalidText(is.list, tool), wc
	}
	// dependents of this config must still validate (e.g. a form opened by a table)
	p.Configs[id] = c
	wc.OK = true
	var b strings.Builder
	what := kind
	if kind == "view" {
		what = typ + " view"
	}
	fmt.Fprintf(&b, "saved %s %s (revision %d).\n", what, id, c.Rev)
	if len(hosts) > 0 {
		fmt.Fprintf(&b, "hosts the user must approve before the first run: %s\n", strings.Join(hosts, ", "))
	}
	b.WriteString(warnText(is.list))
	return b.String(), wc
}

func invalidText(list []Issue, tool string) string {
	var errs []string
	for _, i := range list {
		if !i.Warn {
			errs = append(errs, i.String())
		}
	}
	sort.SliceStable(errs, func(a, b int) bool { return false })
	var b strings.Builder
	fmt.Fprintf(&b, "INVALID, nothing was saved. %d error(s); fix all of them and call %s again with the complete config:\n", len(errs), tool)
	for _, e := range errs {
		b.WriteString("- " + e + "\n")
	}
	b.WriteString(warnText(list))
	return b.String()
}

func warnText(list []Issue) string {
	var b strings.Builder
	for _, i := range list {
		if i.Warn {
			b.WriteString("- " + i.String() + "\n")
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return "warnings (do not block saving):\n" + b.String()
}
