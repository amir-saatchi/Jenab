# SPIKE-014 results

Go go1.26.0, windows/amd64, 2026-09-28. Go's tz database: tzdata 2025c.

Every library gets `t.In(zone)` and returns the first run strictly after `t`. Times are shown in the case's zone.
"ref" is the brute-force reference in `ref.go` (steps minute by minute in the zone).

## 1. Expected next runs

`want` uses Vixie/cronie day matching and, for DST cases, the cronie rule: a fixed-time job (minute and hour without `*`) whose time is skipped runs once at the first minute after the gap, and a repeated time runs only at its first occurrence; a job with `*` in minute or hour follows the real clock. "ok" means the library returned exactly `want`.

| # | Group | Expression | Zone | From | Want | ref | go-cron | go-cron OR | gronx | robfig | cronexpr | Note |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 1 | basic | `* * * * *` | UTC | 2026-09-28 10:00:30 Z | 2026-09-28 10:01 Z | ok | ok | ok | ok | ok | ok | from :30 seconds |
| 2 | basic | `0 * * * *` | UTC | 2026-09-28 10:00 Z | 2026-09-28 11:00 Z | ok | ok | ok | ok | ok | ok | strictly after |
| 3 | basic | `5 4 * * *` | UTC | 2026-09-28 10:00 Z | 2026-09-29 04:05 Z | ok | ok | ok | ok | ok | ok |  |
| 4 | basic | `*/15 * * * *` | UTC | 2026-09-28 10:07 Z | 2026-09-28 10:15 Z | ok | ok | ok | ok | ok | ok |  |
| 5 | basic | `0-10/5 9 * * *` | UTC | 2026-09-28 09:06 Z | 2026-09-28 09:10 Z | ok | ok | ok | ok | ok | ok |  |
| 6 | basic | `0 9-17/4 * * *` | UTC | 2026-09-28 13:00 Z | 2026-09-28 17:00 Z | ok | ok | ok | ok | ok | ok |  |
| 7 | basic | `15,45 * * * *` | UTC | 2026-09-28 10:20 Z | 2026-09-28 10:45 Z | ok | ok | ok | ok | ok | ok |  |
| 8 | basic | `1-3,58-59 23 * * *` | UTC | 2026-09-28 23:03 Z | 2026-09-28 23:58 Z | ok | ok | ok | ok | ok | ok |  |
| 9 | basic | `0 0 1 * *` | UTC | 2026-09-28 10:00 Z | 2026-10-01 00:00 Z | ok | ok | ok | ok | ok | ok |  |
| 10 | basic | `0 0 1 */3 *` | UTC | 2026-09-28 10:00 Z | 2026-10-01 00:00 Z | ok | ok | ok | ok | ok | ok | months 1,4,7,10 |
| 11 | basic | `0 12 * JAN,JUL *` | UTC | 2026-09-28 10:00 Z | 2027-01-01 12:00 Z | ok | ok | ok | ok | ok | ok |  |
| 12 | basic | `0 12 * * MON-FRI` | UTC | 2026-10-02 12:00 Z | 2026-10-05 12:00 Z | ok | ok | ok | ok | ok | ok | Friday to Monday |
| 13 | basic | `0 8 * * sun` | UTC | 2026-09-28 10:00 Z | 2026-10-04 08:00 Z | ok | ok | ok | ok | ok | ok | lower-case name |
| 14 | basic | `0 8 * * 7` | UTC | 2026-09-28 10:00 Z | 2026-10-04 08:00 Z | ok | ok | ok | ok | **parse error: end of range (7) above maximum (6): 7** | ok | 7 = Sunday |
| 15 | basic | `0 0 * * 1-5/2` | UTC | 2026-09-28 00:00 Z | 2026-09-30 00:00 Z | ok | ok | ok | ok | ok | ok | Mon, Wed, Fri |
| 16 | basic | `@daily` | UTC | 2026-09-28 10:00 Z | 2026-09-29 00:00 Z | ok | ok | ok | ok | ok | ok |  |
| 17 | basic | `@hourly` | UTC | 2026-09-28 10:00 Z | 2026-09-28 11:00 Z | ok | ok | ok | ok | ok | ok |  |
| 18 | basic | `@weekly` | UTC | 2026-09-28 10:00 Z | 2026-10-04 00:00 Z | ok | ok | ok | ok | ok | ok |  |
| 19 | basic | `@monthly` | UTC | 2026-09-28 10:00 Z | 2026-10-01 00:00 Z | ok | ok | ok | ok | ok | ok |  |
| 20 | basic | `@yearly` | UTC | 2026-09-28 10:00 Z | 2027-01-01 00:00 Z | ok | ok | ok | ok | ok | ok |  |
| 21 | basic | `30 9 * * *` | Europe/Berlin | 2026-09-28 12:00 +02:00 | 2026-09-29 09:30 +02:00 | ok | ok | ok | ok | ok | ok |  |
| 22 | basic | `0 0 * * *` | Asia/Tehran | 2026-09-28 13:30 +03:30 | 2026-09-29 00:00 +03:30 | ok | ok | ok | ok | ok | ok | +03:30 offset |
| 23 | basic | `0 9 * * 1-5` | America/New_York | 2026-09-26 10:00 -04:00 | 2026-09-28 09:00 -04:00 | ok | ok | ok | ok | ok | ok | Saturday to Monday |
| 24 | dom+dow | `0 0 13 * 5` | UTC | 2026-09-28 00:00 Z | 2026-10-02 00:00 Z | ok | **2026-11-13 00:00 Z** | ok | ok | ok | ok | 13th OR Friday |
| 25 | dom+dow | `0 0 1 * MON` | UTC | 2026-09-28 00:00 Z | 2026-10-01 00:00 Z | ok | **2027-02-01 00:00 Z** | ok | ok | ok | ok | 1st OR Monday |
| 26 | dom+dow | `0 0 1-7 * MON` | UTC | 2026-09-28 00:00 Z | 2026-10-01 00:00 Z | ok | **2026-10-05 00:00 Z** | ok | ok | ok | ok | not 'first Monday' in Vixie cron |
| 27 | dom+dow | `0 0 1,15 * 3` | UTC | 2026-09-28 00:00 Z | 2026-09-30 00:00 Z | ok | **2027-09-01 00:00 Z** | ok | ok | ok | ok |  |
| 28 | dom+dow | `0 0 * * 5` | UTC | 2026-09-28 00:00 Z | 2026-10-02 00:00 Z | ok | ok | ok | ok | ok | ok | DOW only |
| 29 | dom+dow | `0 0 13 * *` | UTC | 2026-09-28 00:00 Z | 2026-10-13 00:00 Z | ok | ok | ok | ok | ok | ok | DOM only |
| 30 | dom+dow | `0 0 29 2 1` | UTC | 2026-09-28 00:00 Z | 2027-02-01 00:00 Z | ok | **zero time** | ok | ok | ok | ok | February only: 29th OR Monday |
| 31 | dom+dow | `0 0 */2 * 1` | UTC | 2026-09-28 00:00 Z | 2026-10-05 00:00 Z | ok | ok | **2026-09-29 00:00 Z** | ok | **2026-09-29 00:00 Z** | **2026-09-29 00:00 Z** | cronie quirk: DOM starts with * -> AND |
| 32 | dom+dow | `0 0 13 * */2` | UTC | 2026-09-28 00:00 Z | 2026-10-13 00:00 Z | ok | ok | **2026-09-29 00:00 Z** | ok | **2026-09-29 00:00 Z** | **2026-09-29 00:00 Z** | cronie quirk: DOW starts with * -> AND |
| 33 | calendar | `0 0 29 2 *` | UTC | 2026-09-28 00:00 Z | 2028-02-29 00:00 Z | ok | ok | ok | ok | ok | ok |  |
| 34 | calendar | `0 0 29 2 *` | UTC | 2028-02-29 00:00 Z | 2032-02-29 00:00 Z | ok | ok | ok | ok | ok | ok |  |
| 35 | calendar | `0 0 29 2 *` | UTC | 2096-03-01 00:00 Z | 2104-02-29 00:00 Z | ok | **zero time** | **zero time** | ok | **zero time** | **zero time** | 2100 is not a leap year: 8-year gap |
| 36 | calendar | `0 0 28-31 2 *` | UTC | 2027-02-28 00:00 Z | 2028-02-28 00:00 Z | ok | ok | ok | ok | ok | ok |  |
| 37 | calendar | `0 0 31 * *` | UTC | 2026-09-28 00:00 Z | 2026-10-31 00:00 Z | ok | ok | ok | ok | ok | ok |  |
| 38 | calendar | `0 0 31 * *` | UTC | 2026-10-31 00:00 Z | 2026-12-31 00:00 Z | ok | ok | ok | ok | ok | ok | skips November |
| 39 | calendar | `0 0 30 * *` | UTC | 2027-01-30 00:00 Z | 2027-03-30 00:00 Z | ok | ok | ok | ok | ok | ok | skips February |
| 40 | calendar | `59 23 31 12 *` | UTC | 2026-12-31 23:59 Z | 2027-12-31 23:59 Z | ok | ok | ok | ok | ok | ok |  |
| 41 | calendar | `0 0 1 1 *` | UTC | 2026-12-31 23:59 Z | 2027-01-01 00:00 Z | ok | ok | ok | ok | ok | ok | year rollover |
| 42 | dst | `30 2 * * *` | Europe/Berlin | 2026-03-28 12:00 +01:00 | 2026-03-29 03:00 +02:00 | ok | **2026-03-29 03:30 +02:00** | **2026-03-29 03:30 +02:00** | **2026-03-30 02:30 +02:00** | **2026-03-30 02:30 +02:00** | **2026-03-30 02:30 +02:00** | 02:30 skipped |
| 43 | dst | `30 2 * * *` | Europe/Berlin | 2026-03-29 03:00 +02:00 | 2026-03-30 02:30 +02:00 | ok | ok | ok | ok | ok | ok | after the gap run |
| 44 | dst | `0 2 * * *` | Europe/Berlin | 2026-03-28 12:00 +01:00 | 2026-03-29 03:00 +02:00 | ok | ok | ok | **2026-03-30 02:00 +02:00** | **2026-03-30 02:00 +02:00** | **2026-03-30 02:00 +02:00** | 02:00 skipped |
| 45 | dst | `*/15 * * * *` | Europe/Berlin | 2026-03-29 01:45 +01:00 | 2026-03-29 03:00 +02:00 | ok | ok | ok | ok | ok | ok |  |
| 46 | dst | `0 * * * *` | Europe/Berlin | 2026-03-29 01:00 +01:00 | 2026-03-29 03:00 +02:00 | ok | ok | ok | ok | ok | ok |  |
| 47 | dst | `*/15 2 * * *` | Europe/Berlin | 2026-03-29 01:00 +01:00 | 2026-03-30 02:00 +02:00 | ok | **2026-03-29 03:00 +02:00** | **2026-03-29 03:00 +02:00** | ok | ok | ok | * job, whole hour skipped: no catch-up |
| 48 | dst | `0,30 2 * * *` | Europe/Berlin | 2026-03-29 03:00 +02:00 | 2026-03-30 02:00 +02:00 | ok | ok | ok | ok | ok | ok | two times in one gap run once |
| 49 | dst | `0 12 * * *` | Europe/Berlin | 2026-03-29 00:00 +01:00 | 2026-03-29 12:00 +02:00 | ok | ok | ok | ok | ok | ok | later on change day |
| 50 | dst | `0 9 * * 1` | Europe/Berlin | 2026-03-27 12:00 +01:00 | 2026-03-30 09:00 +02:00 | ok | ok | ok | ok | ok | ok | weekly across change |
| 51 | dst | `30 2 * * *` | Europe/Berlin | 2026-10-24 12:00 +02:00 | 2026-10-25 02:30 +02:00 | ok | ok | ok | **2026-10-25 02:30 +01:00** | ok | ok | first 02:30 |
| 52 | dst | `30 2 * * *` | Europe/Berlin | 2026-10-25 02:30 +02:00 | 2026-10-26 02:30 +01:00 | ok | **2026-10-25 02:30 +01:00** | **2026-10-25 02:30 +01:00** | ok | **2026-10-25 02:30 +01:00** | **2026-10-25 02:30 +01:00** | second 02:30 must not run |
| 53 | dst | `*/15 * * * *` | Europe/Berlin | 2026-10-25 02:45 +02:00 | 2026-10-25 02:00 +01:00 | ok | ok | ok | **2026-10-25 03:00 +01:00** | ok | ok | repeated hour, * job runs again |
| 54 | dst | `0 * * * *` | Europe/Berlin | 2026-10-25 02:00 +02:00 | 2026-10-25 02:00 +01:00 | ok | ok | ok | **2026-10-25 03:00 +01:00** | ok | ok | repeated hour, * job runs again |
| 55 | dst | `30 2 * * *` | America/New_York | 2026-03-07 12:00 -05:00 | 2026-03-08 03:00 -04:00 | ok | **2026-03-08 03:30 -04:00** | **2026-03-08 03:30 -04:00** | **2026-03-09 02:30 -04:00** | **2026-03-09 02:30 -04:00** | **2026-03-09 02:30 -04:00** | 02:30 skipped |
| 56 | dst | `*/15 * * * *` | America/New_York | 2026-03-08 01:45 -05:00 | 2026-03-08 03:00 -04:00 | ok | ok | ok | ok | ok | ok |  |
| 57 | dst | `0 * * * *` | America/New_York | 2026-03-08 01:00 -05:00 | 2026-03-08 03:00 -04:00 | ok | ok | ok | ok | ok | ok |  |
| 58 | dst | `30 1 * * *` | America/New_York | 2026-10-31 12:00 -04:00 | 2026-11-01 01:30 -04:00 | ok | ok | ok | ok | ok | ok | first 01:30 |
| 59 | dst | `30 1 * * *` | America/New_York | 2026-11-01 01:30 -04:00 | 2026-11-02 01:30 -05:00 | ok | **2026-11-01 01:30 -05:00** | **2026-11-01 01:30 -05:00** | ok | **2026-11-01 01:30 -05:00** | **2026-11-01 01:30 -05:00** | second 01:30 must not run |
| 60 | dst | `*/15 * * * *` | America/New_York | 2026-11-01 01:45 -04:00 | 2026-11-01 01:00 -05:00 | ok | ok | ok | ok | ok | ok | repeated hour, * job runs again |
| 61 | dst | `0 * * * *` | America/New_York | 2026-11-01 01:00 -04:00 | 2026-11-01 01:00 -05:00 | ok | ok | ok | **2026-11-01 02:00 -05:00** | ok | ok | repeated hour, * job runs again |
| 62 | dst | `0 2 * * *` | America/New_York | 2026-11-01 00:00 -04:00 | 2026-11-01 02:00 -05:00 | ok | ok | ok | ok | ok | ok | 02:00 exists once |
| 63 | dst | `0 0 * * *` | America/Santiago | 2026-09-05 12:00 -04:00 | 2026-09-06 01:00 -03:00 | ok | **2026-09-07 00:00 -03:00** | **2026-09-07 00:00 -03:00** | **2026-09-07 00:00 -03:00** | **2026-09-07 00:00 -03:00** | **2026-09-07 00:00 -03:00** | midnight skipped |
| 64 | dst | `30 2 * * *` | Asia/Tehran | 2026-03-20 12:00 +03:30 | 2026-03-21 02:30 +03:30 | ok | ok | ok | ok | ok | ok | no DST since 2022 |
| 65 | dst | `0 0 * * *` | Asia/Tehran | 2026-03-21 12:00 +03:30 | 2026-03-22 00:00 +03:30 | ok | ok | ok | ok | ok | ok | old DST start day |
| 66 | dst | `30 2 * * *` | UTC | 2026-03-29 00:00 Z | 2026-03-29 02:30 Z | ok | ok | ok | ok | ok | ok |  |
| 67 | dst | `30 1 * * *` | UTC | 2026-11-01 00:00 Z | 2026-11-01 01:30 Z | ok | ok | ok | ok | ok | ok |  |

Reference disagrees with the hand-written `want` in 0 of 67 cases.

### Summary (passed / total)

| Library | basic | dom+dow | calendar | dst | all | Failures (with DST layer) |
|---|---|---|---|---|---|---|
| netresearch/go-cron v0.16.1 (ParseStandard, default AND) | 23/23 | 4/9 | 8/9 | 20/26 | 55/67 | see table above |
| netresearch/go-cron v0.16.1 (NewParser with DowOrDom) | 23/23 | 7/9 | 8/9 | 20/26 | 58/67 | see table above |
| adhocore/gronx v1.20.5 (IsValid + NextTickAfter) | 23/23 | 9/9 | 9/9 | 18/26 | 59/67 | see table above |
| robfig/cron/v3 v3.0.1 (ParseStandard) | 22/23 | 7/9 | 8/9 | 20/26 | 57/67 | see table above |
| hashicorp/cronexpr v1.1.3 (Parse) | 23/23 | 7/9 | 8/9 | 20/26 | 58/67 | see table above |
| netresearch/go-cron v0.16.1 (ParseStandard, default AND) + DST layer | 23/23 | 4/9 | 8/9 | 26/26 | 61/67 | #24 2026-11-13 00:00 Z; #25 2027-02-01 00:00 Z; #26 2026-10-05 00:00 Z; #27 2027-09-01 00:00 Z; #30 zero time; #35 zero time |
| netresearch/go-cron v0.16.1 (NewParser with DowOrDom) + DST layer | 23/23 | 7/9 | 8/9 | 26/26 | 64/67 | #31 2026-09-29 00:00 Z; #32 2026-09-29 00:00 Z; #35 zero time |
| adhocore/gronx v1.20.5 (IsValid + NextTickAfter) + DST layer | 23/23 | 9/9 | 9/9 | 26/26 | 67/67 |  |
| robfig/cron/v3 v3.0.1 (ParseStandard) + DST layer | 22/23 | 7/9 | 8/9 | 26/26 | 63/67 | #14 parse error: end of range (7) above maximum (6): 7; #31 2026-09-29 00:00 Z; #32 2026-09-29 00:00 Z; #35 zero time |
| hashicorp/cronexpr v1.1.3 (Parse) + DST layer | 23/23 | 7/9 | 8/9 | 26/26 | 64/67 | #31 2026-09-29 00:00 Z; #32 2026-09-29 00:00 Z; #35 zero time |

"+ DST layer" rows run the same cases through `dst.go`: the library computes only in UTC on the naive wall clock, and Burrow maps the result back to the zone with the cronie rule.

## 2. DST behaviour

Reference in three modes: **wall** = match the wall clock literally (a skipped time never runs, a repeated time runs twice); **once** = rule A; **cronie** = rule A for fixed-time jobs, wall for jobs whose minute or hour field starts with `*` (what cronie does). Each library cell lists the modes its answer agrees with.

| # | Expression | Zone | From | wall | once | cronie | go-cron | go-cron OR | gronx | robfig | cronexpr |
|---|---|---|---|---|---|---|---|---|---|---|---|
| 42 | `30 2 * * *` | Europe/Berlin | 2026-03-28 12:00 +01:00 | 2026-03-30 02:30 +02:00 | 2026-03-29 03:00 +02:00 | 2026-03-29 03:00 +02:00 | **2026-03-29 03:30 +02:00** | **2026-03-29 03:30 +02:00** | wall | wall | wall |
| 43 | `30 2 * * *` | Europe/Berlin | 2026-03-29 03:00 +02:00 | 2026-03-30 02:30 +02:00 | 2026-03-30 02:30 +02:00 | 2026-03-30 02:30 +02:00 | all | all | all | all | all |
| 44 | `0 2 * * *` | Europe/Berlin | 2026-03-28 12:00 +01:00 | 2026-03-30 02:00 +02:00 | 2026-03-29 03:00 +02:00 | 2026-03-29 03:00 +02:00 | once, cronie | once, cronie | wall | wall | wall |
| 45 | `*/15 * * * *` | Europe/Berlin | 2026-03-29 01:45 +01:00 | 2026-03-29 03:00 +02:00 | 2026-03-29 03:00 +02:00 | 2026-03-29 03:00 +02:00 | all | all | all | all | all |
| 46 | `0 * * * *` | Europe/Berlin | 2026-03-29 01:00 +01:00 | 2026-03-29 03:00 +02:00 | 2026-03-29 03:00 +02:00 | 2026-03-29 03:00 +02:00 | all | all | all | all | all |
| 47 | `*/15 2 * * *` | Europe/Berlin | 2026-03-29 01:00 +01:00 | 2026-03-30 02:00 +02:00 | 2026-03-29 03:00 +02:00 | 2026-03-30 02:00 +02:00 | once | once | wall, cronie | wall, cronie | wall, cronie |
| 48 | `0,30 2 * * *` | Europe/Berlin | 2026-03-29 03:00 +02:00 | 2026-03-30 02:00 +02:00 | 2026-03-30 02:00 +02:00 | 2026-03-30 02:00 +02:00 | all | all | all | all | all |
| 49 | `0 12 * * *` | Europe/Berlin | 2026-03-29 00:00 +01:00 | 2026-03-29 12:00 +02:00 | 2026-03-29 12:00 +02:00 | 2026-03-29 12:00 +02:00 | all | all | all | all | all |
| 50 | `0 9 * * 1` | Europe/Berlin | 2026-03-27 12:00 +01:00 | 2026-03-30 09:00 +02:00 | 2026-03-30 09:00 +02:00 | 2026-03-30 09:00 +02:00 | all | all | all | all | all |
| 51 | `30 2 * * *` | Europe/Berlin | 2026-10-24 12:00 +02:00 | 2026-10-25 02:30 +02:00 | 2026-10-25 02:30 +02:00 | 2026-10-25 02:30 +02:00 | all | all | **2026-10-25 02:30 +01:00** | all | all |
| 52 | `30 2 * * *` | Europe/Berlin | 2026-10-25 02:30 +02:00 | 2026-10-25 02:30 +01:00 | 2026-10-26 02:30 +01:00 | 2026-10-26 02:30 +01:00 | wall | wall | once, cronie | wall | wall |
| 53 | `*/15 * * * *` | Europe/Berlin | 2026-10-25 02:45 +02:00 | 2026-10-25 02:00 +01:00 | 2026-10-25 03:00 +01:00 | 2026-10-25 02:00 +01:00 | wall, cronie | wall, cronie | once | wall, cronie | wall, cronie |
| 54 | `0 * * * *` | Europe/Berlin | 2026-10-25 02:00 +02:00 | 2026-10-25 02:00 +01:00 | 2026-10-25 03:00 +01:00 | 2026-10-25 02:00 +01:00 | wall, cronie | wall, cronie | once | wall, cronie | wall, cronie |
| 55 | `30 2 * * *` | America/New_York | 2026-03-07 12:00 -05:00 | 2026-03-09 02:30 -04:00 | 2026-03-08 03:00 -04:00 | 2026-03-08 03:00 -04:00 | **2026-03-08 03:30 -04:00** | **2026-03-08 03:30 -04:00** | wall | wall | wall |
| 56 | `*/15 * * * *` | America/New_York | 2026-03-08 01:45 -05:00 | 2026-03-08 03:00 -04:00 | 2026-03-08 03:00 -04:00 | 2026-03-08 03:00 -04:00 | all | all | all | all | all |
| 57 | `0 * * * *` | America/New_York | 2026-03-08 01:00 -05:00 | 2026-03-08 03:00 -04:00 | 2026-03-08 03:00 -04:00 | 2026-03-08 03:00 -04:00 | all | all | all | all | all |
| 58 | `30 1 * * *` | America/New_York | 2026-10-31 12:00 -04:00 | 2026-11-01 01:30 -04:00 | 2026-11-01 01:30 -04:00 | 2026-11-01 01:30 -04:00 | all | all | all | all | all |
| 59 | `30 1 * * *` | America/New_York | 2026-11-01 01:30 -04:00 | 2026-11-01 01:30 -05:00 | 2026-11-02 01:30 -05:00 | 2026-11-02 01:30 -05:00 | wall | wall | once, cronie | wall | wall |
| 60 | `*/15 * * * *` | America/New_York | 2026-11-01 01:45 -04:00 | 2026-11-01 01:00 -05:00 | 2026-11-01 02:00 -05:00 | 2026-11-01 01:00 -05:00 | wall, cronie | wall, cronie | wall, cronie | wall, cronie | wall, cronie |
| 61 | `0 * * * *` | America/New_York | 2026-11-01 01:00 -04:00 | 2026-11-01 01:00 -05:00 | 2026-11-01 02:00 -05:00 | 2026-11-01 01:00 -05:00 | wall, cronie | wall, cronie | once | wall, cronie | wall, cronie |
| 62 | `0 2 * * *` | America/New_York | 2026-11-01 00:00 -04:00 | 2026-11-01 02:00 -05:00 | 2026-11-01 02:00 -05:00 | 2026-11-01 02:00 -05:00 | all | all | all | all | all |
| 63 | `0 0 * * *` | America/Santiago | 2026-09-05 12:00 -04:00 | 2026-09-07 00:00 -03:00 | 2026-09-06 01:00 -03:00 | 2026-09-06 01:00 -03:00 | wall | wall | wall | wall | wall |
| 64 | `30 2 * * *` | Asia/Tehran | 2026-03-20 12:00 +03:30 | 2026-03-21 02:30 +03:30 | 2026-03-21 02:30 +03:30 | 2026-03-21 02:30 +03:30 | all | all | all | all | all |
| 65 | `0 0 * * *` | Asia/Tehran | 2026-03-21 12:00 +03:30 | 2026-03-22 00:00 +03:30 | 2026-03-22 00:00 +03:30 | 2026-03-22 00:00 +03:30 | all | all | all | all | all |
| 66 | `30 2 * * *` | UTC | 2026-03-29 00:00 Z | 2026-03-29 02:30 Z | 2026-03-29 02:30 Z | 2026-03-29 02:30 Z | all | all | all | all | all |
| 67 | `30 1 * * *` | UTC | 2026-11-01 00:00 Z | 2026-11-01 01:30 Z | 2026-11-01 01:30 Z | 2026-11-01 01:30 Z | all | all | all | all | all |

| Library | agrees with wall | agrees with once | agrees with cronie |
|---|---|---|---|
| go-cron | 22/26 | 17/26 | 20/26 |
| go-cron OR | 22/26 | 17/26 | 20/26 |
| gronx | 20/26 | 19/26 | 18/26 |
| robfig | 26/26 | 15/26 | 20/26 |
| cronexpr | 26/26 | 15/26 | 20/26 |

## 3. Invalid, non-standard and never-matching expressions

Parse + Next from 2026-09-28 10:00 UTC, with a 3 s hang limit. "Burrow" is what the trigger check should do. "ref" is the strict parser in `ref.go` (a never-matching expression shows as "no run in 9 years"). Time is Parse + Next.

| Expression | Note | Burrow | ref | go-cron | go-cron OR | gronx | robfig | cronexpr |
|---|---|---|---|---|---|---|---|---|
| `60 * * * *` | minute out of range | reject | error: minute: 60 out of range 0-59 | parse error: end of range (60) above maximum (59): "60", 4.7 µs | parse error: end of range (60) above maximum (59): "60", 1.2 µs | parse error: IsValid returned false, 3.5 µs | parse error: end of range (60) above maximum (59): 60, 1.8 µs | parse error: syntax error in minute field: '60', 12.1 µs |
| `* 24 * * *` | hour out of range | reject | error: hour: 24 out of range 0-23 | parse error: end of range (24) above maximum (23): "24", 1.0 µs | parse error: end of range (24) above maximum (23): "24", 0.9 µs | parse error: IsValid returned false, 2.2 µs | parse error: end of range (24) above maximum (23): 24, 0.9 µs | parse error: syntax error in hour field: '24', 4.3 µs |
| `* * 0 * *` | day 0 | reject | error: day of month: 0 out of range 1-31 | parse error: beginning of range (0) below minimum (1): "0", 1.8 µs | parse error: beginning of range (0) below minimum (1): "0", 1.1 µs | parse error: IsValid returned false, 2.2 µs | parse error: beginning of range (0) below minimum (1): 0, 0.9 µs | parse error: syntax error in day-of-month field: '0', 172.2 µs |
| `* * 32 * *` | day 32 | reject | error: day of month: 32 out of range 1-31 | parse error: end of range (32) above maximum (31): "32", 4.9 µs | parse error: end of range (32) above maximum (31): "32", 2.0 µs | parse error: IsValid returned false, 99.8 µs | parse error: end of range (32) above maximum (31): 32, 1.8 µs | parse error: syntax error in day-of-month field: '32', 34.3 µs |
| `* * * 13 *` | month 13 | reject | error: month: 13 out of range 1-12 | parse error: end of range (13) above maximum (12): "13", 76.8 µs | parse error: end of range (13) above maximum (12): "13", 1.8 µs | parse error: IsValid returned false, 3.8 µs | parse error: end of range (13) above maximum (12): 13, 1.3 µs | parse error: syntax error in month field: '13', 111.2 µs |
| `* * * * 8` | weekday 8 | reject | error: day of week: 8 out of range 0-7 | parse error: end of range (8) above maximum (7): "8", 1.6 µs | parse error: end of range (8) above maximum (7): "8", 1.3 µs | parse error: IsValid returned false, 2.8 µs | parse error: end of range (8) above maximum (6): 8, 1.3 µs | parse error: syntax error in day-of-week field: '8', 46.3 µs |
| `* * 31 2 *` | never matches (31 Feb) | reject | no run in 9 years | zero time, 21.9 µs | zero time, 8.9 µs | Next error: tried so hard (2026-09-28 10:00 Z), 2.36 ms | zero time, 12.8 µs | zero time, 39.8 µs |
| `0 0 30 2 *` | never matches (30 Feb) | reject | no run in 9 years | zero time, 16.4 µs | zero time, 9.1 µs | Next error: tried so hard (2026-09-28 10:00 Z), 1.78 ms | zero time, 11.7 µs | zero time, 36.2 µs |
| `0 0 31 4,6,9,11 *` | never matches (31st of 30-day months) | reject | no run in 9 years | zero time, 36.3 µs | zero time, 27.3 µs | Next error: tried so hard (2026-09-28 10:00 Z), 1.73 ms | zero time, 33.5 µs | zero time, 77.3 µs |
| `* * * * * *` | 6 fields | reject | error: want 5 fields, got 6 | parse error: expected exactly 5 fields, found 6: [* * * * * *], 17.7 µs | parse error: expected exactly 5 fields, found 6: [* * * * * *], 5.0 µs | 2026-09-28 10:00:01 Z, 5.9 µs | parse error: expected exactly 5 fields, found 6: [* * * * * *], 2.5 µs | 2026-09-28 10:01 Z, 7.6 µs |
| `0 0 * * * 2027` | 6 fields with year | reject | error: want 5 fields, got 6 | parse error: expected exactly 5 fields, found 6: [0 0 * * * 2027], 3.9 µs | parse error: expected exactly 5 fields, found 6: [0 0 * * * 2027], 3.2 µs | 2027-01-01 00:00 Z, 7.7 µs | parse error: expected exactly 5 fields, found 6: [0 0 * * * 2027], 1.0 µs | 2027-01-01 00:00 Z, 48.2 µs |
| `* * * *` | 4 fields | reject | error: want 5 fields, got 4 | parse error: expected exactly 5 fields, found 4: [* * * *], 4.0 µs | parse error: expected exactly 5 fields, found 4: [* * * *], 3.4 µs | parse error: IsValid returned false, 1.3 µs | parse error: expected exactly 5 fields, found 4: [* * * *], 0.9 µs | parse error: missing field(s), 1.2 µs |
| (empty) | empty | reject | error: empty expression | parse error: empty spec string, 0.2 µs | parse error: empty spec string, 0.2 µs | parse error: IsValid returned false, 0.6 µs | parse error: empty spec string, 0.2 µs | parse error: missing field(s), 0.2 µs |
| `"   "` | only spaces | reject | error: empty expression | parse error: expected exactly 5 fields, found 0: [], 0.6 µs | parse error: expected exactly 5 fields, found 0: [], 0.4 µs | parse error: IsValid returned false, 0.4 µs | parse error: expected exactly 5 fields, found 0: [], 0.3 µs | parse error: missing field(s), 0.5 µs |
| `"  0  9   *  * *  "` | extra spaces | accept | 2026-09-29 09:00 Z | 2026-09-29 09:00 Z, 3.2 µs | 2026-09-29 09:00 Z, 1.9 µs | 2026-09-29 09:00 Z, 7.7 µs | 2026-09-29 09:00 Z, 1.8 µs | 2026-09-29 09:00 Z, 19.4 µs |
| `0\t9 * * *` | tab separator | accept | 2026-09-29 09:00 Z | 2026-09-29 09:00 Z, 1.8 µs | 2026-09-29 09:00 Z, 1.7 µs | 2026-09-29 09:00 Z, 7.1 µs | 2026-09-29 09:00 Z, 1.4 µs | 2026-09-29 09:00 Z, 5.7 µs |
| `0 0 L * *` | L (last day) | reject | error: day of month: bad value "L" | parse error: extended day-of-month syntax requires DomL option (for L, L-…, 0.8 µs | parse error: extended day-of-month syntax requires DomL option (for L, L-…, 0.7 µs | 2026-09-30 00:00 Z, 6.8 µs | parse error: failed to parse int from L: strconv.Atoi: parsing "L": inval…, 17.3 µs | 2026-09-30 00:00 Z, 10.8 µs |
| `0 0 15W * *` | W (nearest weekday) | reject | error: day of month: bad value "15W" | parse error: extended day-of-month syntax requires DomL option (for L, L-…, 1.0 µs | parse error: extended day-of-month syntax requires DomL option (for L, L-…, 0.9 µs | 2026-10-15 00:00 Z, 11.3 µs | parse error: failed to parse int from 15W: strconv.Atoi: parsing "15W": i…, 2.1 µs | 2026-10-15 00:00 Z, 9.7 µs |
| `0 0 * * 5#3` | # (third Friday) | reject | error: day of week: bad value "5#3" | parse error: #n/#L syntax requires DowNth or DowLast option to be enabled, 2.6 µs | parse error: #n/#L syntax requires DowNth or DowLast option to be enabled, 0.8 µs | 2026-10-16 00:00 Z, 11.0 µs | parse error: failed to parse int from 5#3: strconv.Atoi: parsing "5#3": i…, 1.6 µs | 2026-10-16 00:00 Z, 20.0 µs |
| `0 0 * * 5L` | L in weekday | reject | error: day of week: bad value "5L" | parse error: failed to parse int from "5L": strconv.Atoi: parsing "5L": i…, 23.2 µs | parse error: failed to parse int from "5L": strconv.Atoi: parsing "5L": i…, 4.8 µs | 2026-10-30 00:00 Z, 16.8 µs | parse error: failed to parse int from 5L: strconv.Atoi: parsing "5L": inv…, 1.6 µs | 2026-10-30 00:00 Z, 8.3 µs |
| `0 0 ? * MON` | ? (Quartz) | reject | error: day of month: bad value "?" | 2026-10-05 00:00 Z, 2.5 µs | 2026-10-05 00:00 Z, 4.7 µs | 2026-10-05 00:00 Z, 7.9 µs | 2026-10-05 00:00 Z, 1.9 µs | 2026-10-05 00:00 Z, 7.2 µs |
| `H * * * *` | Jenkins H | reject | error: minute: bad value "H" | parse error: h expressions require hash option to be enabled, 0.4 µs | parse error: h expressions require hash option to be enabled, 0.3 µs | parse error: IsValid returned false, 1.7 µs | parse error: failed to parse int from H: strconv.Atoi: parsing "H": inval…, 1.2 µs | parse error: syntax error in minute field: 'H', 2.8 µs |
| `0 22-2 * * *` | backwards range | reject | error: hour: range "22-2" runs backwards | 2026-09-28 22:00 Z, 1.8 µs | 2026-09-28 22:00 Z, 1.5 µs | parse error: IsValid returned false, 2.9 µs | parse error: beginning of range (22) beyond end of range (2): 22-2, 1.2 µs | zero time, 3.78 ms |
| `5-1 * * * *` | backwards range | reject | error: minute: range "5-1" runs backwards | 2026-09-28 10:01 Z, 23.7 µs | 2026-09-28 10:01 Z, 3.5 µs | parse error: IsValid returned false, 9.4 µs | parse error: beginning of range (5) beyond end of range (1): 5-1, 3.4 µs | zero time, 79.48 ms |
| `*/0 * * * *` | step 0 | reject | error: minute: bad step "0" | parse error: step of range must be a positive number: "*/0", 3.8 µs | parse error: step of range must be a positive number: "*/0", 0.9 µs | parse error: IsValid returned false, 13.7 µs | parse error: step of range should be a positive number: */0, 1.7 µs | parse error: invalid interval */0, 11.2 µs |
| `5/15 * * * *` | step without range | reject | error: minute: step needs a range or *: "5/15" | 2026-09-28 10:05 Z, 3.1 µs | 2026-09-28 10:05 Z, 1.5 µs | 2026-09-28 10:05 Z, 8.6 µs | 2026-09-28 10:05 Z, 2.5 µs | 2026-09-28 10:05 Z, 14.0 µs |
| `0 9 * * MON-FRI,` | trailing comma | reject | error: day of week: empty list item | 2026-09-29 09:00 Z, 2.6 µs | 2026-09-29 09:00 Z, 2.1 µs | parse error: IsValid returned false, 3.4 µs | 2026-09-29 09:00 Z, 2.1 µs | 2026-09-29 09:00 Z, 18.6 µs |
| `0 9 * * Monday` | full weekday name | reject | error: day of week: bad value "Monday" | parse error: failed to parse int from "Monday": strconv.Atoi: parsing "Mo…, 15.4 µs | parse error: failed to parse int from "Monday": strconv.Atoi: parsing "Mo…, 6.1 µs | parse error: IsValid returned false, 3.6 µs | parse error: failed to parse int from Monday: strconv.Atoi: parsing "Mond…, 1.9 µs | 2026-10-05 09:00 Z, 8.7 µs |
| `CRON_TZ=UTC 0 9 * * *` | zone prefix (zone is a separate field) | reject | error: want 5 fields, got 6 | 2026-09-29 09:00 Z, 2.3 µs | 2026-09-29 09:00 Z, 1.8 µs | parse error: IsValid returned false, 3.0 µs | 2026-09-29 09:00 Z, 1.6 µs | parse error: syntax error in minute field: 'CRON_TZ=UTC', 4.8 µs |
| `TZ=Asia/Tehran 0 9 * * *` | zone prefix | reject | error: want 5 fields, got 6 | 2026-09-29 05:30 Z, 132.1 µs | 2026-09-29 05:30 Z, 56.8 µs | parse error: IsValid returned false, 5.1 µs | 2026-09-29 05:30 Z, 39.6 µs | parse error: syntax error in minute field: 'TZ=Asia/Tehran', 7.2 µs |
| `@reboot` | macro without a time | reject | error: unknown macro "@reboot" | parse error: unrecognized descriptor: "@reboot", 0.7 µs | parse error: unrecognized descriptor: "@reboot", 0.4 µs | parse error: IsValid returned false, 1.4 µs | parse error: unrecognized descriptor: @reboot, 0.6 µs | parse error: missing field(s), 1.0 µs |
| `@every 5m` | robfig interval | reject | error: unknown macro "@every 5m" | 2026-09-28 10:05 Z, 1.0 µs | 2026-09-28 10:05 Z, 0.3 µs | parse error: IsValid returned false, 1.4 µs | 2026-09-28 10:05 Z, 0.5 µs | parse error: missing field(s), 1.0 µs |
| `@midnight` | cronie macro | accept | 2026-09-29 00:00 Z | 2026-09-29 00:00 Z, 0.8 µs | 2026-09-29 00:00 Z, 0.8 µs | parse error: IsValid returned false, 1.0 µs | 2026-09-29 00:00 Z, 0.6 µs | parse error: missing field(s), 0.8 µs |

"Agrees" = the library returns a usable next run exactly when Burrow should accept (a parse error, Next error, zero time, hang or panic all count as rejecting).

| Library | agrees with Burrow |
|---|---|
| ref | 33/33 |
| go-cron | 25/33 |
| go-cron OR | 25/33 |
| gronx | 24/33 |
| robfig | 27/33 |
| cronexpr | 22/33 |

## 4. Time zones in a built Windows exe

Probe loads 4 zones. Each exe runs with `GOROOT` and `ZONEINFO` removed from its environment (a user machine without Go). A plain build still has the build machine's GOROOT baked in (`runtime.GOROOT()`), so it finds `lib/time/zoneinfo.zip` here but would not on a user machine; the second row simulates that.

| Build | exe size | Europe/Berlin | America/New_York | Asia/Tehran | UTC |
|---|---|---|---|---|---|
| plain build | 2533 KB | ok +120 | ok -240 | ok +210 | ok +0 |
| plain build, GOROOT=C:\nonexistent | 2533 KB | error: unknown time zone Europe/Berlin | error: unknown time zone America/New_York | error: unknown time zone Asia/Tehran | ok +0 |
| -trimpath | 2527 KB | error: unknown time zone Europe/Berlin | error: unknown time zone America/New_York | error: unknown time zone Asia/Tehran | ok +0 |
| -trimpath + time/tzdata | 2928 KB | ok +120 | ok -240 | ok +210 | ok +0 |
| -trimpath -ldflags=-s -w | 1715 KB | error: unknown time zone Europe/Berlin | error: unknown time zone America/New_York | error: unknown time zone Asia/Tehran | ok +0 |
| -trimpath -ldflags=-s -w + time/tzdata | 2116 KB | ok +120 | ok -240 | ok +210 | ok +0 |

time/tzdata adds 401 KB to a -trimpath build and 401 KB to a stripped build.

## 5. Speed

Parse + Next, 3000 runs per cell after 200 warm-up runs, Europe/Berlin, from 2026-09-28 12:00. p50 (p99). Timed with the Windows performance counter.

| Library | `*/15 * * * *` | `30 2 * * 1-5` | `0 9 1,15 * MON` | `0 0 29 2 *` | Next only, `30 2 * * 1-5` |
|---|---|---|---|---|---|
| go-cron | 1.6 µs (4.0 µs) | 1.5 µs (1.9 µs) | 62.5 µs (131.9 µs) | 41.5 µs (129.8 µs) | 0.6 µs (0.8 µs) |
| go-cron OR | 1.5 µs (2.1 µs) | 1.5 µs (27.2 µs) | 1.5 µs (3.6 µs) | 40.6 µs (129.9 µs) | 0.8 µs (1.7 µs) |
| gronx | 12.0 µs (42.1 µs) | 8.6 µs (24.6 µs) | 9.2 µs (20.7 µs) | 72.0 µs (202.0 µs) | 6.3 µs (18.9 µs) |
| robfig | 1.4 µs (3.2 µs) | 1.2 µs (2.4 µs) | 1.2 µs (3.3 µs) | 39.4 µs (89.5 µs) | 0.5 µs (0.6 µs) |
| cronexpr | 5.0 µs (22.7 µs) | 8.9 µs (56.1 µs) | 5.3 µs (16.8 µs) | 7.3 µs (23.1 µs) | 3.1 µs (9.3 µs) |
| gronx+layer | 9.7 µs (58.8 µs) | 8.6 µs (17.5 µs) | 10.0 µs (32.5 µs) | 15.7 µs (32.9 µs) | 6.8 µs (12.2 µs) |

