package main

// Deterministic test data, from SPIKE-022: an assistant reply in Markdown
// and chat history, so the numbers compare with the spike's.

import (
	"fmt"
	"math/rand/v2"
	"regexp"
	"strings"
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

// historyMessage is message i of a seeded chat: user questions and
// Markdown answers in turn.
func historyMessage(r *rand.Rand, i int) string {
	if i%2 == 0 {
		return sentence(r, 8+r.IntN(25))
	}
	parts := []string{para(r)}
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
	return strings.Join(parts, "\n\n")
}
