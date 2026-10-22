package scenario

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// A schema agent that fails its first try and then keeps btc_prices: the
// checks on the database and the card fail, and the tries count.
func TestSchemaChecksFail(t *testing.T) {
	s, err := Load(filepath.Join("..", "..", "scenarios", "schema-widen"))
	if err != nil {
		t.Fatal(err)
	}
	prices := map[string]any{"op": "create_table", "table": "prices", "primary_key": []any{"coin", "date"}, "columns": []any{
		map[string]any{"name": "coin", "type": "TEXT"}, map[string]any{"name": "date", "type": "TEXT"}, map[string]any{"name": "price", "type": "REAL"}}}
	sc := &Script{Name: "bad", Scenarios: map[string][]ScriptReply{"T4_track_ethereum": {
		{Calls: []ScriptCall{{Name: "request_schema_change", Args: map[string]any{"goal": "Track Ethereum too."}}}},
		{Calls: []ScriptCall{{Name: "apply_migration", Args: map[string]any{"plan": "Add prices.", "steps": []any{
			map[string]any{"op": "create_table", "table": "prices", "columns": []any{map[string]any{"name": "coin", "type": "TEXT"}}}}}}}},
		{Calls: []ScriptCall{{Name: "apply_migration", Args: map[string]any{"plan": "Add prices.", "steps": []any{prices,
			map[string]any{"op": "copy_data", "into": "prices", "columns": []any{"coin", "date", "price"}, "from": "SELECT 'BTC', date, price FROM btc_prices"}}}}}},
		{Text: "Done."},
	}}}
	rep, err := RunSet(context.Background(), s, Options{
		Models: ScriptedModels(sc), Script: sc, Scale: 0.01, Timeout: time.Minute, Temp: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	r := rep.Runs[0]
	if r.Error != "" || r.Passed() {
		t.Fatalf("passed %v, error %q", r.Passed(), r.Error)
	}
	if r.SubRequests != 2 {
		t.Errorf("subagent requests %d, want 2", r.SubRequests)
	}
	want := map[string]string{
		"SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = 'btc_prices'": Fail, // kept
		"SELECT count(*) FROM prices": Pass, // the 10 rows
		"SELECT count(*), sum(price), count(DISTINCT _run_id), min(_fetched_at), max(_fetched_at) FROM prices WHERE coin = 'BTC' AND _run_id LIKE 'run-%'": Fail, // no run ids
		"SELECT count(*) FROM watch_keywords": Pass,
	}
	for i, a := range r.Asserts {
		as := s.Scenarios[0].Asserts[i]
		switch {
		case a.Type == "db_query" && want[as.Query] != "":
			if a.Status != want[as.Query] {
				t.Errorf("%s: %s (%s), want %s", as.Query, a.Status, a.Why, want[as.Query])
			}
		case a.Type == "max_calls" && as.Agent != "":
			if a.Value != 2 {
				t.Errorf("tries %v, want 2", a.Value)
			}
		case a.Type == "approval_shown":
			if a.Status != Fail || a.Why != "no approval card" {
				t.Errorf("approval_shown %s: %s", a.Status, a.Why)
			}
		case a.Type == "tool_called":
			if a.Status != Pass {
				t.Errorf("tool_called %s: %s", a.Status, a.Why)
			}
		}
	}
}
