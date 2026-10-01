package main

// A test project: a real SQLite file (modernc.org/sqlite v1.59.0) with a writer connection and a
// guarded reader (query_only, SQLITE_LIMIT_ATTACHED 0, SQLITE_LIMIT_LENGTH 16 MiB), plus the
// saved configs. Configs are kept in memory; only the data tables and a few internal tables
// live in the file (the guard must see real _burrow_* tables to reject them).

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	modernc "modernc.org/sqlite"
)

type Column struct {
	Name, Type string
	NotNull    bool
	Default    sql.NullString
	PK         int // position in the primary key (1-based), 0 if not part of it
}

type Index struct {
	Name   string
	Unique bool
	Cols   []string
	Origin string // c (CREATE INDEX), u (UNIQUE constraint), pk
}

type Table struct {
	Name    string
	Cols    []Column
	PK      []string
	Indexes []Index
	Rows    int
}

func (t *Table) col(name string) *Column {
	for i := range t.Cols {
		if strings.EqualFold(t.Cols[i].Name, name) {
			return &t.Cols[i]
		}
	}
	return nil
}

func (t *Table) colNames() []string {
	var out []string
	for _, c := range t.Cols {
		out = append(out, c.Name)
	}
	return out
}

// isKey reports whether cols are exactly the primary key or a unique index.
func (t *Table) isKey(cols []string) bool {
	eq := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		m := map[string]bool{}
		for _, x := range a {
			m[strings.ToLower(x)] = true
		}
		for _, x := range b {
			if !m[strings.ToLower(x)] {
				return false
			}
		}
		return true
	}
	if eq(cols, t.PK) {
		return true
	}
	for _, ix := range t.Indexes {
		if ix.Unique && eq(cols, ix.Cols) {
			return true
		}
	}
	return false
}

type Config struct {
	ID, Kind, Type string // Kind: pipeline or view; Type: table, chart, form (views)
	Src            string
	Val            map[string]any
	Rev            int
}

type Event struct {
	Tool string
	OK   bool
	Note string
}

type Project struct {
	Dir           string
	Path          string
	wdb, rdb      *sql.DB
	W, R          *sql.Conn
	Configs       map[string]*Config
	SchemaVersion int
	Annot         map[string]string // table.column -> kind
	Migrations    int               // committed migrations
	Log           []Event
}

func newProject(dir string) (*Project, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	p := &Project{Dir: dir, Path: filepath.Join(dir, "project.db"), Configs: map[string]*Config{}, Annot: map[string]string{}}
	var err error
	p.wdb, p.W, err = openConn(p.Path, false)
	if err != nil {
		return nil, err
	}
	for _, s := range []string{
		"CREATE TABLE _burrow_meta(key TEXT PRIMARY KEY, value TEXT)",
		"CREATE TABLE _burrow_views(id TEXT PRIMARY KEY, config TEXT, revision INTEGER)",
		"CREATE TABLE _burrow_pipelines(id TEXT PRIMARY KEY, config TEXT, revision INTEGER)",
		"CREATE TABLE _burrow_changes(id INTEGER PRIMARY KEY, at TEXT, source TEXT, target TEXT, op TEXT)",
		"INSERT INTO _burrow_meta VALUES ('name', 'Bitcoin tracker'), ('schema_version', '0')",
	} {
		if _, err := p.W.ExecContext(bg, s); err != nil {
			return nil, err
		}
	}
	p.rdb, p.R, err = openConn(p.Path, true)
	return p, err
}

func openConn(path string, reader bool) (*sql.DB, *sql.Conn, error) {
	dsn := "file:" + filepath.ToSlash(path) + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	if reader {
		dsn += "&_pragma=query_only(1)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, nil, err
	}
	db.SetMaxOpenConns(1)
	c, err := db.Conn(bg)
	if err != nil {
		return nil, nil, err
	}
	if _, err := modernc.Limit(c, 7, 0); err != nil { // SQLITE_LIMIT_ATTACHED
		return nil, nil, err
	}
	if reader {
		if _, err := modernc.Limit(c, 0, 16<<20); err != nil { // SQLITE_LIMIT_LENGTH
			return nil, nil, err
		}
	}
	return db, c, nil
}

func (p *Project) Close() {
	if p.R != nil {
		p.R.Close()
		p.rdb.Close()
	}
	if p.W != nil {
		p.W.Close()
		p.wdb.Close()
	}
	os.RemoveAll(p.Dir)
}

// loadSchema reads the agent tables.
func loadSchema(q Q) (map[string]*Table, error) {
	rows, err := q.QueryContext(bg, `SELECT name FROM sqlite_schema WHERE type = 'table'
		AND name NOT LIKE 'sqlite\_%' ESCAPE '\' AND name NOT LIKE '\_burrow\_%' ESCAPE '\' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	var names []string
	for rows.Next() {
		var n string
		rows.Scan(&n)
		names = append(names, n)
	}
	rows.Close()
	out := map[string]*Table{}
	for _, n := range names {
		t := &Table{Name: n}
		cr, err := q.QueryContext(bg, `SELECT name, type, "notnull", dflt_value, pk FROM pragma_table_info(?)`, n)
		if err != nil {
			return nil, err
		}
		type pkc struct {
			n string
			i int
		}
		var pks []pkc
		for cr.Next() {
			var c Column
			cr.Scan(&c.Name, &c.Type, &c.NotNull, &c.Default, &c.PK)
			if c.PK > 0 {
				pks = append(pks, pkc{c.Name, c.PK})
			}
			t.Cols = append(t.Cols, c)
		}
		cr.Close()
		sort.Slice(pks, func(i, j int) bool { return pks[i].i < pks[j].i })
		for _, x := range pks {
			t.PK = append(t.PK, x.n)
		}
		ir, err := q.QueryContext(bg, `SELECT name, "unique", origin FROM pragma_index_list(?)`, n)
		if err != nil {
			return nil, err
		}
		var idx []Index
		for ir.Next() {
			var ix Index
			ir.Scan(&ix.Name, &ix.Unique, &ix.Origin)
			idx = append(idx, ix)
		}
		ir.Close()
		for i := range idx {
			cr, err := q.QueryContext(bg, `SELECT name FROM pragma_index_info(?) ORDER BY seqno`, idx[i].Name)
			if err != nil {
				return nil, err
			}
			for cr.Next() {
				var c sql.NullString
				cr.Scan(&c)
				idx[i].Cols = append(idx[i].Cols, c.String)
			}
			cr.Close()
		}
		t.Indexes = idx
		q.QueryRowContext(bg, `SELECT count(*) FROM "`+n+`"`).Scan(&t.Rows)
		out[n] = t
	}
	return out, nil
}

func tableNames(s map[string]*Table) []string {
	var out []string
	for n := range s {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// ---- project card (SPEC 3.2) ----

func (p *Project) Card() string {
	s, err := loadSchema(p.R)
	if err != nil {
		return "(schema unavailable: " + err.Error() + ")"
	}
	var b strings.Builder
	b.WriteString("## Project card (generated by the app)\nTables:\n")
	if len(s) == 0 {
		b.WriteString("- (none yet)\n")
	}
	for _, n := range tableNames(s) {
		t := s[n]
		var cols []string
		for _, c := range t.Cols {
			x := c.Name + " " + c.Type
			if c.NotNull {
				x += " NOT NULL"
			}
			if c.Default.Valid {
				x += " DEFAULT " + c.Default.String
			}
			if k := p.Annot[n+"."+c.Name]; k != "" {
				x += " [" + k + "]"
			}
			cols = append(cols, x)
		}
		var uniq []string
		for _, ix := range t.Indexes {
			if ix.Unique && ix.Origin != "pk" {
				uniq = append(uniq, "("+strings.Join(ix.Cols, ", ")+")")
			}
		}
		fmt.Fprintf(&b, "- %s (%d rows): %s; primary key (%s)", n, t.Rows, strings.Join(cols, ", "), strings.Join(t.PK, ", "))
		if len(uniq) > 0 {
			fmt.Fprintf(&b, "; unique %s", strings.Join(uniq, ", "))
		}
		if c := t.col("date"); c != nil && t.Rows > 0 {
			var lo, hi sql.NullString
			p.R.QueryRowContext(bg, `SELECT min(date), max(date) FROM "`+n+`"`).Scan(&lo, &hi)
			fmt.Fprintf(&b, "; date %s..%s", lo.String, hi.String)
		}
		b.WriteString("\n")
	}
	var views, pipes []string
	for _, id := range p.configIDs() {
		c := p.Configs[id]
		if c.Kind == "view" {
			views = append(views, fmt.Sprintf("%s (%s) %q", c.ID, c.Type, asStr(c.Val["title"])))
		} else {
			tr := asMap(c.Val["trigger"])
			var parts []string
			if s := asStr(tr["schedule"]); s != "" {
				parts = append(parts, fmt.Sprintf("schedule %q %s", s, asStr(tr["timezone"])))
			}
			if tr["manual"] == true {
				parts = append(parts, "manual")
			}
			pipes = append(pipes, fmt.Sprintf("%s (%s; never run)", c.ID, strings.Join(parts, ", ")))
		}
	}
	var deps []string
	if cardDeps {
		for _, id := range p.configIDs() {
			c := p.Configs[id]
			var uses []string
			for _, n := range tableNames(s) {
				if regexp.MustCompile(`\b` + regexp.QuoteMeta(n) + `\b`).MatchString(c.Src) {
					uses = append(uses, n)
				}
			}
			if len(uses) > 0 {
				deps = append(deps, fmt.Sprintf("%s uses %s", id, strings.Join(uses, ", ")))
			}
		}
	}
	b.WriteString("Views: " + orNone(views) + "\n")
	b.WriteString("Pipelines: " + orNone(pipes) + "\n")
	if len(deps) > 0 {
		b.WriteString("Dependents (a migration that changes these tables must send these configs, updated, in the same apply_migration call): " + strings.Join(deps, "; ") + "\n")
	}
	b.WriteString("Bucket: empty. Saved links: 0.\n")
	return b.String()
}

func orNone(l []string) string {
	if len(l) == 0 {
		return "(none)"
	}
	return strings.Join(l, "; ")
}

func (p *Project) configIDs() []string {
	var ids []string
	for id := range p.Configs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// ---- read tools ----

func (p *Project) describeTable(name string) string {
	s, err := loadSchema(p.R)
	if err != nil {
		return "error: " + err.Error()
	}
	t := s[name]
	if t == nil {
		if strings.HasPrefix(name, "_burrow_") || strings.HasPrefix(name, "sqlite_") {
			return fmt.Sprintf("error: table %q not found", name)
		}
		return fmt.Sprintf("error: table %q not found%s", name, didYouMean(name, tableNames(s)))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "table %s, %d rows\ncolumns:\n", t.Name, t.Rows)
	for _, c := range t.Cols {
		fmt.Fprintf(&b, "- %s %s", c.Name, c.Type)
		if c.PK > 0 {
			b.WriteString(" PRIMARY KEY")
		}
		if c.NotNull {
			b.WriteString(" NOT NULL")
		}
		if c.Default.Valid {
			b.WriteString(" DEFAULT " + c.Default.String)
		}
		if k := p.Annot[t.Name+"."+c.Name]; k != "" {
			b.WriteString(" kind=" + k)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "primary key: (%s)\n", strings.Join(t.PK, ", "))
	for _, ix := range t.Indexes {
		if ix.Origin == "pk" {
			continue
		}
		u := ""
		if ix.Unique {
			u = "unique "
		}
		fmt.Fprintf(&b, "%sindex %s (%s)\n", u, ix.Name, strings.Join(ix.Cols, ", "))
	}
	return b.String()
}

// runQuery is the query tool (SPEC 8.1): guard, then up to 200 rows or about 4,000 tokens, plus the total.
func (p *Project) runQuery(q string, params map[string]any) string {
	if _, msg := guardSQL(p.R, q); msg != "" {
		return "error: " + msg
	}
	var args []any
	names, _ := namedParams(q)
	for _, n := range names {
		args = append(args, sql.Named(n, params[n]))
	}
	ctx, cancel := contextTimeout(10 * time.Second)
	defer cancel()
	rows, err := p.R.QueryContext(ctx, "SELECT * FROM ("+strings.TrimRight(strings.TrimSpace(q), ";")+") LIMIT 201", args...)
	if err != nil {
		return "error: SQLite: " + cleanSQLiteErr(err)
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	var b strings.Builder
	b.WriteString("columns: " + strings.Join(cols, ", ") + "\n")
	n := 0
	for rows.Next() {
		n++
		if n > 200 || b.Len() > 14000 {
			continue
		}
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		rows.Scan(ptrs...)
		m := map[string]any{}
		for i, c := range cols {
			m[c] = vals[i]
		}
		j, _ := json.Marshal(m)
		b.Write(j)
		b.WriteString("\n")
	}
	var total int
	p.R.QueryRowContext(bg, "SELECT count(*) FROM ("+strings.TrimRight(strings.TrimSpace(q), ";")+")", args...).Scan(&total)
	fmt.Fprintf(&b, "(%d rows total)\n", total)
	return b.String()
}

func contextTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(bg, d)
}

// cardDeps lists which configs use which tables in the card (SPIKE-021 guide v2 runs only;
// SPEC 8.2 gives the schema agent the dependents).
var cardDeps bool
