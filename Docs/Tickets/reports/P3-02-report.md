# P3-02 report: validating views and pages

## What was built

### `internal/view` (new package, no UI)
- **Loader** (`load.go`)
  - Reads YAML with go.yaml.in/yaml/v4, walking the node tree into a JSON-like value. It keeps each JSON pointer's line:col for the key, the value and the container.
  - Refuses:
    - anchors, aliases and merge keys
    - tags, on scalars, maps and lists
    - non-text keys and duplicate keys
    - NaN and infinity
    - numbers with leading zeros
    - depth over 64 and size over 256 KiB
    - a second YAML document after `---` (a lone `---` at the end is no document)
  - Reads a whole number written with a zero fraction (`8.0`) as the integer 8, as JSON does. So every integer field reads it: span, page_size and decimals.
  - YAML errors carry a fix hint, for example "quote this value: it contains `: `".
- **Issues** (`issue.go`)
  - `Issue{Step, Path, Line, Col, Msg, Warning}` and `Result`, sorted by step, then by place.
  - Message form: `line 10:14 columns[2].field: 'avg' is not a result column of the query; it has date, price`.
  - Warnings start with `warning: ` and don't make `Result.OK()` false.
  - A `Result` lists at most `MaxIssues` (50) errors, and apart from them at most 50 warnings. `MoreErrors` and `MoreWarnings` count the rest. `String()` closes the errors with "and N more errors; fix these first" and the warnings with "and N more warnings". The heading counts every error.
- **JSON Schemas** (`schema/*.json`, draft 2020-12, embedded)
  - Shared parts: `defs.json`.
  - Views: `table`, `chart`, `stat` and `form`.
  - Pages: `page`, plus the page blocks `view`, `heading`, `text`, `card`, `filter`, `button` and `image`.
  - Each schema closes its objects with `additionalProperties: false`.
- **Schema check** (`schema.go`)
  - Uses santhosh-tekuri/jsonschema/v6 and keeps only leaf errors. A failed anyOf/oneOf counts as one error.
  - Writes its own messages for type, required, unknown field, enum ("money is not one of text, number, …"), const, length, items, number of fields, range and pattern (with a currency-code message). A null value reads as `null`.
  - An unknown field gets a did-you-mean from the fields its schema object allows: `rwos: unknown field; did you mean rows?`.
  - Unknown and missing fields, and wrong whole objects, point at the key. Everything else points at the value.
- **Block registry** (`registry.go`)
  - `Register[T](reg, Unit[T]{Type, Kind, Schema, Container, Check, Component})` registers one unit.
  - `Kind` is `Data` (a view type) or `PageBlock`.
  - A unit brings:
    - its JSON Schema
    - a Go check that gets the parsed spec through `*Check`
    - a `Component` slot for the kit to fill (`SetComponent`)
  - `Default = Builtin()` holds the four view types and the seven page blocks. Tests build their own registry from `Builtin()`.
  - Page blocks are known by their key, so `Register` refuses a page-block type that is `span`, `rows` or a field of a registered block (`tone`, `alt`, `confirm`, `open_page`…). It also refuses a block whose schema has a registered block type as a field. A data type is a value of `type`, so it is free.
- **Parsed types** (`types.go`) for P3-05 and P3-09:
  - `View` with `Spec` as `*Table`, `*Chart`, `*Stat` or `*Form`, or a made-up unit's type.
  - `Page` with `Rows [][]Block`. Each `Block` holds its spec, for example `*ViewBlock` or `*CardBlock`.
  - Spans are filled in: the blocks without a span share what the others leave of the 12 (see Decisions).
  - `ParseView` and `ParsePage` read a config without checking it, for configs that were saved already.
- **Validator** (`validate.go`, `check.go`, `blocks_data.go`, `blocks_page.go`)
  - `Validator{Reg, Env}` has three methods: `.View(ctx, src)`, `.Page(ctx, src)` and `.Config(ctx, src)`. Each returns the parsed type, a `Result`, and an `error` only for a hard failure: ctx, a closed DB, or something unexpected.
    - `Config` tells views from pages. A config with no `type` is a page, unless it has a `query` or `fields` and no `rows`. So a page with `rows` misspelled is checked as a page.
    - `View` given a config with rows says "this config has rows, so it looks like a page: save it as a page".
  - `Env` is the project, an interface. `ProjectEnv{DB}` is the real one, and a nil Env skips steps 3–5.
  - All issues come back together, in SPEC 10's order. Views and pages use steps 1, 3, 4 and 5.
  - The Go checks run on a best-effort decode even after schema errors. A Go issue under a path that already has a schema issue is dropped, so one mistake gives one message. A field given as `null` counts as given, so `set: null` gets only the schema's error.
  - The ID is normalized with `NormConfigID` (NFC) before the schema and `CheckConfigID` see it. So a decomposed ID is measured as it will be saved. Views and pages share one ID space, so a view can't take a page's ID.
  - `run_pipeline` always fails, in actions, buttons, row actions and form submits, with "pipelines can't be saved yet, so there is no pipeline X to run; remove run_pipeline for now". Phase 4 replaces `Check.Pipeline`.
  - Did-you-mean suggestions use OSA distance (a swap of two letters counts as 1) or a common prefix. They cover columns, tables, views, forms, pages and unknown fields.
- **No network and no LLM call.** Every model gets the same checks.

### Supporting changes
- `sqlguard.Columns(conn, q)` returns a prepared query's result columns with their origin table and column, from modernc's ColumnInfo. A test checks that origins survive the probe subquery.
  - For a compound SELECT (UNION, INTERSECT, EXCEPT) at the top, each arm is prepared on its own, with the query's WITH clause and without the closing ORDER BY and LIMIT. A column keeps its origin only when every arm reads the same table column.
  - A compound inside brackets (in a subquery, a CTE or a WHERE) is not split, and every column of the query counts as an expression. That is safe: an `image` or `file` format on such a query fails, saying the column must be selected as it is.
- `sqlguard.OuterClauses(q)` finds `ORDER BY`, `LIMIT` and `OFFSET` at depth 0, with line:col. It counts `OFFSET` only after a `LIMIT`, because elsewhere it can be a name (`SELECT date AS offset`).
- `sqlworker`:
  - `Request.Prepare` prepares a query without running it: layers 1–3, then columns and sources.
  - `Result.Sources` returns those origins.
  - The protocol carries both. A sources count that isn't 0 or the column count is refused.
- `store`:
  - `ProjectDB.QueryColumns(ctx, q)` calls the worker with a 2 s timeout.
  - `ProjectDB.TableShape(ctx, name)` is `DescribeTable` without the row count. That count reads the whole table, and a save check shouldn't.

## Decisions
- **Spans (Main-Session's decision).** Blocks without a span share what the given spans leave of the 12 columns. When it doesn't divide evenly, the first of them get one more: `[span 3, {}, {}]` is 3, 5, 4.
  - If the given spans add up to more than 12: "the spans of this row add up to N; a row has 12 columns".
  - If they leave fewer columns than there are blocks without a span (12 or more given, for example), the message is: "the spans given in this row add up to N, which leaves K of the 12 columns for M blocks without span; a block without span shares what is left, at least 1 column each…".
- **Checkbox and NOT NULL (Main-Session's decision).** A checkbox covers a NOT NULL column. In a form, an unchecked checkbox writes false, never NULL (P3-08's FormBlock sends false).
- **Result-column names are matched as written**, case-sensitively. The runtime's rows (P3-05) and the kit key cells by the name the query gives. Table and column names in forms, `rows_from` and `set` are matched case-insensitively, as SQLite matches them. A field that differs only in case gets "'title' is not a result column of the query; it has id, Title; names are matched as written, so write it as Title". It never gets a "did you mean" with the same spelling. P3-05 should match result columns as written too.
- **Parameters.** A config binds only `:name` parameters, because SQLite treats `@from` and `$from` as other parameters than `:from`.
  - `@name` and `$name` are errors: "write :from, not @from: only :name parameters are bound". This holds in a view's query and in a form's `load`.
  - An options query takes no parameters, since nothing binds them: "an options query takes no parameters, so :min would always be NULL; write the values into the query".
- **Step numbers.** Page structure checks are step 1, as SPEC 10 lists them: rows, spans, card in card, filter binding, filter blocks shown once, and tones. The page's references to views, pages and forms are sorted under step 3 with the views' references, so a model sees one kind of fix together. A page without `rows` gets the schema's error, not also "binds to no block" for each filter.
- **SQL errors are issues.** SQLite errors become issues at `/query`, cleaned of "SQL logic error: " and " (1)". So do guard refusals and a prepare that takes longer than 2 s. Only ctx, a closed DB or unknown errors are hard errors.
- **Each query is prepared once.** The result is cached by text, so an options query used twice is prepared once.
- **Messages name the fix.** Examples:
  - "T.c is not an object_ref column; mark it object_ref in a migration"
  - "'x' is not a result column of the query; it has a, b"
  - "no form X; X is a page"
- **Relative dates.** `RelativeDate` is a copy of the kit's parser in `view`, because the kit imports MyGo's ui and `view` must not. It also refuses a `-` sign on the amount.
- **Parameter names.** A name starts with a letter, then letters, digits and `_`, or any non-ASCII character except U+FEFF, so Persian names work.

## Deviations
1. **LIMIT and OFFSET in a table query are errors, not ignored.** The runtime wraps the query with its own LIMIT and OFFSET (5.3), so the query's own ones would quietly cut pages. ORDER BY stays a warning, as SPEC says. Stat allows all three (5.10).
2. **Spans:** resolved by Main-Session's decision above. SPEC 5.9's wording needs the change below.
3. **Parameters that bind to nothing are warnings.** A `:name` in the query with no param or filter is NULL unless a page filter binds it. A param or filter the query doesn't use is a warning too. A positional `?`, `@name` and `$name` are errors: no filter can bind them.
4. **An options query returns exactly one column and takes no parameters.**
5. **A `file` field needs a `prefix`.** It must pass `bucket.CheckPrefix`, and only file fields may have one.
6. **NOT NULL coverage and columns a form can't write.**
   - A checkbox counts as covering a NOT NULL column (decided above).
   - Columns with a default, generated columns and an INTEGER PRIMARY KEY need no field.
   - A form field or row-action `set` on a generated column, or on `_run_id` / `_fetched_at`, is an error. Pipelines write those (SPEC 6).
7. **Forms.**
   - An `open_form` to an insert form from a row action is a warning, since the row's key is ignored.
   - A field missing from the `load` query is a warning.
   - `update` needs `load`, and `load` may only use `:key`.
   - An update form can't list its key column as a field: "id is the key of the row this form updates, so the form can't change it; leave the field out".
8. **SPEC 9.3's `btc_price_table` fails with its run_pipeline action**, and so does 5.9's page, until Phase 4. TestRunPipelineFailsForNow checks the message.

## Review fixes (Main-Session's review)
Each fix has a test.

| Point | Fix | Test |
|---|---|---|
| Span decision | `shareSpans` / `fillSpans` and the rows check | TestSpans, TestPageChecks (default spans, no room for a default span) |
| M1 `8.0` | The loader gives int64 for a whole float | TestPageChecks (float span), TestSpans |
| M2 `@name`, `$name` | `namedParams` refuses them in the query and in `load` | TestParamBindings (`@name`, `$name`), TestFormChecks (load @key) |
| L1 `offset` as a name | OFFSET only after LIMIT | TestOuterClauses |
| L2 pie | Both errors reported | TestPieErrors |
| L3 case | Case-sensitive, with "write it as" | TestColumnCase |
| L4 options params | Refused | TestFilters (options query params) |
| L5 second document | Refused | TestOneDocument |
| L6 cap | 50 errors and 50 warnings, with "and N more" lines | TestIssueCap |
| L7 ID | NFC before the schema and CheckConfigID | TestIDNormalizedFirst |
| L8 UNION origins | Per-arm agreement; a nested compound has no origins | TestColumnsCompound (sqlguard), TestTableChecks (image through a union) |
| L9 type names | Register refuses clashes | TestRegistryNames |
| L10 messages | `null`, field counts, set: null, rows/rwos, unknown-field hints | TestMessages |
| L11 update key | Refused as a field | TestFormChecks (update key field) |
| L12 store | Direct tests | TestQueryColumns, TestTableShape (store) |

## Test results
Environment: `GOPROXY=off GOMEMLIMIT=1500MiB`, after the review fixes.

- `go build ./...` and `go vet ./...` are clean.
- `go test -p 2 -short -count=1 ./...`: every package passes.
  - In the first round, `internal/scenario` `TestRunTimeout` (2 s limit) failed once under load and passed on its own. It passed in this run.
- `gofmt -l` on the changed files (CR stripped) is clean.
- `go run ./cmd/jenab-scenarios check scenarios`: all sets ok.
- `TestOnlyTheKitImportsMyGoUI` passes. `view` imports no UI.

Tests in `internal/view`: 28 test functions with 144 table cases. Each case has a passing and a failing config.
- **TestSpecConfigs:** 9.3's six views and 5.9's page (without run_pipeline) validate against a fixture Bitcoin project:
  - `btc_prices` and `btc_news` (WITHOUT ROWID), `watch_keywords`
  - a `docs` table with an object_ref column and a generated column
  - a composite-key table
  - a stored bucket object
- **TestManyErrors:** one config with 17 errors across steps 1, 3, 4 and 5. Each error has its path and line, in step order.
- **TestMessageForm:** checks SPEC 10's exact message form.
- **TestMadeUpBlock:** registers a made-up `kanban` page block and a `gauge` view type, and validates a page that uses them.
- **Per-check tests:** TestParse, TestSchema, TestID, TestFilters, TestParamBindings, TestSQL, TestTableChecks, TestRowActions, TestChartChecks, TestFormChecks, TestPageChecks, TestSpans, TestRegistry, TestParseSaved and TestNoEnv, plus the review tests in the table above.

Also:
- `sqlguard`: TestOuterClauses, TestColumns and TestColumnsCompound.
- `sqlworker`: TestPoolPrepare (prepare through the pool, the protocol round trip, and a bad sources count).
- `store`: TestQueryColumns and TestTableShape.

## SPEC lines Main-Session should change
- **SPEC 10, step 1:** "Parse YAML/JSON and check it against the format's JSON Schema". JSON configs reach the validator as YAML, a superset of JSON, through the same loader. Suggest: "Parse the YAML (JSON is YAML too)…".
  - Also list what the loader refuses: anchors, aliases, merge keys, tags, duplicate keys, NaN and infinity, leading zeros, a second document, depth over 64, size over 256 KiB.
  - Also: `8.0` is the integer 8, and the ID is checked in NFC.
- **SPEC 10, step 1, page list:** "spans adding up to at most 12" → "spans adding up to at most 12, with each block without span getting at least 1 of what is left". Add "the page's ID is not a view's ID, since views and pages share IDs". Also say the existence checks for `view`, buttons and action targets are reported as step 3, as the code does, or move them to step 3 in the text.
- **SPEC 10, step 3:** "every `open_form` and `run_pipeline` target exists". Add "until pipelines exist (Phase 4), every `run_pipeline` fails". Also: `open_form` must name a form view, and `open_page` a page.
- **SPEC 10, step 5:**
  - Add that a table query's outer `LIMIT` and `OFFSET` are errors and its `ORDER BY` a warning.
  - Add that `?`, `@name` and `$name` are errors (only `:name` is bound), and a `:name` with no param or filter is a warning.
  - Add that result columns are matched as written (case-sensitively), and an `image` or `file` column must come straight from an object_ref column in every arm of a compound SELECT.
  - Same in 5.3: "The query does not contain…" → "…`LIMIT` or `OFFSET` fails validation".
- **SPEC 10, after the list:** "All errors are returned together" → "All errors are returned together, at most 50, then 'and N more'; warnings are capped apart".
- **SPEC 10, schema check:** "Checked in Go instead" now also has:
  - the 12-column span sums
  - one action per button, row action or action
  - select options and defaults
  - date defaults (`today-90d`, `start_of_month`, an ISO date)
- **SPEC 5.9 span rule:** "The default is 12 divided by the number of blocks in the row." → "A block without span shares what the others leave of the 12 columns, the first ones one more when it doesn't divide; if nothing is left for it, the row fails validation."
- **SPEC 5.10 block registry:** a page-block type can't be `span`, `rows` or another block's field, and a block's fields can't be block types.
- **SPEC 5.5 forms:**
  - say a `file` field needs `prefix`
  - say an `update` form needs `load`, `load` uses only `:key`, and the key column can't be a field
  - say that in a form an unchecked checkbox writes false, never NULL, so a checkbox counts for a NOT NULL column
  - SPEC 10 step 4: "covered by a required form field" → "…by a required field or a checkbox"
- **SPEC 5.2 filters:** a `select` needs `options`, and only a select has them. An options query returns one column and takes no parameters. A `file` control has no default.
- **SPEC 2.2 or wherever the worker protocol is described:** a prepare-only request returns result columns and their origin table.column. P3-02 uses it for the object_ref and key checks.
