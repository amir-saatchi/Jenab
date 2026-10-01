package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"
)

// judgments.txt: "source<space>qid<space>bits" where bits are 0/1 for the top results in order.
func loadJudgments() map[string]string {
	j := map[string]string{}
	f, err := os.Open("judgments.txt")
	if err != nil {
		return j
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if i := strings.Index(line, "#"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		fs := strings.Fields(line)
		if len(fs) == 3 {
			j[fs[0]+"|"+fs[1]] = fs[2]
		} else if len(fs) == 2 { // no results / nothing to judge
			j[fs[0]+"|"+fs[1]] = ""
		}
	}
	return j
}

// judged is the record whose top 5 is hand-judged: round 1 (or 0) of each source/query.
func judged(recs []Record) []Record {
	var out []Record
	for _, r := range recs {
		if r.Round <= 1 && r.QueryID != "feed" {
			out = append(out, r)
		}
	}
	return out
}

func sourceOrder(recs []Record) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range recs {
		if !seen[r.Source] {
			seen[r.Source] = true
			out = append(out, r.Source)
		}
	}
	rank := func(s string) int {
		for i, p := range []string{"searxng-local", "searxng-local-news", "searxng-public:", "searxng-public (all)", "ddg-instant-answer", "gdelt", "gdelt-7d", "gdelt-7d-slow", "wikipedia", "hn-algolia", "marginalia", "google-news-rss", "crypto-rss", "rss-feed:"} {
			if s == p || (strings.HasSuffix(p, ":") && strings.HasPrefix(s, p)) {
				return i
			}
		}
		return 99
	}
	sort.SliceStable(out, func(i, j int) bool { return rank(out[i]) < rank(out[j]) })
	return out
}

func writeDump(recs []Record) error {
	f, err := os.Create("top5.md")
	if err != nil {
		return err
	}
	defer f.Close()
	js := judged(recs)
	for _, q := range queries {
		fmt.Fprintf(f, "\n## %s %s\n", q.ID, q.Text)
		for _, s := range sourceOrder(js) {
			for _, r := range js {
				if r.Source != s || r.QueryID != q.ID {
					continue
				}
				fmt.Fprintf(f, "\n### %s (%s, n=%d)\n", s, r.Status, len(r.Results))
				for i, x := range r.Results {
					if i >= 5 {
						break
					}
					d := x.Date
					if d == "" {
						d = "-"
					} else {
						d = d[:10]
					}
					fmt.Fprintf(f, "%d. [%s] %s | %s | %s\n", i+1, d, x.Title, x.URL, trunc(x.Snippet, 110))
				}
			}
		}
	}
	return nil
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "..."
	}
	return s
}

func median(xs []int64) int64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]int64(nil), xs...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s[len(s)/2]
}

func pct(a, b int) string {
	if b == 0 {
		return "-"
	}
	return fmt.Sprintf("%d%%", (100*a+b/2)/b)
}

type stats struct {
	req, ok, blocked, errs int
	withResults            int
	counts                 []int64
	top10, dated, fresh    int
	relSum, relN, relQ     int
	judgedQ                int
	lat                    []int64
	retries                int
}

func compute(src string, recs []Record, j map[string]string) stats {
	var s stats
	for _, r := range recs {
		if r.Source != src {
			continue
		}
		s.req++
		s.retries += r.Retries
		if r.LatencyMs > 0 {
			s.lat = append(s.lat, r.LatencyMs)
		}
		switch r.Status {
		case "ok":
			s.ok++
		case "blocked":
			s.blocked++
			continue
		default:
			s.errs++
			continue
		}
		s.counts = append(s.counts, int64(len(r.Results)))
		if len(r.Results) > 0 {
			s.withResults++
		}
		for i, x := range r.Results {
			if i >= 10 {
				break
			}
			s.top10++
			if x.Date != "" {
				s.dated++
				if t, err := time.Parse(time.RFC3339, x.Date); err == nil && r.Time.Sub(t) <= 7*24*time.Hour {
					s.fresh++
				}
			}
		}
		if r.Round <= 1 {
			bits, ok := j[r.Source+"|"+r.QueryID]
			if ok || len(r.Results) == 0 { // an empty answer counts as a query with nothing relevant
				s.judgedQ++
				any := false
				for _, b := range bits {
					s.relN++
					if b == '1' {
						s.relSum++
						any = true
					}
				}
				if any {
					s.relQ++
				}
			}
		}
	}
	return s
}

func isGerman(s string) bool {
	t := " " + strings.ToLower(s) + " "
	if strings.ContainsAny(t, "äöüß") {
		return true
	}
	for _, w := range []string{" der ", " die ", " das ", " und ", " für ", " mit ", " zinsen ", " leitzins", " zins", " nicht ", " über ", " auf ", " eine ", " im "} {
		if strings.Contains(t, w) {
			return true
		}
	}
	return false
}

func isArabicScript(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Arabic, r) {
			return true
		}
	}
	return false
}

func writeReport(w io.Writer, recs []Record) {
	j := loadJudgments()
	all := recs
	// the public SearXNG instances are one row in the summary tables; section 3 has them one by one
	recs = nil
	npub := map[string]bool{}
	for _, r := range all {
		if strings.HasPrefix(r.Source, "searxng-public:") {
			npub[r.Source] = true
			r.Source = "searxng-public (all)"
		}
		recs = append(recs, r)
	}
	srcs := sourceOrder(recs)
	var first, last time.Time
	for _, r := range recs {
		if first.IsZero() || r.Time.Before(first) {
			first = r.Time
		}
		if r.Time.After(last) {
			last = r.Time
		}
	}
	fmt.Fprintf(w, "# SPIKE-020 results — keyless web search\n\nGenerated by `go run . -report > results.md` from `raw/*.json` (run %s to %s UTC) and `judgments.txt` (hand-judged top 5). Hand-written sections come from `notes.md`.\n\n",
		first.Format("2006-01-02 15:04"), last.Format("2006-01-02 15:04"))

	notes, _ := os.ReadFile("notes.md")
	parts := strings.SplitN(string(notes), "<!-- TABLES -->", 2)
	fmt.Fprintln(w, strings.TrimSpace(parts[0]))
	fmt.Fprintln(w)

	fmt.Fprintf(w, "## 1. Per source: measured numbers\n\n")
	fmt.Fprintf(w, "Queries: %d (en 8, de 1, fa 1). \"dated\" and \"≤7 d\" are shares of the top 10 results of each successful request that carry a publication date / a date within 7 days of the request. \"relevant\" is the hand-judged share of top-5 results that obviously answer the query (0/1 each), and in brackets the queries with at least one relevant result in the top 5. Latency is per HTTP request (RSS: per feed fetch).\n\n", len(queries))
	fmt.Fprintln(w, "| source | requests (+429 retries) | ok | blocked | errors | queries with results | median results | dated | ≤7 d | relevant top 5 | latency median / max |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|---|---|---|---|---|")
	for _, s := range srcs {
		if strings.HasPrefix(s, "rss-feed:") {
			continue
		}
		st := compute(s, recs, j)
		lat := st.lat
		if s == "crypto-rss" { // report the feed fetches, not the per-query copies
			lat = nil
			for _, r := range recs {
				if strings.HasPrefix(r.Source, "rss-feed:") {
					lat = append(lat, r.LatencyMs)
				}
			}
		}
		var mx int64
		for _, l := range lat {
			mx = max(mx, l)
		}
		rel := "-"
		if st.judgedQ > 0 {
			rel = fmt.Sprintf("%s (%d/%d)", pct(st.relSum, st.relN), st.relQ, st.judgedQ)
		}
		okq := st.ok
		if s == "searxng-local" {
			okq = st.ok // across rounds
		}
		fmt.Fprintf(w, "| %s | %d (+%d) | %d | %d | %d | %d/%d | %d | %s | %s | %s | %d / %d ms |\n",
			s, st.req, st.retries, st.ok, st.blocked, st.errs, st.withResults, okq, median(st.counts), pct(st.dated, st.top10), pct(st.fresh, st.top10), rel, median(lat), mx)
	}
	fmt.Fprintln(w)

	// Results per query.
	fmt.Fprintln(w, "### Result count per query (round 1)")
	fmt.Fprintln(w)
	hdr := "| source |"
	sep := "|---|"
	for _, q := range queries {
		hdr += " " + q.ID + " |"
		sep += "---|"
	}
	fmt.Fprintln(w, hdr)
	fmt.Fprintln(w, sep)
	for _, s := range srcs {
		if strings.HasPrefix(s, "rss-feed:") {
			continue
		}
		row := "| " + s + " |"
		for _, q := range queries {
			cell := " |"
			for _, r := range recs {
				if r.Source == s && r.QueryID == q.ID && r.Round <= 1 {
					if r.Status == "ok" {
						cell = fmt.Sprintf(" %d |", len(r.Results))
					} else {
						cell = " " + r.Status + " |"
					}
					break
				}
			}
			row += cell
		}
		fmt.Fprintln(w, row)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Queries:")
	fmt.Fprintln(w)
	for _, q := range queries {
		fmt.Fprintf(w, "- %s `%s` (%s, %s)\n", q.ID, q.Text, q.Lang, q.Topic)
	}
	fmt.Fprintln(w)

	// Language handling.
	fmt.Fprintln(w, "### Language handling (top 5 of the German and Persian query)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Automatic script/stop-word check on title+snippet, checked by hand in `top5.md`.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| source | q04 German results in top 5 | q05 Persian-script results in top 5 |")
	fmt.Fprintln(w, "|---|---|---|")
	for _, s := range srcs {
		if strings.HasPrefix(s, "rss-feed:") {
			continue
		}
		cells := []string{}
		for _, qc := range []struct {
			id string
			f  func(string) bool
		}{{"q04", isGerman}, {"q05", isArabicScript}} {
			cell := "-"
			for _, r := range recs {
				if r.Source == s && r.QueryID == qc.id && r.Round <= 1 && r.Status == "ok" {
					n, k := 0, 0
					for i, x := range r.Results {
						if i >= 5 {
							break
						}
						n++
						if qc.f(x.Title + " " + x.Snippet) {
							k++
						}
					}
					cell = fmt.Sprintf("%d/%d", k, n)
					break
				}
			}
			cells = append(cells, cell)
		}
		fmt.Fprintf(w, "| %s | %s | %s |\n", s, cells[0], cells[1])
	}
	fmt.Fprintln(w)

	writeEngineTable(w, recs)
	writePublicTable(w, all)
	writeDDG(w, recs)
	writeFeeds(w, recs)

	if len(parts) == 2 {
		fmt.Fprintln(w, strings.TrimSpace(parts[1]))
	}
}

func writeEngineTable(w io.Writer, recs []Record) {
	for _, cat := range []string{"searxng-local", "searxng-local-news"} {
		type es struct {
			answered int
			unresp   map[string]int
			perRound map[int]int
			results  int
		}
		eng := map[string]*es{}
		get := func(n string) *es {
			if eng[n] == nil {
				eng[n] = &es{unresp: map[string]int{}, perRound: map[int]int{}}
			}
			return eng[n]
		}
		n := 0
		rounds := 0
		for _, r := range recs {
			if r.Source != cat || r.Status != "ok" {
				continue
			}
			n++
			rounds = max(rounds, r.Round)
			seen := map[string]bool{}
			for _, x := range r.Results {
				for _, e := range x.Engines {
					get(e).results++
					if !seen[e] {
						seen[e] = true
						get(e).answered++
					}
				}
			}
			for _, u := range r.Unresponsive {
				if len(u) >= 2 {
					e := get(u[0])
					e.unresp[u[1]]++
					e.perRound[r.Round]++
				}
			}
		}
		if n == 0 {
			if cat == "searxng-local" {
				fmt.Fprint(w, "## 2. SearXNG local — upstream engines\n\nNot run: Docker Desktop failed to start on the test machine (see notes). No data.\n\n")
			}
			continue
		}
		names := make([]string, 0, len(eng))
		for k := range eng {
			names = append(names, k)
		}
		sort.Slice(names, func(i, j int) bool {
			if eng[names[i]].answered != eng[names[j]].answered {
				return eng[names[i]].answered > eng[names[j]].answered
			}
			return names[i] < names[j]
		})
		fmt.Fprintf(w, "## 2. SearXNG local — upstream engines (%s, %d successful requests, %d round(s))\n\n", cat, n, rounds)
		fmt.Fprintln(w, "\"answered\" = requests where the engine contributed at least one result. \"unresponsive\" = SearXNG listed the engine in `unresponsive_engines`, with its reason.")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "| engine | answered | results contributed | unresponsive | reasons | unresponsive per round |")
		fmt.Fprintln(w, "|---|---|---|---|---|---|")
		for _, k := range names {
			e := eng[k]
			tot := 0
			var rs []string
			for reason, c := range e.unresp {
				tot += c
				rs = append(rs, fmt.Sprintf("%s ×%d", reason, c))
			}
			sort.Strings(rs)
			pr := []string{}
			for i := 1; i <= rounds; i++ {
				pr = append(pr, fmt.Sprint(e.perRound[i]))
			}
			fmt.Fprintf(w, "| %s | %d/%d | %d | %d/%d | %s | %s |\n", k, e.answered, n, e.results, tot, n, strings.Join(rs, ", "), strings.Join(pr, " / "))
		}
		fmt.Fprintln(w)
	}
}

func writePublicTable(w io.Writer, recs []Record) {
	type ps struct {
		req, ok, blocked, errs, results int
		lat                             []int64
		detail                          string
	}
	m := map[string]*ps{}
	var order []string
	for _, r := range recs {
		if !strings.HasPrefix(r.Source, "searxng-public:") {
			continue
		}
		p := m[r.Source]
		if p == nil {
			p = &ps{}
			m[r.Source] = p
			order = append(order, r.Source)
		}
		p.req++
		p.lat = append(p.lat, r.LatencyMs)
		switch r.Status {
		case "ok":
			p.ok++
			p.results += len(r.Results)
		case "blocked":
			p.blocked++
			if p.detail == "" {
				p.detail = r.Detail
			}
		default:
			p.errs++
			if p.detail == "" {
				p.detail = r.Detail
			}
		}
	}
	if len(order) == 0 {
		return
	}
	fmt.Fprintln(w, "## 3. Public SearXNG instances (format=json, ≤10 requests each, ≥4 s apart)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| instance | requests | JSON ok | blocked | errors | results total | median latency | first refusal |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|---|---|")
	for _, k := range order {
		p := m[k]
		fmt.Fprintf(w, "| %s | %d | %d | %d | %d | %d | %d ms | %s |\n", strings.TrimPrefix(k, "searxng-public:"), p.req, p.ok, p.blocked, p.errs, p.results, median(p.lat), strings.ReplaceAll(trunc(p.detail, 120), "|", "/"))
	}
	fmt.Fprintln(w)
}

func writeDDG(w io.Writer, recs []Record) {
	fmt.Fprintln(w, "## 4. DuckDuckGo Instant Answer API, what came back")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| query | results | first result | response |")
	fmt.Fprintln(w, "|---|---|---|---|")
	for _, r := range recs {
		if r.Source != "ddg-instant-answer" {
			continue
		}
		f := "-"
		if len(r.Results) > 0 {
			f = trunc(r.Results[0].URL, 70)
		}
		fmt.Fprintf(w, "| %s | %d | %s | %s |\n", r.QueryID, len(r.Results), f, strings.ReplaceAll(trunc(r.Extra+" "+r.Detail, 100), "|", "/"))
	}
	fmt.Fprintln(w)
}

func writeFeeds(w io.Writer, recs []Record) {
	fmt.Fprintln(w, "## 5. RSS feeds fetched (crypto-rss is a local keyword filter over these)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| feed | status | items | dated | newest | oldest | latency |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|---|")
	for _, r := range recs {
		if !strings.HasPrefix(r.Source, "rss-feed:") {
			continue
		}
		d, newest, oldest := 0, "", ""
		for _, x := range r.Results {
			if x.Date == "" {
				continue
			}
			d++
			if newest == "" || x.Date > newest {
				newest = x.Date
			}
			if oldest == "" || x.Date < oldest {
				oldest = x.Date
			}
		}
		fmt.Fprintf(w, "| %s | %s %s | %d | %d | %s | %s | %d ms |\n", strings.TrimPrefix(r.Source, "rss-feed:"), r.Status, trunc(r.Detail, 60), len(r.Results), d, newest, oldest, r.LatencyMs)
	}
	fmt.Fprintln(w)
}
