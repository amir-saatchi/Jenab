# L05_design_table

Design a table for the crypto exchanges I want to compare: name, country, website and trading fee in percent. Describe the design first; don't create it yet.

Good behaviour: Load database-design before the answer; no skills outside database-design, migrations.

## Turn 1

**User:** Design a table for the crypto exchanges I want to compare: name, country, website and trading fee in percent. Describe the design first; don't create it yet.

**Agent:** Here’s the table design for tracking crypto exchanges:

### Table: `crypto_exchanges`
- **Purpose**: Store key details of crypto exchanges for comparison.
- **Columns**:
  1. **name** (`TEXT`): Name of the exchange (e.g., Binance, Coinbase).
     - *Primary key*: Yes (unique identifier for each exchange).
  2. **country** (`TEXT`): Country where the exchange is registered or operates.
  3. **website** (`TEXT`): Official URL of the exchange.
  4. **trading_fee_percent** (`REAL`): Trading fee as a percentage (e.g., 0.1 for 0.1%).
  5. **_run_id** (`TEXT`): Unique identifier for the data fetch run (auto-populated by pipelines).
  6. **_fetched_at** (`TEXT`): Timestamp of when the data was fetched (auto-populated by pipelines).

### Key Features:
- **Primary Key**: `name` ensures no duplicate exchanges.
- **Auto-Columns**: `_run_id` and `_fetched_at` are standard for tracking data provenance.
- **Flexibility**: The `trading_fee_percent` column can store decimal values for precision.

### Why This Design?
- **Simplicity**: Focuses on core attributes for comparison.
- **Scalability**: Easy to add more columns (e.g., `min_deposit`, `supported_cryptos`) later.
- **Pipeline-Friendly**: Auto-columns integrate seamlessly with data pipelines.

Would you like to proceed with creating this table, or adjust any part of the design?

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 137 | 3456 | 0 | 344 | end | 3.5 s |  |
