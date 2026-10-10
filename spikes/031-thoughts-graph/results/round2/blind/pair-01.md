# pair-01

**Task:**

We have to move a 200 GB PostgreSQL 14 database to a new server in another data center. The app may be down for at most 5 minutes. How should we do it?

---

## Answer A

To migrate your 200 GB PostgreSQL 14 database to a new server in another data center with a maximum downtime of 5 minutes, the best approach is to use **PostgreSQL Native Physical Streaming Replication** combined with a secure network tunnel. 

Here is the step-by-step migration plan, along with the key risks to monitor.

---

### Step-by-Step Migration Plan

1. **Establish a Secure Network Tunnel**
   * Set up a secure network tunnel (such as WireGuard or `stunnel`) between the old and new data center servers to protect data in transit.

2. **Configure and Benchmark Network/Bandwidth**
   * Measure the actual cross-DC bandwidth via tools like `iperf3`. 
   * Configure `postgresql.conf` on the source server (`wal_level = replica`, appropriate `max_wal_senders`, etc.) and adjust `pg_hba.conf` to allow replication connections.

3. **Bootstrap the Replica with Rate Limiting**
   * Initiate the initial data sync using `pg_basebackup`. 
   * **Crucial:** Rate-limit `pg_basebackup` (using network-level limiters or `pv`) to prevent saturating the cross-datacenter link and starving other critical services.
   * *Alternative for very constrained bandwidth:* Seed the new server using an encrypted physical disk snapshot, then resume via streaming.

4. **Monitor Replication & WAL Catch-up**
   * Continuously monitor replication lag via `pg_stat_replication` and check network throughput to ensure the replica comfortably tracks the primary's WAL stream.

5. **Perform a Staging Dry-Run**
   * Execute a full dry-run of the cutover process in a staging environment. Measure exact timings to ensure the final switch fits safely within 3 minutes, leaving a 2-minute buffer.

6. **Execute Final Cutover (Within 5-Minute Window)**
   * **Stop the application** to halt incoming write transactions.
   * Monitor `pg_stat_replication` until the replication lag reaches **zero**.
   * **Promote the standby database** on the new server using `pg_ctl promote`.
   * **Update connection strings or DNS** to point the application to the new database server.
   * Start the application and verify its stability.

---

### Risks to Watch

* **Cross-DC Network Bandwidth & Latency:** A 200 GB initial transfer can heavily impact network links if unmanaged. Always rate-limit the initial sync and verify link performance beforehand. High latency during cutover can also slightly delay final WAL flushing and promotion notifications.
* **WAL Generation Rate:** If your application writes data faster than the cross-DC link can transmit WAL updates, the replica will fall behind. Monitor network throughput closely to ensure replication sustainability.

---

## Answer B

Moving a 200 GB PostgreSQL 14 database to a new data center with a **maximum 5-minute downtime** requires a strategy that minimizes data transfer time and maximizes preparation. 

At 200 GB, over a standard encrypted WAN connection, copying the raw data can take anywhere from 10 minutes to a few hours depending on bandwidth. Therefore, you **cannot** just dump and restore during the maintenance window.

Here is the step-by-step blueprint to achieve this using **PostgreSQL Streaming Replication**.

---

### Phase 1: Preparation (Days before the migration)

1. **Network & Security:**
   * Ensure secure network connectivity (VPN, VPC peering, or SSH tunneling) between the old data center (Source) and the new data center (Target).
   * Open port `5432` between the two servers.
   * Verify bandwidth. (A 1Gbps link can transfer 200 GB in ~27 minutes; a 10Gbps link in ~3 minutes. If your bandwidth is low, you may need to compress the initial sync).

2. **Server Provisioning:**
   * Provision the Target server with identical or better hardware.
   * Install **PostgreSQL 14** (ensure the *exact* minor version matches the source).
   * Apply identical OS-level tunings (sysctl, transparent huge pages disabled, etc.) and `postgresql.conf` settings (especially `shared_buffers`, `max_connections`, etc.).

---

### Phase 2: Initial Setup (Hours before the migration)

1. **Configure Source for Replication:**
   Modify `postgresql.conf` on the **Source**:
   ```ini
   wal_level = replica
   max_wal_senders = 5
   archive_mode = on
   archive_command = 'cd .' # Or your actual archiving command
   ```
   Add a replication entry to `pg_hba.conf` on the **Source** to allow the Target IP:
   ```text
   host    replication     replicator     <TARGET_SERVER_IP>/32     md5
   ```
   Restart PostgreSQL on the Source (or reload if `wal_level` was already `replica`).

2. **Create a Replication User on Source:**
   ```sql
   CREATE USER replicator WITH REPLICATION ENCRYPTED PASSWORD 'your_secure_password';
   ```

---

### Phase 3: The Base Backup (Day of migration - Zero Downtime)

To avoid downtime during the heavy lifting, you will stream a base backup from the Source to the Target while the application is still running.

1. **Take the Backup using `pg_basebackup`:**
   Run this command on the **Target** server. This streams the entire 200 GB database without locking writes on the source:
   ```bash
   pg_basebackup -h <SOURCE_SERVER_IP> -U replicator -D /var/lib/postgresql/14/main -P -v -R -X stream
   ```
   * `-P`: Shows progress.
   * `-R`: Automatically writes `standby.signal` and configures `primary_conninfo` in `postgresql.auto.conf`.

2. **Verify Replication is Running:**
   On the **Target**, check the logs to ensure it is connected and streaming WAL logs from the Source.
   On the **Source**, run:
   ```sql
   SELECT client_addr, state, sync_state FROM pg_stat_replication;
   ```
   You should see the Target connected as a replica.

---

### Phase 4: The Cutover (The 5-Minute Window)

When you are ready to switch over, execute the following steps quickly:

1. **Stop the Application (Downtime starts):**
   * Put your application into maintenance mode or shut it down. This stops all writes to the Source database.

2. **Wait for Replication to Catch Up:**
   On the **Source**, check that the LSN (Log Sequence Number) matches on both servers, or check `pg_stat_replication` to ensure the replica is fully caught up (`lag = 0` or very close to it).

3. **Promote the Target Database:**
   Log into the **Target** server and promote it to become the primary database:
   ```bash
   # If using systemd:
   sudo systemctl restart postgresql
   
   # Or using pg_ctl:
   pg_ctl promote -D /var/lib/postgresql/14/main
   ```
   The database is now writable and is the new Source of Truth.

4. **Update Application Configuration:**
   * Update your application's connection strings (DNS update, environment variables, or load balancer) to point to the **Target** server's IP/hostname.

5. **Start the Application:**
   * Bring your application back online and smoke-test it.

---

### Summary of Timeline & Risk Mitigation

* **Total Downtime:** Typically **1 to 3 minutes** (mostly spent on application shutdown, waiting for the last few megabytes of WAL to replicate, promoting the standby, and changing DNS/connection strings).
* **Rollback Plan:** If something goes terribly wrong during the cutover, the Source database was never modified (only read from). You can simply point the application back to the old Source server, provided you haven't accepted new writes on the Target yet. 
* **DNS Tip:** If you use a DNS record (e.g., `db.internal`) for the database connection, lower its TTL (Time To Live) to 5-10 seconds *at least 24 hours before* the migration. This ensures the application picks up the new IP almost instantly without waiting for DNS propagation.

---

## Your verdict

Winner (A, B or tie): 
Score A (1–10): 
Score B (1–10): 
Why: 
