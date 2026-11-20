# P3-06 — Forms and row actions write data
**Type:** Feature
**Status:** Open
**Gate:** 3 (Phase 3)
**Needs:** P3-05
**Requirements:** R-43, R-113

## Goal
Forms insert and update rows, and row actions edit, set and delete them, through the writer queue, in the change log and undoable (SPEC 5.3, 5.5, 5.6).

## Scope
- **Forms (5.5):** `insert` and `update` with `load` (the row key as `:key`), values checked in Go against the column types, constraint errors on their field (`keyword: a row with this value already exists`), `select` options from a query, a `file` control that uploads into the bucket under `prefix` and stores the key.
- **Row actions (5.3):** `open_form` with the row's key, `delete` with `confirm`, `set` with literals, `now()` and `today()` checked against the column types, and `bulk` on the selected rows of the loaded page, at most 500, as one write request and one change-log entry.
- **Source:** `form:<view_id>` for forms and row actions, so *Undo turn* doesn't touch them; each write has its own undo from its notice, as file deletes do (2.6).
- **`run_pipeline`:** forms, actions and buttons with it don't validate until Phase 4 (P3-02). Leave a seam: an interface for "run this pipeline with these inputs" and the "already running since 08:00" answer, with a fake in tests.

## Done when
- Tests: an insert and an update with type errors and a constraint error, a file upload, each row action including `bulk` at 500 and over, the change-log entries and their undo.
