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

**Table: `crypto_exchanges`**
- `name` (TEXT): The name of the exchange. **Primary Key**.
- `country` (TEXT): The country where the exchange is based or registered.
- `website` (TEXT): The official URL of the exchange.
- `trading_fee_percent` (REAL): The trading fee as a percentage (e.g., `0.1` for 0.1%).

If you plan to populate this table using a pipeline, I will also add the standard `_run_id` and `_fetched_at` columns to track when the data was updated.

Does this design work for you?

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 3089 | 0 | 0 | 119 | tool_use | 1.9 s |  |
| 2 | 3314 | 0 | 0 | 540 | end | 2.2 s |  |
