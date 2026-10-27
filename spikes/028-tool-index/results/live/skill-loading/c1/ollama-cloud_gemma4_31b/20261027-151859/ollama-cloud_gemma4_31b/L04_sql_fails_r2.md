# L04_sql_fails

This query fails in the query tool. Why?

Good behaviour: Load sql-queries before the answer; no skills outside sql-queries.

## Turn 1

**User:** This query fails in the query tool. Why?
SELECT status, count(*) FROM _jenab_runs GROUP BY status


> call load_skill {"name":"sql-queries"}

*Skill loaded: sql-queries*

> result load_skill:
> 

**Agent:** The query fails because the SQL guard rejects access to any tables starting with `_jenab_` or `sqlite_`. The table `_jenab_runs` is a system table and cannot be queried directly through the query tool.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 3079 | 0 | 0 | 220 | tool_use | 1.5 s |  |
| 2 | 3397 | 0 | 0 | 171 | end | 6.8 s |  |
