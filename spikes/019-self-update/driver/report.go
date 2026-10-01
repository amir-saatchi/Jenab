package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"burrow/spikes/selfupdate/internal/spike"
)

func msBetween(a, b time.Time) float64 {
	if a.IsZero() || b.IsZero() {
		return -1
	}
	return float64(b.Sub(a).Microseconds()) / 1000
}

// post derives the per-scenario facts from the log.
func post(r *Run) {
	oldVer := ""
	if e := r.find("main-entered", func(e spike.Entry) bool { return int(e.F("pid")) == r.OldPID }); e != nil {
		oldVer = e.S("ver")
	}
	r.fact("oldVersion", oldVer)
	r.fact("oldExitCode", r.OldExitCode)
	for _, ev := range []string{"check-result", "install-result", "restart-returned", "db-refused", "db-migrated", "db-backup", "db-created", "db-opened", "still-alive", "provider-error"} {
		if e := r.find(ev, nil); e != nil {
			m := map[string]any{}
			for k, v := range e {
				if k != "t" && k != "pid" && k != "sc" && k != "ev" {
					m[k] = v
				}
			}
			m["ver"] = e.S("ver")
			r.fact(ev, m)
		}
	}
	var errs []string
	for _, e := range r.all("go-saw") {
		if e.S("event") == "wails:updater:error" {
			b, _ := json.Marshal(e["data"])
			errs = append(errs, string(b))
		}
	}
	if len(errs) > 0 {
		r.fact("errorEvents", errs)
	}
	// New version process (not helper, not child, not the old one).
	newMain := r.find("main-entered", func(e spike.Entry) bool {
		return int(e.F("pid")) != r.OldPID && e.S("helperEnv") == "" && !isChild(e) && e.S("ver") != oldVer
	})
	if newMain != nil {
		nv := newMain.S("ver")
		pid := newMain.F("pid")
		r.fact("newVersion", nv)
		r.fact("newArgs", newMain["args"])
		byPid := func(ev string) spike.Entry {
			return r.find(ev, func(e spike.Entry) bool { return e.F("pid") == pid })
		}
		d := map[string]float64{"exitToMain": msBetween(r.OldExit, newMain.T())}
		if e := byPid("app-started"); e != nil {
			d["exitToAppStarted"] = msBetween(r.OldExit, e.T())
		}
		if e := byPid("window-ready"); e != nil {
			d["exitToWindowReady"] = msBetween(r.OldExit, e.T())
		}
		if e := r.find("restart-called", nil); e != nil {
			d["restartCallToExit"] = msBetween(e.T(), r.OldExit)
		}
		if e := r.find("main-entered", func(e spike.Entry) bool { return e.S("helperEnv") == "1" }); e != nil {
			if rc := r.find("restart-called", nil); rc != nil {
				d["restartCallToHelperMain"] = msBetween(rc.T(), e.T())
			}
		}
		r.fact("timing", d)
	}
	if hl, ok := r.HelperLogs[fmt.Sprintf("wails-update-%d.log", r.OldPID)]; ok {
		r.fact("helperLog", strings.TrimSpace(hl))
	}
	for k, v := range r.HelperLogs {
		if k != fmt.Sprintf("wails-update-%d.log", r.OldPID) {
			r.fact("otherHelperLog "+k, strings.TrimSpace(v))
		}
	}
	r.fact("installDirAfter", r.InstallDir)
	r.fact("tempNew", r.TempNew)
	r.fact("exitCodes", r.ExitCodes)
	if len(r.Killed) > 0 {
		r.fact("killedAtSettle", r.Killed)
	}
	// Children.
	if r.S.Cfg.Child != "" {
		type cb struct {
			Ver                 string
			Total, AfterExit    int
			LastBeatAfterExitMs float64
			Exe                 string
			ExitLogged          bool
		}
		per := map[int]*cb{}
		for _, b := range r.all("child-beat") {
			pid := int(b.F("pid"))
			c := per[pid]
			if c == nil {
				c = &cb{Ver: b.S("ver")}
				per[pid] = c
			}
			c.Total++
			if b.T().After(r.OldExit) {
				c.AfterExit++
				c.LastBeatAfterExitMs = msBetween(r.OldExit, b.T())
				c.Exe = b.S("exe")
			}
		}
		for _, e := range r.all("child-exit") {
			if c := per[int(e.F("pid"))]; c != nil {
				c.ExitLogged = true
			}
		}
		r.fact("childBeats", per)
	}
	// Scheduler.
	if jr := r.all("job-run"); len(jr) > 0 {
		var js []map[string]any
		for _, e := range jr {
			js = append(js, map[string]any{"ver": e.S("ver"), "trigger": e.S("trigger"), "lateMs": e.F("lateMs"), "due": e.S("due"), "at": e.S("t")})
		}
		r.fact("jobRuns", js)
	}
	// Data.
	r.fact("db", inspectDB(filepath.Join(dataDir(dataName(r)), "app.db")))
	// Feed requests.
	reqs := feedRequests(r.S.Name)
	r.fact("feedRequests", len(reqs))
	dump(r)
}

func dataName(r *Run) string {
	if r.S.KeepData != "" {
		return r.S.KeepData
	}
	return r.S.Name
}

func isChild(e spike.Entry) bool {
	args, ok := e["args"].([]any)
	return ok && len(args) > 0 && args[0] == "--child"
}

func inspectDB(p string) map[string]any {
	out := map[string]any{}
	if _, err := os.Stat(p); err != nil {
		out["exists"] = false
		return out
	}
	out["sha"] = fileHash(p)[:16]
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(p)+"?mode=ro")
	if err != nil {
		out["err"] = err.Error()
		return out
	}
	defer db.Close()
	var uv int
	_ = db.QueryRow("PRAGMA user_version").Scan(&uv)
	out["userVersion"] = uv
	var ic string
	_ = db.QueryRow("PRAGMA integrity_check").Scan(&ic)
	out["integrity"] = ic
	rows, err := db.Query("SELECT name FROM pragma_table_info('items')")
	if err == nil {
		var cols []string
		for rows.Next() {
			var c string
			_ = rows.Scan(&c)
			cols = append(cols, c)
		}
		rows.Close()
		out["itemsColumns"] = cols
	}
	var n int
	_ = db.QueryRow("SELECT count(*) FROM items").Scan(&n)
	out["items"] = n
	bs, _ := filepath.Glob(filepath.Join(filepath.Dir(p), "backups", "*.db"))
	var bl []string
	for _, b := range bs {
		bdb, err := sql.Open("sqlite", "file:"+filepath.ToSlash(b)+"?mode=ro")
		v := -1
		if err == nil {
			_ = bdb.QueryRow("PRAGMA user_version").Scan(&v)
			bdb.Close()
		}
		bl = append(bl, fmt.Sprintf("%s (%s, user_version %d)", filepath.Base(b), human(size(b)), v))
	}
	out["backups"] = bl
	return out
}

func feedRequests(sc string) []spike.Entry {
	var out []spike.Entry
	for _, e := range spike.ReadLog(filepath.Join(outDir, "feed.jsonl")) {
		if e.S("sc") == sc {
			out = append(out, e)
		}
	}
	return out
}

func dump(r *Run) {
	b, _ := json.MarshalIndent(map[string]any{"name": r.S.Name, "group": r.S.Group, "facts": r.Facts}, "", "  ")
	f, err := os.OpenFile(filepath.Join(outDir, "runs.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err == nil {
		c, _ := json.Marshal(map[string]any{"name": r.S.Name, "group": r.S.Group, "facts": r.Facts})
		f.Write(append(c, '\n'))
		f.Close()
	}
	_ = os.WriteFile(filepath.Join(outDir, "logs", r.S.Name+".facts.json"), b, 0o644)
}

// ---- summary ----

func writeSummary() {
	var sb strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&sb, f, a...) }
	w("# SPIKE-019 driver summary (%s)\n\n", time.Now().Format(time.RFC3339))

	w("## Results per scenario\n\n| Scenario | old → new | check | install | restart | error event(s) | still alive / exe unchanged | old exit code | feed reqs | staging dirs left |\n|---|---|---|---|---|---|---|---|---|---|\n")
	for _, r := range allRuns {
		f := r.Facts
		get := func(k, sub string) string {
			if m, ok := f[k].(map[string]any); ok {
				if v, ok := m[sub]; ok {
					return fmt.Sprint(v)
				}
			}
			return ""
		}
		check := get("check-result", "result")
		if e := get("check-result", "err"); e != "" {
			check += ": " + e
		}
		inst := get("install-result", "result")
		if e := get("install-result", "err"); e != "" {
			inst += ": " + e
		}
		errs := ""
		if v, ok := f["errorEvents"].([]string); ok {
			errs = strings.Join(v, "; ")
		}
		alive := ""
		if a, ok := f["still-alive"].(map[string]any); ok {
			alive = fmt.Sprintf("rows %v, exe unchanged %v", a["dbRows"], a["exeUnchanged"])
		}
		staging := 0
		if t, ok := f["tempNew"].([]string); ok {
			staging = len(t)
		}
		w("| %s | %v → %v | %s | %s | %s | %s | %s | %v | %v | %d |\n", r.S.Name, f["oldVersion"], orDash(f["newVersion"]), esc(check), esc(inst),
			esc(get("restart-returned", "err")), esc(errs), alive, f["oldExitCode"], f["feedRequests"], staging)
	}

	w("\n## Downtime (old process exit → new process), ms\n\n| Variant | runs | exit→main() median/max | exit→ApplicationStarted median/max | exit→page ready median/max | Restart()→old exit median/max |\n|---|---|---|---|---|---|\n")
	for _, g := range []string{"downtime-same", "downtime-fresh"} {
		var a, b, c, d []float64
		for _, r := range allRuns {
			if r.S.Group != g {
				continue
			}
			if t, ok := r.Facts["timing"].(map[string]float64); ok {
				a = append(a, t["exitToMain"])
				b = append(b, t["exitToAppStarted"])
				c = append(c, t["exitToWindowReady"])
				d = append(d, t["restartCallToExit"])
			}
		}
		w("| %s | %d | %s | %s | %s | %s |\n", g, len(a), mm(a), mm(b), mm(c), mm(d))
	}
	w("\n| Run | exit→main | exit→app started | exit→page ready | Restart()→old exit | Restart()→helper main |\n|---|---|---|---|---|---|\n")
	for _, r := range allRuns {
		if t, ok := r.Facts["timing"].(map[string]float64); ok {
			w("| %s | %.0f | %.0f | %.0f | %.0f | %.0f |\n", r.S.Name, t["exitToMain"], t["exitToAppStarted"], t["exitToWindowReady"], t["restartCallToExit"], t["restartCallToHelperMain"])
		}
	}

	w("\n## Swap failures, kills, children, data, scheduler (raw facts)\n\n")
	for _, r := range allRuns {
		switch r.S.Group {
		case "lock", "kill", "next", "swap", "child", "safe", "data":
		default:
			continue
		}
		w("### %s\n\n", r.S.Name)
		keys := []string{"lock", "lockReleasedAt", "killHelper", "restart-returned", "newVersion", "timing", "installDirAfter", "tempNew", "helperLog", "exitCodes", "killedAtSettle", "childBeats", "jobRuns", "db", "db-refused", "db-backup", "db-migrated", "check-result", "install-result"}
		for k := range r.Facts {
			if strings.HasPrefix(k, "otherHelperLog") {
				keys = append(keys, k)
			}
		}
		for _, k := range keys {
			if v, ok := r.Facts[k]; ok {
				b, _ := json.Marshal(v)
				if k == "helperLog" || strings.HasPrefix(k, "otherHelperLog") {
					w("- %s:\n```\n%s\n```\n", k, v)
					continue
				}
				w("- %s: `%s`\n", k, string(b))
			}
		}
		w("\n")
	}

	// Event timeline of a1.
	for _, r := range allRuns {
		if r.S.Name != "a1-endpoint-http" && r.S.Name != "d1-busy-then-catch-up" && r.S.Name != "a8-endpoint-slow-download-5MBps" {
			continue
		}
		w("## Event timeline %s (ms after the first event)\n\n| t | pid/ver | event | detail |\n|---|---|---|---|\n", r.S.Name)
		var t0 time.Time
		lastProgress := map[string]int{}
		for _, e := range r.Entries {
			if t0.IsZero() {
				t0 = e.T()
			}
			ev := e.S("ev")
			det := ""
			switch ev {
			case "go-saw":
				det = e.S("event")
				if det == "wails:updater:download-progress" {
					lastProgress["go"]++
					if lastProgress["go"] > 3 {
						continue
					}
				}
				b, _ := json.Marshal(e["data"])
				det += " " + trunc(string(b), 120)
			case "js-saw":
				m, _ := e["data"].(map[string]any)
				det = fmt.Sprint(m["name"])
				if jt, ok := m["jsTime"].(float64); ok {
					det += fmt.Sprintf(" (JS clock %+.0f ms)", msBetween(t0, time.UnixMilli(int64(jt))))
				}
				if det == "wails:updater:download-progress" {
					lastProgress["js"]++
					if lastProgress["js"] > 3 {
						continue
					}
				}
			case "child-beat":
				continue
			default:
				b, _ := json.Marshal(stripStd(e))
				det = trunc(string(b), 160)
			}
			w("| %.0f | %v/%s | %s | %s |\n", msBetween(t0, e.T()), e["pid"], e.S("ver"), ev, esc(det))
		}
		w("\n(progress events: Go saw %d, JS saw %d in total)\n\n", count(r, "go-saw", "wails:updater:download-progress"), count(r, "js-saw", "wails:updater:download-progress"))
	}

	// What the check sends.
	w("## Requests seen by the feed\n\n| Scenario | # | method | url | headers |\n|---|---|---|---|---|\n")
	for _, r := range allRuns {
		if r.S.Group != "feed" && r.S.Group != "privacy" && r.S.Name != "b01-good-signed" {
			continue
		}
		for i, e := range feedRequests(r.S.Name) {
			b, _ := json.Marshal(e["headers"])
			w("| %s | %d | %s | %s | %s |\n", r.S.Name, i+1, e.S("method"), esc(e.S("url")), esc(string(b)))
		}
	}
	_ = os.WriteFile(filepath.Join(outDir, "summary.md"), []byte(sb.String()), 0o644)
	logf("wrote out/summary.md and out/runs.jsonl")
}

func count(r *Run, ev, name string) int {
	n := 0
	for _, e := range r.all(ev) {
		if ev == "go-saw" && e.S("event") == name {
			n++
		}
		if ev == "js-saw" {
			if m, ok := e["data"].(map[string]any); ok && m["name"] == name {
				n++
			}
		}
	}
	return n
}

func stripStd(e spike.Entry) map[string]any {
	m := map[string]any{}
	for k, v := range e {
		switch k {
		case "t", "pid", "ver", "sc", "ev":
		default:
			m[k] = v
		}
	}
	return m
}

func mm(v []float64) string {
	if len(v) == 0 {
		return "-"
	}
	s := append([]float64{}, v...)
	sort.Float64s(s)
	var med float64
	if len(s)%2 == 1 {
		med = s[len(s)/2]
	} else {
		med = (s[len(s)/2-1] + s[len(s)/2]) / 2
	}
	return fmt.Sprintf("%.0f / %.0f", med, s[len(s)-1])
}

func orDash(v any) any {
	if v == nil {
		return "-"
	}
	return v
}

func esc(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "|", "\\|"), "\n", " ") }

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
