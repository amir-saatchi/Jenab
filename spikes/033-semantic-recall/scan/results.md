# SPIKE-033 scan benchmark: results

Question: how long does a brute-force top-50 cosine scan take over 100,000 and 250,000 vectors, and what do the vectors cost in memory and storage?

Run on 2026-10-19. The vectors are random unit vectors (seeded), as the ticket allows. Scan time doesn't depend on the text.

## Machine

- CPU: Intel Core i5-1235U (2 performance + 8 efficiency cores, 12 threads)
- RAM: 15.7 GB DDR4-3200
- OS: Windows 11 Pro 10.0.26200
- Go 1.26.0, `modernc.org/sqlite` v1.59.0, `GOMAXPROCS=12`

**The timings are noisy.** This is a laptop. Four other Claude sessions were building and running things at the same time. The two runs below differ by up to 3x for the same case (768 wide, one core: p50 41 ms in run 1, 132 ms in run 2). Read the numbers as an order of magnitude. Run 2 is listed so the spread is visible.

## What is measured

- **Load:** one `SELECT` over the first N rows of a SQLite table (`id`, `v` BLOB; int8 also has a `scale` column). Each BLOB is copied into one flat slice. The file was just written, so it is mostly in the OS file cache. A cold start from disk will be slower. The load time is the whole `SELECT` plus the copy.
- **Scan:** from data already in memory. One query is one pass over all N vectors, keeping the top 50 in a heap. 200 queries, after one warm-up. p50 and p95 per query.
  - float32: dot product of float32 vectors (the vectors are unit length, so dot is cosine).
  - int8: the query is quantized too (symmetric, per vector), then an int32 integer dot product, then rescaled by both scales.
  - "1 core": one goroutine. "All cores": 12 goroutines, each on a slice of the rows, top-50 heaps merged at the end.
- **Memory per 100k:** live heap after the load, divided by the vector count, scaled to 100,000. It is the vectors plus the scale slice. It does not include SQLite's own cache.
- The scan is plain Go. No SIMD, no assembly. The dot product is unrolled by four.

## Scan and load, run 1 (all widths)

| width | form | vectors | load (s) | memory per 100k (MB) | 1 core p50 / p95 (ms) | all cores p50 / p95 (ms) |
|---|---|---|---|---|---|---|
| 3072 | float32 | 100000 | 18.04 | 1228.8 | 127.51 / 142.43 | 45.90 / 101.74 |
| 3072 | int8 | 100000 | 3.01 | 307.6 | 169.18 / 369.95 | 69.57 / 123.64 |
| 3072 | float32 | 250000 | 50.37 | 1228.8 | 330.79 / 474.99 | 109.18 / 144.38 |
| 3072 | int8 | 250000 | 6.74 | 307.6 | 441.30 / 1009.19 | 98.86 / 133.08 |
| 768 | float32 | 100000 | 1.93 | 307.2 | 41.51 / 77.91 | 14.58 / 25.69 |
| 768 | int8 | 100000 | 0.32 | 77.2 | 43.80 / 71.21 | 13.56 / 17.81 |
| 768 | float32 | 250000 | 1.94 | 307.2 | 97.88 / 142.92 | 38.12 / 50.32 |
| 768 | int8 | 250000 | 1.41 | 77.2 | 122.42 / 256.74 | 42.05 / 58.34 |
| 384 | float32 | 100000 | 0.34 | 153.6 | 21.60 / 31.70 | 6.00 / 8.32 |
| 384 | int8 | 100000 | 0.20 | 38.8 | 20.97 / 31.87 | 6.52 / 8.40 |
| 384 | float32 | 250000 | 1.01 | 153.6 | 51.35 / 91.69 | 22.63 / 64.02 |
| 384 | int8 | 250000 | 1.76 | 38.8 | 117.35 / 164.84 | 26.38 / 39.66 |
| 256 | float32 | 100000 | 0.72 | 102.4 | 28.80 / 42.03 | 6.62 / 8.99 |
| 256 | int8 | 100000 | 0.27 | 26.0 | 47.19 / 66.68 | 14.54 / 26.04 |
| 256 | float32 | 250000 | 4.68 | 102.4 | 76.62 / 104.10 | 22.93 / 39.03 |
| 256 | int8 | 250000 | 1.12 | 26.0 | 87.68 / 190.81 | 15.78 / 24.46 |

## Run 2 (768 and 384 only, same machine, a few minutes later)

| width | form | vectors | load (s) | memory per 100k (MB) | 1 core p50 / p95 (ms) | all cores p50 / p95 (ms) |
|---|---|---|---|---|---|---|
| 768 | float32 | 100000 | 2.49 | 307.2 | 131.64 / 226.66 | 20.95 / 26.39 |
| 768 | int8 | 100000 | 0.61 | 77.2 | 117.23 / 158.95 | 18.15 / 23.38 |
| 768 | float32 | 250000 | 3.73 | 307.2 | 126.61 / 208.56 | 45.16 / 62.36 |
| 768 | int8 | 250000 | 1.29 | 77.2 | 131.09 / 322.23 | 43.01 / 70.85 |
| 384 | float32 | 100000 | 0.32 | 153.6 | 20.01 / 27.31 | 12.25 / 20.17 |
| 384 | int8 | 100000 | 0.41 | 38.8 | 53.11 / 90.27 | 8.70 / 11.86 |
| 384 | float32 | 250000 | 1.28 | 153.6 | 54.03 / 105.92 | 22.95 / 29.41 |
| 384 | int8 | 250000 | 0.73 | 38.8 | 70.87 / 127.06 | 27.80 / 39.49 |

## Storage per vector

Payload is what the BLOB holds. int8 has a 4-byte float32 scale next to the bytes. The file figure is the SQLite file size divided by the row count, so it includes page overhead (rowid, page headers, overflow pages for BLOBs over about 4 KB).

| width | float32 payload (bytes) | int8 payload (bytes) | float32 in SQLite file | int8 in SQLite file |
|---|---|---|---|---|
| 3072 | 12288 | 3076 | 12801 | 4106 |
| 768 | 3072 | 772 | 4106 | 821 |
| 384 | 1536 | 388 | 2053 | 411 |
| 256 | 1024 | 260 | 1369 | 293 |

The 768 float32 row is 33% over its payload. A 3,072-byte BLOB plus the row header just misses two rows per 4 KB page, so every row takes a page. Going to int8, or to a width where two rows fit a page, avoids it.

## What the numbers say

- **All cores, 250,000 vectors:** about 25 ms at 384 and 256, about 40 ms at 768, about 100 ms at 3072. All of these are fine for a query a person types.
- **One core, 250,000 vectors:** about 50-130 ms at 384 and 768, 330-440 ms at 3072. A single-goroutine scan is still usable up to 768 wide. Only 3072 at 250,000 is slow.
- **100,000 vectors** take roughly 40% of the 250,000 times, as expected for a linear scan.
- **int8 is not faster to scan in plain Go.** The scalar integer loop is as slow as the float loop, or slower, even though it reads a quarter of the memory. What int8 gives is memory (4x less) and storage (about 4x less) and a faster load. A SIMD dot product would change the scan time. This benchmark doesn't test that.
- **Memory held:** float32 at 768 wide is 307 MB per 100k, so 770 MB at 250,000. At 3072 it is 1.2 GB per 100k, so 3.1 GB at 250,000, which is too much to keep in memory next to the app. int8 at 768 is 77 MB per 100k. At 384 it is 39 MB per 100k (float32: 154 MB).
- **Load is the costly step at 3072 wide.** 50 s for 250,000 float32 vectors, 7 s as int8. At 768 and below it is 0.3-4 s. The scan can run on vectors kept in memory, but a process that loads them at start-up would wait this long. Load time also depends on the OS file cache, so treat it as a lower bound for a cold disk.
- The scan numbers are the cost of the scan only. Embedding the query, FTS5 and the fusion step are not included.

## Rerun

Needs about 6 GB of free disk for the temporary SQLite files and about 4 GB of RAM at 3072 wide. The full run took about 20 minutes.

```bash
cd spikes/033-semantic-recall/scan
go test ./...
go run . -dir /path/with/6GB/free -queries 200 > out.txt
```

Flags: `-widths 3072,768,384,256`, `-sizes 100000,250000`, `-queries 200`, `-seed 33`. The temporary files are deleted after each width. Close other programs first. The timings move a lot when the machine is busy.
