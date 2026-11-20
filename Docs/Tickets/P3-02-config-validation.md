# P3-02 — Validation of views and pages, and the block registry
**Type:** Feature
**Status:** Open
**Gate:** 3 (Phase 3)
**Needs:** P2-01, P3-01
**Requirements:** R-47, R-53

## Goal
Every view and page is checked before it is saved, and all errors come back together with their paths, so the model can fix everything in one try (SPEC 10). Each block type is one unit in a registry: a JSON Schema and a Go validator now, a kit component later (5.10).

## Scope
- **Formats:** JSON Schemas (2020-12, `santhosh-tekuri/jsonschema/v6`, already a dependency) for `table`, `chart`, `stat`, `form` and the page with its blocks (`view`, `heading`, `text`, `card`, `filter`, `button`, `image`). Start from SPIKE-021's `view.schema.json` and SPIKE-013's loader and error paths (`spikes/013-…`, `spikes/021-…`), and follow SPEC 5.1–5.10.
- **The registry:** `internal/view` (or a fitting name) with one unit per block type: its schema, its Go validator, and a slot for its kit component (P3-07, P3-08). A new block type is added by registering one unit; nothing else changes (R-47). Data blocks and page blocks are both units.
- **Checks, in SPEC 10's order, for views and pages:**
  1. Parse and schema, with the page checks listed in step 1 (views exist, 1–4 blocks a row, spans ≤ 12, no card in a card, every page filter binds, each filter block names a page filter once, button and action targets exist, tones only from 5.11, a missing `image` key is a warning).
  3. References: `open_form` targets are form views; `run_pipeline` targets are stored pipelines. There are none before Phase 4, so a view or page with `run_pipeline` fails validation until then, with a message that says so.
  4. Schema: tables and columns of forms and `rows_from`, `set` fields (not the key, `_run_id` or `_fetched_at`), `NOT NULL` columns without a default covered by a required form field, `image` and `file` formats and `file` controls on object-reference columns, no `_jenab_*` table.
  5. SQL: the text check, then the prepare and EXPLAIN check of the SQL guard (2.2), with NULL bound to every parameter, run through the query worker. `columns[].field`, `x`, `y`, `series_by` and the `stat` fields are checked against the prepared query's result columns. A table query with `ORDER BY` is a warning, not an error. A `stat` query may have `ORDER BY` and `LIMIT`.
  Steps 2 and 6–9 are for pipelines (Phase 4).
- **Messages:** SPEC 10's form (`line 4:13 columns[2].field: 'avg' is not a result column of the query; it has date, price`), leaf errors only, YAML fix hints ("quote this value: it contains `: `"), warnings returned but not blocking.
- **IDs:** check a config's `id` with the store's `CheckConfigID` and normalize it with `NormConfigID` (NFC) before comparing it with a saved ID (P3-01).
- **Filters:** controls, `required`, relative date defaults parsed (`today-90d`, `start_of_month`, …), a parameter in `params` or `filters` but not both, `select` options as a list or `{query}`.

## Rules
- The same checks and messages for every model (SPEC 10, model-neutral).
- No network and no LLM call.

## Done when
- A test per check with a passing and a failing config, and one config with many errors that returns all of them with paths.
- SPEC 9.3's six views and 5.9's page (without its `run_pipeline` action) validate against a fixture Bitcoin database.
- A test registers a made-up block type and validates a page that uses it, with no change outside its unit.
