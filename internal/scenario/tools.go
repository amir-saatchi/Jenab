package scenario

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite" // the fixture database, as the store's

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// realOff are the real tools a run leaves out: fetch_page calls the
// internet. A set fakes it when its scenarios need it.
var realOff = []string{"fetch_page"}

// tools are the run's tools: the real Phase 1 tools that need no network,
// with the set's fakes in place of real ones of the same name.
func (r *run) tools() *tool.Registry {
	reg := tool.NewRegistry()
	for _, t := range append(tool.Builtin(tool.Deps{}), agent.Tools()...) {
		n := t.Spec().Name
		if !slices.Contains(realOff, n) && r.sc.set.tool(n) == nil {
			reg.Add(t)
		}
	}
	for _, t := range r.sc.set.Tools {
		reg.Add(r.fake(t))
	}
	return reg
}

// fake makes a fake tool. Its calls take the tool's delay, scaled.
func (r *run) fake(t *ToolSpec) tool.Tool {
	spec := tool.Spec{Name: t.Name, Description: t.Description, Schema: t.schema, Mother: t.Mother}
	if t.Kind != KindRules {
		spec.Effects = tool.ReadsDB
	}
	return tool.Func(spec, func(ctx context.Context, env *tool.Env, args json.RawMessage) (tool.Result, error) {
		raw := string(args)
		var res tool.Result
		var err error
		var rule *Rule
		switch t.Kind {
		case KindSQL:
			res, err = r.query(ctx, argText(args, t.Arg))
		case KindDescribe:
			res, err = r.describe(ctx, argText(args, t.Arg))
		case KindConfig:
			res, err = r.config(argText(args, t.Arg))
		default:
			rule = r.sc.rule(t.Name, raw)
			switch {
			case rule == nil:
				res = tool.Result{Text: "no results"}
			case rule.Error:
				err = &tool.Error{Msg: strings.TrimSpace(rule.Result)}
			default:
				res = tool.Result{Text: strings.TrimSpace(rule.Result)}
			}
		}
		if err := r.sleep(ctx, r.sc.delay(t, rule)); err != nil {
			return tool.Result{}, err
		}
		return res, err
	})
}

// sleep waits d, scaled, or until ctx ends.
func (r *run) sleep(ctx context.Context, d time.Duration) error {
	d = time.Duration(float64(d) * r.opt.Scale)
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// argText is a call's argument as text; a number or an object is its JSON.
func argText(args json.RawMessage, name string) string {
	var m map[string]json.RawMessage
	if json.Unmarshal(args, &m) != nil {
		return ""
	}
	v := m[name]
	var s string
	if json.Unmarshal(v, &s) == nil {
		return s
	}
	return string(v)
}

// isBackground reports whether a call of t starts background work.
func isBackground(t *ToolSpec, args string) bool {
	if t == nil {
		return false
	}
	if t.Background {
		return true
	}
	if t.BackgroundArg == "" {
		return false
	}
	var m map[string]any
	if json.Unmarshal([]byte(args), &m) != nil {
		return false
	}
	switch v := m[t.BackgroundArg].(type) {
	case bool:
		return v
	case string:
		return v == "true"
	}
	return false
}

var dbSeq atomic.Int64

// openFixture opens a fresh in-memory database with the set's fixture,
// read-only once filled.
func openFixture(ctx context.Context, script string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", fmt.Sprintf("file:scenario%d?mode=memory&cache=shared", dbSeq.Add(1)))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, script); err != nil {
		db.Close()
		return nil, fmt.Errorf("fixture: %w", err)
	}
	if _, err := db.ExecContext(ctx, `PRAGMA query_only = ON`); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// Query limits, as the query tool's (8.1).
const maxRows = 200

var reSelect = regexp.MustCompile(`(?is)^\s*(select|with)\b`)

// query runs one read-only SELECT and returns up to maxRows rows and the
// count.
func (r *run) query(ctx context.Context, q string) (tool.Result, error) {
	if !reSelect.MatchString(q) {
		return tool.Result{}, tool.Errorf("only one read-only SELECT is allowed")
	}
	if strings.Contains(strings.TrimRight(strings.TrimSpace(q), ";"), ";") {
		return tool.Result{}, tool.Errorf("only one statement is allowed")
	}
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return tool.Result{}, tool.Errorf("%v", err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return tool.Result{}, tool.Errorf("%v", err)
	}
	var b strings.Builder
	b.WriteString(strings.Join(cols, " | ") + "\n")
	n := 0
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return tool.Result{}, tool.Errorf("%v", err)
		}
		if n++; n > maxRows {
			continue
		}
		cells := make([]string, len(vals))
		for i, v := range vals {
			switch x := v.(type) {
			case nil:
				cells[i] = "NULL"
			case []byte:
				cells[i] = string(x)
			case float64:
				cells[i] = fmt.Sprintf("%.2f", x)
			default:
				cells[i] = fmt.Sprint(x)
			}
		}
		b.WriteString(strings.Join(cells, " | ") + "\n")
	}
	if err := rows.Err(); err != nil {
		return tool.Result{}, tool.Errorf("%v", err)
	}
	fmt.Fprintf(&b, "(%d rows)", n)
	return tool.Result{Text: b.String()}, nil
}

// describe lists a table's columns, keys and row count.
func (r *run) describe(ctx context.Context, name string) (tool.Result, error) {
	var tables []string
	rows, err := r.db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return tool.Result{}, err
	}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return tool.Result{}, err
		}
		tables = append(tables, n)
	}
	rows.Close()
	if !slices.Contains(tables, name) {
		return tool.Result{}, tool.Errorf("table %q not found (tables: %s)", name, strings.Join(tables, ", "))
	}
	rows, err = r.db.QueryContext(ctx, `SELECT name, type, "notnull", pk FROM pragma_table_info(?)`, name)
	if err != nil {
		return tool.Result{}, err
	}
	defer rows.Close()
	var b strings.Builder
	fmt.Fprintf(&b, "table %s\n", name)
	for rows.Next() {
		var n, typ string
		var notNull, pk int
		if err := rows.Scan(&n, &typ, &notNull, &pk); err != nil {
			return tool.Result{}, err
		}
		fmt.Fprintf(&b, "- %s %s", n, typ)
		if notNull > 0 {
			b.WriteString(" NOT NULL")
		}
		if pk > 0 {
			fmt.Fprintf(&b, " (primary key %d)", pk)
		}
		b.WriteString("\n")
	}
	if err := rows.Err(); err != nil {
		return tool.Result{}, err
	}
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM "`+strings.ReplaceAll(name, `"`, `""`)+`"`).Scan(&count); err != nil {
		return tool.Result{}, err
	}
	fmt.Fprintf(&b, "rows: %d", count)
	return tool.Result{Text: b.String()}, nil
}

// config returns a stored config.
func (r *run) config(id string) (tool.Result, error) {
	if c, ok := r.sc.set.Configs[id]; ok {
		return tool.Result{Text: c}, nil
	}
	ids := make([]string, 0, len(r.sc.set.Configs))
	for k := range r.sc.set.Configs {
		ids = append(ids, k)
	}
	slices.Sort(ids)
	return tool.Result{}, tool.Errorf("no config %q (saved: %s)", id, strings.Join(ids, ", "))
}
