# SPIKE-014 — Cron parsing
**Type:** Spike
**Status:** Done
**Gate:** 2

## Question
Which library parses 5-field cron expressions and computes the next run in an IANA time zone correctly (SPEC 6.2, 10)?

1. `Next` is right across DST changes: skipped hours, repeated hours, and zones with and without DST (Europe/Berlin, America/New_York, Asia/Tehran)
2. Day-of-month together with day-of-week (classic cron: either matches)
3. Clear errors for invalid expressions
4. Only the parser and `Next` are needed; the scheduler is ours

Candidates: `netresearch/go-cron`, `adhocore/gronx`, `robfig/cron/v3`, and others found.

## Done when
Each candidate passes or fails a table of expected next runs, and one is chosen.

## Result
Run on 2026-09-28 (Go 1.26.0, tz data 2025c). Code and full output: `spikes/014-cron-parsing/` (`results.md`).

There are 67 cases with expected next runs. A brute-force reference that steps minute by minute agrees with all 67.

| Library | Version (date), license | Basic | Day-of-month + day-of-week | Calendar | DST | All | With Jenab's DST layer |
|---|---|---|---|---|---|---|---|
| `adhocore/gronx` | v1.20.5 (2026-09-27), MIT | 23/23 | 9/9 | 9/9 | 18/26 | 59/67 | **67/67** |
| `netresearch/go-cron`, `DowOrDom` | v0.16.1 (2026-09-22), MIT | 23/23 | 7/9 | 8/9 | 20/26 | 58/67 | 64/67 |
| `netresearch/go-cron`, default | same | 23/23 | 4/9 | 8/9 | 20/26 | 55/67 | 61/67 |
| `hashicorp/cronexpr` | v1.1.3 (2025-08-28), GPLv3 or Apache-2.0 | 23/23 | 7/9 | 8/9 | 20/26 | 58/67 | 64/67 |
| `robfig/cron/v3` | v3.0.1 (2020-01-04), MIT | 22/23 | 7/9 | 8/9 | 20/26 | 57/67 | 63/67 |

**Findings**
- **go-cron's default day matching is AND:** "13th and Friday" means only Friday the 13th. That silently changes the meaning of standard crontabs.
- **robfig** rejects `7` as Sunday.
- **Every library except gronx:**
  - returns no time for 29 Feb from 2096 to 2104 (an 8-year gap)
  - gets cronie's rule wrong that a day field starting with `*` (e.g. `*/2`) makes matching AND
- **No library handles DST well.**
  - robfig and cronexpr follow the wall clock: a skipped 02:30 moves to the next day, and a repeated 02:30 runs twice.
  - go-cron runs a skipped 02:30 at 03:30, not 03:00.
  - gronx is inconsistent.
  - Tehran and UTC are correct everywhere.
- **Jenab's DST layer** (`dst.go`) fixes DST for every library (26/26). The library computes only on the plain wall clock in UTC, and Jenab maps the result back to the zone.
- **Invalid expressions:**
  - Nothing hangs; the slowest is 79 ms (cronexpr on `5-1`).
  - Never-matching expressions (`* * 31 2 *`) return a zero time, or an error in gronx after 2.4 ms.
  - Every library accepts some syntax Jenab should reject (6 fields, `L`, `W`, `#`, `?`, `@every`, `TZ=` prefixes, backwards ranges). A strict 5-field check of our own gets all 33 cases right; the libraries get 22–27.
- **Time zones in a built exe:**
  - Without `time/tzdata`, a build cannot load Berlin, New York or Tehran on a machine without Go installed; only UTC works.
  - `time/tzdata` fixes this and adds 401 KB.
- **Speed:** gronx with the layer takes about 7–16 µs per next run; robfig and go-cron take 0.5–1.5 µs. That doesn't matter for a check once a minute.
- **Not tested:**
  - cronie itself (its DST rule is taken from its documentation)
  - the Windows local zone as the default `timezone`
  - 30-minute DST shifts (Lord Howe)
  - Samoa's skipped day

## Decision
**Decided (2026-09-28):** `adhocore/gronx` v1.20.5 for matching and next run, wrapped in three Jenab rules:
1. **Strict 5-field check before the library.**
   - Accept:
     - numbers, ranges, lists, and steps on `*` or a range
     - three-letter month and day names
     - `7` as Sunday
     - `@yearly`, `@annually`, `@monthly`, `@weekly`, `@daily`, `@midnight`, `@hourly`
   - Reject:
     - 6 or 7 fields
     - `L`, `W`, `#`, `?`, `H`
     - backwards ranges, step 0, `5/15`, empty list items, full day names
     - `TZ=` and `CRON_TZ=` prefixes, `@reboot`, `@every`
     - expressions with no run in the next 9 years
2. **DST rule (as in cronie):**
   - A job at a fixed time (no `*` in minute or hour) whose time is skipped runs once, at the first minute after the gap.
   - A fixed-time job whose time repeats runs only the first time.
   - Jobs with `*` in minute or hour follow the real clock: they run again in a repeated hour, and nothing is caught up in a skipped one.
3. **Import `time/tzdata`** into the app.

Fallback: go-cron v0.16.1 with `DowOrDom`, behind the same check and layer.
