# L04_sql_fails

This query fails in the query tool. Why?

Good behaviour: Load sql-queries before the answer; no skills outside sql-queries.

## Turn 1

**User:** This query fails in the query tool. Why?
SELECT status, count(*) FROM _jenab_runs GROUP BY status


> call query {"sql":"SELECT status, count(*) FROM _jenab_runs GROUP BY status"}

> result query (error):
> SQL logic error: no such table: _jenab_runs (1)

**Agent:** The error is because there is no table named `_jenab_runs` in your project database. The `_jenab_runs` table is an internal system table in Jenab and is not exposed to queries. If you want to track or analyze pipeline runs, you would need to use the `_run_id` column in your existing tables (like `btc_news` or `btc_prices`) or set up a pipeline to log run statuses separately.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 3456 | 128 | 0 | 23 | tool_use | 1.1 s |  |
| 2 | 3626 | 0 | 0 | 96 | end | 12.9 s |  |
