# P2-06 — Schema agent
**Type:** Feature
**Status:** Open
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
