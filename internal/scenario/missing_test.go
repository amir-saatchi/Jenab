package scenario

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The missing-features set's answer checks find what they look for and
// nothing else, with ' or ’ in them (TASK-014, P2-05).
func TestMissingFeaturesMatches(t *testing.T) {
	s, err := Load(filepath.Join("..", "..", "scenarios", "missing-features"))
	if err != nil {
		t.Fatal(err)
	}
	match := func(id, test string) *regexp.Regexp {
		for _, sc := range s.Scenarios {
			for _, a := range sc.Asserts {
				if sc.ID == id && a.Test == test && a.Match != "" {
					return regexp.MustCompile(a.Match)
				}
			}
		}
		t.Fatalf("%s has no %s match", id, test)
		return nil
	}
	for _, c := range []struct {
		id, test string
		hits     []string // the match finds these
		misses   []string // and not these
	}{
		{"F1_greeting", "offer", []string{
			"I can also help you build a dashboard.",
			"Let me make a daily pipeline for that.",
			"We can design views of your trades.",
		}, []string{
			// Tables can be made since P2-06.
			"I can create tables for your prices.",
			"I’ll set up a btc_prices table.",
			"I can read and fill tables, but I can't build pipelines or views yet.",
			"I can query your tables and insert rows into them.",
			"I can't set up pipelines, views or dashboards yet.",
		}},
	} {
		re := match(c.id, c.test)
		for _, a := range c.hits {
			if !re.MatchString(a) {
				t.Errorf("%s %s misses %q", c.id, c.test, a)
			}
		}
		for _, a := range c.misses {
			if m := re.FindString(a); m != "" {
				t.Errorf("%s %s finds %q in %q", c.id, c.test, m, a)
			}
		}
	}
}

// F2 passes on an applied change whose tables have the five columns, in
// any order, and fails on a rejected one, a missing column, or columns
// split over two calls, two tables, or the key and a foreign key (P2-06).
func TestMissingFeaturesTable(t *testing.T) {
	s, err := Load(filepath.Join("..", "..", "scenarios", "missing-features"))
	if err != nil {
		t.Fatal(err)
	}
	var sc *Scenario
	for _, x := range s.Scenarios {
		if x.ID == "F2_table" {
			sc = x
		}
	}
	if sc == nil {
		t.Fatal("no F2_table")
	}
	applied := "[request_schema_change: applied — ref: subagents/c/x.json]\nMigration m applied; the schema version is 1.\nPlan: a table with the date, open and close.\nSteps:\n1. create_table btc_prices\nTables now:\n"
	table := "- btc_prices (0 rows): date TEXT NOT NULL, close REAL, open REAL, high REAL, low REAL; key (date)"
	for _, c := range []struct {
		name    string
		results []string          // one call each
		want    map[string]string // by test
	}{
		{"applied", []string{applied + table}, map[string]string{"made": Pass, "columns": Pass}},
		{"no low", []string{applied + strings.Replace(table, ", low REAL", "", 1)}, map[string]string{"made": Pass, "columns": Fail}},
		{"rejected", []string{"[request_schema_change: rejected — ref: subagents/c/x.json]\nNothing changed. Reason: no.\n" + table},
			map[string]string{"made": Fail, "columns": Fail}},
		{"two calls", []string{applied + "- btc_prices (0 rows): date TEXT NOT NULL, close REAL, open REAL; key (date)",
			applied + "- btc_extra (0 rows): date TEXT NOT NULL, high REAL, low REAL; key (date)"}, map[string]string{"made": Pass, "columns": Fail}},
		{"two tables", []string{applied + "- btc_prices (0 rows): date TEXT NOT NULL, close REAL, open REAL; key (date)\n- btc_extra (0 rows): date TEXT NOT NULL, high REAL, low REAL; key (date)"},
			map[string]string{"made": Pass, "columns": Fail}},
		{"keys", []string{applied + "- btc_prices (0 rows): day TEXT NOT NULL, open REAL, high REAL, low REAL; key (day, close); (date) references days (date, close)"},
			map[string]string{"made": Pass, "columns": Fail}},
		{"case and CRLF", []string{strings.ReplaceAll(applied+"- BTC (1 row): Date TEXT, Close REAL, Open REAL, High REAL, Low REAL; key (Date)", "\n", "\r\n")},
			map[string]string{"made": Pass, "columns": Pass}},
	} {
		rec := &record{sc: sc}
		for i, r := range c.results {
			callID := fmt.Sprintf("c%d", i+1)
			rec.events = append(rec.events,
				event{kind: evCall, turn: 1, tool: "request_schema_change", args: `{"goal": "a table"}`, callID: callID},
				event{kind: evResult, turn: 1, tool: "request_schema_change", callID: callID, text: r})
		}
		got := map[string]string{}
		for _, a := range sc.Asserts {
			if _, ok := c.want[a.Test]; !ok {
				continue
			}
			if st, _, _ := rec.check(a); got[a.Test] == "" || st == Fail {
				got[a.Test] = st
			}
		}
		for test, want := range c.want {
			if got[test] != want {
				t.Errorf("%s: %s %s, want %s", c.name, test, got[test], want)
			}
		}
	}
}
