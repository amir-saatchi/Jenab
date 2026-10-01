package main

// The "Burrow loader": parse YAML (or JSON) into the library's syntax tree, then walk it once to
// build the JSON-like value and a map from JSON pointer to line:column. The walk also enforces
// the rules the libraries do not enforce on their own trees: no anchors, aliases, merge keys or
// custom tags, string keys only, no duplicate keys, bounded depth.

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	goccy "github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	yaml3 "go.yaml.in/yaml/v3"
	yaml4 "go.yaml.in/yaml/v4"
)

type Pos struct{ Line, Col int }

func (p Pos) String() string { return fmt.Sprintf("%d:%d", p.Line, p.Col) }

// Doc is a loaded config: the JSON-like value plus positions.
type Doc struct {
	Value     any
	Key       map[string]Pos  // pointer -> position of the key (object members only)
	Val       map[string]Pos  // pointer -> position of the value
	Container map[string]bool // pointer -> value is an object or a list
}

// LoadError is a parse error or a rule violation, with a position.
type LoadError struct {
	Pos Pos
	Msg string
}

func (e *LoadError) Error() string { return fmt.Sprintf("line %s: %s", e.Pos, e.Msg) }

const maxDepth = 64

type loader struct {
	name string
	load func(src []byte) (*Doc, error)
}

var loaders = []loader{
	{"yaml/v3", loadV3},
	{"yaml/v4", loadV4},
	{"goccy", loadGoccy},
}

func newDoc() *Doc {
	return &Doc{Key: map[string]Pos{}, Val: map[string]Pos{}, Container: map[string]bool{}}
}

func esc(k string) string {
	return strings.ReplaceAll(strings.ReplaceAll(k, "~", "~0"), "/", "~1")
}

var (
	lineRe  = regexp.MustCompile(`line (\d+)(?::(\d+))?`)
	lcRe    = regexp.MustCompile(`\bL(\d+)\.C(\d+)\b`) // yaml/v4: "... at L8.C10: ..."
	bracket = regexp.MustCompile(`^\[(\d+):(\d+)\]`)   // goccy: "[8:10] ..."
)

// parseErr turns a library parse error into a LoadError, reading the line from the message.
func parseErr(err error) error {
	s := err.Error()
	if m := bracket.FindStringSubmatch(s); m != nil {
		l, _ := strconv.Atoi(m[1])
		c, _ := strconv.Atoi(m[2])
		first := strings.SplitN(s, "\n", 2)[0]
		return &LoadError{Pos{l, c}, strings.TrimSpace(bracket.ReplaceAllString(first, ""))}
	}
	if m := lcRe.FindStringSubmatch(s); m != nil {
		l, _ := strconv.Atoi(m[1])
		c, _ := strconv.Atoi(m[2])
		return &LoadError{Pos{l, c}, s}
	}
	if m := lineRe.FindStringSubmatch(s); m != nil {
		l, _ := strconv.Atoi(m[1])
		c := 0
		if m[2] != "" {
			c, _ = strconv.Atoi(m[2])
		}
		return &LoadError{Pos{l, c}, s}
	}
	return &LoadError{Pos{0, 0}, s}
}

// ---- go.yaml.in/yaml/v3 ----

func loadV3(src []byte) (*Doc, error) {
	var n yaml3.Node
	if err := yaml3.Unmarshal(src, &n); err != nil {
		return nil, parseErr(err)
	}
	d := newDoc()
	if len(n.Content) == 0 {
		return d, nil
	}
	v, err := walkV3(n.Content[0], "", 0, d)
	d.Value = v
	return d, err
}

func walkV3(n *yaml3.Node, ptr string, depth int, d *Doc) (any, error) {
	at := Pos{n.Line, n.Column}
	if depth > maxDepth {
		return nil, &LoadError{at, fmt.Sprintf("nesting deeper than %d levels", maxDepth)}
	}
	if n.Anchor != "" {
		return nil, &LoadError{at, "anchors (&" + n.Anchor + ") are not allowed; write the value out"}
	}
	switch n.Kind {
	case yaml3.AliasNode:
		return nil, &LoadError{at, "aliases (*" + n.Value + ") are not allowed; write the value out"}
	case yaml3.MappingNode:
		d.Container[ptr] = true
		m := make(map[string]any, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			kp := Pos{k.Line, k.Column}
			if k.Kind == yaml3.ScalarNode && k.ShortTag() == "!!merge" {
				return nil, &LoadError{kp, "merge keys (<<) are not allowed"}
			}
			if k.Kind != yaml3.ScalarNode || k.ShortTag() != "!!str" {
				return nil, &LoadError{kp, fmt.Sprintf("key %q must be a string (quote it)", k.Value)}
			}
			if _, dup := m[k.Value]; dup {
				return nil, &LoadError{kp, fmt.Sprintf("duplicate key %q", k.Value)}
			}
			p := ptr + "/" + esc(k.Value)
			d.Key[p] = kp
			d.Val[p] = Pos{v.Line, v.Column}
			x, err := walkV3(v, p, depth+1, d)
			if err != nil {
				return nil, err
			}
			m[k.Value] = x
		}
		return m, nil
	case yaml3.SequenceNode:
		d.Container[ptr] = true
		l := make([]any, 0, len(n.Content))
		for i, c := range n.Content {
			p := ptr + "/" + strconv.Itoa(i)
			d.Val[p] = Pos{c.Line, c.Column}
			x, err := walkV3(c, p, depth+1, d)
			if err != nil {
				return nil, err
			}
			l = append(l, x)
		}
		return l, nil
	case yaml3.ScalarNode:
		switch n.ShortTag() {
		case "!!str", "!!timestamp": // dates stay text (SPEC 1)
			return n.Value, nil
		case "!!null":
			return nil, nil
		case "!!bool", "!!int", "!!float":
			var x any
			if err := n.Decode(&x); err != nil {
				return nil, &LoadError{at, err.Error()}
			}
			return jsonScalar(x, at)
		default:
			return nil, &LoadError{at, "tag " + n.Tag + " is not allowed"}
		}
	}
	return nil, &LoadError{at, "unexpected node"}
}

// ---- go.yaml.in/yaml/v4 (same tree shape as v3) ----

func loadV4(src []byte) (*Doc, error) {
	var n yaml4.Node
	if err := yaml4.Load(src, &n); err != nil {
		return nil, parseErr(err)
	}
	d := newDoc()
	if len(n.Content) == 0 {
		return d, nil
	}
	v, err := walkV4(n.Content[0], "", 0, d)
	d.Value = v
	return d, err
}

func walkV4(n *yaml4.Node, ptr string, depth int, d *Doc) (any, error) {
	at := Pos{n.Line, n.Column}
	if depth > maxDepth {
		return nil, &LoadError{at, fmt.Sprintf("nesting deeper than %d levels", maxDepth)}
	}
	if n.Anchor != "" {
		return nil, &LoadError{at, "anchors (&" + n.Anchor + ") are not allowed; write the value out"}
	}
	switch n.Kind {
	case yaml4.AliasNode:
		return nil, &LoadError{at, "aliases (*" + n.Value + ") are not allowed; write the value out"}
	case yaml4.MappingNode:
		d.Container[ptr] = true
		m := make(map[string]any, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			kp := Pos{k.Line, k.Column}
			if k.Kind == yaml4.ScalarNode && k.ShortTag() == "!!merge" {
				return nil, &LoadError{kp, "merge keys (<<) are not allowed"}
			}
			if k.Kind != yaml4.ScalarNode || k.ShortTag() != "!!str" {
				return nil, &LoadError{kp, fmt.Sprintf("key %q must be a string (quote it)", k.Value)}
			}
			if _, dup := m[k.Value]; dup {
				return nil, &LoadError{kp, fmt.Sprintf("duplicate key %q", k.Value)}
			}
			p := ptr + "/" + esc(k.Value)
			d.Key[p] = kp
			d.Val[p] = Pos{v.Line, v.Column}
			x, err := walkV4(v, p, depth+1, d)
			if err != nil {
				return nil, err
			}
			m[k.Value] = x
		}
		return m, nil
	case yaml4.SequenceNode:
		d.Container[ptr] = true
		l := make([]any, 0, len(n.Content))
		for i, c := range n.Content {
			p := ptr + "/" + strconv.Itoa(i)
			d.Val[p] = Pos{c.Line, c.Column}
			x, err := walkV4(c, p, depth+1, d)
			if err != nil {
				return nil, err
			}
			l = append(l, x)
		}
		return l, nil
	case yaml4.ScalarNode:
		switch n.ShortTag() {
		case "!!str", "!!timestamp":
			return n.Value, nil
		case "!!null":
			return nil, nil
		case "!!bool", "!!int", "!!float":
			var x any
			if err := n.Decode(&x); err != nil {
				return nil, &LoadError{at, err.Error()}
			}
			return jsonScalar(x, at)
		default:
			return nil, &LoadError{at, "tag " + n.Tag + " is not allowed"}
		}
	}
	return nil, &LoadError{at, "unexpected node"}
}

// jsonScalar keeps only values JSON can hold.
func jsonScalar(x any, at Pos) (any, error) {
	switch v := x.(type) {
	case bool, string, nil:
		return v, nil
	case int:
		return int64(v), nil
	case int64:
		return v, nil
	case uint64:
		if v > 1<<63-1 {
			return nil, &LoadError{at, "integer too large"}
		}
		return int64(v), nil
	case float64:
		if v != v || v > 1.7e308 || v < -1.7e308 {
			return nil, &LoadError{at, "NaN and infinity are not allowed"}
		}
		return v, nil
	}
	return nil, &LoadError{at, fmt.Sprintf("unsupported value type %T", x)}
}

// ---- github.com/goccy/go-yaml ----

func loadGoccy(src []byte) (*Doc, error) {
	f, err := parser.ParseBytes(src, 0)
	if err != nil {
		return nil, parseErr(err)
	}
	d := newDoc()
	if len(f.Docs) == 0 || f.Docs[0].Body == nil {
		return d, nil
	}
	if len(f.Docs) > 1 {
		return nil, &LoadError{gpos(f.Docs[1]), "only one YAML document is allowed"}
	}
	v, err := walkGoccy(f.Docs[0].Body, "", 0, d)
	d.Value = v
	return d, err
}

// gpos returns a node's position. A block mapping's own token is its first ':' (not where the
// mapping starts), so for mappings the first key's position is used, matching yaml.Node.
func gpos(n ast.Node) Pos {
	if m, ok := n.(*ast.MappingNode); ok && len(m.Values) > 0 && !m.IsFlowStyle {
		n = m.Values[0].Key
	}
	if mv, ok := n.(*ast.MappingValueNode); ok {
		n = mv.Key
	}
	t := n.GetToken()
	if t == nil || t.Position == nil {
		return Pos{}
	}
	return Pos{t.Position.Line, t.Position.Column}
}

func walkGoccy(n ast.Node, ptr string, depth int, d *Doc) (any, error) {
	at := gpos(n)
	if depth > maxDepth {
		return nil, &LoadError{at, fmt.Sprintf("nesting deeper than %d levels", maxDepth)}
	}
	switch t := n.(type) {
	case *ast.AnchorNode:
		return nil, &LoadError{at, "anchors (&" + t.Name.String() + ") are not allowed; write the value out"}
	case *ast.AliasNode:
		return nil, &LoadError{at, "aliases (*" + t.Value.String() + ") are not allowed; write the value out"}
	case *ast.TagNode:
		return nil, &LoadError{at, "tag " + t.Start.Value + " is not allowed"}
	case *ast.MappingNode:
		return goccyMap(t.Values, ptr, depth, d)
	case *ast.MappingValueNode:
		return goccyMap([]*ast.MappingValueNode{t}, ptr, depth, d)
	case *ast.SequenceNode:
		d.Container[ptr] = true
		l := make([]any, 0, len(t.Values))
		for i, c := range t.Values {
			p := ptr + "/" + strconv.Itoa(i)
			d.Val[p] = gpos(c)
			x, err := walkGoccy(c, p, depth+1, d)
			if err != nil {
				return nil, err
			}
			l = append(l, x)
		}
		return l, nil
	case *ast.StringNode:
		return t.Value, nil
	case *ast.LiteralNode:
		return t.Value.Value, nil
	case *ast.NullNode:
		return nil, nil
	case *ast.BoolNode:
		return t.Value, nil
	case *ast.IntegerNode:
		return jsonScalar(t.Value, at)
	case *ast.FloatNode:
		return jsonScalar(t.Value, at)
	case *ast.InfinityNode, *ast.NanNode:
		return nil, &LoadError{at, "NaN and infinity are not allowed"}
	}
	return nil, &LoadError{at, fmt.Sprintf("unsupported node %T", n)}
}

func goccyMap(values []*ast.MappingValueNode, ptr string, depth int, d *Doc) (any, error) {
	d.Container[ptr] = true
	m := make(map[string]any, len(values))
	for _, mv := range values {
		kp := gpos(mv.Key)
		var key string
		switch k := mv.Key.(type) {
		case *ast.StringNode:
			key = k.Value
		case *ast.MergeKeyNode:
			return nil, &LoadError{kp, "merge keys (<<) are not allowed"}
		default:
			return nil, &LoadError{kp, fmt.Sprintf("key %q must be a string (quote it)", mv.Key.String())}
		}
		if _, dup := m[key]; dup {
			return nil, &LoadError{kp, fmt.Sprintf("duplicate key %q", key)}
		}
		p := ptr + "/" + esc(key)
		d.Key[p] = kp
		d.Val[p] = gpos(mv.Value)
		x, err := walkGoccy(mv.Value, p, depth+1, d)
		if err != nil {
			return nil, err
		}
		m[key] = x
	}
	return m, nil
}

// ---- native decoding into `any`, used by the YAML 1.1, duplicate-key and bomb tests ----

type native struct {
	name   string
	decode func(src []byte) (any, error)
}

var natives = []native{
	{"yaml/v3", func(b []byte) (any, error) { var v any; err := yaml3.Unmarshal(b, &v); return v, err }},
	{"yaml/v4", func(b []byte) (any, error) { var v any; err := yaml4.Load(b, &v); return v, err }},
	{"goccy", func(b []byte) (any, error) { var v any; err := goccy.Unmarshal(b, &v); return v, err }},
}

// treeOnly parses to the syntax tree without building values.
var treeOnly = []native{
	{"yaml/v3", func(b []byte) (any, error) { var n yaml3.Node; err := yaml3.Unmarshal(b, &n); return &n, err }},
	{"yaml/v4", func(b []byte) (any, error) { var n yaml4.Node; err := yaml4.Load(b, &n); return &n, err }},
	{"goccy", func(b []byte) (any, error) { f, err := parser.ParseBytes(b, 0); return f, err }},
}
