# L04_sql_fails

This query fails in the query tool. Why?

Good behaviour: Load sql-queries before the answer; no skills outside sql-queries.

## Turn 1

**User:** This query fails in the query tool. Why?
SELECT status, count(*) FROM _jenab_runs GROUP BY status


> call query {"sql":"SELECT status, count(*) FROM _jenab_runs GROUP BY status"}

> result query (error):
> SQL logic error: no such table: _jenab_runs (1)

**Agent:** The query fails because the table `_jenab_runs` does not exist in your database.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2135 | 0 | 0 | 298 | tool_use | 4.2 s |  |
| 2 | 2463 | 0 | 0 | 95 | end | 1.0 s |  |
