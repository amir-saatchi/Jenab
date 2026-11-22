package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/viewrun"
)

// noNetwork fails any HTTP request made while the test runs, and keeps
// its URL.
type noNetwork struct {
	mu   sync.Mutex
	urls []string
}

func (n *noNetwork) RoundTrip(r *http.Request) (*http.Response, error) {
	n.mu.Lock()
	n.urls = append(n.urls, r.URL.String())
	n.mu.Unlock()
	return nil, errors.New("the view runtime made a request")
}

func (n *noNetwork) requests() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]string(nil), n.urls...)
}

// viewsConfigs are the views and pages of the test project.
var viewsConfigs = []store.ConfigSave{
	{Kind: store.KindView, ID: "prices", Type: "table", Title: "Prices", YAML: `version: 1
id: prices
title: Prices
type: table
query: SELECT day, coin, price FROM prices WHERE (:coin IS NULL OR coin = :coin)
filters:
  - { param: coin, control: select, options: [BTC, ETH] }
default_sort: { field: day, direction: asc }
columns:
  - { field: day, format: date }
  - { field: coin }
  - { field: price, format: currency, format_options: { currency: USD } }
`},
	{Kind: store.KindView, ID: "trend", Type: "chart", Title: "Trend", YAML: `version: 1
id: trend
title: Trend
type: chart
chart_type: line
query: SELECT day, price FROM prices WHERE coin = coalesce(:coin, 'BTC')
x: { field: day, format: date }
y:
  - { field: price }
`},
	{Kind: store.KindView, ID: "slow", Type: "stat", Title: "Slow", YAML: `version: 1
id: slow
title: Slow
type: stat
query: |
  WITH RECURSIVE c(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM c)
  SELECT count(*) AS n FROM c
value: { field: n }
`},
	{Kind: store.KindPage, ID: "coins", Title: "Coins", YAML: `version: 1
id: coins
title: Coins
filters:
  - { param: coin, control: select, options: [BTC, ETH] }
rows:
  - [ { view: prices }, { view: trend }, { view: gone } ]
`},
	{Kind: store.KindPage, ID: "slow_page", Title: "Slow", YAML: `version: 1
id: slow_page
title: Slow
rows:
  - [ { view: slow }, { view: trend } ]
`},
}

// viewsProject makes a project with a prices table and viewsConfigs.
func viewsProject(t *testing.T, e *env) id.Project {
	t.Helper()
	ctx := context.Background()
	op, err := e.svc.Project.Create(ctx, "Coins")
	if err != nil {
		t.Fatal(err)
	}
	err = open(ctx, e.pm, op.ID, func(p *project.Project) error {
		_, err := store.Do(ctx, p.DB.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
			for _, s := range []string{
				`CREATE TABLE prices(day TEXT NOT NULL, coin TEXT NOT NULL, price REAL, PRIMARY KEY (day, coin)) WITHOUT ROWID`,
				`INSERT INTO prices VALUES ('2026-11-18', 'BTC', 90000), ('2026-11-19', 'BTC', 91000), ('2026-11-18', 'ETH', 3000)`,
			} {
				if _, err := tx.Exec(s); err != nil {
					return struct{}{}, err
				}
			}
			return struct{}{}, nil
		})
		if err != nil {
			return err
		}
		for _, c := range viewsConfigs {
			c.Source = id.SourceUser
			if _, err := p.DB.SaveConfig(ctx, c); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return op.ID
}

// answers gathers a page's answers.
type answers struct {
	mu      sync.Mutex
	filters [][]viewrun.Filter
	blocks  map[string]viewrun.Block
}

func (a *answers) out(x viewrun.Answer) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if x.Block == nil {
		a.filters = append(a.filters, x.Filters)
		return
	}
	if a.blocks == nil {
		a.blocks = map[string]viewrun.Block{}
	}
	a.blocks[x.Block.View] = *x.Block
}

func (a *answers) get(v string) (viewrun.Block, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	b, ok := a.blocks[v]
	return b, ok
}

// Opening a page runs its saved views on the project, and calls no model
// and makes no request (R-48); a view on its own runs too; a page or a
// view that isn't there is not found.
func TestViewServicePage(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	p := viewsProject(t, e)

	net := &noNetwork{}
	was := http.DefaultTransport
	http.DefaultTransport = net
	t.Cleanup(func() { http.DefaultTransport = was })
	calls := len(e.fp.Calls())

	var a answers
	begin := time.Now()
	if err := e.svc.Views.Page(ctx, p, "coins", viewrun.PageRequest{Values: viewrun.Values{"coin": "BTC"}}, a.out); err != nil {
		t.Fatal(err)
	}
	t.Logf("a three-block page through the service, configs loaded and parsed: %v", time.Since(begin))
	if len(a.filters) != 1 || a.filters[0][0].Value != "BTC" || len(a.blocks) != 3 {
		t.Fatalf("filters %+v, %d blocks", a.filters, len(a.blocks))
	}
	if b, _ := a.get("prices"); b.Error != "" || len(b.Table.Rows) != 2 || b.Table.Rows[0][1] != "BTC" || len(b.Filters) != 0 {
		t.Errorf("prices %+v", b)
	}
	if b, _ := a.get("trend"); b.Error != "" || len(b.Chart.X) != 2 || !b.Chart.XAxis.Time {
		t.Errorf("trend %+v", b)
	}
	if b, _ := a.get("gone"); b.Error != "There is no view gone." {
		t.Errorf("gone %+v", b)
	}

	b, err := e.svc.Views.View(ctx, p, "prices", viewrun.ViewRequest{Values: viewrun.Values{"coin": "ETH"}})
	if err != nil || b.Error != "" || b.Table.Total != 1 || b.Filters[0].Value != "ETH" {
		t.Fatalf("view %+v, %v", b, err)
	}
	_, err = e.svc.Views.View(ctx, p, "nope", viewrun.ViewRequest{})
	uiErr(t, err, KindNotFound)
	err = e.svc.Views.Page(ctx, p, "nope", viewrun.PageRequest{}, a.out)
	uiErr(t, err, KindNotFound)

	if n := len(e.fp.Calls()); n != calls {
		t.Errorf("opening a page called the model %d times", n-calls)
	}
	if r := net.requests(); len(r) != 0 {
		t.Errorf("opening a page made requests: %v", r)
	}
}

// Start runs a page in the background; starting again under the same key
// stops the run before it, whose out gets nothing more; Stop stops a run
// without waiting for it, and done says when it has stopped.
func TestViewServiceStart(t *testing.T) {
	e := newEnv(t, t.TempDir())
	p := viewsProject(t, e)
	vs := e.svc.Views

	var first answers
	firstDone := make(chan error, 1)
	vs.Start("chat-1", p, "slow_page", viewrun.PageRequest{}, first.out, func(err error) { firstDone <- err })
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, ok := first.get("trend"); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the trend block didn't answer")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// The filters change while the slow block runs.
	var second answers
	secondDone := make(chan error, 1)
	begin := time.Now()
	vs.Start("chat-1", p, "coins", viewrun.PageRequest{Values: viewrun.Values{"coin": "ETH"}}, second.out, func(err error) { secondDone <- err })
	select {
	case err := <-firstDone:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("first run: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("the first run didn't stop")
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	t.Logf("restart with new filters answered in %v", time.Since(begin))
	if _, ok := first.get("slow"); ok {
		t.Error("the stopped run's slow block answered")
	}
	if b, _ := second.get("prices"); b.Error != "" || len(b.Table.Rows) != 1 {
		t.Errorf("second run prices %+v", b)
	}

	// Stop, as when the page closes.
	var third answers
	thirdDone := make(chan error, 1)
	vs.Start("chat-2", p, "slow_page", viewrun.PageRequest{}, third.out, func(err error) { thirdDone <- err })
	time.Sleep(200 * time.Millisecond)
	begin = time.Now()
	vs.Stop("chat-2")
	if took := time.Since(begin); took > time.Second {
		t.Errorf("Stop took %v", took)
	}
	select {
	case err := <-thirdDone:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("stopped run: %v", err)
		}
	case <-time.After(time.Second):
		t.Error("the stopped run didn't end within 1 s")
	}
	vs.Stop("chat-2") // nothing under the key: nothing to do
	if _, ok := third.get("slow"); ok {
		t.Error("the stopped run's slow block answered")
	}
}

// Starts and Stops on one key, from two goroutines at once: every run
// ends once, and no answer of a run reaches out after an answer of a
// later one. Run it with -race too.
func TestViewServiceStartStop(t *testing.T) {
	e := newEnv(t, t.TempDir())
	p := viewsProject(t, e)
	vs := e.svc.Views

	const n = 20
	var mu sync.Mutex
	var log []int
	ended := make(chan error, n)
	stopping := make(chan struct{})
	go func() {
		defer close(stopping)
		for range n {
			vs.Stop("chat")
			time.Sleep(3 * time.Millisecond)
		}
	}()
	for i := range n {
		out := func(viewrun.Answer) {
			mu.Lock()
			log = append(log, i)
			mu.Unlock()
		}
		vs.Start("chat", p, "coins", viewrun.PageRequest{}, out, func(err error) { ended <- err })
		time.Sleep(2 * time.Millisecond)
	}
	<-stopping
	for range n {
		select {
		case err := <-ended:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("run: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("a run didn't end")
		}
	}
	select {
	case err := <-ended:
		t.Fatalf("a run ended twice: %v", err)
	default:
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.IsSorted(log) {
		t.Errorf("answers out of order: %v", log)
	}
	t.Logf("%d answers from %d runs", len(log), n)
}

// out may call Stop, and Start, on its own key without a deadlock; after
// Stop, the run's out gets nothing more.
func TestViewServiceReentrant(t *testing.T) {
	e := newEnv(t, t.TempDir())
	p := viewsProject(t, e)
	vs := e.svc.Views

	var mu sync.Mutex
	got := 0
	stopped := make(chan error, 1)
	vs.Start("chat-1", p, "coins", viewrun.PageRequest{}, func(viewrun.Answer) {
		mu.Lock()
		got++
		mu.Unlock()
		vs.Stop("chat-1")
	}, func(err error) { stopped <- err })
	select {
	case err := <-stopped:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("stopped from out: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Stop from out didn't stop the run")
	}
	mu.Lock()
	if got != 1 {
		t.Errorf("%d answers after Stop from out", got)
	}
	mu.Unlock()

	var second answers
	firstDone, secondDone := make(chan error, 1), make(chan error, 1)
	var once sync.Once
	vs.Start("chat-2", p, "coins", viewrun.PageRequest{}, func(viewrun.Answer) {
		once.Do(func() {
			vs.Start("chat-2", p, "coins", viewrun.PageRequest{Values: viewrun.Values{"coin": "ETH"}}, second.out,
				func(err error) { secondDone <- err })
		})
	}, func(err error) { firstDone <- err })
	for name, ch := range map[string]chan error{"first": firstDone, "second": secondDone} {
		select {
		case err := <-ch:
			if want := name == "first"; errors.Is(err, context.Canceled) != want || !want && err != nil {
				t.Errorf("%s run: %v", name, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("the %s run didn't end", name)
		}
	}
	if b, _ := second.get("prices"); b.Error != "" || b.Table == nil || len(b.Table.Rows) != 1 {
		t.Errorf("second run prices %+v", b)
	}
}
