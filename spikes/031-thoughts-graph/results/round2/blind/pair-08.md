# pair-08

**Task:**

Our internal sales report page takes about 12 seconds to load. It runs on PostgreSQL behind a Go API, the data grows by 50,000 rows a day, and the team is two developers. How should we make it load in under 1 second?

---

## Answer A

To bring your internal sales report page load time down from 12 seconds to under 1 second with a lean two-developer team, you should combine **database optimization** with **API-layer caching**. A multi-layered approach ensures that even when cache expires, your database can handle the load efficiently without burning you out on complex maintenance.

Here is the step-by-step approach to take:

### 1. Optimize Database Queries and Indexes
Before caching, ensure your underlying database queries are as fast as possible, keeping in mind that your table grows by 50,000 rows a day.
* **Analyze Execution Plans:** Run `EXPLAIN ANALYZE` on your heavy report queries to identify sequential scans, expensive joins, or sorting bottlenecks.
* **Implement Covering Indexes:** Use PostgreSQL composite indexes with `INCLUDE` clauses. This allows the database to perform index-only scans for your dashboard metrics without touching the table heap, significantly speeding up reads.

### 2. Implement API-Layer Caching with Redis
With ~18 million new rows added per year, raw database queries will eventually degrade. Offload the read load by caching the final report JSON or heavy aggregate blocks in Redis.
* **Define TTL and Invalidation:** Set an appropriate Time-To-Live (TTL) based on how real-time your sales data needs to be (e.g., refreshing every 5 to 15 minutes, or hourly).
* **Prevent Cache Stampedes:** When the cache expires, multiple concurrent users hitting the page could trigger the heavy 12-second query simultaneously, exhausting your database connection pool. Use Go's `singleflight` package to collapse identical concurrent requests so that only one database query runs while others wait for the result.

---

### Risks to Watch
* **Write Amplification and Bloat:** Covering indexes are wide because they duplicate column data in leaf nodes. With 50,000 daily inserts, this leads to write amplification, index bloat, and heavier `VACUUM` overhead. Monitor index size and write performance closely.
* **Long-Term Scaling:** Raw indexes alone will hit limits as the dataset scales into hundreds of millions of rows. If index optimizations and Redis caching aren't enough long-term, prepare to pivot toward hourly summary tables or partitioned materialized views.

---

## Answer B

With a small team (2 developers), a data growth rate of 50,000 rows/day (approx. 18 million rows/year), and a strict target of <1 second, you need to focus on **high-impact, low-effort solutions first**. 

At this volume, a 12-second load time almost certainly means you are doing full table scans, missing indexes, processing heavy aggregations on the fly, or transferring too much data over the network.

Here is a step-by-step playbook to get your sales report under 1 second, ordered from easiest to most impactful.

---

### Phase 1: The Quick Wins (Days 1–3)
*Estimated effort: 2–4 hours. Potential gain: 50%–80% faster.*

#### 1. Analyze with `EXPLAIN ANALYZE`
Stop guessing where the bottleneck is. Run this in your PostgreSQL console:
```sql
EXPLAIN ANALYZE SELECT * FROM sales_reports WHERE ...;
```
Look for `Seq Scan` (Sequential Scan). If PostgreSQL is reading the entire table row by row to find your data, you need indexes immediately.

#### 2. Add Missing Indexes
Sales reports usually filter and group by date ranges, sales reps, regions, or product IDs. 
* Ensure your `WHERE` clauses and `GROUP BY` columns are indexed.
* **Composite Indexes matter:** If you query by `WHERE store_id = X AND sale_date >= Y`, create a composite index:
  ```sql
  CREATE INDEX idx_sales_store_date ON sales(store_id, sale_date);
  ```
* *Warning:* Don’t over-index. Every index slows down writes, but with 50k rows/day (less than 1 row/second), write overhead is negligible.

#### 3. Implement Pagination or Limit Results
Is the Go API returning all rows at once, forcing the frontend to render thousands of DOM elements?
* If it’s a table, implement server-side pagination (`LIMIT 50 OFFSET X` or cursor-based pagination).
* If it’s a chart, don’t send raw transaction logs; send pre-aggregated data points (e.g., daily totals, not second-by-second data).

---

### Phase 2: Architecture & Query Optimization (Week 1)
*Estimated effort: 1–2 days. Potential gain: Sub-second loads for standard queries.*

#### 4. Shift Aggregations to the Database
Make sure your Go API isn't fetching 100,000 rows and using Go structs to calculate sums, averages, and groupings. PostgreSQL is built for this.
* Move all `SUM()`, `COUNT()`, `AVG()`, and `GROUP BY` logic into SQL. Let the database do the heavy lifting and only return the final numbers (e.g., 30 rows for the last 30 days) to Go.

#### 5. Connection Pooling in Go
Ensure your Go application isn't opening a new database connection for every incoming HTTP request. Use `database/sql` properly and configure the pool:
```go
db.SetMaxOpenConns(25)
db.SetMaxIdleConns(25)
db.SetConnMaxLifetime(5 * time.Minute)
```

---

### Phase 3: The Game Changer – Materialized Views (Week 2)
*Estimated effort: 2–3 days. This is your ticket to <1s loads permanently.*

Sales reports rarely need real-time, second-by-second accuracy. Users usually want to see yesterday's data, or hourly trends up to the current hour.

#### 6. Use PostgreSQL Materialized Views
Instead of querying a raw `sales` table with millions of rows, create a Materialized View that pre-aggregates the data.

```sql
CREATE MATERIALIZED VIEW mv_daily_sales_summary AS
SELECT 
    DATE(sale_date) as sale_day,
    store_id,
    COUNT(*) as total_transactions,
    SUM(amount) as total_revenue
FROM sales
GROUP BY DATE(sale_date), store_id;

-- Create an index on the view
CREATE UNIQUE INDEX idx_mv_daily_sales ON mv_daily_sales_summary (sale_day, store_id);
```

* **How it works:** Querying `mv_daily_sales_summary` takes milliseconds because the heavy lifting (grouping millions of rows) was already done in the background.
* **Keeping it fresh:** Set up a cron job (or `pg_cron`) to refresh the view periodically (e.g., every hour or once a day):
  ```sql
  REFRESH MATERIALIZED VIEW CONCURRENTLY mv_daily_sales_summary;
  ```

---

### Phase 4: Caching Layer (If needed)
*Estimated effort: 1 day.*

If multiple internal users are hitting the same reports, or if the dashboard loads multiple widgets:

#### 7. Cache in Go or Redis
* **In-Memory Cache (Go):** For a simple approach, use an in-memory cache like `sync.Map` or a package like `ristretto` or `bigcache` directly inside your Go API. Cache heavy query results for 5–15 minutes.
* **Redis:** If you eventually scale to multiple Go API instances behind a load balancer, introduce Redis to cache the JSON responses of the heavy report endpoints.

---

### Summary Checklist for 2 Developers:
1. **Day 1:** Run `EXPLAIN ANALYZE` on your slowest report query and add missing indexes.
2. **Day 2:** Ensure heavy math (aggregations) is done in SQL, not in Go.
3. **Week 2:** Build a **Materialized View** for the dashboard data and refresh it hourly.
4. **Result:** Your Go API will query a pre-calculated summary table with an index, returning data in **< 200ms**, leaving plenty of headroom for network latency.

---

## Your verdict

Winner (A, B or tie): 
Score A (1–10): 
Score B (1–10): 
Why: 
