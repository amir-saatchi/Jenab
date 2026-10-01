package main

// Go checks for views (SPEC 5, 10): SQL guard + EXPLAIN, columns checked against the result
// columns of the prepared query, parameters bound, targets exist, forms cover NOT NULL columns.

import (
	"fmt"
	"regexp"
	"strings"
)

var idRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func checkView(q Q, schema map[string]*Table, configs map[string]*Config, annot map[string]string, d *Doc, is *issues) {
	schemaIssues(viewSchema, d.Value, is)
	v := asMap(d.Value)
	if v == nil {
		return
	}
	id, typ := asStr(v["id"]), asStr(v["type"])
	if c := configs[id]; c != nil && c.Kind != "view" {
		is.add("/id", CatSemantic, "id %q is already used by the pipeline %q; ids are unique within a project", id, id)
	}
	for i, a := range asList(v["actions"]) {
		if t := asStr(asMap(a)["run_pipeline"]); t != "" && !isKind(configs, t, "pipeline") {
			is.add(fmt.Sprintf("/actions/%d/run_pipeline", i), CatRefs, "pipeline %q not found%s", t, didYouMean(t, idsOf(configs, "pipeline")))
		}
	}
	switch typ {
	case "table", "chart":
		checkQueryView(q, schema, configs, v, typ, is)
	case "form":
		checkForm(q, schema, configs, annot, v, is)
	}
}

func isKind(configs map[string]*Config, id, kind string) bool {
	c := configs[id]
	return c != nil && c.Kind == kind
}

func idsOf(configs map[string]*Config, kind string) []string {
	var out []string
	for id, c := range configs {
		if c.Kind == kind {
			out = append(out, id)
		}
	}
	return out
}

// topLevelOrderBy reports ORDER BY, LIMIT or OFFSET outside parentheses.
func topLevelOrderBy(q string) string {
	toks, _ := lex(q)
	depth := 0
	for i, t := range toks {
		if t.kind == 'p' && t.text == "(" {
			depth++
		}
		if t.kind == 'p' && t.text == ")" {
			depth--
		}
		if depth != 0 || t.kind != 'w' {
			continue
		}
		w := strings.ToLower(t.text)
		if w == "order" && i+1 < len(toks) && strings.EqualFold(toks[i+1].text, "by") {
			return "ORDER BY"
		}
		if w == "limit" || w == "offset" {
			return strings.ToUpper(w)
		}
	}
	return ""
}

func checkQueryView(q Q, schema map[string]*Table, configs map[string]*Config, v map[string]any, typ string, is *issues) {
	query := asStr(v["query"])
	if query == "" {
		return
	}
	var cols []string
	colOK := false
	if strings.Contains(query, "${{") {
		is.add("/query", CatSQL, "view queries are plain SQL; ${{ }} expressions are only for pipelines. Use named parameters (:name) bound to filters or params")
	} else {
		var msg string
		cols, msg = guardSQL(q, query)
		if msg != "" {
			is.add("/query", CatSQL, "%s", msg)
		} else {
			colOK = true
		}
		if typ == "table" {
			if w := topLevelOrderBy(query); w != "" {
				is.warn("/query", "%s in a table query is ignored; the runtime sorts and paginates (use default_sort)", w)
			}
		}
	}
	// parameters
	names, pos := namedParams(query)
	if pos > 0 {
		is.add("/query", CatSQL, "use named parameters (:name), not ?")
	}
	params := asMap(v["params"])
	filterParams := map[string]int{}
	for i, f := range asList(v["filters"]) {
		fm := asMap(f)
		p := asStr(fm["param"])
		if p == "" {
			continue
		}
		if _, dup := filterParams[p]; dup {
			is.add(fmt.Sprintf("/filters/%d/param", i), CatSemantic, "two filters bind parameter %q", p)
		}
		filterParams[p] = i
		if _, both := params[p]; both {
			is.add(fmt.Sprintf("/filters/%d/param", i), CatSemantic, "parameter %q is in both params and filters; use one", p)
		}
		ctl := asStr(fm["control"])
		if def, ok := fm["default"].(string); ok && ctl == "date" {
			if why := checkDateDefault(def); why != "" {
				is.add(fmt.Sprintf("/filters/%d/default", i), CatSchema, "%s", why)
			}
		}
		if ctl == "select" && fm["options"] == nil {
			is.add(fmt.Sprintf("/filters/%d", i), CatSemantic, "a select filter needs options")
		}
		if oq := asStr(asMap(fm["options"])["query"]); oq != "" {
			if _, msg := guardSQL(q, oq); msg != "" {
				is.add(fmt.Sprintf("/filters/%d/options/query", i), CatSQL, "%s", msg)
			}
		}
	}
	used := map[string]bool{}
	for _, n := range names {
		used[n] = true
		_, inP := params[n]
		_, inF := filterParams[n]
		if !inP && !inF {
			is.add("/query", CatSemantic, "parameter :%s is not bound; add a filter with param: %s or a value in params", n, n)
		}
	}
	for p, i := range filterParams {
		if !used[p] {
			is.warn(fmt.Sprintf("/filters/%d/param", i), "filter parameter %q is not used in the query", p)
		}
	}
	for p := range params {
		if !used[p] {
			is.warn("/params/"+esc(p), "parameter %q is not used in the query", p)
		}
	}
	if !colOK {
		return
	}
	have := map[string]bool{}
	for _, c := range cols {
		have[strings.ToLower(c)] = true
	}
	field := func(ptr, f string) {
		if f != "" && !have[strings.ToLower(f)] {
			is.add(ptr, CatRefs, "field %q is not a column of the query result%s (result columns: %s)", f, didYouMean(f, cols), strings.Join(cols, ", "))
		}
	}
	if typ == "table" {
		for i, c := range asList(v["columns"]) {
			field(fmt.Sprintf("/columns/%d/field", i), asStr(asMap(c)["field"]))
		}
		field("/default_sort/field", asStr(asMap(v["default_sort"])["field"]))
		rf := asMap(v["rows_from"])
		if rf != nil {
			tn, key := asStr(rf["table"]), asStr(rf["key"])
			t := schema[tn]
			if t == nil && tn != "" {
				is.add("/rows_from/table", CatRefs, "table %q not found%s", tn, didYouMean(tn, tableNames(schema)))
			}
			if t != nil && key != "" {
				if t.col(key) == nil {
					is.add("/rows_from/key", CatRefs, "table %s has no column %q", tn, key)
				} else if !t.isKey([]string{key}) {
					is.add("/rows_from/key", CatSemantic, "%s is not the primary key or a unique column of %s (primary key: %s)", key, tn, strings.Join(t.PK, ", "))
				}
				if !have[strings.ToLower(key)] {
					is.add("/rows_from/key", CatSemantic, "the query must select the key column %q", key)
				}
			}
		}
		for i, a := range asList(v["row_actions"]) {
			am := asMap(a)
			if rf == nil {
				is.add(fmt.Sprintf("/row_actions/%d", i), CatSemantic, "row actions need rows_from { table, key }")
			}
			if f := asStr(am["open_form"]); f != "" {
				c := configs[f]
				if c == nil || c.Kind != "view" || c.Type != "form" {
					is.add(fmt.Sprintf("/row_actions/%d/open_form", i), CatRefs, "form %q not found%s", f, didYouMean(f, idsOf(configs, "view")))
				}
			}
		}
	}
	if typ == "chart" {
		field("/x/field", asStr(asMap(v["x"])["field"]))
		ys := asList(v["y"])
		for i, y := range ys {
			field(fmt.Sprintf("/y/%d/field", i), asStr(asMap(y)["field"]))
		}
		if sb := asStr(v["series_by"]); sb != "" {
			field("/series_by", sb)
			if len(ys) != 1 {
				is.add("/y", CatSemantic, "with series_by, y must have exactly one entry (it has %d)", len(ys))
			}
		}
	}
}

func checkForm(q Q, schema map[string]*Table, configs map[string]*Config, annot map[string]string, v map[string]any, is *issues) {
	sub := asMap(v["submit"])
	action := asStr(sub["action"])
	fields := asList(v["fields"])
	for i, f := range fields {
		fm := asMap(f)
		if oq := asStr(asMap(fm["options"])["query"]); oq != "" {
			if _, msg := guardSQL(q, oq); msg != "" {
				is.add(fmt.Sprintf("/fields/%d/options/query", i), CatSQL, "%s", msg)
			}
		}
		if asStr(fm["control"]) == "select" && fm["options"] == nil {
			is.add(fmt.Sprintf("/fields/%d", i), CatSemantic, "a select field needs options")
		}
	}
	switch action {
	case "insert", "update":
		tn := asStr(sub["table"])
		t := schema[tn]
		if t == nil {
			if tn != "" {
				is.add("/submit/table", CatRefs, "table %q not found%s", tn, didYouMean(tn, tableNames(schema)))
			}
			return
		}
		has := map[string]bool{}
		req := map[string]bool{}
		for i, f := range fields {
			fm := asMap(f)
			name := asStr(fm["field"])
			if name == "" {
				continue
			}
			has[strings.ToLower(name)] = true
			if fm["required"] == true || fm["default"] != nil {
				req[strings.ToLower(name)] = true
			}
			c := t.col(name)
			if c == nil {
				is.add(fmt.Sprintf("/fields/%d/field", i), CatRefs, "table %s has no column %q%s (columns: %s)", tn, name, didYouMean(name, t.colNames()), strings.Join(t.colNames(), ", "))
				continue
			}
			if asStr(fm["control"]) == "file" && annot[tn+"."+c.Name] != "object_ref" {
				is.add(fmt.Sprintf("/fields/%d/control", i), CatSemantic, "a file control needs an object_ref column; %s.%s is not annotated", tn, c.Name)
			}
		}
		if action == "insert" {
			for _, c := range t.Cols {
				if !c.NotNull || c.Default.Valid || req[strings.ToLower(c.Name)] {
					continue
				}
				if len(t.PK) == 1 && t.PK[0] == c.Name && strings.EqualFold(c.Type, "INTEGER") {
					continue
				}
				if has[strings.ToLower(c.Name)] {
					is.add("/fields", CatSemantic, "column %s.%s is NOT NULL without a default; mark its field required: true", tn, c.Name)
				} else {
					is.add("/fields", CatSemantic, "column %s.%s is NOT NULL without a default, but no required form field sets it", tn, c.Name)
				}
			}
		}
		if action == "update" {
			key := asStr(sub["key"])
			if key != "" && t.col(key) == nil {
				is.add("/submit/key", CatRefs, "table %s has no column %q", tn, key)
			}
			load := asStr(v["load"])
			if load == "" {
				is.add("/submit", CatSemantic, "an update form needs load: a query that fills the fields, using :key")
			} else {
				if _, msg := guardSQL(q, load); msg != "" {
					is.add("/load", CatSQL, "%s", msg)
				}
				names, _ := namedParams(load)
				if len(names) != 1 || names[0] != "key" {
					is.add("/load", CatSemantic, "load receives the row key as :key and no other parameter")
				}
			}
		}
	case "run_pipeline":
		pn := asStr(sub["pipeline"])
		c := configs[pn]
		if c == nil || c.Kind != "pipeline" {
			is.add("/submit/pipeline", CatRefs, "pipeline %q not found%s", pn, didYouMean(pn, idsOf(configs, "pipeline")))
			return
		}
		inputs := asMap(c.Val["inputs"])
		for i, f := range fields {
			name := asStr(asMap(f)["field"])
			if _, ok := inputs[name]; !ok && name != "" {
				is.add(fmt.Sprintf("/fields/%d/field", i), CatRefs, "pipeline %s has no input %q", pn, name)
			}
		}
	}
	if action != "update" && v["load"] != nil {
		is.add("/load", CatSemantic, "load is only for update forms")
	}
}
