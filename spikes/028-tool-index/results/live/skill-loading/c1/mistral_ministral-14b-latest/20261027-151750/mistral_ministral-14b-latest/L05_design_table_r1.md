# L05_design_table

Design a table for the crypto exchanges I want to compare: name, country, website and trading fee in percent. Describe the design first; don't create it yet.

Good behaviour: Load database-design before the answer; no skills outside database-design, migrations.

## Turn 1

**User:** Design a table for the crypto exchanges I want to compare: name, country, website and trading fee in percent. Describe the design first; don't create it yet.

**Agent:** Here’s the design for a table to track crypto exchanges for comparison:

### Table: `crypto_exchanges`
- **Columns**:
  - `name` (TEXT, NOT NULL): The name of the exchange (e.g., Binance, Coinbase).
  - `country` (TEXT, NOT NULL): The country where the exchange is registered or operates.
  - `website` (TEXT, NOT NULL): The URL of the exchange’s official website.
  - `trading_fee_percent` (REAL, NOT NULL): The trading fee as a percentage (e.g., 0.1 for 0.1%).
  - `_run_id` (TEXT): A unique identifier for the data fetch run (auto-populated by pipelines).
  - `_fetched_at` (TEXT): The timestamp when the data was fetched (auto-populated by pipelines).

- **Primary Key**: `(name)` (ensures each exchange is listed once).
- **Notes**:
  - The `_run_id` and `_fetched_at` columns are standard for tracking data provenance in Jenab.
  - If an exchange has multiple fee tiers (e.g., maker/taker), you could widen the table later (e.g., add `maker_fee_percent` and `taker_fee_percent` columns) instead of creating parallel tables.

Would you like to proceed with creating this table, or adjust the design?

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 137 | 3456 | 0 | 289 | end | 2.9 s |  |
