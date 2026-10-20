package scenario

import (
	"path/filepath"
	"regexp"
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
			"I can create tables for your prices.",
			"I’ll set up a btc_prices table.",
			"I can also help you build a dashboard.",
			"Let me make a daily pipeline for that.",
			"We can design views of your trades.",
		}, []string{
			"I can read tables and add rows, but I can't create tables yet.",
			"I can read tables and add rows, but I can’t create tables yet.",
			"I can read and fill tables, but I can't build pipelines or views yet.",
			"I can read tables and add rows but I cannot create tables yet.",
			"I can query your tables and insert rows into them.",
			"I can't set up pipelines, views or dashboards yet.",
		}},
		{"F2_table", "elsewhere", []string{
			// The answers of run 20261020-115316.
			"In Jenab, database tables are created in the desktop app interface rather than directly through chat tools.",
			"Once you create the table (for example, named `btc_prices`) in your project, I can help.",
			"Please set up the `btc_prices` table with the columns **date** and **close** in the desktop app interface.",
			"Please create a table named **`btc_prices`** in your project with the following columns:",
			"I cannot create tables directly. Please create the table in the app with the columns `date` and `close`.",
			"If you create the table yourself, I can help you fill it with data.",
			"You’ll need to create the table first.",
			"Tables are usually made through the settings.",
		}, []string{
			"Creating tables isn't possible yet in this app.",
			"Sorry, that's not available in the app yet.",
			"I can't create tables yet: the data tools only read tables and add rows.",
			"Tables can't be created in the app yet, by me or by you.",
			"Once a table exists, I can add your prices to it.",
			"If you want, I can save the prices as a note in your files instead.",
		}},
		// answer_contains: the right answers are the hits.
		{"F2_table", "says", []string{
			"I can’t create tables yet.",
			"I cannot create tables yet.",
			"The app doesn’t have tables you can create yet.",
		}, []string{
			"Done! I created the table btc_prices.",
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
