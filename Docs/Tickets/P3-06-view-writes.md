# P3-06 — Forms and row actions write data
**Type:** Feature
**Status:** Done
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

## Result
Done on 2026-11-26. Report: [P3-06-report](reports/P3-06-report.md).

- **Writes:** forms and row actions use the existing `insert`, `update` and `delete` requests, one interactive request and one change-log entry each, with the source `form:<view_id>`. *Undo turn* leaves them, and each notice undoes its own change. There is no `form_submit` request; SPEC 7.2 says so.
- **Values:** the new `internal/viewwrite` checks form values against the column types in Go first, read as each control gives them (Persian and Arabic digits too). Row keys are read against the key column's type, exact past 2^53. Dates keep the day the user picked.
- **Constraints:** the store's `ConstraintError` puts a broken UNIQUE, NOT NULL, CHECK or foreign key on its field ("a row with this value already exists"). A delete that other rows refer to names their tables.
- **Row actions:** `open_form`, `delete` with confirm, and `set` with `now()` (RFC 3339 UTC) and `today()`. Bulk is limited to 500 keys as one entry; 501 is refused.
- **Files:** a form's file field uploads under its prefix through the same key reservations as the composer. It takes only the keys it handed out, or the row's own on an update, and trashes them when the submit fails.
- **Seams:** `run_pipeline` goes through a `Pipelines` seam that Phase 4 fills, with "already running since 08:00" tested on a fake. `app.FormService` (`Open`, `Submit`, `Upload`, `RowAction`, `Action`, `Undo`) is ready for P3-09.
- **Left for later:** a checkbox on a TEXT column fails when submitted; validation (P3-02's rules) could refuse it at save time.
- **Review:** five Medium and six Low points are fixed: `now()`'s format, foreign-key messages, typed keys, upload checks, dates, scoped undo, issued file keys and missing update fields.
