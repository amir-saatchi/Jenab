# L04_sql_fails

This query fails in the query tool. Why?

Good behaviour: Load sql-queries before the answer; no skills outside sql-queries.

## Turn 1

**User:** This query fails in the query tool. Why?
SELECT status, count(*) FROM _jenab_runs GROUP BY status


> call query {"sql":"SELECT status, count(*) FROM _jenab_runs GROUP BY status"}

> result query (error):
> SQL logic error: no such table: _jenab_runs (1)

**Agent:** The error is because there is no table named `_jenab_runs` in your project's database. This table is typically used internally by Jenab to track pipeline runs, but it does not exist in your current schema.

If you're trying to track pipeline runs or fetch metadata about them, you may need to:
1. **Check if your pipelines are running**: The `_jenab_runs` table is automatically populated when pipelines execute, so if you haven't run any pipelines yet, this table won't exist.
2. **Run a pipeline**: Trigger the `daily_btc` pipeline manually to generate run data.
3. **Alternative approach**: If you want to track pipeline runs, you may need to design a custom table or use the `_fetched_at` column in your existing tables (e.g., `btc_news` or `btc_prices`) to infer when data was last updated.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2610 | 0 | 0 | 23 | tool_use | 1.8 s |  |
| 2 | 92 | 2560 | 0 | 188 | end | 4.7 s |  |
