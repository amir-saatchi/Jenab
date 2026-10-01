# SPIKE-014 — Cron parsing

Throwaway code for [SPIKE-014](../../Docs/Tickets/SPIKE-014-cron-parsing.md).

```bash
go run . > results.md    # about 20 seconds
```

- `cases.go`: 67 hand-written next-run cases (plain, ranges/steps/lists, names, macros, day-of-month + day-of-week, leap years, end of month, DST in Berlin, New York, Santiago, Tehran, UTC) and 33 invalid or non-standard expressions
- `ref.go`: a strict 5-field parser and a brute-force Next that steps minute by minute in the zone, with three DST modes (wall, once, cronie); it must agree with every hand-written `want`
- `libs.go`: adapters for netresearch/go-cron (default AND and `DowOrDom`), adhocore/gronx, robfig/cron/v3, hashicorp/cronexpr
- `dst.go`: a candidate Burrow DST layer: the library only computes in UTC on the naive wall clock, Burrow maps the result back to the zone
- `tz.go`: builds a small probe exe with and without `time/tzdata`, runs it without `GOROOT`, deletes it
- `main.go`: part 1 expected runs, part 2 DST behaviour, part 3 invalid expressions, part 4 time zones in a built exe, part 5 speed
- `clock.go`: high-resolution timer (copied from SPIKE-005)
- `results.md`: output of the last run

Day matching follows Vixie cron and cronie: if both day-of-month and day-of-week are restricted, either may match; a field that starts with `*` (also `*/2`) counts as unrestricted, so then both must match.

The DST rule in `want` is the one cronie describes in cron(8): jobs at a fixed time (minute and hour without `*`) whose time is skipped run once right after the change, and do not run a second time when the clock goes back; jobs with `*` in minute or hour follow the real clock. Part 2 also shows the simpler "once" rule (every wall time at most once, also for `*` jobs), which leaves a one-hour hole in `*/15` and hourly jobs on fall-back day.

Every library gets `t.In(zone)`, never a `CRON_TZ=` prefix, because the zone is a separate pipeline field.
