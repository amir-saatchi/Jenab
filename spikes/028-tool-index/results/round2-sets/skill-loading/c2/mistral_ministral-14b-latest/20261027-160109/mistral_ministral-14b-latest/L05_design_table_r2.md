# L05_design_table

Design a table for the crypto exchanges I want to compare: name, country, website and trading fee in percent. Describe the design first; don't create it yet.

Good behaviour: Load database-design before the answer; no skills outside database-design, migrations.

## Turn 1

**User:** Design a table for the crypto exchanges I want to compare: name, country, website and trading fee in percent. Describe the design first; don't create it yet.

**Agent:** Here’s the design for a **crypto_exchanges** table to compare exchanges:

### **Columns**
1. **name** (`TEXT NOT NULL`)
   - The name of the exchange (e.g., "Binance", "Coinbase").
   - Primary key: Ensures each exchange is unique.

2. **country** (`TEXT`)
   - The country where the exchange is registered or operates (e.g., "United States", "Singapore").

3. **website** (`TEXT NOT NULL`)
   - The official URL of the exchange (e.g., "https://binance.com").

4. **trading_fee_percent** (`REAL`)
   - The trading fee as a percentage (e.g., `0.1` for 0.1%).

5. **_run_id** (`TEXT`)
   - A unique identifier for the data fetch run (auto-populated by pipelines).

6. **_fetched_at** (`TEXT`)
   - Timestamp of when the data was last fetched (auto-populated by pipelines).

### **Constraints**
- **Primary Key**: `(name)` to ensure no duplicate exchange names.
- **Unique**: `(website)` to avoid duplicate entries for the same exchange.

### **Why This Design?**
- **Flexibility**: The `trading_fee_percent` column can store decimal values (e.g., `0.075` for 0.075%).
- **Auto-tracking**: `_run_id` and `_fetched_at` help track data freshness in pipelines.
- **No redundant tables**: If more attributes (e.g., "minimum trade amount") are needed later, this table can be widened instead of creating parallel tables.

Would you like to proceed with creating this table? If so, I’ll draft the migration steps.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 2491 | 128 | 0 | 371 | end | 7.0 s |  |
