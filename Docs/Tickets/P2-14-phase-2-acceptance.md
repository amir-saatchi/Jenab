# P2-14 — Phase 2 acceptance
**Type:** Task
**Status:** Open
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
