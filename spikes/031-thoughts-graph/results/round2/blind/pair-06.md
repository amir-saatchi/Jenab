# pair-06

**Task:**

Our internal sales report page takes about 12 seconds to load. It runs on PostgreSQL behind a Go API, the data grows by 50,000 rows a day, and the team is two developers. How should we make it load in under 1 second?

---

## Answer A

To reduce your sales report load time from 12 seconds to under 1 second with a small team, you should follow a tiered approach. Start with low-effort database optimizations, move to pre-aggregation if queries remain slow, and implement structural changes only if the data volume demands it.

### Step-by-Step Implementation Plan

**Phase 1: Database Diagnostic & Optimization (Immediate)**
1.  **Analyze Execution Plans:** Use `EXPLAIN (ANALYZE, BUFFERS)` on the report's main SQL queries. Identify whether the bottleneck is a sequential scan, expensive joins, or high-cost sorts.
2.  **Implement Covering Indexes:** Based on the analysis, create indexes on the most frequent filter columns. Use the `INCLUDE` clause to create covering indexes, allowing PostgreSQL to perform "Index Only Scans" and avoid fetching data from the main heap.
3.  **Optimize Query Logic:** Rewrite inefficient joins or subqueries identified in the execution plans to ensure the Go API is requesting only the necessary data.

**Phase 2: Pre-Aggregation (For Heavy Reports)**
If the report calculates totals or averages across millions of rows, real-time calculation will eventually fail regardless of indexing.
1.  **Materialized Views:** Create Materialized Views to store the results of complex aggregations.
2.  **Indexing Views:** Apply indexes directly to the Materialized Views to ensure the Go API can query the summary data instantly.
3.  **Refresh Strategy:** Define a refresh schedule (e.g., hourly or nightly) that balances data freshness with system load.

**Phase 3: Structural Scaling (Long-term)**
As your data grows by 50,000 rows daily, the table size will eventually degrade performance.
1.  **Date Partitioning:** Implement declarative range partitioning (e.g., by month). This allows PostgreSQL to use "partition pruning," scanning only the relevant time slices instead of the entire dataset.

### Risks to Monitor
*   **Write Latency:** Every new index slows down the 50k daily insertions. Balance the number of indexes to ensure that report read-speed doesn't break data ingestion.
*   **Index Bloat:** Frequent updates and high growth can lead to index bloat. Monitor your `VACUUM` behavior to maintain performance.
*   **Data Staleness:** If using Materialized Views, ensure the internal sales team is aware that the data is a "snapshot" and not real-time.

---

## Answer B

Reducing a load time from 12 seconds to under 1 second requires a systematic approach. Since you only have two developers, you cannot afford to "guess" or rewrite the entire system. You need to follow a **Diagnostic $\rightarrow$ Quick Win $\rightarrow$ Structural Change** pipeline.

Here is the step-by-step playbook to solve this.

---

### Phase 1: Diagnosis (Stop Guessing)
Before changing any code, find the exact bottleneck.

1.  **PostgreSQL `EXPLAIN ANALYZE`**: This is your most important tool. Take the query the Go API is running and run it directly in Postgres with `EXPLAIN ANALYZE`.
    *   **Look for "Seq Scan"**: This means Postgres is reading every single row on the disk.
    *   **Look for "External Merge Disk"**: This means your `work_mem` is too low, and Postgres is using the hard drive to sort data.
2.  **API Tracing**: Add simple timing logs in your Go code to see if the delay is:
    *   The DB query (usually 90% of the problem).
    *   JSON Marshalling/Processing in Go.
    *   Network latency/Payload size (transferring 10MB of JSON takes time).

---

### Phase 2: Quick Wins (Low Effort, High Impact)
If the `EXPLAIN ANALYZE` showed "Seq Scans," start here.

1.  **Indexing**: 
    *   **B-Tree Indexes**: Ensure every column used in a `WHERE` clause or `JOIN` is indexed.
    *   **Composite Indexes**: If you always filter by `store_id` AND `created_at`, a single index on `(store_id, created_at)` is significantly faster than two separate indexes.
2.  **Avoid `SELECT *`**: 
    *   Fetching 50 columns when you only display 5 increases I/O and memory usage. Explicitly name your columns.
3.  **Limit the Result Set**:
    *   Does the user actually need 1 million rows on one page? Implement **Pagination** (`LIMIT` and `OFFSET` or, better yet, **Keyset Pagination/Cursor-based** using the ID of the last seen record).

---

### Phase 3: Structural Changes (The "Scaling" Phase)
If you are calculating aggregates (e.g., `SUM(sales)`) over millions of rows, indexes aren't enough. You cannot calculate millions of rows in under 1 second.

1.  **Materialized Views**:
    *   Instead of calculating the report on the fly, create a `MATERIALIZED VIEW` that stores the result of the complex query.
    *   **Refresh Strategy**: Refresh it every hour or once a night: `REFRESH MATERIALIZED VIEW CONCURRENTLY sales_report_summary;`.
2.  **Summary Tables (The Gold Standard)**:
    *   Create a table like `daily_sales_summary`. 
    *   Use a cron job or a DB trigger to increment these totals daily.
    *   Your report then queries the summary table (hundreds of rows) instead of the raw sales table (millions of rows).
3.  **Covering Indexes (Index-Only Scans)**:
    *   If you only need two columns, use an index with the `INCLUDE` clause. This allows Postgres to return the data directly from the index without even touching the main table heap.

---

### Phase 4: Go API Optimizations
Once the DB returns data in < 200ms, optimize the "plumbing."

1.  **Concurrent Fetching**:
    *   If your report page loads three different datasets (e.g., Total Sales, Top Products, Regional Map), don't fetch them sequentially. Use **Goroutines** and `sync.WaitGroup` or `errgroup` to fetch all three from the DB simultaneously.
2.  **Avoid N+1 Queries**:
    *   Ensure you aren't doing one query to get a list of orders and then a separate query inside a loop to get the customer name for each order. Use a `JOIN`.
3.  **Streaming JSON**:
    *   If the response payload is huge, use `json.Encoder` to stream the response directly to the `http.ResponseWriter` instead of loading a massive slice into memory and then calling `json.Marshal`.

---

### Summary Checklist for the 2-Dev Team

| Priority | Action | Estimated Time | Impact |
| :--- | :--- | :--- | :--- |
| **1** | `EXPLAIN ANALYZE` the slow query | 1 hour | Critical (Direction) |
| **2** | Add missing/composite indexes | 2 hours | High |
| **3** | Implement Pagination | 1 day | High |
| **4** | Move aggregates to Materialized Views | 1-2 days | Massive |
| **5** | Use Goroutines for parallel API calls | 4 hours | Medium |

**The "Nuclear" Option:** If the data grows so fast that Materialized Views are too slow, introduce **Redis** to cache the final JSON response of the report for 15 minutes. This reduces DB load to nearly zero.

---

## Your verdict

Winner (A, B or tie): 
Score A (1–10): 
Score B (1–10): 
Why: 
