---
name: pipelines
description: "Pipelines: YAML format, triggers and schedules, the step catalog, for_each, expressions and functions. Load before save_pipeline or any pipeline change."
load_with: [save_pipeline]
---
## 2. Pipelines: save_pipeline

```yaml
version: 1
id: daily_weather            # lower_snake_case, unique in the project
description: Daily weather for all cities
trigger:
  schedule: "0 7 * * *"      # 5-field cron, in timezone
  timezone: Europe/Oslo      # IANA name
  manual: true               # also runnable by hand; a trigger needs schedule and/or manual: true
inputs:                      # optional: { name: { type: string|number|boolean|date, default } }
  days: { type: number, default: 1 }
steps:
  - id: cities
    use: db.query
    with:
      sql: SELECT name FROM cities
  - id: weather
    use: http.get
    for_each: ${{ steps.cities.rows }}
    with:
      url: https://api.example.org/weather
      query: { city: "${{ item.name }}" }
  - id: rows
    use: transform.map
    with:
      items: ${{ steps.weather.each }}
      fields:
        city: ${{ item.item.name }}
        temp: ${{ item.json.temp }}
        date: ${{ today() }}
  - id: save
    use: db.insert_many
    with:
      table: weather
      on_conflict: upsert
      rows: ${{ steps.rows.items }}
```

Top level: `version: 1`, `id`, `description`, `trigger`, `inputs`, `steps`, `on_error` (stop/continue), `timeout` (e.g. 5m). Nothing else.
Step fields: `id`, `use`, `with`, `if` (`${{ }}` boolean; skipped when false), `for_each`, `retry` {max, backoff: a duration like 10s}, `timeout`, `continue_on_error`. Nothing else; step inputs always go under `with`.

`for_each: ${{ <list> }}` runs the step once per element, with the element as `item`. Its output is NOT the normal output: it is `each` (a list, one entry per element: the element under `item` plus that run's outputs) and `count`. Example: after `price` (http.get with for_each over coin rows), `steps.price.each` is `[{item: {symbol: BTC}, status, headers, body, json}, ...]`, so a following transform.map over `${{ steps.price.each }}` uses `item.item.symbol` and `item.json.close`.
Keep for_each to one step where you can: search once with all names in one query instead of once per coin. If a later step must save a list per element, give that db.insert_many its own `for_each: ${{ steps.x.each }}` and `rows: ${{ item.items }}`. There is no flatten function.

Step catalog (`use`: inputs → outputs). Only these exist:
- `http.get`: url, query (map), headers, timeout, fail_on_status (default true) → status, headers, body, json
- `http.download`: url, key, headers, max_size → key, size, mime
- `web.search`: query, limit, freshness (day/week/month/any) → results[] {title, url, snippet, published}
- `html.extract`: url or html, mode (readable/selector), selector → title, text, links, status
- `json.extract`: input, path (JSONPath `$...`) → value
- `transform.map`: items, fields (name → expression using `item`) → items[]
- `transform.filter`: items, condition (`${{ }}` using `item`) → items[]
- `db.query`: sql (read-only SELECT, named params `:name`), params → rows[], count
- `db.insert`: table, row (object), on_conflict (error/ignore/upsert), conflict_key (default: primary key) → inserted, updated, id
- `db.insert_many`: table, rows (list), on_conflict, conflict_key → inserted, updated, skipped
- `db.update`: table, set, where (SQL with :params), params → updated
- `llm.select`: items, instruction, count, add (fields the LLM adds per item, e.g. `{ reason: string }`; types string/number/boolean) → items[] (the original items plus the add fields), indexes[]
- `llm.extract`: input, instruction, output (field → string/number/boolean/date/list) → data
- `bucket.put`: key, content, mime → key, size; `bucket.list`: prefix → objects[]
- `app.notify`: title, message, level (info/warning)

Build rows with transform.map (one output field per column), not with map literals. Row keys of db.insert/insert_many must be real columns of the table; every NOT NULL column without a default must be set; with upsert the row must contain the conflict key. Use `ignore` or `upsert` so a rerun is safe. Do not set `_run_id`/`_fetched_at`.

## 3. Expressions

Written as `${{ ... }}` (expr-lang syntax). A value that is only an expression keeps its type (list, number); an expression inside a longer string becomes text: `"news about ${{ inputs.topic }}"`. YAML: a plain value must not contain `: ` or ` #`, and a `${{ }}` inside a `{ ... }` flow block must be quoted. When in doubt, put the whole value in double quotes: `url: "https://api.example.com/${{ lower(item.symbol) }}/daily"`.
Names: `steps.<id>.<output>` (earlier steps only), `inputs.<name>` (declared inputs only), `item` (inside for_each and per-item fields), `today()` (YYYY-MM-DD), `now()`, `date(s)`, `format_date(d, layout)`, `add_days(d, n)`.
Functions: `map(list, .field)`, `filter(list, .x > 1)`, `all`, `any`, `len`, `lower`, `upper`, `trim`, `split`, `join(list, sep)`, `round`, `int`, `float`, `string`. Inside map/filter predicates use `.field` or `#`; no nested list functions in a predicate. A map literal in a predicate needs parentheses: `map(xs, ({a: .x}))`.
A missing field is an error; use `??` for a default: `${{ steps.price.json.close ?? 0 }}`. Not allowed: `let`, `matches`, method calls (`x.map(...)`, `s.toLowerCase()`: write `map(x, .f)`, `lower(s)`), `$env`.
