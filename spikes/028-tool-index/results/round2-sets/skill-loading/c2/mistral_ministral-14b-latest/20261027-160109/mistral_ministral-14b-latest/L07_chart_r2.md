# L07_chart

Build a bar chart of the number of saved news items per day.

Good behaviour: Load config-guide before the first write; no skills outside config-guide, sql-queries.

## Turn 1

**User:** Build a bar chart of the number of saved news items per day.

> call load_skill {"name":"config-guide"}

*Skill loaded: config-guide*

> result load_skill:
> 

> call query {"sql":"SELECT date, COUNT(*) as count FROM btc_news GROUP BY date ORDER BY date"}

> result query:
> date | count
> 2026-10-02 | 1
> 2026-10-03 | 1
> 2026-10-04 | 1
> 2026-10-05 | 1
> (4 rows)

**Agent:** Here’s a YAML config for a bar chart showing the number of saved Bitcoin news items per day:

```yaml
version: 1
id: btc_news_per_day
title: "Bitcoin News Items per Day"
type: chart
chart_type: bar
query: |
  SELECT date, COUNT(*) as count
  FROM btc_news
  GROUP BY date
  ORDER BY date
x:
  field: date
  label: "Date"
  format: date
y:
  - field: count
    label: "Number of Items"
```

This config:
- Groups news items by date and counts them.
- Uses a **bar chart** to visualize the count per day.
- Labels the x-axis as "Date" and the y-axis as "Number of Items."

Would you like to save this as a new view?

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 39 | 2560 | 0 | 13 | tool_use | 1.2 s |  |
| 2 | 121 | 4992 | 0 | 27 | tool_use | 1.2 s |  |
| 3 | 2647 | 2560 | 0 | 180 | end | 4.1 s |  |
