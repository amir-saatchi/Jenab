// Deterministic test data: an assistant reply in Markdown, chat history,
// table rows (12 mixed columns) and chart points.
package main

import (
	"fmt"
	"math"
	"math/rand/v2"
	"regexp"
	"strings"
	"time"
)

var words = strings.Fields(`the pipeline reads prices from the exchange api and writes one row per coin
into the daily table while the view shows a chart of the last ninety days with a moving average and the
agent explains each step before it runs a query against the project database using a read only connection`)

func sentence(r *rand.Rand, n int) string {
	var b strings.Builder
	for i := range n {
		w := words[r.IntN(len(words))]
		if i == 0 {
			w = strings.ToUpper(w[:1]) + w[1:]
		}
		if i > 0 {
			b.WriteByte(' ')
		}
		if r.IntN(12) == 0 {
			w = "**" + w + "**"
		} else if r.IntN(15) == 0 {
			w = "`" + w + "`"
		}
		b.WriteString(w)
	}
	b.WriteByte('.')
	return b.String()
}

func para(r *rand.Rand) string {
	var s []string
	for range 2 + r.IntN(3) {
		s = append(s, sentence(r, 8+r.IntN(10)))
	}
	return strings.Join(s, " ")
}

const goCode = "```go\nfunc fetchPrices(ctx context.Context, coins []string) ([]Price, error) {\n\tout := make([]Price, 0, len(coins))\n\tfor _, c := range coins {\n\t\treq, err := http.NewRequestWithContext(ctx, \"GET\", apiURL+\"?coin=\"+c, nil)\n\t\tif err != nil {\n\t\t\treturn nil, fmt.Errorf(\"build request: %w\", err)\n\t\t}\n\t\tresp, err := client.Do(req)\n\t\tif err != nil {\n\t\t\treturn nil, err\n\t\t}\n\t\tvar p Price\n\t\tif err := json.NewDecoder(resp.Body).Decode(&p); err != nil {\n\t\t\tresp.Body.Close()\n\t\t\treturn nil, err\n\t\t}\n\t\tresp.Body.Close()\n\t\tout = append(out, p)\n\t}\n\treturn out, nil\n}\n```"

const tsCode = "```typescript\ninterface Row { id: number; coin: string; price: number; at: string }\n\nexport function movingAverage(rows: Row[], window = 7): number[] {\n  const out: number[] = [];\n  let sum = 0;\n  for (let i = 0; i < rows.length; i++) {\n    sum += rows[i].price;\n    if (i >= window) sum -= rows[i - window].price;\n    out.push(i >= window - 1 ? sum / window : NaN);\n  }\n  return out;\n}\n```"

const sqlCode = "```sql\nSELECT date(at) AS day, coin, avg(price) AS price\nFROM prices\nWHERE at >= :from_date AND (:coin IS NULL OR coin = :coin)\nGROUP BY day, coin\n```"

func table(r *rand.Rand) string {
	var b strings.Builder
	b.WriteString("| Coin | Price (USD) | Change 24h | Volume |\n|---|---:|---:|---:|\n")
	for _, c := range []string{"BTC", "ETH", "SOL", "ADA", "DOT", "XRP", "LTC", "DOGE"} {
		fmt.Fprintf(&b, "| %s | %.2f | %+.2f%% | %d |\n", c, 10+r.Float64()*60000, r.Float64()*10-5, r.IntN(1e9))
	}
	return strings.TrimRight(b.String(), "\n")
}

func list(r *rand.Rand, ordered bool) string {
	var s []string
	for i := range 4 + r.IntN(3) {
		m := "-"
		if ordered {
			m = fmt.Sprintf("%d.", i+1)
		}
		s = append(s, m+" "+sentence(r, 6+r.IntN(8)))
		if !ordered && r.IntN(3) == 0 {
			s = append(s, "  - "+sentence(r, 5))
		}
	}
	return strings.Join(s, "\n")
}

// reply builds an assistant answer of about target tokens with headings,
// paragraphs, lists, a table and code blocks.
func reply(seed uint64, target int) string {
	r := rand.New(rand.NewPCG(seed, 22))
	parts := []string{"## Plan for the price pipeline", para(r), list(r, true), para(r), goCode, para(r), table(r), "### Details", list(r, false), tsCode, para(r), sqlCode}
	i := 0
	for countTokens(strings.Join(parts, "\n\n")) < target {
		switch i % 5 {
		case 0:
			parts = append(parts, para(r))
		case 1:
			parts = append(parts, list(r, i%2 == 0))
		case 2:
			parts = append(parts, "### Step "+fmt.Sprint(i), para(r))
		case 3:
			parts = append(parts, tsCode)
		case 4:
			parts = append(parts, para(r))
		}
		i++
	}
	return strings.Join(parts, "\n\n")
}

// tokenRe splits text into LLM-like tokens: optional leading spaces + up to 4 non-space chars, or a newline run.
var tokenRe = regexp.MustCompile(`\n+|[ \t]*[^\s]{1,4}`)

func tokenize(s string) []string { return tokenRe.FindAllString(s, -1) }
func countTokens(s string) int   { return len(tokenize(s)) }

type HistMsg struct {
	ID   int    `json:"id"`
	Role string `json:"role"`
	Text string `json:"text"`
}

func history(n int) []HistMsg {
	r := rand.New(rand.NewPCG(7, 7))
	out := make([]HistMsg, 0, n)
	for i := range n {
		if i%2 == 0 {
			out = append(out, HistMsg{i, "user", sentence(r, 8+r.IntN(25))})
			continue
		}
		var parts []string
		parts = append(parts, para(r))
		switch r.IntN(4) {
		case 0:
			parts = append(parts, list(r, false), para(r))
		case 1:
			parts = append(parts, tsCode, para(r))
		case 2:
			parts = append(parts, table(r))
		case 3:
			parts = append(parts, para(r), list(r, true))
		}
		out = append(out, HistMsg{i, "assistant", strings.Join(parts, "\n\n")})
	}
	return out
}

// ---- table rows ----

var Columns = []string{"id", "name", "email", "city", "status", "amount", "qty", "ratio", "created", "updated", "score", "note"}

var (
	first    = []string{"Anna", "Ben", "Clara", "David", "Eva", "Felix", "Greta", "Hannes", "Ida", "Jonas", "Karla", "Lukas", "Mia", "Noah"}
	last     = []string{"Schmidt", "Meyer", "Wagner", "Becker", "Hoffmann", "Koch", "Richter", "Klein", "Wolf", "Neumann"}
	cities   = []string{"Berlin", "Hamburg", "Munich", "Cologne", "Frankfurt", "Stuttgart", "Leipzig", "Dresden", "Bremen", "Hanover"}
	statuses = []string{"active", "paused", "failed", "done"}
)

// row returns row i as an ordered slice; deterministic per index.
func row(i int) []any {
	r := rand.New(rand.NewPCG(uint64(i), 99))
	fn, ln := first[r.IntN(len(first))], last[r.IntN(len(last))]
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	created := base.Add(time.Duration(r.IntN(900*24)) * time.Hour)
	updated := created.Add(time.Duration(r.IntN(3600*24*30)) * time.Second)
	return []any{
		i + 1,
		fn + " " + ln,
		strings.ToLower(fn+"."+ln) + fmt.Sprintf("%d@example.com", r.IntN(100)),
		cities[r.IntN(len(cities))],
		statuses[r.IntN(len(statuses))],
		math.Round(r.Float64()*100000) / 100,
		r.IntN(500),
		math.Round(r.Float64()*10000) / 10000,
		created.Format("2006-01-02"),
		updated.Format(time.RFC3339),
		math.Round(r.NormFloat64()*1000) / 100,
		sentence(r, 4+r.IntN(6)),
	}
}

type PageArr struct {
	Offset  int      `json:"offset"`
	Total   int      `json:"total"`
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

type PageObj struct {
	Offset int              `json:"offset"`
	Total  int              `json:"total"`
	Rows   []map[string]any `json:"rows"`
}

const totalRows = 10000

func pageArr(offset, limit int) PageArr {
	p := PageArr{Offset: offset, Total: totalRows, Columns: Columns}
	for i := offset; i < min(offset+limit, totalRows); i++ {
		p.Rows = append(p.Rows, row(i))
	}
	return p
}

func pageObj(offset, limit int) PageObj {
	p := PageObj{Offset: offset, Total: totalRows}
	for i := offset; i < min(offset+limit, totalRows); i++ {
		vals := row(i)
		m := make(map[string]any, len(Columns))
		for j, c := range Columns {
			m[c] = vals[j]
		}
		p.Rows = append(p.Rows, m)
	}
	return p
}

// chart: x = epoch ms (hourly), y = random walk
type ChartData struct {
	X []int64   `json:"x"`
	Y []float64 `json:"y"`
}

func chartData(n int) ChartData {
	r := rand.New(rand.NewPCG(5, 5))
	d := ChartData{X: make([]int64, n), Y: make([]float64, n)}
	t := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	v := 30000.0
	for i := range n {
		v += r.NormFloat64() * 150
		d.X[i] = t + int64(i)*3600_000
		d.Y[i] = math.Round(v*100) / 100
	}
	return d
}
