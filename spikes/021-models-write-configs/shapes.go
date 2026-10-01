package main

// Expression checks (SPEC 10 steps 3 and 6): every ${{ }} compiles in the SPIKE-005 sandbox, and
// a walk over the syntax tree checks references: steps.<id> must be an earlier step, the output
// must exist for that step type, inputs.<name> must be declared, item only where it exists, and
// fields of values whose shape is known (search results, query rows, transform.map fields,
// llm.select add fields) must exist ("type errors where types are known").

import (
	"fmt"
	"sort"
	"strings"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

type Shape struct {
	Kind   byte              // 'o' object, 'l' list, 's' scalar
	Fields map[string]*Shape // object fields; nil = fields unknown
	Elem   *Shape            // list element; nil = unknown
	What   string            // for messages
}

func scalar() *Shape { return &Shape{Kind: 's'} }

func obj(what string, fields ...string) *Shape {
	s := &Shape{Kind: 'o', Fields: map[string]*Shape{}, What: what}
	for _, f := range fields {
		s.Fields[f] = nil
	}
	return s
}

func list(elem *Shape, what string) *Shape { return &Shape{Kind: 'l', Elem: elem, What: what} }

func (s *Shape) fieldNames() string {
	var n []string
	for k := range s.Fields {
		n = append(n, k)
	}
	sort.Strings(n)
	return strings.Join(n, ", ")
}

// outputs lists the documented outputs per step type (SPEC 6.5).
var stepOutputs = map[string][]string{
	"http.get": {"status", "headers", "body", "json"}, "http.download": {"key", "size", "mime"},
	"web.search": {"results"}, "html.extract": {"title", "text", "links", "status"},
	"json.extract": {"value"}, "transform.map": {"items"}, "transform.filter": {"items"},
	"db.query": {"rows", "count"}, "db.insert": {"inserted", "updated", "id"},
	"db.insert_many": {"inserted", "updated", "skipped"}, "db.update": {"updated"},
	"llm.select": {"items", "indexes"}, "llm.extract": {"data"},
	"bucket.put": {"key", "size"}, "bucket.list": {"objects"}, "app.notify": {},
}

type exprEnv struct {
	is       *issues
	sb       *Sandbox
	earlier  map[string]*Shape // outputs of earlier steps
	order    map[string]int    // all step ids -> index
	stepUse  map[string]string
	forEach  map[string]bool
	cur      int
	inputs   map[string]bool
	item     *Shape
	itemOK   bool
	itemWhy  string
	preds    []*Shape
	lenient  int
	ptr      string
	reported map[string]bool
}

func (e *exprEnv) errf(cat, format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	if e.reported[e.ptr+msg] {
		return
	}
	e.reported[e.ptr+msg] = true
	e.is.add(e.ptr, cat, "%s", msg)
}

// exprSegments splits a string into its ${{ }} expressions. whole is true when the string is
// exactly one expression (its result keeps its type).
func exprSegments(s string) (exprs []string, whole bool, bad string) {
	t := strings.TrimSpace(s)
	rest := s
	for {
		i := strings.Index(rest, "${{")
		if i < 0 {
			break
		}
		j := strings.Index(rest[i+3:], "}}")
		if j < 0 {
			return exprs, false, "unclosed ${{ (expressions are written as ${{ ... }})"
		}
		exprs = append(exprs, strings.TrimSpace(rest[i+3:i+3+j]))
		rest = rest[i+3+j+2:]
	}
	if len(exprs) == 0 {
		if k := strings.Index(s, "{{"); k >= 0 && (k == 0 || s[k-1] != '$') && strings.Contains(s[k:], "}}") {
			return nil, false, "expressions are written as ${{ ... }} (found {{ without $)"
		}
	}
	whole = len(exprs) == 1 && strings.HasPrefix(t, "${{") && strings.HasSuffix(t, "}}")
	return exprs, whole, ""
}

// checkString checks every expression in a string value and returns the shape of the value when
// the whole string is one expression (nil when unknown), or a scalar for plain/interpolated text.
func (e *exprEnv) checkString(ptr, s string) *Shape {
	e.ptr = ptr
	exprs, whole, bad := exprSegments(s)
	if bad != "" {
		e.errf(CatExpr, "%s", bad)
		return nil
	}
	var last *Shape
	for _, x := range exprs {
		if x == "" {
			e.errf(CatExpr, "empty expression")
			continue
		}
		if _, err := e.sb.Compile(x, compileEnv); err != nil {
			e.errf(CatExpr, "expression %q does not compile: %s", short(x, 120), cleanExprErr(err))
			continue
		}
		tree, err := parser.Parse(x)
		if err != nil {
			continue
		}
		e.preds = nil
		e.lenient = 0
		last = e.walk(tree.Node)
	}
	if whole {
		return last
	}
	return scalar()
}

var compileEnv = map[string]any{
	"steps":   map[string]any{},
	"inputs":  map[string]any{},
	"item":    map[string]any{},
	"secrets": map[string]any{},
}

func cleanExprErr(err error) string {
	s := err.Error()
	if i := strings.Index(s, "\n"); i >= 0 {
		s = s[:i]
	}
	return s
}

// memberPath returns the dotted path of a member chain for messages.
func memberPath(n ast.Node) string {
	switch t := n.(type) {
	case *ast.IdentifierNode:
		return t.Value
	case *ast.PointerNode:
		return "#"
	case *ast.MemberNode:
		base := memberPath(t.Node)
		switch p := t.Property.(type) {
		case *ast.StringNode:
			return base + "." + p.Value
		case *ast.IntegerNode:
			return fmt.Sprintf("%s[%d]", base, p.Value)
		}
		return base + "[...]"
	case *ast.ChainNode:
		return memberPath(t.Node)
	}
	return "(value)"
}

func (e *exprEnv) walk(n ast.Node) *Shape {
	switch t := n.(type) {
	case nil:
		return nil
	case *ast.NilNode, *ast.IntegerNode, *ast.FloatNode, *ast.BoolNode, *ast.StringNode, *ast.ConstantNode:
		return scalar()
	case *ast.IdentifierNode:
		switch t.Value {
		case "steps":
			return &Shape{Kind: 'o', What: "steps"}
		case "inputs":
			return &Shape{Kind: 'o', What: "inputs"}
		case "secrets":
			return &Shape{Kind: 'o', What: "secrets"}
		case "item":
			if !e.itemOK {
				e.errf(CatRefs, "item is only available in steps with for_each, transform.map fields and transform.filter conditions%s", e.itemWhy)
				return nil
			}
			return e.item
		}
		return nil
	case *ast.PointerNode:
		if len(e.preds) > 0 {
			return e.preds[len(e.preds)-1]
		}
		return nil
	case *ast.ChainNode:
		return e.walk(t.Node)
	case *ast.MemberNode:
		base := e.walk(t.Node)
		if id, ok := t.Node.(*ast.IdentifierNode); ok {
			if prop, ok := t.Property.(*ast.StringNode); ok {
				switch id.Value {
				case "steps":
					return e.stepRef(prop.Value)
				case "inputs":
					if !e.inputs[prop.Value] {
						e.errf(CatRefs, "input %q is not declared in inputs", prop.Value)
					}
					return nil
				case "secrets":
					e.errf(CatRefs, "secret %q is not in the keychain (no secrets are configured for this project)", prop.Value)
					return nil
				}
			}
		}
		e.walk(t.Property)
		if base == nil {
			return nil
		}
		switch p := t.Property.(type) {
		case *ast.StringNode:
			if base.Kind == 'l' {
				if e.lenient == 0 {
					e.errf(CatRefs, "%s is a list, so it has no field %q; take one item with [0] or use map(...)", memberPath(t.Node), p.Value)
				}
				return nil
			}
			if base.Kind == 's' {
				if e.lenient == 0 {
					e.errf(CatRefs, "%s is a single value, so it has no field %q", memberPath(t.Node), p.Value)
				}
				return nil
			}
			if base.Fields == nil {
				return nil
			}
			f, ok := base.Fields[p.Value]
			if !ok {
				if e.lenient == 0 && base.What != "" {
					e.errf(CatRefs, "%s (%s) has no field %q; fields: %s", memberPath(t.Node), base.What, p.Value, base.fieldNames())
				}
				return nil
			}
			return f
		case *ast.IntegerNode:
			if base.Kind == 'l' {
				return base.Elem
			}
			return nil
		}
		return nil
	case *ast.SliceNode:
		b := e.walk(t.Node)
		e.walk(t.From)
		e.walk(t.To)
		return b
	case *ast.UnaryNode:
		e.walk(t.Node)
		return scalar()
	case *ast.BinaryNode:
		if t.Operator == "??" {
			e.lenient++
			l := e.walk(t.Left)
			e.lenient--
			r := e.walk(t.Right)
			if l != nil {
				return l
			}
			return r
		}
		e.walk(t.Left)
		e.walk(t.Right)
		switch t.Operator {
		case "+", "-", "*", "/", "%", "==", "!=", "<", ">", "<=", ">=", "and", "or", "&&", "||", "in", "contains", "startsWith", "endsWith":
			return scalar()
		}
		return nil
	case *ast.ConditionalNode:
		e.walk(t.Cond)
		a := e.walk(t.Exp1)
		e.walk(t.Exp2)
		return a
	case *ast.ArrayNode:
		for _, x := range t.Nodes {
			e.walk(x)
		}
		return list(nil, "")
	case *ast.MapNode:
		s := &Shape{Kind: 'o', Fields: map[string]*Shape{}, What: "object"}
		for _, pn := range t.Pairs {
			pair, ok := pn.(*ast.PairNode)
			if !ok {
				continue
			}
			v := e.walk(pair.Value)
			if k, ok := pair.Key.(*ast.StringNode); ok {
				s.Fields[k.Value] = v
			} else {
				s.Fields = nil
			}
			if s.Fields == nil {
				break
			}
		}
		return s
	case *ast.PairNode:
		e.walk(t.Value)
		return nil
	case *ast.PredicateNode:
		return e.walk(t.Node)
	case *ast.BuiltinNode:
		return e.call(t.Name, t.Arguments)
	case *ast.CallNode:
		name := ""
		if id, ok := t.Callee.(*ast.IdentifierNode); ok {
			name = id.Value
		}
		return e.call(name, t.Arguments)
	}
	return nil
}

func (e *exprEnv) call(name string, args []ast.Node) *Shape {
	switch name {
	case "map", "filter", "all", "any":
		if len(args) == 0 {
			return nil
		}
		src := e.walk(args[0])
		var elem *Shape
		if src != nil {
			if src.Kind != 'l' {
				e.errf(CatRefs, "%s needs a list, but %s is not a list", name, memberPath(args[0]))
			} else {
				elem = src.Elem
			}
		}
		e.preds = append(e.preds, elem)
		var body *Shape
		for _, a := range args[1:] {
			body = e.walk(a)
		}
		e.preds = e.preds[:len(e.preds)-1]
		switch name {
		case "map":
			return list(body, "map result")
		case "filter":
			return src
		}
		return scalar()
	case "split":
		for _, a := range args {
			e.walk(a)
		}
		return list(scalar(), "split result")
	}
	for _, a := range args {
		e.walk(a)
	}
	switch name {
	case "len", "lower", "upper", "trim", "join", "round", "int", "float", "string", "today", "now", "date", "format_date", "add_days":
		return scalar()
	}
	return nil
}

func (e *exprEnv) stepRef(id string) *Shape {
	idx, exists := e.order[id]
	if !exists {
		var ids []string
		for k := range e.order {
			ids = append(ids, k)
		}
		sort.Strings(ids)
		e.errf(CatRefs, "step %q does not exist%s", id, didYouMean(id, ids))
		return nil
	}
	if idx >= e.cur {
		e.errf(CatRefs, "step %q is not an earlier step (a step can only use outputs of steps above it)", id)
		return nil
	}
	s := e.earlier[id]
	if s == nil {
		return nil
	}
	// wrap so the output name is checked with a clear message
	return &Shape{Kind: 'o', Fields: s.Fields, What: fmt.Sprintf("outputs of step %s (%s%s)", id, e.stepUse[id], map[bool]string{true: " with for_each: outputs are each and count", false: ""}[e.forEach[id]])}
}

// outputShape builds the output shape of a step from its type and inputs.
func outputShape(use string, with map[string]any, withShapes map[string]*Shape, sqlCols []string) *Shape {
	o := func(fields map[string]*Shape) *Shape {
		return &Shape{Kind: 'o', Fields: fields}
	}
	switch use {
	case "http.get":
		return o(map[string]*Shape{"status": scalar(), "headers": {Kind: 'o'}, "body": scalar(), "json": nil})
	case "web.search":
		return o(map[string]*Shape{"results": list(obj("web.search result", "title", "url", "snippet", "published"), "results")})
	case "db.query":
		var row *Shape
		if sqlCols != nil {
			row = obj("row of "+"db.query", sqlCols...)
			for k := range row.Fields {
				row.Fields[k] = scalar()
			}
		}
		return o(map[string]*Shape{"rows": list(row, "rows"), "count": scalar()})
	case "transform.map":
		f := asMap(with["fields"])
		row := obj("transform.map item")
		for k := range f {
			row.Fields[k] = nil
		}
		return o(map[string]*Shape{"items": list(row, "items")})
	case "transform.filter":
		return o(map[string]*Shape{"items": withShapes["items"]})
	case "llm.select":
		var elem *Shape
		if in := withShapes["items"]; in != nil && in.Kind == 'l' && in.Elem != nil && in.Elem.Fields != nil {
			elem = &Shape{Kind: 'o', Fields: map[string]*Shape{}, What: "llm.select item (" + in.Elem.What + " plus add fields)"}
			for k, v := range in.Elem.Fields {
				elem.Fields[k] = v
			}
			for k := range asMap(with["add"]) {
				elem.Fields[k] = scalar()
			}
		}
		return o(map[string]*Shape{"items": list(elem, "items"), "indexes": list(scalar(), "indexes")})
	case "llm.extract":
		d := obj("llm.extract data")
		for k := range asMap(with["output"]) {
			d.Fields[k] = nil
		}
		return o(map[string]*Shape{"data": d})
	case "bucket.list":
		return o(map[string]*Shape{"objects": list(obj("bucket object", "key", "size", "mime", "updated_at"), "objects")})
	case "script.starlark":
		return &Shape{Kind: 'o'}
	}
	if outs, ok := stepOutputs[use]; ok {
		f := map[string]*Shape{}
		for _, k := range outs {
			f[k] = nil
		}
		return o(f)
	}
	return nil
}
