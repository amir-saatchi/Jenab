package viewrun

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/sqlworker"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/view"
)

// The fixture is a project with a 10,000-row table, a 10,001-row table of
// points, a few coins and two files in the bucket. The tests share it and
// only read it.

var fixture *store.ProjectDB

const bigRows = 10_000

var fixtureSQL = []string{
	`CREATE TABLE big(id INTEGER PRIMARY KEY, name TEXT NOT NULL, amount REAL, share REAL, day TEXT, at TEXT,
		ok INTEGER, status TEXT, link TEXT, file TEXT)`,
	// 10,000 rows: amounts falling, five statuses, a file on every 1,000th.
	`WITH RECURSIVE c(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM c WHERE i < 10000)
	 INSERT INTO big SELECT i, 'row ' || printf('%05d', i), 20000.5 - i, i / 10000.0,
		date('2026-01-01', '+' || (i % 300) || ' days'), '2026-11-20 08:00:00',
		i % 2, CASE i % 5 WHEN 0 THEN 'new' WHEN 1 THEN 'read' WHEN 2 THEN 'failed' WHEN 3 THEN 'new' ELSE 'kept' END,
		'https://example.test/' || i,
		CASE WHEN i % 1000 = 1 THEN 'images/a.png' WHEN i % 1000 = 2 THEN 'images/gone.png' WHEN i % 1000 = 3 THEN '../escape' END
	 FROM c`,
	`CREATE TABLE points(id INTEGER PRIMARY KEY, x REAL NOT NULL, y REAL, coin TEXT NOT NULL)`,
	// 10,001 points, x falling as id rises, so the chart must sort them.
	`WITH RECURSIVE c(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM c WHERE i < 10001)
	 INSERT INTO points SELECT i, 10001 - i, i * 2, CASE i % 2 WHEN 0 THEN 'BTC' ELSE 'ETH' END FROM c`,
	`CREATE TABLE coins(sym TEXT PRIMARY KEY, price REAL, change REAL, rank INTEGER) WITHOUT ROWID`,
	`INSERT INTO coins VALUES ('BTC', 92500, 0.025, 1), ('ETH', 3100, -0.01, 2), ('XRP', 0.6, NULL, 3)`,
	`CREATE TABLE prices(day TEXT NOT NULL, coin TEXT NOT NULL, price REAL, PRIMARY KEY (day, coin)) WITHOUT ROWID`,
	`INSERT INTO prices VALUES ('2026-11-18', 'BTC', 90000), ('2026-11-19', 'BTC', 91000), ('2026-11-20', 'BTC', 92500),
		('2026-11-18', 'ETH', 3000), ('2026-11-20', 'ETH', 3100), ('2026-11-21', 'BTC', 93000)`,
}

// TestMain lets the test binary be its own query worker.
func TestMain(m *testing.M) {
	sqlworker.IfWorker()
	os.Exit(run0(m))
}

func run0(m *testing.M) int {
	dir, err := os.MkdirTemp("", "jenab-viewrun-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(dir)
	ctx := context.Background()
	p, err := store.OpenProject(ctx, dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer p.Close(ctx)
	if err := setUp(ctx, p); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fixture = p
	return m.Run()
}

func setUp(ctx context.Context, p *store.ProjectDB) error {
	_, err := store.Do(ctx, p.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		for _, s := range fixtureSQL {
			if _, err := tx.Exec(s); err != nil {
				return struct{}{}, fmt.Errorf("%s: %w", s, err)
			}
		}
		return struct{}{}, nil
	})
	if err != nil {
		return err
	}
	_, err = p.PutObject(ctx, limit.Interactive, id.SourceUser, "images/a.png", strings.NewReader("png"), store.PutOptions{Source: "upload"})
	return err
}

// runner runs views on the fixture, on 2026-11-20 at 22:00 UTC, which is
// already the 21st in Tehran, the project's zone.
func runner() *Runner {
	tehran := time.FixedZone("Tehran", 3*3600+1800)
	return &Runner{Source: fixture, Now: func() time.Time { return time.Date(2026, 11, 20, 22, 0, 0, 0, time.UTC).In(tehran) }}
}

// parse parses a view's YAML.
func parse(t testing.TB, y string) *view.View {
	t.Helper()
	v, err := view.ParseView(view.Default, []byte(y))
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, y)
	}
	return v
}

// parsePage parses a page's YAML.
func parsePage(t testing.TB, y string) *view.Page {
	t.Helper()
	p, err := view.ParsePage(view.Default, []byte(y))
	if err != nil {
		t.Fatalf("parse page: %v\n%s", err, y)
	}
	return p
}

// noError fails when b has an error.
func noError(t testing.TB, b Block) {
	t.Helper()
	if b.Error != "" {
		t.Fatalf("block %s: %s", b.View, b.Error)
	}
}

// hooked is the fixture with its calls changed, for faults a real
// project can't be made to show on demand.
type hooked struct {
	query  func(ctx context.Context, q string, args []any, max int) (store.Rows, error)
	exists func(ctx context.Context, keys []string) (map[string]bool, error)
}

func (h hooked) ViewQuery(ctx context.Context, q string, args []any, max int) (store.Rows, error) {
	if h.query != nil {
		return h.query(ctx, q, args, max)
	}
	return fixture.ViewQuery(ctx, q, args, max)
}

func (h hooked) ObjectsExist(ctx context.Context, keys []string) (map[string]bool, error) {
	if h.exists != nil {
		return h.exists(ctx, keys)
	}
	return fixture.ObjectsExist(ctx, keys)
}

// hookedRunner is runner on h.
func hookedRunner(h hooked) *Runner {
	r := runner()
	r.Source = h
	return r
}
