# P3-05 — Running views: queries, filters, tables, charts and stats
**Type:** Feature
**Status:** Done
**Gate:** 3 (Phase 3)
**Needs:** P3-01, P3-02
**Requirements:** R-40, R-41, R-42, R-44, R-48

## Goal
The Go side that turns a view and its filter values into what a block draws, fast and without any LLM call or network request (SPEC 5.2–5.4, 5.7, 5.10).

## Scope
- **Queries (5.7):** on the read-only path through the query worker, 2 s per block, named parameters only, waiting for no writer. The blocks of a page query in parallel and each answers on its own.
- **Filters (5.2):** defaults, relative dates computed in the project's local time, a cleared optional filter binds `NULL`, `select` options from a query (one column).
- **Table (5.3):** the wrap `SELECT * FROM (<query>) ORDER BY <sort> LIMIT ? OFFSET ?` and the count query; a count that hits the timeout gives "many rows"; `page_size` default 50; sorting by any shown column. Values come back typed for the formats. Run the parsed `View` from P3-02's `ParseView`, and match result columns as written, as validation does.
- **Chart (5.4):** `LIMIT 10001` with the 10,000-row notice; line and scatter sorted by `x`; `series_by`; pie with at most 12 slices plus "Other" and an error for negative values; bar charts capped at 1,000 bars with a notice. SPEC 5.4's size rules are now P3-08's, measured on MyGo: no downsampling, so pass every row up to 10,000. The notices go to the block as `BlockState.Notice`.
- **Stat (5.10):** exactly one row, else an error on the block; `change` with its sign.
- **Formats:** the values each format needs (`currency` with `format_options`, `percent`, `date`, `datetime`, `url`, `boolean`, `badge`, `image` and `file` keys checked against the bucket).
- **Services:** a `ViewService` (or fitting name) that runs a view or a page's blocks for given filter values, cancellable when the page closes or the filters change.
- **No LLM and no network:** a test fails if opening a page calls a provider or opens a connection (R-48).

## Done when
- Tests for each rule above on fixture data, including a 10,000-row table, a 10,001-row chart, a slow query that times out on its own block while the others answer, and a cancelled run.

## Result
Done on 2026-11-23. Report: [P3-05-report](reports/P3-05-report.md).

- **Runtime:** `internal/viewrun` runs a parsed view or page with filter values on the query workers' read-only path, 2 s a query from when a worker takes it, `:name` parameters only. A page's blocks run in parallel, and each answers on its own; a panic is an error on its block. `ViewService` (`Bound.Views`) loads saved configs and runs them, with `Start`/`Stop` keyed runs for the page screen, a generation per key, and `out` called without a lock.
- **Filters:** defaults and relative dates in the project's zone; `""` is cleared; cleared optional filters bind NULL, and required ones fall back to their default or first option and say they need a value; NaN and infinities are refused; options come from one-column queries run in parallel, capped at 1,000; a page's filters override and hide a block's own.
- **Table:** wrapped `ORDER BY … LIMIT … OFFSET`, with the `rows_from` key as a tiebreaker; `page_size` at most 500, and a page past the last is the last. The count runs only when needed (always for a page the 8 MiB cap cut), and "many rows" shows when it times out. Values are typed per format, and image and file keys are checked against the bucket within 2 s, blank with a notice when it can't be read.
- **Chart:** 10,001-row limit with the notice, line and scatter sorted by x with no downsampling, `series_by`, and the 1,000-bar cap. Pie rows pass through (the kit folds them), and negative values are an error. A NULL x, or a year outside 1–9999, is left out; NaN and infinities are gaps.
- **Stat:** exactly one row, with a signed change; NaN and infinities show nothing.
- **R-48:** a test fails a page open that makes any HTTP request or provider call, and an import test keeps sockets, providers and the agent out of the runtime.
- **Speed:** with the workers running, a 10,000-row table page takes about 6 ms, a 10,001-row chart about 12 ms, and a page of six blocks about 8 ms; cold, each first query adds about 150 ms for starting a worker. A slow block stops at 2 s while the others answer in milliseconds.
- **Review:** two Medium and eight Low points are fixed. A view's 2 s now start when a worker takes its query, a page the 8 MiB cap cut always counts, and Stop and Start on one key no longer mix answers. The -race run of the Start/Stop tests waits for WSL or CI.
