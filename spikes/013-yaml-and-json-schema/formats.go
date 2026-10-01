package main

// Custom formats for SPEC 6 / 10: cron, IANA time zone, Go duration, http(s) URL that may hold
// expressions. Each check returns "" when the value is fine, else a short reason.

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // embed the zone database: a shipped binary cannot rely on GOROOT/lib/time
)

var customFormats = map[string]func(string) string{
	"cron":        checkCron,
	"iana-tz":     checkTZ,
	"go-duration": checkDuration,
	"http-url":    checkHTTPURL,
}

var cronFields = []struct {
	name     string
	min, max int
	names    []string
}{
	{"minute", 0, 59, nil},
	{"hour", 0, 23, nil},
	{"day of month", 1, 31, nil},
	{"month", 1, 12, []string{"JAN", "FEB", "MAR", "APR", "MAY", "JUN", "JUL", "AUG", "SEP", "OCT", "NOV", "DEC"}},
	{"day of week", 0, 6, []string{"SUN", "MON", "TUE", "WED", "THU", "FRI", "SAT"}},
}

func checkCron(s string) string {
	f := strings.Fields(s)
	if len(f) != 5 {
		return fmt.Sprintf("needs 5 fields (minute hour day month weekday), got %d", len(f))
	}
	for i, part := range f {
		cf := cronFields[i]
		num := func(x string) (int, bool) {
			for j, n := range cf.names {
				if strings.EqualFold(x, n) {
					return j + cf.min, true
				}
			}
			n, err := strconv.Atoi(x)
			return n, err == nil && n >= cf.min && n <= cf.max
		}
		for _, item := range strings.Split(part, ",") {
			rng, step, hasStep := strings.Cut(item, "/")
			if hasStep {
				if n, err := strconv.Atoi(step); err != nil || n < 1 {
					return fmt.Sprintf("%s: bad step %q", cf.name, step)
				}
			}
			if rng == "*" {
				continue
			}
			lo, hi, isRange := strings.Cut(rng, "-")
			a, ok := num(lo)
			if !ok {
				return fmt.Sprintf("%s: %q is not in %d-%d", cf.name, lo, cf.min, cf.max)
			}
			if isRange {
				b, ok := num(hi)
				if !ok || b < a {
					return fmt.Sprintf("%s: bad range %q", cf.name, rng)
				}
			}
		}
	}
	return ""
}

func checkTZ(s string) string {
	if s == "" || s == "Local" {
		return "not an IANA time zone name"
	}
	if _, err := time.LoadLocation(s); err != nil {
		return "unknown IANA time zone"
	}
	return ""
}

func checkDuration(s string) string {
	d, err := time.ParseDuration(s)
	if err != nil {
		return "not a Go duration like 500ms, 60s, 5m, 2h"
	}
	if d <= 0 {
		return "must be positive"
	}
	return ""
}

// checkHTTPURL checks the literal part only; a host that comes from an expression is checked at
// run time (SPEC 6.7).
func checkHTTPURL(s string) string {
	lit := s
	if i := strings.Index(s, "${{"); i >= 0 {
		lit = s[:i]
		if lit == "" {
			return ""
		}
		if !strings.HasPrefix(lit, "http://") && !strings.HasPrefix(lit, "https://") {
			return "only http and https URLs are allowed"
		}
		return ""
	}
	u, err := url.Parse(lit)
	if err != nil {
		return "not a valid URL"
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "only http and https URLs are allowed"
	}
	if u.Host == "" {
		return "URL has no host"
	}
	return ""
}
