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

**Agent:** The query fails because the query tool's SQL guard rejects access to system tables starting with `_jenab_` or `sqlite_`.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2443 | 0 | 0 | 224 | tool_use | 1.6 s |  |
| 2 | 2765 | 0 | 0 | 108 | end | 0.9 s |  |
