# P2-14 report: Phase 2 acceptance

Branch `claude/brave-jemison-0adb81`. Runs on 2026-11-14 and 2026-11-15 (UTC), Windows 11, i5-1235U.

**Status:** the set, the runner's undo step and the Go test are in. All three reference models pass 3 of 3: NVIDIA mistral-large-4, Haiku 5.5 and gemini-3.5-flash-lite. Flash-lite's reps ran on the user's third and fourth Gemini keys, after the first key ran out of its daily quota (below). The manual run in the built app is a checklist for the user (below).

## What was built

- **`scenarios/phase2-acceptance`**, one scenario, `P2_schema_memory`, 9 turns in two chats on a new, empty project (`project_card: true`, no fake tools):
  1. Turn 1: the user describes the tracker (daily Bitcoin closes in USD, typed by hand) and the goal ("whether buying a fixed 50 USD every Monday beats buying only after a 5 % drop"). The agent makes the table through `request_schema_change` and saves the goal with `update_memory`.
  2. Turn 2: three closes by hand (`insert`).
  3. Turn 3: "Also track Ethereum": one `prices(coin, date, price)` table, every Bitcoin row kept, the old table dropped. The migration asks on a card; the runner approves it once.
  4. The app restarts (`restart: true`) and a new chat starts (`new_chat: true`).
  5. Turns 4–6 ask about a price and the highest day, the tables and the key, and the goal.
  6. Turns 7 and 8 add rows by hand (3 Ethereum days, then one more day of each coin). *Undo turn* on turn 7. Turn 9 asks which Ethereum days are left.
- **Checks** (all real except those marked info):
  - design: turn 1's `request_schema_change` applied.
  - by_hand: turn 2's `insert`.
  - goal: project or user memory holds the goal at the end (Monday and the 5 % drop).
  - migration: turn 3's change applied, an approved migration card, `prices` keyed `(coin, date)`, no other table with a price or close column.
  - rows: the four Bitcoin rows with their exact prices at the end (the three from chat 1 survive the migration, the restart and the undo).
  - prices: turn 4 calls `query` and answers 92,115 and 2026-11-11.
  - schema: turn 5 names `prices`, `coin` and `date`.
  - recall: turn 6 gives the goal.
  - no_history: no `search_history`, `read_messages` or `list_chats` in turns 4, 5 and 6.
  - undo: turns 7 and 8 inserted into `prices`; the notice `[turn N was undone: 3 rows in prices…`; at the end Ethereum has only 2026-11-13 = 3,251 and `prices` has 5 rows; turn 9's answer has 3,251.
  - info: `update_memory` with scope project in turn 1; turn 9 doesn't name the undone prices; the tables at the end; at most 2 schema changes; 9 turns; no failed turn.
- **The runner's undo step:** a message `undo: N` runs *Undo turn* on turn N of the record (counted across chats as the asserts count, so turn 7 is the second chat's turn 4), once the chat is idle, through the orchestrator's `UndoTurn` as the footer does. Conflicts or "nothing to undo" are recorded in the transcript's "Undo" section, not a run error, so the checks see the rows as they are. Loading refuses an undo with text, `at`, `new_chat`, `refresh`, `restart` or `day`, on the first message, or of a turn after it. `scenarios/README.md` documents it and lists the set.
- **`TestPhase2Acceptance`** (`internal/scenario/phase2_test.go`) plays the set's `_offline.yaml` through the real orchestrator, tools, schema agent, database and memory on `provider/fake`, so it runs in `go test` (and in CI once billing allows). Every check of the set must pass, and it also reads what the model was sent:
  - turn 4's request (the new chat after the restart) has the goal from memory and `prices` from the card, and nothing of the first chat (its messages or the old table's name);
  - turn 6's request has the goal;
  - turn 9's request has the undo notice;
  - the transcript says `Undo turn 7: done, 3 rows in prices`.
  `TestUndoNothing` checks that an undo of a turn without changes is recorded and the run goes on. `TestLoadErrors` has five new cases.
- Commit `fb197fe` (code and set); this report in a second commit.

## Checks run

- `go build ./...`, `go vet ./...`: pass. The worktree needed a placeholder `frontend/dist/index.html` (git-ignored) for the embed.
- `go test -p 2 -short ./...` (`GOMEMLIMIT=1500MiB`, `GOPROXY=off`): pass, 31 packages.
- gofmt: the changed files are clean (checked with LF endings; the worktree checks out CRLF, so `gofmt -l` lists every file there).
- `go run ./cmd/jenab-scenarios check scenarios`: every set loads, `phase2-acceptance` with 1 scenario.
- Scripted run: `jenab-scenarios run -model scripted:scenarios/phase2-acceptance/_offline.yaml`: pass, 14+2 requests.

## Live runs

`-reps 3`, the same set, prompts and settings for every model. `scenarios/models.yaml` (git-ignored, not committed): gemini with `gemini-3.5-flash-lite` at 1,048,576; `nvidia` (openai_compatible, `https://integrate.api.nvidia.com/v1/`) with `mistralai/mistral-large-4-1t-a52b` at 131,072; `anthropic` (`paid: true`) with `claude-haiku-5-5` at 1,000,000. `-usage-log` for each.

| Model | Passed | Requests a run (chat + schema agent; to the provider) | Time a run | Prompt tokens a run | Cost |
|---|---|---|---|---|---|
| nvidia/mistral-large-4-1t-a52b | **3/3** | 20+2, 15+3, 17+2; 22, 20, 19 | 418 s, 545 s, 586 s | 75,859 (80 % cache reads) | free |
| anthropic/claude-haiku-5-5 | **3/3** | 14+2, 15+2, 14+2; 16, 17, 16 | 32 s, 35 s, 26 s | 95,562 (78 % cache reads) | **$0.017** for the 3 reps |
| gemini/gemini-3.5-flash-lite | **3/3** | 20+2, 22+2, 17+4; 22, 24, 21 | 84 s, 110 s, 71 s | 88,301 (34 % cache reads) | free |

- **Haiku cost:** from the usage log, 49 requests: input 176, cache read 246,729, cache write 68,778, output 12,167 tokens, at $0.10 / $0.01 / $0.125 / $0.50 a million (prompts under 100K), the rates of the HAIKU-PASS report: $0.017, far under the $0.50 cap. The runner reported 12 thinking blocks dropped and 3 cache breaks over the 3 reps; no request failed.
- **NVIDIA:** 61 requests; median 8.6 s, slowest 180 s. Five `rate_limited` 429s, each retried by the app, and one request cancelled after 62 s and retried. None failed a run, so there were no provider-outage reruns.
- **Gemini, `quota` 429:** the first rep got "limit: 500 requests per day on Free Tier" from its first requests; 7 of its 34 requests went through, then reps 2 and 3 failed at once. The key was used up by earlier work today. I stopped Gemini, told Main-Session and didn't touch the reserve keys. **Google's limit is 500 requests a day for flash-lite, where DEVELOPMENT.md says about 1,500** (MODELS.md already says 500). The runner went on after the quota error, as TASK-048 says; its report for that run is in the results but counts nothing.
- **Gemini on GEMINI_API_KEY_III (2026-11-15, 06:42 UTC):** the user said in chat to use it. The worktree's `models.yaml` named it for the gemini provider; nothing else changed. Reps 1 and 2 passed every check. Rep 3 got the same "500 requests per day on Free Tier" 429 in its turn 3, after the migration was applied: turns 3–5 stopped, a request in turn 6 went through and answered turns 4–6 at once, and turns 7–9 never inserted, so its rows, prices, schema and undo checks failed. That is the quota, not the model. The key had been used earlier, so it gave out after 60 requests of this run. I stopped Gemini there.
- **Gemini rep 3 on GEMINI_API_KEY_IV (06:48 UTC):** the user cleared it in chat. One rep, `-reps 1`, the same set and settings: passed every check, 17+4 requests, 71 s, no quota error. The three passing reps are r1 and r2 of `results/20261115-064217` and r1 of `results/20261115-064758`; the broken one doesn't count.

### What the models did

- **Turn 1:** all 9 passing runs made one table and saved the goal to project memory in the same turn. Each named it its own way: Haiku `bitcoin_daily_close`, `daily_btc_prices`, `btc_daily_prices`; mistral `daily_prices` twice and `btc_daily_prices`; flash-lite `bitcoin_prices` twice and `daily_prices`. In flash-lite's rep 3 the schema agent needed three `apply_migration` calls for the first table: a `table` field at the top level, then `name` for `table` in the step; the tool's errors named both, and the third call applied.
- **Turn 3:** every run asked on a migration card (`widen_table`, "changes a primary key") and kept the 3 rows. One mistral run's schema agent first checked the table with `describe_table` and `query`.
- **Turns 4–6:** prices came from `query` in every run, the goal from memory, and no run called a history tool. Memory ended at 38–82 tokens.
- **Turn 5 costs more than it needs:** the card lists `prices (… rows): coin TEXT NOT NULL, date TEXT NOT NULL, price REAL; key (coin, date)`, yet every run called `describe_table`, and 7 of 9 (every flash-lite rep among them) first tried `SELECT name FROM sqlite_master …`, which the guard refused ("`sqlite_master` is reserved …"). Each recovered at once. So SPEC 3.2's "for a table the card lists, `describe_table` is a wasted request" happens on all three models; the answers were right. Recorded, not worked around.
- **Undo:** in all 9 passing runs the undo reverted exactly turn 7's 3 rows ("turn 4 was undone: 3 rows in prices"), with no conflicts: turn 8's two rows and the Bitcoin rows stayed. Turn 9 named only 2026-11-13 in every run.

## Numbers

### N-08: a 5,000-row upsert with the change log, and its undo (target under 1 s)

`JENAB_PERF_TEST=1 go test -run 'TestUpsertSpeed|TestUndoSpeed' ./internal/store/`, on a table of 100,000 rows, 2026-11-14:

| | Time |
|---|---|
| upsert, 0 % updates | 136 ms |
| upsert, 50 % updates (3 runs) | 162, 161, 171 ms |
| upsert, 100 % updates | 169 ms |
| undo of a 5,000-row turn, 0 % updates | 120 ms (redo 221 ms) |
| undo of a 5,000-row turn, 50 % updates | 174 ms (redo 215 ms) |

Both pass, with the machine also running the scenario runs. P2-02 measured 150–300 ms for the upsert on a quiet machine; SPEC 2.6 has 0.3 s for the undo from SPIKE-009.

### N-20: the SQL guard

`go test ./internal/sqlguard/` and `go test -run 'TestAgentQuery|TestReadersAreGuarded|TestMigrateGuard' ./internal/store/`: all pass, 2026-11-14.

- SPIKE-007's 63 cases (`TestAgentQueryCases`): every attack refused at its layer, every normal query answers, and the project is unchanged after each.
- `TestAgentQueryAttacks`, `TestAgentQueryResourceAttacks` (6.7 s: the deadline and the worker's memory cap stop them), `TestAgentQueryByteSweep`, `TestReadersAreGuarded` (readers are `query_only`), and the migration guard (`TestMigrateGuard`, `TestMigrateGuardDependentWrites`).
- `BenchmarkCheck` (layer 1): **1.6 µs** a query (target under 10 µs). `BenchmarkExplain` (layer 2): **78 µs** (target under 200 µs).
- Live: 4 of the 6 runs above tried `sqlite_master`, and the guard refused it with a message the model acted on.

### The benchmark (P2-13, TASK-033)

Not rerun; from the tickets' results:
- **P2-13** (2026-10-31, `bench-bitcoin`, 3 reps): Jenab was as accurate as the plain harness (51/51 vs 51/51 on gemma4:31b, 51/51 vs 50/51 on flash-lite) but used more tokens a turn (flash-lite 37,466 vs 9,962). N-42 didn't hold, and TASK-033 followed.
- **TASK-033** (2026-11-01): flash-lite fell to 19,981 tokens a turn (−46.7 %), 49/51 vs plain 51/51 at 9,760. On the three-week `bench-bitcoin-long`, flash-lite's first request is smaller than plain's (7,115 vs 8,727) and it is cheaper per turn from turn 14, but summed over three weeks it costs more (567k vs 391k); gemma4:31b was cheaper summed at turn 30 (519.6k vs 523.6k).
- So the token target (N-42) holds per turn on long chats but not summed within three weeks on flash-lite. Neither of the current reference models besides flash-lite has run the benchmark; gemma4:31b is no longer one.

## Manual run in the built app (for the user)

I didn't launch the built app: it uses the user's own data folder. Steps, with the three screenshots the ticket asks for:

1. Build with `wails3 task build` and start the app. Use a model you're happy to spend on (any reference model).
2. New project "Bitcoin tracker". In its chat, send turn 1 of the scenario (the tracker and the goal). Check the schema chip and the memory chip `Memory updated: goals [undo] [edit]`. **Screenshot 1: the memory chip.**
3. Send turn 2 (the three closes). Check the turn footer offers *Undo turn* with "3 rows in btc_prices" (or the table's name).
4. Send turn 3 ("Also track Ethereum…"). A migration card asks, with the plan and the steps. **Screenshot 2: the migration card.** Approve once.
5. Quit the app completely and start it again.
6. Open the project, start a new chat, and send turns 4, 5 and 6. Check the answers: 92,115 on 2026-11-12, highest 93,480 on 2026-11-11; `prices` keyed by coin and date; the goal. Check no history tool shows in those turns.
7. Send turns 7 and 8. On turn 7's footer, press *Undo turn* ("3 rows in prices"). **Screenshot 3: the footer before pressing, or the notice after.** Check the notice "[turn 4 was undone: 3 rows in prices]".
8. Send turn 9, or open the prices table: only 2026-11-13 is left for Ethereum (3,251), and Bitcoin has 4 days.

## Notes

- Nothing model-specific: the same set, prompts and settings for every model; no provider or URL is built in.
- Reserve keys: GEMINI_API_KEY_III and _IV, each only after the user cleared it in chat; no other. No personal identifiers in the commits or this report; the NVIDIA errors above are their kind and status only.
- The results folders (`scenarios/phase2-acceptance/results/`) are git-ignored and not committed.

## Proposed Result for the ticket

> Done on 2026-11-1x (commit fb197fe and the report commit), pending the manual run.
>
> - **Scenario:** `scenarios/phase2-acceptance`, `P2_schema_memory`, 9 turns in two chats on a new project. The first chat makes the Bitcoin table through the schema agent, adds three days by hand and saves the goal to project memory; "also track Ethereum" asks on a migration card, approved, and keeps every row. The app restarts, and a new chat answers about a price, the tables and the goal with `query`, the card and memory, and no history tool. *Undo turn* on the first of two turns of inserts takes out its 3 rows and nothing else.
>   - The runner got an `undo: N` message, which runs *Undo turn* on turn N of the record; conflicts or nothing to undo go to the transcript.
>   - 2026-11-14, 3 reps: mistral-large-4 3/3 (19–22 requests, 7–10 min a run), Haiku 5.5 3/3 (16–17 requests, about 30 s, $0.017 for all 3). 2026-11-15: flash-lite 3/3 (21–24 requests, 71–110 s), on two more keys after the free tier's 500-a-day quota stopped the first. In every run the undo reverted exactly the turn's 3 rows.
>   - Turn 5 asks for the key, which the card shows; every run still called `describe_table`, and 7 of 9 first tried `sqlite_master`, which the guard refused.
> - **Go test:** `TestPhase2Acceptance` (`internal/scenario`) plays the set on a scripted model in `go test`: every check, plus the new chat's first request has the goal from memory and the card's `prices` and nothing of the first chat, and turn 9's request has the undo notice. CI is blocked by billing, so it runs locally for now.
> - **N-08:** a 5,000-row upsert 136–171 ms, its undo 120–174 ms (target 1 s).
> - **N-20:** SPIKE-007's 63 cases, the attack, resource and byte-sweep tests pass; layer 1 1.6 µs, layer 2 78 µs.
> - **Benchmark:** from P2-13 and TASK-033: as accurate as plain; flash-lite 19,981 tokens a turn after TASK-033 against plain's 9,760, cheaper per turn on long chats from turn 14 but not summed within three weeks.
> - **Built app:** <the manual run, with the three screenshots>.
