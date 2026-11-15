# TASK-051 — The `query` tool points to the card, not `sqlite_master`
**Type:** Task
**Status:** Open
**Gate:** 3
**Needs:** —

## Goal
In P2-14 every run on all three reference models called `describe_table` for a table the card already shows with its columns and key, and 7 of 9 first tried `SELECT name FROM sqlite_master …`, which the guard refused (also in the built app with Haiku 5.5). Each wasted one or two requests. `describe_table`'s description says the card already lists the columns, but `query`'s, the tool the model is about to use, only says "Jenab's internal tables can't be queried: use describe_table for a table's columns". It never says the card lists the tables. Every model does it, so this is our wording, not one model's habit.

## Scope
1. **`query`'s description** (`internal/tool/data.go`): say that the project card lists the tables and, for the tables it shows, their columns and keys; that `sqlite_schema` and Jenab's own tables can't be queried; and that `describe_table` is for a table the card names without columns.
2. **The refusal for a reserved table** starts with the same pointer, such as "The project card lists the tables: prices." It keeps the list and the hint it has now.
3. **Measure before and after** on `scenarios/phase2-acceptance`, 3 reps on each reference model: `describe_table` calls and `sqlite_master` tries in turn 5, and the requests a run. Haiku 5.5 is paid: about $0.02 for the 3 reps after the change; ask the user before the run.

## Rules
- The same text for every model (SPEC 1). If the counts don't fall, record it and stop; no more prompt text.
- The guard doesn't change.

## Done when
- The new text, with the tool-description test updated.
- The before and after counts in the Result. The before counts are P2-14's: `describe_table` in 9 of 9 runs, `sqlite_master` in 7 of 9.
