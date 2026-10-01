package main

// The Burrow loader (SPIKE-013, yaml/v3 part): parse YAML or JSON into yaml.Node, then one walk
// that builds the JSON-like value and a map from JSON pointer to line:column, and rejects
// anchors, aliases, merge keys, tags, non-string keys, duplicate keys, NaN/Inf, leading-zero
// numbers and nesting over 64 (SPEC 1).

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	yaml3 "go.yaml.in/yaml/v3"
)

type Pos struct{ Line, Col int }

func (p Pos) String() string { return fmt.Sprintf("%d:%d", p.Line, p.Col) }

type Doc struct {
	Value     any
	Key       map[string]Pos
	Val       map[string]Pos
	Container map[string]bool
}

type LoadError struct {
	Pos Pos
	Msg string
	Ptr string // where in the config, when known
}

func (e *LoadError) Error() string { return fmt.Sprintf("line %s: %s", e.Pos, e.Msg) }

const (
	maxDepth   = 64
	maxCfgSize = 256 << 10 // size cap for one config
)

func newDoc() *Doc {
	return &Doc{Key: map[string]Pos{}, Val: map[string]Pos{}, Container: map[string]bool{}}
}

func esc(k string) string {
	return strings.ReplaceAll(strings.ReplaceAll(k, "~", "~0"), "/", "~1")
}

var lineRe = regexp.MustCompile(`line (\d+)(?::(\d+))?`)

// yamlHints adds a fix hint to common YAML errors (SPIKE-021 guide v2 runs only).
var yamlHints bool

func parseErr(err error) error {
	s := strings.TrimPrefix(err.Error(), "yaml: ")
	if yamlHints {
		switch {
		case strings.Contains(s, "mapping values are not allowed"):
			s += ` (a plain value on that line contains ": ", often a map literal inside ${{ }}; put the whole value in double quotes, or build the rows with transform.map)`
		case strings.Contains(s, "did not find expected ',' or '}'"), strings.Contains(s, "did not find expected ',' or ']'"):
			s += " (a ${{ }} expression inside a { } or [ ] flow block must be quoted; or write the block one key per line)"
		case strings.Contains(s, "could not find expected ':'"):
			s += " (check the indentation of this line and the line above; multi-line text needs | )"
		}
	}
	if m := lineRe.FindStringSubmatch(s); m != nil {
		l, _ := strconv.Atoi(m[1])
		c := 0
		if m[2] != "" {
			c, _ = strconv.Atoi(m[2])
		}
		return &LoadError{Pos: Pos{l, c}, Msg: s}
	}
	return &LoadError{Msg: s}
}

func loadYAML(src string) (*Doc, error) {
	if len(src) > maxCfgSize {
		return nil, &LoadError{Pos: Pos{1, 1}, Msg: fmt.Sprintf("config larger than %d bytes", maxCfgSize)}
	}
	var n yaml3.Node
	if err := yaml3.Unmarshal([]byte(src), &n); err != nil {
		return nil, parseErr(err)
	}
	d := newDoc()
	if len(n.Content) == 0 {
		return nil, &LoadError{Pos: Pos{1, 1}, Msg: "empty config"}
	}
	v, err := walkV3(n.Content[0], "", 0, d)
	d.Value = v
	return d, err
}

var leadingZero = regexp.MustCompile(`^[-+]?0[0-9]`)

func walkV3(n *yaml3.Node, ptr string, depth int, d *Doc) (any, error) {
	at := Pos{n.Line, n.Column}
	if depth > maxDepth {
		return nil, &LoadError{Pos: at, Msg: fmt.Sprintf("nesting deeper than %d levels", maxDepth)}
	}
	if n.Anchor != "" {
		return nil, &LoadError{Pos: at, Msg: "anchors (&" + n.Anchor + ") are not allowed; write the value out"}
	}
	switch n.Kind {
	case yaml3.AliasNode:
		return nil, &LoadError{Pos: at, Msg: "aliases (*" + n.Value + ") are not allowed; write the value out"}
	case yaml3.MappingNode:
		d.Container[ptr] = true
		m := make(map[string]any, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			kp := Pos{k.Line, k.Column}
			if k.Kind == yaml3.ScalarNode && k.ShortTag() == "!!merge" {
				return nil, &LoadError{Pos: kp, Msg: "merge keys (<<) are not allowed"}
			}
			if k.Kind == yaml3.MappingNode || k.Kind == yaml3.SequenceNode {
				return nil, &LoadError{Pos: kp, Msg: "a value starting with { or [ is read by YAML as a flow mapping or list; " +
					"expressions are written ${{ ... }} (with $), and other text starting with { or [ must be quoted", Ptr: ptr}
			}
			if k.Kind != yaml3.ScalarNode || k.ShortTag() != "!!str" {
				return nil, &LoadError{Pos: kp, Msg: fmt.Sprintf("key %q must be a string (quote it)", k.Value), Ptr: ptr}
			}
			if _, dup := m[k.Value]; dup {
				return nil, &LoadError{Pos: kp, Msg: fmt.Sprintf("duplicate key %q", k.Value)}
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
		if n.Tag != "" && n.Tag[0] == '!' && !strings.HasPrefix(n.Tag, "!!") {
			return nil, &LoadError{Pos: at, Msg: "tag " + n.Tag + " is not allowed"}
		}
		if n.Style&(yaml3.TaggedStyle) != 0 {
			return nil, &LoadError{Pos: at, Msg: "tag " + n.Tag + " is not allowed"}
		}
		switch n.ShortTag() {
		case "!!str", "!!timestamp":
			return n.Value, nil
		case "!!null":
			return nil, nil
		case "!!bool", "!!int", "!!float":
			if n.Style == 0 && n.ShortTag() == "!!int" && leadingZero.MatchString(n.Value) {
				return nil, &LoadError{Pos: at, Msg: fmt.Sprintf("number %s has a leading zero; quote it", n.Value)}
			}
			var x any
			if err := n.Decode(&x); err != nil {
				return nil, &LoadError{Pos: at, Msg: err.Error()}
			}
			return jsonScalar(x, at)
		default:
			return nil, &LoadError{Pos: at, Msg: "tag " + n.Tag + " is not allowed"}
		}
	}
	return nil, &LoadError{Pos: at, Msg: "unexpected node"}
}

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
			return nil, &LoadError{Pos: at, Msg: "integer too large"}
		}
		return int64(v), nil
	case float64:
		if v != v || v > 1.7e308 || v < -1.7e308 {
			return nil, &LoadError{Pos: at, Msg: "NaN and infinity are not allowed"}
		}
		return v, nil
	}
	return nil, &LoadError{Pos: at, Msg: fmt.Sprintf("unsupported value type %T", x)}
}

// posFor picks where an error points: the key for unknown/missing fields and whole objects,
// the value otherwise (SPEC 10). Unknown pointers fall back to the parent.
func (d *Doc) posFor(ptr string, atKey bool) Pos {
	if d == nil {
		return Pos{}
	}
	for {
		if atKey || d.Container[ptr] {
			if p, ok := d.Key[ptr]; ok {
				return p
			}
		}
		if p, ok := d.Val[ptr]; ok {
			return p
		}
		if ptr == "" {
			if len(d.Key) == 0 && len(d.Val) == 0 {
				return Pos{} // came as JSON tool arguments: no lines
			}
			return Pos{1, 1}
		}
		ptr = ptr[:strings.LastIndex(ptr, "/")]
		atKey = false
	}
}

// specPath turns /steps/2/with/table into steps[2].with.table (SPEC 1).
func specPath(ptr string) string {
	if ptr == "" {
		return "(top level)"
	}
	var sb strings.Builder
	for _, t := range strings.Split(ptr[1:], "/") {
		t = strings.ReplaceAll(strings.ReplaceAll(t, "~1", "/"), "~0", "~")
		if _, err := strconv.Atoi(t); err == nil {
			sb.WriteString("[" + t + "]")
			continue
		}
		if sb.Len() > 0 {
			sb.WriteString(".")
		}
		sb.WriteString(t)
	}
	return sb.String()
}
