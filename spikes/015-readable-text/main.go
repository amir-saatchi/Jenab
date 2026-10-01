// Command readable scores Go readable-text libraries on the saved fixtures.
//
//	go run . > results.md      # no network
//	go run ./fetch             # refetch fixtures
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/metrics"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	dumpDir   = flag.String("dump", "", "write every extraction to this directory (for writing phrases)")
	skipSpeed = flag.Bool("nospeed", false, "skip timing sections")
	child     = flag.Bool("child", false, "internal: run one extraction and report memory")
)

func main() {
	flag.Parse()
	if *child {
		runChild(flag.Args())
		return
	}
	fx, err := loadFixtures()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *dumpDir != "" {
		dump(fx)
		return
	}
	report(fx)
}

func dump(fx []*Fixture) {
	_ = os.MkdirAll(*dumpDir, 0o755)
	for _, f := range fx {
		_ = os.WriteFile(filepath.Join(*dumpDir, f.ID+".visible.txt"), []byte(f.Visible), 0o644)
		for i, e := range extractors {
			r := e.Run(f.UTF8, f.URL, true)
			md, _ := toMarkdown(r.Node, f.URL)
			if i == len(extractors)-1 {
				md = r.Text
			}
			name := strings.NewReplacer(" ", "_", "+", "_", "(", "", ")", "", "→", "").Replace(e.Name)
			out := fmt.Sprintf("TITLE: %s\nERR: %v\n\n%s\n\n===== MARKDOWN =====\n%s", r.Title, r.Err, r.Text, md)
			_ = os.WriteFile(filepath.Join(*dumpDir, f.ID+"."+name+".txt"), []byte(out), 0o644)
		}
		fmt.Fprintln(os.Stderr, "dumped", f.ID)
	}
}

// ---------------------------------------------------------------------------

func report(fx []*Fixture) {
	p := func(format string, a ...any) { fmt.Printf(format+"\n", a...) }
	p("# SPIKE-015 results")
	p("")
	p("%s, %s/%s, %d CPUs, %s. %d fixtures (%d fetched, %d derived). Input to every library is the page after Go decoded it to UTF-8, unless a section says otherwise.",
		runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), time.Now().Format("2006-01-02"),
		len(fx), countFetched(fx), len(fx)-countFetched(fx))
	p("")
	p("Libraries: %s.", strings.Join(moduleVersions(), ", "))
	p("")

	checkPhrases(fx, p)

	// ---- run everything once ----
	scores := make([][]Score, len(extractors)) // [extractor][fixture]
	for i, e := range extractors {
		scores[i] = make([]Score, len(fx))
		for j, f := range fx {
			r := e.Run(f.UTF8, f.URL, true)
			md := r.Text
			if i != len(extractors)-1 {
				md, _ = toMarkdown(r.Node, f.URL)
			}
			scores[i][j] = scorePage(f, r, md, i == len(extractors)-1)
		}
	}

	summaryTable(fx, scores, p)
	perPageTable(fx, scores, p)
	tableSection(fx, scores, p)
	languageSection(fx, scores, p)
	encodingSection(fx, p)
	jsSection(fx, scores, p)
	if !*skipSpeed {
		speedSection(fx, p)
		memorySection(fx, p)
	}
	selectorSection(fx, p)
	missesSection(fx, scores, p)
	p("")
	p("## Manual notes")
	p("")
	p("%s", strings.TrimSpace(manualNotes))
}

func countFetched(fx []*Fixture) int {
	n := 0
	for _, f := range fx {
		if f.From == "" {
			n++
		}
	}
	return n
}

func moduleVersions() []string {
	want := []string{
		"github.com/markusmobius/go-trafilatura/v2", "codeberg.org/readeck/go-readability/v2",
		"github.com/go-shiori/go-readability", "github.com/markusmobius/go-domdistiller",
		"github.com/JohannesKaufmann/html-to-markdown/v2", "github.com/PuerkitoBio/goquery",
		"github.com/andybalholm/cascadia", "golang.org/x/net",
	}
	bi, ok := debug.ReadBuildInfo()
	var out []string
	for _, w := range want {
		v := "?"
		if ok {
			for _, d := range bi.Deps {
				if d.Path == w {
					v = d.Version
				}
			}
		}
		out = append(out, "`"+w+"` "+v)
	}
	return out
}

func checkPhrases(fx []*Fixture, p func(string, ...any)) {
	var bad []string
	nMust, nNot := 0, 0
	for _, f := range fx {
		nMust += len(f.Must)
		nNot += len(f.MustNot)
		for _, s := range append(append([]string{}, f.Must...), f.MustNot...) {
			if !strings.Contains(f.Visible, normalize(s)) {
				bad = append(bad, fmt.Sprintf("%s: %q", f.ID, s))
			}
		}
		for _, row := range f.Row {
			for _, c := range row {
				if !strings.Contains(f.Visible, normalize(c)) {
					bad = append(bad, fmt.Sprintf("%s: row cell %q", f.ID, c))
				}
			}
		}
	}
	p("Labels: %d main-text phrases, %d boilerplate phrases. Every phrase was checked against the visible text of the whole page (script and style removed): %d not found.", nMust, nNot, len(bad))
	for _, b := range bad {
		p("- phrase not on page: %s", b)
	}
	p("")
}

func summaryTable(fx []*Fixture, sc [][]Score, p func(string, ...any)) {
	p("## 1. Summary")
	p("")
	p("Main text = main-text phrases found. Boilerplate = boilerplate phrases that leaked into the text (lower is better). Title = extracted title contains the expected title. Links = pages where at least one link came back / pages scored. Empty = pages with fewer than 200 characters of text or an error. RTL = Persian pages where all phrases (including the ones with a ZWNJ) are found and no U+FFFD. Size = total text characters over all pages.")
	p("")
	p("| Library | Main text | Boilerplate | Title | Links | Empty/error | RTL (fa) | Size (text) | Size (Markdown) |")
	p("|---|---|---|---|---|---|---|---|---|")
	for i, e := range extractors {
		var hit, tot, leak, ltot, title, ttot, withLinks, empty, rtl, rtot, chars, mdchars int
		for j, f := range fx {
			s := sc[i][j]
			hit += s.MustHit
			tot += s.MustTotal
			leak += s.Leaks
			ltot += s.LeakTotal
			if f.Title != "" {
				ttot++
				if s.TitleOK {
					title++
				}
			}
			if s.Links > 0 {
				withLinks++
			}
			if s.Err || s.Chars < 200 {
				empty++
			}
			if f.Lang == "fa" {
				rtot++
				if s.RTLOk {
					rtl++
				}
			}
			chars += s.Chars
			mdchars += s.MDChars
		}
		p("| %s | %d of %d | %d of %d | %d of %d | %d of %d | %d | %d of %d | %s | %s |",
			e.Name, hit, tot, leak, ltot, title, ttot, withLinks, len(fx), empty, rtl, rtot, kchars(chars), kchars(mdchars))
	}
	p("")
}

func kchars(n int) string {
	return strconv.FormatFloat(float64(n)/1e6, 'f', 2, 64) + " M"
}

func shortName(i int) string {
	return []string{"traf", "traf+fb", "readeck", "shiori", "distiller", "whole"}[i]
}

func perPageTable(fx []*Fixture, sc [][]Score, p func(string, ...any)) {
	p("## 2. Per page")
	p("")
	p("Cell: main-text phrases found / total, then boilerplate leaks as `-n`, `T` if the title is right, `E` for an error, `∅` for under 200 characters. Columns: traf = trafilatura, traf+fb = trafilatura with fallback, readeck = readeck v2, shiori = go-shiori, distiller = domdistiller, whole = whole page to Markdown.")
	p("")
	head := "| Page | Group | Lang | KB |"
	sep := "|---|---|---|---|"
	for i := range extractors {
		head += " " + shortName(i) + " |"
		sep += "---|"
	}
	p("%s", head)
	p("%s", sep)
	for j, f := range fx {
		row := fmt.Sprintf("| %s | %s | %s | %d |", f.ID, f.Group, f.Lang, len(f.Raw)/1024)
		for i := range extractors {
			s := sc[i][j]
			if s.Err {
				row += " E |"
				continue
			}
			c := fmt.Sprintf("%d/%d", s.MustHit, s.MustTotal)
			if s.Leaks > 0 {
				c += fmt.Sprintf(" -%d", s.Leaks)
			}
			if s.TitleOK {
				c += " T"
			}
			if s.Chars < 200 {
				c += " ∅"
			}
			row += " " + c + " |"
		}
		p("%s", row)
	}
	p("")
}

func tableSection(fx []*Fixture, sc [][]Score, p func(string, ...any)) {
	p("## 3. Tables")
	p("")
	p("For each table page, a few rows are labelled by their cells. Cell: rows whose cells all stay on one line in the plain text / in the Markdown output (html-to-markdown with the table plugin, run on the library's content node).")
	p("")
	head := "| Page | Rows |"
	sep := "|---|---|"
	for i := range extractors {
		head += " " + shortName(i) + " |"
		sep += "---|"
	}
	p("%s", head)
	p("%s", sep)
	for j, f := range fx {
		if len(f.Row) == 0 {
			continue
		}
		row := fmt.Sprintf("| %s | %d |", f.ID, len(f.Row))
		for i := range extractors {
			s := sc[i][j]
			if s.Err {
				row += " E |"
				continue
			}
			row += fmt.Sprintf(" %d / %d |", s.TextRows, s.MDRows)
		}
		p("%s", row)
	}
	p("")
}

func languageSection(fx []*Fixture, sc [][]Score, p func(string, ...any)) {
	p("## 4. By language")
	p("")
	p("Main-text phrases found / boilerplate leaks, per language (legacy-encoding copies included).")
	p("")
	langs := []string{"en", "de", "fa", "ja"}
	head := "| Library |"
	sep := "|---|"
	for _, l := range langs {
		n := 0
		for _, f := range fx {
			if f.Lang == l {
				n++
			}
		}
		head += fmt.Sprintf(" %s (%d pages) |", l, n)
		sep += "---|"
	}
	p("%s", head)
	p("%s", sep)
	for i, e := range extractors {
		row := "| " + e.Name + " |"
		for _, l := range langs {
			var hit, tot, leak int
			for j, f := range fx {
				if f.Lang == l {
					hit += sc[i][j].MustHit
					tot += sc[i][j].MustTotal
					leak += sc[i][j].Leaks
				}
			}
			row += fmt.Sprintf(" %d of %d / %d |", hit, tot, leak)
		}
		p("%s", row)
	}
	p("")
}

func encodingSection(fx []*Fixture, p func(string, ...any)) {
	p("## 5. Encodings")
	p("")
	p("Raw = the bytes as served, handed to the library (it cannot see the HTTP header). Go = decoded first by Go with `x/net/html/charset` (HTTP header, BOM, `<meta>`), then handed over as UTF-8. Cell: main-text phrases found / total, `M` if the text has U+FFFD or mis-decoded sequences.")
	p("")
	head := "| Page | Header charset | Meta charset | Go chose |"
	sep := "|---|---|---|---|"
	for i := range extractors[:len(extractors)-1] {
		head += " " + shortName(i) + " raw | " + shortName(i) + " Go |"
		sep += "---|---|"
	}
	p("%s", head)
	p("%s", sep)
	for _, f := range fx {
		if f.Group != "legacy" {
			continue
		}
		row := fmt.Sprintf("| %s | %s | %s | %s |", f.ID, dash(f.Meta.HeaderCharset), dash(f.Meta.MetaCharset), f.Charset)
		for _, e := range extractors[:len(extractors)-1] {
			for _, utf8in := range []bool{false, true} {
				in := f.Raw
				if utf8in {
					in = f.UTF8
				}
				s := scorePage(f, e.Run(in, f.URL, utf8in), "", false)
				c := fmt.Sprintf("%d/%d", s.MustHit, s.MustTotal)
				if s.Err {
					c = "E"
				} else if s.Mojibake {
					c += " M"
				}
				row += " " + c + " |"
			}
		}
		p("%s", row)
	}
	p("")
	// Raw input on the UTF-8 pages: does the library's own detection ever break them?
	p("Raw input on the %d non-legacy pages (all UTF-8): pages where the raw result finds fewer phrases than the Go-decoded result, or has mis-decoded text.", len(fx)-countGroup(fx, "legacy"))
	p("")
	p("| Library | Pages worse with raw input |")
	p("|---|---|")
	for _, e := range extractors[:len(extractors)-1] {
		var worse []string
		for _, f := range fx {
			if f.Group == "legacy" {
				continue
			}
			a := scorePage(f, e.Run(f.Raw, f.URL, false), "", false)
			b := scorePage(f, e.Run(f.UTF8, f.URL, true), "", false)
			if a.MustHit < b.MustHit || (a.Mojibake && !b.Mojibake) || (a.Err && !b.Err) {
				worse = append(worse, f.ID)
			}
		}
		p("| %s | %d %s |", e.Name, len(worse), strings.Join(worse, ", "))
	}
	p("")
}

func countGroup(fx []*Fixture, g string) int {
	n := 0
	for _, f := range fx {
		if f.Group == g {
			n++
		}
	}
	return n
}

func dash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func jsSection(fx []*Fixture, sc [][]Score, p func(string, ...any)) {
	p("## 6. JavaScript-heavy pages")
	p("")
	p("Script share = bytes inside `<script>` / page bytes. Visible = characters of visible text on the whole page. Other cells: characters of extracted text.")
	p("")
	head := "| Page | KB | Script share | Visible |"
	sep := "|---|---|---|---|"
	for i := range extractors[:len(extractors)-1] {
		head += " " + shortName(i) + " |"
		sep += "---|"
	}
	p("%s", head)
	p("%s", sep)
	for j, f := range fx {
		if f.Group != "js" {
			continue
		}
		row := fmt.Sprintf("| %s | %d | %.0f%% | %d |", f.ID, len(f.Raw)/1024, 100*scriptShare(f.UTF8), utf8.RuneCountInString(f.Visible))
		for i := range extractors[:len(extractors)-1] {
			row += fmt.Sprintf(" %d |", sc[i][j].Chars)
		}
		p("%s", row)
	}
	p("")
}

// ---------------------------------------------------------------------------
// Speed

type timing struct {
	ns     float64
	allocs float64
	bytes  float64
}

func measure(run func()) timing {
	var runs int
	var ms0, ms1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&ms0)
	var times []float64
	start := time.Now()
	for runs == 0 || (runs < 5 && time.Since(start) < 300*time.Millisecond) {
		t := time.Now()
		run()
		times = append(times, float64(time.Since(t).Nanoseconds()))
		runs++
	}
	runtime.ReadMemStats(&ms1)
	sort.Float64s(times)
	return timing{
		ns:     times[len(times)/2],
		allocs: float64(ms1.Mallocs-ms0.Mallocs) / float64(runs),
		bytes:  float64(ms1.TotalAlloc-ms0.TotalAlloc) / float64(runs),
	}
}

func speedSection(fx []*Fixture, p func(string, ...any)) {
	p("## 7. Speed and allocations")
	p("")
	p("Per page: median of up to 5 runs after one warm-up run (pages over 4 MB: no warm-up). Then p50 and max over the %d pages. Allocated = bytes allocated per extraction (not peak memory). The Markdown row converts each library's content node; its numbers are for the readeck v2 output.", len(fx))
	p("")
	p("| Library | Time p50 | Time max (page) | Allocs p50 | Allocated p50 | Allocated max |")
	p("|---|---|---|---|---|---|")
	type row struct {
		name string
		ts   []timing
		ids  []string
	}
	var rows []row
	for _, e := range extractors {
		r := row{name: e.Name}
		for _, f := range fx {
			if len(f.UTF8) <= 4<<20 {
				e.Run(f.UTF8, f.URL, true)
			}
			t := measure(func() { e.Run(f.UTF8, f.URL, true) })
			r.ts = append(r.ts, t)
			r.ids = append(r.ids, f.ID)
		}
		rows = append(rows, r)
	}
	md := row{name: "html-to-markdown on readeck v2 output (step only)"}
	for _, f := range fx {
		res := runReadeck(f.UTF8, f.URL, true)
		t := measure(func() { _, _ = toMarkdown(res.Node, f.URL) })
		md.ts = append(md.ts, t)
		md.ids = append(md.ids, f.ID)
	}
	rows = append(rows, md)
	for _, r := range rows {
		ns := make([]float64, len(r.ts))
		al := make([]float64, len(r.ts))
		by := make([]float64, len(r.ts))
		maxI := 0
		for i, t := range r.ts {
			ns[i], al[i], by[i] = t.ns, t.allocs, t.bytes
			if t.ns > r.ts[maxI].ns {
				maxI = i
			}
		}
		p("| %s | %s | %s (%s) | %s | %s | %s |", r.name, dur(pct(ns, 50)), dur(r.ts[maxI].ns), r.ids[maxI],
			count(pct(al, 50)), mb(pct(by, 50)), mb(maxOf(by)))
	}
	p("")
	// Largest pages in detail.
	p("Largest pages, time per extraction:")
	p("")
	head := "| Page | MB |"
	sep := "|---|---|"
	for _, r := range rows[:len(extractors)] {
		head += " " + r.name + " |"
		sep += "---|"
	}
	p("%s", head)
	p("%s", sep)
	for j, f := range fx {
		if len(f.UTF8) < 1<<20 {
			continue
		}
		line := fmt.Sprintf("| %s | %.1f |", f.ID, float64(len(f.UTF8))/(1<<20))
		for _, r := range rows[:len(extractors)] {
			line += " " + dur(r.ts[j].ns) + " |"
		}
		p("%s", line)
	}
	p("")
}

func pct(v []float64, q int) float64 {
	s := append([]float64{}, v...)
	sort.Float64s(s)
	return s[(len(s)-1)*q/100]
}

func maxOf(v []float64) float64 {
	m := 0.0
	for _, x := range v {
		if x > m {
			m = x
		}
	}
	return m
}

func dur(ns float64) string {
	switch {
	case ns >= 1e9:
		return fmt.Sprintf("%.2f s", ns/1e9)
	default:
		return fmt.Sprintf("%.1f ms", ns/1e6)
	}
}

func mb(b float64) string { return fmt.Sprintf("%.1f MB", b/(1<<20)) }

func count(n float64) string {
	if n >= 1e6 {
		return fmt.Sprintf("%.2f M", n/1e6)
	}
	return fmt.Sprintf("%.0f k", n/1e3)
}

// ---------------------------------------------------------------------------
// Peak memory, measured in a fresh child process per library and page.

type childOut struct {
	FirstMs  float64 `json:"first_ms"`
	SecondMs float64 `json:"second_ms"`
	PeakMB   float64 `json:"peak_mb"`
	TotalMB  float64 `json:"total_mb"`
	Chars    int     `json:"chars"`
	Err      string  `json:"err"`
}

func runChild(args []string) {
	idx, _ := strconv.Atoi(args[0])
	limit, _ := strconv.Atoi(args[2])
	fx, err := loadFixturesOnly(args[1])
	if err != nil {
		panic(err)
	}
	var f *Fixture
	for _, x := range fx {
		if x.ID == args[1] {
			f = x
		}
	}
	in := f.UTF8
	if limit > 0 && len(in) > limit {
		in = in[:limit]
	}
	// Drop everything loadFixtures kept except the input.
	fx = nil
	f.Raw, f.UTF8, f.Visible = nil, nil, ""
	runtime.GC()
	debug.FreeOSMemory()

	samples := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}, {Name: "/memory/classes/total:bytes"}}
	metrics.Read(samples)
	base := samples[0].Value.Uint64()
	var peak uint64
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		s := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}}
		for {
			metrics.Read(s)
			if v := s[0].Value.Uint64(); v > peak {
				peak = v
			}
			select {
			case <-stop:
				close(done)
				return
			case <-time.After(time.Millisecond):
			}
		}
	}()
	e := extractors[idx]
	t := time.Now()
	r := e.Run(in, f.URL, true)
	first := time.Since(t)
	close(stop)
	<-done
	metrics.Read(samples)
	total := samples[1].Value.Uint64()
	t = time.Now()
	e.Run(in, f.URL, true)
	second := time.Since(t)
	out := childOut{
		FirstMs:  float64(first.Microseconds()) / 1000,
		SecondMs: float64(second.Microseconds()) / 1000,
		PeakMB:   float64(peak-min(peak, base)) / (1 << 20),
		TotalMB:  float64(total) / (1 << 20),
		Chars:    utf8.RuneCountInString(r.Text),
	}
	if r.Err != nil {
		out.Err = r.Err.Error()
	}
	_ = json.NewEncoder(os.Stdout).Encode(out)
}

func memorySection(fx []*Fixture, p func(string, ...any)) {
	p("## 8. Peak memory and first call")
	p("")
	p("Each cell is a fresh child process that loads one page and runs one extraction. Peak = highest live heap above the starting heap, sampled every 1 ms. Total = memory the Go runtime holds from the OS after the run. First = the first call in the process (includes lazy setup such as regex compilation and model loading); second = a repeat call. `large-whatwg-html` is 14.9 MB; the 5 MB row cuts the same page at the SPEC 6.6 response limit.")
	p("")
	exe, err := os.Executable()
	if err != nil {
		p("cannot find own executable: %v", err)
		return
	}
	cases := []struct {
		id    string
		limit int
		label string
	}{
		{"blog-go-slices", 0, "blog-go-slices (44 KB)"},
		{"forum-hn", 0, "forum-hn (3.3 MB)"},
		{"large-whatwg-html", 5 << 20, "large-whatwg-html cut at 5 MB"},
		{"large-whatwg-html", 0, "large-whatwg-html (14.9 MB)"},
	}
	p("| Page | Library | First | Second | Peak heap | Total from OS | Text chars |")
	p("|---|---|---|---|---|---|---|")
	for _, c := range cases {
		for i, e := range extractors {
			cmd := exec.Command(exe, "-child", strconv.Itoa(i), c.id, strconv.Itoa(c.limit))
			cmd.Stderr = os.Stderr
			b, err := cmd.Output()
			var o childOut
			if err == nil {
				err = json.Unmarshal(b, &o)
			}
			if err != nil {
				p("| %s | %s | failed: %v | | | | |", c.label, e.Name, err)
				continue
			}
			note := ""
			if o.Err != "" {
				note = " (error: " + o.Err + ")"
			}
			p("| %s | %s | %s | %s | %.0f MB | %.0f MB | %d%s |", c.label, e.Name, dur(o.FirstMs*1e6), dur(o.SecondMs*1e6), o.PeakMB, o.TotalMB, o.Chars, note)
		}
	}
	p("")
}

func missesSection(fx []*Fixture, sc [][]Score, p func(string, ...any)) {
	p("## 10. Missed and leaked phrases (trafilatura, readeck v2)")
	p("")
	for _, i := range []int{0, 2} {
		p("**%s**", extractors[i].Name)
		p("")
		for j, f := range fx {
			s := sc[i][j]
			if s.Err {
				p("- %s: error", f.ID)
				continue
			}
			if len(s.MissedMust) == 0 && len(s.LeakedNot) == 0 {
				continue
			}
			var parts []string
			if len(s.MissedMust) > 0 {
				parts = append(parts, "missed "+quoteAll(s.MissedMust))
			}
			if len(s.LeakedNot) > 0 {
				parts = append(parts, "leaked "+quoteAll(s.LeakedNot))
			}
			p("- %s: %s", f.ID, strings.Join(parts, "; "))
		}
		p("")
	}
}

func quoteAll(ss []string) string {
	q := make([]string, len(ss))
	for i, s := range ss {
		q[i] = strconv.Quote(s)
	}
	return strings.Join(q, ", ")
}
