# P3-05 — Running views: queries, filters, tables, charts and stats
**Type:** Feature
**Status:** Open
**Gate:** 3 (Phase 3)
**Needs:** P3-01, P3-02
**Requirements:** R-40, R-41, R-42, R-44, R-48

## Goal
The Go side that turns a view and its filter values into what a block draws, fast and without any LLM call or network request (SPEC 5.2–5.4, 5.7, 5.10).

## Scope
- **Queries (5.7):** on the read-only path through the query worker, 2 s per block, named parameters only, waiting for no writer. The blocks of a page query in parallel and each answers on its own.
- **Filters (5.2):** defaults, relative dates computed in the project's local time, a cleared optional filter binds `NULL`, `select` options from a query (one column).
- **Table (5.3):** the wrap `SELECT * FROM (<query>) ORDER BY <sort> LIMIT ? OFFSET ?` and the count query; a count that hits the timeout gives "many rows"; `page_size` default 50; sorting by any shown column. Values come back typed for the formats.
- **Chart (5.4):** `LIMIT 10001` with the 10,000-row notice; line and scatter sorted by `x`; `series_by`; pie with at most 12 slices plus "Other" and an error for negative values; bar charts capped at 1,000 bars with a notice. Downsampling: SPEC 5.4's size rules were measured on Recharts in a WebView. Measure on MyGo's chart package (it has `MinMax` downsampling) and propose the rules for the native app; Main-Session updates 5.4.
- **Stat (5.10):** exactly one row, else an error on the block; `change` with its sign.
- **Formats:** the values each format needs (`currency` with `format_options`, `percent`, `date`, `datetime`, `url`, `boolean`, `badge`, `image` and `file` keys checked against the bucket).
- **Services:** a `ViewService` (or fitting name) that runs a view or a page's blocks for given filter values, cancellable when the page closes or the filters change.
- **No LLM and no network:** a test fails if opening a page calls a provider or opens a connection (R-48).

## Done when
- Tests for each rule above on fixture data, including a 10,000-row table, a 10,001-row chart, a slow query that times out on its own block while the others answer, and a cancelled run.
- The report gives the measured chart numbers and the proposed size rules.
