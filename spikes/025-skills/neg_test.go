package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Broken variants of the reference configs: each must be rejected with the expected category.
func TestNegatives(t *testing.T) {
	initSchemas()
	initTools()
	loadGuides()
	cases := []struct {
		name, tool, args string
		state            int
		want             string // category that must appear
	}{
		{"unknown step", "save_pipeline", strings.Replace(refPipeline, "use: web.search", "use: web.fetch", 1), 3, CatStepType},
		{"unknown field", "save_pipeline", strings.Replace(refPipeline, "      limit: 20", "      max_results: 20", 1), 3, CatUnknown},
		{"bad ref", "save_pipeline", strings.Replace(refPipeline, "steps.top5.items", "steps.top.items", 1), 3, CatRefs},
		{"later ref", "save_pipeline", strings.Replace(refPipeline, "${{ steps.price.json.close }}", "${{ steps.news.results }}", 1), 3, CatRefs},
		{"bad expr", "save_pipeline", strings.Replace(refPipeline, "${{ today() }}", "${{ today( }}", 1), 3, CatExpr},
		{"no $", "save_pipeline", strings.Replace(refPipeline, "${{ item.url }}", "{{ item.url }}", 1), 3, CatYAML},
		{"bad cron", "save_pipeline", strings.Replace(refPipeline, `"0 8 * * *"`, `"0 8 * *"`, 1), 3, CatSchema},
		{"bad sql", "save_pipeline", strings.Replace(refPipeline, "SELECT keyword FROM watch_keywords", "SELECT word FROM watch_keywords", 1), 3, CatSQL},
		{"bad column", "save_pipeline", strings.Replace(refPipeline, "        reason: ${{ item.reason }}", "        why: ${{ item.reason }}", 1), 3, CatRefs},
		{"unknown item field", "save_pipeline", strings.Replace(refPipeline, "${{ item.url }}", "${{ item.link }}", 1), 3, CatRefs},
		{"view field", "save_view", strings.Replace(refViewsT3[1], "field: price,", "field: close,", 1), 3, CatRefs},
		{"view write sql", "save_view", strings.Replace(refViewsT3[1], "query: SELECT date, price FROM btc_prices", "query: DELETE FROM btc_prices", 1), 3, CatSQL},
		{"unbound param", "save_view", strings.Replace(refViewsT3[0], "param: from_date", "param: since", 1), 3, CatSemantic},
		{"form not null", "save_form", strings.Replace(refViewsT3[3], ", required: true", "", 1), 3, CatSemantic},
		{"migration no pk", "apply_migration", strings.Replace(refT1, `,"primary_key":"url"`, "", 1), 1, CatSchema},
		{"migration kind", "apply_migration", strings.Replace(refT1, `"op":"create_table","table":"btc_news"`, `"op":"new_table","table":"btc_news"`, 1), 1, CatStepType},
		{"dependents missing", "apply_migration", `{"steps":[{"op":"drop_table","table":"btc_prices"}]}`, 4, CatDepend},
	}
	for _, c := range cases {
		dir, _ := os.MkdirTemp("", "spike021-neg-")
		p, err := buildState(filepath.Join(dir, "p"), c.state)
		if err != nil {
			t.Fatal(err)
		}
		args := c.args
		if c.tool != "apply_migration" {
			args = yamlJSON(c.args)
		}
		res, wc := p.callTool(c.tool, args)
		found := false
		for _, i := range wc.Issues {
			if i.Cat == c.want && !i.Warn {
				found = true
			}
		}
		if wc.OK || !found {
			t.Errorf("%s: ok=%v, want category %q:\n%s", c.name, wc.OK, c.want, res)
		} else {
			t.Logf("%s:\n%s", c.name, res)
		}
		p.Close()
		os.RemoveAll(dir)
	}
}
