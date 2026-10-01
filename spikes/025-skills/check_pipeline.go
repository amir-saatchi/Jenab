package main

// Go checks for pipelines that the JSON Schema cannot express (SPEC 10 steps 2-8).

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type pipeCheck struct {
	p      *Project
	q      Q
	schema map[string]*Table
	is     *issues
	env    *exprEnv
	hosts  map[string]bool
	ctx    map[string]*Config // configs as they will be after this save (for id checks)
}

// checkPipeline validates a loaded pipeline document. configs is the set of configs the project
// will have (used for id uniqueness); schema is the table set to check against.
func checkPipeline(q Q, schema map[string]*Table, configs map[string]*Config, d *Doc, is *issues) []string {
	schemaIssues(pipelineSchema, d.Value, is)
	v := asMap(d.Value)
	if v == nil {
		return nil
	}
	pc := &pipeCheck{q: q, schema: schema, is: is, hosts: map[string]bool{}, ctx: configs}
	id := asStr(v["id"])
	if c := configs[id]; c != nil && c.Kind != "pipeline" {
		is.add("/id", CatSemantic, "id %q is already used by the %s view %q; ids are unique within a project", id, c.Type, id)
	}
	inputs := map[string]bool{}
	for name := range asMap(v["inputs"]) {
		if !idRe.MatchString(name) {
			is.addKey("/inputs/"+esc(name), CatSchema, "input name %q must be snake_case", name)
		}
		inputs[name] = true
	}
	loc := time.UTC
	if tz := asStr(asMap(v["trigger"])["timezone"]); tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	steps := asList(v["steps"])
	env := &exprEnv{is: is, sb: NewSandbox(loc), earlier: map[string]*Shape{}, order: map[string]int{},
		stepUse: map[string]string{}, forEach: map[string]bool{}, inputs: inputs, reported: map[string]bool{}}
	pc.env = env
	for i, s := range steps {
		m := asMap(s)
		sid := asStr(m["id"])
		if sid == "" {
			continue
		}
		if j, dup := env.order[sid]; dup {
			is.add(fmt.Sprintf("/steps/%d/id", i), CatSemantic, "duplicate step id %q (also used by steps[%d])", sid, j)
			continue
		}
		env.order[sid] = i
		env.stepUse[sid] = asStr(m["use"])
		env.forEach[sid] = m["for_each"] != nil
	}
	for i, s := range steps {
		pc.step(i, asMap(s))
	}
	var hosts []string
	for h := range pc.hosts {
		hosts = append(hosts, h)
	}
	sort.Strings(hosts)
	return hosts
}

func (pc *pipeCheck) step(i int, m map[string]any) {
	if m == nil {
		return
	}
	env := pc.env
	env.cur = i
	base := fmt.Sprintf("/steps/%d", i)
	use := asStr(m["use"])
	with := asMap(m["with"])
	sid := asStr(m["id"])
	env.itemOK, env.item, env.itemWhy = false, nil, ""
	var feElem *Shape
	if fe, ok := m["for_each"].(string); ok {
		sh := env.checkString(base+"/for_each", fe)
		if sh != nil && sh.Kind != 'l' {
			pc.is.add(base+"/for_each", CatRefs, "for_each must be a list")
		}
		if sh != nil {
			feElem = sh.Elem
		}
		env.itemOK, env.item = true, feElem
	} else {
		env.itemWhy = " (this step has no for_each)"
	}
	if s, ok := m["if"].(string); ok {
		env.checkString(base+"/if", s)
	}
	// with values; per-item fields get item = element of items
	withShapes := map[string]*Shape{}
	stepItemOK, stepItem, stepWhy := env.itemOK, env.item, env.itemWhy
	for _, k := range sortedKeys(with) {
		ptr := base + "/with/" + esc(k)
		perItem := (use == "transform.map" && k == "fields") || (use == "transform.filter" && k == "condition")
		if perItem {
			in := withShapes["items"]
			if in == nil {
				if s, ok := with["items"].(string); ok {
					env.itemOK, env.item = stepItemOK, stepItem
					in = env.checkString(base+"/with/items", s)
					pc.is.list = dropDupIssues(pc.is.list)
				}
			}
			env.itemOK, env.itemWhy = true, ""
			env.item = nil
			if in != nil && in.Kind == 'l' {
				env.item = in.Elem
			}
		} else {
			env.itemOK, env.item, env.itemWhy = stepItemOK, stepItem, stepWhy
		}
		withShapes[k] = pc.value(ptr, with[k])
	}
	env.itemOK, env.item, env.itemWhy = stepItemOK, stepItem, stepWhy

	var sqlCols []string
	switch use {
	case "db.query":
		sqlCols = pc.checkSQLParams(base+"/with", asStr(with["sql"]), asMap(with["params"]), "")
	case "db.insert":
		pc.insertCols(base+"/with", with, withShapes["row"], false)
	case "db.insert_many":
		pc.insertCols(base+"/with", with, withShapes["rows"], true)
	case "db.update":
		t := pc.table(base+"/with/table", asStr(with["table"]))
		if t != nil {
			for k := range asMap(with["set"]) {
				if t.col(k) == nil {
					pc.is.addKey(base+"/with/set/"+esc(k), CatRefs, "table %s has no column %q%s", t.Name, k, didYouMean(k, t.colNames()))
				}
			}
			if w := asStr(with["where"]); w != "" {
				pc.checkSQLParams(base+"/with", `SELECT 1 FROM "`+t.Name+`" WHERE `+w, asMap(with["params"]), "where")
			}
		}
	case "http.get", "http.download":
		u := asStr(with["url"])
		if h := literalHost(u); h != "" {
			pc.hosts[h] = true
		} else if u != "" && with["headers"] != nil {
			pc.is.add(base+"/with/headers", CatSemantic, "custom headers need a URL with a literal host (SPEC 6.7)")
		}
	case "script.starlark":
		pc.is.warn(base+"/use", "script.starlark needs user approval of the code before it runs")
	}
	if sid != "" && env.order[sid] == i {
		out := outputShape(use, with, withShapes, sqlCols)
		if m["for_each"] != nil && out != nil {
			each := &Shape{Kind: 'o', Fields: map[string]*Shape{"item": feElem}, What: "each entry of " + sid}
			for k, v := range out.Fields {
				each.Fields[k] = v
			}
			if out.Fields == nil {
				each.Fields = nil
			}
			out = &Shape{Kind: 'o', Fields: map[string]*Shape{"each": list(each, "each"), "count": scalar()}}
		}
		env.earlier[sid] = out
	}
}

func dropDupIssues(in []Issue) []Issue {
	seen := map[string]bool{}
	var out []Issue
	for _, i := range in {
		k := i.Ptr + "|" + i.Msg
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, i)
	}
	return out
}

// value checks a with value recursively and returns its shape.
func (pc *pipeCheck) value(ptr string, v any) *Shape {
	switch t := v.(type) {
	case string:
		return pc.env.checkString(ptr, t)
	case map[string]any:
		s := &Shape{Kind: 'o', Fields: map[string]*Shape{}, What: "object"}
		for _, k := range sortedKeys(t) {
			s.Fields[k] = pc.value(ptr+"/"+esc(k), t[k])
		}
		return s
	case []any:
		var elem *Shape
		for i, x := range t {
			e := pc.value(fmt.Sprintf("%s/%d", ptr, i), x)
			if i == 0 {
				elem = e
			}
		}
		return list(elem, "list")
	}
	return scalar()
}

func (pc *pipeCheck) table(ptr, name string) *Table {
	if name == "" {
		return nil
	}
	if strings.HasPrefix(name, "_burrow_") || strings.HasPrefix(name, "sqlite_") {
		pc.is.add(ptr, CatRefs, "internal table %q cannot be used", name)
		return nil
	}
	t := pc.schema[name]
	if t == nil {
		pc.is.add(ptr, CatRefs, "table %q not found%s", name, didYouMean(name, tableNames(pc.schema)))
	}
	return t
}

// checkSQLParams runs the SQL guard and checks that named parameters get values.
func (pc *pipeCheck) checkSQLParams(base, sql string, params map[string]any, what string) []string {
	if sql == "" {
		return nil
	}
	ptr := base + "/sql"
	if what != "" {
		ptr = base + "/" + what
	}
	if strings.Contains(sql, "${{") {
		pc.is.add(ptr, CatSQL, "SQL must be literal text; pass values as named parameters (:name) in params")
		return nil
	}
	cols, msg := guardSQL(pc.q, sql)
	if msg != "" {
		pc.is.add(ptr, CatSQL, "%s", msg)
	}
	names, pos := namedParams(sql)
	if pos > 0 {
		pc.is.add(ptr, CatSQL, "use named parameters (:name), not ?")
	}
	used := map[string]bool{}
	for _, n := range names {
		used[n] = true
		if _, ok := params[n]; !ok {
			pc.is.add(ptr, CatSemantic, "parameter :%s has no value in params", n)
		}
	}
	for k := range params {
		if !used[k] {
			pc.is.warn(base+"/params/"+esc(k), "parameter %q is not used in the SQL", k)
		}
	}
	return cols
}

// insertCols checks the columns of db.insert / db.insert_many rows against the table.
func (pc *pipeCheck) insertCols(base string, with map[string]any, rowShape *Shape, many bool) {
	t := pc.table(base+"/table", asStr(with["table"]))
	if t == nil {
		return
	}
	var keys []string
	known := false
	rowPtr := base + "/row"
	if many {
		rowPtr = base + "/rows"
		if rowShape != nil && rowShape.Kind == 'l' && rowShape.Elem != nil && rowShape.Elem.Fields != nil {
			for k := range rowShape.Elem.Fields {
				keys = append(keys, k)
			}
			known = true
		} else if rowShape != nil && rowShape.Kind != 'l' {
			what := "a single value"
			if rowShape.Kind == 'o' {
				what = "one object"
				if rowShape.What != "" {
					what += " (" + rowShape.What + ")"
				}
				if f := rowShape.fieldNames(); f != "" {
					what += " with fields " + f
				}
			}
			pc.is.add(rowPtr, CatRefs, "rows must be a list of objects, but this expression gives %s. Use db.insert for one row, or pass a list such as steps.<id>.items", what)
		}
	} else if rowShape != nil && rowShape.Kind == 'o' && rowShape.Fields != nil {
		for k := range rowShape.Fields {
			keys = append(keys, k)
		}
		known = true
	} else if rowShape != nil && rowShape.Kind == 'l' {
		pc.is.add(rowPtr, CatRefs, "row must be one object; use db.insert_many for a list of rows")
	}
	sort.Strings(keys)
	has := map[string]bool{}
	if known {
		for _, k := range keys {
			has[strings.ToLower(k)] = true
			if t.col(k) == nil {
				if many {
					pc.is.add(rowPtr, CatRefs, "rows have a field %q, but table %s has no such column%s (columns: %s)", k, t.Name, didYouMean(k, t.colNames()), strings.Join(t.colNames(), ", "))
				} else {
					pc.is.addKey(rowPtr+"/"+esc(k), CatRefs, "table %s has no column %q%s (columns: %s)", t.Name, k, didYouMean(k, t.colNames()), strings.Join(t.colNames(), ", "))
				}
			}
			if k == "_run_id" || k == "_fetched_at" {
				pc.is.warn(rowPtr, "%s is filled automatically; leave it out", k)
			}
		}
		for _, c := range t.Cols {
			if !c.NotNull || c.Default.Valid || has[strings.ToLower(c.Name)] {
				continue
			}
			if len(t.PK) == 1 && t.PK[0] == c.Name && strings.EqualFold(c.Type, "INTEGER") {
				continue
			}
			pc.is.add(rowPtr, CatSemantic, "column %s.%s is NOT NULL without a default, but the row does not set it", t.Name, c.Name)
		}
	}
	oc := asStr(with["on_conflict"])
	ck := strList(with["conflict_key"])
	if len(ck) > 0 {
		ok := true
		for _, c := range ck {
			if t.col(c) == nil {
				pc.is.add(base+"/conflict_key", CatRefs, "table %s has no column %q", t.Name, c)
				ok = false
			}
		}
		if ok && !t.isKey(ck) {
			pc.is.add(base+"/conflict_key", CatSemantic, "conflict_key (%s) is not the primary key or a unique index of %s (primary key: %s)", strings.Join(ck, ", "), t.Name, strings.Join(t.PK, ", "))
		}
	}
	if oc == "upsert" && known {
		key := ck
		if len(key) == 0 {
			key = t.PK
		}
		for _, c := range key {
			if !has[strings.ToLower(c)] {
				pc.is.add(rowPtr, CatSemantic, "an upsert row must set the conflict key column %q", c)
			}
		}
	}
	if many && oc == "" {
		pc.is.warn(base, "set on_conflict to ignore or upsert, so a retried step is safe to repeat")
	}
}

func itoa(i int) string { return strconv.Itoa(i) }
