package main

// A mock dry run (SPEC 6.8 with fake network and LLM) used only by the task assertions: the
// pipeline runs against a VACUUM INTO copy of the project, http.get and web.search return
// synthetic data (no request ever leaves the process), llm.select picks the first items and fills
// the add fields. Views are "opened" with their filter defaults, forms are submitted with test values.

import (
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

type mockRun struct {
	conn    *sql.Conn
	db      *sql.DB
	path    string
	sb      *Sandbox
	runID   string
	Log     []string
	Err     string
	Fetched []string // URLs "fetched"
	Queries []string // web.search queries
	loc     *time.Location
}

// copyProject makes a writable copy of the project database.
func copyProject(p *Project) (*sql.DB, *sql.Conn, string, error) {
	path := filepath.Join(p.Dir, fmt.Sprintf("copy-%d.db", time.Now().UnixNano()))
	// VACUUM INTO attaches the target internally; the writer's ATTACHED limit is 0.
	modernc.Limit(p.W, 7, 1)
	_, err := p.W.ExecContext(bg, "VACUUM INTO ?", path)
	modernc.Limit(p.W, 7, 0)
	if err != nil {
		return nil, nil, "", err
	}
	db, c, err := openConn(path, false)
	return db, c, path, err
}

func (r *mockRun) close() {
	if r.conn != nil {
		r.conn.Close()
		r.db.Close()
	}
	os.Remove(r.path)
	os.Remove(r.path + "-wal")
	os.Remove(r.path + "-shm")
}

// normVal turns database and JSON values into what the sandbox expects (int, float64, string...).
func normVal(v any) any {
	switch t := v.(type) {
	case int64:
		return int(t)
	case int32:
		return int(t)
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return int(i)
		}
		f, _ := t.Float64()
		return f
	case []byte:
		return string(t)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, x := range t {
			out[k] = normVal(x)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = normVal(x)
		}
		return out
	}
	return v
}

func jsonVal(s string) any {
	d := json.NewDecoder(strings.NewReader(s))
	d.UseNumber()
	var v any
	d.Decode(&v)
	return normVal(v)
}

func (r *mockRun) eval(src string, env map[string]any) (any, error) {
	prog, err := r.sb.Compile(src, env)
	if err != nil {
		return nil, err
	}
	v, err := r.sb.Run(prog, env)
	return normVal(v), err
}

func textOf(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", t), "0"), ".")
	case map[string]any, []any:
		b, _ := json.Marshal(t)
		return string(b)
	}
	return fmt.Sprint(v)
}

func (r *mockRun) value(v any, env map[string]any) (any, error) {
	switch t := v.(type) {
	case string:
		exprs, whole, bad := exprSegments(t)
		if bad != "" {
			return nil, fmt.Errorf("%s", bad)
		}
		if len(exprs) == 0 {
			return t, nil
		}
		if whole {
			return r.eval(exprs[0], env)
		}
		out := t
		for _, x := range exprs {
			val, err := r.eval(x, env)
			if err != nil {
				return nil, err
			}
			i := strings.Index(out, "${{")
			j := strings.Index(out[i:], "}}")
			out = out[:i] + textOf(val) + out[i+j+2:]
		}
		return out, nil
	case map[string]any:
		out := map[string]any{}
		for k, x := range t {
			y, err := r.value(x, env)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", k, err)
			}
			out[k] = y
		}
		return out, nil
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			y, err := r.value(x, env)
			if err != nil {
				return nil, err
			}
			out[i] = y
		}
		return out, nil
	}
	return v, nil
}

// ---- synthetic network and LLM ----

var mockPrices = map[string]float64{"BTC": 65000.5, "ETH": 3200.25}

func (r *mockRun) httpGet(u string, query map[string]any, failOnStatus bool) (map[string]any, error) {
	full := u
	for k, v := range query {
		full += fmt.Sprintf(" %s=%v", k, v)
	}
	r.Fetched = append(r.Fetched, full)
	low := strings.ToLower(full)
	coin := ""
	switch {
	case !strings.Contains(low, "api.example.com"):
	case strings.Contains(low, "eth"):
		coin = "ETH"
	case strings.Contains(low, "btc") || strings.Contains(low, "bitcoin"):
		coin = "BTC"
	}
	if coin == "" {
		if failOnStatus {
			return nil, fmt.Errorf("GET %s: status 404", u)
		}
		return map[string]any{"status": 404, "headers": map[string]any{}, "body": "not found", "json": nil}, nil
	}
	body := fmt.Sprintf(`{"coin":%q,"date":%q,"close":%v,"currency":"USD"}`, coin, time.Now().In(r.loc).Format("2006-01-02"), mockPrices[coin])
	return map[string]any{"status": 200, "headers": map[string]any{"content-type": "application/json"}, "body": body, "json": jsonVal(body)}, nil
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func (r *mockRun) webSearch(q string, limit int) []any {
	r.Queries = append(r.Queries, q)
	low := strings.ToLower(q)
	var coins []string
	if strings.Contains(low, "bitcoin") || strings.Contains(low, "btc") {
		coins = append(coins, "Bitcoin")
	}
	if strings.Contains(low, "ethereum") || strings.Contains(low, "eth") {
		coins = append(coins, "Ethereum")
	}
	if len(coins) == 0 {
		coins = []string{"Crypto"}
	}
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	slug := strings.Trim(slugRe.ReplaceAllString(low, "-"), "-")
	if len(slug) > 40 {
		slug = slug[:40]
	}
	var out []any
	for i := 0; i < limit; i++ {
		c := coins[i%len(coins)]
		out = append(out, map[string]any{
			"title":     fmt.Sprintf("%s story %d", c, i+1),
			"url":       fmt.Sprintf("https://news.example.com/%s/%d", slug, i+1),
			"snippet":   fmt.Sprintf("Synthetic snippet %d about %s.", i+1, c),
			"published": time.Now().UTC().Format(time.RFC3339),
		})
	}
	return out
}

func (r *mockRun) llmSelect(items []any, count int, add map[string]any) map[string]any {
	n := min(count, len(items))
	var out, idx []any
	for i := 0; i < n; i++ {
		src := asMap(items[i])
		it := map[string]any{}
		for k, v := range src {
			it[k] = v
		}
		if src == nil {
			it["value"] = items[i]
		}
		for k, t := range add {
			switch asStr(t) {
			case "number":
				it[k] = i + 1
			case "boolean":
				it[k] = true
			default:
				if strings.Contains(strings.ToLower(k), "coin") || strings.Contains(strings.ToLower(k), "symbol") {
					title := strings.ToLower(asStr(src["title"]))
					if strings.Contains(title, "ethereum") {
						it[k] = "ETH"
					} else {
						it[k] = "BTC"
					}
				} else {
					it[k] = fmt.Sprintf("mock %s %d", k, i+1)
				}
			}
		}
		out = append(out, it)
		idx = append(idx, i)
	}
	return map[string]any{"items": out, "indexes": idx}
}

// ---- database steps ----

func (r *mockRun) tableCols(t string) (map[string]bool, []string, error) {
	s, err := loadSchema(r.conn)
	if err != nil {
		return nil, nil, err
	}
	tb := s[t]
	if tb == nil {
		return nil, nil, fmt.Errorf("no such table: %s", t)
	}
	m := map[string]bool{}
	for _, c := range tb.Cols {
		m[c.Name] = true
	}
	return m, tb.PK, nil
}

func (r *mockRun) insert(table string, rows []any, onConflict string, ck []string) (map[string]any, error) {
	cols, pk, err := r.tableCols(table)
	if err != nil {
		return nil, err
	}
	if len(ck) == 0 {
		ck = pk
	}
	ins, upd, skip := 0, 0, 0
	now := time.Now().UTC().Format(time.RFC3339)
	for _, row := range rows {
		m := asMap(row)
		if m == nil {
			return nil, fmt.Errorf("row is not an object: %v", row)
		}
		rm := map[string]any{}
		for k, v := range m {
			rm[k] = v
		}
		if cols["_run_id"] && cols["_fetched_at"] {
			rm["_run_id"], rm["_fetched_at"] = r.runID, now
		}
		var names, ph, set []string
		var args []any
		for _, k := range sortedKeys(rm) {
			if !cols[k] {
				return nil, fmt.Errorf("table %s has no column named %s", table, k)
			}
			names = append(names, qi(k))
			ph = append(ph, "?")
			v := rm[k]
			if _, ok := v.(map[string]any); ok {
				v = textOf(v)
			} else if _, ok := v.([]any); ok {
				v = textOf(v)
			}
			args = append(args, v)
			isKey := false
			for _, c := range ck {
				if c == k {
					isKey = true
				}
			}
			if !isKey {
				set = append(set, qi(k)+" = excluded."+qi(k))
			}
		}
		var qk []string
		for _, c := range ck {
			qk = append(qk, qi(c))
		}
		stmt := "INSERT INTO " + qi(table) + " (" + strings.Join(names, ", ") + ") VALUES (" + strings.Join(ph, ", ") + ")"
		switch onConflict {
		case "ignore":
			stmt += " ON CONFLICT DO NOTHING"
		case "upsert":
			if len(set) == 0 {
				stmt += " ON CONFLICT (" + strings.Join(qk, ", ") + ") DO NOTHING"
			} else {
				stmt += " ON CONFLICT (" + strings.Join(qk, ", ") + ") DO UPDATE SET " + strings.Join(set, ", ")
			}
		}
		var before int
		r.conn.QueryRowContext(bg, "SELECT total_changes()").Scan(&before)
		res, err := r.conn.ExecContext(bg, stmt, args...)
		if err != nil {
			return nil, fmt.Errorf("insert into %s: %s", table, cleanSQLiteErr(err))
		}
		n, _ := res.RowsAffected()
		switch {
		case n == 0:
			skip++
		case onConflict == "upsert":
			upd++ // counted together; the mock does not tell inserts from updates
		default:
			ins++
		}
	}
	return map[string]any{"inserted": ins, "updated": upd, "skipped": skip, "id": nil}, nil
}

func (r *mockRun) query(q string, params map[string]any) (map[string]any, error) {
	names, _ := namedParams(q)
	var args []any
	for _, n := range names {
		args = append(args, sql.Named(n, params[n]))
	}
	rows, err := r.conn.QueryContext(bg, q, args...)
	if err != nil {
		return nil, fmt.Errorf("db.query: %s", cleanSQLiteErr(err))
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	var out []any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		rows.Scan(ptrs...)
		m := map[string]any{}
		for i, c := range cols {
			m[c] = normVal(vals[i])
		}
		out = append(out, m)
	}
	return map[string]any{"rows": out, "count": len(out)}, nil
}

// ---- the run ----

// runPipelineMock runs the pipeline `times` times on one copy of the database (the second run
// checks that a rerun is safe). The caller closes the result.
func runPipelineMock(p *Project, id string, times int) *mockRun {
	r := &mockRun{}
	c := p.Configs[id]
	if c == nil || c.Kind != "pipeline" {
		r.Err = "pipeline " + id + " not saved"
		return r
	}
	var err error
	r.db, r.conn, r.path, err = copyProject(p)
	if err != nil {
		r.Err = "copy: " + err.Error()
		return r
	}
	r.loc = time.UTC
	if tz := asStr(asMap(c.Val["trigger"])["timezone"]); tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			r.loc = l
		}
	}
	r.sb = NewSandbox(r.loc)
	for n := 1; n <= times && r.Err == ""; n++ {
		r.runID = fmt.Sprintf("run_mock_%d", n)
		r.runOnceAll(c, n)
	}
	return r
}

func (r *mockRun) runOnceAll(c *Config, n int) {
	inputs := map[string]any{}
	for k, v := range asMap(c.Val["inputs"]) {
		inputs[k] = normVal(asMap(v)["default"])
	}
	steps := map[string]any{}
	onErr := asStr(c.Val["on_error"])
	for i, s := range asList(c.Val["steps"]) {
		m := asMap(s)
		sid := asStr(m["id"])
		env := map[string]any{"steps": steps, "inputs": inputs, "secrets": map[string]any{}, "item": nil}
		out, err := r.runStep(m, env)
		if err != nil {
			r.Log = append(r.Log, fmt.Sprintf("run %d steps[%d] %s: failed: %v", n, i, sid, err))
			if m["continue_on_error"] == true || onErr == "continue" {
				steps[sid] = map[string]any{}
				continue
			}
			r.Err = fmt.Sprintf("run %d steps[%d] (%s) failed: %v", n, i, sid, err)
			return
		}
		steps[sid] = out
		r.Log = append(r.Log, fmt.Sprintf("run %d steps[%d] %s: ok %s", n, i, sid, short(textOf(out), 160)))
	}
}

func (r *mockRun) runStep(m map[string]any, env map[string]any) (any, error) {
	if s, ok := m["if"].(string); ok {
		v, err := r.value(s, env)
		if err != nil {
			return nil, fmt.Errorf("if: %w", err)
		}
		if v == false || v == nil {
			return map[string]any{"skipped": true}, nil
		}
	}
	fe, ok := m["for_each"].(string)
	if !ok {
		return r.runOnce(m, env)
	}
	lv, err := r.value(fe, env)
	if err != nil {
		return nil, fmt.Errorf("for_each: %w", err)
	}
	items, ok := lv.([]any)
	if !ok {
		return nil, fmt.Errorf("for_each did not return a list")
	}
	var each []any
	for _, it := range items {
		e2 := map[string]any{}
		for k, v := range env {
			e2[k] = v
		}
		e2["item"] = it
		out, err := r.runOnce(m, e2)
		if err != nil {
			return nil, err
		}
		entry := map[string]any{"item": it}
		for k, v := range asMap(out) {
			entry[k] = v
		}
		each = append(each, entry)
	}
	return map[string]any{"each": each, "count": len(each)}, nil
}

func toInt(v any, def int) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	}
	return def
}

func (r *mockRun) runOnce(m map[string]any, env map[string]any) (any, error) {
	use := asStr(m["use"])
	raw := asMap(m["with"])
	// per-item fields are not evaluated up front
	with := map[string]any{}
	for k, v := range raw {
		if (use == "transform.map" && k == "fields") || (use == "transform.filter" && k == "condition") {
			continue
		}
		x, err := r.value(v, env)
		if err != nil {
			return nil, fmt.Errorf("with.%s: %w", k, err)
		}
		with[k] = x
	}
	switch use {
	case "http.get":
		fail := true
		if b, ok := with["fail_on_status"].(bool); ok {
			fail = b
		}
		return r.httpGet(asStr(with["url"]), asMap(with["query"]), fail)
	case "web.search":
		return map[string]any{"results": r.webSearch(asStr(with["query"]), toInt(with["limit"], 10))}, nil
	case "llm.select":
		items, ok := with["items"].([]any)
		if !ok {
			return nil, fmt.Errorf("llm.select: items is not a list")
		}
		return r.llmSelect(items, toInt(with["count"], 5), asMap(raw["add"])), nil
	case "transform.map", "transform.filter":
		items, ok := with["items"].([]any)
		if !ok {
			return nil, fmt.Errorf("%s: items is not a list", use)
		}
		var out []any
		for _, it := range items {
			e2 := map[string]any{}
			for k, v := range env {
				e2[k] = v
			}
			e2["item"] = it
			if use == "transform.map" {
				row := map[string]any{}
				for k, v := range asMap(raw["fields"]) {
					x, err := r.value(v, e2)
					if err != nil {
						return nil, fmt.Errorf("fields.%s: %w", k, err)
					}
					row[k] = x
				}
				out = append(out, row)
			} else {
				x, err := r.value(raw["condition"], e2)
				if err != nil {
					return nil, fmt.Errorf("condition: %w", err)
				}
				if x == true {
					out = append(out, it)
				}
			}
		}
		return map[string]any{"items": out}, nil
	case "db.query":
		return r.query(asStr(with["sql"]), asMap(with["params"]))
	case "db.insert":
		row := with["row"]
		return r.insert(asStr(with["table"]), []any{row}, asStr(with["on_conflict"]), strList(with["conflict_key"]))
	case "db.insert_many":
		rows, ok := with["rows"].([]any)
		if !ok {
			return nil, fmt.Errorf("db.insert_many: rows is not a list")
		}
		return r.insert(asStr(with["table"]), rows, asStr(with["on_conflict"]), strList(with["conflict_key"]))
	case "db.update":
		var set []string
		var args []any
		for _, k := range sortedKeys(asMap(with["set"])) {
			set = append(set, qi(k)+" = ?")
			args = append(args, asMap(with["set"])[k])
		}
		w := asStr(with["where"])
		names, _ := namedParams(w)
		for _, n := range names {
			args = append(args, sql.Named(n, asMap(with["params"])[n]))
		}
		res, err := r.conn.ExecContext(bg, "UPDATE "+qi(asStr(with["table"]))+" SET "+strings.Join(set, ", ")+" WHERE "+w, args...)
		if err != nil {
			return nil, err
		}
		n, _ := res.RowsAffected()
		return map[string]any{"updated": int(n)}, nil
	case "json.extract":
		return map[string]any{"value": jsonPath(with["input"], asStr(with["path"]))}, nil
	case "html.extract":
		return map[string]any{"title": "Mock page", "text": "Mock readable text.", "links": []any{}, "status": "ok"}, nil
	case "llm.extract":
		d := map[string]any{}
		for k, t := range asMap(raw["output"]) {
			switch asStr(t) {
			case "number":
				d[k] = 0
			case "boolean":
				d[k] = false
			case "list":
				d[k] = []any{}
			default:
				d[k] = "mock"
			}
		}
		return map[string]any{"data": d}, nil
	case "app.notify":
		return map[string]any{}, nil
	case "bucket.put":
		return map[string]any{"key": with["key"], "size": len(textOf(with["content"]))}, nil
	case "bucket.list":
		return map[string]any{"objects": []any{}}, nil
	case "http.download":
		return map[string]any{"key": with["key"], "size": 1234, "mime": "image/png"}, nil
	}
	return nil, fmt.Errorf("%s is not supported by the mock run", use)
}

// jsonPath supports $.a.b, $.a[0].b and $.a[*].b.
func jsonPath(v any, path string) any {
	p := strings.TrimPrefix(path, "$")
	cur := []any{v}
	for p != "" {
		var part string
		if strings.HasPrefix(p, ".") {
			p = p[1:]
			end := strings.IndexAny(p, ".[")
			if end < 0 {
				end = len(p)
			}
			part, p = p[:end], p[end:]
			var next []any
			for _, c := range cur {
				if m := asMap(c); m != nil {
					if x, ok := m[part]; ok {
						next = append(next, x)
					}
				}
			}
			cur = next
		} else if strings.HasPrefix(p, "[") {
			end := strings.Index(p, "]")
			if end < 0 {
				return nil
			}
			part, p = p[1:end], p[end+1:]
			var next []any
			for _, c := range cur {
				l := asList(c)
				if part == "*" {
					next = append(next, l...)
				} else if i := toInt(jsonVal(part), -1); i >= 0 && i < len(l) {
					next = append(next, l[i])
				}
			}
			cur = next
		} else {
			return nil
		}
	}
	if len(cur) == 1 && !strings.Contains(path, "*") {
		return cur[0]
	}
	if len(cur) == 0 {
		return nil
	}
	return cur
}

// ---- views and forms ----

// openView runs a view query as the runtime would (filter defaults bound, sort, first page).
func openView(q Q, v map[string]any, overrides map[string]any) ([]map[string]any, error) {
	query := strings.TrimRight(strings.TrimSpace(asStr(v["query"])), ";")
	vals := map[string]any{}
	for k, x := range asMap(v["params"]) {
		vals[k] = x
	}
	now := time.Now().UTC()
	for _, f := range asList(v["filters"]) {
		fm := asMap(f)
		p := asStr(fm["param"])
		d := fm["default"]
		if s, ok := d.(string); ok && asStr(fm["control"]) == "date" {
			d = resolveDateDefault(s, now)
		}
		vals[p] = d
	}
	for k, x := range overrides {
		vals[k] = x
	}
	var wrapped string
	switch asStr(v["type"]) {
	case "table":
		ds := asMap(v["default_sort"])
		dir := "ASC"
		if asStr(ds["direction"]) == "desc" {
			dir = "DESC"
		}
		ps := toInt(v["page_size"], 50)
		wrapped = fmt.Sprintf("SELECT * FROM (%s) ORDER BY %s %s LIMIT %d OFFSET 0", query, qi(asStr(ds["field"])), dir, ps)
	default:
		wrapped = "SELECT * FROM (" + query + ") LIMIT 10001"
	}
	names, _ := namedParams(query)
	var args []any
	for _, n := range names {
		args = append(args, sql.Named(n, vals[n]))
	}
	rows, err := q.QueryContext(bg, wrapped, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	var out []map[string]any
	for rows.Next() {
		vs := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vs {
			ptrs[i] = &vs[i]
		}
		rows.Scan(ptrs...)
		m := map[string]any{}
		for i, c := range cols {
			m[c] = normVal(vs[i])
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// submitForm inserts a form's values the way the writer would (typed insert).
func submitForm(q Q, v map[string]any, values map[string]any) error {
	sub := asMap(v["submit"])
	if asStr(sub["action"]) != "insert" {
		return fmt.Errorf("submit action is %q, not insert", asStr(sub["action"]))
	}
	var cols, ph []string
	var args []any
	var keys []string
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		cols = append(cols, qi(k))
		ph = append(ph, "?")
		args = append(args, values[k])
	}
	_, err := q.ExecContext(bg, "INSERT INTO "+qi(asStr(sub["table"]))+" ("+strings.Join(cols, ", ")+") VALUES ("+strings.Join(ph, ", ")+")", args...)
	return err
}
