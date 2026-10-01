package main

// Brute-force reference: a small, strict 5-field parser (Vixie/cronie syntax only)
// and a Next that steps minute by minute through real instants in the zone.

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type refSpec struct {
	min, hour, dom, month, dow uint64
	minStar, hourStar          bool // field text starts with '*' (cronie MIN_STAR / HR_STAR)
	domStar, dowStar           bool // cronie DOM_STAR / DOW_STAR
}

var refMacros = map[string]string{
	"@yearly": "0 0 1 1 *", "@annually": "0 0 1 1 *", "@monthly": "0 0 1 * *",
	"@weekly": "0 0 * * 0", "@daily": "0 0 * * *", "@midnight": "0 0 * * *", "@hourly": "0 * * * *",
}

var monthNames = map[string]int{"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6, "jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12}
var dowNames = map[string]int{"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6}

func refParse(expr string) (*refSpec, error) {
	e := strings.TrimSpace(expr)
	if e == "" {
		return nil, errors.New("empty expression")
	}
	if strings.HasPrefix(e, "@") {
		m, ok := refMacros[strings.ToLower(e)]
		if !ok {
			return nil, fmt.Errorf("unknown macro %q", e)
		}
		e = m
	}
	f := strings.Fields(e)
	if len(f) != 5 {
		return nil, fmt.Errorf("want 5 fields, got %d", len(f))
	}
	s := &refSpec{}
	var err error
	if s.min, err = refField(f[0], 0, 59, nil); err != nil {
		return nil, fmt.Errorf("minute: %w", err)
	}
	if s.hour, err = refField(f[1], 0, 23, nil); err != nil {
		return nil, fmt.Errorf("hour: %w", err)
	}
	if s.dom, err = refField(f[2], 1, 31, nil); err != nil {
		return nil, fmt.Errorf("day of month: %w", err)
	}
	if s.month, err = refField(f[3], 1, 12, monthNames); err != nil {
		return nil, fmt.Errorf("month: %w", err)
	}
	if s.dow, err = refField(f[4], 0, 7, dowNames); err != nil {
		return nil, fmt.Errorf("day of week: %w", err)
	}
	if s.dow&(1<<7) != 0 {
		s.dow |= 1 // 7 = Sunday
	}
	s.minStar, s.hourStar = f[0][0] == '*', f[1][0] == '*'
	s.domStar, s.dowStar = f[2][0] == '*', f[4][0] == '*'
	return s, nil
}

func refValue(v string, lo, hi int, names map[string]int) (int, error) {
	if n, ok := names[strings.ToLower(v)]; ok {
		return n, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || strings.HasPrefix(v, "+") || strings.HasPrefix(v, "-") {
		return 0, fmt.Errorf("bad value %q", v)
	}
	if n < lo || n > hi {
		return 0, fmt.Errorf("%d out of range %d-%d", n, lo, hi)
	}
	return n, nil
}

func refField(field string, lo, hi int, names map[string]int) (uint64, error) {
	var bits uint64
	for _, part := range strings.Split(field, ",") {
		if part == "" {
			return 0, errors.New("empty list item")
		}
		rng, stepText, hasStep := strings.Cut(part, "/")
		step := 1
		if hasStep {
			n, err := strconv.Atoi(stepText)
			if err != nil || n < 1 {
				return 0, fmt.Errorf("bad step %q", stepText)
			}
			step = n
		}
		var a, b int
		switch {
		case rng == "*":
			a, b = lo, hi
			if hi == 7 {
				b = 6
			}
		case strings.Contains(rng, "-"):
			x, y, _ := strings.Cut(rng, "-")
			var err error
			if a, err = refValue(x, lo, hi, names); err != nil {
				return 0, err
			}
			if b, err = refValue(y, lo, hi, names); err != nil {
				return 0, err
			}
			if a > b {
				return 0, fmt.Errorf("range %q runs backwards", rng)
			}
		default:
			if hasStep {
				return 0, fmt.Errorf("step needs a range or *: %q", part)
			}
			var err error
			if a, err = refValue(rng, lo, hi, names); err != nil {
				return 0, err
			}
			b = a
		}
		for v := a; v <= b; v += step {
			bits |= 1 << uint(v)
		}
	}
	return bits, nil
}

// match tests a wall-clock time (naive, stored as UTC).
func (s *refSpec) match(w time.Time) bool {
	if s.min&(1<<uint(w.Minute())) == 0 || s.hour&(1<<uint(w.Hour())) == 0 || s.month&(1<<uint(w.Month())) == 0 {
		return false
	}
	dom := s.dom&(1<<uint(w.Day())) != 0
	dow := s.dow&(1<<uint(w.Weekday())) != 0
	if s.domStar || s.dowStar {
		return dom && dow
	}
	return dom || dow
}

// DST modes of the reference.
type dstMode int

const (
	modeWall   dstMode = iota // match the wall clock literally: skipped times never run, repeated times run twice
	modeOnce                  // proposed rule A: skipped time runs once at the first minute after the gap; repeated time only at its first occurrence
	modeCronie                // cronie: rule A for fixed-time jobs, modeWall for jobs whose minute or hour field starts with '*'
)

var modeNames = map[dstMode]string{modeWall: "wall", modeOnce: "once", modeCronie: "cronie"}

func naive(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, time.UTC)
}

// isRepeat: the wall-clock time of instant m already occurred in the 3 hours before m.
func isRepeat(m time.Time, loc *time.Location) bool {
	w := naive(m.In(loc))
	for k := 1; k <= 180; k++ {
		if naive(m.Add(-time.Duration(k) * time.Minute).In(loc)).Equal(w) {
			return true
		}
	}
	return false
}

const refLimit = 9 * 366 * 24 * 60 // minutes; 29 Feb can be 8 years apart

// refNext returns the first run strictly after t, or ok=false within ~9 years.
func refNext(s *refSpec, t time.Time, loc *time.Location, mode dstMode) (time.Time, bool) {
	if mode == modeCronie {
		if s.minStar || s.hourStar {
			mode = modeWall
		} else {
			mode = modeOnce
		}
	}
	m := t.Truncate(time.Minute).Add(time.Minute)
	prev := naive(m.Add(-time.Minute).In(loc))
	for i := 0; i < refLimit; i++ {
		w := naive(m.In(loc))
		if s.match(w) && (mode == modeWall || !isRepeat(m, loc)) {
			return m.In(loc), true
		}
		if mode == modeOnce && w.Sub(prev) > time.Minute {
			for x := prev.Add(time.Minute); x.Before(w); x = x.Add(time.Minute) {
				if s.match(x) {
					return m.In(loc), true
				}
			}
		}
		prev = w
		m = m.Add(time.Minute)
	}
	return time.Time{}, false
}
