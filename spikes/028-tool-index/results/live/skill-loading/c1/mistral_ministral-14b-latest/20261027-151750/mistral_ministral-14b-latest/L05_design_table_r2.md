# L05_design_table

Design a table for the crypto exchanges I want to compare: name, country, website and trading fee in percent. Describe the design first; don't create it yet.

Good behaviour: Load database-design before the answer; no skills outside database-design, migrations.

## Turn 1

**User:** Design a table for the crypto exchanges I want to compare: name, country, website and trading fee in percent. Describe the design first; don't create it yet.

**Agent:** Here’s the design for a table to track and compare crypto exchanges:

### Table Name: `crypto_exchanges`
### Columns:
1. **name** (`TEXT`): The name of the exchange (e.g., Binance, Coinbase).
   - *Primary Key*: Yes (unique identifier for each exchange).
   - *Purpose*: Identify the exchange.

2. **country** (`TEXT`): The country where the exchange is registered or primarily operates (e.g., "United States," "Singapore").
   - *Purpose*: Track regulatory jurisdiction and regional availability.

3. **website** (`TEXT`): The official URL of the exchange (e.g., `https://binance.com`).
   - *Purpose*: Direct users to the exchange’s platform.

4. **trading_fee_percent** (`REAL`): The trading fee as a percentage (e.g., `0.1` for 0.1%).
   - *Purpose*: Compare fee structures across exchanges.

5. **_run_id** (`TEXT`): A unique identifier for the data fetch run (standard for tracking data updates).
   - *Purpose*: Track when and how data was fetched.

6. **_fetched_at** (`TEXT`): Timestamp of when the data was last fetched (standard for tracking freshness).
   - *Purpose*: Ensure data is up-to-date.

### Key Features:
- **Primary Key**: `name` (ensures no duplicate exchanges).
- **Indexes**: Consider adding an index on `country` or `trading_fee_percent` if filtering by these columns is common.
- **Data Type Notes**:
  - `trading_fee_percent` uses `REAL` to support decimal values (e.g., `0.075` for 0.075%).
  - `_run_id` and `_fetched_at` follow the project’s standard for tracking data provenance.

### Example Row:
```plaintext
name: Binance, country: Cayman Islands, website: https://binance.com, trading_fee_percent: 0.1, _run_id: abc123, _fetched_at: 2026-10-06
```

### Next Steps:
Let me know if you'd like to:
1. **Create this table** (I’ll generate the migration).
2. **Add more columns** (e.g., supported cryptocurrencies, withdrawal fees, or API availability).
3. **Adjust the design** (e.g., use a different primary key or data type).

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 137 | 3456 | 0 | 538 | end | 4.6 s |  |
