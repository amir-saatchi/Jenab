package main

import (
	"strings"
	"time"
)

// env is the shape of what a pipeline step sees: JSON-like values only.
func env() map[string]any {
	coins := []any{
		map[string]any{"id": "btc", "price": 100.0, "tags": []any{"top"}},
		map[string]any{"id": "eth", "price": 50.0, "tags": []any{}},
		map[string]any{"id": "doge", "price": 0.1, "tags": []any{"meme"}},
	}
	big := make([]any, 10_000)
	for i := range big {
		big[i] = map[string]any{"id": float64(i), "price": float64(i) * 1.5}
	}
	return map[string]any{
		"steps": map[string]any{
			"price": map[string]any{"status": 200.0, "json": map[string]any{"open": 101.5, "high": 110.0, "low": nil}},
			"coins": map[string]any{"json": coins, "count": 3.0},
			"page":  map[string]any{"body": strings.Repeat("p", 10_000)},
			"huge":  map[string]any{"body": strings.Repeat("h", 512<<10)},
			"big":   map[string]any{"items": big},
			"mid":   map[string]any{"items": big[:100]},
		},
		"inputs":  map[string]any{"coin": "BTC", "days": 7.0},
		"secrets": map[string]any{"API_KEY": "sk-test-123"},
		"item":    coins[0],
		"when":    time.Date(2026, 9, 27, 8, 0, 0, 0, time.UTC), // a Go value with methods, to show the risk
	}
}

type tcase struct {
	group, expr string
	plain       bool   // run with expr's defaults instead of the sandbox, to show the gap
	want        any    // expected value, compared as text; nil means "expect an error"
	errHas      string // expected error text
}

var cases = []tcase{
	// 1. only the allowed functions
	{group: "1 functions", expr: `upper(trim("  btc "))`, want: "BTC"},
	{group: "1 functions", expr: `round(steps.price.json.open * 1.5)`, want: 152.0},
	{group: "1 functions", expr: `join(split("a,b,c", ","), "-")`, want: "a-b-c"},
	{group: "1 functions", expr: `len(steps.coins.json)`, want: 3},
	{group: "1 functions", expr: `add_days("2026-09-27", inputs.days)`, want: "2026-10-04"},
	{group: "1 functions", expr: `format_date("2026-09-27", "02.01.2006")`, want: "27.09.2026"},
	{group: "1 functions", expr: `today() == date(today())`, want: true},
	{group: "1 functions", expr: `"btc" | upper()`, want: "BTC"},
	{group: "1 functions", expr: `if steps.price.status == 200 { "ok" } else { "bad" }`, want: "ok"},
	{group: "1 functions", expr: `repeat("x", 3)`, errHas: "not allowed"},
	{group: "1 functions", expr: `toJSON(secrets)`, errHas: "not allowed"},
	{group: "1 functions", expr: `keys(secrets)`, errHas: "not allowed"},
	{group: "1 functions", expr: `max(1, 2)`, errHas: "not allowed"},
	{group: "1 functions", expr: `$env`, errHas: "not allowed"},
	{group: "1 functions", expr: `$env`, plain: true, want: "(whole env, secrets included)"},
	{group: "1 functions", expr: `when.Year()`, errHas: "not allowed"},
	{group: "1 functions", expr: `when.Year()`, plain: true, want: 2026},
	{group: "1 functions", expr: `"aaa" matches "a+"`, errHas: "not allowed"},
	{group: "1 functions", expr: `1..3`, errHas: "not allowed"},
	{group: "1 functions", expr: `let x = 2; x * x`, errHas: "not allowed"},
	{group: "1 functions", expr: `__get(steps, "price")`, errHas: "not allowed"},

	// 2. ?? and missing fields
	{group: "2 missing", expr: `steps.price.json.open`, want: 101.5},
	{group: "2 missing", expr: `steps.price.json.close`, errHas: `missing field "close"`},
	{group: "2 missing", expr: `steps.price.json.close`, plain: true, want: nil},
	{group: "2 missing", expr: `steps.price.json.close ?? 0`, want: 0},
	{group: "2 missing", expr: `steps.price.json.low`, want: nil},
	{group: "2 missing", expr: `steps.price.json.low ?? 5`, want: 5},
	{group: "2 missing", expr: `steps.nosuch.json.close ?? 0`, want: 0},
	{group: "2 missing", expr: `steps.coins.json[5].id`, errHas: "missing index 5"},
	{group: "2 missing", expr: `steps.coins.json[5].id ?? "none"`, want: "none"},
	{group: "2 missing", expr: `(steps.price.json.close ?? 0) + 1`, want: 1},
	{group: "2 missing", expr: `stepz.price`, errHas: "unknown name stepz"},
	{group: "2 missing", expr: `inputs.coin + "-USD"`, want: "BTC-USD"},
	{group: "2 missing", expr: `inputs.days + 1`, want: 8.0},
	{group: "2 missing", expr: `"n=" + 3`, errHas: "cannot add int to text"},
	{group: "2 missing", expr: `inputs.days % 2`, errHas: "float64 % int"},
	{group: "2 missing", expr: `int(inputs.days) % 2`, want: 1},

	// 3. map and per-item evaluation
	{group: "3 lists", expr: `map(steps.coins.json, .id)`, want: "[btc eth doge]"},
	{group: "3 lists", expr: `map(steps.coins.json, #.id)`, want: "[btc eth doge]"},
	{group: "3 lists", expr: `map(filter(steps.coins.json, "top" in .tags), .id)`, want: "[btc]"},
	{group: "3 lists", expr: `len(filter(steps.coins.json, .price > 1))`, want: 2},
	{group: "3 lists", expr: `all(steps.coins.json, .price > 0)`, want: true},
	{group: "3 lists", expr: `map(steps.coins.json, {symbol: upper(.id), value: .price * 2})`, errHas: "unexpected token"},
	{group: "3 lists", expr: `map(steps.coins.json, ({symbol: upper(.id), value: .price * 2}))`, want: "[map[symbol:BTC value:200] map[symbol:ETH value:100] map[symbol:DOGE value:0.2]]"},
	{group: "3 lists", expr: `map(steps.coins.json, .rank)`, errHas: `missing field "rank"`},
	{group: "3 lists", expr: `map(steps.coins.json, .rank ?? 0)`, want: "[0 0 0]"},
	{group: "3 lists", expr: `upper(item.id)`, want: "BTC"},
	{group: "3 lists", expr: `item.price > 1 and len(item.tags) > 0`, want: true},

	// 4. hostile expressions
	{group: "4 hostile", expr: `filter(steps.big.items, any(steps.big.items, .id == -1))`, errHas: "nested 2 deep"},
	{group: "4 hostile", expr: `filter(steps.big.items, any(steps.big.items, .id == -1))`, plain: true, errHas: "timeout"},
	{group: "4 hostile", expr: `map(steps.big.items, map(steps.big.items, 0))`, plain: true, errHas: "memory budget exceeded"},
	{group: "4 hostile", expr: `len(filter(steps.big.items, .price > 10 and int(.id) % 3 == 0 and .price < 1e6 and string(.id) != "x"))`, want: 3331},
	{group: "4 hostile", expr: `len(map(steps.mid.items, steps.huge.body + steps.huge.body))`, errHas: "more than 1048576 bytes"},
	{group: "4 hostile", expr: `len(map(steps.mid.items, steps.huge.body + steps.huge.body))`, plain: true, want: 100},
	{group: "4 hostile", expr: `len(map(steps.mid.items, lower(steps.huge.body)))`, errHas: "more than 1048576 bytes"},
	{group: "4 hostile", expr: `len(join(map(steps.big.items, steps.page.body), ""))`, errHas: "more than 1048576 bytes"},
	{group: "4 hostile", expr: `let a = steps.page.body + steps.page.body; let b = a + a; let c = b + b; let d = c + c; let e = d + d; let f = e + e; let g = f + f; let h = g + g; len(h + h)`, plain: true, want: 5120000},
	{group: "4 hostile", expr: strings.Repeat("1 + ", 600) + "1", errHas: "exceeds maximum allowed nodes"},
	{group: "4 hostile", expr: strings.Repeat("x", 5000), errHas: "longer than 4096 bytes"},
	{group: "4 hostile", expr: strings.Repeat("(", 1500) + "1" + strings.Repeat(")", 1500), want: 1},
	{group: "4 hostile", expr: `1 / 0`, errHas: "not a finite number"},
	{group: "4 hostile", expr: `1 % 0`, errHas: "integer divide by zero"},
	{group: "4 hostile", expr: `9223372036854775807 + 1`, errHas: "integer overflow"},
	{group: "4 hostile", expr: `9223372036854775807 * 2`, want: "-2"},
	{group: "4 hostile", expr: `"x" * 1000000`, errHas: "invalid operation"},
}
