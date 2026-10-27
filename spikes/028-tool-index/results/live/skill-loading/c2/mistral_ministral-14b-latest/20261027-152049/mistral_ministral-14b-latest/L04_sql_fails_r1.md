# L04_sql_fails

This query fails in the query tool. Why?

Good behaviour: Load sql-queries before the answer; no skills outside sql-queries.

## Turn 1

**User:** This query fails in the query tool. Why?
SELECT status, count(*) FROM _jenab_runs GROUP BY status


> call query {"sql":"SELECT status, count(*) FROM _jenab_runs GROUP BY status"}

> result query (error):
> SQL logic error: no such table: _jenab_runs (1)

**Agent:** The error occurs because there is no table named `_jenab_runs` in your project's database. This table is typically an internal system table used by Jenab to track pipeline runs, but it does not appear to exist in your project.

If you're trying to track pipeline runs or fetch metadata about them, you may need to:
1. **Check if there is an alternative table** (e.g., a custom table tracking runs).
2. **Use the `daily_btc` pipeline's metadata** (if it has run before) via the `_run_id` column in your existing tables (`btc_news` or `btc_prices`).
3. **Ensure the pipeline has run at least once** (the `_jenab_runs` table is auto-populated when pipelines execute).

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 29 | 2176 | 0 | 23 | tool_use | 0.9 s |  |
| 2 | 71 | 2176 | 0 | 167 | end | 1.9 s |  |
