// SPIKE-004: how Go timers and a SPEC 6.2 scheduler behave across sleep
// (Modern Standby) on Windows, which OS events arrive, and (for SPIKE-006)
// whether the keychain can be read while the screen is locked.
//
//	go build -o sleepwatch.exe .
//	./sleepwatch.exe watch --jobs 12:40,12:50,13:00   # stop with Ctrl+C or a file named "stop" next to the log
//	./sleepwatch.exe analyze sleep.jsonl > results.md
//	./sleepwatch.exe cleanup                          # removes the test credential after a killed run
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/zalando/go-keyring"
)

const (
	service     = "burrow-spike004"
	account     = "API_KEY"
	secretValue = "sk-test-spike004-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	onTimeLimit = 65 * time.Second // "within 1 minute of its time", plus 5 s slack for the check itself
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "watch":
		watch(os.Args[2:])
	case "analyze":
		if len(os.Args) < 3 {
			usage()
		}
		if err := analyze(os.Args[2], os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "cleanup":
		err := keyring.Delete(service, account)
		fmt.Println("delete", service+":"+account+":", errText(err))
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  sleepwatch watch [--jobs HH:MM[:SS],...] [--log file] [--for duration]
  sleepwatch analyze <log.jsonl> > results.md
  sleepwatch cleanup`)
	os.Exit(2)
}

func errText(err error) string {
	if err == nil {
		return "ok"
	}
	return "error: " + err.Error()
}

// wall returns b-a on the wall clock (Round(0) drops the monotonic reading).
func wall(a, b time.Time) time.Duration { return b.Round(0).Sub(a.Round(0)) }

func parseJobs(s string, now time.Time) ([]time.Time, error) {
	var jobs []time.Time
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var h, m, sec int
		n, _ := fmt.Sscanf(part, "%d:%d:%d", &h, &m, &sec)
		if n < 2 {
			return nil, fmt.Errorf("bad job time %q (want HH:MM or HH:MM:SS)", part)
		}
		jobs = append(jobs, time.Date(now.Year(), now.Month(), now.Day(), h, m, sec, 0, time.Local))
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].Before(jobs[j]) })
	return jobs, nil
}

func watch(args []string) {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	jobsFlag := fs.String("jobs", "", "scheduled times today, local, comma-separated HH:MM or HH:MM:SS")
	logFlag := fs.String("log", "", "JSON-lines log (default sleep.jsonl next to the exe)")
	forFlag := fs.Duration("for", 0, "stop after this much wall time (0 = until Ctrl+C or a stop file)")
	fs.Parse(args)

	exe, _ := os.Executable()
	logPath := *logFlag
	if logPath == "" {
		logPath = filepath.Join(filepath.Dir(exe), "sleep.jsonl")
	}
	logPath, _ = filepath.Abs(logPath)
	stopPath := filepath.Join(filepath.Dir(logPath), "stop")
	os.Remove(stopPath)

	l, err := openLog(logPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer l.close()
	start := l.start
	jobs, err := parseJobs(*jobsFlag, start)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	info := []string{
		fmt.Sprintf("pid %d, %s, %s/%s", os.Getpid(), runtime.Version(), runtime.GOOS, runtime.GOARCH),
		"args: " + strings.Join(os.Args[1:], " "),
		"log: " + logPath + ", stop file: " + stopPath,
		"timezone: " + start.Format("MST -07:00"),
		"QueryInterruptTime: " + dllOf(pIntTime) + ", QueryUnbiasedInterruptTime: " + dllOf(pUIntTime),
	}
	for _, j := range jobs {
		info = append(info, "job "+j.Format(time.RFC3339))
	}
	info = append(info, "keychain set "+service+":"+account+": "+errText(keyring.Set(service, account, secretValue)))

	wake := make(chan string, 16) // resume events for the SPEC scheduler
	sendWake := func(reason string) {
		select {
		case wake <- reason:
		default:
		}
	}
	readKey := func(reason string) {
		t := clock()
		got, err := keyring.Get(service, account)
		d := since(t)
		res := errText(err)
		if err == nil && got != secretValue {
			res = "wrong value"
		}
		l.emit(Rec{Kind: "keychain", Reason: reason, Result: res, DurMS: float64(d.Microseconds()) / 1000, Desk: inputDesktop()})
	}
	onResume := func(src string) {
		sendWake(src)
		go readKey("resume")
	}

	notes := startWindow(winHandlers{
		power: func(code uintptr) {
			l.emit(Rec{Kind: "power", Name: pbtName(code), Desk: inputDesktop()})
			if code == pbtAPMResumeAutomatic || code == pbtAPMResumeSuspend {
				onResume("window " + pbtName(code))
			}
		},
		setting: func(name string, v uint32) {
			l.emit(Rec{Kind: "setting", Name: name, Detail: settingValue(name, v), N: int(v)})
			if name == "GUID_CONSOLE_DISPLAY_STATE" && v == 1 {
				sendWake("display on")
			}
		},
		session: func(code, sid uintptr) {
			name := wtsName(code)
			l.emit(Rec{Kind: "session", Name: name, Detail: fmt.Sprintf("session %d", sid), Desk: inputDesktop()})
			if code == 7 || code == 8 {
				go readKey(name)
				time.AfterFunc(5*time.Second, func() { readKey(name + "+5s") })
			}
		},
		endSession: func(msg string) { l.emit(Rec{Kind: "endsession", Name: msg}) },
	})
	notes = append(notes, registerSuspendResume(func(code uintptr) {
		l.emit(Rec{Kind: "powercb", Name: pbtName(code), Desk: inputDesktop()})
		if code == pbtAPMResumeAutomatic || code == pbtAPMResumeSuspend {
			onResume("callback " + pbtName(code))
		}
	}))
	l.emit(Rec{Kind: "start", Desk: inputDesktop(), Info: append(info, notes...)})

	stop := make(chan struct{})

	// 10 s samples: all clocks plus the input desktop (a gap here = the process didn't run)
	go func() {
		t := time.NewTicker(10 * time.Second)
		for {
			select {
			case <-t.C:
				l.emit(Rec{Kind: "sample", Desk: inputDesktop()})
			case <-stop:
				return
			}
		}
	}()

	// probe: time.Ticker(1 min)
	go func() {
		t := time.NewTicker(time.Minute)
		prev, n := time.Now(), 0
		for {
			select {
			case now := <-t.C:
				now = time.Now() // the channel value is when the tick was due, not when it was received
				n++
				w := wall(prev, now)
				l.emit(Rec{Kind: "probe", Name: "ticker_1m", N: n, WallS: w.Seconds(), MonoDS: now.Sub(prev).Seconds(), LateS: (w - time.Minute).Seconds()})
				prev = now
			case <-stop:
				return
			}
		}
	}()

	// probe: a loop of time.Sleep(1 min)
	go func() {
		for n := 1; ; n++ {
			prev := time.Now()
			time.Sleep(time.Minute)
			now := time.Now()
			w := wall(prev, now)
			l.emit(Rec{Kind: "probe", Name: "sleep_1m", N: n, WallS: w.Seconds(), MonoDS: now.Sub(prev).Seconds(), LateS: (w - time.Minute).Seconds()})
		}
	}()

	// probe: time.AfterFunc(10 min)
	time.AfterFunc(10*time.Minute, func() {
		now := time.Now()
		w := wall(start, now)
		l.emit(Rec{Kind: "probe", Name: "afterfunc_10m", WallS: w.Seconds(), MonoDS: now.Sub(start).Seconds(), LateS: (w - 10*time.Minute).Seconds(),
			Target: start.Add(10 * time.Minute).Round(0).Format(time.RFC3339)})
	})

	// probe and naive scheduler: one time.NewTimer per job, duration computed once at start
	for _, j := range jobs {
		if !j.After(start) {
			continue
		}
		j := j
		t := time.NewTimer(j.Sub(start))
		go func() {
			select {
			case <-t.C:
				now := time.Now()
				l.emit(Rec{Kind: "probe", Name: "timer_job", Target: j.Format(time.RFC3339), LateS: wall(j, now).Seconds(), MonoDS: now.Sub(start).Seconds()})
			case <-stop:
			}
		}()
	}

	// SPEC 6.2 scheduler (1-minute check + wake events) and the same without wake events
	go runScheduler(l, "spec", jobs, wake, stop)
	go runScheduler(l, "tick_only", jobs, nil, stop)

	// keychain read every minute
	go func() {
		readKey("minute")
		t := time.NewTicker(time.Minute)
		for {
			select {
			case <-t.C:
				readKey("minute")
			case <-stop:
				return
			}
		}
	}()

	// graceful stop: Ctrl+C / console close, a stop file, or --for
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	poll := time.NewTicker(2 * time.Second)
	why := ""
	for why == "" {
		select {
		case s := <-sig:
			why = "signal " + s.String()
		case <-poll.C:
			if _, err := os.Stat(stopPath); err == nil {
				why = "stop file"
			} else if *forFlag > 0 && wall(start, time.Now()) >= *forFlag {
				why = "--for " + forFlag.String() + " reached"
			}
		}
	}
	close(stop)
	del := keyring.Delete(service, account)
	l.emit(Rec{Kind: "stop", Reason: why, Result: "keychain delete: " + errText(del), Desk: inputDesktop()})
	os.Remove(stopPath)
}

func dllOf(p interface{ Find() error }) string {
	if err := p.Find(); err != nil {
		return "missing (" + err.Error() + ")"
	}
	return "ok"
}

// runScheduler follows SPEC 6.2: check at start, on a 1-minute ticker aligned
// to the minute, and (if wake != nil) on every resume event. A scheduled time
// that passed without a run is run once as catch_up, even if several passed.
func runScheduler(l *logger, name string, jobs []time.Time, wake <-chan string, stop <-chan struct{}) {
	done := make([]bool, len(jobs))
	check := func(reason string) {
		now := time.Now()
		var due []int
		for i, j := range jobs {
			if !done[i] && !j.After(now) {
				due = append(due, i)
			}
		}
		if len(due) == 0 {
			if reason != "tick" {
				l.emit(Rec{Kind: "sched", Name: name, Reason: reason, Detail: "nothing due"})
			}
			return
		}
		trigger := "on_time"
		if len(due) > 1 || wall(jobs[due[0]], now) > onTimeLimit {
			trigger = "catch_up"
		}
		var covers []string
		for _, i := range due {
			done[i] = true
			covers = append(covers, jobs[i].Format(time.RFC3339))
		}
		l.emit(Rec{Kind: "sched", Name: name, Reason: reason, Trigger: trigger, Covers: covers, N: len(due),
			LateS: wall(jobs[due[0]], now).Seconds(), Detail: fmt.Sprintf("run with trigger %s for %d scheduled time(s)", trigger, len(due))})
	}
	check("start")
	now := time.Now()
	align := time.NewTimer(now.Truncate(time.Minute).Add(time.Minute + time.Second).Sub(now))
	var tick <-chan time.Time
	for {
		select {
		case <-align.C:
			tick = time.NewTicker(time.Minute).C
			check("tick")
		case <-tick:
			check("tick")
		case r := <-wake:
			check("wake: " + r)
		case <-stop:
			return
		}
	}
}
