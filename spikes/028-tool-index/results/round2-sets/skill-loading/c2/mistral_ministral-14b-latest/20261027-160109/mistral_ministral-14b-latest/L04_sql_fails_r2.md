# L04_sql_fails

This query fails in the query tool. Why?

Good behaviour: Load sql-queries before the answer; no skills outside sql-queries.

## Turn 1

**User:** This query fails in the query tool. Why?
SELECT status, count(*) FROM _jenab_runs GROUP BY status


> call query {"sql":"SELECT status, count(*) FROM _jenab_runs GROUP BY status"}

> result query (error):
> SQL logic error: no such table: _jenab_runs (1)

**Agent:** The error is because there is no table named `_jenab_runs` in your project's database. The query tool only interacts with the tables you have defined: `btc_news`, `btc_prices`, and `watch_keywords`.

If you're looking for run statuses or pipeline execution logs, those are not stored in a table but are typically available in the app's interface or logs. If you need to track pipeline runs, you may need to design a custom table for this purpose or check the app's documentation for built-in ways to access run history.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 50 | 2560 | 0 | 23 | tool_use | 1.8 s |  |
| 2 | 92 | 2560 | 0 | 118 | end | 6.2 s |  |
