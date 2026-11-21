package view

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v4"
)

// The loader (SPIKE-013, SPIKE-021): YAML, or JSON, which is YAML too, is
// parsed into yaml.Node and walked once. The walk builds the JSON-like
// value the schemas check and a map from each JSON pointer to its line and
// column, and refuses what SPEC 1 leaves out of configs: anchors,
// aliases, merge keys, tags, keys that aren't text, duplicate keys,
// NaN and the infinities, numbers with a leading zero, and nesting deeper
// than maxDepth.

// MaxSize is the largest config, in bytes.
const MaxSize = 256 << 10

const maxDepth = 64

// pos is a line and column, both from 1; 0 for none, as for a config
// that came as JSON tool arguments without lines.
type pos struct{ line, col int }

// doc is a loaded config: its value and where each part of it is.
type doc struct {
	value     any
	key       map[string]pos  // pointer → position of the key (object members only)
	val       map[string]pos  // pointer → position of the value
	container map[string]bool // pointer → the value is an object or a list
}

func newDoc() *doc {
	return &doc{key: map[string]pos{}, val: map[string]pos{}, container: map[string]bool{}}
}

// loadError is a config that doesn't parse, or breaks a rule of the walk.
type loadError struct {
	at  pos
	msg string
}

func (e *loadError) Error() string { return e.msg }

// load parses src. A config that doesn't parse is a *loadError.
func load(src []byte) (*doc, error) {
	if len(src) > MaxSize {
		return nil, &loadError{pos{1, 1}, fmt.Sprintf("the config is %d bytes; at most %d are allowed", len(src), MaxSize)}
	}
	if strings.TrimSpace(string(src)) == "" {
		return nil, &loadError{pos{1, 1}, "the config is empty"}
	}
	dec := yaml.NewDecoder(bytes.NewReader(src))
	var n yaml.Node
	if err := dec.Decode(&n); errors.Is(err, io.EOF) {
		return nil, &loadError{pos{1, 1}, "the config is empty"}
	} else if err != nil {
		return nil, parseError(err, src)
	}
	if len(n.Content) == 0 {
		return nil, &loadError{pos{1, 1}, "the config is empty"}
	}
	// A second document after --- would be dropped without a word.
	var more yaml.Node
	switch err := dec.Decode(&more); {
	case errors.Is(err, io.EOF):
	case err != nil:
		return nil, parseError(err, src)
	case len(more.Content) > 0 && more.Content[0].ShortTag() != "!!null":
		return nil, &loadError{pos{more.Content[0].Line, more.Content[0].Column}, "a config is one YAML document, and a second one starts here, after ---; remove it, and save each config on its own"}
	}
	root := n.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, &loadError{pos{root.Line, root.Column}, "a config is a mapping of fields, as version: 1 on its first line"}
	}
	d := newDoc()
	v, err := walk(root, "", 0, d)
	if err != nil {
		return nil, err
	}
	d.value = v
	return d, nil
}

// parseError turns yaml's error into a loadError with its position and a
// fix hint for the mistakes models make most (SPEC 10, SPIKE-021).
func parseError(err error, src []byte) error {
	var le *yaml.LoadError
	if !errors.As(err, &le) {
		msg := strings.TrimPrefix(err.Error(), "yaml: ")
		at := pos{}
		if m := lineRe.FindStringSubmatch(msg); m != nil {
			at.line, _ = strconv.Atoi(m[1])
			msg = strings.TrimSpace(lineRe.ReplaceAllString(msg, ""))
		}
		return &loadError{at, msg + hint(msg, src, at.line)}
	}
	at := pos{le.Mark.Line, le.Mark.Column}
	// At the end of the text, as with a quote never closed, the line is
	// past the config's last one: the line the open construct starts on
	// says more.
	if le.Mark.Index >= len(src) && le.ContextMark.Line > 0 {
		at = pos{le.ContextMark.Line, le.ContextMark.Column}
	}
	msg := le.Message
	if le.ContextMsg != "" {
		msg = le.ContextMsg + ": " + msg
	}
	return &loadError{at, msg + hint(msg, src, at.line)}
}

var lineRe = regexp.MustCompile(`^line (\d+)(?::(\d+))?:?`)

// hint is a fix hint for a parse error, "" for none.
func hint(msg string, src []byte, line int) string {
	text := lineOf(src, line)
	switch {
	case strings.Contains(msg, "mapping values are not allowed"):
		return `; quote this value: it contains ": "`
	case strings.Contains(msg, "did not find expected ',' or '}'"), strings.Contains(msg, "did not find expected ',' or ']'"):
		return "; quote values inside { } or [ ] that contain a comma, a colon or a bracket, or write the block one field per line"
	case strings.Contains(msg, "found character that cannot start any token"):
		if strings.Contains(text, "\t") {
			return "; indent with spaces: YAML doesn't allow tabs"
		}
		return "; quote this value: it starts with a character YAML reserves, such as ` @ or %"
	case strings.Contains(msg, "found unexpected end of stream"), strings.Contains(msg, "found unexpected end of stream while scanning a quoted scalar"):
		return "; a quote is opened and never closed"
	case strings.Contains(msg, "could not find expected ':'"), strings.Contains(msg, "did not find expected key"),
		strings.Contains(msg, "did not find expected '-' indicator"):
		return "; check the indentation of this line and the one above it; text over several lines goes after | on its own lines"
	case strings.Contains(msg, "found a tab character"):
		return "; indent with spaces: YAML doesn't allow tabs"
	case strings.Contains(msg, "block sequence entries are not allowed"):
		return "; put the list on its own lines under the key, one - item a line"
	}
	return ""
}

// lineOf returns line n of src, from 1; "" when there is none.
func lineOf(src []byte, n int) string {
	if n < 1 {
		return ""
	}
	lines := strings.SplitN(string(src), "\n", n+1)
	if len(lines) < n {
		return ""
	}
	return lines[n-1]
}

var leadingZero = regexp.MustCompile(`^[-+]?0[0-9]`)

func walk(n *yaml.Node, ptr string, depth int, d *doc) (any, error) {
	at := pos{n.Line, n.Column}
	if depth > maxDepth {
		return nil, &loadError{at, fmt.Sprintf("nesting deeper than %d levels", maxDepth)}
	}
	if n.Anchor != "" {
		return nil, &loadError{at, "anchors (&" + n.Anchor + ") are not allowed; write the value out"}
	}
	if (n.Kind == yaml.MappingNode || n.Kind == yaml.SequenceNode) && n.Style&yaml.TaggedStyle != 0 {
		return nil, &loadError{at, "tags (" + n.Tag + ") are not allowed"}
	}
	switch n.Kind {
	case yaml.AliasNode:
		return nil, &loadError{at, "aliases (*" + n.Value + ") are not allowed; write the value out"}
	case yaml.MappingNode:
		d.container[ptr] = true
		m := make(map[string]any, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			kp := pos{k.Line, k.Column}
			if k.Kind == yaml.ScalarNode && k.ShortTag() == "!!merge" {
				return nil, &loadError{kp, "merge keys (<<) are not allowed; write the fields out"}
			}
			if k.Kind == yaml.MappingNode || k.Kind == yaml.SequenceNode {
				return nil, &loadError{kp, "YAML reads a value that starts with { or [ as a mapping or a list; quote text that starts with { or ["}
			}
			if k.Kind != yaml.ScalarNode || k.ShortTag() != "!!str" {
				return nil, &loadError{kp, fmt.Sprintf("the key %s must be text; quote it", k.Value)}
			}
			if _, dup := m[k.Value]; dup {
				return nil, &loadError{kp, fmt.Sprintf("the key %s is given twice", k.Value)}
			}
			p := ptr + "/" + escape(k.Value)
			d.key[p] = kp
			d.val[p] = pos{v.Line, v.Column}
			x, err := walk(v, p, depth+1, d)
			if err != nil {
				return nil, err
			}
			m[k.Value] = x
		}
		return m, nil
	case yaml.SequenceNode:
		d.container[ptr] = true
		l := make([]any, 0, len(n.Content))
		for i, c := range n.Content {
			p := ptr + "/" + strconv.Itoa(i)
			d.val[p] = pos{c.Line, c.Column}
			x, err := walk(c, p, depth+1, d)
			if err != nil {
				return nil, err
			}
			l = append(l, x)
		}
		return l, nil
	case yaml.ScalarNode:
		if n.Style&yaml.TaggedStyle != 0 {
			return nil, &loadError{at, "tags (" + n.Tag + ") are not allowed"}
		}
		switch n.ShortTag() {
		case "!!str", "!!timestamp": // dates stay text (SPEC 1)
			return n.Value, nil
		case "!!null":
			return nil, nil
		case "!!bool", "!!int", "!!float":
			if n.ShortTag() == "!!int" && leadingZero.MatchString(n.Value) {
				return nil, &loadError{at, fmt.Sprintf("the number %s starts with a zero; quote it if it is text", n.Value)}
			}
			var x any
			if err := n.Decode(&x); err != nil {
				return nil, &loadError{at, err.Error()}
			}
			return jsonScalar(x, at)
		default:
			return nil, &loadError{at, "tags (" + n.Tag + ") are not allowed"}
		}
	}
	return nil, &loadError{at, "this part of the config can't be read"}
}

// jsonScalar keeps only values JSON can hold: whole numbers as int64,
// also when written with a fraction of zero (8.0), others as float64.
func jsonScalar(x any, at pos) (any, error) {
	switch v := x.(type) {
	case bool, string, nil:
		return v, nil
	case int:
		return int64(v), nil
	case int64:
		return v, nil
	case uint64:
		if v > 1<<63-1 {
			return nil, &loadError{at, "the number is too large"}
		}
		return int64(v), nil
	case float64:
		if v != v || v > 1.7e308 || v < -1.7e308 {
			return nil, &loadError{at, "NaN and infinity are not allowed"}
		}
		// A whole number written as 8.0 is the integer 8, as in JSON, so
		// every integer field reads it as one.
		if v == math.Trunc(v) && math.Abs(v) <= 1<<53 {
			return int64(v), nil
		}
		return v, nil
	}
	return nil, &loadError{at, fmt.Sprintf("a value of type %T is not allowed", x)}
}

// escape escapes a key for a JSON pointer.
func escape(k string) string {
	return strings.ReplaceAll(strings.ReplaceAll(k, "~", "~0"), "/", "~1")
}

func unescape(k string) string {
	return strings.ReplaceAll(strings.ReplaceAll(k, "~1", "/"), "~0", "~")
}

// at returns where an issue at ptr points: the key for an unknown or
// missing field and for a whole object or list, the value otherwise
// (SPEC 10). A pointer the config doesn't have points at its parent.
func (d *doc) at(ptr string, key bool) pos {
	if d == nil {
		return pos{}
	}
	for {
		if key || d.container[ptr] {
			if p, ok := d.key[ptr]; ok {
				return p
			}
		}
		if p, ok := d.val[ptr]; ok {
			return p
		}
		if ptr == "" {
			if len(d.key) == 0 && len(d.val) == 0 {
				return pos{}
			}
			return pos{1, 1}
		}
		ptr = ptr[:strings.LastIndexByte(ptr, '/')]
		key = true
	}
}
