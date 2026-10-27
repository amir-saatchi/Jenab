# L05_design_table

Design a table for the crypto exchanges I want to compare: name, country, website and trading fee in percent. Describe the design first; don't create it yet.

Good behaviour: Load database-design before the answer; no skills outside database-design, migrations.

## Turn 1

**User:** Design a table for the crypto exchanges I want to compare: name, country, website and trading fee in percent. Describe the design first; don't create it yet.

> call load_skill {"name":"database-design"}

*Skill loaded: database-design*

> result load_skill:
> 

**Agent:** I propose the following design for the `crypto_exchanges` table:

- **`name`** (TEXT, Primary Key): The name of the exchange.
- **`country`** (TEXT): The country where the exchange is based.
- **`website`** (TEXT): The official website URL.
- **`fee_percent`** (REAL): The trading fee expressed as a percentage (e.g., `0.1` for 0.1%).

If you plan to populate this table using a pipeline in the future, I will also add `_run_id` and `_fetched_at` columns to track the data source and timing.

Does this design work for you?

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2453 | 0 | 0 | 87 | tool_use | 1.7 s |  |
| 2 | 2646 | 0 | 0 | 512 | end | 11.0 s |  |
