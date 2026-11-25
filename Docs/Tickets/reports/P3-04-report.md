# P3-04 report: migrations and their dependents

Branch `claude/brave-brahmagupta-779aba`. A migration and the updates to the views and forms that depend on it now commit in one transaction or not at all. A view that no longer validates is marked in the project card, and its block shows an error instead of its data (R-31; SPEC 5.7, 7.5, 8.2).

## What I built

### Store: dependents in the migration's transaction

- **`store.DependentTx`** (new, `internal/store/deptx.go`) is how a dependent sees the migration's transaction. It shows:
  - the agent tables and their shapes after the steps;
  - the live configs, including the ones the migration has already saved;
  - the files in `_jenab_objects`;
  - the SQL check of a query (text check, EXPLAIN, prepare) on the migration's own connection, because the query workers can't see uncommitted DDL.

  `SaveConfig` saves through `SaveConfigTx` and always uses the migration's source, whatever the caller passes.
- **The `Dependent` interface** now takes a `*DependentTx`, not a bare `*sql.Tx`. `DependentLister.List` does too, and `DependentInfo` gained `Type`.
- **`run.all` in `migrate.go`** now runs in this order:
  1. It generates the migration's change ID up front, so the configs it saves get the source `migration:<id>`. `logMigrationAs` writes the entry under that ID.
  2. It asks each dependent kind which configs use a table the steps change (`Reached`), and checks those before the steps, to get a baseline.
  3. It runs the steps.
  4. It saves the updates. An error is pinned at `updates[i]`, for example `updates[1].config`.
  5. It checks the reached configs and the updated ones again.
- **What blocks the migration:** a dependent that the migration updated, or one that passed before it, must pass after it, or the migration fails with its problems. A dependent that already failed before and wasn't updated blocks only with an issue it didn't have before (see Review fixes, point 1). Otherwise it goes in `MigrationReport.Broken`.
- **`MigrationReport.Configs`** holds the config changes. They go out as `project:configs` right after the commit (through `afterCommit`), and never for a try or a rollback.

### Views and forms as dependents

- **`internal/view/migrate.go`** (new) registers the dependent kinds:
  - `view`, which checks and lists every saved view, forms included;
  - `form`, which is only accepted as the kind of an update.

  `project.Manager` registers both when it opens a project (`view.AddDependents`).
- **An update's config** is the view's whole YAML, as a JSON string, as `read_config` shows it. A JSON object is also accepted. The update is validated against the new schema with `TxEnv`, which is the `view.Env` over a `DependentTx`. It is refused when:
  - its id differs from the update's id;
  - no view has that id (a page id gets "pages only show views, so they need no update");
  - a form update names a non-form;
  - any validation error remains.
- **`ColumnsUsed`** lists what a view uses, as `table.column`, or a table alone when it names no columns:
  - names in the view query, filter option queries, the form's `load` query and field option queries, matched against the columns of the tables those queries read;
  - a form's fields and key, as columns of `submit.table`;
  - `rows_from.key` and the row actions' `set` keys, as columns of `rows_from.table`.
- **`store.IsSQLiteError`** lets the validator recognise SQLite's own errors from the migration's connection, as it already does for the worker's.

### Schema agent

- It gets `read_config` beside `describe_table` and `query`.
- Its task lists each dependent that uses a table, with its type and title, for example `view:edit_news (form, "Edit news"): uses btc_news.title, btc_news.reason, btc_news.url`. A line then says that a dependent of a renamed or dropped table or column goes in `updates` with its whole new config, read first with `read_config`.
- `apply_migration` describes `updates.config` as the whole new YAML string, and the kind as `view` for views and forms.
- After a migration, the result also lists the dependents that were already broken and still are: "Still failing their check, as before the migration: …".
- `prompts/schema.md` and the `migrations` skill's Dependents section say the same. The skill also covers the failure and baseline rules.

### Broken views: card and page screen

- **`view.BrokenViews(ctx, db)`** validates every saved view against the project as it is now. It caches the result per project, keyed by schema version and config stamp, the same pair the agent watches to rebuild the card. `view.Invalid` gives an `*InvalidError{ID, Type, Title, Issues}` for one view.
- **Project card:** `view.Card` marks a broken view on its line as `- old_chart (chart): "Price" — broken: <first issue> (and N more); read_config shows it, save_view fixes it`. The short card lists "Broken views (…): a, b.". `store.ConfigRefs` gained `Broken`.
- **Page screen (data for P3-09):**
  - `viewrun.Block` gained `Issues []string`.
  - `viewrun.ErrorBlock` builds the block of a view that couldn't be loaded. For an `*InvalidError`, it also fills Type, Title and Issues, and `Error` says "The view X no longer works with the project: <first issue> (and N more). Ask the agent to fix it."
  - `ViewService.loadView` checks the view before running it. `Page` puts the error on the view's block. `View` returns the block with the error instead of failing the call.
  - If the check itself fails (not the view), the view runs as before and shows its own errors.

### Scenario

- **New assert `views_valid`** (no fields). It passes when every saved view validates at the end of the run. The transcript gets a "Broken views at the end" section when any view fails.
- **New set `scenarios/schema-dependents`:**
  - `data.sql` is the views-and-pages Bitcoin project plus an `edit_news` update form over `btc_news` that uses `reason`.
  - `D1_rename_column` asks "In btc_news, rename the column reason to topic."
  - The checks:
    - the column is renamed;
    - `btc_news_list` and `edit_news` read `topic`, have no `reason` left (case-sensitive, so a "Reason" label may stay), and were saved by `migration:%`;
    - the other views are at revision 1;
    - `views_valid` passes;
    - all 4 news rows keep their value.
  - Reported only: the schema agent called `read_config`, at most one `apply_migration` and one `request_schema_change` call, and no failed turn.
  - `_offline.yaml` scripts a passing run.

  I made a new set rather than adding V6 to views-and-pages. Adding the form to that set's data.sql would break V5's exact list of the views that remain.

## Decisions

- **Validation uses the migration's own connection.** Query workers read committed state only, so validating through `ProjectEnv` inside the migration would check the old schema.
- **Baseline rule.** If any invalid view blocked every migration, one view broken by an undo, or by a change of the app, would block all schema work, including the migration that fixes it. Now only what the migration breaks, or what it updates, must pass. The rest is reported and marked.
- **Migration saves survive *Undo turn*.** They have the source `migration:<id>`, not `message:<id>`, so Undo turn leaves them alone, as it leaves the migration (SPEC 2.6). The test also undoes a view the same turn saved, and that one is reverted. If a turn saved a view and a migration in that turn then updated it, undoing the turn's save is a conflict ("a later change to an item"). That is intended, because reverting it would put the old column name back.
- **Forms are views.** There is one check over all views. `form` is only an alias kind for updates, so a model that sends `{"kind": "form"}` isn't refused for the word.
- **The broken-views cache** is keyed by schema version, config stamp and objects stamp. It forgets every project once it holds 16.

## Deviations

- SPEC 5.7 says open views re-validate "on next load". The check runs on every load, but it is cached by schema version and config stamp, so most loads don't validate again.
- The agent's card is still rebuilt only when the schema version or config stamp changes. Today no view error depends on the bucket (see Review fixes, point 3), so no mark is missed.
- No pipelines yet (Phase 4). The `Dependent` interface and `DependentTx` are ready for them.

## Review fixes

Main-Session's review, all 8 points. Each fix has a test in `internal/view/migrate_test.go`, or in `internal/store/migrate_test.go` where noted.

1. **The baseline excuses only old issues.**
   - Before the steps, the store keeps each failing dependent's issues, keyed by **target plus message text** (`view:old_chart` + "no such column: close").
   - After the steps, a dependent that failed before blocks the migration when it has any issue not in that set. All its issues then go back to the schema agent, and the old ones get the prefix "already before the migration: ". It is listed in `Broken` only when every issue is an old one.
   - Path plus message alone would miss your example. SQLite names only the first unknown name in a query, so a view that already names a missing `x` still says "no such column: x" after the migration drops `y`. So the view check also does this:
     - Before the steps, it keeps what each failing view uses (`ColumnsUsed`: tables and columns that exist then), through `DependentTx.Memo`.
     - After them, it adds an issue for each of those that is gone: "uses btc_prices.price, which the migration renamed or dropped".
     - It skips this for a view whose YAML changed in between, because an update must pass in full anyway.
   - Message text stays the key for the old issues. A rename the view doesn't use leaves its message as it was. A rename it does use is caught by the check of what it uses.
   - Tests: `TestMigrationBrokenViewNewIssue` (a view missing `close` that also uses `price`; renaming `price` is refused with both issues, and the same-reason migration still passes), and `TestMigrationLeavesBrokenView` (unchanged case).
2. **Only reached configs are checked.**
   - `Dependent` gained `Reached(t, tables)`, and `Check` now takes the targets.
   - `stepTables` gives the tables the steps change, in lower case, under old and new names (`rename_table`, `widen_table` with `to`).
   - Index, `insert_rows` and `copy_data` steps reach none, as they don't change what a view can use.
   - A step without a known table gives nil, which means every config. Today every shape-changing op names its table, so this is a fallback.
   - A view is reached when its queries, `submit.table` or `rows_from.table` name one of those tables, or when a query's tables can't be read (`sqlguard.Names` fails).
   - After the steps, the reached views plus the updated ones are checked.
   - Tests: `TestMigrationChecksReachedViews` counts validations: a rename in `btc_news` validates the 2 news views twice and none of the 6 charts; an `add_column` to `btc_prices` validates the 6 charts twice; an index validates nothing. `TestStepTables` is in the store.
   - **Timing** (`TestMigrationScopeTiming`, not in `-short`, 203 views of which 201 are charts over `btc_prices`, best of 3 runs, this PC): the same `add_column` took **about 17 ms** on `btc_news` (reaches 2 views) and **about 63–68 ms** on `btc_prices` (reaches 201, as every migration did before). These are simple one-table views; views with joins cost more per check.
3. **Objects stamp in the broken-view cache.**
   - `ProjectDB.ObjectStamp` is the count of files, the count of their versions and the newest `updated_at` in `_jenab_objects`, and it is part of the `BrokenViews` key. It changes on put, replace, rename and delete.
   - Test: `TestBrokenViewsObjectStamp`. A second `BrokenViews` call doesn't validate again. Adding a file and then deleting it each make it validate again.
   - The test checks that it validates again, not that a mark flips. No view error depends on a file today: `Check.Object` is only used for page image blocks, and a missing file there is a warning, which `BrokenViews` doesn't count.
4. **Nil view.** `Update` returns a `MigrationError` when the config doesn't load. It carries the validation errors, or "the config doesn't load as a view" when there are none, before `pv` is used. Test: `TestMigrationUpdateMissingView`, second case (`not: [yaml`).
5. **The same view updated twice.** `TestMigrationUpdatesViewTwice`:
   - `Updated` lists `view:news_table` once;
   - `Configs` has `news_table@2`, `edit_news@2`, `news_table@3`, in that order;
   - the view ends at revision 3 with the second YAML;
   - one `project:configs` goes out with those 3 changes.
6. **No-op save.** There is a comment at the update loop in `run.all`: a save that changes nothing is listed in `Updated` but sends no `project:configs`. The same test updates `price_chart` with its own YAML: it is in `Updated`, not in `Configs` or the event, and stays at revision 1.
7. **A missing view first.** `Update` checks that the view exists before it reads or validates the config. A missing view (or a page id) gives only that problem. Test: `TestMigrationUpdateMissingView`, where an invalid config for `nope` gives exactly `updates[2].id: there is no view nope to update; …`.
8. **A rollback with updates.** `TestMigrationRollbackLeavesNothing`:
   - one update passes and the other fails;
   - the sum of `_jenab_views` revisions is unchanged;
   - the row count of `_jenab_changes` plus `_jenab_change_rows` is unchanged;
   - no `project:configs` arrives.

Also:

- The `migrations` skill now says that a broken dependent that the migration breaks in a new way fails it, with its old issues marked.
- `Broken` now lists only the already-broken views the migration reaches. A broken view over an untouched table is still marked in the card, but the migration's report no longer names it.

## Live runs

`scenarios/schema-dependents`, `D1_rename_column`, 3 reps per model, on 2026-11-25. The prompts, tools and configs were the same for both models. No Haiku runs.

| Model | Runs passed | Checks | Time per run | Requests per run (main+schema agent) |
|---|---|---|---|---|
| gemini/gemini-3.5-flash-lite (`GEMINI_API_KEY`) | 3/3 | db_query 15/15, tool_called 3/3, views_valid 3/3 | 5m59s, 3m04s, 1m36s | 6+2, 4+2, 5+2 |
| nvidia/mistralai/mistral-large-4-1t-a52b (`NVIDIA_API_KEY`) | 3/3 | db_query 15/15, tool_called 3/3, views_valid 3/3 | 7m41s, 2m57s, 1m37s | 3+2, 2+2, 2+2 |

- **Every run went the same way:** the schema agent called `read_config` on `btc_news_list` and `edit_news`, then a single `apply_migration` with the rename and both configs as whole YAML strings. The migration applied on that first call, with no failed try. Both views and the form validated at the end.
- **The info-only `max_calls` failures came from provider trouble, not from the agent:**
  - flash-lite r1 and r2 called `request_schema_change` 3 and 2 times, because the schema agent stopped on Gemini 503s ("high demand") and the main agent asked again;
  - mistral-large-4 r1 called it twice, after the NVIDIA stream stalled for 60 s.
- **The models differed only in a label:** flash-lite renamed the list's "Reason" label to "Topic", and mistral kept "Reason". Both are fine.
- **Gemini usage:** about 36 requests in all, retries included (18, 11 and 7), well under the 400-a-day limit.

## Checks run

- `go build ./...`: OK.
- `go vet ./...`: OK.
- `go test -p 2 -short -count=1 ./...` with `GOPROXY=off GOMEMLIMIT=1500MiB`: all 46 packages with tests OK.
- `gofmt -l` on every changed or new `.go` file, with CRs stripped: clean.
- `go run ./cmd/jenab-scenarios check scenarios`: OK, `schema-dependents: 1 scenarios, 1 run now`.
- Offline: `jenab-scenarios run -model scripted:scenarios/schema-dependents/_offline.yaml scenarios/schema-dependents`: 1/1 passed, views_valid 1/1.
- New tests:
  - `internal/view/migrate_test.go`:
    - `TestDependentsListViewsAndForms`: what each view uses, and its type.
    - `TestMigrationUpdatesDependents`: commits all of it, with the source `migration:<id>` and one `project:configs` after the commit; a try saves nothing.
    - `TestMigrationDependentFails`: a dependent left out, an update that fails its check, or an update of a view that doesn't exist each stop the migration with its errors, and neither the schema nor any config changes, including the update that passed.
    - `TestMigrationLeavesBrokenView`: a view that was already broken doesn't block; it is listed in `Broken`, marked in the card, given an `InvalidError`, and unmarked once fixed. A view the migration breaks is a problem.
    - `TestMigrationSavesSurviveUndoTurn`: Undo turn reverts the turn's own save but keeps the migration and its config saves.
  - `internal/app/views_test.go`, `TestViewServiceBrokenView`: through the project manager, a rename that breaks views is refused. A broken view's block on a page, and on its own, has the error and its issues.
  - `internal/agent/schema_test.go`: the schema agent's tools include `read_config`.

## SPEC lines Main-Session should change

- **7.5, the transaction bullet:** add the scope and the baseline rule. "The migration checks the dependents that use a table its steps change, before and after them, and the ones it updates. A dependent the migration updates, or one that passed before it, must pass after it. One that already failed before and isn't updated stops the migration only with an issue it didn't have before, including a table or column it used that the steps removed; then all its issues go back, the old ones marked. Otherwise it doesn't stop the migration; the report lists it, and the project card marks it broken."
- **7.5 or 2.6:** "Configs a migration saves have the source `migration:<id>`, so Undo turn keeps them, as it keeps the migration."
- **8.2:** "The schema agent has `read_config`; each dependent comes with its type and the columns it uses, and an update's `config` is the dependent's whole YAML as a string."
- **5.7, line ~1013:** "A view that no longer validates shows an error with its issues on its block (`Block.Issues`) and is marked in the project card with its first issue. Whether a view validates is cached by schema version, config stamp and objects stamp."
- **3.2 (project card):** "A view that fails validation is marked on its line: `— broken: <first issue> (and N more); read_config shows it, save_view fixes it`; the short card lists the broken views by id."
- **8.4 (scenario runner):** add the `views_valid` assertion.

## Proposed Result paragraph

P3-04 is done. A migration now validates the views and forms that depend on it in its own transaction, against the new schema, on its own connection. It saves the updated configs the schema agent sends, with the source `migration:<id>`, so *Undo turn* keeps them. It commits them with the schema change, or rolls everything back when any updated view, or any view that passed before, fails, or a view that was already broken gets a new issue, and the problems go back to the schema agent. It checks only the views that use a table the migration changes. Views that were already broken and get no new issue don't block: the migration reports them, the project card marks them with their first issue, and their blocks show an error with the issues (`Block.Issues`, for P3-09) instead of data. The schema agent gets each dependent with its type and the columns it uses, and reads configs with `read_config`. The new `schema-dependents` scenario (rename a column used by a table view and a form) passed 3/3 on gemini-3.5-flash-lite and 3/3 on mistral-large-4. Each schema agent read both configs and applied the migration on its first call.
