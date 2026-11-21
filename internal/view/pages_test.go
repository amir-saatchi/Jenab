package view

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// page is a page with the from_date filter and rows.
func page(id, rows string) string {
	return `version: 1
id: ` + id + `
title: Page
filters:
  - { param: from_date, control: date, default: today-90d }
rows:
  - [ { view: btc_price_table } ]
` + rows
}

func TestPageChecks(t *testing.T) {
	for _, c := range []tc{
		{name: "view exists", page: true, src: page("g1", "  - [ { view: btc_today } ]\n"), bad: page("g1", "  - [ { view: btc_todya } ]\n"),
			path: "/rows/1/0/view", parts: []string{"no view btc_todya", "did you mean btc_today?"}},
		{name: "view is a page", page: true, src: page("g1", ""), bad: page("g1", "  - [ { view: bitcoin } ]\n"),
			path: "/rows/1/0/view", parts: []string{"no view bitcoin; bitcoin is a page"}},
		{name: "blocks a row", page: true, src: page("g2", "  - [ { heading: A }, { heading: B }, { heading: C }, { heading: D } ]\n"),
			bad: page("g2", "  - [ { heading: A }, { heading: B }, { heading: C }, { heading: D }, { heading: E } ]\n"), path: "/rows/1", parts: []string{"has at most 4 items, not 5"}},
		{name: "empty row", page: true, src: page("g2", ""), bad: page("g2", "  - []\n"), path: "/rows/1", parts: []string{"needs at least 1 item"}},
		{name: "spans", page: true, src: page("g3", "  - [ { heading: A, span: 6 }, { heading: B } ]\n"),
			bad: page("g3", "  - [ { heading: A, span: 8 }, { heading: B, span: 6 } ]\n"), path: "/rows/1", parts: []string{"add up to 14"}},
		{name: "default spans", page: true, src: page("g3", "  - [ { heading: A, span: 8 }, { heading: B } ]\n"),
			bad: page("g3", "  - [ { heading: A, span: 11 }, { heading: B }, { heading: C } ]\n"), path: "/rows/1",
			parts: []string{"add up to 11", "leaves 1 of the 12 columns for 2 blocks without span"}},
		{name: "no room for a default span", page: true, src: page("g3", "  - [ { heading: A, span: 11 }, { heading: B } ]\n"),
			bad: page("g3", "  - [ { heading: A, span: 12 }, { heading: B } ]\n"), path: "/rows/1", parts: []string{"leaves 0 of the 12 columns for 1 block without span"}},
		{name: "float span", page: true, src: page("g3", "  - [ { heading: A, span: 6.0 }, { heading: B, span: 6.0 } ]\n"),
			bad: page("g3", "  - [ { heading: A, span: 8.0 }, { heading: B, span: 6.0 } ]\n"), path: "/rows/1", parts: []string{"add up to 14"}},
		{name: "span range", page: true, src: page("g3", "  - [ { heading: A, span: 12 } ]\n"), bad: page("g3", "  - [ { heading: A, span: 13 } ]\n"),
			path: "/rows/1/0/span", parts: []string{"13 is too large; the most is 12"}},
		{name: "spans in a card", page: true, src: page("g4", "  - [ { card: K, rows: [ [ { heading: A, span: 6 }, { heading: B, span: 6 } ] ] } ]\n"),
			bad: page("g4", "  - [ { card: K, rows: [ [ { heading: A, span: 6 }, { heading: B, span: 7 } ] ] } ]\n"), path: "/rows/1/0/rows/0", parts: []string{"add up to 13"}},
		{name: "card in a card", page: true, src: page("g5", "  - [ { card: K, rows: [ [ { heading: A } ] ] } ]\n"),
			bad: page("g5", "  - [ { card: K, rows: [ [ { card: L, rows: [ [ { heading: A } ] ] } ] ] } ]\n"), path: "/rows/1/0/rows/0/0/card", parts: []string{"a card can't be inside a card"}},
		{name: "views in a card", page: true, src: page("g5", "  - [ { card: K, rows: [ [ { view: keyword_list } ] ] } ]\n"),
			bad: page("g5", "  - [ { card: K, rows: [ [ { view: keyword_lists } ] ] } ]\n"), path: "/rows/1/0/rows/0/0/view", parts: []string{"no view keyword_lists"}},
		{name: "filter binds", page: true, src: page("g6", ""), bad: strings.Replace(page("g6", ""), "from_date, control", "to_date, control", 1),
			path: "/filters/0/param", parts: []string{"binds to no block", ":to_date"}},
		{name: "filter block", page: true, src: page("g7", "  - [ { filter: from_date } ]\n"), bad: page("g7", "  - [ { filter: from } ]\n"),
			path: "/rows/1/0/filter", parts: []string{"from is not a page filter; the page has from_date; did you mean from_date?"}},
		{name: "filter shown once", page: true, src: page("g8", "  - [ { filter: from_date } ]\n"), bad: page("g8", "  - [ { filter: from_date }, { card: K, rows: [ [ { filter: from_date } ] ] } ]\n"),
			path: "/rows/1/1/rows/0/0/filter", parts: []string{"shown already, at rows[1][0]"}},
		{name: "button target", page: true, src: page("g9", "  - [ { button: Open, open_page: bitcoin } ]\n"), bad: page("g9", "  - [ { button: Open, open_page: bitcion } ]\n"),
			path: "/rows/1/0/open_page", parts: []string{"no page bitcion", "did you mean bitcoin?"}},
		{name: "button self", page: true, src: page("g10", "  - [ { button: Top, open_page: g10 } ]\n"), bad: page("g10", "  - [ { button: Top } ]\n"),
			path: "/rows/1/0", parts: []string{"a button needs one of run_pipeline, open_page or open_form"}},
		{name: "button two targets", page: true, src: page("g11", "  - [ { button: Add, open_form: add_keyword } ]\n"), bad: page("g11", "  - [ { button: Add, open_form: add_keyword, open_page: bitcoin } ]\n"),
			path: "/rows/1/0", parts: []string{"open_page and open_form"}},
		{name: "button form", page: true, src: page("g12", "  - [ { button: Add, open_form: add_keyword } ]\n"), bad: page("g12", "  - [ { button: Add, open_form: btc_today } ]\n"),
			path: "/rows/1/0/open_form", parts: []string{"btc_today is a stat view, not a form"}},
		{name: "button pipeline", page: true, src: page("g13", "  - [ { button: Add, open_form: add_keyword } ]\n"), bad: page("g13", "  - [ { button: Run, run_pipeline: daily_btc } ]\n"),
			path: "/rows/1/0/run_pipeline", parts: []string{"pipelines can't be saved yet"}},
		{name: "action target", page: true, src: strings.Replace(page("g14", ""), "rows:", "actions: [ { label: Add, open_form: add_keyword } ]\nrows:", 1),
			bad: strings.Replace(page("g14", ""), "rows:", "actions: [ { label: Add, open_form: add_kw } ]\nrows:", 1), path: "/actions/0/open_form", parts: []string{"no form add_kw"}},
		{name: "action one target", page: true, src: strings.Replace(page("g15", ""), "rows:", "actions: [ { label: Go, open_page: bitcoin } ]\nrows:", 1),
			bad: strings.Replace(page("g15", ""), "rows:", "actions: [ { label: Go } ]\nrows:", 1), path: "/actions/0", parts: []string{"a page action needs one of"}},
		{name: "image", page: true, src: page("g16", "  - [ { image: images/logo.png, alt: Logo } ]\n"), bad: page("g16", "  - [ { image: images/chart.png } ]\n"),
			path: "/rows/1/0/image", parts: []string{"no file images/chart.png yet"}, warn: true},
		{name: "image key", page: true, src: page("g17", "  - [ { image: images/logo.png } ]\n"), bad: page("g17", "  - [ { image: ../logo.png } ]\n"),
			path: "/rows/1/0/image", parts: []string{"is not a bucket key"}},
		{name: "block type", page: true, src: page("g18", "  - [ { text: \"**Hi**\" } ]\n"), bad: page("g18", "  - [ { paragraph: Hi } ]\n"),
			path: "/rows/1/0", parts: []string{"a block needs one of these fields", "button, card, filter, heading, image, text, view"}},
		{name: "two types", page: true, src: page("g19", "  - [ { heading: A } ]\n"), bad: page("g19", "  - [ { heading: A, text: B } ]\n"),
			path: "/rows/1/0", parts: []string{"this one has heading and text"}},
		{name: "block field", page: true, src: page("g20", "  - [ { heading: A, span: 6 } ]\n"), bad: page("g20", "  - [ { heading: A, width: 6 } ]\n"),
			path: "/rows/1/0/width", parts: []string{"unknown field"}},
		{name: "page field", page: true, src: page("g21", ""), bad: page("g21", "") + "layout: grid\n", path: "/layout", parts: []string{"unknown field"}},
		{name: "page id", page: true, src: page("g22", ""), bad: page("btc_today", ""), path: "/id", parts: []string{"a view has the ID btc_today"}},
	} {
		c.run(t)
	}
}

// Kanban is a made-up page block for the registry test: a column of the
// cards of a view's rows, grouped by a field.
type Kanban struct {
	Kanban string `json:"kanban"` // a view ID
	By     string `json:"by"`
}

const kanbanSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$ref": "defs.json#/$defs/block",
  "required": ["kanban", "by"],
  "additionalProperties": false,
  "properties": {
    "span": {},
    "kanban": { "$ref": "defs.json#/$defs/ref" },
    "by": { "$ref": "defs.json#/$defs/name" },
    "tone": { "$ref": "defs.json#/$defs/tone" }
  }
}`

// A new block type is one unit: its schema, its validator and its
// component. Registering it is the only change (R-47, P3-02 done when).
func TestMadeUpBlock(t *testing.T) {
	reg := Builtin()
	err := Register(reg, Unit[Kanban]{
		Type:   "kanban",
		Kind:   PageBlock,
		Schema: []byte(kanbanSchema),
		Check: func(c *Check, k *Kanban) {
			v, ok := c.ViewRef("/kanban", k.Kanban)
			if !ok {
				c.Unbound()
				return
			}
			c.Binds(queryParams(v.Query)...)
			if _, ok := v.Spec.(*Table); !ok {
				c.Errorf(StepReferences, "/kanban", "a kanban shows a table view; %s is a %s", v.ID, v.Type)
			}
		},
		Component: "kanban component",
	})
	if err != nil {
		t.Fatal(err)
	}
	v := &Validator{Reg: reg, Env: ProjectEnv{DB: fixture}}
	src := page("k1", "  - [ { kanban: keyword_list, by: keyword, span: 6 }, { kanban: btc_price_table, by: date, span: 6 } ]\n")
	p, res, err := v.Page(context.Background(), []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	wantOK(t, res)
	if k, _ := p.Rows[1][0].Spec.(*Kanban); k == nil || k.By != "keyword" || p.Rows[1][0].Type != "kanban" {
		t.Errorf("parsed %+v", p.Rows[1][0])
	}
	if reg.Component(PageBlock, "kanban") != "kanban component" {
		t.Error("no component")
	}
	// Its schema and its validator report with paths.
	_, res, err = v.Page(context.Background(), []byte(page("k2", "  - [ { kanban: btc_today, by: price, tone: pink } ]\n")))
	if err != nil {
		t.Fatal(err)
	}
	wantErr(t, res, "/rows/1/0/tone", "pink is not one of")
	wantErr(t, res, "/rows/1/0/kanban", "a kanban shows a table view; btc_today is a stat")
	// The default registry doesn't know it.
	res = checkPage(t, src)
	wantErr(t, res, "/rows/1/0", "a block needs one of")

	// A made-up data unit is a view type the same way.
	type Gauge struct {
		Value struct {
			Field string `json:"field"`
		} `json:"value"`
		Max float64 `json:"max"`
	}
	err = Register(reg, Unit[Gauge]{Type: "gauge", Kind: Data, Schema: []byte(`{
  "$ref": "defs.json#/$defs/queryView",
  "required": ["value", "max"],
  "additionalProperties": false,
  "properties": {
    "version": {}, "id": {}, "title": {}, "description": {}, "actions": {}, "query": {}, "params": {}, "filters": {},
    "type": { "const": "gauge" },
    "value": { "type": "object", "required": ["field"], "additionalProperties": false, "properties": { "field": { "type": "string" } } },
    "max": { "type": "number", "exclusiveMinimum": 0 }
  }
}`), Check: func(c *Check, g *Gauge) { c.Field("/value/field", g.Value.Field) }})
	if err != nil {
		t.Fatal(err)
	}
	gauge := "version: 1\nid: g_price\ntitle: Price\ntype: gauge\nquery: SELECT max(price) AS price FROM btc_prices\nvalue: { field: price }\nmax: 100000\n"
	gv, res, err := v.View(context.Background(), []byte(gauge))
	if err != nil {
		t.Fatal(err)
	}
	wantOK(t, res)
	if g, _ := gv.Spec.(*Gauge); g == nil || g.Max != 100000 {
		t.Errorf("parsed %+v", gv.Spec)
	}
	_, res, _ = v.View(context.Background(), []byte(strings.Replace(gauge, "{ field: price }", "{ field: cost }", 1)))
	wantErr(t, res, "/value/field", "'cost' is not a result column")
}

func TestRegistry(t *testing.T) {
	reg := Builtin()
	if err := Register(reg, Unit[TextBlock]{Type: "text", Kind: PageBlock, Schema: schemaFile("text.json")}); err == nil {
		t.Error("registered text twice")
	}
	if err := Register(reg, Unit[TextBlock]{Type: "bad", Kind: PageBlock, Schema: []byte(`{"type": 5}`)}); err == nil {
		t.Error("a schema that doesn't compile")
	}
	if err := Register(reg, Unit[TextBlock]{Type: "box", Kind: Data, Container: true, Schema: []byte(`{}`)}); err == nil {
		t.Error("a data container")
	}
	if got := strings.Join(Default.Types(Data), ","); got != "chart,form,stat,table" {
		t.Errorf("data types %s", got)
	}
	if got := strings.Join(Default.Types(PageBlock), ","); got != "button,card,filter,heading,image,text,view" {
		t.Errorf("page types %s", got)
	}
	if !Default.Container("card") || Default.Container("view") {
		t.Error("containers")
	}
	if err := reg.SetComponent(Data, "table", 42); err != nil || reg.Component(Data, "table") != 42 || Default.Component(Data, "table") != nil {
		t.Errorf("components %v", err)
	}
	if err := reg.SetComponent(Data, "kanban", 1); err == nil {
		t.Error("set a component of no unit")
	}
}

func TestParseSaved(t *testing.T) {
	v, err := ParseView(Default, []byte(`version: 1
id: parsed
title: P
type: table
query: SELECT 1 AS n
params: { n: 5, f: 1.5, b: true }
filters: [ { param: x, control: select, options: [1, two, 3.5], default: 1 } ]
default_sort: { field: n, direction: asc }
columns: [ { field: n, format: number, format_options: { decimals: 2 } } ]
`))
	if err != nil {
		t.Fatal(err)
	}
	if v.Params["n"] != int64(5) || v.Params["f"] != 1.5 || v.Params["b"] != true {
		t.Errorf("params %#v", v.Params)
	}
	f := v.Filters[0]
	if f.Default != int64(1) || f.Options.List[0] != int64(1) || f.Options.List[1] != "two" || f.Options.List[2] != 3.5 {
		t.Errorf("filter %#v %#v", f, f.Options)
	}
	tb := v.Spec.(*Table)
	if *tb.Columns[0].FormatOptions.Decimals != 2 || tb.DefaultSort.Direction != "asc" {
		t.Errorf("table %+v", tb)
	}
	p, err := ParsePage(Default, []byte(specPage))
	if err != nil {
		t.Fatal(err)
	}
	if p.Filters[0].Default != "today-90d" || p.Rows[1][1].Span != 7 {
		t.Errorf("page %+v", p)
	}
	if _, err := ParseView(Default, []byte("version: 1\ntype: kanban\n")); err == nil {
		t.Error("an unknown type parsed")
	}
}

// Blocks without a span share what the others leave of the 12 columns,
// the first ones one more when it doesn't divide; 8.0 is the integer 8.
func TestSpans(t *testing.T) {
	for src, want := range map[string]string{
		"[ { heading: A, span: 8 }, { heading: B } ]":                                 "[8 4]",
		"[ { heading: A, span: 3 }, { heading: B }, { heading: C } ]":                 "[3 5 4]",
		"[ { heading: A }, { heading: B }, { heading: C } ]":                          "[4 4 4]",
		"[ { heading: A, span: 2 }, { heading: B }, { heading: C }, { heading: D } ]": "[2 4 3 3]",
		"[ { heading: A, span: 8.0 }, { heading: B } ]":                               "[8 4]",
		"[ { heading: A, span: 12 }, { heading: B } ]":                                "[12 1]",
	} {
		p, err := ParsePage(Default, []byte("version: 1\nid: s\ntitle: S\nrows:\n  - "+src+"\n"))
		if err != nil {
			t.Fatal(err)
		}
		var got []int
		for _, b := range p.Rows[0] {
			got = append(got, b.Span)
		}
		if fmt.Sprint(got) != want {
			t.Errorf("%s: spans %v, want %s", src, got, want)
		}
	}
}
