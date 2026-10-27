# L05_design_table

Design a table for the crypto exchanges I want to compare: name, country, website and trading fee in percent. Describe the design first; don't create it yet.

Good behaviour: Load database-design before the answer; no skills outside database-design, migrations.

## Turn 1

**User:** Design a table for the crypto exchanges I want to compare: name, country, website and trading fee in percent. Describe the design first; don't create it yet.

**Agent:** Here’s the design for a table to track and compare crypto exchanges:

### Table Name: `crypto_exchanges`
This table will store key details about crypto exchanges for comparison.

#### Columns:
1. **`name`** (TEXT, NOT NULL)
   - The name of the exchange (e.g., Binance, Coinbase).
   - **Primary Key**: Unique identifier for each exchange.

2. **`country`** (TEXT)
   - The country where the exchange is registered or primarily operates (e.g., "United States", "Malta").

3. **`website`** (TEXT)
   - The official URL of the exchange (e.g., `https://binance.com`).

4. **`trading_fee_percent`** (REAL)
   - The trading fee as a percentage (e.g., `0.1` for 0.1%).
   - Can store decimal values for precision.

5. **`_run_id`** (TEXT)
   - A unique identifier for the data fetch run (standard for tracking updates).

6. **`_fetched_at`** (TEXT)
   - Timestamp of when the data was last fetched (standard for tracking updates).

---

### Design Notes:
- **Primary Key**: `name` ensures each exchange is uniquely identified.
- **Flexibility**: The `trading_fee_percent` column can be updated if fees change over time.
- **Standard Columns**: `_run_id` and `_fetched_at` are included for tracking data updates, which is useful for pipelines that refresh this data periodically.

Would you like to proceed with creating this table? If so, I’ll draft the migration steps.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2491 | 128 | 0 | 345 | end | 20.9 s |  |
