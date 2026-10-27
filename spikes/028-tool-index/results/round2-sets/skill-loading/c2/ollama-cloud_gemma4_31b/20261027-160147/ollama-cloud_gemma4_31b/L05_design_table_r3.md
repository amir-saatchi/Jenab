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
- **`website`** (TEXT): The URL of the exchange.
- **`trading_fee_pct`** (REAL): The trading fee expressed as a percentage.

Does this look correct to you, or would you like to add any other columns?

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2453 | 0 | 0 | 128 | tool_use | 1.2 s |  |
| 2 | 2687 | 0 | 0 | 362 | end | 3.4 s |  |
