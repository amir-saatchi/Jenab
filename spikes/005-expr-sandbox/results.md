# SPIKE-005 results

expr-lang/expr v1.17.8, Go go1.26.0, 2026-09-27.

"Sandbox" is the candidate in `sandbox.go`. "expr defaults" is `expr.Compile(src, expr.Env(env))` with nothing else, to show what the sandbox adds.

| # | Group | Expression | Engine | Expected | Got | Time | Allocated | Pass |
|---|---|---|---|---|---|---|---|---|
| 1 | 1 functions | `upper(trim("  btc "))` | sandbox | BTC | BTC | 270 µs | 30 KB | yes |
| 2 | 1 functions | `round(steps.price.json.open * 1.5)` | sandbox | 152 | 152 | 212 µs | 32 KB | yes |
| 3 | 1 functions | `join(split("a,b,c", ","), "-")` | sandbox | a-b-c | a-b-c | 99 µs | 30 KB | yes |
| 4 | 1 functions | `len(steps.coins.json)` | sandbox | 3 | 3 | 87 µs | 29 KB | yes |
| 5 | 1 functions | `add_days("2026-09-27", inputs.days)` | sandbox | 2026-10-04 | 2026-10-04 | 111 µs | 29 KB | yes |
| 6 | 1 functions | `format_date("2026-09-27", "02.01.2006")` | sandbox | 27.09.2026 | 27.09.2026 | 79 µs | 27 KB | yes |
| 7 | 1 functions | `today() == date(today())` | sandbox | true | true | 268 µs | 29 KB | yes |
| 8 | 1 functions | `"btc" | upper()` | sandbox | BTC | BTC | 128 µs | 27 KB | yes |
| 9 | 1 functions | `if steps.price.status == 200 { "ok" } else { "bad" }` | sandbox | ok | ok | 174 µs | 31 KB | yes |
| 10 | 1 functions | `repeat("x", 3)` | sandbox | error: not allowed | error: function repeat is not allowed | 27 µs | 3 KB | yes |
| 11 | 1 functions | `toJSON(secrets)` | sandbox | error: not allowed | error: function toJSON is not allowed | 31 µs | 3 KB | yes |
| 12 | 1 functions | `keys(secrets)` | sandbox | error: not allowed | error: function keys is not allowed | 40 µs | 3 KB | yes |
| 13 | 1 functions | `max(1, 2)` | sandbox | error: not allowed | error: function max is not allowed | 38 µs | 3 KB | yes |
| 14 | 1 functions | `$env` | sandbox | error: not allowed | error: name "$env" is not allowed | 48 µs | 2 KB | yes |
| 15 | 1 functions | `$env` | expr defaults | (whole env, secrets included) | (whole env, secrets included) | 43 µs | 11 KB | yes |
| 16 | 1 functions | `when.Year()` | sandbox | error: not allowed | error: method calls are not allowed | 10 µs | 1 KB | yes |
| 17 | 1 functions | `when.Year()` | expr defaults | 2026 | 2026 | 444 µs | 46 KB | yes |
| 18 | 1 functions | `"aaa" matches "a+"` | sandbox | error: not allowed | error: operator "matches" is not allowed | 27 µs | 3 KB | yes |
| 19 | 1 functions | `1..3` | sandbox | error: not allowed | error: operator ".." is not allowed | 23 µs | 3 KB | yes |
| 20 | 1 functions | `let x = 2; x * x` | sandbox | error: not allowed | error: *ast.VariableDeclaratorNode is not allowed | 18 µs | 3 KB | yes |
| 21 | 1 functions | `__get(steps, "price")` | sandbox | error: not allowed | error: name "__get" is not allowed | 24 µs | 3 KB | yes |
| 22 | 2 missing | `steps.price.json.open` | sandbox | 101.5 | 101.5 | 81 µs | 30 KB | yes |
| 23 | 2 missing | `steps.price.json.close` | sandbox | error: missing field "close" | error: missing field "close" (1:18) | 173 µs | 31 KB | yes |
| 24 | 2 missing | `steps.price.json.close` | expr defaults | value nil | <nil> | 72 µs | 12 KB | yes |
| 25 | 2 missing | `steps.price.json.close ?? 0` | sandbox | 0 | 0 | 127 µs | 31 KB | yes |
| 26 | 2 missing | `steps.price.json.low` | sandbox | value nil | <nil> | 78 µs | 30 KB | yes |
| 27 | 2 missing | `steps.price.json.low ?? 5` | sandbox | 5 | 5 | 95 µs | 31 KB | yes |
| 28 | 2 missing | `steps.nosuch.json.close ?? 0` | sandbox | 0 | 0 | 132 µs | 31 KB | yes |
| 29 | 2 missing | `steps.coins.json[5].id` | sandbox | error: missing index 5 | error: missing index 5 (list has 3 items) (1:17) | 107 µs | 32 KB | yes |
| 30 | 2 missing | `steps.coins.json[5].id ?? "none"` | sandbox | none | none | 145 µs | 34 KB | yes |
| 31 | 2 missing | `(steps.price.json.close ?? 0) + 1` | sandbox | 1 | 1 | 127 µs | 33 KB | yes |
| 32 | 2 missing | `stepz.price` | sandbox | error: unknown name stepz | error: unknown name stepz (1:1) | 79 µs | 25 KB | yes |
| 33 | 2 missing | `inputs.coin + "-USD"` | sandbox | BTC-USD | BTC-USD | 152 µs | 29 KB | yes |
| 34 | 2 missing | `inputs.days + 1` | sandbox | 8 | 8 | 109 µs | 29 KB | yes |
| 35 | 2 missing | `"n=" + 3` | sandbox | error: cannot add int to text | error: cannot add int to text (1:6) | 145 µs | 27 KB | yes |
| 36 | 2 missing | `inputs.days % 2` | sandbox | error: float64 % int | error: invalid operation: float64 % int (1:13) | 169 µs | 28 KB | yes |
| 37 | 2 missing | `int(inputs.days) % 2` | sandbox | 1 | 1 | 141 µs | 28 KB | yes |
| 38 | 3 lists | `map(steps.coins.json, .id)` | sandbox | [btc eth doge] | [btc eth doge] | 187 µs | 33 KB | yes |
| 39 | 3 lists | `map(steps.coins.json, #.id)` | sandbox | [btc eth doge] | [btc eth doge] | 144 µs | 33 KB | yes |
| 40 | 3 lists | `map(filter(steps.coins.json, "top" in .tags), .id)` | sandbox | [btc] | [btc] | 195 µs | 38 KB | yes |
| 41 | 3 lists | `len(filter(steps.coins.json, .price > 1))` | sandbox | 2 | 2 | 180 µs | 35 KB | yes |
| 42 | 3 lists | `all(steps.coins.json, .price > 0)` | sandbox | true | true | 121 µs | 34 KB | yes |
| 43 | 3 lists | `map(steps.coins.json, {symbol: upper(.id), value: .price * 2})` | sandbox | error: unexpected token | error: unexpected token Operator(":") (1:30) | 35 µs | 4 KB | yes |
| 44 | 3 lists | `map(steps.coins.json, ({symbol: upper(.id), value: .price * 2}))` | sandbox | [map[symbol:BTC value:200] map[symbol:ETH value:100] map[symbol:DOGE value:0.2]] | [map[symbol:BTC value:200] map[symbol:ETH value:100] map[symbol:DOGE value:0.2]] | 140 µs | 41 KB | yes |
| 45 | 3 lists | `map(steps.coins.json, .rank)` | sandbox | error: missing field "rank" | error: missing field "rank" (1:24) | 152 µs | 33 KB | yes |
| 46 | 3 lists | `map(steps.coins.json, .rank ?? 0)` | sandbox | [0 0 0] | [0 0 0] | 123 µs | 34 KB | yes |
| 47 | 3 lists | `upper(item.id)` | sandbox | BTC | BTC | 126 µs | 29 KB | yes |
| 48 | 3 lists | `item.price > 1 and len(item.tags) > 0` | sandbox | true | true | 113 µs | 31 KB | yes |
| 49 | 4 hostile | `filter(steps.big.items, any(steps.big.items, .id == -1))` | sandbox | error: nested 2 deep | error: list functions nested 2 deep; at most 1 allowed | 28 µs | 5 KB | yes |
| 50 | 4 hostile | `filter(steps.big.items, any(steps.big.items, .id == -1))` | expr defaults | error: timeout | error: timeout after 100ms | 100.3 ms | 17 MB | yes |
| 51 | 4 hostile | `map(steps.big.items, map(steps.big.items, 0))` | expr defaults | error: memory budget exceeded | error: memory budget exceeded (1:22) | 21.6 ms | 18 MB | yes |
| 52 | 4 hostile | `len(filter(steps.big.items, .price > 10 and int(.id) % 3 == … (103 bytes)` | sandbox | 3331 | 3331 | 3.2 ms | 1 MB | yes |
| 53 | 4 hostile | `len(map(steps.mid.items, steps.huge.body + steps.huge.body))` | sandbox | error: more than 1048576 bytes | error: expression built more than 1048576 bytes of text (1:42) | 271 µs | 1 MB | yes |
| 54 | 4 hostile | `len(map(steps.mid.items, steps.huge.body + steps.huge.body))` | expr defaults | 100 | 100 | 23.7 ms | 103 MB | yes |
| 55 | 4 hostile | `len(map(steps.mid.items, lower(steps.huge.body)))` | sandbox | error: more than 1048576 bytes | error: expression built more than 1048576 bytes of text (1:26) | 963 µs | 151 KB | yes |
| 56 | 4 hostile | `len(join(map(steps.big.items, steps.page.body), ""))` | sandbox | error: more than 1048576 bytes | error: expression built more than 1048576 bytes of text (1:5) | 1.5 ms | 7 MB | yes |
| 57 | 4 hostile | `let a = steps.page.body + steps.page.body; let b = a + a; le… (158 bytes)` | expr defaults | 5120000 | 5120000 | 3.6 ms | 10 MB | yes |
| 58 | 4 hostile | `1 + 1 + 1 + 1 + 1 + 1 + 1 + 1 + 1 + 1 + 1 + 1 + 1 + 1 + 1 + … (2401 bytes)` | sandbox | error: exceeds maximum allowed nodes | error: compilation failed: expression exceeds maximum allowed nodes (1:1003) | 397 µs | 261 KB | yes |
| 59 | 4 hostile | `xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx… (5000 bytes)` | sandbox | error: longer than 4096 bytes | error: expression longer than 4096 bytes | 8 µs | 2 KB | yes |
| 60 | 4 hostile | `((((((((((((((((((((((((((((((((((((((((((((((((((((((((((((… (3001 bytes)` | sandbox | 1 | 1 | 1.1 ms | 240 KB | yes |
| 61 | 4 hostile | `1 / 0` | sandbox | error: not a finite number | error: result is not a finite number | 66 µs | 30 KB | yes |
| 62 | 4 hostile | `1 % 0` | sandbox | error: integer divide by zero | error: integer divide by zero (1:3) | 87 µs | 37 KB | yes |
| 63 | 4 hostile | `9223372036854775807 + 1` | sandbox | error: integer overflow | error: integer overflow (1:21) | 90 µs | 36 KB | yes |
| 64 | 4 hostile | `9223372036854775807 * 2` | sandbox | -2 | -2 | 100 µs | 38 KB | yes |
| 65 | 4 hostile | `"x" * 1000000` | sandbox | error: invalid operation | error: invalid operation: * (mismatched types string and int) (1:5) | 55 µs | 30 KB | yes |

**65 of 65 cases pass.**

## Per-item evaluation, 10,000 items

Compiled once, run once per item with `item` set. Times are per item (total / 10,000).

| Expression | Sandbox | expr defaults |
|---|---|---|
| `item.price * 2` | 0.49 µs | 0.14 µs |
| `item.price > 100 and int(item.id) % 2 == 0` | 0.48 µs | 0.26 µs |
| `string(item.id) + "-" + string(item.price)` | 1.23 µs | 0.60 µs |

The sandbox adds a function call per field access (strict mode) and a fresh text budget per item.

The nested-list case with expr defaults returned a timeout after 100 ms, but its goroutine kept running for 11.85s in total.
