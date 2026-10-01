// Runner for SPIKE-022: builds the frontend (bun) and the Wails app, measures
// the bundle, runs every scenario N times (one fresh app process per run),
// and writes out/summary.md and out/summary.json.
//
//	go run ./cmd/runner                 # everything, 3 runs
//	go run ./cmd/runner -only chat -runs 1
//	go run ./cmd/runner -summary        # only re-aggregate out/*.json
package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type scenario struct {
	Name    string
	Query   string
	Timeout int
	Fresh   bool // delete the WebView2 profile before each run
	Group   string
}

func scenarios() []scenario {
	var s []scenario
	s = append(s, scenario{"idle-fresh", "s=idle", 60, true, "idle"}, scenario{"idle", "s=idle", 60, false, "idle"})
	for _, rate := range []int{100, 1000} {
		for _, mm := range [][2]string{{"token", "rm"}, {"sync", "rm"}, {"raf", "rm"}, {"token", "rmb"}, {"raf", "rmb"}, {"token", "marked"}, {"raf", "marked"}, {"raf", "markedn"}, {"t50", "rm"}, {"t50", "rmb"}} {
			s = append(s, scenario{fmt.Sprintf("chat-%d-%s-%s", rate, mm[0], mm[1]), fmt.Sprintf("s=chat&rate=%d&mode=%s&md=%s", rate, mm[0], mm[1]), 180, false, "chat"})
		}
		s = append(s, scenario{fmt.Sprintf("chat-%d-token-rm-hist0", rate), fmt.Sprintf("s=chat&rate=%d&mode=token&md=rm&hist=0", rate), 180, false, "chat"})
	}
	s = append(s, scenario{"table", "s=table", 240, false, "table"})
	for _, v := range []string{"line", "line-opt", "line-lttb2000", "line-lttb1000", "bar", "bar-opt", "bar-agg1000", "uplot-line", "uplot-bar"} {
		s = append(s, scenario{"chart-" + v, "s=chart&v=" + v, 180, false, "chart"})
	}
	s = append(s, scenario{"wails", "s=wails", 300, false, "wails"})
	return s
}

var (
	runs    = flag.Int("runs", 3, "runs per scenario")
	only    = flag.String("only", "", "regexp: only scenarios whose name matches")
	noBuild = flag.Bool("skip-build", false, "use the existing bin/spike022.exe")
	sumOnly = flag.Bool("summary", false, "only aggregate existing results")
	missing = flag.Bool("missing", false, "only run scenario/run pairs without a result file")
	outDir  = flag.String("out", "out", "output directory")
)

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func sh(dir, name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Dir, cmd.Stdout, cmd.Stderr = dir, os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "VITE_CONFIG_NATIVE_IGNORE_WARNING=true")
	log.Printf("$ %s %s", name, strings.Join(args, " "))
	must(cmd.Run())
}

func main() {
	flag.Parse()
	log.SetFlags(log.Ltime)
	must(os.MkdirAll(*outDir, 0o755))
	if !*sumOnly {
		if !*noBuild {
			if _, err := os.Stat("frontend/node_modules"); err != nil {
				sh("frontend", "bun", "install", "--frozen-lockfile")
			}
			sh("frontend", "bun", "run", "build")
			sh(".", "go", "build", "-tags", "production", "-ldflags", "-H=windowsgui", "-o", "bin/spike022.exe", ".")
		}
		bundle()
		var re *regexp.Regexp
		if *only != "" {
			re = regexp.MustCompile(*only)
		}
		for _, sc := range scenarios() {
			if re != nil && !re.MatchString(sc.Name) {
				continue
			}
			for r := 1; r <= *runs; r++ {
				if _, err := os.Stat(filepath.Join(*outDir, fmt.Sprintf("%s-r%d.json", sc.Name, r))); *missing && err == nil {
					continue
				}
				// a start that fails within seconds (seen: WebView2 profile still held by the previous run) is retried
				for try := 0; try < 3 && !runOne(sc, r); try++ {
					time.Sleep(3 * time.Second)
				}
				time.Sleep(1500 * time.Millisecond)
			}
		}
	}
	summarise()
}

func runOne(sc scenario, r int) bool {
	name := fmt.Sprintf("%s-r%d", sc.Name, r)
	args := []string{"-name", name, "-q", sc.Query, "-out", *outDir, "-timeout", fmt.Sprint(sc.Timeout)}
	if sc.Fresh {
		dir := filepath.Join(*outDir, "wv-fresh")
		os.RemoveAll(dir)
		args = append(args, "-wvdata", dir)
	}
	os.Remove(filepath.Join(*outDir, name+".json"))
	cmd := exec.Command("bin/spike022.exe", args...)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	start := time.Now()
	must(cmd.Start())
	pid := uint32(cmd.Process.Pid)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		log.Printf("%-28s %5.1fs exit=%v", name, time.Since(start).Seconds(), err)
	case <-time.After(time.Duration(sc.Timeout+30) * time.Second):
		kids := descendants(pid)
		cmd.Process.Kill()
		for k := range kids {
			if h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, k); err == nil {
				windows.TerminateProcess(h, 1)
				windows.CloseHandle(h)
			}
		}
		log.Printf("%-28s killed after timeout (pid %d, %d children)", name, pid, len(kids))
	}
	// the webview processes exit shortly after the host; wait for them so runs do not overlap
	for i := 0; i < 50 && len(descendants(pid)) > 0; i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if _, err := os.Stat(filepath.Join(*outDir, name+".json")); err != nil {
		log.Printf("  no result; stderr: %s", strings.TrimSpace(errb.String()))
		os.Rename(filepath.Join(*outDir, name+".log"), filepath.Join(*outDir, fmt.Sprintf("%s.failed-%d.log", name, time.Now().Unix())))
		return false
	}
	return true
}

func descendants(root uint32) map[uint32]bool {
	out := map[uint32]bool{}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return out
	}
	defer windows.CloseHandle(snap)
	parent := map[uint32]uint32{}
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		parent[e.ProcessID] = e.ParentProcessID
	}
	in := map[uint32]bool{root: true}
	for changed := true; changed; {
		changed = false
		for p, pp := range parent {
			if !in[p] && in[pp] && p != pp {
				in[p], out[p], changed = true, true, true
			}
		}
	}
	return out
}

// ---------- bundle ----------

type chunk struct {
	File    string `json:"file"`
	Raw     int    `json:"raw"`
	Gzip    int    `json:"gzip"`
	Imports []string
}

var importRe = regexp.MustCompile(`(?:import|export)\s*(?:[\w$*{},\s]*?\bfrom\s*)?["']\./([\w.\-]+\.js)["']`)
var cssRefRe = regexp.MustCompile(`assets/([\w.\-]+\.css)`)

func gz(b []byte) int {
	var buf bytes.Buffer
	w, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	w.Write(b)
	w.Close()
	return buf.Len()
}

func bundle() {
	dir := "frontend/dist/assets"
	ents, err := os.ReadDir(dir)
	must(err)
	chunks := map[string]*chunk{}
	for _, e := range ents {
		b, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		c := &chunk{File: e.Name(), Raw: len(b), Gzip: gz(b)}
		if strings.HasSuffix(e.Name(), ".js") {
			for _, m := range importRe.FindAllStringSubmatch(string(b), -1) {
				c.Imports = append(c.Imports, m[1])
			}
		}
		chunks[e.Name()] = c
	}
	find := func(prefix, ext string) string {
		for n := range chunks {
			if strings.HasPrefix(n, prefix+"-") && strings.HasSuffix(n, ext) {
				return n
			}
		}
		return ""
	}
	closure := func(root string) map[string]bool {
		seen := map[string]bool{}
		var walk func(string)
		walk = func(n string) {
			if n == "" || seen[n] {
				return
			}
			seen[n] = true
			if c := chunks[n]; c != nil {
				for _, i := range c.Imports {
					walk(i)
				}
			}
		}
		walk(root)
		return seen
	}
	html, _ := os.ReadFile("frontend/dist/index.html")
	sizeOf := func(set map[string]bool) (raw, g int) {
		for n := range set {
			if c := chunks[n]; c != nil {
				raw += c.Raw
				g += c.Gzip
			}
		}
		return
	}
	shell := closure(find("index", ".js"))
	shell[find("index", ".css")] = true
	minus := func(a, b map[string]bool) map[string]bool {
		out := map[string]bool{}
		for k := range a {
			if !b[k] {
				out[k] = true
			}
		}
		return out
	}
	union := func(sets ...map[string]bool) map[string]bool {
		out := map[string]bool{}
		for _, s := range sets {
			for k := range s {
				out[k] = true
			}
		}
		return out
	}
	withCSS := func(set map[string]bool, css ...string) map[string]bool {
		for _, c := range css {
			if n := find(c, ".css"); n != "" {
				set[n] = true
			}
		}
		return set
	}
	groups := map[string]map[string]bool{
		"shell (React, Zustand, shadcn, Tailwind CSS, Wails glue)": shell,
		"chat scenario code":                               withCSS(minus(closure(find("chat", ".js")), shell), "chat"),
		"react-markdown + remark-gfm + rehype-highlight":   minus(closure(find("md-rm", ".js")), union(shell, closure(find("chat", ".js")))),
		"marked + highlight.js core (7 langs) + DOMPurify": minus(closure(find("md-marked", ".js")), union(shell, closure(find("chat", ".js")))),
		"table (TanStack Table + Virtual)":                 minus(closure(find("table", ".js")), shell),
		"chart (Recharts)":                                 minus(closure(find("chart", ".js")), shell),
		"uPlot":                                            withCSS(minus(closure(find("uPlot.esm", ".js")), shell), "uPlot"),
	}
	type row struct {
		Group string   `json:"group"`
		Raw   int      `json:"raw"`
		Gzip  int      `json:"gzip"`
		Files []string `json:"files"`
	}
	var rows []row
	for g, set := range groups {
		raw, gzz := sizeOf(set)
		var files []string
		for f := range set {
			files = append(files, f)
		}
		sort.Strings(files)
		rows = append(rows, row{g, raw, gzz, files})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Group < rows[j].Group })
	fonts := 0
	for n, c := range chunks {
		if strings.HasSuffix(n, ".woff2") {
			fonts += c.Raw
		}
	}
	full := union(shell, closure(find("chat", ".js")), closure(find("md-rm", ".js")), closure(find("table", ".js")), closure(find("chart", ".js")))
	noCharts := minus(full, groups["chart (Recharts)"])
	noMD := minus(full, groups["react-markdown + remark-gfm + rehype-highlight"])
	noBoth := minus(noCharts, groups["react-markdown + remark-gfm + rehype-highlight"])
	totals := map[string][2]int{}
	for k, s := range map[string]map[string]bool{"app: shell+chat(react-markdown)+table+chart": full, "without Recharts": noCharts, "without markdown": noMD, "without both": noBoth, "shell only": shell} {
		r, g := sizeOf(s)
		totals[k] = [2]int{r, g}
	}
	res := map[string]any{"groups": rows, "totals": totals, "fontsWoff2Bytes": fonts, "indexHtmlBytes": len(html), "chunks": chunks}
	b, _ := json.MarshalIndent(res, "", " ")
	must(os.WriteFile(filepath.Join(*outDir, "bundle.json"), b, 0o644))
	log.Printf("bundle: %v", totals)
}

// ---------- summary ----------

type metric struct{ Label, Path string }

var metricSets = map[string][]metric{
	"idle": {
		{"proc→first paint ms", "page.startup.procToShellPaintMs"},
		{"proc→nav start ms", "page.startup.procToNavStartMs"},
		{"nav→first visible ms", "page.startup.navToFirstVisibleMs"},
		{"proc→runtime ready ms", "page.startup.procToRuntimeReadyMs"},
		{"WebView2 WS MB", "memAtEnd.webviewWorkingSetMB"},
		{"WebView2 private MB", "memAtEnd.webviewPrivateMB"},
		{"total WS MB", "memAtEnd.totalWorkingSetMB"},
		{"total private MB", "memAtEnd.totalPrivateMB"},
		{"JS heap MB", "page.jsHeapMB"},
		{"webview procs", "memAtEnd.webviewProcesses"},
		{"sys CPU %", "cpu.whole.sysMeanPct"},
	},
	"chat": {
		{"fps", "page.result.frames.fps"},
		{"frame p99 ms", "page.result.frames.p99"},
		{"frames >18ms %", "page.result.frames.pctOver18"},
		{"frames >33.4ms %", "page.result.frames.pctOver33_4"},
		{"frame max ms", "page.result.frames.max"},
		{"long tasks", "page.result.blocking.longTasks"},
		{"long task max ms", "page.result.blocking.longTaskMaxMs"},
		{"long tasks >100ms", "page.result.blocking.longTasksOver100"},
		{"LoAF max ms", "page.result.blocking.loafMaxMs"},
		{"commits", "page.result.render.commits"},
		{"commit p50 ms", "page.result.render.commitMs.p50"},
		{"commit max ms", "page.result.render.commitMs.max"},
		{"key→paint p50 ms", "page.result.input.realKeyToPaintMs.p50"},
		{"key→paint max ms", "page.result.input.realKeyToPaintMs.max"},
		{"keys", "page.result.input.trustedKeys"},
		{"timer→paint p50 ms", "page.result.input.synthEventToPaintMs.p50"},
		{"timer→paint max ms", "page.result.input.synthEventToPaintMs.max"},
		{"ET max input delay ms", "page.result.input.eventTimingMaxInputDelayMs"},
		{"emit→recv p50 ms", "page.result.stream.emitToReceiveMs.p50"},
		{"emit→recv p99 ms", "page.result.stream.emitToReceiveMs.p99"},
		{"missing", "page.result.stream.missing"},
		{"out of order", "page.result.stream.outOfOrder"},
		{"dupes", "page.result.stream.dupes"},
		{"text intact", "page.result.stream.textIntact"},
		{"stream ms", "page.result.stream.durationMs"},
		{"history render ms", "page.result.history.renderMs"},
		{"total WS MB", "memAtEnd.totalWorkingSetMB"},
		{"total private MB", "memAtEnd.totalPrivateMB"},
		{"JS heap MB", "page.jsHeapMB"},
		{"sys CPU % (stream)", "cpu.stream.sysMeanPct"},
		{"app tree CPU % of core", "cpu.stream.treeMeanCorePct"},
	},
	"table": {
		{"page call p50 ms", "page.result.load.callMs.p50"},
		{"page call max ms", "page.result.load.callMs.max"},
		{"page bytes", "page.result.load.bytesPerPage.p50"},
		{"append→paint p50 ms", "page.result.load.appendToPaintMs.p50"},
		{"append→paint max ms", "page.result.load.appendToPaintMs.max"},
		{"load 10k total ms", "page.result.load.totalMs"},
		{"scroll fps", "page.result.scroll.frames.fps"},
		{"scroll frame p99 ms", "page.result.scroll.frames.p99"},
		{"scroll >18ms %", "page.result.scroll.frames.pctOver18"},
		{"scroll >33.4ms %", "page.result.scroll.frames.pctOver33_4"},
		{"scroll frame max ms", "page.result.scroll.frames.max"},
		{"scroll long tasks", "page.result.scroll.blocking.longTasks"},
		{"scroll long task max", "page.result.scroll.blocking.longTaskMaxMs"},
		{"blank viewport checks", "page.result.scroll.blankViewport"},
		{"sort amount asc paint", "page.result.sortFilter.sort amount asc.paintMs.p50"},
		{"sort name asc paint", "page.result.sortFilter.sort name asc.paintMs.p50"},
		{"sort created desc paint", "page.result.sortFilter.sort created desc.paintMs.p50"},
		{"sort updated asc paint", "page.result.sortFilter.sort updated asc.paintMs.p50"},
		{"global 'a' paint", "page.result.sortFilter.global 'a'.paintMs.p50"},
		{"global 'ann' paint", "page.result.sortFilter.global 'ann'.paintMs.p50"},
		{"amount range paint", "page.result.sortFilter.amount 100-500.paintMs.p50"},
		{"status = paint", "page.result.sortFilter.status = active.paintMs.p50"},
		{"sort+filter paint", "page.result.sortFilter.sort amount + global 'an'.paintMs.p50"},
		{"worst sort/filter paint", "page.result.sortFilterWorstPaintMs"},
		{"total WS MB", "memAtEnd.totalWorkingSetMB"},
		{"total private MB", "memAtEnd.totalPrivateMB"},
		{"JS heap MB", "page.jsHeapMB"},
		{"sys CPU % (scroll)", "cpu.scroll.sysMeanPct"},
	},
	"chart": {
		{"points", "page.result.points"},
		{"downsample ms", "page.result.downsampleMs"},
		{"first paint ms", "page.result.mount.firstPaintMs"},
		{"commit ms", "page.result.mount.commitMs"},
		{"mount long task max", "page.result.mount.blocking3s.longTaskMaxMs"},
		{"mount LoAF max", "page.result.mount.blocking3s.loafMaxMs"},
		{"mount 3s fps", "page.result.mount.frames3s.fps"},
		{"resize p50 ms", "page.result.resize.ms.p50"},
		{"resize max ms", "page.result.resize.ms.max"},
		{"resize misses", "page.result.resize.misses"},
		{"hover p50 ms", "page.result.hover.ms.p50"},
		{"hover max ms", "page.result.hover.ms.max"},
		{"hover misses", "page.result.hover.misses"},
		{"sweep fps", "page.result.hover.sweepFrames.fps"},
		{"sweep >33.4ms %", "page.result.hover.sweepFrames.pctOver33_4"},
		{"sweep long task max", "page.result.hover.sweepBlocking.longTaskMaxMs"},
		{"DOM nodes", "page.result.mount.domNodes"},
		{"total WS MB", "memAtEnd.totalWorkingSetMB"},
		{"total private MB", "memAtEnd.totalPrivateMB"},
		{"JS heap MB", "page.jsHeapMB"},
		{"sys CPU % (mount)", "cpu.mount.sysMeanPct"},
	},
}

func lookup(v any, path string) (float64, bool) {
	cur := v
	for _, k := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return 0, false
		}
		cur = m[k]
	}
	switch x := cur.(type) {
	case float64:
		return x, true
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

func median(xs []float64) float64 {
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

func fmtN(x float64) string {
	switch {
	case math.Abs(x) >= 100:
		return fmt.Sprintf("%.0f", x)
	case math.Abs(x) >= 10:
		return fmt.Sprintf("%.1f", x)
	default:
		return fmt.Sprintf("%.2f", x)
	}
}

func summarise() {
	files, _ := filepath.Glob(filepath.Join(*outDir, "*-r[0-9]*.json"))
	byScenario := map[string][]map[string]any{}
	runRe := regexp.MustCompile(`-r\d+$`)
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var m map[string]any
		if json.Unmarshal(b, &m) != nil {
			continue
		}
		name := runRe.ReplaceAllString(strings.TrimSuffix(filepath.Base(f), ".json"), "")
		byScenario[name] = append(byScenario[name], m)
	}
	var md strings.Builder
	all := map[string]any{}
	fmt.Fprintf(&md, "# SPIKE-022 summary (generated %s)\n\nEach cell: median (max) over runs.\n", time.Now().Format(time.RFC3339))
	for _, sc := range []string{"idle", "chat", "table", "chart"} {
		ms := metricSets[sc]
		var names []string
		for _, s := range scenarios() {
			if s.Group == sc && byScenario[s.Name] != nil {
				names = append(names, s.Name)
			}
		}
		if len(names) == 0 {
			continue
		}
		fmt.Fprintf(&md, "\n## %s\n\n| metric |", sc)
		for _, n := range names {
			fmt.Fprintf(&md, " %s |", n)
		}
		md.WriteString("\n|---|")
		for range names {
			md.WriteString("---|")
		}
		md.WriteString("\n")
		fmt.Fprintf(&md, "| runs |")
		for _, n := range names {
			fmt.Fprintf(&md, " %d |", len(byScenario[n]))
		}
		md.WriteString("\n")
		for _, m := range ms {
			fmt.Fprintf(&md, "| %s |", m.Label)
			for _, n := range names {
				var xs []float64
				for _, r := range byScenario[n] {
					if x, ok := lookup(r, m.Path); ok {
						xs = append(xs, x)
					}
				}
				if len(xs) == 0 {
					md.WriteString(" – |")
					continue
				}
				mx := xs[0]
				for _, x := range xs {
					mx = math.Max(mx, x)
				}
				med := median(xs)
				fmt.Fprintf(&md, " %s (%s) |", fmtN(med), fmtN(mx))
				key := n + "|" + m.Label
				all[key] = map[string]any{"median": med, "max": mx, "values": xs}
			}
			md.WriteString("\n")
		}
	}
	// wails: nested maps, print per run
	if rs := byScenario["wails"]; len(rs) > 0 {
		md.WriteString("\n## wails\n\n")
		type wm struct{ label, path string }
		var ws []wm
		for _, k := range []string{"noop", "echo1k", "rows500arr", "rows500obj", "rows5000arr", "history200", "chart10k"} {
			ws = append(ws, wm{"binding " + k + " p50 ms", "page.result.bindings." + k + ".ms.p50"}, wm{"binding " + k + " p95 ms", "page.result.bindings." + k + ".ms.p95"}, wm{"binding " + k + " bytes", "page.result.bindings." + k + ".bytes"})
		}
		ws = append(ws, wm{"Go side rows500 build ms p50", "page.result.bindings.rows500goSide.genMs.p50"}, wm{"Go side rows500 marshal ms p50", "page.result.bindings.rows500goSide.marshalMs.p50"}, wm{"Go side rows500 JSON bytes", "page.result.bindings.rows500goSide.bytes"})
		ws = append(ws, wm{"10 concurrent rows500 ms", "page.result.bindings.rows500x10concurrentMs"}, wm{"rows500 via event p50 ms", "page.result.rowsEvent.ms.p50"}, wm{"rows500 via event p95 ms", "page.result.rowsEvent.ms.p95"})
		for _, tr := range []string{"floods", "streams"} {
			for _, rate := range []string{"100", "1000", "2000", "5000", "10000", "20000", "max"} {
				p := "page.result." + tr + "." + rate + "."
				for _, f := range [][2]string{{"goAchievedHz", "Go Hz"}, {"recvHz", "recv Hz"}, {"missing", "missing"}, {"outOfOrder", "out of order"}, {"latencyMs.p50", "lat p50"}, {"latencyMs.p99", "lat p99"}, {"latencyMs.max", "lat max"}, {"drainLagMs", "drain lag ms"}, {"goEmitMaxMs", "Go emit max ms"}, {"frames.fps", "fps"}} {
					ws = append(ws, wm{tr + " " + rate + "/s " + f[1], p + f[0]})
				}
			}
		}
		md.WriteString("| metric | median (max) |\n|---|---|\n")
		for _, w := range ws {
			var xs []float64
			for _, r := range rs {
				if x, ok := lookup(r, w.path); ok {
					xs = append(xs, x)
				}
			}
			if len(xs) == 0 {
				continue
			}
			mx := xs[0]
			for _, x := range xs {
				mx = math.Max(mx, x)
			}
			fmt.Fprintf(&md, "| %s | %s (%s) |\n", w.label, fmtN(median(xs)), fmtN(mx))
			all["wails|"+w.label] = map[string]any{"median": median(xs), "max": mx, "values": xs}
		}
	}
	must(os.WriteFile(filepath.Join(*outDir, "summary.md"), []byte(md.String()), 0o644))
	b, _ := json.MarshalIndent(all, "", " ")
	must(os.WriteFile(filepath.Join(*outDir, "summary.json"), b, 0o644))
	log.Printf("wrote %s", filepath.Join(*outDir, "summary.md"))
}
