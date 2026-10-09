package main

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/amir-saatchi/jenab/internal/tool"
)

// world is a fake project that the work tools change: tables, rows,
// pipelines, views and project memory. The app doesn't have these tools
// yet, so the spike keeps their state in memory. The project card is
// built from it at each cut, as Phase 2 will (SPEC 3.2).
type world struct {
	mu        sync.Mutex
	tables    []*table
	pipelines []*pipeline
	views     []*view
	memory    []section
	writes    []string // the write tools called, in order
}

type column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type table struct {
	Name    string
	Columns []column
	Key     []string
	Rows    []map[string]any
}

type pipeline struct {
	ID          string
	Schedule    string
	Description string
	Steps       string
	LastRun     string
}

type view struct {
	ID, Title, Query string
}

type section struct {
	Name, Content string
}

// writeTools change the project. A turn that calls one wrote changes,
// which is what the app's check looks at (condition 4).
var writeTools = []string{"create_table", "insert_rows", "update_rows", "save_pipeline", "run_pipeline", "save_view", "update_memory"}

func (w *world) table(name string) *table {
	for _, t := range w.tables {
		if strings.EqualFold(t.Name, name) {
			return t
		}
	}
	return nil
}

func (w *world) pipeline(id string) *pipeline {
	for _, p := range w.pipelines {
		if strings.EqualFold(p.ID, id) {
			return p
		}
	}
	return nil
}

func (w *world) wrote(name string) {
	w.writes = append(w.writes, name)
}

// card is the project card: tables, pipelines, views and project memory.
func (w *world) card() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	var b strings.Builder
	b.WriteString("Project card (as of the last cut)\n\nTables:")
	if len(w.tables) == 0 {
		b.WriteString(" none yet")
	}
	for _, t := range w.tables {
		cols := make([]string, len(t.Columns))
		for i, c := range t.Columns {
			cols[i] = c.Name + " " + c.Type
		}
		fmt.Fprintf(&b, "\n- %s (%s), key (%s), %d rows", t.Name, strings.Join(cols, ", "), strings.Join(t.Key, ", "), len(t.Rows))
	}
	b.WriteString("\n\nPipelines:")
	if len(w.pipelines) == 0 {
		b.WriteString(" none yet")
	}
	for _, p := range w.pipelines {
		run := p.LastRun
		if run == "" {
			run = "never run"
		}
		fmt.Fprintf(&b, "\n- %s, schedule %q, last run: %s", p.ID, p.Schedule, run)
	}
	b.WriteString("\n\nViews:")
	if len(w.views) == 0 {
		b.WriteString(" none yet")
	}
	for _, v := range w.views {
		fmt.Fprintf(&b, "\n- %s %q", v.ID, v.Title)
	}
	b.WriteString("\n\nProject memory:")
	if len(w.memory) == 0 {
		b.WriteString(" empty")
	}
	for _, s := range w.memory {
		fmt.Fprintf(&b, "\n[%s]\n%s", s.Name, s.Content)
	}
	return b.String()
}

// tools are the fake work tools. Their effects are ReadsDB only, so no
// approval card stops a run; approvals are not what this spike tests.
func (w *world) tools() []tool.Tool {
	type createArgs struct {
		Name       string   `json:"name"`
		Columns    []column `json:"columns"`
		PrimaryKey []string `json:"primary_key"`
	}
	type insertArgs struct {
		Table string           `json:"table"`
		Rows  []map[string]any `json:"rows"`
	}
	type updateArgs struct {
		Table string         `json:"table"`
		Where string         `json:"where"`
		Set   map[string]any `json:"set"`
	}
	type nameArgs struct {
		Name string `json:"name"`
	}
	type pipelineArgs struct {
		ID          string `json:"id"`
		Schedule    string `json:"schedule"`
		Description string `json:"description"`
		Steps       string `json:"steps"`
	}
	type idArgs struct {
		ID string `json:"id"`
	}
	type viewArgs struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Query string `json:"query"`
	}
	type memoryArgs struct {
		Section string `json:"section"`
		Content string `json:"content"`
	}
	type searchArgs struct {
		Query string `json:"query"`
	}
	return []tool.Tool{
		tool.Func(spec("create_table", "Create a table in the project's database.", `{
			"type":"object","required":["name","columns","primary_key"],"additionalProperties":false,
			"properties":{
				"name":{"type":"string"},
				"columns":{"type":"array","minItems":1,"items":{"type":"object","required":["name","type"],"additionalProperties":false,
					"properties":{"name":{"type":"string"},"type":{"type":"string","enum":["TEXT","INTEGER","REAL","DATE"]}}}},
				"primary_key":{"type":"array","minItems":1,"items":{"type":"string"}}}}`),
			func(_ context.Context, _ *tool.Env, a createArgs) (tool.Result, error) {
				w.mu.Lock()
				defer w.mu.Unlock()
				if w.table(a.Name) != nil {
					return tool.Result{}, tool.Errorf("table %s already exists", a.Name)
				}
				for _, k := range a.PrimaryKey {
					if !slices.ContainsFunc(a.Columns, func(c column) bool { return strings.EqualFold(c.Name, k) }) {
						return tool.Result{}, tool.Errorf("primary key column %s is not a column", k)
					}
				}
				w.tables = append(w.tables, &table{Name: a.Name, Columns: a.Columns, Key: a.PrimaryKey})
				w.wrote("create_table")
				return tool.Result{Text: fmt.Sprintf("table %s created", a.Name)}, nil
			}),
		tool.Func(spec("insert_rows", "Insert rows into a table. Each row is an object of column values.", `{
			"type":"object","required":["table","rows"],"additionalProperties":false,
			"properties":{"table":{"type":"string"},"rows":{"type":"array","minItems":1,"items":{"type":"object"}}}}`),
			func(_ context.Context, _ *tool.Env, a insertArgs) (tool.Result, error) {
				w.mu.Lock()
				defer w.mu.Unlock()
				t := w.table(a.Table)
				if t == nil {
					return tool.Result{}, tool.Errorf("no table %s", a.Table)
				}
				t.Rows = append(t.Rows, a.Rows...)
				w.wrote("insert_rows")
				return tool.Result{Text: fmt.Sprintf("%d rows inserted into %s", len(a.Rows), t.Name)}, nil
			}),
		tool.Func(spec("update_rows", "Change rows of a table: where is an SQL condition, set the new column values.", `{
			"type":"object","required":["table","where","set"],"additionalProperties":false,
			"properties":{"table":{"type":"string"},"where":{"type":"string"},"set":{"type":"object"}}}`),
			func(_ context.Context, _ *tool.Env, a updateArgs) (tool.Result, error) {
				w.mu.Lock()
				defer w.mu.Unlock()
				if w.table(a.Table) == nil {
					return tool.Result{}, tool.Errorf("no table %s", a.Table)
				}
				w.wrote("update_rows")
				return tool.Result{Text: fmt.Sprintf("1 row updated in %s", a.Table)}, nil
			}),
		tool.Func(spec("describe_table", "A table's columns, key and row count.", `{
			"type":"object","required":["name"],"additionalProperties":false,"properties":{"name":{"type":"string"}}}`),
			func(_ context.Context, _ *tool.Env, a nameArgs) (tool.Result, error) {
				w.mu.Lock()
				defer w.mu.Unlock()
				t := w.table(a.Name)
				if t == nil {
					return tool.Result{}, tool.Errorf("no table %s", a.Name)
				}
				b, _ := json.Marshal(map[string]any{"name": t.Name, "columns": t.Columns, "primary_key": t.Key, "rows": len(t.Rows)})
				return tool.Result{Text: string(b)}, nil
			}),
		tool.Func(spec("save_pipeline", "Create or replace a pipeline. schedule is a cron expression; steps say what it fetches, how it transforms the data and where it writes or sends it.", `{
			"type":"object","required":["id","schedule","description","steps"],"additionalProperties":false,
			"properties":{"id":{"type":"string"},"schedule":{"type":"string"},"description":{"type":"string"},"steps":{"type":"string"}}}`),
			func(_ context.Context, _ *tool.Env, a pipelineArgs) (tool.Result, error) {
				w.mu.Lock()
				defer w.mu.Unlock()
				verb := "replaced"
				p := w.pipeline(a.ID)
				if p == nil {
					p, verb = &pipeline{ID: a.ID}, "created"
					w.pipelines = append(w.pipelines, p)
				}
				p.Schedule, p.Description, p.Steps = a.Schedule, a.Description, a.Steps
				w.wrote("save_pipeline")
				return tool.Result{Text: fmt.Sprintf("pipeline %s %s, schedule %q", p.ID, verb, p.Schedule)}, nil
			}),
		tool.Func(spec("run_pipeline", "Run a pipeline once now.", `{
			"type":"object","required":["id"],"additionalProperties":false,"properties":{"id":{"type":"string"}}}`),
			func(_ context.Context, _ *tool.Env, a idArgs) (tool.Result, error) {
				w.mu.Lock()
				defer w.mu.Unlock()
				p := w.pipeline(a.ID)
				if p == nil {
					return tool.Result{}, tool.Errorf("no pipeline %s", a.ID)
				}
				p.LastRun = "success, 1 row written"
				w.wrote("run_pipeline")
				return tool.Result{Text: fmt.Sprintf("run of %s: success, 1 row written", p.ID)}, nil
			}),
		tool.Func(spec("get_config", "Read a saved pipeline or view by its ID.", `{
			"type":"object","required":["id"],"additionalProperties":false,"properties":{"id":{"type":"string"}}}`),
			func(_ context.Context, _ *tool.Env, a idArgs) (tool.Result, error) {
				w.mu.Lock()
				defer w.mu.Unlock()
				if p := w.pipeline(a.ID); p != nil {
					b, _ := json.Marshal(map[string]string{"kind": "pipeline", "id": p.ID, "schedule": p.Schedule, "description": p.Description, "steps": p.Steps})
					return tool.Result{Text: string(b)}, nil
				}
				for _, v := range w.views {
					if strings.EqualFold(v.ID, a.ID) {
						b, _ := json.Marshal(map[string]string{"kind": "view", "id": v.ID, "title": v.Title, "query": v.Query})
						return tool.Result{Text: string(b)}, nil
					}
				}
				return tool.Result{}, tool.Errorf("no pipeline or view %s", a.ID)
			}),
		tool.Func(spec("save_view", "Create or replace a view: a chart or table on a page, from a SELECT query.", `{
			"type":"object","required":["id","title","query"],"additionalProperties":false,
			"properties":{"id":{"type":"string"},"title":{"type":"string"},"query":{"type":"string"}}}`),
			func(_ context.Context, _ *tool.Env, a viewArgs) (tool.Result, error) {
				w.mu.Lock()
				defer w.mu.Unlock()
				i := slices.IndexFunc(w.views, func(v *view) bool { return strings.EqualFold(v.ID, a.ID) })
				if i < 0 {
					w.views = append(w.views, &view{})
					i = len(w.views) - 1
				}
				*w.views[i] = view{a.ID, a.Title, a.Query}
				w.wrote("save_view")
				return tool.Result{Text: fmt.Sprintf("view %s saved", a.ID)}, nil
			}),
		tool.Func(spec("update_memory", "Replace a section of the project memory, which every chat of the project sees. Memory holds intent: goals, preferences, decisions with their reasons, conventions. Empty content deletes the section.", `{
			"type":"object","required":["section","content"],"additionalProperties":false,
			"properties":{"section":{"type":"string"},"content":{"type":"string"}}}`),
			func(_ context.Context, _ *tool.Env, a memoryArgs) (tool.Result, error) {
				w.mu.Lock()
				defer w.mu.Unlock()
				i := slices.IndexFunc(w.memory, func(s section) bool { return strings.EqualFold(s.Name, a.Section) })
				switch {
				case strings.TrimSpace(a.Content) == "" && i >= 0:
					w.memory = slices.Delete(w.memory, i, i+1)
				case i >= 0:
					w.memory[i].Content = a.Content
				default:
					w.memory = append(w.memory, section{a.Section, a.Content})
				}
				w.wrote("update_memory")
				return tool.Result{Text: fmt.Sprintf("memory section %s saved", a.Section)}, nil
			}),
		tool.Func(spec("web_search", "Search the web.", `{
			"type":"object","required":["query"],"additionalProperties":false,"properties":{"query":{"type":"string"}}}`),
			func(_ context.Context, _ *tool.Env, a searchArgs) (tool.Result, error) {
				return tool.Result{Text: searchResults(a.Query)}, nil
			}),
	}
}

func spec(name, desc, schema string) tool.Spec {
	return tool.Spec{Name: name, Description: desc, Schema: json.RawMessage(schema), Effects: tool.ReadsDB}
}

// searchResults are canned results, so plain questions that a model
// looks up stay cheap and the same in every run.
func searchResults(q string) string {
	q = strings.ToLower(q)
	switch {
	case strings.Contains(q, "coingecko"):
		return "1. CoinGecko API pricing: the Demo plan is free with 30 calls a minute and 10,000 a month; paid plans start at $129 a month. https://www.coingecko.com/en/api/pricing"
	case strings.Contains(q, "salary"):
		return "1. Data engineer salaries in Berlin, 2026: median €68,000, range €55,000–€85,000. https://example.org/salaries/berlin"
	}
	return "No results."
}
