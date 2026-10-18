package sqlguard

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"

	"modernc.org/sqlite"
)

// Guard is layer 2, the EXPLAIN check (2.2). It keeps one database file's
// map from root pages to table names, and reads it again when the file's
// schema_version changes. Use one Guard per file; it is safe for
// concurrent use.
type Guard struct {
	mu      sync.Mutex
	version int64
	tables  map[int64]string // root page → table name, for tables and their indexes
}

func New() *Guard { return &Guard{} }

// Explain is layer 2. It runs Check first, so it never prepares text that
// failed layer 1, makes sure SQLite reads the query as one statement, then
// compiles EXPLAIN q with NULL bound to every parameter and reads the
// program. It refuses a query that opens a _jenab_ or sqlite_ table or
// sqlite_schema, reads the temp or an attached schema, opens a virtual
// table other than json_each and json_tree, or writes. This catches reads
// through CTEs, subqueries, IN table and index-only lookups. Run it on the
// reader the query will run on, inside the same read transaction, so both
// see the same schema. SQLite's own errors (no such column) come back as
// they are.
func (g *Guard) Explain(ctx context.Context, c *sql.Conn, q string) error {
	ck, rerr := check(q)
	if rerr != nil {
		return rerr
	}
	tables, err := g.tableMap(ctx, c)
	if err != nil {
		return err
	}
	body := ck.body()
	if err := oneStatement(c, body); err != nil {
		return err
	}
	var vtabs map[string]bool
	if ck.vtab {
		if vtabs, err = jsonTables(ctx, c); err != nil {
			return err
		}
	}
	return program(ctx, c, body, ck.nullArgs(), tables, vtabs)
}

// oneStatement makes sure SQLite reads body as one statement, whatever
// layer 1 made of it, before anything runs it: modernc runs every
// statement of a text it is given, EXPLAIN or not. It prepares, without
// running, SELECT (SELECT 1 FROM (body)), and SQLite names that column
// after the text of its expression: if the name is all of it, the
// statement SQLite read reaches the end of the text, so there is no ';'
// inside. Layer 1 refuses ';' already; this only guards against SQLite
// reading the text differently (a hidden ';' in what layer 1 took for a
// name or a parameter).
func oneStatement(c *sql.Conn, body string) error {
	expr := "(SELECT 1 FROM (" + body + "\n))"
	name, err := firstColumn(c, "SELECT "+expr)
	if err == nil && name == expr {
		return nil
	}
	// A query that prepares on its own but not inside a SELECT is a write
	// (WITH … DELETE) or more than one statement; anything else is SQLite's
	// own error, which reads better from the query alone.
	if _, err := firstColumn(c, body); err != nil {
		return err
	}
	return &RejectError{Layer: 2, Pos: -1,
		Reason: "only one read query is allowed; SQLite reads this as a write or as more than one statement"}
}

// firstColumn prepares the first statement of q, without running it, and
// returns the name of its first column.
func firstColumn(c *sql.Conn, q string) (name string, err error) {
	err = c.Raw(func(dc any) error {
		ci, ok := dc.(interface {
			ColumnInfo(string) ([]sqlite.ColumnInfo, error)
		})
		if !ok {
			return fmt.Errorf("sqlguard: driver %T can't prepare a query without running it", dc)
		}
		info, err := ci.ColumnInfo(q)
		if len(info) > 0 {
			name = info[0].Name
		}
		return err
	})
	return name, err
}

// jsonTables returns the virtual tables json_each and json_tree open on
// c, as EXPLAIN shows them: "vtab:" and the table's address. Both are
// eponymous: a connection makes each once and keeps it until it closes,
// so no other virtual table can have the same address while c lives.
func jsonTables(ctx context.Context, c *sql.Conn) (map[string]bool, error) {
	m := map[string]bool{}
	err := scanProgram(ctx, c, "SELECT * FROM json_each('[]'), json_tree('[]')", nil, func(op, p4 string, _, _, _ int64) string {
		if op == "VOpen" {
			m[p4] = true
		}
		return ""
	})
	if err == nil && len(m) != 2 {
		err = fmt.Errorf("sqlguard: json_each and json_tree open %d virtual tables, want 2", len(m))
	}
	return m, err
}

// program reads the program of EXPLAIN body, which must be one statement,
// and refuses it if a step does something agent queries may not. vtabs
// are the virtual tables it may open.
func program(ctx context.Context, c *sql.Conn, body string, args []any, tables map[int64]string, vtabs map[string]bool) error {
	return scanProgram(ctx, c, body, args, func(op, p4 string, p2, p3, p5 int64) string {
		return refusal(op, p2, p3, p5, p4, tables, vtabs)
	})
}

func scanProgram(ctx context.Context, c *sql.Conn, body string, args []any, step func(op, p4 string, p2, p3, p5 int64) string) error {
	rows, err := c.QueryContext(ctx, "EXPLAIN "+body, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var addr, p1, p2, p3, p5 sql.NullInt64
		var op, p4, comment sql.NullString
		if err := rows.Scan(&addr, &op, &p1, &p2, &p3, &p4, &p5, &comment); err != nil {
			return err
		}
		if why := step(op.String, p4.String, p2.Int64, p3.Int64, p5.Int64); why != "" {
			return &RejectError{Layer: 2, Reason: why, Pos: -1}
		}
	}
	return rows.Err()
}

// writeOps change a database, a schema or the connection. Row opcodes
// (Insert, Delete, IdxInsert) are not here: CTEs, DISTINCT and sorting use
// them on temporary tables, and writing a real table needs OpenWrite.
var writeOps = map[string]bool{
	"OpenWrite": true, "CreateBtree": true, "Destroy": true, "Clear": true,
	"DropTable": true, "DropIndex": true, "DropTrigger": true, "ParseSchema": true,
	"SetCookie": true, "Expire": true, "Vacuum": true, "IncrVacuum": true,
	"JournalMode": true, "Checkpoint": true, "LoadAnalysis": true, "SqlExec": true,
	"AutoCommit": true, "Savepoint": true, "MaxPgcnt": true,
	"VUpdate": true, "VCreate": true, "VDestroy": true, "VBegin": true, "VRename": true,
}

// funcOps call a function named in P4 as name(args).
var funcOps = map[string]bool{
	"Function": true, "PureFunc": true, "AggStep": true, "AggStep1": true,
	"AggFinal": true, "AggValue": true, "AggInverse": true,
}

// p2IsReg is OPFLAG_P2ISREG: an Open opcode takes its root page from a
// register, known only when it runs.
const p2IsReg = 0x10

// refusal says why one step of the program refuses the query, or "".
func refusal(op string, p2, p3, p5 int64, p4 string, tables map[int64]string, vtabs map[string]bool) string {
	switch {
	case op == "Transaction" && p2 != 0, writeOps[op]:
		return "only read queries are allowed; this one would change the database"
	case op == "OpenRead" || op == "ReopenIdx":
		if p3 != 0 {
			return "the query reads the temp schema or another database file"
		}
		if p5&p2IsReg != 0 {
			return "the query opens a table chosen while it runs"
		}
		name, ok := tables[p2]
		if !ok {
			return "`sqlite_schema` is reserved" // page 1, or a page no table owns
		}
		switch n := lower(name); {
		case strings.HasPrefix(n, "_jenab_"):
			return "`" + name + "` is internal"
		case strings.HasPrefix(n, "sqlite_"):
			return "`" + name + "` is reserved"
		}
	case op == "VOpen" && !vtabs[p4]:
		return "json_each(...) and json_tree(...) are the only virtual tables a query can use"
	case funcOps[op]:
		// The function list is an allow-list here: layer 1 can't tell a
		// function from a column, the program can.
		if f, _, _ := strings.Cut(p4, "("); !allowedFuncs[lower(f)] {
			return "`" + f + "` is not allowed"
		}
	}
	return ""
}

// tableMap returns the root page map for c's file, read again when its
// schema_version has changed.
func (g *Guard) tableMap(ctx context.Context, c *sql.Conn) (map[int64]string, error) {
	var v int64
	if err := c.QueryRowContext(ctx, "PRAGMA schema_version").Scan(&v); err != nil {
		return nil, err
	}
	g.mu.Lock()
	if g.tables != nil && g.version == v {
		m := g.tables
		g.mu.Unlock()
		return m, nil
	}
	g.mu.Unlock()
	rows, err := c.QueryContext(ctx, "SELECT rootpage, tbl_name FROM sqlite_schema WHERE rootpage > 0")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[int64]string{}
	for rows.Next() {
		var page int64
		var name string
		if err := rows.Scan(&page, &name); err != nil {
			return nil, err
		}
		m[page] = name
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	g.mu.Lock()
	g.version, g.tables = v, m
	g.mu.Unlock()
	return m, nil
}
