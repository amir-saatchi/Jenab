# pair-05

**Task:**

We have to move a 200 GB PostgreSQL 14 database to a new server in another data center. The app may be down for at most 5 minutes. How should we do it?

---

## Answer A

To move a 200 GB database with a maximum downtime of 5 minutes, you cannot use a traditional "Dump and Restore" (logical backup), as transferring and indexing 200 GB would take hours.

The correct approach is **Physical Streaming Replication**. This allows you to synchronize the data in the background while the application is still running, reducing the final downtime to the few seconds it takes to promote the standby server to primary.

Here is the step-by-step execution plan.

---

### Phase 1: Preparation (Zero Downtime)
Perform these steps while your application is online and running on the **Old Server**.

1.  **Network Connectivity:** Ensure the **New Server** can reach the **Old Server** on port `5432`. Since they are in different data centers, ensure you are using a secure tunnel (VPN or SSH tunnel) or a restricted firewall rule.
2.  **Configure Old Server (`postgresql.conf`):**
    Ensure the following settings are active:
    *   `wal_level = replica`
    *   `max_wal_senders = 10`
    *   `archive_mode = on` (recommended)
3.  **Configure Access (`pg_hba.conf`):**
    Add an entry to allow the New Server's IP to connect for replication:
    ```conf
    host  replication  replication_user  [New_Server_IP]/32  md5
    ```
4.  **Create Replication User:**
    ```sql
    CREATE ROLE replication_user WITH REPLICATION LOGIN PASSWORD 'your_password';
    ```

### Phase 2: Initial Synchronization (Zero Downtime)
Now, we move the bulk of the 200 GB to the new server without stopping the app.

1.  **Stop PostgreSQL on the New Server** (if it's running).
2.  **Perform a Base Backup:**
    Run this command on the **New Server** to pull a full copy of the data from the Old Server:
    ```bash
    pg_basebackup -h [Old_Server_IP] -D /var/lib/postgresql/data -U replication_user -P -R -X stream
    ```
    *   `-D`: Data directory.
    *   `-R`: Creates the `standby.signal` file and `postgresql.auto.conf` automatically.
    *   `-X stream`: Streams the WAL files while the backup is running.
3.  **Start PostgreSQL on the New Server.**
    The New Server will now start in **Hot Standby** mode. It will connect to the Old Server and "catch up" on every change happening in real-time.

### Phase 3: Monitoring (Zero Downtime)
Before scheduling the cutover, you must ensure the servers are perfectly in sync.

1.  **Check Replication Lag:**
    Run this on the **Old Server**:
    ```sql
    SELECT pg_current_wal_lsn(), pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn) 
    FROM pg_stat_replication;
    ```
    Wait until the lag (diff) is near zero.

### Phase 4: The Switchover (The 5-Minute Window)
This is the only part where the application is offline.

1.  **Stop the Application:** Stop all app servers or put them in "Maintenance Mode" to ensure no new writes are hitting the Old Server. **(Timer starts now)**.
2.  **Final Sync Check:** Verify one last time that the New Server has caught up to the Old Server.
3.  **Promote the New Server:**
    Run this on the **New Server** to turn it from a Read-Only standby into a Read-Write primary:
    ```bash
    pg_ctl promote -D /var/lib/postgresql/data
    # OR via SQL:
    SELECT pg_promote();
    ```
4.  **Update Application Configuration:**
    Update your app's connection string (or update your DNS record/Load Balancer) to point to the **New Server IP**.
5.  **Start the Application:** Bring the app back online. **(Timer ends)**.

---

### Summary Table

| Step | Action | Impact | Duration |
| :--- | :--- | :--- | :--- |
| **Config** | Setup `pg_hba.conf` & `wal_level` | None | 10 mins |
| **Sync** | `pg_basebackup` | Low CPU/Network | $\sim$ 2-5 hours |
| **Replication** | Streaming WAL logs | Negligible | Continuous |
| **Cutover** | Stop App $\rightarrow$ Promote $\rightarrow$ Start App | **Downtime** | **$< 5$ minutes** |

### Pro-Tips for a Smoother Migration:
*   **DNS TTL:** If you use a DNS name for your DB host, lower the TTL (Time to Live) to 60 seconds a day before the migration to avoid DNS caching issues.
*   **Dry Run:** If possible, test the `pg_basebackup` process on a staging environment to estimate the network transfer time.
*   **Compression:** If the network between data centers is slow, use a tool like `mbstream` or use the `--compress` flag if using newer PG versions to speed up the initial 200 GB transfer.

---

## Answer B

To migrate a 200 GB PostgreSQL 14 database across data centers with a maximum downtime of 5 minutes, the most reliable approach is **Physical Streaming Replication**. This method ensures the destination is a bit-for-bit copy of the source, minimizing the cutover window to only the time it takes to promote the standby and update connection strings.

### Execution Plan

#### 1. Preparation and Initial Sync
*   **Configure Source:** Set `wal_level = replica` and ensure `max_wal_senders` is sufficient to accommodate the new standby.
*   **Baseline Transfer:** Use `pg_basebackup` to transfer the 200 GB dataset to the new server. To prevent network saturation between data centers, use compression (e.g., zstd) or a tool like `mbuffer` to manage the throughput.
*   **Initialize Standby:** Start the destination server in standby mode. It will begin consuming Write-Ahead Logs (WAL) from the primary to catch up with all transactions that occurred during the baseline transfer.

#### 2. Stabilization and Verification
*   **Monitor Lag:** Use `pg_stat_replication` on the primary server to monitor the replication lag. The standby must be "caught up" (near-zero lag) before attempting the cutover.
*   **Verify Parity:** Ensure the Log Sequence Number (LSN) on the standby is closely trailing the primary.

#### 3. Final Cutover (The 5-Minute Window)
*   **Stop Writes:** Put the application into read-only mode or stop the application services to ensure no new data is written to the source.
*   **Final Sync:** Wait a few seconds for the final remaining WAL records to stream to the destination.
*   **Promote Standby:** Execute `SELECT pg_promote();` or use `pg_ctl promote` on the new server to transition it from a read-only standby to a read-write primary.
*   **Redirect Traffic:** Update the application connection strings or DNS records to point to the new server's IP address.
*   **Resume Application:** Restart the application or disable read-only mode.

### Critical Risks to Manage
*   **WAN Bandwidth:** A 200 GB transfer across data centers can saturate the link, impacting production traffic. Use bandwidth throttling or compressed transfers to mitigate this.
*   **WAL Accumulation:** If the network is slow and the primary generates WAL files faster than the standby can consume them, the primary's disk may fill up. Use **replication slots** to ensure the primary does not delete WAL files until the standby has received them.
*   **Network Latency:** High latency can increase the time it takes for the final sync to complete. Ensure the network path is optimized before the cutover window begins.

---

## Your verdict

Winner (A, B or tie): 
Score A (1–10): 
Score B (1–10): 
Why: 
