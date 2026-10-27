# L04_sql_fails

This query fails in the query tool. Why?

Good behaviour: Load sql-queries before the answer; no skills outside sql-queries.

## Turn 1

**User:** This query fails in the query tool. Why?
SELECT status, count(*) FROM _jenab_runs GROUP BY status


> call query {"sql":"SELECT status, count(*) FROM _jenab_runs GROUP BY status"}

> result query (error):
> SQL logic error: no such table: _jenab_runs (1)

**Agent:** The error occurs because there is no table named `_jenab_runs` in your project's database. This table is not part of the default schema for Jenab projects, and it is not created automatically.

If you are trying to track pipeline runs or execution statuses, you would need to:
1. **Create a table** to store run statuses (e.g., `_runs` or `pipeline_runs`), or
2. **Use an existing table** that might contain run-related data (e.g., `_fetched_at` in your tables indicates when data was fetched, but not status).

Would you like help designing a table to track pipeline runs? If so, let me know the details (e.g., columns like `run_id`, `status`, `timestamp`).

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 29 | 2176 | 0 | 23 | tool_use | 0.9 s |  |
| 2 | 71 | 2176 | 0 | 162 | end | 2.0 s |  |
