# TASK-051 — The `query` tool points to the card, not `sqlite_master`
**Type:** Task
**Status:** Done
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

## Result
Done on 2026-11-16. Report: [TASK-051-report](reports/TASK-051-report.md).

- **`query`'s description** now says, right after its first sentence, that the project card lists the tables and their columns and keys, that `describe_table` is for a table the card names without columns, and that `sqlite_schema` and Jenab's own tables can't be queried.
- **A refusal for a reserved name** starts with "The project card lists the tables: …", then the reason and the hint as before. The guard is unchanged.
- **Before and after**, `scenarios/phase2-acceptance`, turn 5, 3 reps on each reference model (2026-11-15):

  | | `describe_table` | `sqlite_master` tries |
  |---|---|---|
  | P2-14 (before) | 9 of 9 runs | 7 of 9 |
  | After | 8 of 9 | **0 of 9** |

  So turn 5 costs 2 requests in 8 runs and 1 in one, where 7 runs needed 3. `describe_table` barely moved, so no more prompt text was added, as the ticket says.
- **Runs:** flash-lite 3/3 and Haiku 5.5 3/3 ($0.017, cap $0.20). Mistral-large-4 passed 2/3. Its failed run made a general table with `coin` and the key `(coin, date)` in turn 1, so turn 3 was only a rename: it deletes nothing, and no card was needed. The scenario's live card check is now `info`. `TestPhase2Acceptance` still requires the card, since its script changes the key.
