package main

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
	"github.com/expr-lang/expr/vm"
)

// Sandbox is the candidate expression engine for SPEC 6.4.
type Sandbox struct {
	MaxSource    int           // longest expression source, in bytes
	MaxNodes     uint          // largest AST
	MaxListDepth int           // how deep map/filter/all/any may nest
	MaxText      int           // text bytes one evaluation may build with +, join, lower, … (all together)
	Timeout      time.Duration // watchdog per evaluation
	Location     *time.Location
	Strict       bool // missing fields are errors, except on the left of ??
	Whitelist    bool // reject node types, operators and names outside the rules
}

func NewSandbox() *Sandbox {
	loc, _ := time.LoadLocation("Europe/Berlin")
	return &Sandbox{MaxSource: 4096, MaxNodes: 500, MaxListDepth: 1, MaxText: 1 << 20, Timeout: 100 * time.Millisecond,
		Location: loc, Strict: true, Whitelist: true}
}

// Builtins kept from expr; everything else is disabled.
var keptBuiltins = []string{"map", "filter", "all", "any", "len", "round", "int", "float"}

// Functions we provide ourselves. The text functions draw on a per-evaluation byte budget.
var ourFunctions = []string{"lower", "upper", "trim", "split", "join", "string", "today", "now", "date", "format_date", "add_days"}

var textFunctions = map[string]bool{"lower": true, "upper": true, "trim": true, "split": true, "join": true, "string": true}

var listFunctions = map[string]bool{"map": true, "filter": true, "all": true, "any": true}

var allowedBinary = map[string]bool{
	"+": true, "-": true, "*": true, "/": true, "%": true,
	"==": true, "!=": true, "<": true, ">": true, "<=": true, ">=": true,
	"and": true, "or": true, "&&": true, "||": true, "in": true, "??": true,
	"contains": true, "startsWith": true, "endsWith": true,
}

var allowedUnary = map[string]bool{"!": true, "not": true, "-": true, "+": true}

// ---------------------------------------------------------------- compile

func (s *Sandbox) Compile(src string, env map[string]any) (*vm.Program, error) {
	if len(src) > s.MaxSource {
		return nil, fmt.Errorf("expression longer than %d bytes", s.MaxSource)
	}
	if s.Whitelist {
		tree, err := parser.Parse(src)
		if err != nil {
			return nil, err
		}
		if err := s.check(tree.Node); err != nil {
			return nil, err
		}
	}
	env = withBudget(env, s.MaxText)
	opts := []expr.Option{expr.Env(env), expr.DisableAllBuiltins(), expr.MaxNodes(s.MaxNodes)}
	for _, b := range keptBuiltins {
		opts = append(opts, expr.EnableBuiltin(b))
	}
	opts = append(opts, s.functions()...)
	opts = append(opts, expr.Patch(patch{strict: s.Strict}))
	return expr.Compile(src, opts...)
}

// check applies the whitelist to the parsed tree.
func (s *Sandbox) check(root ast.Node) error {
	c := &checker{}
	ast.Walk(&root, c)
	if c.err != nil {
		return c.err
	}
	if d := listDepth(root); d > s.MaxListDepth {
		return fmt.Errorf("list functions nested %d deep; at most %d allowed", d, s.MaxListDepth)
	}
	return nil
}

type checker struct{ err error }

func (c *checker) fail(format string, args ...any) {
	if c.err == nil {
		c.err = fmt.Errorf(format, args...)
	}
}

func (c *checker) Visit(node *ast.Node) {
	switch n := (*node).(type) {
	case *ast.NilNode, *ast.IntegerNode, *ast.FloatNode, *ast.BoolNode, *ast.StringNode, *ast.ConstantNode,
		*ast.ChainNode, *ast.SliceNode, *ast.PredicateNode, *ast.PointerNode, *ast.ConditionalNode,
		*ast.ArrayNode, *ast.MapNode, *ast.PairNode:
	case *ast.IdentifierNode:
		if strings.HasPrefix(n.Value, "$") || strings.HasPrefix(n.Value, "__") {
			c.fail("name %q is not allowed", n.Value)
		}
	case *ast.UnaryNode:
		if !allowedUnary[n.Operator] {
			c.fail("operator %q is not allowed", n.Operator)
		}
	case *ast.BinaryNode:
		if !allowedBinary[n.Operator] {
			c.fail("operator %q is not allowed", n.Operator)
		}
	case *ast.MemberNode:
		if n.Method {
			c.fail("method calls are not allowed")
		}
	case *ast.CallNode:
		id, ok := n.Callee.(*ast.IdentifierNode)
		if !ok || !(contains(ourFunctions, id.Value) || contains(keptBuiltins, id.Value)) {
			c.fail("function %s is not allowed", n.Callee)
		}
	case *ast.BuiltinNode:
		if !contains(keptBuiltins, n.Name) && !contains(ourFunctions, n.Name) {
			c.fail("function %s is not allowed", n.Name)
		}
	default:
		c.fail("%T is not allowed", n)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// listDepth returns how deeply map/filter/all/any calls nest inside each other's predicates.
// A chain such as map(filter(list, …), …) has depth 1: the inner call runs once.
func listDepth(n ast.Node) int {
	best := 0
	var visit func(ast.Node, int)
	visit = func(n ast.Node, d int) {
		if b, ok := n.(*ast.BuiltinNode); ok && listFunctions[b.Name] && len(b.Arguments) > 0 {
			best = max(best, d+1)
			visit(b.Arguments[0], d)
			for _, a := range b.Arguments[1:] {
				visit(a, d+1)
			}
			return
		}
		for _, ch := range children(n) {
			if ch != nil {
				visit(ch, d)
			}
		}
	}
	visit(n, 0)
	return best
}

func children(n ast.Node) []ast.Node {
	switch n := n.(type) {
	case *ast.UnaryNode:
		return []ast.Node{n.Node}
	case *ast.BinaryNode:
		return []ast.Node{n.Left, n.Right}
	case *ast.ChainNode:
		return []ast.Node{n.Node}
	case *ast.MemberNode:
		return []ast.Node{n.Node, n.Property}
	case *ast.SliceNode:
		return []ast.Node{n.Node, n.From, n.To}
	case *ast.CallNode:
		return append([]ast.Node{n.Callee}, n.Arguments...)
	case *ast.BuiltinNode:
		return n.Arguments
	case *ast.PredicateNode:
		return []ast.Node{n.Node}
	case *ast.ConditionalNode:
		return []ast.Node{n.Cond, n.Exp1, n.Exp2}
	case *ast.ArrayNode:
		return n.Nodes
	case *ast.MapNode:
		return n.Pairs
	case *ast.PairNode:
		return []ast.Node{n.Key, n.Value}
	}
	return nil
}

// ---------------------------------------------------------------- patches

// patch rewrites the tree before expr checks it:
//   - a.b becomes __get(a, "b"), which fails on a missing field (strict mode). On the left of ??,
//     the whole access chain becomes __try, which returns nil instead.
//   - a + b becomes __add(__budget, a, b), and text functions get __budget as first argument,
//     so all text built in one evaluation counts against one budget.
type patch struct{ strict bool }

func (p patch) Visit(node *ast.Node) {
	budget := &ast.IdentifierNode{Value: "__budget"}
	switch n := (*node).(type) {
	case *ast.MemberNode:
		if p.strict && !n.Optional {
			ast.Patch(node, &ast.CallNode{Callee: &ast.IdentifierNode{Value: "__get"}, Arguments: []ast.Node{n.Node, n.Property}})
		}
	case *ast.BinaryNode:
		switch n.Operator {
		case "??":
			if p.strict {
				lenient(n.Left)
			}
		case "+":
			ast.Patch(node, &ast.CallNode{Callee: &ast.IdentifierNode{Value: "__add"}, Arguments: []ast.Node{budget, n.Left, n.Right}})
		}
	case *ast.CallNode:
		if id, ok := n.Callee.(*ast.IdentifierNode); ok && textFunctions[id.Value] {
			n.Arguments = append([]ast.Node{budget}, n.Arguments...)
		}
	}
}

func lenient(n ast.Node) {
	for {
		call, ok := n.(*ast.CallNode)
		if !ok {
			return
		}
		id, ok := call.Callee.(*ast.IdentifierNode)
		if !ok || id.Value != "__get" {
			return
		}
		id.Value = "__try"
		n = call.Arguments[0]
	}
}

// budget counts text bytes built during one evaluation.
type budget struct{ used, max int }

func (b *budget) take(s string) (string, error) {
	b.used += len(s)
	if b.used > b.max {
		return "", fmt.Errorf("expression built more than %d bytes of text", b.max)
	}
	return s, nil
}

func (b *budget) room(n int) error {
	if b.used+n > b.max {
		return fmt.Errorf("expression built more than %d bytes of text", b.max)
	}
	return nil
}

// withBudget returns a copy of env with a fresh budget.
func withBudget(env map[string]any, max int) map[string]any {
	out := make(map[string]any, len(env)+1)
	for k, v := range env {
		out[k] = v
	}
	out["__budget"] = &budget{max: max}
	return out
}

var errMissing = errors.New("missing")

func fetch(obj, key any) (any, error) {
	switch o := obj.(type) {
	case map[string]any:
		k, _ := key.(string)
		v, ok := o[k]
		if !ok {
			return nil, fmt.Errorf("%w field %q", errMissing, k)
		}
		return v, nil
	case []any:
		i, ok := key.(int)
		if !ok || i < 0 || i >= len(o) {
			return nil, fmt.Errorf("%w index %v (list has %d items)", errMissing, key, len(o))
		}
		return o[i], nil
	case nil:
		return nil, fmt.Errorf("%w field %v: value is null", errMissing, key)
	}
	return nil, fmt.Errorf("cannot read %v of a %T", key, obj)
}

// ---------------------------------------------------------------- functions

func (s *Sandbox) functions() []expr.Option {
	str := func(v any) (string, error) {
		t, ok := v.(string)
		if !ok {
			return "", fmt.Errorf("expected text, got %T", v)
		}
		return t, nil
	}
	day := func(v any) (time.Time, error) {
		t, err := str(v)
		if err != nil {
			return time.Time{}, err
		}
		return time.ParseInLocation("2006-01-02", t, s.Location)
	}
	return []expr.Option{
		expr.Function("__get", func(p ...any) (any, error) { return fetch(p[0], p[1]) }),
		expr.Function("__try", func(p ...any) (any, error) {
			v, err := fetch(p[0], p[1])
			if errors.Is(err, errMissing) {
				return nil, nil
			}
			return v, err
		}),
		expr.Function("__add", func(p ...any) (any, error) {
			b := p[0].(*budget)
			switch x := p[1].(type) {
			case string:
				y, ok := p[2].(string)
				if !ok {
					return nil, fmt.Errorf("cannot add %T to text", p[2])
				}
				if err := b.room(len(x) + len(y)); err != nil {
					return nil, err
				}
				return b.take(x + y)
			case int:
				switch y := p[2].(type) {
				case int:
					if (y > 0 && x > math.MaxInt-y) || (y < 0 && x < math.MinInt-y) {
						return nil, fmt.Errorf("integer overflow")
					}
					return x + y, nil
				case float64:
					return float64(x) + y, nil
				}
			case float64:
				switch y := p[2].(type) {
				case int:
					return x + float64(y), nil
				case float64:
					return x + y, nil
				}
			}
			return nil, fmt.Errorf("cannot add %T and %T", p[1], p[2])
		}),
		text1("lower", strings.ToLower),
		text1("upper", strings.ToUpper),
		text1("trim", strings.TrimSpace),
		expr.Function("string", func(p ...any) (any, error) { return p[0].(*budget).take(fmt.Sprint(p[1])) }),
		expr.Function("split", func(p ...any) (any, error) {
			t, err := str(p[1])
			if err != nil {
				return nil, err
			}
			sep, err := str(p[2])
			if err != nil {
				return nil, err
			}
			if _, err := p[0].(*budget).take(t); err != nil {
				return nil, err
			}
			parts := strings.Split(t, sep)
			out := make([]any, len(parts))
			for i, x := range parts {
				out[i] = x
			}
			return out, nil
		}),
		expr.Function("join", func(p ...any) (any, error) {
			b := p[0].(*budget)
			list, ok := p[1].([]any)
			if !ok {
				return nil, fmt.Errorf("join: expected a list, got %T", p[1])
			}
			sep := ""
			if len(p) > 2 {
				sep, _ = p[2].(string)
			}
			var sb strings.Builder
			for i, v := range list {
				if i > 0 {
					sb.WriteString(sep)
				}
				fmt.Fprint(&sb, v)
				if err := b.room(sb.Len()); err != nil {
					return nil, err
				}
			}
			return b.take(sb.String())
		}),
		expr.Function("today", func(p ...any) (any, error) { return time.Now().In(s.Location).Format("2006-01-02"), nil }),
		expr.Function("now", func(p ...any) (any, error) { return time.Now().UTC().Format(time.RFC3339), nil }),
		expr.Function("date", func(p ...any) (any, error) {
			t, err := day(p[0])
			if err != nil {
				return nil, err
			}
			return t.Format("2006-01-02"), nil
		}),
		expr.Function("add_days", func(p ...any) (any, error) {
			t, err := day(p[0])
			if err != nil {
				return nil, err
			}
			var n int
			switch v := p[1].(type) {
			case int:
				n = v
			case float64: // numbers from JSON
				if v != math.Trunc(v) || math.Abs(v) > 1e6 {
					return nil, fmt.Errorf("add_days: days must be a whole number")
				}
				n = int(v)
			default:
				return nil, fmt.Errorf("add_days: days must be a number")
			}
			return t.AddDate(0, 0, n).Format("2006-01-02"), nil
		}),
		expr.Function("format_date", func(p ...any) (any, error) {
			t, err := day(p[0])
			if err != nil {
				return nil, err
			}
			layout, err := str(p[1])
			if err != nil {
				return nil, err
			}
			return t.Format(layout), nil
		}),
	}
}

func text1(name string, fn func(string) string) expr.Option {
	return expr.Function(name, func(p ...any) (any, error) {
		t, ok := p[1].(string)
		if !ok {
			return nil, fmt.Errorf("%s: expected text, got %T", name, p[1])
		}
		if err := p[0].(*budget).room(len(t)); err != nil {
			return nil, err
		}
		return p[0].(*budget).take(fn(t))
	})
}

// ---------------------------------------------------------------- run

// Run evaluates with a watchdog. On timeout it returns an error at once;
// the abandoned evaluation keeps running on its goroutine until it ends.
func (s *Sandbox) Run(p *vm.Program, env map[string]any) (any, error) {
	type result struct {
		v   any
		err error
	}
	ch := make(chan result, 1)
	env = withBudget(env, s.MaxText)
	go func() {
		v, err := expr.Run(p, env)
		if err == nil {
			err = s.checkResult(v)
		}
		ch <- result{v, err}
	}()
	select {
	case r := <-ch:
		return r.v, r.err
	case <-time.After(s.Timeout):
		return nil, fmt.Errorf("timeout after %v", s.Timeout)
	}
}

func (s *Sandbox) checkResult(v any) error {
	switch t := v.(type) {
	case string:
		if len(t) > s.MaxText {
			return fmt.Errorf("result longer than %d bytes", s.MaxText)
		}
	case float64:
		if math.IsInf(t, 0) || math.IsNaN(t) {
			return fmt.Errorf("result is not a finite number")
		}
	}
	return nil
}
