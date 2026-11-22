# P3-05 report — the view runtime

Branch `claude/intelligent-blackburn-7b1f65`. SPEC 5.2–5.4, 5.7, 5.10; R-40, R-41, R-42, R-44, R-48.

## What was built

- **`internal/viewrun`** is a new package. It turns a parsed view (`view.ParseView`), or a page (`view.ParsePage`), plus filter values into what each block draws. It needs no UI and doesn't import the kit.
  - `Runner{Source, Now}`: `View(ctx, v, req) Block` runs one view. `Page(ctx, p, views, req, out) error` runs a page's data blocks in parallel and calls `out` once for the page's filters, then once per block as it answers.
  - `Source` is two methods of `*store.ProjectDB`: `ViewQuery` and `ObjectsExist`. Tests could use a fake, but these use a real project.
  - `run.go` holds the requests, `Block` and `Answer`, binding and errors. The other files are `filters.go`, `table.go`, `chart.go`, `stat.go`, and `values.go` (typing values for formats).
- **`internal/store`** has two additions:
  - `ProjectDB.ViewQuery(ctx, q, args, max)` is the agent query's guarded path (all three layers, through the query worker) with the 2 s view deadline. `ViewTimeout` is exported.
  - `ProjectDB.ObjectsExist(ctx, keys)` checks a page's image and file keys against `_jenab_objects` in one query per 500 keys. A key that isn't a valid bucket key is reported missing.
- **`internal/app/views.go`** adds `ViewService`, reachable as `Bound.Views`:
  - `View(ctx, project, viewID, req)` and `Page(ctx, project, pageID, req, out)` load and parse the saved configs, then run them. A view the page names that is gone, or that doesn't parse, becomes an error on its block, not on the page.
  - `Start(key, project, pageID, req, out, done)` runs a page in the background under a key (as the chat the page is open in). Starting again under the same key cancels the run before it; the new run begins once the old one's queries have stopped, so no old answer arrives after a new one.
  - `Stop(key)` cancels the run, for when the page closes. It doesn't wait, so `out` and `done` may call `Start` and `Stop`; `done` says when the run has ended. A generation counter per key drops the answers of a run that was replaced or stopped, and `out` is never called under the service's lock.
  - Errors go through the usual `guard`, so a missing page or view is a `not_found` UIError.

### The rules, as built

- **Queries (5.7).** Each block runs on the read-only guarded path through the project's query workers, with 2 s per query, and waits for no writer.
  - A view query's 2 s start when a worker takes it (`sqlworker.Request.TimeoutOnSlot`). The wait for a worker is bounded by the page run's context, not by the 2 s. Agent SQL keeps its rule: the wait counts.
  - Only `:name` parameters bind, each as `sql.Named`. A query with `?`, `?1`, `@x` or `$x` is an error on its block that names the parameter.
  - A parameter takes the page filter's value if the page has that parameter, else the view's own filter, else the view's `params`, else NULL.
  - Blocks run in parallel and each one answers on its own. A timeout, a refusal, an SQL error or a panic is shown on that block only.
- **Filters (5.2).**
  - Defaults come first. Relative dates (`today-3m`, `start_of_month`, …) are computed from `Now()` in the project's zone; `Now` is the clock's local time. A value given as a relative date is computed the same way.
  - A value given as `""`, or as blanks, is cleared, as nil is. A cleared optional filter binds NULL. A cleared required filter takes its default, a required select with no value its first option, and its `Error` is "This filter needs a value." (`NeedsValue`). A required checkbox cleared is unchecked (false), with no error. An unchecked optional checkbox binds NULL.
  - A select's options come from its static list or its one-column query, which runs once per run even when several blocks share it, capped at 1,000. A view's options queries run in parallel with each other; a block's own run inside its block, in parallel with the other blocks.
  - A value given as the option's text binds the option's typed value, so `"2"` binds `2` for a numeric option.
  - A value that can't be read (a bad date or number) is shown as the filter's `Error`, and the filter binds its default. NaN and infinities, as numbers or as text (`NaN`, `Inf`), are "… is not a number".
  - On a page, a block's own filter whose parameter the page's filter covers is hidden, and the page's value binds.
- **Table (5.3).**
  - The query is `SELECT * FROM (<query>) ORDER BY "<field>" [DESC][, "<rows_from.key>"] LIMIT <size> OFFSET <page·size>`, followed by `SELECT count(*) FROM (<query>)`.
  - The count, with max 2, is skipped when the page itself tells the total: a page that isn't full and wasn't cut by the 8 MiB result cap, or an empty first page. A page the cap cut (`Full`) always counts, and has the notice "Showing the rows that fit in 8 MiB. Show fewer columns or rows a page." A count that hits its 2 s gives `Total = ManyRows` (-1), and the rows still show.
  - A page past the last is the last: an empty page after the first counts, and fetches the last page; `Table.Page` reports it.
  - `page_size` defaults to 50, and is at most 500 (`MaxPageSize`): the schema refuses more, and the runtime clamps a view that reaches it anyway.
  - The sort is any shown column except `image` and `file` columns. Anything else falls back to `default_sort`, and `Table.Sort` reports the sort used.
  - Values are typed per format.
  - Keys of `rows_from` come back in `Table.Keys`, for row actions.
  - Result columns are matched as written. A column that is gone is an error naming the columns the query has.
- **Chart (5.4).**
  - The query is `SELECT * FROM (<query>) LIMIT 10001`. Past 10,000 rows, the first 10,000 in query order are kept, with the notice "Showing the first 10,000 rows. Aggregate in the query to see them all."
  - Line and scatter are sorted by x (stable), with no downsampling. A date x becomes a day (midnight UTC); a datetime becomes its instant. A time outside the years 1 to 9999 is missing, as NULL is.
  - A line along text gets categories. A scatter of text is an error.
  - `series_by` gives each series its own x (line, scatter) or its own y per category (bar, pie), NaN where it has no row. Series come in the order the query first gives them.
  - Bars: past 1,000 bars, counting every series' bars, the first categories are kept, with the notice "Showing the first 1,000 bars. …".
  - Pie: rows pass through, and the kit folds the slices into 12. A negative value is an error naming the slice. Zero and empty values pass, and the kit skips them.
  - A row with no x is left out, on every chart type: a NULL x has no bar or slice. A y that is NULL, not a number, NaN or infinite is NaN, a gap.
- **Stat (5.10).**
  - The query runs as written, with `ORDER BY` and `LIMIT` allowed. Exactly one row is needed: none gives "The query returned no rows; a stat needs exactly one.", and more gives "…returned N rows; a stat needs exactly one, as with LIMIT 1."
  - The value is typed for its format (default `number`). `change` keeps its sign. A NULL change is nil, so there is no arrow. A NaN or infinite value or change, as a number or as text, is nil.
- **Formats.**
  - `number`, `currency` and `percent` give int64 or float64. A whole number past int64 stays exact as text.
  - `date` and `datetime` give `time.Time`, by the kit's own rule: a date alone, or midnight in its own zone, is a day; a zoneless time is UTC.
  - `boolean` gives a bool, `badge` text, and `url` and `text` as they are. UTF-8 bytes become strings.
  - `image` and `file` keys that are not in the bucket, or are not valid keys, become nil, so the cell is blank and has no broken link. The bucket check has a view's 2 s; when it fails or times out, every image and file cell is blank and the block's notice says why ("Can't check the files in the bucket, so they are left out: …"). The rows still show.
  - `format_options` (currency, decimals, tones) is copied through.
- **No LLM and no network (R-48).**
  - `TestViewServicePage` replaces `http.DefaultTransport` with one that fails and records every request, opens a page and a view through `ViewService`, and asserts that no request was made and the fake provider got no call.
  - `TestNoNetworkImports` walks `viewrun`'s imports through the module. Nothing it builds on may import `net` (sockets), `crypto/tls`, `internal/provider`, `internal/agent`, `internal/web` or `internal/native`, and `viewrun` itself may import nothing under `net/`.

## How the result types map onto the kit

`viewrun` defines its own types. The page screen (P3-06) maps them one-to-one:

| `viewrun` | kit | Notes |
|---|---|---|
| `Block.Error`, `Block.Notice` | `BlockState{Error, Notice}` | `Loading` is the page screen's, until the block's answer arrives. |
| `Table{Columns, Rows, Total, PageSize}` | `TableSpec{Columns, Rows, Total, PageSize}` | `ManyRows` = `kit.ManyRows` = -1. Rows hold string, int64, float64, bool, `time.Time` or nil, as `TableSpec.Rows` takes. |
| `Column{Field, Label, Format, Options, Align}` | `BlockColumn{Field, Label, Format, Options, Align}` | `Format` → `CellFormat(format)`; `Options.Tones` map values → `Tone(tone)`. |
| `Table.Keys` | — | Kept beside the rows for `RowAction`s (P3-09/P3-10): row i's key. |
| `Table.Page`, `Table.Sort` | the table's own state | Sent back in `TableRequest` on the next page or sort. |
| `Chart{Type, Categories, X, Series}` | `ChartData{Type, Categories, X, Series}` | `ChartSeries{Name, X, Y}` is the same shape. |
| `Axis{Label, Format string, Time}` | `ChartAxis{Label, Format func(float64) string, Time}` | The format name becomes a func: `func(v float64) string { return kit.FormatValue(c, v, kit.CellFormat(a.Format), opts) }`. A time axis is formatted by the kit from Unix seconds. |
| `Stat{Value, Format, Options, Change, ChangeFormat, Caption}` | `StatSpec{…}` | Same fields. |
| `Filter{Param, Control, Label, Required, Options, Value, Error}` | `FilterSpec{Label, Control, Required, Options}` | Options are typed (`[]any`); the kit shows `[]string`, each option's text. The picked text goes back as the value, and the runtime binds the typed option. `Error` shows under the control. |
| `Answer{Filters}` / `Answer{Block}` | — | First the page's filters, then each block as it answers. |

`Block.Place` ("row.col", or "row.col.row.col" inside a card) is how the page screen finds the block an answer is for. `viewrun.Places(page)` lists the places in order. `PageRequest.Only` re-runs just some places, such as the blocks a changed filter reaches. `PageRequest.Blocks` holds each block's own filter values and table state by place, so two blocks of one view keep their own page and sort.

## Decisions and deviations

1. **A new package, `internal/viewrun`.** `internal/view` is the config's schema and validation; running views needs the store and the clock. Keeping them apart keeps `view` free of the worker pool.
2. **LIMIT and OFFSET are literals,** not `?`. The SPEC shows `LIMIT ? OFFSET ?`, but the runtime binds only named parameters, and these two are its own integers, not user input.
3. **Max = limit + 1.** The worker re-counts a query whose rows fill `Max`, so asking for one more row than the runtime keeps avoids a second run of the query.
4. **The count runs after the rows,** with its own 2 s, and only when the page doesn't tell the total. A table block can take up to 4 s of query time when its count times out (6 s past the last page: rows, count, last page); its rows still show, with "many rows".
5. **The `rows_from` key is a tiebreaker** after the sort column, so pages don't repeat or skip rows when the sort column has ties.
6. **The bar cap counts bars, not categories:** with two series, 500 categories make 1,000 bars. For a pie there is no cap; the kit folds the slices.
7. **Series order is the query's first appearance,** for every chart type, not the sorted x order. The legend stays stable when the data moves.
8. **Duplicate categories on a bar or pie:** the later row wins. The runtime doesn't sum them; that is the query's job.
9. **Options are capped at 1,000** (`MaxOptions`). A select of more can't be used anyway.
10. **Missing or invalid bucket keys become nil,** so the cell is blank rather than a broken thumbnail.
11. **No cache of parsed views.** Each run loads and parses its configs, so a saved change shows at once. I didn't time the parse on its own; next to a query it should be small, but that is not measured. A cache can come later if a profile asks for one.
12. **No re-validation at run time.** A view that a migration broke (a column gone) is an error on its block, with the columns the query has, not a refusal of the whole page.
13. **`Start`/`Stop` are keyed runs.** A new `Start` under the same key waits for the old run's queries to stop before it begins, so answers never interleave across runs. `Stop` doesn't wait, so `out` can call it. The one gap: an `out` call already under way when `Stop` or `Start` is called from another goroutine may still finish. Stopping kills the busy worker (SPEC 2.2); the next query starts a new one.
14. **`internal/app` gets a `TestMain`** with `sqlworker.IfWorker()`, so the app tests can run view queries in workers.

## Timings

Windows 11, from the tests' logs; warm numbers are the 2nd and 3rd runs of `-count=3`. Blocks run in the 2 query workers. "Cold" includes starting a worker; "warm" doesn't.

| What | Cold | Warm |
|---|---|---|
| 10,000-row table, first page by `amount desc`, with its count | ~155 ms | ~6 ms (5.5–6.1) |
| 10,001-row line chart (10,000 points sorted, notice) | ~165 ms | ~12 ms (11.4–12.2) |
| Page of 6 blocks: a 10,000-row table, a 5,001-point line, a stat, a small table, a form, a missing view | ~49–57 ms (one of the two workers still starting) | ~7.6 ms for the whole page |
| Table whose count times out (rows plus "many rows") | ~2.13 s | — |
| Slow block among 3 fast ones | — | slow one stopped at 2.003 s; the others at 0.5, 1.1 and 11 ms |
| Cancelled run (slow block, cancelled after 300 ms) | — | `Page` returned 3 ms after the cancel |
| `Start` again with new filters while a slow block runs | — | the new page answered in 5–7 ms |
| Three-block page through `ViewService`, configs loaded and parsed | ~153 ms (workers starting) | — |
| Table behind four slow blocks (both workers busy for 4 s) | — | the table answered at 4.7 s, with its rows and count. Before the fix, its options query timed out while waiting, at 2.6 s |
| Two 300 ms options queries and the view | — | ~302 ms (run in parallel) |

A view's 2 s start when a worker takes its query, so slow blocks no longer make fast ones time out while they wait (review point 1).

## Checks run

With `GOPROXY=off GOMEMLIMIT=1500MiB`:

- `go build ./...`: ok
- `go vet ./...`: ok
- `go test -p 2 -short -count=1 ./...`: ok, every package
- `gofmt -l` on every changed and new `.go` file, with CRs stripped first: clean
- `go run ./cmd/jenab-scenarios check scenarios`: ok

The new tests:

- `internal/viewrun`: `TestTable`, `TestTableShort`, `TestTableManyRows`, `TestTableMissingColumn`, `TestChartTenThousandRows`, `TestChartTimes`, `TestChartBars`, `TestChartPieAndText`, `TestChartGaps`, `TestStat`, `TestFilters`, `TestParams`, `TestPage`, `TestSlowBlock`, `TestCancel`, `TestNoNetworkImports`.
- `internal/app`: `TestViewServicePage` (R-48) and `TestViewServiceStart`.
- From the review: `TestPoolTimeoutOnSlot` (`internal/sqlworker`); `TestWaitForWorker`, `TestTableFullPage`, `TestFiltersCleared`, `TestNotANumber`, `TestBucketCheckFails`, `TestPanics`, `TestOptionsParallel`, `TestChartNullX`, `TestSameViewTwice` (`internal/viewrun`); `TestViewServiceStartStop`, `TestViewServiceReentrant` (`internal/app`). `TestTable` and `TestChartTimes` changed for the page clamp and year 0.

**`-race` was not run.** This machine builds with `CGO_ENABLED=0` and has no C compiler, and CI can't run Actions at the moment. `TestViewServiceStartStop` and `TestViewServiceReentrant` are written to be run with `-race` (`go test -race -run TestViewService ./internal/app/`) in WSL or CI; here they passed 8 times in a row without it.

The fixture is one project with a 10,000-row table, a 10,001-row table of points, coins, prices and a bucket file. The clock is fixed at 2026-11-20 22:00 UTC in a +03:30 zone, where it is already the 21st, so relative dates are tested across the date line.

## Review fixes

Main-Session's review had ten points. Each is fixed and has a test.

1. **Waiting for a worker burned the 2 s.** `sqlworker.Request` has a new `TimeoutOnSlot`: the `Timeout` counts from when the query takes a worker's slot, and the wait is bounded only by the context. `ProjectDB.ViewQuery` sets it; agent SQL (`guardedQuery`) doesn't, so the agent's rule is unchanged. A page's wait is bounded by the page run's context, which `Stop` or a new `Start` cancels. Tests: `TestPoolTimeoutOnSlot` (with and without), and `TestWaitForWorker`: four slow blocks hold both workers for 4 s, and the table behind them gets its options, rows and count. It fails without the fix.
2. **A Full short page wasn't the last.** A page the 8 MiB cap cut always counts, with a notice. `page_size` is clamped in the runtime to 500 (`MaxPageSize`), on top of the schema's own maximum of 500. I chose the runtime clamp because `ParseView` doesn't re-check a saved config, so a hand-edited row could reach the runtime. Test: `TestTableFullPage`.
3. **The count query ran the inner query twice.** It now asks for max 2, so one row doesn't make the worker re-count. Test: `TestTableFullPage` records the count's max.
4. **"" wasn't cleared.** `""` and blanks are nil before a value is read. An optional filter binds NULL. A required one binds its default (or a select's first option) and says "This filter needs a value." A required checkbox is unchecked, with no error. Test: `TestFiltersCleared` covers optional text, date, select and number, required date and both selects, the required checkbox, and blanks.
5. **ObjectsExist had no deadline.** The check has a view's 2 s. On an error or a timeout, every image and file cell is blank and the block's notice says why; the rows still show. Test: `TestBucketCheckFails` checks the deadline and both kinds of fault.
6. **Stop and Start could interleave, and an out that called Stop deadlocked.** There is a generation counter per key: `Start` and `Stop` bump it, and an answer is passed on only while its run's generation is current. The check is under the lock, and `out` is called without it. `Stop` no longer waits; a `Start` after it still waits for the stopped run's queries to end. Tests: `TestViewServiceStartStop` (20 Starts against concurrent Stops: every run ends once, and answers never go back to an older run) and `TestViewServiceReentrant` (out calls Stop, out calls Start). `-race` was not run here; see Checks run.
7. **Panics in block goroutines.** `placed` and `View` recover a panic into "Something went wrong showing this view: …" on that block. An options query that panics is an error on its filter. Test: `TestPanics` panics once in the views lookup and once in a block's query; the third block is fine.
8. **NaN and Inf.** A filter refuses them, as numbers or as text: "NaN is not a number". A chart's y treats them as a gap (NaN). A stat's value and change treat them as nil. Text such as `NaN` no longer parses as a number anywhere. Test: `TestNotANumber`.
9. **Options queries ran one by one.** A view's options queries now run in parallel with each other. A block's own filters' queries run inside its block goroutine, so they also overlap the other blocks. I picked "with each other" over "with the blocks": the page's filters must be resolved before the blocks bind them. Test: `TestOptionsParallel`: two 300 ms queries take about 302 ms.
10. **Small points.**
    - A NULL x on a bar or pie is skipped, not labelled (the choice); also on a line along text. Test: `TestChartNullX`.
    - `PageRequest.Views` is now `PageRequest.Blocks`, keyed by block place. Test: `TestSameViewTwice`.
    - A page past the last is clamped to the last. Test: `TestTable` asks for pages 199, 200 and 5000.
    - A year outside 1–9999 is missing, as NULL is, not an error. Test: `TestChartTimes`.

## SPEC lines Main-Session should change

- **2.2, add a views exception:** "A view query's 2 s start when a worker takes it; its wait for a worker is bounded by the page run, which closing the page or changing its filters cancels. Agent queries count the wait."
- **5.7, first bullet:** "Queries run on a read-only connection from the project's reader pool (`PRAGMA query_only = ON`)" → "Queries run on the guarded read-only path through the project's query workers (2.2), as agent queries do, with the view deadline."
- **5.7, second bullet:** "The blocks of a page query in parallel on the reader pool" → "…in parallel on the project's 2 query workers. Each query's 2 s start when a worker takes it (2.2), so blocks that wait behind slow ones don't time out. A panic is an error on its block only."
- **5.3, the wrap:** `LIMIT ? OFFSET ?` → `LIMIT <n> OFFSET <m>`, and add "The rows are ordered by the sort column, then by `rows_from.key` when there is one. The count runs only when the page tells no total, with its own 2 s: a full page, or one cut by the 8 MiB result cap, which also shows a notice."
- **5.3, pages:** add "`page_size` is at most 500. A page past the last shows the last."
- **5.3, sorting:** add "Any shown column can sort the table, except `image` and `file` columns."
- **5.3, formats:** add "An `image` or `file` key that isn't in the bucket shows blank. When the bucket can't be checked within 2 s, all of them show blank, with a notice on the block."
- **5.2:** add "A value of `""` or blanks is cleared. A cleared required filter binds its default, or a select's first option, and says it needs a value. `select` options from a query are capped at 1,000, and a view's options queries run in parallel. A filter value that can't be read, NaN and infinities included, binds the default and shows why under the filter. A block's own filter on a parameter the page's filters set is hidden on the page."
- **5.4, bars:** "the first 1,000 are shown" → "the first 1,000 bars, counting each series' bars, are shown".
- **5.4:** add "A row whose x is NULL, or a time outside the years 1 to 9999, is left out; it has no point, bar or slice. A y that is NULL, not a number, NaN or infinite is a gap. With `series_by`, series come in the order the query first gives them. On a bar or pie chart, a repeated category keeps its last row."
- **5.4, pie:** "At most 12 slices are drawn…" → add "(the kit folds them; the runtime passes every row)".
- **5.10:** add "A value or change that is NaN or infinite shows nothing."

## Proposed Result paragraph

Done on 2026-11-23. Report: [P3-05-report](reports/P3-05-report.md).

- **Runtime:** `internal/viewrun` runs a parsed view or page with filter values on the query workers' read-only path, 2 s a query from when a worker takes it, `:name` parameters only. A page's blocks run in parallel, and each answers on its own; a panic is an error on its block. `ViewService` (`Bound.Views`) loads saved configs and runs them, with `Start`/`Stop` keyed runs for the page screen, a generation per key, and `out` called without a lock.
- **Filters:** defaults and relative dates in the project's zone; `""` is cleared; cleared optional filters bind NULL, and required ones fall back to their default or first option and say they need a value; NaN and infinities are refused; options come from one-column queries run in parallel, capped at 1,000; a page's filters override and hide a block's own.
- **Table:** wrapped `ORDER BY … LIMIT … OFFSET`, with the `rows_from` key as a tiebreaker; `page_size` at most 500, and a page past the last is the last. The count runs only when needed (always for a page the 8 MiB cap cut), and "many rows" shows when it times out. Values are typed per format, and image and file keys are checked against the bucket within 2 s, blank with a notice when it can't be read.
- **Chart:** 10,001-row limit with the notice, line and scatter sorted by x with no downsampling, `series_by`, and the 1,000-bar cap. Pie rows pass through (the kit folds them), and negative values are an error. A NULL x, or a year outside 1–9999, is left out; NaN and infinities are gaps.
- **Stat:** exactly one row, with a signed change; NaN and infinities show nothing.
- **R-48:** a test fails a page open that makes any HTTP request or provider call, and an import test keeps sockets, providers and the agent out of the runtime.
- **Speed:** with the workers running, a 10,000-row table page takes about 6 ms, a 10,001-row chart about 12 ms, and a page of six blocks about 8 ms; cold, each first query adds about 150 ms for starting a worker. A slow block stops at 2 s while the others answer in milliseconds.
