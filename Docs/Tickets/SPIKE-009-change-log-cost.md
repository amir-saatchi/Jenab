# SPIKE-009 — Change log and undo cost
**Type:** Spike
**Status:** Done
**Gate:** 2

## Question
Is the change log from SPEC 2.5–2.6 fast and small enough, with row keys in the indexed table `_jenab_change_rows`?

1. A 5,000-row upsert in 500-row chunks, capturing before-images of updated rows: time compared with a plain upsert
2. Undo of that upsert, with and without conflicts
3. The conflict check: time to find later changes to the same rows when the log holds 1 million row entries
4. Database size after one simulated year: a daily 500-row pipeline, 20 chat turns a day, and the 90-day before-image cleanup

## Done when
Each case has a measured time and size, and either the design holds or the spec is changed.

Targets: before-images add at most 50% to write time; the conflict check takes under 100 ms; the change log stays below 30% of the database size.

## Result
Run on 2026-09-27, Windows 11, Go 1.26.0, modernc v1.59.0, WAL with `synchronous = NORMAL`.
- **Setup:** before-images are stored per row, as JSON, in `_jenab_change_rows.before`, next to a `kind` (`i` inserted, `u` updated).
- **Layouts tested:** A = `WITHOUT ROWID` table keyed on `(table_name, row_key, change_id)`; B = rowid table with an index on the same columns. Both also index `change_id`.
- **Code and full output:** [spikes/009-change-log-cost](../../spikes/009-change-log-cost/) (`results.md`).

| Question | Target | Result (layout B) | Met? |
|---|---|---|---|
| 1. Before-images, new rows only | ≤ +50% | 21 → 57 ms per 5,000 rows | no, but small in absolute terms |
| 1. Before-images, 50–100% updates | ≤ +50% | 151 → 274 ms and 288 → 770 ms (+80% to +235%) | **no** |
| 2. Undo a 5,000-row run | — | 260–320 ms; correct with and without conflicts, and for redo | yes |
| 2. Undo a 3-row turn | — | under 1 ms (below the clock resolution, see Limits) | yes |
| 3. Conflict check, 1M row entries | < 100 ms | 8–10 ms; 0.05 ms for a 3-row turn | yes |
| 4. One year, cleanup drops before-images only | < 30% | 55% | **no** |
| 4. One year, cleanup also drops row entries | < 30% | 30% (A: 28%) | yes |

**Findings**
- **Undo and conflict checks are cheap.** All undo cases restored exactly the right rows. The conflict check stayed an index lookup even at 1 million entries.
- **Updates cost more than inserts.** Recording row keys alone makes an update-heavy upsert 2–3 times slower, and before-images add another 20–50%. New rows cost little because their keys arrive in order. Updated rows hit random places in the key index.
  - A 64 MB page cache made no difference, so the likely cause is writing many scattered index pages per commit, not cache misses.
  - In absolute terms, the worst case is about 0.15 ms per updated row, under 1 s for 5,000 rows.
- **Size comes from row entries, not from before-images.** There is one row entry per touched row, including inserts. For a narrow table like `prices`, that is about as big as the data itself. Dropping before-images after 90 days saved only 4%.
- **Deleting row entries after 90 days fixes the size.**
  - What is kept: the change entry, with row counts per kind in `after`.
  - Result: the log falls to 28–30% after one year, and its share keeps shrinking because it only holds the last 90 days.
  - Daily cleanup cost: median 9 ms (B) or 18 ms (A).
- **Layout B vs A:**
  - B's cleanup is steadier: max 81 ms, against 2.2 s once for A.
  - B's undo is a bit faster.
  - A is about 10% smaller.
  - Write times were within noise of each other.

**Limits:** timings on one machine and with synthetic data. Go's clock on this Windows machine moves only every ~0.5 ms (found in SPIKE-010), so single timings under 1 ms are rounded; the averaged ones (conflict check for a 3-row turn) are not affected. Rows touched by later changes inside the 90 days are still covered. After 90 days a change can no longer be undone.

## Decision
**Decided (2026-09-27):**
- **Layout B:** keep `_jenab_change_rows` as a rowid table.
  - Columns: `change_id, table_name, row_key, kind, before`.
  - Indexes: `(table_name, row_key, change_id)` and `(change_id)`.
- **Where before-images live:**
  - rows: per row, in `_jenab_change_rows.before`
  - configs, memory and objects: the previous revision stays in `_jenab_changes.before`
- **Undo window of 90 days (configurable):** after 90 days the cleanup deletes the row entries and before-images and keeps only the change entry, with row counts in `after`. Older changes are history only and cannot be undone. The cleanup works in small steps from a stored watermark.
- **Change the write target** from "+50%" to an absolute limit: a 5,000-row upsert with change log in under 1 s.
