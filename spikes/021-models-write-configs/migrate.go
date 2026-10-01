package main

// Structured migrations (SPEC 7.5, 8.2): Go validates each step against the schema inside the
// transaction, generates the DDL, runs the schema guard, validates all dependents (the ones sent
// with the request replace the stored ones) against the new schema, and commits only if
// everything passes. Destructive steps count as approved (the user asked for the change).

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

type migResult struct {
	OK          bool
	Issues      []Issue
	Destructive []string
	Updated     []string // dependents replaced
	Summary     []string
}

func qi(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }

func sqlLiteral(v any) string {
	switch t := v.(type) {
	case nil:
		return "NULL"
	case bool:
		if t {
			return "1"
		}
		return "0"
	case string:
		return "'" + strings.ReplaceAll(t, "'", "''") + "'"
	default:
		return fmt.Sprint(t)
	}
}

type migCtx struct {
	p     *Project
	is    *issues
	annot map[string]string
}

func (mc *migCtx) schema() map[string]*Table {
	s, err := loadSchema(mc.p.W)
	if err != nil {
		mc.is.add("", CatSemantic, "cannot read schema: %v", err)
	}
	return s
}

func reservedName(n string) bool {
	return strings.HasPrefix(n, "_burrow_") || strings.HasPrefix(n, "sqlite_")
}

// tableDef builds the column and constraint list for create_table / rebuild_table.
func (mc *migCtx) tableDef(ptr string, m map[string]any, s map[string]*Table) (string, bool) {
	ok := true
	cols := asList(m["columns"])
	names := map[string]string{}
	var colTypes = map[string]string{}
	for i, c := range cols {
		cm := asMap(c)
		n := asStr(cm["name"])
		if _, dup := names[strings.ToLower(n)]; dup {
			mc.is.add(fmt.Sprintf("%s/columns/%d/name", ptr, i), CatSemantic, "duplicate column %q", n)
			ok = false
		}
		names[strings.ToLower(n)] = n
		colTypes[strings.ToLower(n)] = strings.ToUpper(asStr(cm["type"]))
	}
	pk := strList(m["primary_key"])
	for _, k := range pk {
		if _, has := names[strings.ToLower(k)]; !has {
			mc.is.add(ptr+"/primary_key", CatRefs, "primary key column %q is not in columns", k)
			ok = false
		}
	}
	inlinePK := len(pk) == 1 && colTypes[strings.ToLower(pk[0])] == "INTEGER"
	var parts []string
	for i, c := range cols {
		cm := asMap(c)
		n := asStr(cm["name"])
		d := qi(n) + " " + strings.ToUpper(asStr(cm["type"]))
		if inlinePK && strings.EqualFold(n, pk[0]) {
			d += " PRIMARY KEY"
		}
		if cm["not_null"] == true {
			d += " NOT NULL"
		}
		if dv, has := cm["default"]; has {
			d += " DEFAULT " + sqlLiteral(dv)
		}
		if ck := asStr(cm["check"]); ck != "" {
			if why := checkExprText(ck); why != "" {
				mc.is.add(fmt.Sprintf("%s/columns/%d/check", ptr, i), CatSQL, "%s", why)
				ok = false
			}
			d += " CHECK (" + ck + ")"
		}
		parts = append(parts, d)
	}
	quoteList := func(l []string) string {
		var q []string
		for _, x := range l {
			q = append(q, qi(x))
		}
		return strings.Join(q, ", ")
	}
	if len(pk) > 0 && !inlinePK {
		parts = append(parts, "PRIMARY KEY ("+quoteList(pk)+")")
	}
	for i, u := range asList(m["unique"]) {
		ul := strList(u)
		for _, k := range ul {
			if _, has := names[strings.ToLower(k)]; !has {
				mc.is.add(fmt.Sprintf("%s/unique/%d", ptr, i), CatRefs, "unique column %q is not in columns", k)
				ok = false
			}
		}
		parts = append(parts, "UNIQUE ("+quoteList(ul)+")")
	}
	for i, f := range asList(m["foreign_keys"]) {
		fm := asMap(f)
		ref := asMap(fm["references"])
		rt := asStr(ref["table"])
		if s[rt] == nil && !strings.EqualFold(rt, asStr(m["table"])) {
			mc.is.add(fmt.Sprintf("%s/foreign_keys/%d/references/table", ptr, i), CatRefs, "table %q not found", rt)
			ok = false
		}
		d := "FOREIGN KEY (" + quoteList(strList(fm["columns"])) + ") REFERENCES " + qi(rt) + " (" + quoteList(strList(ref["columns"])) + ")"
		if od := asStr(fm["on_delete"]); od != "" {
			d += " ON DELETE " + strings.ToUpper(strings.ReplaceAll(od, "_", " "))
		}
		parts = append(parts, d)
	}
	return "(\n  " + strings.Join(parts, ",\n  ") + "\n)", ok
}

// checkExprText guards a CHECK expression: one expression, no statements, no internal names.
func checkExprText(s string) string {
	toks, err := lex(s)
	if err != nil {
		return err.Error()
	}
	for _, t := range toks {
		if t.kind == ';' {
			return "a check must be one expression"
		}
		if t.kind == 'w' || t.kind == 'i' {
			n := strings.ToLower(t.text)
			if n == "select" || reservedName(n) || strings.HasPrefix(n, "pragma_") {
				return "a check may not contain " + t.text
			}
		}
	}
	return ""
}

func (mc *migCtx) exec(ptr, cat, stmt string, args ...any) bool {
	if _, err := mc.p.W.ExecContext(bg, stmt, args...); err != nil {
		mc.is.add(ptr, cat, "SQLite: %s", cleanSQLiteErr(err))
		return false
	}
	return true
}

func (mc *migCtx) needTable(ptr string, s map[string]*Table, name string) *Table {
	t := s[name]
	if t == nil {
		mc.is.add(ptr, CatRefs, "table %q not found%s", name, didYouMean(name, tableNames(s)))
	}
	return t
}

func (mc *migCtx) needCol(ptr string, t *Table, name string) bool {
	if t.col(name) == nil {
		mc.is.add(ptr, CatRefs, "table %s has no column %q%s", t.Name, name, didYouMean(name, t.colNames()))
		return false
	}
	return true
}

// step runs one migration step; false stops the migration.
func (mc *migCtx) step(i int, m map[string]any, res *migResult) bool {
	ptr := fmt.Sprintf("/steps/%d", i)
	s := mc.schema()
	op := asStr(m["op"])
	n0 := mc.is.errCount()
	switch op {
	case "create_table":
		name := asStr(m["table"])
		if reservedName(name) {
			mc.is.add(ptr+"/table", CatSemantic, "table names may not start with _burrow_ or sqlite_")
			return false
		}
		if s[name] != nil {
			mc.is.add(ptr+"/table", CatSemantic, "table %q already exists", name)
			return false
		}
		def, ok := mc.tableDef(ptr, m, s)
		if !ok {
			return false
		}
		if !mc.exec(ptr, CatSemantic, "CREATE TABLE "+qi(name)+" "+def) {
			return false
		}
		res.Summary = append(res.Summary, "created table "+name)
	case "add_column":
		t := mc.needTable(ptr+"/table", s, asStr(m["table"]))
		if t == nil {
			return false
		}
		c := asMap(m["column"])
		cn := asStr(c["name"])
		if t.col(cn) != nil {
			mc.is.add(ptr+"/column/name", CatSemantic, "table %s already has a column %q", t.Name, cn)
			return false
		}
		if c["not_null"] == true && c["default"] == nil {
			mc.is.add(ptr+"/column", CatSemantic, "an added NOT NULL column needs a default (existing rows get it)")
			return false
		}
		d := qi(cn) + " " + strings.ToUpper(asStr(c["type"]))
		if c["not_null"] == true {
			d += " NOT NULL"
		}
		if dv, has := c["default"]; has {
			d += " DEFAULT " + sqlLiteral(dv)
		}
		if ck := asStr(c["check"]); ck != "" {
			if why := checkExprText(ck); why != "" {
				mc.is.add(ptr+"/column/check", CatSQL, "%s", why)
				return false
			}
			d += " CHECK (" + ck + ")"
		}
		if !mc.exec(ptr, CatSemantic, "ALTER TABLE "+qi(t.Name)+" ADD COLUMN "+d) {
			return false
		}
		res.Summary = append(res.Summary, fmt.Sprintf("added column %s.%s", t.Name, cn))
	case "rename_table":
		from, to := asStr(m["from"]), asStr(m["to"])
		if mc.needTable(ptr+"/from", s, from) == nil {
			return false
		}
		if s[to] != nil || reservedName(to) {
			mc.is.add(ptr+"/to", CatSemantic, "table %q already exists or is reserved", to)
			return false
		}
		if !mc.exec(ptr, CatSemantic, "ALTER TABLE "+qi(from)+" RENAME TO "+qi(to)) {
			return false
		}
		for k, v := range mc.annot {
			if strings.HasPrefix(k, from+".") {
				delete(mc.annot, k)
				mc.annot[to+"."+strings.TrimPrefix(k, from+".")] = v
			}
		}
		res.Summary = append(res.Summary, "renamed table "+from+" to "+to)
	case "rename_column":
		t := mc.needTable(ptr+"/table", s, asStr(m["table"]))
		if t == nil || !mc.needCol(ptr+"/from", t, asStr(m["from"])) {
			return false
		}
		if t.col(asStr(m["to"])) != nil {
			mc.is.add(ptr+"/to", CatSemantic, "column %q already exists", asStr(m["to"]))
			return false
		}
		if !mc.exec(ptr, CatSemantic, "ALTER TABLE "+qi(t.Name)+" RENAME COLUMN "+qi(asStr(m["from"]))+" TO "+qi(asStr(m["to"]))) {
			return false
		}
		res.Summary = append(res.Summary, fmt.Sprintf("renamed column %s.%s", t.Name, asStr(m["from"])))
	case "create_index":
		t := mc.needTable(ptr+"/table", s, asStr(m["table"]))
		if t == nil {
			return false
		}
		cols := strList(m["columns"])
		for _, c := range cols {
			if !mc.needCol(ptr+"/columns", t, c) {
				return false
			}
		}
		name := asStr(m["name"])
		if name == "" {
			name = "idx_" + t.Name + "_" + strings.Join(cols, "_")
		}
		u := ""
		if m["unique"] == true {
			u = "UNIQUE "
		}
		var q []string
		for _, c := range cols {
			q = append(q, qi(c))
		}
		if !mc.exec(ptr, CatSemantic, "CREATE "+u+"INDEX "+qi(name)+" ON "+qi(t.Name)+" ("+strings.Join(q, ", ")+")") {
			return false
		}
		res.Summary = append(res.Summary, "created index "+name)
	case "drop_index":
		name := asStr(m["name"])
		var n int
		mc.p.W.QueryRowContext(bg, `SELECT count(*) FROM sqlite_schema WHERE type = 'index' AND name = ? AND sql IS NOT NULL AND name NOT LIKE '\_burrow\_%' ESCAPE '\'`, name).Scan(&n)
		if n == 0 {
			mc.is.add(ptr+"/name", CatRefs, "index %q not found (describe_table lists indexes)", name)
			return false
		}
		if !mc.exec(ptr, CatSemantic, "DROP INDEX "+qi(name)) {
			return false
		}
	case "annotate_column":
		t := mc.needTable(ptr+"/table", s, asStr(m["table"]))
		if t == nil || !mc.needCol(ptr+"/column", t, asStr(m["column"])) {
			return false
		}
		mc.annot[t.Name+"."+asStr(m["column"])] = asStr(m["kind"])
	case "insert_rows":
		t := mc.needTable(ptr+"/table", s, asStr(m["table"]))
		if t == nil {
			return false
		}
		for ri, r := range asList(m["rows"]) {
			rm := asMap(r)
			var cols, ph []string
			var args []any
			for _, k := range sortedKeys(rm) {
				if !mc.needCol(fmt.Sprintf("%s/rows/%d/%s", ptr, ri, esc(k)), t, k) {
					return false
				}
				cols = append(cols, qi(k))
				ph = append(ph, "?")
				args = append(args, rm[k])
			}
			if !mc.exec(fmt.Sprintf("%s/rows/%d", ptr, ri), CatSemantic, "INSERT INTO "+qi(t.Name)+" ("+strings.Join(cols, ", ")+") VALUES ("+strings.Join(ph, ", ")+")", args...) {
				return false
			}
		}
		res.Summary = append(res.Summary, fmt.Sprintf("inserted %d rows into %s", len(asList(m["rows"])), t.Name))
	case "copy_data":
		t := mc.needTable(ptr+"/into", s, asStr(m["into"]))
		if t == nil {
			return false
		}
		cols := strList(m["columns"])
		for _, c := range cols {
			if !mc.needCol(ptr+"/columns", t, c) {
				return false
			}
		}
		from := asStr(m["from"])
		rc, msg := guardSQL(mc.p.W, from)
		if msg != "" {
			mc.is.add(ptr+"/from", CatSQL, "%s", msg)
			return false
		}
		if len(rc) != len(cols) {
			mc.is.add(ptr+"/from", CatSemantic, "the query returns %d columns (%s) but columns lists %d", len(rc), strings.Join(rc, ", "), len(cols))
			return false
		}
		var q []string
		for _, c := range cols {
			q = append(q, qi(c))
		}
		r, err := mc.p.W.ExecContext(bg, "INSERT INTO "+qi(t.Name)+" ("+strings.Join(q, ", ")+") SELECT * FROM ("+strings.TrimRight(strings.TrimSpace(from), ";")+")")
		if err != nil {
			mc.is.add(ptr+"/from", CatSemantic, "SQLite: %s", cleanSQLiteErr(err))
			return false
		}
		n, _ := r.RowsAffected()
		res.Summary = append(res.Summary, fmt.Sprintf("copied %d rows into %s", n, t.Name))
	case "rebuild_table":
		name := asStr(m["table"])
		old := mc.needTable(ptr+"/table", s, name)
		if old == nil {
			return false
		}
		def, ok := mc.tableDef(ptr, m, s)
		if !ok {
			return false
		}
		tmp := name + "__rebuild"
		if !mc.exec(ptr, CatSemantic, "CREATE TABLE "+qi(tmp)+" "+def) {
			return false
		}
		var nc, oc []string
		mapping := asMap(m["mapping"])
		dropped := false
		for _, c := range asList(m["columns"]) {
			cn := asStr(asMap(c)["name"])
			src, has := mapping[cn]
			if !has {
				if old.col(cn) != nil {
					src = cn
				} else {
					continue
				}
			}
			if src == nil {
				continue
			}
			if !mc.needCol(ptr+"/mapping/"+esc(cn), old, asStr(src)) {
				return false
			}
			nc = append(nc, qi(cn))
			oc = append(oc, qi(asStr(src)))
		}
		if len(oc) < len(old.Cols) {
			dropped = true
		}
		if !mc.exec(ptr, CatSemantic, "INSERT INTO "+qi(tmp)+" ("+strings.Join(nc, ", ")+") SELECT "+strings.Join(oc, ", ")+" FROM "+qi(name)) ||
			!mc.exec(ptr, CatSemantic, "DROP TABLE "+qi(name)) || !mc.exec(ptr, CatSemantic, "ALTER TABLE "+qi(tmp)+" RENAME TO "+qi(name)) {
			return false
		}
		if dropped {
			res.Destructive = append(res.Destructive, "rebuild_table "+name+" (drops columns)")
		}
		res.Summary = append(res.Summary, "rebuilt table "+name)
	case "drop_column":
		t := mc.needTable(ptr+"/table", s, asStr(m["table"]))
		if t == nil || !mc.needCol(ptr+"/column", t, asStr(m["column"])) {
			return false
		}
		if !mc.exec(ptr, CatSemantic, "ALTER TABLE "+qi(t.Name)+" DROP COLUMN "+qi(asStr(m["column"]))) {
			return false
		}
		res.Destructive = append(res.Destructive, "drop_column "+t.Name+"."+asStr(m["column"]))
	case "drop_table":
		t := mc.needTable(ptr+"/table", s, asStr(m["table"]))
		if t == nil {
			return false
		}
		if !mc.exec(ptr, CatSemantic, "DROP TABLE "+qi(t.Name)) {
			return false
		}
		res.Destructive = append(res.Destructive, fmt.Sprintf("drop_table %s (%d rows)", t.Name, t.Rows))
		res.Summary = append(res.Summary, "dropped table "+t.Name)
	default:
		return false
	}
	return mc.is.errCount() == n0
}

// schemaGuard (SPEC 7.5, SPIKE-007) inside the transaction.
func schemaGuard(c Q, before string) string {
	now, err := internalSnapshot(c)
	if err != nil {
		return err.Error()
	}
	if now != before {
		return "internal schema changed"
	}
	var name, typ string
	if err := c.QueryRowContext(bg, `SELECT type, name FROM sqlite_schema WHERE type IN ('trigger', 'view') LIMIT 1`).Scan(&typ, &name); err == nil {
		return fmt.Sprintf("%s %s not allowed", typ, name)
	}
	s, err := loadSchema(c)
	if err != nil {
		return err.Error()
	}
	for _, t := range s {
		if len(t.PK) == 0 {
			return "table " + t.Name + " has no primary key"
		}
	}
	rows, err := c.QueryContext(bg, "PRAGMA foreign_key_check")
	if err != nil {
		return err.Error()
	}
	defer rows.Close()
	if rows.Next() {
		return "foreign key violation"
	}
	return ""
}

func internalSnapshot(c Q) (string, error) {
	rows, err := c.QueryContext(bg, `SELECT type, name, tbl_name, coalesce(sql, '') FROM sqlite_schema
		WHERE name LIKE '\_burrow\_%' ESCAPE '\' OR tbl_name LIKE '\_burrow\_%' ESCAPE '\' ORDER BY type, name`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var t, n, tn, s string
		rows.Scan(&t, &n, &tn, &s)
		fmt.Fprintf(&b, "%s|%s|%s|%s\n", t, n, tn, s)
	}
	return b.String(), rows.Err()
}

// applyMigration is the apply_migration tool.
func (p *Project) applyMigration(steps any, pipelines, views []string) migResult {
	var res migResult
	doc := newDoc()
	doc.Value = map[string]any{"steps": steps}
	is := &issues{doc: doc}
	schemaIssues(migrationSchema, doc.Value, is)
	if is.errCount() > 0 {
		res.Issues = is.list
		return res
	}
	if _, err := p.W.ExecContext(bg, "BEGIN IMMEDIATE"); err != nil {
		is.add("", CatSemantic, "cannot start transaction: %v", err)
		res.Issues = is.list
		return res
	}
	committed := false
	defer func() {
		if !committed {
			p.W.ExecContext(bg, "ROLLBACK")
		}
	}()
	before, _ := internalSnapshot(p.W)
	annot := map[string]string{}
	for k, v := range p.Annot {
		annot[k] = v
	}
	mc := &migCtx{p: p, is: is, annot: annot}
	list := asList(steps)
	for i, st := range list {
		if !mc.step(i, asMap(st), &res) {
			if i+1 < len(list) {
				is.add(fmt.Sprintf("/steps/%d", i), CatSemantic, "migration stopped here; steps after this one were not checked")
				is.list[len(is.list)-1].Warn = true
			}
			break
		}
	}
	if is.errCount() == 0 {
		if g := schemaGuard(p.W, before); g != "" {
			is.add("", CatSemantic, "schema guard: %s", g)
		}
	}
	var newConfigs map[string]*Config
	if is.errCount() == 0 {
		newConfigs = p.checkDependents(p.W, annot, pipelines, views, is, &res)
	}
	for i := range is.list {
		if is.list[i].Prefix == "" {
			is.list[i].Pos = Pos{}
		}
	}
	res.Issues = is.list
	if is.errCount() > 0 {
		return res
	}
	if _, err := p.W.ExecContext(bg, "COMMIT"); err != nil {
		is.add("", CatSemantic, "commit failed: %v", err)
		res.Issues = is.list
		return res
	}
	committed = true
	p.Configs = newConfigs
	p.Annot = annot
	p.SchemaVersion++
	p.Migrations++
	res.OK = true
	return res
}

// checkDependents validates every config against the schema inside the transaction. Configs
// sent with the migration replace stored ones with the same id.
func (p *Project) checkDependents(q Q, annot map[string]string, pipelines, views []string, is *issues, res *migResult) map[string]*Config {
	schema, err := loadSchema(q)
	if err != nil {
		is.add("", CatSemantic, "cannot read schema: %v", err)
		return nil
	}
	next := map[string]*Config{}
	for k, v := range p.Configs {
		next[k] = v
	}
	type sent struct {
		prefix string
		doc    *Doc
		kind   string
	}
	var docs []sent
	for i, src := range pipelines {
		docs = append(docs, sent{fmt.Sprintf("pipelines[%d]: ", i), nil, "pipeline"})
		d, err := loadYAML(src)
		if err != nil {
			is.list = append(is.list, loadIssue(err, docs[len(docs)-1].prefix))
			continue
		}
		docs[len(docs)-1].doc = d
	}
	for i, src := range views {
		docs = append(docs, sent{fmt.Sprintf("views[%d]: ", i), nil, "view"})
		d, err := loadYAML(src)
		if err != nil {
			is.list = append(is.list, loadIssue(err, docs[len(docs)-1].prefix))
			continue
		}
		docs[len(docs)-1].doc = d
	}
	sentIDs := map[string]bool{}
	for _, s := range docs {
		if s.doc == nil {
			continue
		}
		v := asMap(s.doc.Value)
		id := asStr(v["id"])
		if id == "" {
			continue
		}
		sentIDs[id] = true
		next[id] = &Config{ID: id, Kind: s.kind, Type: asStr(v["type"]), Val: v, Rev: revOf(p.Configs[id]) + 1}
		res.Updated = append(res.Updated, s.kind+" "+id)
	}
	// validate sent configs
	for i, s := range docs {
		if s.doc == nil {
			continue
		}
		sub := &issues{doc: s.doc}
		if s.kind == "pipeline" {
			checkPipeline(q, schema, next, s.doc, sub)
		} else {
			checkView(q, schema, next, annot, s.doc, sub)
		}
		for _, x := range sub.list {
			x.Prefix = s.prefix
			is.list = append(is.list, x)
		}
		if id := asStr(asMap(s.doc.Value)["id"]); id != "" {
			src := pipelines
			j := i
			if s.kind == "view" {
				src, j = views, i-len(pipelines)
			}
			next[id].Src = src[j]
		}
	}
	// re-validate the stored configs that were not sent
	var ids []string
	for id := range p.Configs {
		if !sentIDs[id] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		c := p.Configs[id]
		d, err := loadYAML(c.Src)
		if err != nil {
			continue
		}
		sub := &issues{doc: d}
		if c.Kind == "pipeline" {
			checkPipeline(q, schema, next, d, sub)
		} else {
			checkView(q, schema, next, annot, d, sub)
		}
		if sub.errCount() == 0 {
			continue
		}
		for _, x := range sub.list {
			if x.Warn {
				continue
			}
			x.Prefix = fmt.Sprintf("stored %s %s (not sent with this migration) would break: ", c.Kind, id)
			x.Pos = Pos{}
			x.Cat = CatDepend
			is.list = append(is.list, x)
		}
	}
	return next
}

func revOf(c *Config) int {
	if c == nil {
		return 0
	}
	return c.Rev
}

func loadIssue(err error, prefix string) Issue {
	le, ok := err.(*LoadError)
	if !ok {
		return Issue{Cat: CatYAML, Msg: err.Error(), Prefix: prefix}
	}
	return Issue{Ptr: le.Ptr, Pos: le.Pos, Cat: CatYAML, Msg: "YAML: " + le.Msg, Prefix: prefix}
}

var _ = sql.ErrNoRows
