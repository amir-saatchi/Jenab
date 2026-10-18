# P2-07 — Schema skills and model checks
**Type:** Task
**Status:** Open
**Gate:** 3 (Phase 2)
**Needs:** P2-06, TASK-013
**Requirements:** R-83, R-84, N-40, N-41

## Goal
The schema agent's skills are written, and the reference models design and change tables with them. This covers the "before Phase 2" checks from SPIKE-021 and SPIKE-025 that are about tables.

## Scope
- **Skills:** `database-design` (tables, keys, annotations, when to widen a table: the 8.2 guidelines) and `migrations` (steps, destructive changes, dependents). They start from SPIKE-025's split of guide v2 (`spikes/025-skills/skills.go`), with SPIKE-021's lessons. The same text for every model (SPEC 1).
- **Scenario sets** in the runner, on real tools and a real project:
  - SPIKE-021 T1: design the Bitcoin tables from a conversation (SPEC 9.1).
  - SPIKE-021 T4: "also track Ethereum" (9.5), with the plan, the approval and the copied rows checked.
  - Checks: the migration applied, the tables and keys as asked, no rows lost, the number of tries.
- **Models:** the reference models, gemini-3.5-flash-lite and gemma4:31b, with at least 5 reps for T4. gemma failed T4 in SPIKE-021 and SPIKE-025 (0/2 with skills); this is the open risk.
- **If a model still fails:** a simpler step shape or a higher-level step, not a longer skill or a line for one model (SPIKE-021 decision 6).
- **Moved out:** SPIKE-021's T2 (a pipeline), T3 (views and a form) and T5 (a page) need configs that don't exist yet. They run in Phases 3 and 4, with guide v3.

## Done when
- T1 and T4 pass at least 4 of 5 runs on each reference model, or the step shape is changed until they do, and the result is recorded with the numbers.

## Result
