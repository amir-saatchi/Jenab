# P3-06 report: forms and row actions write data

Branch `claude/elated-chatterjee-7fe662`. Go side only; P3-09 wires it into the page screen. Nothing in `Docs/` was edited, and nothing imports the kit.

## What was built

- **`internal/viewwrite`** (new package). The writes a view makes, on one `*store.ProjectDB`:
  - **`Writer.Open`** reads a form for showing: fields, select options, an insert form's defaults, or the row an update form's `load` reads by `:key`.
    - The load and options take the views' read-only path: `ViewQuery` and `viewrun.Runner.Options`, the same code as the filters.
    - A date field's relative default (`today`, `today-7d`) is resolved in the project's zone.
  - **`Writer.Submit`** handles an insert, an update by key, or a run_pipeline.
    1. Each value is read the way its control gives it:
       - number: `Int`, then `Number`, then the text with Persian and Arabic digits and separators;
       - date: `Date`, or text in `2006-01-02` form;
       - select: the option's text maps back to its value;
       - checkbox: false when unchecked, never NULL;
       - file: a key under the field's prefix;
       - text: blank becomes nil.
    2. Each value is then checked in Go against its column's declared type, using SQLite's affinity rules, before anything is written. Messages include "must be a whole number", "is not a number", "is not a date; write it as 2026-11-20" and "is not one of the choices".
    3. A broken constraint comes back on its field: `keyword: a row with this value already exists`.
  - **`Writer.RowAction`** runs `open_form` (with the row's key), `delete`, or `set`.
    - `set` takes literals, `now()` and `today()`, checked against the column types.
    - `confirm` returns the question without writing.
    - `bulk` takes at most 500 keys, written as one request with one change-log entry. Above 500, the answer is a problem: "At most 500 rows at once; 501 are chosen.".
  - **`Writer.Action`** runs a view's `actions` button: `run_pipeline` (with `confirm`), plus `open_page` and `open_form` as answers.
  - **The seam for `run_pipeline`:**
    - `viewwrite.Pipelines` is the interface `Run(ctx, project, pipeline, inputs, src) (id.Run, error)`.
    - `*viewwrite.RunningError{Pipeline, Run, Since}` is the "already running" answer. `Message(now)` gives "already running since 08:00" on the same day, with the date on another day.
    - A nil `Pipelines` answers "Pipelines can't run yet.".
    - Form fields go in as typed inputs: string, int64, float64, bool, or a date as `2006-01-02`.
- **`internal/store`**
  - **`*store.ConstraintError`** has `Kind` (`primary_key`, `unique`, `not_null`, `check`, `foreign_key`, `datatype`), `Table`, `Columns` and, for a CHECK, `Name`.
    - `checkErr` wraps every constraint error in it.
    - The error text and `errors.Is(ErrRowExists / ErrNotUnique / ErrInvalid)` are unchanged, and every existing store test passes.
  - **`store.MissingObject(err)`** returns the key of a missing file in an `object_ref` column.
- **`internal/app`**
  - **`FormService`** is the page screen's service. It is in `Bound.Forms`, and `Services.Pipelines` feeds it.
  - The upload key reservations moved from the upload route's handler into one package-level set. The composer's uploads and a form's file fields now share it, so two uploads at once never pick the same free key.

## Service API for P3-09 (`Bound.Forms`)

Every method follows the service conventions: ctx first, typed structs, errors as `*UIError`. The `viewwrite` structs carry JSON tags.

| Method | Returns | Notes |
|---|---|---|
| `Open(ctx, p, formID, key any)` | `viewwrite.Form{View, Title, Action, Table, Key, Fields []Field{Field, Label, Control, Required, Options []Option{Text, Value}, Problem, Prefix}, Values map[field]Value, Missing, Error}` | `key` is nil for an insert form. For an update form it is `Acted.Key` from the row action. |
| `Submit(ctx, p, formID, key any, values map[field]viewwrite.Value)` | `viewwrite.Submitted{Change, Table, Errors map[field]msg, Problem, Missing, Run, Notice, Removed}` | `OK()` is true when nothing stopped it. `Errors` maps onto the kit's `FormState.Errors`, `Problem` onto `FormState.Problem`, and `Missing` onto `FormState.Missing`. `Value` matches the kit's `FormValue` field for field. An update sends every field of the form. `Removed` lists the file fields whose upload went to the trash because the submit failed: clear them. |
| `Upload(ctx, p, formID, field, path, progress func(float64))` | `FormUpload{Key, Name, MIME, Size}` | `path` must come from the user's file picker. Only a regular file is read. Stores the file under the field's prefix with the file's name, made to follow the key rules. A taken key gets `-2`, `-3`, … `progress` runs on the calling goroutine from 0 to 1, so P3-09 posts it to the UI thread for `FormSpec.Upload`. Put `Key` in the field's `Value.Text`; it is good for one submit of that form and field in this app run. |
| `RowAction(ctx, p, viewID, action int, keys []any, confirmed bool)` | `viewwrite.Acted{Confirm, OpenForm, Key, OpenPage, Change, Table, Rows, Skipped, Run, Notice, Problem}` | `keys` are `viewrun.Table.Keys` of the chosen rows. When `Confirm` is set, ask that question, then call again with `confirmed`. When `OpenForm` is set, open that form with `Key`. |
| `Action(ctx, p, viewID, action int, confirmed bool)` | `viewwrite.Acted` | For a view's `actions` buttons. |
| `Undo(ctx, p, change id.Change)` | error | The notice's *Undo*. It works for a submit, a row action or an upload, and only for a change whose source is `form:…`; any other change is `KindNotFound`. Both refusals are `KindBusy`, as on the other undo paths; `errors.Is(err, store.ErrUndoConflict)` is a later change ("…changed again since…"), `errors.Is(err, store.ErrNothingToUndo)` is undone already or past the window. |

Errors: a missing view is `KindNotFound` with "There is no view X.". A view that no longer validates is `KindInvalid` with its `InvalidError` text. A request the view can't take is `KindInvalid` with "The view can't do this.", for example several rows for an action without `bulk`, an action index that isn't there, or a table view given to `Open`.

After any answer with a `Table`, P3-09 re-runs the page's blocks. No data event is sent (see below).

## Decisions and deviations

1. **No new `form_submit` request type.**
   - Writes use the existing typed requests: `Insert` (ConflictFail), `Update` and `Delete`, all at interactive priority, with source `form:<view_id>` (`viewwrite.Source`).
   - A request of at most 500 keys is one chunk, so one change-log entry, which is what `bulk` needs.
   - SPEC 7.2 still lists `form_submit` (see the SPEC lines below).
2. **The source is the same for submits, row actions and uploads.**
   - All three use `form:<view_id>`: the form's ID for a submit or upload, the table view's ID for a row action.
   - *Undo turn* touches none of them (tested), and each has its own undo by change ID, like file deletes.
3. **Constraint errors on fields.**
   - Unique or primary key:
     - on one column, "a row with this value already exists" on that field;
     - on several columns, the message names them and goes on the first field among them.
   - NOT NULL gives "needs a value".
   - A CHECK goes on a field only when its text names exactly one of the form's columns. Otherwise, including a CHECK SQLite reports by its constraint name, it is the form's problem.
   - A foreign key goes on the field when exactly one reference's column is in the form, and otherwise becomes the form's problem.
   - A delete stopped by a foreign key says "Not deleted: rows of T still refer to it.", naming the tables whose references don't cascade, or "other rows" when none are known. A `set` names the set column.
   - A STRICT datatype error gives "must fit the column's type T".
   - A file gone by the time of the submit gives "the file is no longer there; upload it again".
4. **Insert and update treat empty fields differently.**
   - An insert leaves out empty fields, so column defaults apply. An empty required field, or a NOT NULL column with no default, is "needs a value".
   - An update sets empty fields to NULL. A NOT NULL column there is "needs a value". An update must send every declared field; one lacking a field writes nothing, with the problem "Not saved: missing value for <field>.".
   - An update form's key field is shown but never written. Changing it is a field error: "is the row's key, which can't be changed".
   - An update that finds no row is `Missing`, with the problem "This row is no longer there.".
5. **Type checks are stricter than SQLite.**
   - Text that SQLite would store as text in a number column, such as "abc" in INTEGER, is refused.
   - A date goes only into TEXT, untyped, or NUMERIC-declared-for-dates (`DATE`, `DATETIME`, `TIMESTAMP`) columns.
   - A checkbox in a TEXT column is refused.
   - BLOB in a STRICT table is refused.
   - A non-STRICT column declared `ANY` has NUMERIC affinity, as in SQLite, so it takes numbers only.
6. **`now()` and `today()`.**
   - `now()` is RFC 3339 in UTC (`2026-11-26T10:00:00Z`), as SPEC 7.2 says pipelines write it.
   - `today()` is the project's local date.
   - At runtime both are also accepted in NUMERIC date columns. Validation (P3-02) allows them only in TEXT and untyped columns.
7. **Uploads of a failed submit go to the trash; those of a form never submitted stay.** When a submit fails, each file uploaded for it is deleted as file deletes are (moved to the bucket's trash), with the source `form:<view>`, and `Submitted.Removed` names its field. A file uploaded for a form that is never submitted stays in the bucket, with its own `form:<view>` change entry that its undo can remove. Nothing cleans those up.
8. **No data event.** Nothing tells other open pages that rows changed. The answer carries `Table`, and P3-09 re-runs the blocks.
9. **`run_pipeline` can't be saved yet.**
   - P3-02 validation refuses `run_pipeline` until Phase 4 ("pipelines can't be saved yet").
   - So through `FormService`, `Action` only answers `open_page` and `open_form` today.
   - The seam is tested in `viewwrite` with a fake: typed inputs, the source, "already running since 08:00", a missing pipeline, and "can't run yet".
10. **Select options are read again at submit,** so a value is checked against the current choices. A failing options query is an error on that field.

## Tests

- `internal/viewwrite/write_test.go`:
  - `TestInsert`: options and defaults; a typed write with one `form:add_keyword` entry; five type errors with nothing written; the UNIQUE keyword on its field; a CHECK on its field; an unchecked box writing false.
  - `TestUpdate`: load; a type error; an update and its undo; undoing twice; changing the key; a unique clash; a gone row; no key.
  - `TestFileField`: a key under the prefix, the wrong prefix, a gone file.
  - `TestRowActions`:
    - open_form;
    - delete with confirm and its undo;
    - a gone row;
    - set with `now()` and `today()`;
    - a literal that doesn't fit;
    - bulk of 500 as one entry with 500 row entries, and its undo;
    - 501 refused for both set and delete, with nothing written;
    - no row, several rows without bulk, and no such action.
  - `TestUndoTurnLeavesForms`, `TestPipelines` and `TestFit`.
- `internal/viewwrite/review_test.go` (review fixes): `TestNowFormat`, `TestForeignKeyMessages`, `TestBigKeys`, `TestDateZone`, `TestCheckPlacement`, `TestDuplicateKeys`, `TestFileKeys`. `TestUpdate` adds the missing-field case.
- `internal/app/forms_review_test.go` (review fixes): `TestFormUploadRegularOnly`, `TestFormUploadKeys`, `TestFormUndo`, `TestFormBulkDuplicates`, `TestUploadsShareReservation`.
- `internal/store/constraint_test.go`: the kind and columns of each constraint for insert and update, and `MissingObject`.
- `internal/app/forms_test.go`, `TestFormService`, runs through the service with saved and validated views:
  - a type error and a constraint error;
  - an upload with its progress, and a second upload of the same file getting `-2`;
  - both uploads as `form:add_doc` entries;
  - a submit with the file key, its undo, and a second undo giving `KindBusy`;
  - upload refusals;
  - open_form, then `Open` with its key;
  - a bulk delete of 500 after its confirm as one entry, and its undo;
  - 501 refused;
  - `KindInvalid` and `KindNotFound` cases.

## Checks run

All with `GOPROXY=off GOMEMLIMIT=1500MiB`:

- `go build ./...`: ok
- `go vet ./...`: ok
- `go test -p 2 -short -count=1 ./...`: all ok
- gofmt -l on the changed files, with CRs stripped: clean
- `go run ./cmd/jenab-scenarios check scenarios`: all ok

## Review fixes

Main-Session's review, each point with its final behaviour and test.

1. **`now()`** writes RFC 3339 in UTC, `2026-11-26T10:00:00Z`, as SPEC 7.2 has pipelines write it. `TestNowFormat` pins it at 13:30 in +03:30; `TestRowActions` checks it too.
2. **Foreign-key messages** take the operation.
   - A delete a child row stops: "Not deleted: rows of deals still refer to it.", naming the tables whose reference to it has no `ON DELETE CASCADE`, `SET NULL` or `SET DEFAULT`, or "other rows still refer to it" when none are known.
   - A `set`: "Not saved: owner refers to a row that isn't there.", using the set columns.
   - `TestForeignKeyMessages` covers a blocking child, a cascading child that doesn't block, and a set.
3. **Typed keys.** Every key (`RowAction`, an update form's `Open` and `Submit`) is read against the key column's type.
   - INTEGER takes an int64, its digits, or a whole float64 below 2^53. A float from 2^53 up could be another key rounded, so it is refused, as is a fraction, text that isn't digits, or a bool.
   - TEXT takes text only.
   - A refused key is a request error (`KindInvalid`), never a write.
   - `TestBigKeys` uses rows 2^53 and 2^53+1: the float is refused, and the int64 or the digits delete exactly the right row.
4. **`Upload(path)`** stats the path and refuses anything but a regular file before opening it ("Choose a file, not a folder.", "Choose a regular file."), then checks the opened file again. The doc comment says `path` must come from the user's file picker. `TestFormUploadRegularOnly` covers a folder and `os.DevNull`, a character device on Windows and Unix, so no skip is needed.
5. **Dates** write the year, month and day of the picked time in its own zone, never via UTC. `TestDateZone` submits local midnight in +03:30 (the previous day in UTC) and gets that day.
6. **`FormService.Undo`** reads the change's source first. Unless it is `form:…`, the answer is `KindNotFound` (Cause `store.ErrNotFound`), as for an unknown change. `TestFormUndo` tries an agent's change.
7. **File keys.** A file field takes only:
   - a key `Upload` handed out for that form and field in this app run, and not yet used by a submit, or
   - on an update, the row's current value.
   Anything else gets "is not a file uploaded to this field" on the field, even a file that is in the bucket under the prefix. The handed-out keys live in `FormService` (memory only) and reach `viewwrite` through `Writer.Issued`. `TestFileKeys` covers another row's file, another form's upload, the kept file and a new upload. `TestFormUploadKeys` covers the same through the service.
8. **An update lacking a declared field** writes nothing: "Not saved: missing value for added.". The case is in `TestUpdate`.
9. **CHECK placement.** A CHECK goes on a field only when its text names exactly one of the form's columns. A check on two columns, or one SQLite reports by its constraint name, is the form's problem. `TestCheckPlacement` covers one column, two columns and a named constraint.
10. **Orphan uploads.**
    - When a submit fails (field errors, a constraint, a problem, or an error), each file `Upload` handed out for it is deleted as file deletes are: it goes to the bucket's trash, under the same `form:<view>` source. `Submitted.Removed` names the fields, and the key is spent.
    - A failed delete is logged and the file stays.
    - Uploads for a form that is never submitted stay.
    - `TestFormUploadKeys` checks that the object is gone, the two change entries (upload and delete), that the key is spent, and that the never-submitted upload stays.
11. **Undo errors.** Both refusals stay `KindBusy`, as the link and bucket undo paths report a conflict. The `UIError`'s Cause tells them apart for `errors.Is`: `store.ErrUndoConflict` or `store.ErrNothingToUndo`. A conflict on several rows says how many. `TestFormUndo` checks both.

Added tests:
- 500 and 501 rows through `FormService` (`TestFormService`), and 500 rows each chosen twice, as int64 and float64, which is 500 rows and one entry (`TestFormBulkDuplicates`).
- Duplicate keys in `viewwrite` (`TestDuplicateKeys`): the confirm counts distinct rows, and 500 rows given twice fit in one bulk.
- `TestUploadsShareReservation`: while a composer upload of `same.pdf` holds its key, a form upload of the same name to an `uploads/` field gets `uploads/same-2.pdf`, and the composer's finishes as `uploads/same.pdf`.

## SPEC lines for Main-Session

- **7.2, `form_submit`:** drop it from the typed requests, or say that form writes use `insert`, `update` and `delete` with the source `form:<view_id>` (accepted deviation).
- **7.2, `now()`:** no change. Forms now write it as the table says, RFC 3339 in UTC; 5.3 could point to it.
- **5.5:** after "Constraint errors are shown on the field", consider adding: "Values are read as their control gives them (Persian and Arabic digits in numbers), and text a number column would keep as text is refused. An update sends every field, and its key field can't change the key. A file field takes only a file uploaded to it, or on an update the row's own. When a submit fails, its uploads go to the bucket's trash; a file uploaded for a form never submitted stays."
- **5.3, `set`:** consider "`now()` is the current time as RFC 3339 in UTC, as pipelines write it (7.2); `today()` is the project's local date. A row chosen twice counts once toward the 500."
- **2.6:** consider "A form's or row action's write is undone from its notice by its change ID; the same goes for a form's file upload."
- **5.3 or 5.5, validation:** a checkbox on a TEXT column fails at runtime. A P3-02 rule could refuse it at save time.

## Proposed Result paragraph

> **Result.** Forms and row actions write through the existing typed requests, one interactive request and one change-log entry each, with source `form:<view_id>`, so *Undo turn* leaves them and each notice undoes its own change. A new `internal/viewwrite` package checks form values against the column types in Go first. Values are read the way each control gives them, including Persian and Arabic digits. The store's new `ConstraintError` lets a broken UNIQUE, NOT NULL, CHECK or foreign key land on its field ("a row with this value already exists"). Row actions cover `open_form`, `delete` with confirm, and `set` with `now()` and `today()`; bulk is limited to 500 keys as one entry, and 501 is refused. A form's file field uploads under its prefix through the same key reservations as the composer, takes only the keys it handed out, and trashes them when the submit fails. `run_pipeline` goes through a `Pipelines` seam that Phase 4 fills, with "already running since 08:00" tested on a fake. `app.FormService` (`Open`, `Submit`, `Upload`, `RowAction`, `Action`, `Undo`) is ready for P3-09.
