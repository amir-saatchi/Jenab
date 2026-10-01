package main

// Candidate Burrow DST layer on top of any library (the "cronie rule"). The library
// only ever computes in UTC on the naive wall clock, where there is no DST; this
// file maps each wall time back to the zone:
//
//   - Fixed-time jobs (minute and hour fields do not start with '*'): a wall time
//     that does not exist runs once at the first instant after the gap; a wall time
//     that exists twice runs only at its first occurrence.
//   - Jobs whose minute or hour field starts with '*' follow the real clock: they
//     run again in the second pass of a repeated hour, and skipped times are not
//     caught up.
import (
	"strings"
	"time"
)

func isFixedTime(expr string) bool {
	e := strings.TrimSpace(expr)
	if m, ok := refMacros[strings.ToLower(e)]; ok {
		e = m
	}
	f := strings.Fields(e)
	if len(f) < 2 {
		return true
	}
	return f[0][0] != '*' && f[1][0] != '*'
}

// wallToUTC keeps the wall-clock fields and drops the zone.
func wallToUTC(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.UTC)
}

// instantsFor returns the instants (0, 1 or 2) whose wall clock in loc equals w.
func instantsFor(w time.Time, loc *time.Location) []time.Time {
	var out []time.Time
	seen := map[int]bool{}
	for _, probe := range []time.Duration{-26 * time.Hour, 26 * time.Hour} {
		_, off := w.Add(probe).In(loc).Zone()
		if seen[off] {
			continue
		}
		seen[off] = true
		m := w.Add(-time.Duration(off) * time.Second)
		if wallToUTC(m.In(loc)).Equal(w) {
			out = append(out, m)
		}
	}
	if len(out) == 2 && out[1].Before(out[0]) {
		out[0], out[1] = out[1], out[0]
	}
	return out
}

func withDSTRule(l lib) lib {
	return lib{l.name + " + DST layer", l.short + "+layer", func(expr string) (nextFunc, error) {
		nf, err := l.parse(expr)
		if err != nil {
			return nil, err
		}
		fixed := isFixedTime(expr)
		return func(t time.Time) (time.Time, error) {
			loc := t.Location()
			a, err := naiveSearch(nf, wallToUTC(t), t, loc, fixed)
			if err != nil || fixed {
				return a, err
			}
			// A '*' job also runs in the second pass of a repeated hour: if the zone
			// period of t ends with the clock going back, search again from there.
			_, off := t.Zone()
			if _, end := t.ZoneBounds(); !end.IsZero() && (a.IsZero() || end.Before(a)) {
				if _, offEnd := end.In(loc).Zone(); offEnd < off {
					nw, err := nf(wallToUTC(end.In(loc)).Add(-time.Second))
					if err == nil && !nw.IsZero() {
						for _, m := range instantsFor(nw.UTC(), loc) {
							if !m.Before(end) && (a.IsZero() || m.Before(a)) {
								return m.In(loc), nil
							}
						}
					}
				}
			}
			return a, nil
		}, nil
	}}
}

// naiveSearch asks the library for wall-clock times in UTC (no DST there) and maps
// each one back to the zone until one lands after t.
func naiveSearch(nf nextFunc, w, t time.Time, loc *time.Location, fixed bool) (time.Time, error) {
	for i := 0; i < 1000; i++ {
		nw, err := nf(w)
		if err != nil || nw.IsZero() {
			return time.Time{}, err
		}
		nw = nw.UTC()
		ins := instantsFor(nw, loc)
		if len(ins) == 0 {
			if fixed { // skipped time: first instant after the gap
				_, off := nw.Add(-26 * time.Hour).In(loc).Zone()
				start, _ := nw.Add(-time.Duration(off) * time.Second).In(loc).ZoneBounds()
				if start.After(t) {
					return start.In(loc), nil
				}
			}
		} else if fixed {
			if ins[0].After(t) { // only the first occurrence counts
				return ins[0].In(loc), nil
			}
		} else {
			for _, m := range ins {
				if m.After(t) {
					return m.In(loc), nil
				}
			}
		}
		w = nw
	}
	return time.Time{}, nil
}
