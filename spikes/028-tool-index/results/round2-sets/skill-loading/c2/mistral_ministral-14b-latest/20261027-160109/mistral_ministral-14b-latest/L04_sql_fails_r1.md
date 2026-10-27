# L04_sql_fails

This query fails in the query tool. Why?

Good behaviour: Load sql-queries before the answer; no skills outside sql-queries.

## Turn 1

**User:** This query fails in the query tool. Why?
SELECT status, count(*) FROM _jenab_runs GROUP BY status


> call query {"sql":"SELECT status, count(*) FROM _jenab_runs GROUP BY status"}

> result query (error):
> SQL logic error: no such table: _jenab_runs (1)

**Agent:** The error is because there is no table named `_jenab_runs` in your project's database. The `_jenab_runs` table is an internal system table used by Jenab to track pipeline runs, but it is not exposed or accessible directly in the project database.

If you're trying to track or analyze pipeline runs, you would need to:
1. **Check the `btc_news` or `btc_prices` tables** for `_run_id` columns, which link rows to specific pipeline runs.
2. **Use the `_fetched_at` column** in those tables to analyze when data was fetched.
3. **Modify a pipeline** to log custom run statuses in a dedicated table if needed.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 50 | 2560 | 0 | 23 | tool_use | 1.3 s |  |
| 2 | 2652 | 0 | 0 | 150 | end | 6.6 s |  |
