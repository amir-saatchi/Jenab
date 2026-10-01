package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"modernc.org/sqlite"
)

// DB is an in-memory project database with 10,000 price rows, plus a read-only
// connection for scripts.
type DB struct {
	writer *sql.DB   // keeps the shared in-memory database alive
	reader *sql.Conn // query_only, no ATTACH, 16 MiB value limit
	ctx    context.Context
}

const (
	maxRows          = 100_000 // rows one db.query may return
	limitLength      = 0       // SQLITE_LIMIT_LENGTH
	limitAttached    = 7       // SQLITE_LIMIT_ATTACHED
	symbols          = 10
	daysPerSymbol    = 1000
	firstDay         = "2021-01-01"
	transformSQL     = "SELECT symbol, day, price FROM prices WHERE day >= ? ORDER BY symbol, day"
	transformSince   = "2021-01-01"
	transformDateFmt = "02.01.2006"
)

func openDB() (*DB, error) {
	dsn := fmt.Sprintf("file:spike016_%d?mode=memory&cache=shared", time.Now().UnixNano())
	w, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if _, err := w.Exec(`CREATE TABLE prices(id INTEGER PRIMARY KEY, symbol TEXT NOT NULL, day TEXT NOT NULL, price REAL NOT NULL, volume INTEGER NOT NULL)`); err != nil {
		return nil, err
	}
	tx, _ := w.Begin()
	st, _ := tx.Prepare(`INSERT INTO prices(symbol, day, price, volume) VALUES(?,?,?,?)`)
	d0, _ := time.Parse("2006-01-02", firstDay)
	for s := 0; s < symbols; s++ {
		sym := fmt.Sprintf("SYM%02d", s)
		for d := 0; d < daysPerSymbol; d++ {
			price := 100 + float64(s)*10 + 5*math.Sin(float64(d)/17) + float64((d*7919+s*104729)%1000)/100
			if _, err := st.Exec(sym, d0.AddDate(0, 0, d).Format("2006-01-02"), math.Round(price*100)/100, (d*31+s)%5000); err != nil {
				return nil, err
			}
		}
	}
	st.Close()
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	r, err := sql.Open("sqlite", dsn+"&_pragma=query_only(1)")
	if err != nil {
		return nil, err
	}
	r.SetMaxOpenConns(1)
	ctx := context.Background()
	c, err := r.Conn(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := sqlite.Limit(c, limitAttached, 0); err != nil {
		return nil, err
	}
	if _, err := sqlite.Limit(c, limitLength, 16<<20); err != nil {
		return nil, err
	}
	return &DB{writer: w, reader: c, ctx: ctx}, nil
}

// Rows is a query result in column order.
type Rows struct {
	Cols []string
	Data [][]any
}

// Query is the read-only db.query exposed to scripts. The text check is a
// simplified stand-in for the SPIKE-007 guard: one statement, SELECT or WITH.
// query_only is reset before every call because preparing a PRAGMA can flip it.
func (d *DB) Query(ctx context.Context, q string, params []any) (*Rows, error) {
	s := strings.TrimSpace(q)
	s = strings.TrimSpace(strings.TrimSuffix(s, ";"))
	if strings.Contains(s, ";") {
		return nil, errors.New("db.query: one statement only")
	}
	head := strings.ToUpper(strings.Fields(s + " x")[0])
	if head != "SELECT" && head != "WITH" {
		return nil, errors.New("db.query: only SELECT or WITH")
	}
	if ctx == nil {
		ctx = d.ctx
	}
	if _, err := d.reader.ExecContext(ctx, "PRAGMA query_only = 1"); err != nil {
		return nil, err
	}
	rows, err := d.reader.QueryContext(ctx, s, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	out := &Rows{Cols: cols}
	for rows.Next() {
		if len(out.Data) >= maxRows {
			return nil, fmt.Errorf("db.query: more than %d rows", maxRows)
		}
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				vals[i] = string(b)
			}
		}
		out.Data = append(out.Data, vals)
	}
	return out, rows.Err()
}

// The three transform functions exposed to scripts.

func fnRound(x float64, digits int) float64 {
	p := math.Pow(10, float64(digits))
	return math.Round(x*p) / p
}

func fnAddDays(day string, n int) (string, error) {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return "", err
	}
	return t.AddDate(0, 0, n).Format("2006-01-02"), nil
}

func fnFormatDate(day, layout string) (string, error) {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return "", err
	}
	return t.Format(layout), nil
}
