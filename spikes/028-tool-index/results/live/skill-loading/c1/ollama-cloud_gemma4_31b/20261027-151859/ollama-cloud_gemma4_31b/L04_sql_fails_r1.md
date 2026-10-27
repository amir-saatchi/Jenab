# L04_sql_fails

This query fails in the query tool. Why?

Good behaviour: Load sql-queries before the answer; no skills outside sql-queries.

## Turn 1

**User:** This query fails in the query tool. Why?
SELECT status, count(*) FROM _jenab_runs GROUP BY status


> call describe_table {"name":"_jenab_runs"}

> result describe_table (error):
> table "_jenab_runs" not found (tables: btc_news, btc_prices, watch_keywords)

**Agent:** The query fails because the table `_jenab_runs` does not exist in your database. The available tables are `btc_news`, `btc_prices`, and `watch_keywords`.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 3079 | 0 | 0 | 242 | tool_use | 1.9 s |  |
| 2 | 3363 | 0 | 0 | 100 | end | 0.9 s |  |
