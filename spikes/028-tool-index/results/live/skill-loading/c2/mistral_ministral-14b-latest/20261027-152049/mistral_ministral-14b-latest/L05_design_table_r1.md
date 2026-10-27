# L05_design_table

Design a table for the crypto exchanges I want to compare: name, country, website and trading fee in percent. Describe the design first; don't create it yet.

Good behaviour: Load database-design before the answer; no skills outside database-design, migrations.

## Turn 1

**User:** Design a table for the crypto exchanges I want to compare: name, country, website and trading fee in percent. Describe the design first; don't create it yet.

> call load_skill {"name":"database-design"}

*Skill loaded: database-design*

> result load_skill:
> 

**Agent:** Here’s the design for a **crypto_exchanges** table to track exchanges for comparison:

### Design
- **Name**: `crypto_exchanges`
- **Columns**:
  - `name` (TEXT, NOT NULL): Name of the exchange (e.g., "Binance", "Coinbase").
  - `country` (TEXT): Country where the exchange is registered.
  - `website` (TEXT): URL of the exchange (e.g., "https://binance.com").
  - `trading_fee_percent` (REAL): Trading fee as a percentage (e.g., `0.1` for 0.1%).
  - `_run_id` (TEXT): Pipeline runtime ID (auto-filled by Jenab).
  - `_fetched_at` (TEXT): Timestamp of data fetch (auto-filled by Jenab).

- **Primary Key**: `(name)` (ensures each exchange is unique by name).

### Notes
- This design assumes you want to track **unique exchanges by name** and compare their attributes (country, website, fees).
- If you later need to track **multiple entries per exchange** (e.g., different fee tiers), we can add a `type` or `variant` column or migrate to a wider design.
- The `_run_id` and `_fetched_at` columns are required for pipelines that populate this table.

Would you like to proceed with creating this table? If so, I’ll draft the migration steps.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 38 | 2176 | 0 | 13 | tool_use | 0.9 s |  |
| 2 | 140 | 2176 | 0 | 302 | end | 3.6 s |  |
