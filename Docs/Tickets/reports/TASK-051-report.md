# TASK-051 report: `query` points to the project card

Branch `claude/strange-williamson-a99382`. Runs on 2026-11-15 (UTC), Windows 11.

**Status:** the new texts and tests are in, and every check passes. After the change, no run on any of the three reference models tried `sqlite_master` (P2-14: 7 of 9). `describe_table` in turn 5 barely moved: 8 of 9 runs still called it (P2-14: 9 of 9). As the ticket says, I recorded that and added no more prompt text.

## What changed

All in `internal/tool/data.go`. The guard (`internal/sqlguard/`) is unchanged, and so are its decisions.

**`query`'s description.** Old:

> Run one read-only SQLite query (SELECT, WITH or VALUES) on the project's tables. Returns up to 200 rows or 4,000 tokens and the total row count; a larger result comes as a preview and a ref, read on with read_ref or search_ref. Jenab's internal tables can't be queried: use describe_table for a table's columns.

New:

> Run one read-only SQLite query (SELECT, WITH or VALUES) on the project's tables. The project card lists the tables and, for the tables it shows, their columns and keys; use describe_table for a table it names without columns. sqlite_schema and Jenab's own tables can't be queried. Returns up to 200 rows or 4,000 tokens and the total row count; a larger result comes as a preview and a ref, read on with read_ref or search_ref.

The pointer comes right after the first sentence, so the model reads it before the limits. The text is about 25 words longer than before.

**The refusal for a reserved name** (`queryError`, when the guard's `RejectError.Schema` is set: `_jenab_*`, `sqlite_*`, PRAGMA functions). Old, for `SELECT name FROM sqlite_master`:

> the query was refused: line 1:18: \`sqlite_master\` is reserved. Jenab's own tables, sqlite_schema and PRAGMA functions can't be queried; the project's tables are: prices. Use describe_table for a table's columns, keys and row count.

New:

> The project card lists the tables: prices. The query was refused: line 1:18: \`sqlite_master\` is reserved. Jenab's own tables, sqlite_schema and PRAGMA functions can't be queried. Use describe_table for a table's columns, keys and row count.

The list and the hint stay as they were. A new helper, `cardPointer`, builds the first sentence. A project with no tables gets "The project has no tables yet." If the tables can't be read, it says only "The project card lists the tables." Other refusals (layer 2, one statement only, and so on) and SQLite's own errors are unchanged. `tableList` still serves "no such table" and `describe_table`/`insert`.

**Tests** (`internal/tool/data_test.go`):
- `TestQueryRefusals`: a refusal for `_jenab_columns`, `sqlite_schema` or `pragma_table_info` must start with "The project card lists the tables: big, coins, holdings, loose, prices. The query was refused: " and keep the hint. The others still read "the query was refused: ".
- New `TestQueryDescription` pins the three new phrases in `query`'s description. No test pinned it before; `TestBuiltinSpecs` only checks that it isn't empty.

Commit `7444fbe` (code and tests); this report in a second commit.

## Checks run

With `GOPROXY=off GOMEMLIMIT=1500MiB`, and a placeholder `frontend/dist/index.html` (git-ignored) for the embed:
- `go build ./...`, `go vet ./...`: pass.
- `go test -p 2 -short -count=1 ./...`: pass.
- gofmt: `data.go` and `data_test.go` are clean, checked with CRs stripped.
- `go run ./cmd/jenab-scenarios check scenarios`: every set loads.

## Live runs

`jenab-scenarios run -model <m> -reps 3 -timeout 20m -usage-log …` on `scenarios/phase2-acceptance` (`P2_schema_memory`). The three models ran at the same time, each in its own process, with the same set, prompts and settings. `-timeout 20m` is the one change from P2-14 (default 10m), for NVIDIA's slow streams; it is the same for every model. `scenarios/models.yaml` (git-ignored, not committed):
- gemini: `gemini-3.5-flash-lite` at 1,048,576 on `GEMINI_API_KEY`, the main key. No reserve key was used.
- `nvidia` (openai_compatible, `https://integrate.api.nvidia.com/v1/`): `mistralai/mistral-large-4-1t-a52b` at 131,072.
- `anthropic` (`paid: true`): `claude-haiku-5-5` at 1,000,000. The user approved it in chat with a $0.20 cap.

### Turn 5 ("Which tables does this project have, and what is the key of the prices table?")

| Model | `describe_table` in turn 5, before → after | `sqlite_master` tries in turn 5, before → after |
|---|---|---|
| gemini-3.5-flash-lite | 3/3 → **2/3** | 3/3 → **0/3** |
| mistral-large-4 | 3/3 → **3/3** | — → **0/3** |
| claude-haiku-5-5 | 3/3 → **3/3** | — → **0/3** |
| **All** | **9/9 → 8/9** | **7/9 → 0/9** |

"Before" is P2-14. Its report gives `sqlite_master` per model only for flash-lite (every rep). Mistral and Haiku together were 4 of 6.

- Turn 5 after the change: one call in 8 runs (`describe_table` on `prices`, then the answer) and none in flash-lite's r1, which answered from the card. Before, 7 of 9 runs made two calls (a refused `sqlite_master` query, then `describe_table`). So turn 5 now costs 2 requests in 8 runs and 1 in one run, where 7 runs needed 3 before.
- No model call named `sqlite_master` or `sqlite_schema` in any turn. The only hits in the transcripts are the scenario's own end-of-run checks. The new refusal text never fired live, so only the tests cover it.
- Every turn-5 answer named `prices` and the key `(coin, date)`, and every run passed the `schema` check.

### Runs

| Model | Passed | Requests a run (chat + schema agent; to the provider) | Time a run | Cost |
|---|---|---|---|---|
| gemini/gemini-3.5-flash-lite | **3/3** | 14+2, 17+2, 21+2; 16, 19, 23 | 46 s, 84 s, 118 s | free |
| nvidia/mistral-large-4-1t-a52b | **2/3** | 18+2, 15+2, 15+2; 20, 17, 17 | 1,078 s, 396 s, 334 s | free |
| anthropic/claude-haiku-5-5 | **3/3** | 17+2, 15+2, 14+2; 19, 17, 16 | 32 s, 29 s, 32 s | **$0.017** for the 3 reps |

P2-14 for comparison (requests to the provider a run): flash-lite 22, 24, 21; mistral 22, 20, 19; Haiku 16, 17, 16. The totals depend mostly on turns 1 and 3 (the schema agent and how many tries it takes), so they move by more than turn 5's saving. Flash-lite's r3 made 4 calls in turn 1 and 2 in turn 3. Haiku's r1 made 4 calls in turn 1 and 2 in turn 3.

- **Haiku cost:** the usage log has 52 requests: input 182, cache read 275,865, cache write 62,273 and output 11,988 tokens. At $0.10 / $0.01 / $0.125 / $0.50 a million (P2-14's rates) that comes to **$0.0166**, under the $0.20 cap. The runner reported 14 thinking blocks dropped and 2 cache breaks. No request failed.
- **Gemini:** 58 requests on the main key, with no `quota` error. Four `rate_limited` 429s (the 15-a-minute limit) and one `overloaded` 503 were each retried by the app, and no run failed because of them.
- **NVIDIA:** 54 requests. Three of r1's requests stalled (`transport`: no event for 2 minutes) and were retried, which made r1 take 18 minutes.
- **NVIDIA r1 failed `approval_shown`, which has nothing to do with this change.** In turn 1 mistral made `daily_prices` with a `coin` column and key `(coin, date)` already. So turn 3 ("also track Ethereum… one general table prices(coin, date, price)") was a rename of the table and a column. That change deletes no data and doesn't change a key, so no migration card was shown, and the scenario's check expects one. Every other check passed: rows, prices, schema, recall, undo ("Undo turn 7: done, 3 rows in prices"). This is the model's design in turn 1, before `query`'s text matters. In P2-14 every run's turn 3 changed a primary key and asked on a card. Recorded, not worked around. The scenario could accept "no card needed" when turn 1 already has the general key, but that's Main-Session's call.

The results folders (`scenarios/phase2-acceptance/results/`) are git-ignored and not committed.

## Notes

- Nothing model-specific: one description and one refusal for every model.
- No reserve key; Haiku ran only after the user's OK in chat. NVIDIA and Gemini errors above are given only by kind and status.
- No personal identifiers in the commits or this report.

## Proposed Result for the ticket

> Done on 2026-11-15 (commit 7444fbe and the report commit).
>
> - `query`'s description now says the project card lists the tables and, for the tables it shows, their columns and keys; that `describe_table` is for a table it names without columns; and that `sqlite_schema` and Jenab's own tables can't be queried. A refusal for a reserved name starts with "The project card lists the tables: …" and keeps the list and the `describe_table` hint. The guard is unchanged. `TestQueryRefusals` pins the new start, and a new `TestQueryDescription` pins the description.
> - `P2_schema_memory`, 3 reps on each reference model after the change (P2-14 before), turn 5: **`sqlite_master` tries 7/9 → 0/9**; **`describe_table` 9/9 → 8/9** (flash-lite 2/3, mistral-large-4 3/3, Haiku 5.5 3/3). Turn 5 now takes 2 requests in 8 runs and 1 in one, where 7 runs needed 3. No model named `sqlite_master` in any turn, so the new refusal didn't fire live.
> - Runs: flash-lite 3/3 (16–23 requests, 46–118 s), Haiku 5.5 3/3 (16–19 requests, about 30 s, $0.017 for the 3), mistral-large-4 2/3 (17–20 requests, 6–18 min). Mistral's failed run made a `(coin, date)` table in turn 1 already, so turn 3 was a rename that showed no migration card and `approval_shown` failed. That is the model's design and not this change; the other checks passed.
> - `describe_table` for a table the card already shows still happens on all three models. As the ticket's rule says, I added no more prompt text.
