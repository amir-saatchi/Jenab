# P3-01 report — Views and pages: storage, change log and undo

## What I built

**Store (`internal/store/configs.go`)**

- `project.db` format step 6 (`configTables`): `_jenab_views` and `_jenab_pages`, both
  `(id, type, title, config, revision, updated_at, updated_by)`, `WITHOUT ROWID`. `config` is the YAML as the
  agent wrote it, comments and all. A page's `type` is always `page`.
- Typed writes through the writer queue, interactive priority, each in one transaction with its change-log entry:
  - `ProjectDB.SaveConfig(ctx, ConfigSave)`: target `view:<id>` / `page:<id>`, op `save`, `before`/`after` the
    revisions as JSON (NULL `before` for a first save), one row entry (`i` or `u`) with the before-image.
  - `ProjectDB.DeleteConfig(ctx, ConfigDelete)`: op `delete`, row entry `d`. `Kind: ""` finds the view or page by ID
    alone, as `delete_config(id)` needs. A config that isn't there is `ErrNotFound`.
  - Reads: `Config(ctx, kind, id)` (kind `""` = either), `Configs(ctx, kind)` (live ones, by ID).
  - `SaveConfigTx` and `ConfigsTx` for a transaction the caller holds, for P3-04's migration dependents.
- Undo: `revertConfig` is registered in `reverters` for both tables, so *Undo turn*, *Redo turn*, rewind's
  *Undo changes*, *Revert run*, the chip's `UndoChange`, partly undone turns, skips and the conflict check all
  work as for memory sections. Undoing a first save deletes the config, undoing a delete restores it, and every
  revert writes a new revision (so a writer holding the old revision is refused). `internalNouns` gives
  "1 view", "2 pages" in summaries.
- Events: `ProjectDB.OnConfigs(fn)` gets `[]ConfigChange{kind, id, op, revision}` after each committed save, delete,
  undo or redo. `UndoResult.Configs` lists what an undo put back. Checks (`UndoOptions.Check`), refused undos and
  saves that change nothing send nothing.

**Project and app**

- `project.Publisher` has a third method, `Configs(ConfigsChanged{Project, Changes})`. The manager sets
  `OnConfigs` each time it opens a project's files, so every write path is covered, including undos run by the
  orchestrator.
- `app.Publisher.Configs` emits `project:configs` (`EventConfigs`). The native stores ignore unknown events, so
  nothing changes in the UI yet.
- `app.ConfigService` (`Bound.Config`): `List(ctx, project, kind)` (kind `""` = views then pages, without YAML) and
  `Read(ctx, project, kind, id)` (with YAML). Read-only; the agent's tools come in P3-03.
- `internal/native/chat/logic/undotext.go`: `ItemName` names "the view X" / "the page X" in the conflict dialog,
  instead of "row view:X of _jenab_views". This is a text helper, no UI change.

## Decisions

1. **Views and pages share IDs.** `delete_config(id)` and `read_config(id)` (SPEC 8.1, P3-03) take an ID alone, so
   a save of a page with a live view's ID (or the reverse) is refused with `ErrConfigIDTaken` (matches
   `ErrInvalid`). A deleted config's ID is free again. Undoing a delete whose ID the other kind has taken since is
   a conflict (`ConflictConstraint`, `Reached` = the other config).
2. **Deletes keep a tombstone row** (empty `config`), as memory sections do, so the revision keeps counting and an
   old revision is never valid again. Lists and reads leave tombstones out.
3. **A `title` column**, given by the caller after validation (like `type`). The Pages tab and the project card list
   titles, and the store must not parse YAML.
4. **No stored parsed form.** The ticket allowed one. I left it out because a parsed form stored in the database
   goes stale when P3-02's Go types change in a later version, and every such change would need a format step.
   Parsing a validated config on read is cheap; P3-05 can cache by `(id, revision)` in memory.
5. **`Expected` revision is optional** (`0` = save over whatever is there). The agent tools have no
   expected-revision argument (8.1); a UI editor can pass it to get a `*ConfigConflictError` with the current
   config.
6. **A save of the same YAML, type and title writes nothing** (op `""`, no change entry, no event), as for memory.
7. **ID rule in the store:** 1–64 letters, digits, `_` or `-` (and ZWNJ and combining marks, for Persian), starting
   with a letter or digit (`CheckConfigID`, exported for P3-02). It keeps `view:<id>` targets parseable and
   rejects `/` and `:`. Further YAML/schema rules are P3-02's.
8. **The store checks only what it must:** a source, a known kind, the ID, non-empty valid UTF-8 YAML, a view's
   type. Nothing about YAML structure.

## Fixes after review

Main-Session's review found two Low points; both are fixed:

1. **Events in commit order.** `project:configs` used to go out after `Do` returned to each caller, so two writers
   could publish revisions 2 and 1 in that order. Saves, deletes and undos now run the hook on the writer, right
   after the commit and before the next write starts (`doThen` in `writer.go`, a `Do` with an after-commit
   function). The hook must not block, which `app.Publisher` already promises. A panic in the hook loses its
   event but not the write's answer. `TestConfigEventsInCommitOrder`: 40 writers race on one view while the hook
   holds the first event; the revisions arrive 1 to 40. It failed every run on the old code.
2. **IDs are NFC.** `NormConfigID` (exported, for P3-02) normalizes an ID to NFC on save, read, delete and the
   taken check, so the composed and decomposed spellings of a Persian or accented ID are one config, stored and
   logged (`view:<id>`) in the composed form. `TestConfigIDNormalized` saves a view under an NFD Persian ID (آ as
   ا + U+0653), saves, reads, lists, refuses a page and deletes it through either spelling. P3-02 should
   normalize the YAML's `id` the same way before comparing it with the saved ID.

## Deviations from the ticket

- Added the `title` column (decision 3) and left out the parsed form (decision 4).
- The event is named `project:configs` as the ticket suggested; its payload is
  `{project, changes: [{kind, id, op, revision}]}`. An undo sends one event with all the configs it put back.
- The ticket lists `migration:<id>` as a source; nothing writes it yet (P3-04 will, through `SaveConfigTx`).
  `SaveConfigTx` doesn't publish: the caller publishes after its commit. P3-04 has to do that for the
  dependents a migration saves (its `Dependent.Update` returns only an error today).

## Tests

New tests:

- `internal/store/configs_test.go`
  - `TestConfigSaveReviseDelete`: save, same-save no-op, revise, stale revision, the shared ID rule, list and read,
    delete by ID alone, delete twice, save again after a delete, bad saves, a Persian ID. Checks the change-log
    entries and row entries (`i`, `u`, `d`, before-images).
  - `TestConfigUndo`: undo and redo, a writer holding a pre-undo revision, a conflict with a later save, skip all,
    undo order (newest first) and redo order (oldest first), undo of a first save (deletes) and of a delete
    (restores), the chip's `UndoChange` and `ChangeUndone`.
  - `TestConfigUndoPartlyWithRows`: a turn with rows, a page and a view; the view is skipped for a later save, so the
    turn is partly undone (`UndoState` has both sides, `Last` is an undo); the rest is undone after the later save
    is; redo puts everything back.
  - `TestConfigUndoIDTaken`: undo of a view's delete after a page took the ID is a constraint conflict.
  - `TestConfigEvents`: events for save, delete and undo; none for no-op saves, refused saves, checks, refused undos,
    or row-only undos.
  - `TestConfigFormatUpdate`: a format-5 `project.db` with memory is backed up to
    `snapshots/project-pre-update-*-v5.db` and migrated to 6, its data kept; the backup still reads as 5; configs
    save and undo in the migrated file; a format-5 app refuses the new file with `ErrNewerFormat`.
- `internal/app/configs_test.go` `TestConfigService`: list and read, a bad kind, not found; events reach the app's
  `Publisher` from saves, from *Undo turn* and *Redo turn* through the orchestrator, and from rewind's *Undo
  changes*, but not from the rewind itself.
- `TestPublisher` covers `project:configs`; `TestItemName` covers views and pages.
- After review: `TestConfigEventsInCommitOrder`, `TestConfigIDNormalized` (see "Fixes after review").
- Changed: `TestSkipEntriesOldUndos` (see below).

Checks:

All with `GOPROXY=off GOMEMLIMIT=1500MiB`, on Windows:

- `go build ./...`: ok
- `go vet ./...`: ok
- `go test -p 2 -short -count=1 ./...`: all 43 packages with tests ok. The first full run failed one existing test,
  `TestSkipEntriesOldUndos`: it fakes a format-4 file by setting `user_version = 4` on a current file, so step 6
  ran again on a file that already had the new tables. The test now drops `_jenab_views` and `_jenab_pages` before
  lowering the version, as a real format-4 file wouldn't have them. Step 6 itself stays a plain `CREATE TABLE`.
- `gofmt -l` on the changed files: clean.
- `go run ./cmd/jenab-scenarios check scenarios`: exit 0, all 21 sets ok.

No live model calls were made, and the app wasn't launched.

## SPEC lines for Main-Session to change

- **2.2**, table of internal tables:
  - `| `_jenab_views` | View configs with revision |` →
    `| `_jenab_views` | View configs (the YAML as written) with type, title and revision (5) |`
  - add after it: `| `_jenab_pages` | Page configs, the same way (5.9) |`
  - optionally, under the table: "A deleted view or page keeps its row with no config, so its revision goes on.
    Views and pages share one set of IDs, since `delete_config` and `read_config` take an ID alone."
- **2.5**, `target` row:
  `` `table:<name>`, `view:<id>`, `pipeline:<id>`, … `` → `` `table:<name>`, `view:<id>`, `page:<id>`, `pipeline:<id>`, … ``
- **2.8**, Storage format version, add after the step 5 bullet:
  "`project.db` step 6 (P3-01) adds `_jenab_views` and `_jenab_pages`."
- **5.1**, `id` row: "Unique view ID" → "Unique among the project's views and pages".
- **5.9**, near "A page changed by another chat refreshes live": add "Every committed save, delete, undo or redo of
  a view or page sends `project:configs` with the IDs, kinds, ops and new revisions."
- Outside SPEC (if Main-Session keeps them current): the event lists in `Docs/Code-Design/CODE-OUTLINE.md` (line
  with `EventDelta`) and `Docs/Code-Design/QUESTIONS.md` Q32 could name `project:configs`.
