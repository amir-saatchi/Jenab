# P2-07 — Schema skills and model checks
**Type:** Task
**Status:** Done
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
Done on 2026-10-22 (commit 9128824), after three review rounds. **The bar is not met for gemma4:31b on T4 with CHECKs (3/5).** This is recorded as a reference-model failure; the test stays as SPEC 8.2 has it.

- **Built:**
  - **`widen_table`** `{table, column, value, to?}`, a new migration step. The new NOT NULL column goes first in the primary key, in each UNIQUE and unique index, and on both sides of a self-referencing foreign key. Old rows get `value`, and every other column, CHECK, foreign key and index is kept. `to` renames the table. It changes a key, so it asks for approval, and the card says the rows keep their data.
    - It refuses a default, a BLOB column, NULL as the value, a rowid INTEGER key, foreign keys from other tables that the new key breaks (each named), partial or expression unique indexes and table CHECKs.
  - **`add_column` takes `value`,** the old rows' value as SQL. It is required when the table has rows and the column has no default (`NULL` for none). A real value is one guarded UPDATE of the new column, so undo of earlier turns stays open (SPEC 2.6). A NOT NULL column without a default is added with a rebuild, which blocks undo like `rebuild_table`.
  - `rebuild_table` refuses a `default` on a column its mapping fills. Rebuilds from the schema text carry column CHECKs over, checked again by the SQL guard.
  - The planner costs `add_column` as a copy or a rebuild when it writes rows. The report has `KeysWidened`, so the card tells a widening from other key changes.
  - The skills `migrations` and `database-design` were rewritten from SPIKE-025's split: the step table, a widening section with a neutral example, and SPEC 8.2's rules. The same text for every model.
  - Runner: `db_query` and `approval_shown` asserts, an `agent` field on the call asserts, and the schema agent's calls, cards and the final database in the transcripts.
  - Sets: `scenarios/schema-design` (T1), `scenarios/schema-widen` (T4) and `scenarios/schema-widen-check` (T4 with CHECKs on price and title).
- **Scores** (5 runs each; requests are the main agent's plus the schema agent's, over 5 runs):

  | Model | T1 | T4 | T4 with CHECKs |
  |---|---|---|---|
  | gemma4:31b | 4/5 (25) | 4/5 (17) | **3/5** (18) |
  | gemini-3.5-flash-lite | 5/5 (29) | 4/5 (41) | 4/5 (40) |
  | minimax-m3.1-flash (not a reference model) | 0/5 (59) | 4/5 (73) | 5/5 (151) |

  Gemini T1 and minimax are from the code before the last review round, whose changes were the planner's costs and neutral wording.
- **Why runs failed** (from the transcripts):
  - **The news key (gemma and Gemini, T4):** the main agent's goal says "add a coin column to btc_news". The schema agent does just that, with `value` 'BTC', and the key stays `url` instead of `(coin, url)`.
    - The schema agent has had `database-design` since P2-06, so a skill doesn't reach it.
    - A required `key` field on `add_column` was tried and reverted: gemma then chose "not in the key" in 4 of 5 runs (T4 3/5).
    - Wording aimed at T4's own case helped gemma, but was taken out as teaching to the test.
  - **Gemini, T4 with CHECKs:** one run made a new table and copied the rows instead of widening, and the price CHECK was lost.
  - **gemma, T1:** named news columns `published_at` and `reason_it_matters` when the user said "its date … the reason it matters".
  - **minimax:**
    - In T1 it asks with `ask_user` and builds in the same turn, so there is no proposal turn.
    - Its schema agent wraps lists as `{"item": [...]}` and sends `not_null` as `"true"` or `1`, which wastes tries.
    - At about 30 s a request it hits the 10-minute run limit.
- **Changes from the ticket:** the new `widen_table` step and `add_column`'s `value` (SPEC 8.2 updated). It also takes over P2-06's "P2-07's scenario sets pass".
- **Open points:**
  - Ask the main agent for the goal as tables and keys (the key of each changed table) in `request_schema_change`. This targets the news-key failure; not built.
  - Type `apply_migration`'s steps fully (columns, `primary_key`, `not_null`), which might stop the `{"item"}` wrapping on any model.
  - The T1 naming misses.
  - Reports are in `scenarios/<set>/results/` (git-ignored).
