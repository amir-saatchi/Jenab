package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	gapLimit   = 25 * time.Second // samples come every 10 s; a longer silence means the process didn't run
	asleepMin  = 1.0              // seconds of (biased - unbiased) interrupt time that count as sleep
	lateLimit  = 5.0              // a probe firing more than this many seconds late counts as late
	wakeMerge  = 2 * time.Minute  // wake signals this close together are one wake
	maxTimeRow = 400
)

// asleep is the cumulative time the machine spent asleep according to the
// interrupt-time counters: biased interrupt time counts sleep, unbiased doesn't.
func asleep(r Rec) float64 { return float64(int64(r.IT-r.UIT)) / 1e7 }
func qpcS(r Rec) float64   { return float64(r.QPC) / float64(r.QPCF) }
func hms(r Rec) string {
	if len(r.Local) >= 19 {
		return r.Local[11:19]
	}
	return r.Local
}
func hmsOf(rfc string) string {
	t, err := time.Parse(time.RFC3339, rfc)
	if err != nil {
		return rfc
	}
	return t.Format("15:04:05")
}
func secs(d time.Duration) string { return fmt.Sprintf("%.1f s", d.Seconds()) }
func dur(s float64) string {
	if math.Abs(s) >= 120 {
		return fmt.Sprintf("%.1f min", s/60)
	}
	return fmt.Sprintf("%.1f s", s)
}
func locked(desk string) bool { return desk != "" && desk != "Default" }

type wakeT struct {
	at   time.Time
	rec  Rec
	srcs []string
}

func analyze(path string, w io.Writer) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var recs []Rec
	bad := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var r Rec
		if json.Unmarshal(sc.Bytes(), &r) != nil {
			bad++
			continue
		}
		recs = append(recs, r)
	}
	if len(recs) == 0 {
		return fmt.Errorf("%s: no log lines", path)
	}
	p := func(format string, a ...any) { fmt.Fprintf(w, format, a...) }
	first, last := recs[0], recs[len(recs)-1]
	t0 := first.UTC
	rel := func(r Rec) string {
		d := r.UTC.Sub(t0)
		return fmt.Sprintf("+%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
	}
	var startInfo []string
	var jobs []string
	starts := 0
	for _, r := range recs {
		if r.Kind == "start" {
			starts++
			if startInfo == nil {
				startInfo = r.Info
			}
		}
	}
	for _, s := range startInfo {
		if strings.HasPrefix(s, "job ") {
			jobs = append(jobs, strings.TrimPrefix(s, "job "))
		}
	}

	p("# SPIKE-004 sleep watch: results\n\n")
	p("Log `%s`: %d lines", path, len(recs))
	if bad > 0 {
		p(" (%d unreadable lines skipped)", bad)
	}
	p(", %s to %s local, %s of wall time.", first.Local[:19], last.Local[:19], dur(last.UTC.Sub(t0).Seconds()))
	if starts > 1 {
		p(" **The log holds %d runs;** the tables treat them as one, analyze one run per file for clean numbers.", starts)
	}
	stopped := "no `stop` line: the process was killed or is still running"
	for _, r := range recs {
		if r.Kind == "stop" {
			stopped = "stopped by " + r.Reason + ", " + r.Result
		}
	}
	p(" End: %s.\n\n", stopped)
	for _, s := range startInfo {
		p("- %s\n", s)
	}

	// wakes: resume events, the first line after a log gap, and counter jumps
	var cands []wakeT
	for i, r := range recs {
		if (r.Kind == "power" || r.Kind == "powercb") && (r.Name == "PBT_APMRESUMEAUTOMATIC" || r.Name == "PBT_APMRESUMESUSPEND") {
			src := "window " + r.Name
			if r.Kind == "powercb" {
				src = "callback " + r.Name
			}
			cands = append(cands, wakeT{r.UTC, r, []string{src}})
		}
		if i > 0 {
			a := recs[i-1]
			if r.UTC.Sub(a.UTC) > gapLimit {
				cands = append(cands, wakeT{r.UTC, r, []string{fmt.Sprintf("first line after a %s log gap", dur(r.UTC.Sub(a.UTC).Seconds()))}})
			} else if asleep(r)-asleep(a) > asleepMin {
				cands = append(cands, wakeT{r.UTC, r, []string{fmt.Sprintf("interrupt-time counters show %s asleep", dur(asleep(r)-asleep(a)))}})
			}
		}
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].at.Before(cands[j].at) })
	var wakes []wakeT
	for _, c := range cands {
		if n := len(wakes); n > 0 && c.at.Sub(wakes[n-1].at) < wakeMerge {
			wakes[n-1].srcs = append(wakes[n-1].srcs, c.srcs...)
			continue
		}
		wakes = append(wakes, c)
	}
	lastWakeBefore := func(t time.Time) *wakeT {
		var found *wakeT
		for i := range wakes {
			if !wakes[i].at.After(t) {
				found = &wakes[i]
			}
		}
		return found
	}

	// 1. event sources
	count := map[string]int{}
	for _, r := range recs {
		switch r.Kind {
		case "power", "powercb", "session", "endsession":
			count[r.Kind+" "+r.Name]++
		case "setting":
			count["setting "+r.Name]++
		}
	}
	p("\n## 1. Event sources\n\n")
	p("\"Wails v3\" is what v3.0.0-beta.26 `pkg/application/application_windows.go` does: its hidden top-level main-thread window turns `WM_POWERBROADCAST` into application events. It registers no power settings, no suspend/resume callback and no session notifications.\n\n")
	p("| Source | Event | Seen | Exposed by Wails v3 beta.26 |\n|---|---|---|---|\n")
	wails := map[string]string{
		"PBT_APMSUSPEND":           "yes: `events.Windows.APMSuspend`, also as `events.Common.SystemWillSleep`",
		"PBT_APMRESUMEAUTOMATIC":   "yes: `events.Windows.APMResumeAutomatic`, also as `events.Common.SystemDidWake`",
		"PBT_APMRESUMESUSPEND":     "yes: `events.Windows.APMResumeSuspend`",
		"PBT_APMPOWERSTATUSCHANGE": "yes: `events.Windows.APMPowerStatusChange`",
	}
	for _, n := range []string{"PBT_APMSUSPEND", "PBT_APMRESUMEAUTOMATIC", "PBT_APMRESUMESUSPEND", "PBT_APMPOWERSTATUSCHANGE"} {
		p("| `WM_POWERBROADCAST` (hidden window) | `%s` | %d | %s |\n", n, count["power "+n], wails[n])
	}
	for _, ps := range powerSettings {
		p("| `WM_POWERBROADCAST` `PBT_POWERSETTINGCHANGE` | `%s` | %d | only if the app registers the GUID on Wails' window itself; Wails then raises `APMPowerSettingChange` without the GUID or value |\n", ps.name, count["setting "+ps.name])
	}
	for _, n := range []string{"PBT_APMSUSPEND", "PBT_APMRESUMEAUTOMATIC", "PBT_APMRESUMESUSPEND"} {
		p("| `PowerRegisterSuspendResumeNotification` callback | `%s` | %d | no |\n", n, count["powercb "+n])
	}
	for _, n := range []string{"lock", "unlock"} {
		p("| `WTSRegisterSessionNotification` | %s | %d | no |\n", n, count["session "+n])
	}
	var others []string
	for k, v := range count {
		if !strings.HasPrefix(k, "setting ") && !strings.Contains(k, "session lock") && !strings.Contains(k, "session unlock") &&
			!(strings.HasPrefix(k, "power") && wails[strings.Fields(k)[1]] != "") {
			others = append(others, fmt.Sprintf("%s (%d)", k, v))
		}
	}
	sort.Strings(others)
	if len(others) > 0 {
		p("\nOther events seen: %s.\n", strings.Join(others, ", "))
	}

	// 2. timeline
	p("\n## 2. Timeline\n\n")
	p("All OS events, scheduler runs, the AfterFunc and job timers, probe fires more than %.0f s late, keychain reads not from the minute loop, and desktop changes. A row in italics marks a silence in the log. Windows sends each registered power setting's current value right after registration, so the first `setting` rows are not changes.\n\n", lateLimit)
	p("| Local | Since start | Kind | Event | Detail | Desktop |\n|---|---|---|---|---|---|\n")
	rows := 0
	prevDesk := ""
	for i, r := range recs {
		if i > 0 {
			a := recs[i-1]
			if g := r.UTC.Sub(a.UTC); g > gapLimit {
				p("| *%s–%s* | | | *no log lines for %s* | *asleep per counters: %s; Go monotonic advanced %s* | |\n",
					hms(a), hms(r), dur(g.Seconds()), dur(asleep(r)-asleep(a)), dur(r.MonoS-a.MonoS))
				rows++
			}
		}
		show, event, detail := false, r.Name, r.Detail
		switch r.Kind {
		case "start", "stop", "error", "endsession", "power", "powercb", "setting", "session":
			show = true
			if r.Kind == "start" {
				detail = fmt.Sprintf("%d jobs", len(jobs))
			}
			if r.Kind == "stop" {
				detail = r.Reason + "; " + r.Result
			}
		case "sched":
			show = true
			event = r.Name + " scheduler"
			if r.Trigger != "" {
				var ts []string
				for _, c := range r.Covers {
					ts = append(ts, hmsOf(c))
				}
				detail = fmt.Sprintf("**%s** run for %s, %s after the first, check: %s", r.Trigger, strings.Join(ts, ", "), dur(r.LateS), r.Reason)
			} else {
				detail = "check (" + r.Reason + "): " + r.Detail
			}
		case "probe":
			if r.Name == "afterfunc_10m" || r.Name == "timer_job" || math.Abs(r.LateS) > lateLimit {
				show = true
				detail = fmt.Sprintf("%s late (wall %s, Go monotonic %s)", dur(r.LateS), dur(r.WallS), dur(r.MonoDS))
				if r.Name == "timer_job" {
					detail = fmt.Sprintf("job %s, fired %s after it", hmsOf(r.Target), dur(r.LateS))
				}
			}
		case "keychain":
			if r.Reason != "minute" || !strings.HasPrefix(r.Result, "ok") {
				show = true
				event = "read (" + r.Reason + ")"
				detail = fmt.Sprintf("%s, %.2f ms", r.Result, r.DurMS)
			}
		case "sample":
			if prevDesk != "" && r.Desk != prevDesk {
				show = true
				event = "desktop changed"
				detail = prevDesk + " → " + r.Desk
			}
		}
		if r.Kind == "sample" {
			prevDesk = r.Desk
		}
		if show && rows < maxTimeRow {
			p("| %s | %s | %s | %s | %s | %s |\n", hms(r), rel(r), r.Kind, event, detail, r.Desk)
			rows++
		}
	}
	if rows >= maxTimeRow {
		p("\n(Timeline cut at %d rows.)\n", maxTimeRow)
	}

	// 3. sleep intervals
	p("\n## 3. Sleep intervals\n\n")
	p("**From suspend/resume events.** \"Asleep\" is the growth of (interrupt time − unbiased interrupt time) between the two events. \"Lines written\" are log lines between the events other than power events: if there are any, the process ran during sleep.\n\n")
	p("| Source | Suspend | Resume | Wall | Asleep (counters) | Go monotonic | QPC | GetTickCount64 | Lines written in between |\n|---|---|---|---|---|---|---|---|---|\n")
	episodes := 0
	for _, kind := range []string{"power", "powercb"} {
		for i, s := range recs {
			if s.Kind != kind || s.Name != "PBT_APMSUSPEND" {
				continue
			}
			// the next resume from the same source; if that source sent none, from the other one
			isResume := func(r Rec, any bool) bool {
				return (r.Kind == kind || (any && (r.Kind == "power" || r.Kind == "powercb"))) && (r.Name == "PBT_APMRESUMEAUTOMATIC" || r.Name == "PBT_APMRESUMESUSPEND")
			}
			any := true
			for j := i + 1; j < len(recs); j++ {
				if isResume(recs[j], false) {
					any = false
					break
				}
			}
			for j := i + 1; j < len(recs); j++ {
				r := recs[j]
				if isResume(r, any) {
					kinds := map[string]int{}
					n := 0
					for _, x := range recs[i+1 : j] {
						if x.Kind != "power" && x.Kind != "powercb" && x.Kind != "setting" {
							kinds[x.Kind]++
							n++
						}
					}
					src := map[string]string{"power": "window", "powercb": "callback"}[kind]
					if r.Kind != kind {
						src += " suspend, " + map[string]string{"power": "window", "powercb": "callback"}[r.Kind] + " resume"
					}
					p("| %s | %s | %s | %s | %s | %s | %s | %s | %d %s |\n", src, hms(s), hms(r), dur(r.UTC.Sub(s.UTC).Seconds()),
						dur(asleep(r)-asleep(s)), dur(r.MonoS-s.MonoS), dur(qpcS(r)-qpcS(s)), dur(float64(r.Tick-s.Tick)/1000), n, kindList(kinds))
					episodes++
					break
				}
			}
		}
	}
	if episodes == 0 {
		p("| none | | | | | | | | |\n")
	}
	p("\n**From the counters and log gaps** (consecutive lines more than %s apart, or more than %.0f s asleep between them; touching spans are merged):\n\n", secs(gapLimit), asleepMin)
	p("| From (last line before) | To (first line after) | Wall | Asleep (counters) | Interrupt time | Unbiased interrupt time | Go monotonic | QPC | GetTickCount64 | Lines inside |\n|---|---|---|---|---|---|---|---|---|---|\n")
	sleepish := func(i int) bool { // pair (i-1, i)
		a, b := recs[i-1], recs[i]
		return b.UTC.Sub(a.UTC) > gapLimit || asleep(b)-asleep(a) > asleepMin
	}
	spans := 0
	for i := 1; i < len(recs); i++ {
		if !sleepish(i) {
			continue
		}
		j := i
		for j+1 < len(recs) && sleepish(j+1) {
			j++
		}
		a, b := recs[i-1], recs[j]
		kinds := map[string]int{}
		for _, x := range recs[i:j] {
			kinds[x.Kind]++
		}
		p("| %s | %s | %s | %s | %s | %s | %s | %s | %s | %d %s |\n", hms(a), hms(b), dur(b.UTC.Sub(a.UTC).Seconds()), dur(asleep(b)-asleep(a)),
			dur(float64(b.IT-a.IT)/1e7), dur(float64(b.UIT-a.UIT)/1e7), dur(b.MonoS-a.MonoS), dur(qpcS(b)-qpcS(a)), dur(float64(b.Tick-a.Tick)/1000), j-i, kindList(kinds))
		spans++
		i = j
	}
	if spans == 0 {
		p("| none | | | | | | | | | |\n")
	}

	// 4. wakes and Go timers
	p("\n## 4. Wakes and what fired after them\n\n")
	if len(wakes) == 0 {
		p("No wake detected.\n")
	}
	type firstSpec struct{ label, kind, name string }
	specs := []firstSpec{
		{"10 s sample ticker", "sample", ""},
		{"`time.Ticker(1 min)`", "probe", "ticker_1m"},
		{"`time.Sleep(1 min)` loop", "probe", "sleep_1m"},
		{"`time.AfterFunc(10 min)`", "probe", "afterfunc_10m"},
		{"job `time.Timer` (naive scheduler)", "probe", "timer_job"},
		{"keychain minute read", "keychain", "minute"},
		{"spec scheduler run", "sched", "spec"},
		{"tick-only scheduler run", "sched", "tick_only"},
	}
	for wi, wk := range wakes {
		end := last.UTC.Add(time.Second)
		if wi+1 < len(wakes) {
			end = wakes[wi+1].at
		}
		p("### Wake %d at %s (%s)\n\nSignals: %s.\n\n", wi+1, hms(wk.rec), rel(wk.rec), strings.Join(dedupe(wk.srcs), "; "))
		p("| Timer or loop | First fire after the wake | Seconds after the wake | Late vs its own schedule |\n|---|---|---|---|\n")
		for _, s := range specs {
			var hit *Rec
			for k := range recs {
				r := recs[k]
				if r.UTC.Before(wk.at) || !r.UTC.Before(end) || r.Kind != s.kind {
					continue
				}
				if s.kind == "sample" || (s.kind == "keychain" && r.Reason == s.name) || (s.kind == "probe" && r.Name == s.name) || (s.kind == "sched" && r.Name == s.name && r.Trigger != "") {
					hit = &recs[k]
					break
				}
			}
			if hit == nil {
				p("| %s | not before the next wake / end | | |\n", s.label)
				continue
			}
			late := ""
			if s.kind == "probe" || s.kind == "sched" {
				late = dur(hit.LateS)
			}
			if s.kind == "sched" {
				late = hit.Trigger + ", " + dur(hit.LateS) + " after the scheduled time (" + hit.Reason + ")"
			}
			p("| %s | %s | %s | %s |\n", s.label, hms(*hit), dur(hit.UTC.Sub(wk.at).Seconds()), late)
		}
		p("\n")
	}
	p("\n**All probe fires** (late = more than %.0f s off the wall clock):\n\n", lateLimit)
	p("| Probe | Fires | On time | Late fires |\n|---|---|---|---|\n")
	for _, name := range []string{"ticker_1m", "sleep_1m", "afterfunc_10m", "timer_job"} {
		n, ok := 0, 0
		var lates []string
		for _, r := range recs {
			if r.Kind != "probe" || r.Name != name {
				continue
			}
			n++
			if math.Abs(r.LateS) <= lateLimit {
				ok++
			} else if len(lates) < 12 {
				lates = append(lates, fmt.Sprintf("%s: %s late (Go monotonic %s)", hms(r), dur(r.LateS), dur(r.MonoDS)))
			}
		}
		p("| `%s` | %d | %d | %s |\n", name, n, ok, strings.Join(lates, "; "))
	}

	// 5. jobs
	p("\n## 5. Scheduled jobs\n\n")
	p("*spec* = SPEC 6.2 scheduler: check at start, every minute (1-minute ticker) and on each resume or display-on event. *tick-only* = the same without OS events. *naive* = one `time.Timer` per job, duration computed once at start. \"After wake\" is filled when the job time fell before the wake that preceded the run.\n\n")
	p("| Job | spec | tick-only | naive timer |\n|---|---|---|---|\n")
	for _, j := range jobs {
		jt, _ := time.Parse(time.RFC3339, j)
		cell := func(sched string) string {
			for _, r := range recs {
				if r.Kind != "sched" || r.Name != sched {
					continue
				}
				for _, c := range r.Covers {
					if c == j {
						word := "on time"
						if r.Trigger == "catch_up" {
							word = "caught up"
						}
						s := fmt.Sprintf("**%s** at %s, %s after the job time", word, hms(r), dur(r.UTC.Sub(jt).Seconds()))
						if wk := lastWakeBefore(r.UTC); wk != nil && jt.Before(wk.at) {
							s += fmt.Sprintf(", %s after wake", dur(r.UTC.Sub(wk.at).Seconds()))
						}
						return s + " (" + r.Reason + ")"
					}
				}
			}
			return "**missed** (no run by the end of the log)"
		}
		naive := "**never fired**"
		if jt.Before(t0) {
			naive = "not armed (time before start)"
		}
		for _, r := range recs {
			if r.Kind == "probe" && r.Name == "timer_job" && r.Target == j {
				word := "on time"
				if math.Abs(r.LateS) > 65 {
					word = "late"
				}
				naive = fmt.Sprintf("**%s**: fired at %s, %s after the job time", word, hms(r), dur(r.LateS))
				if wk := lastWakeBefore(r.UTC); wk != nil && jt.Before(wk.at) {
					naive += fmt.Sprintf(", %s after wake", dur(r.UTC.Sub(wk.at).Seconds()))
				}
			}
		}
		p("| %s | %s | %s | %s |\n", hmsOf(j), cell("spec"), cell("tick_only"), naive)
	}
	if len(jobs) == 0 {
		p("| no jobs given | | | |\n")
	}

	// 6. keychain
	p("\n## 6. Keychain reads (SPIKE-006 lock-screen row)\n\n")
	p("One `go-keyring` read of `%s:%s` every minute, plus on lock/unlock (at once and 5 s later) and on resume. Desktop `Default` = unlocked; `Winlogon` or no access = lock screen.\n\n", service, account)
	p("| Input desktop | Reads | ok | Errors | Median | Max |\n|---|---|---|---|---|---|\n")
	byDesk := map[string][]Rec{}
	var desks []string
	for _, r := range recs {
		if r.Kind == "keychain" {
			if _, ok := byDesk[r.Desk]; !ok {
				desks = append(desks, r.Desk)
			}
			byDesk[r.Desk] = append(byDesk[r.Desk], r)
		}
	}
	for _, d := range desks {
		rs := byDesk[d]
		ok := 0
		var ms []float64
		errs := map[string]int{}
		for _, r := range rs {
			if r.Result == "ok" {
				ok++
			} else {
				errs[r.Result]++
			}
			ms = append(ms, r.DurMS)
		}
		sort.Float64s(ms)
		label := d
		if locked(d) {
			label += " (locked)"
		}
		p("| %s | %d | %d | %s | %.2f ms | %.2f ms |\n", label, len(rs), ok, kindList(errs), ms[len(ms)/2], ms[len(ms)-1])
	}
	if len(desks) == 0 {
		p("| no reads | | | | | |\n")
	}

	// 7. clocks over the whole run
	p("\n## 7. Clocks over the whole log\n\n")
	wallAll := last.UTC.Sub(first.UTC).Seconds()
	p("| Clock | Elapsed first → last line | Minus wall time |\n|---|---|---|\n")
	row := func(name string, v float64) { p("| %s | %s | %s |\n", name, dur(v), dur(v-wallAll)) }
	row("wall (`time.Now`, UTC)", wallAll)
	row("Go monotonic (`time.Since`)", last.MonoS-first.MonoS)
	row("`QueryPerformanceCounter`", qpcS(last)-qpcS(first))
	row("`QueryInterruptTime`", float64(last.IT-first.IT)/1e7)
	row("`QueryUnbiasedInterruptTime`", float64(last.UIT-first.UIT)/1e7)
	row("`GetTickCount64`", float64(last.Tick-first.Tick)/1000)
	p("\nAsleep over the run per the counters: %s.\n", dur(asleep(last)-asleep(first)))
	return nil
}

func kindList(m map[string]int) string {
	if len(m) == 0 {
		return ""
	}
	var ks []string
	for k, v := range m {
		ks = append(ks, fmt.Sprintf("%s %d", k, v))
	}
	sort.Strings(ks)
	return "(" + strings.Join(ks, ", ") + ")"
}

func dedupe(s []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range s {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}
