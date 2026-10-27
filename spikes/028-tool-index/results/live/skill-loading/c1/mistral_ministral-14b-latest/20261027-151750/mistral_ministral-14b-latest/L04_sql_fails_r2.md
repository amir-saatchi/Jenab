# L04_sql_fails

This query fails in the query tool. Why?

Good behaviour: Load sql-queries before the answer; no skills outside sql-queries.

## Turn 1

**User:** This query fails in the query tool. Why?
SELECT status, count(*) FROM _jenab_runs GROUP BY status


> call query {"sql":"SELECT status, count(*) FROM _jenab_runs GROUP BY status"}

> result query (error):
> SQL logic error: no such table: _jenab_runs (1)

**Agent:** The error is because there is no table named `_jenab_runs` in your project database. The `_jenab_runs` table is an internal system table that tracks pipeline runs, but it is not exposed or accessible directly in the project database schema.

If you want to track pipeline runs or statuses, you would need to:
1. **Check pipeline logs**: Use the pipeline execution logs or triggers to track runs.
2. **Add a custom tracking table**: Design a table to manually log run statuses if needed.
3. **Use the `_run_id` column**: If you want to track runs related to your tables (e.g., `btc_news` or `btc_prices`), query those tables directly using their `_run_id` column.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 128 | 3456 | 0 | 23 | tool_use | 1.4 s |  |
| 2 | 170 | 3456 | 0 | 158 | end | 1.6 s |  |
