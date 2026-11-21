package view

import (
	"context"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/store"
)

// SPEC 9.3's six views and 5.9's page validate against the Bitcoin
// project (P3-02, done when).
func TestSpecConfigs(t *testing.T) {
	for cid, y := range specViews {
		t.Run(cid, func(t *testing.T) {
			v, res, err := validator().View(context.Background(), []byte(y))
			if err != nil {
				t.Fatal(err)
			}
			wantOK(t, res)
			if v.ID != cid || v.Spec == nil {
				t.Errorf("parsed %+v", v)
			}
		})
	}
	t.Run("bitcoin", func(t *testing.T) {
		p, res, err := validator().Page(context.Background(), []byte(specPage))
		if err != nil {
			t.Fatal(err)
		}
		wantOK(t, res)
		if len(p.Rows) != 3 || p.Rows[0][0].Span != 8 || p.Rows[2][0].Type != "card" {
			t.Fatalf("rows %+v", p.Rows)
		}
		card := p.Rows[2][0]
		if len(card.Rows) != 1 || len(card.Rows[0]) != 2 || card.Rows[0][1].Span != 6 {
			t.Errorf("card rows %+v", card.Rows)
		}
		if vb, _ := card.Rows[0][1].Spec.(*ViewBlock); vb == nil || vb.View != "add_keyword" {
			t.Errorf("card block %+v", card.Rows[0][1].Spec)
		}
	})
	t.Run("config", func(t *testing.T) {
		kind, parsed, res, err := validator().Config(context.Background(), []byte(specPage))
		if err != nil || kind != store.KindPage || !res.OK() {
			t.Fatalf("%v %v %v\n%s", kind, parsed, err, res)
		}
		kind, parsed, res, err = validator().Config(context.Background(), []byte(specViews["btc_today"]))
		if err != nil || kind != store.KindView || !res.OK() {
			t.Fatalf("%v %v %v\n%s", kind, parsed, err, res)
		}
		if s, _ := parsed.(*View).Spec.(*Stat); s == nil || s.Change == nil || s.Change.Format != "percent" {
			t.Errorf("stat %+v", parsed.(*View).Spec)
		}
	})
}

// With its run_pipeline action, 9.3's price table and 5.9's page fail
// until pipelines exist, saying why.
func TestRunPipelineFailsForNow(t *testing.T) {
	res := checkView(t, specViews["btc_price_table"]+`actions:
  - { label: "Refresh now", run_pipeline: daily_btc }
`)
	i := wantErr(t, res, "/actions/0/run_pipeline", "pipelines can't be saved yet", "daily_btc")
	if i.Step != StepReferences || i.Line != 15 {
		t.Errorf("issue %+v", i)
	}
	wantN(t, res, 1)

	page := strings.Replace(specPage, "rows:\n", "actions:\n  - { label: \"Refresh now\", run_pipeline: daily_btc }\nrows:\n", 1)
	res = checkPage(t, page)
	wantErr(t, res, "/actions/0/run_pipeline", "pipelines can't be saved yet")
	wantN(t, res, 1)

	res = checkView(t, `version: 1
id: run_form
title: Run
type: form
fields: [ { field: coin, control: text } ]
submit: { action: run_pipeline, pipeline: daily_btc }
`)
	wantErr(t, res, "/submit/pipeline", "pipelines can't be saved yet")
	wantN(t, res, 1)
}

// One config with many errors returns all of them, each with its path and
// place, in SPEC 10's order (P3-02, done when).
func TestManyErrors(t *testing.T) {
	src := `version: 1
id: many errors
title: ""
type: table
colour: red
query: SELECT date, price AS avg_price FROM btc_prices WHERE date >= ?
params: { coin: BTC }
filters:
  - { param: coin, control: select, label: Coin }
  - { param: day, control: date, default: yesterday }
default_sort: { field: day, direction: down }
columns:
  - { field: date, format: date }
  - { field: price, format: money }
  - { field: avg, format: currency, format_options: { currency: usd } }
rows_from: { table: _jenab_views, key: id }
row_actions:
  - { label: Read, set: { read: true }, delete: true }
actions:
  - { label: Refresh, run_pipeline: daily_btc }
`
	res := checkView(t, src)
	for _, w := range []struct {
		path  string
		parts []string
	}{
		{"/id", []string{"an ID is 1 to 64"}},
		{"/title", []string{"must not be empty"}},
		{"/colour", []string{"unknown field"}},
		{"/default_sort/direction", []string{"asc", "desc"}},
		{"/columns/1/format", []string{"money is not one of text, number"}},
		{"/columns/2/format_options/currency", []string{"not a currency code"}},
		{"/filters/0/param", []string{"in params too"}},
		{"/filters/0/control", []string{"a select needs options"}},
		{"/filters/1/default", []string{"is not a date"}},
		{"/row_actions/0", []string{"does one thing", "set and delete"}},
		{"/actions/0/run_pipeline", []string{"pipelines can't be saved yet"}},
		{"/rows_from/table", []string{"Jenab's own tables"}},
		{"/query", []string{"named parameters"}},
		{"/columns/1/field", []string{"'price' is not a result column", "it has date, avg_price"}},
		{"/default_sort/field", []string{"'day' is not a result column"}},
	} {
		wantErr(t, res, w.path, w.parts...)
	}
	// avg is a near miss of avg_price.
	wantErr(t, res, "/columns/2/field", "did you mean avg_price?")
	// Every issue has its place, and they come in step order.
	last := Step(0)
	for _, i := range res.Issues {
		if i.Line == 0 {
			t.Errorf("no line: %v", i)
		}
		if i.Step < last {
			t.Errorf("out of order: %v", i)
		}
		last = i.Step
	}
	t.Logf("\n%s", res)
}

// The message form of SPEC 10: line 4:13 columns[2].field: ...
func TestMessageForm(t *testing.T) {
	res := checkView(t, `version: 1
id: msg_form
title: Prices
type: table
query: SELECT date, price FROM btc_prices
default_sort: { field: date, direction: desc }
columns:
  - { field: date }
  - { field: price }
  - { field: avg }
`)
	wantN(t, res, 1)
	got := res.Errors()[0].String()
	if wantS := "line 10:14 columns[2].field: 'avg' is not a result column of the query; it has date, price"; got != wantS {
		t.Errorf("got  %s\nwant %s", got, wantS)
	}
	if !strings.HasPrefix(res.String(), "1 error:\n") {
		t.Errorf("result %q", res.String())
	}
}

// Without an Env only the config itself is checked: steps 3 to 5 need a
// project.
func TestNoEnv(t *testing.T) {
	v := &Validator{}
	_, res, err := v.View(context.Background(), []byte(specViews["btc_price_table"]+"actions:\n  - { label: Run, run_pipeline: daily_btc }\n"))
	if err != nil {
		t.Fatal(err)
	}
	// run_pipeline fails without a project too: no project has pipelines.
	wantErr(t, res, "/actions/0/run_pipeline", "pipelines")
	wantN(t, res, 1)
	_, res, err = v.Page(context.Background(), []byte(specPage))
	if err != nil {
		t.Fatal(err)
	}
	wantOK(t, res)
}
