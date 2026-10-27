# L04_sql_fails

This query fails in the query tool. Why?

Good behaviour: Load sql-queries before the answer; no skills outside sql-queries.

## Turn 1

**User:** This query fails in the query tool. Why?
SELECT status, count(*) FROM _jenab_runs GROUP BY status


> call query {"sql":"SELECT status, count(*) FROM _jenab_runs GROUP BY status"}

> result query (error):
> SQL logic error: no such table: _jenab_runs (1)

**Agent:** The error occurs because there is no table named `_jenab_runs` in your project's database. The `_jenab_runs` table is an internal system table used by Jenab to track pipeline runs, but it is not exposed or created automatically in the user's database schema.

If you want to track or analyze pipeline runs, you would need to:
1. **Check existing tables**: Verify if there is a table like `_run_id` or similar in your existing tables (e.g., `btc_news`, `btc_prices`) that might contain run status or metadata.
2. **Use pipeline logs**: If you need to track pipeline runs, you can check the pipeline execution logs or use the Jenab UI to see run history.
3. **Design a custom table**: If you need to track run statuses manually, you could design a new table for this purpose.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2077 | 128 | 0 | 23 | tool_use | 1.9 s |  |
| 2 | 71 | 2176 | 0 | 183 | end | 2.3 s |  |
