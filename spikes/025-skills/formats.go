package main

// Custom formats (SPEC 10): cron (SPEC 6.2 syntax), IANA time zone, Go duration, http(s) URL
// whose literal part is checked (a host from an expression is checked at run time, SPEC 6.7).

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
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
	{"day of week", 0, 7, []string{"SUN", "MON", "TUE", "WED", "THU", "FRI", "SAT"}},
}

var cronMacros = map[string]bool{"@yearly": true, "@annually": true, "@monthly": true, "@weekly": true, "@daily": true, "@midnight": true, "@hourly": true}

func checkCron(s string) string {
	t := strings.TrimSpace(s)
	if strings.HasPrefix(t, "@") {
		if cronMacros[t] {
			return ""
		}
		return fmt.Sprintf("%s is not supported (use 5 fields or @daily, @hourly, @weekly, @monthly, @yearly)", t)
	}
	if strings.HasPrefix(strings.ToUpper(t), "TZ=") || strings.HasPrefix(strings.ToUpper(t), "CRON_TZ=") {
		return "TZ= prefixes are not allowed; use trigger.timezone"
	}
	f := strings.Fields(t)
	if len(f) != 5 {
		return fmt.Sprintf("needs 5 fields (minute hour day month weekday), got %d", len(f))
	}
	for i, part := range f {
		cf := cronFields[i]
		if strings.ContainsAny(part, "#?") || (cf.names == nil && strings.ContainsAny(part, "LWH")) {
			return fmt.Sprintf("%s: %q uses L, W, #, ? or H, which are not supported", cf.name, part)
		}
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
			if hasStep && !isRange {
				return fmt.Sprintf("%s: a step needs * or a range, got %q", cf.name, item)
			}
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
		return "unknown IANA time zone (use a name like UTC or Europe/Berlin)"
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

// literalHost returns the host of a URL whose host part is literal ("" if it comes from an expression).
func literalHost(s string) string {
	lit := s
	if i := strings.Index(s, "${{"); i >= 0 {
		lit = s[:i]
	}
	rest, ok := strings.CutPrefix(lit, "https://")
	if !ok {
		rest, ok = strings.CutPrefix(lit, "http://")
	}
	if !ok {
		return ""
	}
	end := strings.IndexAny(rest, "/?#")
	if end < 0 {
		if strings.Contains(s, "${{") {
			return "" // host not finished before the expression
		}
		end = len(rest)
	}
	return strings.ToLower(rest[:end])
}

// Filter date defaults (SPEC 5.2): ISO date or today, today-7d, today-3m, today-1y,
// start_of_month, start_of_year.
var relDate = regexp.MustCompile(`^today(-[0-9]+[dmy])?$`)

func checkDateDefault(s string) string {
	if relDate.MatchString(s) || s == "start_of_month" || s == "start_of_year" {
		return ""
	}
	if _, err := time.Parse("2006-01-02", s); err == nil {
		return ""
	}
	return fmt.Sprintf("%q is not a date default (use YYYY-MM-DD, today, today-7d, today-3m, today-1y, start_of_month or start_of_year)", s)
}

func resolveDateDefault(s string, now time.Time) string {
	switch s {
	case "start_of_month":
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	case "start_of_year":
		return time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	}
	if relDate.MatchString(s) {
		if s == "today" {
			return now.Format("2006-01-02")
		}
		n, _ := strconv.Atoi(s[6 : len(s)-1])
		switch s[len(s)-1] {
		case 'd':
			return now.AddDate(0, 0, -n).Format("2006-01-02")
		case 'm':
			return now.AddDate(0, -n, 0).Format("2006-01-02")
		case 'y':
			return now.AddDate(-n, 0, 0).Format("2006-01-02")
		}
	}
	return s
}
