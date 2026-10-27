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

If you want to track or analyze pipeline runs, you would need to use the **pipeline history** feature or check the `_run_id` columns in your existing tables (like `btc_news` and `btc_prices`), which are populated by Jenab to link rows to their source runs.

Would you like help querying pipeline runs indirectly (e.g., via `_run_id` in your tables) or checking pipeline execution logs?

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 128 | 3456 | 0 | 23 | tool_use | 0.9 s |  |
| 2 | 3498 | 128 | 0 | 147 | end | 1.7 s |  |
