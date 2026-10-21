# P2-06 — Schema agent
**Type:** Feature
**Status:** Done
**Gate:** 3 (Phase 2)
**Needs:** P2-04, P2-05
**Requirements:** R-30, R-84, R-18, R-54, N-22, N-40

## Goal
`request_schema_change(goal)` hands a schema request to the schema agent, which returns steps that Go checks and applies (SPEC 8.2).

## Scope
- **`request_schema_change(goal)`:** an agent tool with `Areas: tool.Tables` and `load_with` for `migrations`.
- **The schema agent:** a run of its own with the `database-design` and `migrations` skills. Its input is the goal, the current schema with annotations, the dependents with the columns they use, and row counts. It can `read_config` a dependent once there are any.
  - Subagents come in Phase 5, so this ticket builds the in-memory run they will use: messages in memory, one level, the parent's source on every write, and the transcript in the bucket with a ref in the parent's result (8.6).
- **Output:** `applied` (migration ID, schema version, dependents updated), `rejected` (a reason) or `needs_approval`.
- **Try first:** it tries the migration, and a failed try's report goes back to it. It gets at most 4 more attempts (the repair loop, 10), then returns `rejected` with the last errors.
- **Approvals (8.8):** a destructive migration always asks, with the plan, the destructive steps, dependents, row counts and the time for long ones. A change that deletes nothing asks only at Strict.
- **Chat:** the card, then a chip with the migration and *View steps*. The main agent gets the result and tells the user.
- **Writes:** only the schema agent changes the schema. It can't edit memory.

## Done when
- Tests with `provider/fake`: applied, rejected after 4 failed tries, a destructive plan approved and denied, Strict asking for a plain `create_table`, *Stop* during the card.
- P2-07's scenario sets pass.

## Result
Done on 2026-10-21 (commit 51d2f57), after two review rounds.

- **Built:**
  - `request_schema_change` in `internal/agent/schema.go`, with the in-memory subagent run in `subagent.go` and its prompt in `prompts/schema.md`. The transcript goes to the bucket with source `chat:<id>`, and the parent's result carries its ref.
  - The built-in skills `database-design` (`needs: [request_schema_change]`) and `migrations` (loaded with the tool).
  - Store: `MigrateOptions.Planned` and `Long`, and `MigrationReport.Lost`, `LostRows` and `KeysChanged`, so the card shows what a plan drops.
  - Tools: `Env.Approve`, `Env.Subagent`, `Env.Progress` and `Result.Notices`.
  - Chat: `Status.Doing` and the *Doing* bar while the schema agent works, and the migration chip (`NoticeMigration`) with its steps.
- **Tests:** 16 tests with `provider/fake` cover applied, rejected after 4 failed tries, a destructive plan approved and denied, Strict asking for `create_table`, *Stop* during the card, planned losses, key changes, undo keeping the transcript, progress, a clipped long plan, a missing skill and provider errors.
- **Scenarios (missing-features, 3 runs each):**
  - gemma4:31b: 18 of 18.
  - gemini-3.5-flash-lite: 16 of 18. Both failures are F4 (a BTC chart). Every fetched page gave only today's price, yet the agent made up the other 29 days, saved them in a new `prices` table and charted them as stored data. This is a model failure we record, and the scenario stays as it is.
- **Decided here:**
  - `needs_approval` is handled inside the call: the card is shown and the answer goes back to the schema agent, so the main agent sees only `applied` or `rejected`.
  - A plan's losses are matched to its steps by table and column name (`Planned`).
  - A skill can list the tools it `needs`, and it is offered only when they are there.
- **Changes from the ticket:**
  - "P2-07's scenario sets pass" moved to P2-07. Here the missing-features set was rerun, and F2 now expects `request_schema_change` to be applied.
- **Open points:**
  - `read_config` on dependents comes once there are dependents.
  - Subagent traces aren't shown in the UI.
  - The schema agent's input lists every table, which gets long on big projects.
  - HARNESS.md and CODE-OUTLINE.md could mention `Env.Approve`, `Subagent`, `Progress` and `Result.Notices`.
