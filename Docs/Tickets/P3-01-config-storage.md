# P3-01 — Views and pages: storage, change log and undo
**Type:** Feature
**Status:** Open
**Gate:** 3 (Phase 3)
**Needs:** P2-02, P2-03, P2-04
**Requirements:** R-34, R-53

## Goal
Views and pages are saved in `project.db` with a revision, every save and delete is in the change log, and *Undo turn* puts the previous revision back (SPEC 2.2, 2.5, 2.6, 5). Nothing draws them yet.

## Scope
- **Tables:** `_jenab_views(id, type, config, revision, updated_at, updated_by)` and the same shape for pages. SPEC 2.2 lists only `_jenab_views`; this ticket adds `_jenab_pages`, and Main-Session updates SPEC 2.2. The stored config is the YAML the agent wrote, kept as written (comments too), plus its parsed form if that helps reads.
- **Storage format:** a new `project.db` version with the migration and backup rules of R-06 (SPEC 2.8). An older app refuses the newer file.
- **Change log:** targets `view:<id>` and `page:<id>` (SPEC 2.5 lists `view:` only; Main-Session adds `page:`), op `save` or `delete`, `before` and `after` revisions, sources `message:<id>`, `user` and `migration:<id>`.
- **Undo:** *Undo turn*, rewind's *Undo changes* and the conflict check cover configs like memory sections (2.6): a later save of the same config is a conflict. Undoing a first save deletes the config; undoing a delete restores it.
- **Typed writes in Go:** save, delete and read by ID, and list by kind, through the writer queue (7.3). Configs are only ever saved after validation (P3-02); the store doesn't parse YAML itself.
- **Events:** a save, delete or undo publishes `project:configs` (or a name that fits the existing events) through the `Publisher`, with the IDs, so open pages refresh (5.9).
- **Services:** `ConfigService` (or a fitting name in `internal/app`) with list, read and the events, for the native stores. No UI.

## Rules
- Writes go through the writer queue and commit with their change-log entry in one transaction.
- No YAML or JSON Schema logic in `internal/store`.

## Done when
- Store tests: save, revise, delete, list; undo and redo of each, conflicts with a later save, a partly undone turn with a config and rows; the storage-format migration from the current version with its backup.
- The report lists the SPEC lines to change (2.2, 2.5).
