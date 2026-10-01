# SPIKE-005 — expr-lang sandbox
**Type:** Spike
**Status:** Done
**Gate:** 2

## Question
Can `expr-lang/expr` give us the expression rules in SPEC 6.4?
1. Only allowed functions available; other builtins disabled
2. `??` defaults, and an error on missing fields
3. `map(list, .field)` and per-item evaluation with `item`
4. Limits on run time and memory for hostile expressions

## Done when
Tests cover each rule, and any gaps have a workaround.

## Result
Run on 2026-09-27 with expr-lang/expr v1.17.8 and Go 1.26.0 on Windows 11.
- **Setup:** a candidate sandbox (`sandbox.go`) wraps expr. Some cases also run with expr's defaults, to show what the sandbox adds.
- **Code and full output:** [spikes/005-expr-sandbox](../../spikes/005-expr-sandbox/) (`results.md`).
- **Outcome:** all 65 cases pass.

| Rule | expr on its own | What the sandbox adds | Result |
|---|---|---|---|
| 1. Only allowed functions | `DisableAllBuiltins` + `EnableBuiltin` work. But `$env` returns the whole environment, **secrets included**, and methods on Go values can be called (`when.Year()`). | A check on the parsed tree allows only listed node types, operators and names. It rejects `$env`, method calls, `let`, `matches`, `..` and names starting with `__`. | yes |
| 2. `??` and missing fields | A missing field returns `nil` **without an error**. | Every `a.b` is rewritten into a strict lookup that fails with `missing field "b"`. On the left of `??`, the whole chain becomes lenient. Index out of range is handled the same way. | yes |
| 3. `map(list, .field)` and `item` | `map(list, .id)` and `#.id` work. Per-item: compile once, run per item. | Missing fields inside predicates are also errors. | yes |
| 4. Run time | **No cancellation.** A nested `filter(big, any(big, …))` on 10,000 items ran 11.9 s; the watchdog returned after 100 ms, but the goroutine kept running. | Predicates may not nest list functions (depth 1), so cost stays linear. A heavy allowed filter on 10,000 items took 3 ms. | yes |
| 4. Memory | The memory budget counts list elements only. `map(100 items, huge + huge)` built **103 MB** of text without an error, and `let` doubling grows text exponentially. | `+` and all text functions (`lower`, `upper`, `trim`, `split`, `join`, `string`) share a 1 MB text budget per evaluation. Also: 4 KB source limit, 500 AST nodes. | yes |

**Findings**
- **Per-item cost is small:** 0.5–1.2 µs per item in the sandbox, against 0.1–0.6 µs with expr's defaults. 10,000 items take 5–12 ms.
- **JSON numbers are float64.** So `inputs.days % 2` fails with `float64 % int`, and `int(inputs.days) % 2` works.
- **Integer overflow wraps silently.** The sandbox's `+` now reports it; `9223372036854775807 * 2` still gives `-2`.
- **Gotcha:** a map literal as a predicate body, `map(list, {k: v})`, is read as a closure block and fails to parse. It needs parentheses: `map(list, ({k: v}))`.
- **`if … { } else { }` and pipes (`"btc" | upper()`) work** and are harmless.
- **Our own functions:** `date` and `now` exist as expr builtins, but ours replace them so the results use the pipeline timezone and fixed formats.

**Limits:** the whitelist and the rewrite rely on expr's AST types, so an expr upgrade must re-run these cases.

## Decision
**Decided (2026-09-27):** use `expr-lang/expr` behind a Jenab sandbox with these rules, added to SPEC 6.4:
- **Only listed syntax:**
  - allowed: literals, field and index access, arithmetic, comparisons, `and`/`or`/`not`, `in`, `contains`/`startsWith`/`endsWith`, `??`, `? :` and `if/else`, list and map literals, pipes
  - not allowed: `let`, `matches`, `..`, `$env`, method calls
- **Strict fields:** a missing field or index is an error, except on the left of `??`.
- **Limits per evaluation:**
  - source at most 4 KB
  - at most 500 AST nodes
  - list functions may not nest inside a predicate
  - at most 1 MB of text built
  - a finite number as the result
- **Timeouts:** whole-list expressions get a 1 s watchdog. Per-item expressions run inside the step timeout. expr cannot be cancelled, so the nesting and size limits are what bound the cost.
- **Environment:** only JSON-like values (maps, lists, text, numbers, booleans, null), never Go structs. Secrets are added only for fields where 6.7 allows them.
- **Numbers:** whole JSON numbers that fit in int64 are decoded as integers. Then `%` works and SQLite stores them as `INTEGER`. Integer `+`, `-` and `*` check for overflow.
- **Map literal in a predicate:** needs parentheses, `map(list, ({k: v}))`. This goes in the agent's expression guide.
