---
name: sql-queries
description: "SQL for the query tool: the SQLite dialect, dates, window functions, parameters, and what the SQL guard refuses. Load to write, fix or explain a query."
---
## SQL for `query`

The project's tables are in SQLite. `query` runs one read-only query; rows are added with `insert`, never with SQL.

### What a query may be

- One statement that starts with `SELECT`, `WITH` or `VALUES`. No second statement after a `;`, and no `INSERT`, `UPDATE`, `DELETE`, `CREATE`, `DROP`, `ATTACH`, `PRAGMA` or `VACUUM`.
- Only the project's own tables. Names that start with `_jenab_` (Jenab's internal tables), `sqlite_` (such as `sqlite_schema` or `sqlite_master`) or `pragma_` (such as `pragma_table_info`) are refused. To learn the tables and their columns, use the table names you already have and `describe_table(name)`; don't query the schema.
- Virtual tables: `json_each` and `json_tree` only.
- Functions: SQLite's core, aggregate, window, date and time, math and JSON functions. Others, such as `load_extension`, are refused.
- Outside strings and quoted names, only plain SQL characters: no `#`.
- At most 64 KiB of text, 500 `SELECT`, `WITH` and `VALUES`, and 100 CTEs.
- Each query gets 10 seconds and 256 MiB. A stored text or blob over 1 MiB can't be read, but `length()` works on it.

A refused query comes back with the reason, often with its line and column. Change that part and run it again.

### Parameters

Pass values as parameters, never by pasting them into the text: `WHERE coin = :coin` with `params: {"coin": "BTC"}`. A number names a numbered parameter: `?1`, or the first `?`, takes `{"1": "BTC"}`.

### Results

`query` returns up to 200 rows or 4,000 tokens, with the total row count. A larger result comes as a preview and a ref; read on with `read_ref` or `search_ref`. Ask for what you need instead: aggregate (`count`, `sum`, `avg`, `min`, `max` with `GROUP BY`), filter with `WHERE`, or take the top rows with `ORDER BY … LIMIT`.

### The SQLite dialect

- Strings go in single quotes, names that need quoting in double quotes. `||` joins text.
- `LIKE` ignores case only for ASCII letters; `GLOB` is case-sensitive with `*` and `?`.
- A column holds what was stored: compare a date column with ISO text such as `'2026-10-18'` or `'2026-10-18 14:30:00'`, which also sorts right.
- Dates: `date('now')`, `date('now', '-7 days')`, `date(d, 'start of month')`, `strftime('%Y-%m', d)` for the month, `julianday(a) - julianday(b)` for days between. `'now'` is UTC; add `'localtime'` for the user's clock.
- Whole numbers divide to a whole number: `5 / 2` is 2. Write `5 / 2.0` or `CAST(x AS REAL)`.
- `coalesce(x, 0)`, `nullif(x, 0)` against dividing by zero, `round(x, 2)`.
- `iif(cond, a, b)` or `CASE WHEN … THEN … ELSE … END`.
- Booleans are 1 and 0.
- `LIMIT n OFFSET m`. There is no `TOP`, `ILIKE`, `::type`, `DATE_TRUNC` or `EXTRACT`; use the forms above.
- JSON in a text column: `json_extract(col, '$.a')` or `col ->> '$.a'`; `json_each(col)` turns an array into rows.

### Window functions

`fn(…) OVER (PARTITION BY … ORDER BY … ROWS BETWEEN n PRECEDING AND CURRENT ROW)`. They run after `WHERE` and `GROUP BY`, so filter on their result in an outer query.

- Moving average over 7 rows: `avg(price) OVER (ORDER BY date ROWS BETWEEN 6 PRECEDING AND CURRENT ROW)`
- Change from the row before: `price - lag(price) OVER (ORDER BY date)`
- Running total: `sum(amount) OVER (ORDER BY date)`
- Place in a group: `row_number() OVER (PARTITION BY coin ORDER BY date DESC)`

### Examples

```sql
-- the latest row for each coin
SELECT coin, date, price FROM (
  SELECT coin, date, price, row_number() OVER (PARTITION BY coin ORDER BY date DESC) AS rn
  FROM prices
) WHERE rn = 1;

-- the average of each month for one coin
SELECT strftime('%Y-%m', date) AS month, round(avg(price), 2) AS avg_price
FROM prices WHERE coin = :coin
GROUP BY month ORDER BY month;

-- the last 30 days
SELECT date, price FROM prices
WHERE coin = :coin AND date >= date('now', '-30 days')
ORDER BY date;
```
