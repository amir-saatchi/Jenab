package tool

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/store"
)

// otherChat is a write from another chat, landing between a tool's
// checks and its write (testBeforeWrite).
func otherChat(t *testing.T, f func()) {
	t.Helper()
	testBeforeWrite = func() {
		testBeforeWrite = nil
		f()
	}
	t.Cleanup(func() { testBeforeWrite = nil })
}

var otherSource = id.SourceOf("message", "other-chat")

// A page saved with a view while the view's delete was being checked
// keeps the view: the check runs in the delete's write.
func TestDeleteRacesSave(t *testing.T) {
	ctx := context.Background()
	ct := newConfigTurn(t)
	db := ct.env.Project.DB
	ct.ok(t, "save_view", cfg(chartYAML))
	otherChat(t, func() {
		if _, err := db.SaveConfig(ctx, store.ConfigSave{Kind: store.KindPage, ID: "bitcoin", Title: "Bitcoin", YAML: pageYAML, Source: otherSource}); err != nil {
			t.Errorf("other chat's save: %v", err)
		}
	})
	msg := ct.fail(t, "delete_config", cid("btc_price_chart"))
	contains(t, msg, "Not deleted: the view btc_price_chart is used by page bitcoin.")
	if _, err := db.Config(ctx, store.KindView, "btc_price_chart"); err != nil {
		t.Fatalf("deleted: %v", err)
	}
	// Without a race the delete goes on once the page is gone.
	if _, err := db.DeleteConfig(ctx, store.ConfigDelete{Kind: store.KindPage, ID: "bitcoin", Source: otherSource}); err != nil {
		t.Fatal(err)
	}
	ct.ok(t, "delete_config", cid("btc_price_chart"))
}

// A save whose view, form or page was deleted after it was validated is
// refused in its write, with validation's message, and counts as a
// failed save.
func TestSaveRacesDelete(t *testing.T) {
	ctx := context.Background()
	ct := newConfigTurn(t)
	db := ct.env.Project.DB
	del := func(kind store.ConfigKind, cid string) func() {
		return func() {
			if _, err := db.DeleteConfig(ctx, store.ConfigDelete{Kind: kind, ID: cid, Source: otherSource}); err != nil {
				t.Errorf("other chat's delete: %v", err)
			}
		}
	}
	for _, y := range []string{chartYAML, editKeywordYAML, keywordListYAML} {
		ct.ok(t, "save_view", cfg(y))
	}

	// view: the page's chart.
	otherChat(t, del(store.KindView, "btc_price_chart"))
	msg := ct.fail(t, "save_page", cfg(pageYAML))
	contains(t, msg, "Not saved: 1 error:\nno view btc_price_chart; save the view first")
	if _, err := db.Config(ctx, store.KindPage, "bitcoin"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("saved: %v", err)
	}
	// validate says the same now.
	r, _ := ct.run(t, "validate", cfg(pageYAML))
	contains(t, r.Text, "no view btc_price_chart; save the view first")

	// open_form: a table's row action.
	otherChat(t, del(store.KindView, "edit_keyword"))
	msg = ct.fail(t, "save_view", cfg(strings.Replace(keywordListYAML, "News keywords", "Keywords", 1)))
	contains(t, msg, "Not saved: 1 error:\nno form edit_keyword; save the view first")
	if c, _ := db.Config(ctx, store.KindView, "keyword_list"); c.Title != "News keywords" {
		t.Errorf("saved: %q", c.Title)
	}

	// open_page: a page's button.
	ct.ok(t, "save_view", cfg(chartYAML))
	ct.ok(t, "save_page", cfg(pageYAML))
	other := "version: 1\nid: home\ntitle: Home\nrows:\n  - [ { button: \"Bitcoin\", open_page: bitcoin } ]\n"
	otherChat(t, del(store.KindPage, "bitcoin"))
	msg = ct.fail(t, "save_page", cfg(other))
	contains(t, msg, "Not saved: 1 error:\nno page bitcoin")

	// It counts toward the repair limit and the early-stop notice.
	k, s := ct.answer()
	if k != chat.NoticeEarlyStop || !strings.Contains(s, "save_page home failed validation (1 error)") {
		t.Errorf("notice: %s %q", k, s)
	}
}

// A form opened only by a page's button can't be deleted.
func TestDeleteFormOfPageButton(t *testing.T) {
	ctx := context.Background()
	ct := newConfigTurn(t)
	ct.ok(t, "save_view", cfg(editKeywordYAML))
	ct.ok(t, "save_page", cfg("version: 1\nid: keywords\ntitle: Keywords\nrows:\n  - [ { button: \"Add keyword\", open_form: edit_keyword } ]\n"))
	msg := ct.fail(t, "delete_config", cid("edit_keyword"))
	contains(t, msg, "Not deleted: the view edit_keyword is used by page keywords.")
	if _, err := ct.env.Project.DB.Config(ctx, store.KindView, "edit_keyword"); err != nil {
		t.Fatalf("deleted: %v", err)
	}
}

// save_view and save_page take the revision read_config showed and
// refuse a config changed or deleted since.
func TestSaveRevision(t *testing.T) {
	ctx := context.Background()
	ct := newConfigTurn(t)
	db := ct.env.Project.DB
	ct.ok(t, "save_view", cfg(chartYAML))
	r := ct.ok(t, "read_config", cid("btc_price_chart"))
	contains(t, r.Text, "revision 1")
	retitled := func(s string) string { return strings.Replace(chartYAML, "Bitcoin price chart", s, 1) }

	r = ct.ok(t, "save_view", map[string]any{"config": retitled("BTC"), "revision": 1})
	contains(t, r.Text, "Saved view btc_price_chart, revision 2.")
	// Another chat saves revision 3; a save from revision 2 is refused.
	if _, err := db.SaveConfig(ctx, store.ConfigSave{Kind: store.KindView, ID: "btc_price_chart", Type: "chart", Title: "Other", YAML: retitled("Other"), Source: otherSource}); err != nil {
		t.Fatal(err)
	}
	msg := ct.fail(t, "save_view", map[string]any{"config": retitled("Mine"), "revision": 2})
	contains(t, msg, "changed after you read revision 2; it is at revision 3 now. Read it again with read_config")
	if c, _ := db.Config(ctx, store.KindView, "btc_price_chart"); c.Title != "Other" {
		t.Errorf("overwritten: %q", c.Title)
	}
	// Without a revision it saves over whatever is there.
	ct.ok(t, "save_view", cfg(retitled("Mine")))
	// Deleted since.
	ct.ok(t, "delete_config", cid("btc_price_chart"))
	msg = ct.fail(t, "save_view", map[string]any{"config": chartYAML, "revision": 4})
	contains(t, msg, "was deleted after you read revision 4")
	msg = ct.fail(t, "save_view", map[string]any{"config": chartYAML, "revision": -1})
	contains(t, msg, "revision is the number read_config showed")
	// A page too.
	ct.ok(t, "save_view", cfg(chartYAML))
	ct.ok(t, "save_page", cfg(pageYAML))
	msg = ct.fail(t, "save_page", map[string]any{"config": pageYAML, "revision": 7})
	contains(t, msg, "the page bitcoin changed after you read revision 7; it is at revision 1 now")
	// A refused revision isn't a failed save: no early-stop notice.
	if _, s := ct.answer(); s != "" {
		t.Errorf("notice: %q", s)
	}
}

// The repair count's key is the id of the parsed YAML, whatever its
// style; the id line only when it doesn't parse.
func TestRepairKeyParsed(t *testing.T) {
	for src, want := range map[string]string{
		"id: daily_btc\n":                   "daily_btc",
		"\"id\": daily_btc\n":               "daily_btc",
		"'id' : \"daily_btc\"\n":            "daily_btc",
		"{version: 1, id: daily_btc}\n":     "daily_btc",
		"version: 1\nid:   Daily_BTC # x\n": "Daily_BTC",
		"id: daily_btc\nrows: [\n":          "daily_btc",
		"title: x\n":                        "(no id)",
		"id: [a, b]\n":                      "(no id)",
	} {
		if got := yamlID(src); got != want {
			t.Errorf("yamlID(%q) = %q, want %q", src, got, want)
		}
	}
	// Changing the id's style doesn't start the count again.
	ct := newConfigTurn(t)
	styles := []string{badChart,
		strings.Replace(badChart, "id: daily_btc", `"id": daily_btc`, 1),
		strings.Replace(badChart, "id: daily_btc", `id : "daily_btc"`, 1),
		strings.Replace(badChart, "id: daily_btc", "'id': 'daily_btc'", 1),
	}
	for i := range RepairAttempts + 1 {
		ct.fail(t, "save_view", cfg(styles[i%len(styles)]))
	}
	msg := ct.fail(t, "save_view", cfg(styles[1]))
	contains(t, msg, "is the limit; don't try again")
}

// Each subagent run counts its own repairs; its end clears them.
func TestRepairPerRun(t *testing.T) {
	ct := newConfigTurn(t)
	for range RepairAttempts + 1 {
		ct.fail(t, "save_view", cfg(badChart))
	}
	contains(t, ct.fail(t, "save_view", cfg(badChart)), "is the limit")
	ct.env.Run = "helper:1"
	for i := 1; i <= RepairAttempts+1; i++ {
		msg := ct.fail(t, "save_view", cfg(badChart))
		if strings.Contains(msg, "is the limit") || strings.Contains(msg, "last try") != (i == RepairAttempts+1) {
			t.Fatalf("run try %d: %s", i, msg)
		}
	}
	contains(t, ct.fail(t, "save_view", cfg(badChart)), "is the limit")
	ct.env.Run = "helper:2"
	if msg := ct.fail(t, "save_view", cfg(badChart)); strings.Contains(msg, "limit") {
		t.Fatalf("a new run: %s", msg)
	}
}

// open_page takes one value a filter, fitting its control, 4 KiB in all.
func TestOpenPageValues(t *testing.T) {
	ct := newConfigTurn(t)
	ct.ok(t, "save_view", cfg(`version: 1
id: btc_filtered
title: Filtered prices
type: table
query: >-
  SELECT date, price FROM btc_prices WHERE date >= :from_date AND price >= :min_price
  AND (:only_up = 0 OR price > 0) AND (:coin IS NULL OR :coin = 'btc') AND (:q IS NULL OR date LIKE :q)
params: { from_date: "2026-01-01", min_price: 0, only_up: false, coin: btc, q: "" }
default_sort: { field: date, direction: asc }
columns:
  - { field: date, label: "Date", format: date }
`))
	ct.ok(t, "save_page", cfg(`version: 1
id: bitcoin
title: Bitcoin
filters:
  - { param: from_date, control: date, default: "today-90d" }
  - { param: min_price, control: number, default: 0 }
  - { param: only_up, control: checkbox, default: false }
  - { param: coin, control: select, options: [btc, eth] }
  - { param: q, control: text }
rows:
  - [ { view: btc_filtered } ]
`))
	open := func(k string, v any) (Result, error) {
		return ct.run(t, "open_page", map[string]any{"id": "bitcoin", "filters": map[string]any{k: v}})
	}
	for _, ok := range []struct {
		k string
		v any
	}{
		{"from_date", "2026-01-01"}, {"from_date", "today-30d"}, {"from_date", "start_of_month"}, {"from_date", nil},
		{"min_price", 90000}, {"only_up", true}, {"coin", "btc"}, {"coin", 2}, {"q", "halving"}, {"q", 42},
	} {
		if _, err := open(ok.k, ok.v); err != nil {
			t.Errorf("%s=%v: %v", ok.k, ok.v, err)
		}
	}
	for _, bad := range []struct {
		k    string
		v    any
		want string
	}{
		{"from_date", "last week", `The filter from_date of bitcoin is a date: give it as YYYY-MM-DD or as today, today-30d`},
		{"from_date", 2026, "The filter from_date of bitcoin is a date, not a number."},
		{"min_price", "90000", `The filter min_price of bitcoin is a number: give it as a JSON number, not "90000".`},
		{"only_up", "yes", "The filter only_up of bitcoin is a checkbox: give true or false"},
		{"only_up", 1, "is a checkbox, not a number"},
		{"coin", true, "The filter coin of bitcoin is a select, not true or false."},
		{"q", []any{"a", "b"}, "The filter q of bitcoin takes one value: a string, a number, true, false or null, not a list or an object."},
		{"q", map[string]any{"a": 1}, "not a list or an object"},
		{"q", strings.Repeat("x", 5000), "The filter values are 5008 bytes, more than 4096; give shorter values."},
	} {
		_, err := open(bad.k, bad.v)
		var te *Error
		if !errors.As(err, &te) || !strings.Contains(te.Msg, bad.want) {
			t.Errorf("%s=%v: got %v, want %q", bad.k, bad.v, err, bad.want)
		}
	}
}

// The chip's kind says what was done; the text is for people.
func TestConfigChipKinds(t *testing.T) {
	ct := newConfigTurn(t)
	ct.ok(t, "save_view", cfg(chartYAML))
	r := ct.ok(t, "save_page", cfg(pageYAML))
	if n := r.Notices[0]; n.Kind != chat.NoticeConfigSaved || n.Target != "page:bitcoin" {
		t.Errorf("save: %+v", n)
	}
	r = ct.ok(t, "delete_config", cid("bitcoin"))
	if n := r.Notices[0]; n.Kind != chat.NoticeConfigDeleted || n.Target != "page:bitcoin" || n.Text != "Page deleted: bitcoin" {
		t.Errorf("delete: %+v", n)
	}
}
