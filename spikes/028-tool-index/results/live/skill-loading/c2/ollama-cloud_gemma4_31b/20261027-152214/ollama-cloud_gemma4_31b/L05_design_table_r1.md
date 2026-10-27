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
- **`website`** (TEXT): The official URL.
- **`trading_fee`** (REAL): The trading fee as a percentage (e.g., `0.1` for 0.1%).
- **`_run_id`** (TEXT): To track the pipeline run that fetched the data.
- **`_fetched_at`** (TEXT): Timestamp of when the data was retrieved.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2145 | 0 | 0 | 107 | tool_use | 2.0 s |  |
| 2 | 2358 | 0 | 0 | 433 | end | 2.2 s |  |
