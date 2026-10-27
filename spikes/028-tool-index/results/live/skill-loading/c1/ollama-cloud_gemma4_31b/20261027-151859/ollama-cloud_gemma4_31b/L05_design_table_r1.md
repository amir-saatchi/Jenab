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

**Columns:**
- `name` (TEXT): The name of the exchange. **Primary Key**.
- `country` (TEXT): The country where the exchange is based.
- `website` (TEXT): The official website URL.
- `trading_fee_percent` (REAL): The trading fee as a percentage (e.g., 0.1 for 0.1%).
- `_run_id` (TEXT): ID of the pipeline run that fetched the data.
- `_fetched_at` (TEXT): Timestamp of when the data was fetched.

I've included `_run_id` and `_fetched_at` so the table is ready if you decide to automate the data collection via a pipeline later.

Does this look correct, or would you like to add any other fields?

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 3089 | 0 | 0 | 110 | tool_use | 1.7 s |  |
| 2 | 3305 | 0 | 0 | 493 | end | 2.4 s |  |
