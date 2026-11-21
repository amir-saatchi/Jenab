package view

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"golang.org/x/text/unicode/norm"
)

// Tests for the points of Main-Session's review of P3-02.

// A pie chart with series_by and two y entries has both errors.
func TestPieErrors(t *testing.T) {
	res := checkView(t, `version: 1
id: pie2
title: Price
type: chart
chart_type: pie
query: SELECT date, price, 'BTC' AS coin FROM btc_prices
x: { field: coin }
y: [ { field: price }, { field: date } ]
series_by: coin
`)
	wantErr(t, res, "/series_by", "a pie chart can't have series_by")
	wantErr(t, res, "/y", "a pie chart has one y")
	wantN(t, res, 2)
}

// Result columns are matched as written; a name that differs only in case
// says how to write it, not "did you mean" the same spelling.
func TestColumnCase(t *testing.T) {
	ok := tableQ("cc1", "SELECT id, title AS Title FROM docs", "  - { field: Title }\n", "")
	wantOK(t, checkView(t, ok))
	res := checkView(t, strings.Replace(ok, "{ field: Title }", "{ field: title }", 1))
	i := wantErr(t, res, "/columns/0/field", "'title' is not a result column", "names are matched as written, so write it as Title")
	if strings.Contains(i.Msg, "did you mean") {
		t.Errorf("message %q", i.Msg)
	}
}

// A second YAML document is refused, not dropped; a --- with nothing
// after it is no document.
func TestOneDocument(t *testing.T) {
	ok := table("d1", "")
	wantOK(t, checkView(t, "---\n"+ok))
	wantOK(t, checkView(t, ok+"---\n"))
	res := checkView(t, ok+"---\nversion: 1\nid: d2\n")
	wantN(t, res, 1)
	if i := res.Issues[0]; !strings.Contains(i.Msg, "a config is one YAML document") || i.Line != 11 {
		t.Errorf("issue %+v", i)
	}
	res = checkView(t, ok+"---\nversion: [\n")
	if res.OK() {
		t.Error("a broken second document passes")
	}
}

// At most MaxIssues errors and MaxIssues warnings are listed, and the
// rest counted.
func TestIssueCap(t *testing.T) {
	var cols, params strings.Builder
	for i := range 120 {
		fmt.Fprintf(&cols, "  - { field: f%d }\n", i)
	}
	params.WriteString("params:\n")
	for i := range 60 {
		fmt.Fprintf(&params, "  p%d: 1\n", i)
	}
	res := checkView(t, tableQ("cap", "SELECT id, title FROM docs", cols.String(), params.String()))
	if n, w := len(res.Errors()), len(res.Warnings()); n != MaxIssues || w != MaxIssues || res.MoreErrors != 70 || res.MoreWarnings != 10 {
		t.Fatalf("%d errors, %d warnings, %d and %d more", n, w, res.MoreErrors, res.MoreWarnings)
	}
	s := res.String()
	for _, part := range []string{"120 errors:\n", "line 8:14 columns[0].field:", "and 70 more errors; fix these first\n", "and 10 more warnings\n"} {
		if !strings.Contains(s, part) {
			t.Errorf("no %q in\n%s", part, s)
		}
	}
	if strings.Index(s, "and 70 more errors") > strings.Index(s, "warning: ") {
		t.Errorf("errors after warnings:\n%s", s)
	}
}

// The ID is checked as it is saved, in NFC: 40 letters é are 80 code
// points decomposed, and 40 composed.
func TestIDNormalizedFirst(t *testing.T) {
	nfd := strings.Repeat(norm.NFD.String("é"), 40)
	v, res, err := validator().View(context.Background(), []byte(table(nfd, "")))
	if err != nil {
		t.Fatal(err)
	}
	wantOK(t, res)
	if v.ID != norm.NFC.String(nfd) {
		t.Errorf("ID %q", v.ID)
	}
	wantErr(t, checkView(t, table(strings.Repeat(norm.NFD.String("é"), 65), "")), "/id")
}

// A page block type can't be a field of a block, nor have a block type
// for a field: blocks are known by their key.
func TestRegistryNames(t *testing.T) {
	reg := Builtin()
	schema := func(typ, field string) []byte {
		return []byte(`{"type": "object", "properties": {"` + typ + `": {"type": "string"}, "` + field + `": {}, "span": {}}, "additionalProperties": false}`)
	}
	for _, typ := range []string{"span", "rows", "tone", "alt", "confirm", "open_page", "run_pipeline", "label"} {
		err := Register(reg, Unit[TextBlock]{Type: typ, Kind: PageBlock, Schema: schema(typ, "note")})
		if typ == "label" {
			if err != nil { // no built-in block has a label field
				t.Errorf("%s: %v", typ, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("registered a block type %s", typ)
		}
	}
	if err := Register(reg, Unit[TextBlock]{Type: "banner", Kind: PageBlock, Schema: schema("banner", "heading")}); err == nil || !strings.Contains(err.Error(), "heading is a block type") {
		t.Errorf("a block with a heading field: %v", err)
	}
	if err := Register(reg, Unit[TextBlock]{Type: "banner", Kind: PageBlock, Schema: schema("banner", "note")}); err != nil {
		t.Error(err)
	}
	// A data type is a value of type, so it may share a block field's name.
	if err := Register(reg, Unit[Stat]{Type: "tone", Kind: Data, Schema: []byte(`{}`)}); err != nil {
		t.Error(err)
	}
}

// Messages read like the rest: no <nil>, no raw keyword names, no second
// error for a field given as null, and a config with rows or a misspelled
// rows is taken for a page.
func TestMessages(t *testing.T) {
	kw := specViews["keyword_list"]
	del := `{ label: "Delete", delete: true, confirm: true }`
	res := checkView(t, strings.Replace(kw, del, `{ label: "Set", set: null }`, 1))
	wantErr(t, res, "/row_actions/0/set", "must be an object (fields), not null")
	wantN(t, res, 1)
	res = checkView(t, strings.Replace(kw, del, `{ label: "Set", set: {} }`, 1))
	wantErr(t, res, "/row_actions/0/set", "needs at least 1 field")
	res = checkPage(t, page("m1", "  - [ { button: Go, open_page: bitcoin, tone: null } ]\n"))
	wantErr(t, res, "/rows/1/0/tone", "null is not one of neutral")
	for _, i := range res.Issues {
		if strings.Contains(i.Msg, "<nil>") || strings.Contains(i.Msg, "minProperties") {
			t.Errorf("raw message %q", i.Msg)
		}
	}

	ctx := context.Background()
	kind, _, res, err := validator().Config(ctx, []byte(strings.Replace(specPage, "rows:", "rwos:", 1)))
	if err != nil || kind != "page" {
		t.Fatalf("%v %v", kind, err)
	}
	wantErr(t, res, "/rwos", "unknown field; did you mean rows?")
	wantErr(t, res, "/rows", "is required")
	wantN(t, res, 2)
	_, res, err = validator().View(ctx, []byte(specPage))
	if err != nil {
		t.Fatal(err)
	}
	wantErr(t, res, "/type", "a view has a type, one of chart, form, stat, table", "it looks like a page")
	// A misspelled field of a view gets a suggestion too.
	wantErr(t, checkView(t, table("m2", "page_szie: 20\n")), "/page_szie", "did you mean page_size?")
}
