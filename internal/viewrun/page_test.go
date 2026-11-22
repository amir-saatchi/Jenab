package viewrun

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/view"
)

// filterView binds one parameter of each control, and returns them all as
// one row, so a stat-like table shows what each bound.
const filterView = `version: 1
id: filtered
title: Filtered
type: table
query: |
  SELECT :day AS day, :since AS since, :coin AS coin, :rank AS rank, :min AS min, :sym AS sym,
         :mine AS mine, :must AS must, :word AS word
filters:
  - { param: day, control: date, default: today }
  - { param: since, control: date, default: "today-1m", required: true }
  - { param: coin, control: select, options: [BTC, ETH] }
  - { param: rank, control: select, options: { query: "SELECT rank FROM coins ORDER BY rank DESC" }, required: true }
  - { param: min, control: number, default: 5 }
  - { param: sym, control: select, options: { query: "SELECT sym FROM coins ORDER BY sym" }, required: true }
  - { param: mine, control: checkbox }
  - { param: must, control: checkbox, required: true }
  - { param: word, control: text, default: "x" }
default_sort: { field: day, direction: asc }
columns:
  - { field: day }
  - { field: since }
  - { field: coin }
  - { field: rank }
  - { field: min }
  - { field: sym }
  - { field: mine }
  - { field: must }
  - { field: word }
`

// Filters: defaults, relative dates in the project's zone, a cleared
// optional filter NULL, a cleared required one its default or first
// option, options from a query, a select's text bound as its option.
func TestFilters(t *testing.T) {
	ctx := context.Background()
	v := parse(t, filterView)
	bound := func(b Block) map[string]any {
		t.Helper()
		noError(t, b)
		out := map[string]any{}
		for i, c := range b.Table.Columns {
			out[c.Field] = b.Table.Rows[0][i]
		}
		return out
	}
	// The defaults. Today is the 21st in Tehran, though the 20th in UTC.
	b := runner().View(ctx, v, ViewRequest{})
	got := bound(b)
	want := map[string]any{"day": "2026-11-21", "since": "2026-10-21", "coin": nil, "rank": int64(3), "min": int64(5),
		"sym": "BTC", "mine": nil, "must": int64(0), "word": "x"}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("default %s = %#v, want %#v", k, got[k], w)
		}
	}
	if f := b.Filters[3]; f.Param != "rank" || !slices.Equal(f.Options, []any{int64(3), int64(2), int64(1)}) || f.Value != int64(3) {
		t.Errorf("rank filter %+v", f)
	}
	if f := b.Filters[7]; f.Value != false {
		t.Errorf("required checkbox %+v", f)
	}

	// Values as the controls bind them; cleared ones; a relative date the
	// agent gave; a select's text for a number option.
	b = runner().View(ctx, v, ViewRequest{Values: Values{
		"day": nil, "since": nil, "coin": "ETH", "rank": "2", "min": nil, "sym": nil, "mine": true, "must": true, "word": nil,
	}})
	got = bound(b)
	want = map[string]any{"day": nil, "since": "2026-10-21", "coin": "ETH", "rank": int64(2), "min": nil,
		"sym": "BTC", "mine": int64(1), "must": int64(1), "word": nil}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("given %s = %#v, want %#v", k, got[k], w)
		}
	}
	b = runner().View(ctx, v, ViewRequest{Values: Values{"day": "today-7d", "min": "12.5", "mine": false, "since": "2026-01-01"}})
	got = bound(b)
	if got["day"] != "2026-11-14" || got["min"] != 12.5 || got["mine"] != nil || got["since"] != "2026-01-01" {
		t.Errorf("relative day %v, min %v, mine %v, since %v", got["day"], got["min"], got["mine"], got["since"])
	}
	// What can't be read binds the default, and says why.
	b = runner().View(ctx, v, ViewRequest{Values: Values{"since": "soon", "min": "many"}})
	got = bound(b)
	if got["since"] != "2026-10-21" || got["min"] != int64(5) ||
		b.Filters[1].Error != "soon is not a date; write it as 2026-11-20" || b.Filters[4].Error != "many is not a number" {
		t.Errorf("since %v (%s), min %v (%s)", got["since"], b.Filters[1].Error, got["min"], b.Filters[4].Error)
	}
	// An options query that fails says so on its filter.
	v.Filters[3].Options.Query = "SELECT nope FROM coins"
	b = runner().View(ctx, v, ViewRequest{})
	if f := b.Filters[3]; f.Options != nil || !strings.HasPrefix(f.Error, "Can't read the options: No such column: nope") {
		t.Errorf("bad options %+v", f)
	}
}

// Only :name parameters bind; params give fixed values.
func TestParams(t *testing.T) {
	v := parse(t, `version: 1
id: at
title: At
type: stat
query: SELECT @x AS v
value: { field: v }
`)
	b := runner().View(context.Background(), v, ViewRequest{})
	if !strings.Contains(b.Error, "The query has the parameter @x; only :name parameters are bound") {
		t.Errorf("@x: %q", b.Error)
	}
	v.Query = "SELECT :n * 2 AS v, :none IS NULL AS unset"
	v.Params = map[string]any{"n": int64(21)}
	b = runner().View(context.Background(), v, ViewRequest{Values: Values{"n": 1}})
	noError(t, b)
	if b.Stat.Value != int64(42) || !slices.Equal(b.Params, []string{"n", "none"}) {
		t.Errorf("%+v, %v", b.Stat, b.Params)
	}
}

// pageViews are the views of the test page.
var pageViews = map[string]string{
	"big_table": bigTable,
	"line": `version: 1
id: line
title: Line
type: chart
chart_type: line
query: SELECT x, y FROM points WHERE (:coin IS NULL OR coin = :coin)
filters:
  - { param: coin, control: select, options: [BTC, ETH] }
x: { field: x }
y:
  - { field: y }
`,
	"price": `version: 1
id: price
title: Price
type: stat
query: SELECT price FROM coins WHERE sym = coalesce(:coin, 'BTC')
value: { field: price, format: currency, format_options: { currency: USD } }
`,
	"since": `version: 1
id: since
title: Since
type: table
query: SELECT day, coin, price FROM prices WHERE day >= :from
filters:
  - { param: from, control: date, default: "today-3d", required: true }
default_sort: { field: day, direction: asc }
columns:
  - { field: day, format: date }
  - { field: price, format: number }
`,
	"add": `version: 1
id: add
title: Add
type: form
fields:
  - { field: sym, control: text, required: true }
submit: { action: insert, table: coins }
`,
	"slow": `version: 1
id: slow
title: Slow
type: stat
query: |
  WITH RECURSIVE c(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM c)
  SELECT count(*) AS n FROM c
value: { field: n }
`,
}

const testPage = `version: 1
id: coins
title: Coins
filters:
  - { param: coin, control: select, label: Coin, options: { query: "SELECT sym FROM coins ORDER BY sym" } }
rows:
  - [ { view: big_table, span: 8 }, { view: price } ]
  - [ { view: line }, { view: since }, { view: gone } ]
  - [ { card: Write, rows: [ [ { text: "Add a coin" }, { view: add } ] ] } ]
`

func views(t testing.TB, ids ...string) Views {
	m := map[string]*view.View{}
	for _, id := range ids {
		m[id] = parse(t, pageViews[id])
	}
	return ViewMap(m)
}

// collect gathers a page's answers.
type collect struct {
	mu      sync.Mutex
	filters [][]Filter
	blocks  []Block
	at      map[string]time.Duration // since the run began
	begin   time.Time
}

func newCollect() *collect { return &collect{at: map[string]time.Duration{}, begin: time.Now()} }

func (c *collect) out(a Answer) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if a.Block == nil {
		c.filters = append(c.filters, a.Filters)
		return
	}
	c.blocks = append(c.blocks, *a.Block)
	c.at[a.Block.Place] = time.Since(c.begin)
}

func (c *collect) block(t *testing.T, place string) Block {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, b := range c.blocks {
		if b.Place == place {
			return b
		}
	}
	t.Fatalf("no block at %s", place)
	return Block{}
}

// A page runs its blocks in parallel: the page's filter binds the
// parameter in every block that has it and hides the blocks' own; a view
// that is gone is an error on its block; a form has no query.
func TestPage(t *testing.T) {
	ctx := context.Background()
	p := parsePage(t, testPage)
	vs := views(t, "big_table", "line", "price", "since", "add")
	if got := Places(p); !slices.Equal(got, []PlacedView{{"0.0", "big_table"}, {"0.1", "price"}, {"1.0", "line"},
		{"1.1", "since"}, {"1.2", "gone"}, {"2.0.0.1", "add"}}) {
		t.Fatalf("places %v", got)
	}
	r := runner()
	// Warm the workers, so the time below is the page's.
	r.View(ctx, vs.must(t, "price"), ViewRequest{})

	c := newCollect()
	begin := time.Now()
	if err := r.Page(ctx, p, vs, PageRequest{Values: Values{"coin": "ETH"}}, c.out); err != nil {
		t.Fatal(err)
	}
	t.Logf("page of 6 blocks (a 10,000-row table, a 10,001-row line, a stat, a small table, a form, a missing view): %v", time.Since(begin))
	for _, b := range c.blocks {
		t.Logf("  %s %s: answered at %v, took %v", b.Place, b.View, c.at[b.Place], b.Took)
	}
	if len(c.filters) != 1 || len(c.filters[0]) != 1 || c.filters[0][0].Value != "ETH" || len(c.filters[0][0].Options) != 3 || len(c.blocks) != 6 {
		t.Fatalf("filters %+v, %d blocks", c.filters, len(c.blocks))
	}
	line := c.block(t, "1.0")
	noError(t, line)
	// ETH is every odd id of 10,001.
	if len(line.Filters) != 0 || len(line.Chart.X) != 5001 || line.Notice != "" {
		t.Errorf("line: own filters %v, %d points, %q", line.Filters, len(line.Chart.X), line.Notice)
	}
	if price := c.block(t, "0.1"); price.Error != "" || price.Stat.Value != 3100.0 {
		t.Errorf("price %+v", price)
	}
	// The table's own status filter stays; the page has no status.
	if big := c.block(t, "0.0"); big.Error != "" || big.Table.Total != bigRows || len(big.Filters) != 1 {
		t.Errorf("big %d %v", big.Table.Total, big.Filters)
	}
	// today-3d is the 18th in the project's zone: all six prices.
	if since := c.block(t, "1.1"); since.Error != "" || len(since.Table.Rows) != 6 || since.Filters[0].Value != "2026-11-18" {
		t.Errorf("since %+v", since.Table)
	}
	if gone := c.block(t, "1.2"); gone.Error != "There is no view gone." || gone.View != "gone" {
		t.Errorf("gone %+v", gone)
	}
	if add := c.block(t, "2.0.0.1"); add.Error != "" || add.Type != "form" || add.Table != nil {
		t.Errorf("form %+v", add)
	}

	// Re-run only the blocks a filter reaches, with the table's next page.
	c = newCollect()
	err := r.Page(ctx, p, vs, PageRequest{Values: Values{"coin": nil}, Only: []string{"1.0", "0.0"},
		Blocks: map[string]ViewRequest{"0.0": {Table: TableRequest{Page: 1}}}}, c.out)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.blocks) != 2 || len(c.block(t, "1.0").Chart.X) != MaxChartRows || c.block(t, "0.0").Table.Page != 1 {
		t.Errorf("re-run: %d blocks", len(c.blocks))
	}
}

func (vs Views) must(t testing.TB, id string) *view.View {
	v, err := vs(id)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// A slow query times out on its own block after 2 s; the others answer
// long before.
func TestSlowBlock(t *testing.T) {
	ctx := context.Background()
	p := parsePage(t, `version: 1
id: slow_page
title: Slow
rows:
  - [ { view: slow }, { view: price }, { view: since }, { view: line } ]
`)
	c := newCollect()
	begin := time.Now()
	if err := runner().Page(ctx, p, views(t, "slow", "price", "since", "line"), PageRequest{}, c.out); err != nil {
		t.Fatal(err)
	}
	took := time.Since(begin)
	slow := c.block(t, "0.0")
	if !strings.HasPrefix(slow.Error, "The query took longer than 2s and was stopped.") || slow.Stat != nil {
		t.Errorf("slow block: %q", slow.Error)
	}
	for _, pl := range []string{"0.1", "0.2", "0.3"} {
		b := c.block(t, pl)
		noError(t, b)
		if c.at[pl] > time.Second {
			t.Errorf("%s answered at %v, behind the slow one", b.View, c.at[pl])
		}
	}
	if c.at["0.0"] < 2*time.Second || took > 4*time.Second {
		t.Errorf("slow block at %v, page %v", c.at["0.0"], took)
	}
	if last := c.blocks[len(c.blocks)-1]; last.Place != "0.0" {
		t.Errorf("the slow block answered before %s", last.View)
	}
	t.Logf("slow block stopped at %v; others at %v, %v, %v", c.at["0.0"], c.at["0.1"], c.at["0.2"], c.at["0.3"])
}

// A cancelled run stops its queries at once, sends nothing more, and the
// next run works.
func TestCancel(t *testing.T) {
	p := parsePage(t, `version: 1
id: slow_page
title: Slow
rows:
  - [ { view: slow }, { view: price } ]
`)
	vs := views(t, "slow", "price")
	ctx, cancel := context.WithCancel(context.Background())
	c := newCollect()
	done := make(chan error)
	go func() { done <- runner().Page(ctx, p, vs, PageRequest{}, c.out) }()
	time.Sleep(300 * time.Millisecond)
	cancel()
	stopped := time.Now()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Page = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("the run didn't stop within 1 s of its cancel")
	}
	t.Logf("cancelled run stopped in %v", time.Since(stopped))
	c.mu.Lock()
	for _, b := range c.blocks {
		if b.Place == "0.0" {
			t.Errorf("the slow block answered after the cancel: %+v", b)
		}
	}
	c.mu.Unlock()
	// The worker was killed; the next query starts a new one.
	b := runner().View(context.Background(), vs.must(t, "price"), ViewRequest{})
	noError(t, b)
}
