package main

// The agent SQL guard (SPEC 2.2, SPIKE-007): layer 1 text check in Go, then layer 2 EXPLAIN
// check, then the query runs on a guarded reader connection. Copied from SPIKE-007 and adapted
// to take any connection (reader, or the writer inside a migration).

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Q is what the checks need from a connection: *sql.Conn satisfies it.
type Q interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

var bg = context.Background()
type tok struct {
	kind byte // 'w' word, 'i' quoted identifier, 's' string, ';', 'p' other
	text string
}

func isWord(c byte) bool {
	return c == '_' || c == '$' || c >= 0x80 ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// lex splits SQL into tokens following SQLite's lexical rules for comments,
// strings and quoted identifiers.
func lex(s string) ([]tok, error) {
	var out []tok
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == 0:
			return nil, errors.New("NUL byte in query")
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f':
			i++
		case c == '-' && i+1 < len(s) && s[i+1] == '-':
			for i < len(s) && s[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			j := strings.Index(s[i+2:], "*/")
			if j < 0 {
				i = len(s) // SQLite accepts an unterminated comment at the end
			} else {
				i += j + 4
			}
		case c == '\'' || c == '"' || c == '`':
			var b strings.Builder
			j := i + 1
			for {
				if j >= len(s) {
					return nil, errors.New("unterminated quote")
				}
				if s[j] == c {
					if j+1 < len(s) && s[j+1] == c {
						b.WriteByte(c)
						j += 2
						continue
					}
					break
				}
				b.WriteByte(s[j])
				j++
			}
			kind := byte('i')
			if c == '\'' {
				kind = 's'
			}
			out = append(out, tok{kind, b.String()})
			i = j + 1
		case c == '[':
			j := strings.IndexByte(s[i+1:], ']')
			if j < 0 {
				return nil, errors.New("unterminated [")
			}
			out = append(out, tok{'i', s[i+1 : i+1+j]})
			i += j + 2
		case isWord(c):
			j := i
			for j < len(s) && isWord(s[j]) {
				j++
			}
			out = append(out, tok{'w', s[i:j]})
			i = j
		case c == ';':
			out = append(out, tok{';', ";"})
			i++
		default:
			out = append(out, tok{'p', string(c)})
			i++
		}
	}
	return out, nil
}

// allowedVtabs are the only table-valued functions agent queries may use.
var allowedVtabs = map[string]bool{"json_each": true, "json_tree": true}

// deniedFuncs are functions agent queries may never call.
var deniedFuncs = []string{"load_extension", "fts3_tokenizer", "sqlite_attach", "sqlite_detach"}

// textCheck returns a rejection reason ("" if allowed) and whether the query
// names an allowed virtual table.
func textCheck(q string, deniedNames map[string]bool) (reason string, vtab bool) {
	toks, err := lex(q)
	if err != nil {
		return err.Error(), false
	}
	if len(toks) == 0 {
		return "empty query", false
	}
	for i, t := range toks {
		if t.kind == ';' && i != len(toks)-1 {
			return "more than one statement", false
		}
	}
	if first := strings.ToLower(toks[0].text); toks[0].kind != 'w' || (first != "select" && first != "with" && first != "values") {
		return "must start with SELECT, WITH or VALUES", false
	}
	for _, t := range toks {
		if t.kind != 'w' && t.kind != 'i' {
			continue
		}
		n := strings.ToLower(t.text)
		if allowedVtabs[n] {
			vtab = true
		}
		if reason != "" {
			continue
		}
		switch {
		case strings.HasPrefix(n, "_burrow_"):
			reason = "internal name " + t.text
		case strings.HasPrefix(n, "sqlite_"):
			reason = "reserved name " + t.text
		case strings.HasPrefix(n, "pragma_"):
			reason = "pragma function " + t.text
		case deniedNames[n]:
			reason = "denied name " + t.text
		}
	}
	return reason, vtab
}

// ---------------------------------------------------------------- layer 2: EXPLAIN check

var writeOps = map[string]bool{
	"OpenWrite": true, "VUpdate": true, "VCreate": true, "VDestroy": true, "VBegin": true,
	"Destroy": true, "Clear": true, "DropTable": true, "DropIndex": true, "DropTrigger": true,
	"ParseSchema": true, "SetCookie": true, "Vacuum": true, "IncrVacuum": true,
	"JournalMode": true, "Checkpoint": true, "LoadAnalysis": true, "SqlExec": true,
}

// namedParams lists the named parameters (:name, @name, $name) of a query in order.
func namedParams(q string) (names []string, positional int) {
	toks, _ := lex(q)
	seen := map[string]bool{}
	for i, t := range toks {
		name := ""
		switch {
		case t.kind == 'p' && (t.text == ":" || t.text == "@") && i+1 < len(toks) && toks[i+1].kind == 'w':
			name = toks[i+1].text
		case t.kind == 'w' && strings.HasPrefix(t.text, "$") && len(t.text) > 1:
			name = t.text[1:]
		case t.kind == 'p' && t.text == "?":
			positional++
		}
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names, positional
}

// nullParams binds NULL to every parameter, so EXPLAIN can compile the query.
func nullParams(q string) []any {
	names, pos := namedParams(q)
	var args []any
	for i := 0; i < pos; i++ {
		args = append(args, nil)
	}
	for _, n := range names {
		args = append(args, sql.Named(n, nil))
	}
	return args
}

// rootMap maps root pages to table names (not cached here: the schema changes inside migrations).
func rootMap(c Q) (map[int64]string, error) {
	rows, err := c.QueryContext(bg, "SELECT rootpage, tbl_name FROM sqlite_schema WHERE rootpage > 0")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[int64]string{}
	for rows.Next() {
		var rp int64
		var name string
		if err := rows.Scan(&rp, &name); err != nil {
			return nil, err
		}
		m[rp] = name
	}
	return m, rows.Err()
}

// explainCheck reads the compiled program of the query without running it. A prepare error is
// returned as prepErr (syntax errors, unknown tables and columns).
func explainCheck(c Q, q string, allowVtab bool) (reason string, prepErr error) {
	roots, err := rootMap(c)
	if err != nil {
		return "", err
	}
	rows, err := c.QueryContext(bg, "EXPLAIN "+q, nullParams(q)...)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var addr, p1, p2, p3, p5 sql.NullInt64
		var opcode, p4, comment sql.NullString
		if err := rows.Scan(&addr, &opcode, &p1, &p2, &p3, &p4, &p5, &comment); err != nil {
			return "", err
		}
		if reason != "" {
			continue
		}
		op := opcode.String
		switch {
		case op == "Transaction" && p2.Int64 != 0:
			reason = "write transaction"
		case writeOps[op]:
			reason = "opcode " + op
		case op == "OpenRead" || op == "ReopenIdx":
			if p3.Int64 != 0 {
				reason = fmt.Sprintf("reads schema %d (temp or attached)", p3.Int64)
			} else if name, ok := roots[p2.Int64]; !ok {
				reason = "reads sqlite_schema"
			} else if strings.HasPrefix(strings.ToLower(name), "_burrow_") {
				reason = "reads internal table " + name
			}
		case op == "VOpen" && !allowVtab:
			reason = "virtual table"
		case op == "Function" || op == "PureFunc":
			f := strings.ToLower(p4.String)
			for _, d := range deniedFuncs {
				if strings.HasPrefix(f, d+"(") {
					reason = "calls " + d
				}
			}
		}
	}
	return reason, rows.Err()
}

// guardSQL runs layers 1 and 2 and returns the result column names. Errors are messages for the
// LLM; "" means the query passed.
func guardSQL(c Q, q string) (cols []string, msg string) {
	reason, vtab := textCheck(q, nil)
	if reason != "" {
		return nil, "rejected by the SQL guard: " + reason
	}
	r, err := explainCheck(c, q, vtab)
	if err != nil {
		return nil, "SQLite: " + cleanSQLiteErr(err)
	}
	if r != "" {
		return nil, "rejected by the SQL guard: " + r
	}
	cols, err = resultColumns(c, q)
	if err != nil {
		return nil, "SQLite: " + cleanSQLiteErr(err)
	}
	return cols, ""
}

// resultColumns prepares the query inside a LIMIT 0 wrapper and reads its column names.
func resultColumns(c Q, q string) ([]string, error) {
	rows, err := c.QueryContext(bg, "SELECT * FROM ("+strings.TrimRight(strings.TrimSpace(q), ";")+") LIMIT 0", nullParams(q)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return rows.Columns()
}

func cleanSQLiteErr(err error) string {
	s := err.Error()
	for _, p := range []string{"SQL logic error: ", "sqlite: "} {
		s = strings.ReplaceAll(s, p, "")
	}
	if i := strings.Index(s, " (1)"); i >= 0 {
		s = s[:i] + s[i+4:]
	}
	return s
}

var _ = errors.New
