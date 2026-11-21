package view

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/unicode/norm"
)

// A test per check of P3-02, each with a passing and a failing config.

// tc is one check: src passes, bad fails with an issue at path whose
// message has parts. warn: the issue is a warning, and bad still saves.
type tc struct {
	name  string
	page  bool
	src   string
	bad   string
	path  string
	parts []string
	warn  bool
}

func (c tc) run(t *testing.T) {
	t.Run(c.name, func(t *testing.T) {
		check := checkView
		if c.page {
			check = checkPage
		}
		if c.src != "" {
			wantOK(t, check(t, c.src))
		}
		res := check(t, c.bad)
		want(t, res, c.warn, c.path, c.parts...)
		if c.warn && !res.OK() {
			t.Errorf("a warning only, but:\n%s", res)
		}
	})
}

// table is a table view on btc_prices with extra lines.
func table(id, extra string) string {
	return `version: 1
id: ` + id + `
title: Prices
type: table
query: SELECT date, price FROM btc_prices
default_sort: { field: date, direction: desc }
columns:
  - { field: date, format: date }
  - { field: price, format: currency, format_options: { currency: USD } }
` + extra
}

// tableQ is a table view with its own query, columns and extra lines.
func tableQ(id, query, columns, extra string) string {
	return `version: 1
id: ` + id + `
title: Docs
type: table
query: ` + query + `
default_sort: { field: id, direction: desc }
columns:
` + columns + extra
}

func TestParse(t *testing.T) {
	for _, c := range []tc{
		{name: "colon", src: table("p1", ""), bad: strings.Replace(table("p1", ""), "title: Prices", "title: Price: USD", 1),
			parts: []string{"mapping values are not allowed", `quote this value: it contains ": "`}},
		{name: "flow", bad: table("p2", "") + "x: { a: b, c\n", parts: []string{"quote values inside { } or [ ]"}},
		{name: "tab", bad: "version: 1\nid: p3\n\ttitle: x\n", parts: []string{"tabs"}},
		{name: "quote", bad: "version: 1\nid: p4\ntitle: \"Prices\n", parts: []string{"never closed"}},
		{name: "anchor", bad: table("p5", "x: &a 1\n"), parts: []string{"anchors (&a) are not allowed"}},
		{name: "alias", bad: table("p6", "y: &b [1]\nz: *b\n"), parts: []string{"anchors"}},
		{name: "merge", bad: table("p7", "x: { <<: { a: 1 } }\n"), parts: []string{"merge keys"}},
		{name: "duplicate", bad: table("p8", "title: Again\n"), parts: []string{"the key title is given twice"}},
		{name: "tag", bad: table("p9", "x: !!str 1\n"), parts: []string{"tags (!!str) are not allowed"}},
		{name: "map tag", bad: table("p10", "x: !!map { a: 1 }\n"), parts: []string{"tags (!!map) are not allowed"}},
		{name: "zero", bad: table("p11", "page_size: 050\n"), parts: []string{"starts with a zero"}},
		{name: "key", bad: table("p12", "1: x\n"), parts: []string{"the key 1 must be text"}},
		{name: "list", bad: "- a\n- b\n", parts: []string{"a config is a mapping"}},
		{name: "empty", bad: "  \n", parts: []string{"the config is empty"}},
		{name: "big", bad: "version: 1\ndescription: " + strings.Repeat("x", MaxSize) + "\n", parts: []string{"at most 262144"}},
	} {
		c.run(t)
	}
	// A parse error has no path, only its place.
	res := checkView(t, "version: 1\ntitle: Price: USD\n")
	if len(res.Issues) != 1 || res.Issues[0].Line != 2 || !strings.HasPrefix(res.Issues[0].String(), "line 2:") || strings.Contains(res.Issues[0].String(), "top level") {
		t.Errorf("%q", res.String())
	}
	// JSON is YAML too.
	wantOK(t, checkView(t, `{"version": 1, "id": "json_chart", "title": "Price", "type": "chart", "chart_type": "line",
		"query": "SELECT date, price FROM btc_prices", "x": {"field": "date"}, "y": [{"field": "price"}]}`))
}

func TestSchema(t *testing.T) {
	ok := table("s1", "")
	for _, c := range []tc{
		{name: "unknown field", src: ok, bad: table("s1", "colour: red\n"), path: "/colour", parts: []string{"unknown field"}},
		{name: "missing", src: ok, bad: strings.Replace(ok, "default_sort: { field: date, direction: desc }\n", "", 1), path: "/default_sort", parts: []string{"is required"}},
		{name: "type", src: ok, bad: table("s1", "page_size: lots\n"), path: "/page_size", parts: []string{"must be an integer, not a string"}},
		{name: "range", src: ok, bad: table("s1", "page_size: 900\n"), path: "/page_size", parts: []string{"900 is too large; the most is 500"}},
		{name: "version", src: ok, bad: strings.Replace(ok, "version: 1", "version: 2", 1), path: "/version", parts: []string{"must be 1, not 2"}},
		{name: "options", src: table("s1", ""), bad: table("s1", "filters: [ { param: p, control: select, options: 5 } ]\n"),
			path: "/filters/0/options", parts: []string{"must be one of these", "an array", "an object"}},
		{name: "unknown type", src: ok, bad: strings.Replace(ok, "type: table", "type: kanban", 1), path: "/type", parts: []string{"unknown view type", "chart, form, stat, table"}},
		{name: "no type", bad: strings.Replace(ok, "type: table\n", "", 1), path: "/type", parts: []string{"is required"}},
		{name: "tone", page: true, src: page("s2", "  - [ { button: Go, open_page: bitcoin, tone: red } ]\n"),
			bad: page("s2", "  - [ { button: Go, open_page: bitcoin, tone: \"#ff0000\" } ]\n"), path: "/rows/1/0/tone", parts: []string{"#ff0000 is not one of neutral, blue, green, amber, red, purple"}},
	} {
		c.run(t)
	}
	// Unknown and missing fields point at the key, others at the value.
	res := checkView(t, table("s3", "colour: red\npage_size: lots\n"))
	if i := wantErr(t, res, "/colour", "unknown"); i.Col != 1 {
		t.Errorf("unknown field at %+v", i)
	}
	if i := wantErr(t, res, "/page_size", "integer"); i.Col != 12 {
		t.Errorf("wrong value at %+v", i)
	}
}

func TestID(t *testing.T) {
	for _, c := range []tc{
		{name: "letters", src: table("سعر_بیت-کوین", ""), bad: table("prices table", ""), path: "/id", parts: []string{"an ID is 1 to 64"}},
		{name: "start", src: table("1prices", ""), bad: table("_prices", ""), path: "/id", parts: []string{"starting with a letter or digit"}},
		{name: "taken by a page", src: table("bitcoin_prices", ""), bad: table("bitcoin", ""), path: "/id", parts: []string{"a page has the ID bitcoin"}},
	} {
		c.run(t)
	}
	// The ID is NFC, so two spellings are one (P3-01).
	nfd := norm.NFD.String("آب")
	if nfd == "آب" {
		t.Fatal("no decomposed spelling")
	}
	v, res, err := validator().View(context.Background(), []byte(table(nfd, "")))
	if err != nil || !res.OK() {
		t.Fatalf("%v\n%s", err, res)
	}
	if v.ID != "آب" {
		t.Errorf("ID %q is not NFC", v.ID)
	}
}

func TestFilters(t *testing.T) {
	// view uses each filter's param in its query.
	view := func(filters string) string {
		var ps []string
		for _, m := range regexp.MustCompile(`param: (\w+)`).FindAllStringSubmatch(filters, -1) {
			ps = append(ps, ":"+m[1])
		}
		return strings.Replace(table("f1", "filters:\n"+filters), "FROM btc_prices\n", "FROM btc_prices WHERE coalesce("+strings.Join(ps, ", ")+", 1) IS NOT NULL\n", 1)
	}
	okFilters := "  - { param: from, control: date, default: today-90d, required: true }\n  - { param: min, control: number, default: 100 }\n"
	for _, c := range []tc{
		{name: "control", src: view(okFilters), bad: view("  - { param: from, control: calendar }\n  - { param: min, control: number }\n"),
			path: "/filters/0/control", parts: []string{"calendar is not one of text, number, date, select, checkbox"}},
		{name: "required", src: view(okFilters), bad: view("  - { param: from, control: date, required: yes please }\n  - { param: min, control: number }\n"),
			path: "/filters/0/required", parts: []string{"must be a boolean"}},
		{name: "relative date", src: view(okFilters), bad: view("  - { param: from, control: date, default: today+3d }\n  - { param: min, control: number }\n"),
			path: "/filters/0/default", parts: []string{"is not a date", "today-7d"}},
		{name: "iso date", src: view("  - { param: from, control: date, default: \"2026-01-31\" }\n  - { param: min, control: number }\n"),
			bad:  view("  - { param: from, control: date, default: \"2026-02-31\" }\n  - { param: min, control: number }\n"),
			path: "/filters/0/default", parts: []string{"is not a date"}},
		{name: "start_of", src: view("  - { param: from, control: date, default: start_of_month }\n  - { param: min, control: number }\n"),
			bad:  view("  - { param: from, control: date, default: start_of_week }\n  - { param: min, control: number }\n"),
			path: "/filters/0/default", parts: []string{"is not a date"}},
		{name: "number default", src: view(okFilters), bad: view("  - { param: from, control: date }\n  - { param: min, control: number, default: lots }\n"),
			path: "/filters/1/default", parts: []string{"must be a number"}},
		{name: "params and filters", src: view(okFilters), bad: strings.Replace(view(okFilters), "filters:\n", "params: { min: 5 }\nfilters:\n", 1),
			path: "/filters/1/param", parts: []string{"min is in params too"}},
		{name: "twice", src: view(okFilters), bad: view(okFilters + "  - { param: min, control: text }\n"),
			path: "/filters/2/param", parts: []string{"filters[1] has this parameter already"}},
		{name: "param name", src: view(okFilters), bad: view(okFilters + "  - { param: 2nd, control: text }\n"),
			path: "/filters/2/param", parts: []string{"not a parameter name"}},
		{name: "select list", src: view(okFilters + "  - { param: coin, control: select, options: [BTC, ETH], default: BTC }\n"),
			bad:  view(okFilters + "  - { param: coin, control: select, options: [BTC, ETH], default: DOGE }\n"),
			path: "/filters/2/default", parts: []string{"DOGE is not one of the options"}},
		{name: "select needs options", src: view(okFilters + "  - { param: coin, control: select, options: [BTC] }\n"),
			bad: view(okFilters + "  - { param: coin, control: select }\n"), path: "/filters/2/control", parts: []string{"a select needs options"}},
		{name: "options on text", src: view(okFilters), bad: view(okFilters + "  - { param: coin, control: text, options: [BTC] }\n"),
			path: "/filters/2/options", parts: []string{"only a select has options"}},
		{name: "options query", src: view(okFilters + "  - { param: kw, control: select, options: { query: \"SELECT keyword FROM watch_keywords\" } }\n"),
			bad:  view(okFilters + "  - { param: kw, control: select, options: { query: \"SELECT id, keyword FROM watch_keywords\" } }\n"),
			path: "/filters/2/options/query", parts: []string{"returns one column", "returns 2 (id, keyword)"}},
		{name: "options query fails", src: view(okFilters + "  - { param: kw, control: select, options: { query: \"SELECT keyword FROM watch_keywords\" } }\n"),
			bad:  view(okFilters + "  - { param: kw, control: select, options: { query: \"SELECT word FROM watch_keywords\" } }\n"),
			path: "/filters/2/options/query", parts: []string{"no such column: word"}},
		{name: "options query params", src: view(okFilters + "  - { param: kw, control: select, options: { query: \"SELECT keyword FROM watch_keywords\" } }\n"),
			bad:  view(okFilters + "  - { param: kw, control: select, options: { query: \"SELECT keyword FROM watch_keywords WHERE id > :min\" } }\n"),
			path: "/filters/2/options/query", parts: []string{"an options query takes no parameters", ":min would always be NULL"}},
		{name: "checkbox", src: view(okFilters + "  - { param: on, control: checkbox, default: true }\n"),
			bad: view(okFilters + "  - { param: on, control: checkbox, default: yes }\n"), path: "/filters/2/default", parts: []string{"true or false"}},
	} {
		c.run(t)
	}
	// Relative dates go back from today.
	today := mustDate(t, "2026-03-31")
	for expr, wantD := range map[string]string{"today": "2026-03-31", "today-7d": "2026-03-24", "today-1m": "2026-03-03",
		"today-1y": "2025-03-31", "start_of_month": "2026-03-01", "start_of_year": "2026-01-01", "2026-11-20": "2026-11-20"} {
		got, ok := RelativeDate(expr, today)
		if !ok || got.Format("2006-01-02") != wantD {
			t.Errorf("%s: %v %v, want %s", expr, got, ok, wantD)
		}
	}
	for _, bad := range []string{"today-", "today-d", "today--1d", "today-+1d", "today-1w", "tomorrow", "2026-13-01"} {
		if _, ok := RelativeDate(bad, today); ok {
			t.Errorf("%s reads", bad)
		}
	}
}

func TestParamBindings(t *testing.T) {
	bound := strings.Replace(table("b1", "params: { from: \"2026-01-01\" }\n"), "FROM btc_prices\n", "FROM btc_prices WHERE date >= :from\n", 1)
	for _, c := range []tc{
		{name: "positional", src: bound, bad: strings.Replace(table("b1", ""), "FROM btc_prices\n", "FROM btc_prices WHERE date >= ?\n", 1),
			path: "/query", parts: []string{"use named parameters"}},
		{name: "unbound", src: bound, bad: strings.Replace(table("b1", ""), "FROM btc_prices\n", "FROM btc_prices WHERE date >= :from\n", 1),
			path: "/query", parts: []string{":from is not in params or filters", "page filter"}, warn: true},
		{name: "unused param", src: bound, bad: table("b1", "params: { coin: BTC }\n"), path: "/params/coin", parts: []string{"the query has no :coin"}, warn: true},
		{name: "unused filter", src: bound, bad: table("b1", "filters: [ { param: coin, control: text } ]\n"), path: "/filters/0/param", parts: []string{"the query has no :coin"}, warn: true},
		{name: "param name", src: bound, bad: strings.Replace(bound, "params: { from:", "params: { \"2x\": 1, from:", 1), path: "/params/2x", parts: []string{"not a parameter name"}},
		{name: "@name", src: bound, bad: strings.Replace(bound, ":from", "@from", 1), path: "/query", parts: []string{"write :from, not @from", "only :name parameters"}},
		{name: "$name", src: bound, bad: strings.Replace(bound, ":from", "$from", 1), path: "/query", parts: []string{"write :from, not $from"}},
	} {
		c.run(t)
	}
}

func TestSQL(t *testing.T) {
	ok := table("q1", "")
	for _, c := range []tc{
		{name: "text check", src: ok, bad: strings.Replace(ok, "SELECT date, price FROM btc_prices", "DELETE FROM btc_prices", 1),
			path: "/query", parts: []string{"line 1:1 of the query"}},
		{name: "jenab table", src: ok, bad: strings.Replace(ok, "FROM btc_prices", "FROM _jenab_views", 1), path: "/query", parts: []string{"_jenab_"}},
		{name: "two statements", src: ok, bad: strings.Replace(ok, "FROM btc_prices", "FROM btc_prices; SELECT 1", 1), path: "/query"},
		{name: "unknown column", src: ok, bad: strings.Replace(ok, "SELECT date, price", "SELECT date, prise AS price", 1), path: "/query", parts: []string{"no such column: prise"}},
		{name: "unknown table", src: ok, bad: strings.Replace(ok, "FROM btc_prices", "FROM btc_price", 1), path: "/query", parts: []string{"no such table: btc_price"}},
		{name: "alias", src: strings.Replace(ok, "SELECT date, price", "SELECT date, AVG(price) AS price", 1),
			bad: strings.Replace(ok, "SELECT date, price", "SELECT date, AVG(price) AS avg_price", 1), path: "/columns/1/field", parts: []string{"'price' is not a result column", "date, avg_price"}},
		{name: "order by", src: ok, bad: strings.Replace(ok, "FROM btc_prices", "FROM btc_prices ORDER BY date", 1),
			path: "/query", parts: []string{"line 1:36 of the query", "ORDER BY is ignored"}, warn: true},
		{name: "inner order by", src: strings.Replace(ok, "FROM btc_prices", "FROM (SELECT * FROM btc_prices ORDER BY date LIMIT 5)", 1),
			bad: strings.Replace(ok, "FROM btc_prices", "FROM btc_prices LIMIT 5", 1), path: "/query", parts: []string{"can't have LIMIT"}},
		{name: "stat order by and limit", src: specViews["btc_today"], bad: strings.Replace(specViews["btc_today"], "AS change", "AS delta", 1),
			path: "/change/field", parts: []string{"'change' is not a result column", "price, delta"}},
	} {
		c.run(t)
	}
}

func TestTableChecks(t *testing.T) {
	docs := func(id, extra string) string {
		return tableQ(id, "SELECT id, title, file, cover, status FROM docs", "  - { field: title }\n", extra)
	}
	for _, c := range []tc{
		{name: "sort field", src: table("t1", ""), bad: strings.Replace(table("t1", ""), "field: date, direction", "field: day, direction", 1),
			path: "/default_sort/field", parts: []string{"'day' is not a result column"}},
		{name: "image format", src: docs("t2", "  - { field: file, format: file }\n"), bad: docs("t2", "  - { field: cover, format: image }\n"),
			path: "/columns/1/format", parts: []string{"docs.cover is not an object_ref column"}},
		{name: "image expression", src: docs("t3", "  - { field: file, format: image }\n"),
			bad:  tableQ("t3", "SELECT id, title, 'x/' || file AS file FROM docs", "  - { field: file, format: image }\n", ""),
			path: "/columns/0/format", parts: []string{"selected as it is, not an expression"}},
		{name: "image through subquery", src: tableQ("t4", "SELECT * FROM (SELECT id, file FROM docs)", "  - { field: file, format: image }\n", ""),
			bad:  tableQ("t4", "SELECT * FROM (SELECT id, note AS file FROM docs)", "  - { field: file, format: image }\n", ""),
			path: "/columns/0/format", parts: []string{"docs.note is not an object_ref column"}},
		{name: "image through a union", src: tableQ("t5", "SELECT id, file FROM docs UNION SELECT id, file FROM docs", "  - { field: file, format: image }\n", ""),
			bad:  tableQ("t5", "SELECT id, file FROM docs UNION SELECT id, note FROM docs", "  - { field: file, format: image }\n", ""),
			path: "/columns/0/format", parts: []string{"not an expression"}},
		{name: "format options", src: docs("t5", "  - { field: status, format: badge, format_options: { tones: { new: blue } } }\n"),
			bad: docs("t5", "  - { field: status, format: text, format_options: { tones: { new: blue } } }\n"), path: "/columns/1/format_options/tones",
			parts: []string{"only the badge format uses tones"}, warn: true},
		{name: "tone", src: docs("t6", "  - { field: status, format: badge, format_options: { tones: { new: blue } } }\n"),
			bad: docs("t6", "  - { field: status, format: badge, format_options: { tones: { new: navy } } }\n"), path: "/columns/1/format_options/tones/new",
			parts: []string{"navy is not one of neutral"}},
	} {
		c.run(t)
	}
}

func TestRowActions(t *testing.T) {
	docs := func(id, rowsFrom, actions string) string {
		return tableQ(id, "SELECT id, title, read FROM docs", "  - { field: title }\n", "rows_from: "+rowsFrom+"\nrow_actions:\n"+actions)
	}
	rf := "{ table: docs, key: id }"
	okAct := "  - { label: Read, set: { read: true, note: today(), score: 1.5, status: null }, bulk: true }\n"
	for _, c := range []tc{
		{name: "rows_from needed", src: docs("r1", rf, okAct), bad: tableQ("r1", "SELECT id, title FROM docs", "  - { field: title }\n", "row_actions:\n"+okAct),
			path: "/row_actions", parts: []string{"row actions need rows_from"}},
		{name: "rows_from table", src: docs("r2", rf, okAct), bad: docs("r2", "{ table: doc, key: id }", okAct), path: "/rows_from/table", parts: []string{"no table doc", "did you mean docs?"}},
		{name: "rows_from jenab", src: docs("r2", rf, okAct), bad: docs("r2", "{ table: _jenab_objects, key: key }", okAct), path: "/rows_from/table", parts: []string{"Jenab's own tables"}},
		{name: "rows_from key", src: docs("r3", rf, okAct), bad: docs("r3", "{ table: docs, key: title }", okAct), path: "/rows_from/key", parts: []string{"title is not the key of docs: its key is id"}},
		{name: "composite key", src: tableQ("r4", "SELECT a || b AS id, a, b, note FROM pairs", "  - { field: note }\n", ""),
			bad:  tableQ("r4", "SELECT a || b AS id, a, b, note FROM pairs", "  - { field: note }\n", "rows_from: { table: pairs, key: a }\nrow_actions: [ { label: Del, delete: true } ]\n"),
			path: "/rows_from/key", parts: []string{"one-column key"}},
		{name: "key selected", src: docs("r5", rf, okAct), bad: tableQ("r5", "SELECT title, read FROM docs", "  - { field: title }\n", "rows_from: "+rf+"\nrow_actions:\n"+okAct),
			path: "/rows_from/key", parts: []string{"the query must select the key column id"}},
		{name: "one action", src: docs("r6", rf, okAct), bad: docs("r6", rf, "  - { label: X, open_form: edit_keyword, delete: true }\n"), path: "/row_actions/0", parts: []string{"open_form and delete"}},
		{name: "no action", src: docs("r6", rf, okAct), bad: docs("r6", rf, "  - { label: X }\n"), path: "/row_actions/0", parts: []string{"needs one of open_form, set or delete"}},
		{name: "bulk", src: docs("r7", rf, okAct), bad: docs("r7", rf, "  - { label: X, open_form: edit_keyword, bulk: true }\n"), path: "/row_actions/0/bulk", parts: []string{"not open_form"}},
		{name: "open_form", src: docs("r8", rf, "  - { label: Edit, open_form: edit_keyword }\n"), bad: docs("r8", rf, "  - { label: Edit, open_form: edit_keywrd }\n"),
			path: "/row_actions/0/open_form", parts: []string{"no form edit_keywrd", "did you mean edit_keyword?"}},
		{name: "open_form a table", src: docs("r9", rf, "  - { label: Edit, open_form: edit_keyword }\n"), bad: docs("r9", rf, "  - { label: Edit, open_form: keyword_list }\n"),
			path: "/row_actions/0/open_form", parts: []string{"keyword_list is a table view, not a form"}},
		{name: "open_form insert", src: docs("r10", rf, "  - { label: Edit, open_form: edit_keyword }\n"), bad: docs("r10", rf, "  - { label: Edit, open_form: add_keyword }\n"),
			path: "/row_actions/0/open_form", parts: []string{"an insert form"}, warn: true},
		{name: "set column", src: docs("r11", rf, okAct), bad: docs("r11", rf, "  - { label: X, set: { red: true } }\n"), path: "/row_actions/0/set/red", parts: []string{"red is not a column of docs", "did you mean read?"}},
		{name: "set key", src: docs("r12", rf, okAct), bad: docs("r12", rf, "  - { label: X, set: { id: 5 } }\n"), path: "/row_actions/0/set/id", parts: []string{"id is the key of docs"}},
		{name: "set _run_id", src: docs("r13", rf, okAct), bad: docs("r13", rf, "  - { label: X, set: { _run_id: x } }\n"), path: "/row_actions/0/set/_run_id", parts: []string{"_run_id is not a column of docs"}},
		{name: "set generated", src: docs("r14", rf, okAct), bad: docs("r14", rf, "  - { label: X, set: { doubled: 4 } }\n"), path: "/row_actions/0/set/doubled", parts: []string{"generated"}},
		{name: "set type", src: docs("r15", rf, okAct), bad: docs("r15", rf, "  - { label: X, set: { read: yes please } }\n"), path: "/row_actions/0/set/read", parts: []string{"is text, but docs.read is INTEGER"}},
		{name: "set now", src: docs("r16", rf, okAct), bad: docs("r16", rf, "  - { label: X, set: { score: now() } }\n"), path: "/row_actions/0/set/score", parts: []string{"now() gives the time as text, but docs.score is REAL"}},
		{name: "set null", src: docs("r17", rf, okAct), bad: docs("r17", rf, "  - { label: X, set: { title: null } }\n"), path: "/row_actions/0/set/title", parts: []string{"can't be null"}},
		{name: "set text", src: docs("r18", rf, okAct), bad: docs("r18", rf, "  - { label: X, set: { status: 3 } }\n"), path: "/row_actions/0/set/status", parts: []string{"is a number, but docs.status is TEXT; quote it"}},
	} {
		c.run(t)
	}
	// _run_id and _fetched_at can't be set where they are columns.
	res := checkView(t, tableQ("r19", "SELECT date, price FROM btc_prices", "  - { field: price }\n",
		"rows_from: { table: btc_prices, key: date }\nrow_actions: [ { label: X, set: { _fetched_at: now() } } ]\n"))
	wantErr(t, res, "/row_actions/0/set/_fetched_at", "written by pipelines")
}

func TestChartChecks(t *testing.T) {
	chart := func(id, extra string) string {
		return `version: 1
id: ` + id + `
title: Price
type: chart
query: SELECT date, price, 'BTC' AS coin FROM btc_prices
` + extra
	}
	for _, c := range []tc{
		{name: "x", src: chart("c1", "chart_type: line\nx: { field: date }\ny: [ { field: price } ]\n"), bad: chart("c1", "chart_type: line\nx: { field: day }\ny: [ { field: price } ]\n"),
			path: "/x/field", parts: []string{"'day' is not a result column"}},
		{name: "y", src: chart("c2", "chart_type: bar\nx: { field: date }\ny: [ { field: price } ]\n"), bad: chart("c2", "chart_type: bar\nx: { field: date }\ny: [ { field: price }, { field: cost } ]\n"),
			path: "/y/1/field", parts: []string{"'cost' is not a result column"}},
		{name: "series_by", src: chart("c3", "chart_type: line\nx: { field: date }\ny: [ { field: price } ]\nseries_by: coin\n"),
			bad: chart("c3", "chart_type: line\nx: { field: date }\ny: [ { field: price }, { field: coin } ]\nseries_by: coin\n"), path: "/y", parts: []string{"with series_by, y has exactly one entry"}},
		{name: "series_by field", src: chart("c4", "chart_type: line\nx: { field: date }\ny: [ { field: price } ]\nseries_by: coin\n"),
			bad: chart("c4", "chart_type: line\nx: { field: date }\ny: [ { field: price } ]\nseries_by: symbol\n"), path: "/series_by", parts: []string{"'symbol' is not a result column"}},
		{name: "pie series_by", src: chart("c5", "chart_type: pie\nx: { field: coin }\ny: [ { field: price } ]\n"),
			bad: chart("c5", "chart_type: pie\nx: { field: coin }\ny: [ { field: price } ]\nseries_by: coin\n"), path: "/series_by", parts: []string{"a pie chart can't have series_by"}},
		{name: "pie y", src: chart("c6", "chart_type: pie\nx: { field: coin }\ny: [ { field: price } ]\n"),
			bad: chart("c6", "chart_type: pie\nx: { field: coin }\ny: [ { field: price }, { field: date } ]\n"), path: "/y", parts: []string{"a pie chart has one y"}},
		{name: "chart_type", src: chart("c7", "chart_type: scatter\nx: { field: date }\ny: [ { field: price } ]\n"),
			bad: chart("c7", "chart_type: area\nx: { field: date }\ny: [ { field: price } ]\n"), path: "/chart_type", parts: []string{"area is not one of line, bar, scatter, pie"}},
	} {
		c.run(t)
	}
}

func TestFormChecks(t *testing.T) {
	form := func(id, fields, submit, extra string) string {
		return `version: 1
id: ` + id + `
title: Doc
type: form
fields:
` + fields + "submit: " + submit + "\n" + extra
	}
	ins := "{ action: insert, table: docs }"
	okFields := "  - { field: title, control: text, required: true }\n  - { field: file, control: file, prefix: uploads/ }\n"
	for _, c := range []tc{
		{name: "table", src: form("o1", okFields, ins, ""), bad: form("o1", okFields, "{ action: insert, table: doc }", ""), path: "/submit/table", parts: []string{"no table doc"}},
		{name: "jenab table", src: form("o1", okFields, ins, ""), bad: form("o1", okFields, "{ action: insert, table: _jenab_changes }", ""), path: "/submit/table", parts: []string{"Jenab's own tables"}},
		{name: "column", src: form("o2", okFields, ins, ""), bad: form("o2", okFields+"  - { field: titel, control: text }\n", ins, ""),
			path: "/fields/2/field", parts: []string{"titel is not a column of docs", "did you mean title?"}},
		{name: "not null", src: form("o3", okFields, ins, ""), bad: form("o3", "  - { field: file, control: file, prefix: uploads/ }\n", ins, ""),
			path: "/fields", parts: []string{"docs.title is NOT NULL and has no default", "required field title"}},
		{name: "not null required", src: form("o4", okFields, ins, ""), bad: form("o4", "  - { field: title, control: text }\n", ins, ""),
			path: "/fields/0/required", parts: []string{"must be true"}},
		{name: "without rowid key", src: form("o5", "  - { field: a, control: text, required: true }\n  - { field: b, control: text, required: true }\n", "{ action: insert, table: pairs }", ""),
			bad: form("o5", "  - { field: a, control: text, required: true }\n", "{ action: insert, table: pairs }", ""), path: "/fields", parts: []string{"pairs.b is NOT NULL"}},
		{name: "file column", src: form("o6", okFields, ins, ""), bad: form("o6", okFields+"  - { field: cover, control: file, prefix: covers/ }\n", ins, ""),
			path: "/fields/2/control", parts: []string{"docs.cover is not an object_ref column"}},
		{name: "file prefix", src: form("o7", okFields, ins, ""), bad: form("o7", "  - { field: title, control: text, required: true }\n  - { field: file, control: file }\n", ins, ""),
			path: "/fields/1/control", parts: []string{"a file field needs a prefix"}},
		{name: "bad prefix", src: form("o8", okFields, ins, ""), bad: form("o8", "  - { field: title, control: text, required: true }\n  - { field: file, control: file, prefix: ../up }\n", ins, ""),
			path: "/fields/1/prefix", parts: []string{"is not a bucket prefix"}},
		{name: "prefix only for files", src: form("o9", okFields, ins, ""), bad: form("o9", "  - { field: title, control: text, required: true, prefix: up/ }\n", ins, ""),
			path: "/fields/0/prefix", parts: []string{"only a file field has a prefix"}},
		{name: "generated", src: form("o10", okFields, ins, ""), bad: form("o10", okFields+"  - { field: doubled, control: number }\n", ins, ""),
			path: "/fields/2/field", parts: []string{"generated"}},
		{name: "pipeline column", src: form("o11", "  - { field: date, control: date, required: true }\n  - { field: price, control: number, required: true }\n", "{ action: insert, table: btc_prices }", ""),
			bad:  form("o11", "  - { field: date, control: date, required: true }\n  - { field: price, control: number, required: true }\n  - { field: _run_id, control: text }\n", "{ action: insert, table: btc_prices }", ""),
			path: "/fields/2/field", parts: []string{"written by pipelines"}},
		{name: "twice", src: form("o12", okFields, ins, ""), bad: form("o12", okFields+"  - { field: Title, control: text }\n", ins, ""),
			path: "/fields/2/field", parts: []string{"fields[0] is Title already"}},
		{name: "select options", src: form("o13", okFields+"  - { field: status, control: select, options: { query: \"SELECT DISTINCT status FROM docs\" } }\n", ins, ""),
			bad: form("o13", okFields+"  - { field: status, control: select }\n", ins, ""), path: "/fields/2/control", parts: []string{"a select needs options"}},
		{name: "update load", src: specViewsForm("edit_keyword"), bad: strings.Replace(specViewsForm("edit_keyword"), "load: \"SELECT keyword FROM watch_keywords WHERE id = :key\"\n", "", 1),
			path: "/submit/action", parts: []string{"an update form needs load"}},
		{name: "load params", src: specViewsForm("edit_keyword"), bad: strings.Replace(specViewsForm("edit_keyword"), "id = :key", "id = :row", 1),
			path: "/load", parts: []string{"load gets only the row's :key, not :row"}},
		{name: "load @key", src: specViewsForm("edit_keyword"), bad: strings.Replace(specViewsForm("edit_keyword"), "id = :key", "id = @key", 1),
			path: "/load", parts: []string{"write :key, not @key"}},
		{name: "update key field", src: specViewsForm("edit_keyword"), bad: strings.Replace(specViewsForm("edit_keyword"), "required: true }\n", "required: true }\n  - { field: id, control: number }\n", 1),
			path: "/fields/1/field", parts: []string{"id is the key of the row this form updates"}},
		{name: "load fields", src: specViewsForm("edit_keyword"), bad: strings.Replace(specViewsForm("edit_keyword"), "SELECT keyword FROM", "SELECT keyword AS kw FROM", 1),
			path: "/fields/0/field", parts: []string{"load doesn't select keyword"}, warn: true},
		{name: "update key", src: specViewsForm("edit_keyword"), bad: strings.Replace(specViewsForm("edit_keyword"), "key: id }", "key: keyword }", 1),
			path: "/submit/key", parts: []string{"keyword is not the key of watch_keywords"}},
		{name: "insert key", src: form("o14", okFields, ins, ""), bad: form("o14", okFields, "{ action: update, table: docs }", "load: SELECT title, file FROM docs WHERE id = :key\n"),
			path: "/submit/key", parts: []string{"is required"}},
		{name: "default", src: form("o15", okFields+"  - { field: read, control: checkbox, default: false }\n", ins, ""),
			bad: form("o15", okFields+"  - { field: read, control: checkbox, default: 0 }\n", ins, ""), path: "/fields/2/default", parts: []string{"true or false"}},
	} {
		c.run(t)
	}
}

func specViewsForm(id string) string { return extraViews[id] }

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
