# L05_design_table

Design a table for the crypto exchanges I want to compare: name, country, website and trading fee in percent. Describe the design first; don't create it yet.

Good behaviour: Load database-design before the answer; no skills outside database-design, migrations.

## Turn 1

**User:** Design a table for the crypto exchanges I want to compare: name, country, website and trading fee in percent. Describe the design first; don't create it yet.

> call load_skill {"name":"database-design"}

*Skill loaded: database-design*

> result load_skill:
> 

**Agent:** I suggest the following design for the `crypto_exchanges` table:

- **`name`** (TEXT): The name of the exchange. This will serve as the **primary key**.
- **`country`** (TEXT): The home country or jurisdiction.
- **`website`** (TEXT): The official URL.
- **`trading_fee_percent`** (REAL): The fee as a decimal (e.g., `0.1` for 0.1%).
- **`_run_id`** (TEXT): Tracking ID for pipeline runs.
- **`_fetched_at`** (TEXT): Timestamp of when the data was last updated.

Does this look correct, or would you like to add any other fields (e.g., supported coins or volume)?

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 3089 | 0 | 0 | 87 | tool_use | 1.2 s |  |
| 2 | 3282 | 0 | 0 | 523 | end | 4.2 s |  |
