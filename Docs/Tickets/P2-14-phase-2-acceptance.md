# P2-14 — Phase 2 acceptance
**Type:** Task
**Status:** Done
**Gate:** 3 (Phase 2)
**Needs:** P2-01 to P2-13

## Goal
Check Phase 2's "done when" in the built app (PROPOSAL 9): the agent creates and evolves a schema from a conversation, and a new chat answers questions about earlier work using only queries and memory.

## Scope
- **Scenario, as P1-18 did for Phase 1,** on the reference models:
  1. In one chat, the user describes a Bitcoin tracker. The agent designs the tables, adds a few days by hand and records the goal in memory.
  2. The user says "also track Ethereum". The migration asks, is approved, and keeps every row.
  3. The app restarts.
  4. A new chat answers questions about prices, the schema and the goal, using `query` and memory only, with no history search.
  5. *Undo turn* on a turn with inserts reverts only those rows.
- **Numbers:** N-08 (a 5,000-row upsert and its undo), N-20 (the guard), and the benchmark's result (P2-13).
- **Built app:** the same steps by hand in the Wails build, with screenshots of the migration card, the memory chip and *Undo turn*.

## Done when
- The scenario passes on both reference models with 3 reps, and the manual run in the built app is recorded.

## Result
Done on 2026-11-15. The scenario passes on all three reference models, and the manual run in the built app passed. Report: [P2-14-report](reports/P2-14-report.md).

- **Scenario:** `scenarios/phase2-acceptance`, `P2_schema_memory`, 9 turns in two chats on a new project.
  - The first chat makes the Bitcoin table through the schema agent, adds three days by hand and saves the goal to project memory.
  - "Also track Ethereum" asks on a migration card. It is approved and keeps every row.
  - The app restarts. A new chat answers about a price, the tables and the goal with `query`, the card and memory, and no history tool.
  - *Undo turn* on the first of two turns of inserts takes out its 3 rows and nothing else.
  - The runner has a new message step, `undo: N`: it runs *Undo turn* on turn N of the record. Conflicts, or nothing to undo, go to the transcript.
- **Live runs, 3 reps each:**

  | Model | Passed | Requests a run | Time a run | Cost |
  |---|---|---|---|---|
  | NVIDIA `mistral-large-4-1t-a52b` | 3/3 | 19–22 | 7–10 min | free |
  | Anthropic `claude-haiku-5-5` | 3/3 | 16–17 | about 30 s | $0.017 for all 3 |
  | Gemini `gemini-3.5-flash-lite` | 3/3 | 21–24 | 71–110 s | free |

  - Flash-lite ran on two more keys, after the free tier's 500-a-day quota stopped the first. One rep broken by the quota doesn't count.
  - In every run the undo took out exactly the turn's 3 rows.
  - Turn 5 asks for the key, which the card shows. Every run still called `describe_table`, and 7 of 9 first tried `sqlite_master`, which the guard refused. Recorded, not worked around.
- **Go test:** `TestPhase2Acceptance` (`internal/scenario`) plays the set on a scripted model in `go test`. Every check must pass. It also checks that the new chat's first request has the goal from memory and the card's `prices`, and nothing of the first chat, and that turn 9's request has the undo notice.
- **N-08:** a 5,000-row upsert takes 136–171 ms, and its undo 120–174 ms (target 1 s).
- **N-20:** SPIKE-007's 63 cases and the attack, resource and byte-sweep tests pass. Layer 1 takes 1.6 µs, layer 2 78 µs.
- **Benchmark (P2-13, TASK-033):** Jenab is as accurate as plain. After TASK-033, flash-lite uses 19,981 tokens a turn against plain's 9,760. It is cheaper per turn on long chats from turn 14, but not summed within three weeks.
- **Built app (2026-11-15, the user, Haiku 5.5, build 5735016):** passed. The screenshots stay with the user, since they show the data folder's path.
  - Turn 1: `update_memory` saved the goal to project memory, with the chip `Memory updated: goals` and its *Undo* and *Edit*. The schema agent made `btc_prices`.
  - Turn 3: the card "Apply a schema change that deletes data?" was approved. The 3 Bitcoin rows moved to `prices` as `BTC`, and `btc_prices` was dropped.
  - After a restart, a new chat answered the three questions (sent as one message) with `query`, `describe_table`, the card and memory: 92,115, 93,480 on 2026-11-11, `prices` keyed `(coin, date)`, and the goal. No history tool. In a second try, the model first tried `sqlite_master`, as in the live runs.
  - The two insert messages were sent as one, so one turn inserted 5 rows. *Undo turn* took out those 5 ("turn 2 was undone: 5 rows in prices") and kept chat 1's rows; *Redo turn* appeared. The last question got "no Ethereum days", which is right. The two-turn case is covered by the 9 live runs.
- **Findings:** TASK-050 (the UI: Project settings is hard to find, *Settings → Memory* doesn't say it's for every project, and the schema tool's row shows a raw ref) and TASK-051 (the `query` tool's text sends models to `sqlite_master` and `describe_table`).
- **Open:** `TestPhase2Acceptance` in CI, once the billing lock is lifted.
