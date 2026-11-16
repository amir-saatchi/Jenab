package logic

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The JavaScript the React app leans on, for strings the user reads:
// lengths in UTF-16 units, trim, and JSON as JSON.parse and JSON.stringify
// see it, keys in a JavaScript object's order.

// utf16Len is s.length in JavaScript.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		if r > 0xFFFF {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// utf16Cut is s.slice(0, n) in JavaScript, except that it doesn't split a
// character outside the BMP: it stops before one that would be.
func utf16Cut(s string, n int) string {
	used := 0
	for i, r := range s {
		w := 1
		if r > 0xFFFF {
			w = 2
		}
		if used+w > n {
			return s[:i]
		}
		used += w
	}
	return s
}

// isJSSpace is a character String.prototype.trim removes.
func isJSSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', 0x2028, 0x2029, 0xFEFF:
		return true
	}
	return unicode.Is(unicode.Zs, r)
}

// jsTrim is s.trim() in JavaScript.
func jsTrim(s string) string {
	return strings.TrimFunc(s, isJSSpace)
}

// upperFirst is s.charAt(0).toUpperCase() + s.slice(1).
func upperFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 {
		return s
	}
	return strings.ToUpper(string(r)) + s[n:]
}

// thousands is n.toLocaleString() in English: 24,000.
func thousands(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

type jsKind int

const (
	jsNull jsKind = iota
	jsBool
	jsNumber
	jsString
	jsArray
	jsObject
)

// jsValue is a value JSON.parse returns. An object keeps its keys in
// JavaScript's order: index-like keys first, ascending, then the rest as
// written.
type jsValue struct {
	kind  jsKind
	b     bool
	n     float64
	s     string
	items []jsValue // an array's
	keys  []string  // an object's, with vals
	vals  []jsValue
}

// parseJSON is JSON.parse.
func parseJSON(data []byte) (jsValue, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	v, err := readJSON(dec)
	if err != nil {
		return jsValue{}, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return jsValue{}, errors.New("logic: data after the JSON value")
	}
	return v, nil
}

func readJSON(dec *json.Decoder) (jsValue, error) {
	t, err := dec.Token()
	if err != nil {
		return jsValue{}, err
	}
	switch x := t.(type) {
	case nil:
		return jsValue{kind: jsNull}, nil
	case bool:
		return jsValue{kind: jsBool, b: x}, nil
	case float64:
		return jsValue{kind: jsNumber, n: x}, nil
	case string:
		return jsValue{kind: jsString, s: x}, nil
	case json.Delim:
		if x == '[' {
			v := jsValue{kind: jsArray}
			for dec.More() {
				it, err := readJSON(dec)
				if err != nil {
					return jsValue{}, err
				}
				v.items = append(v.items, it)
			}
			_, err := dec.Token()
			return v, err
		}
		v := jsValue{kind: jsObject}
		for dec.More() {
			kt, err := dec.Token()
			if err != nil {
				return jsValue{}, err
			}
			k, _ := kt.(string)
			val, err := readJSON(dec)
			if err != nil {
				return jsValue{}, err
			}
			// A repeated key keeps its place and takes the last value.
			if i := slices.Index(v.keys, k); i >= 0 {
				v.vals[i] = val
				continue
			}
			v.keys = append(v.keys, k)
			v.vals = append(v.vals, val)
		}
		if _, err := dec.Token(); err != nil {
			return jsValue{}, err
		}
		v.orderKeys()
		return v, nil
	}
	return jsValue{}, errors.New("logic: unexpected JSON token")
}

// orderKeys puts index-like keys first, ascending, as a JavaScript object
// does.
func (v *jsValue) orderKeys() {
	type kv struct {
		k   string
		v   jsValue
		idx uint64
		is  bool
	}
	all := make([]kv, len(v.keys))
	for i, k := range v.keys {
		n, ok := arrayIndex(k)
		all[i] = kv{k, v.vals[i], n, ok}
	}
	slices.SortStableFunc(all, func(a, b kv) int {
		switch {
		case a.is && b.is:
			return cmp.Compare(a.idx, b.idx)
		case a.is:
			return -1
		case b.is:
			return 1
		}
		return 0
	})
	for i, e := range all {
		v.keys[i], v.vals[i] = e.k, e.v
	}
}

// arrayIndex reads a key that is an array index: "0", or digits without a
// leading 0, below 2³²−1.
func arrayIndex(k string) (uint64, bool) {
	if k == "" || (len(k) > 1 && k[0] == '0') {
		return 0, false
	}
	n, err := strconv.ParseUint(k, 10, 64)
	if err != nil || n >= 1<<32-1 {
		return 0, false
	}
	return n, true
}

// entries is Object.entries(v): an array's by index, an object's by key.
func (v jsValue) entries() ([]string, []jsValue) {
	if v.kind == jsArray {
		keys := make([]string, len(v.items))
		for i := range v.items {
			keys[i] = strconv.Itoa(i)
		}
		return keys, v.items
	}
	return v.keys, v.vals
}

// jsNumberText is String(n) for a finite number.
func jsNumberText(n float64) string {
	if n == 0 {
		return "0"
	}
	// encoding/json writes floats as ES6's number to string does.
	b, err := json.Marshal(n)
	if err != nil {
		return "NaN"
	}
	return string(b)
}

// String is String(v); an array's is its items' joined by commas.
func (v jsValue) String() string {
	switch v.kind {
	case jsNull:
		return "null"
	case jsBool:
		return strconv.FormatBool(v.b)
	case jsNumber:
		return jsNumberText(v.n)
	case jsString:
		return v.s
	case jsArray:
		parts := make([]string, len(v.items))
		for i, it := range v.items {
			if it.kind != jsNull {
				parts[i] = it.String()
			}
		}
		return strings.Join(parts, ",")
	}
	return "[object Object]"
}

// stringify is JSON.stringify(v, null, indent); indent "" is compact.
func (v jsValue) stringify(indent string) string {
	var b strings.Builder
	v.write(&b, indent, "")
	return b.String()
}

func (v jsValue) write(b *strings.Builder, indent, at string) {
	switch v.kind {
	case jsString:
		b.WriteString(jsQuote(v.s))
	case jsArray, jsObject:
		open, closing := "[", "]"
		if v.kind == jsObject {
			open, closing = "{", "}"
		}
		keys, vals := v.entries()
		if len(vals) == 0 {
			b.WriteString(open + closing)
			return
		}
		in := at + indent
		b.WriteString(open)
		for i, val := range vals {
			if i > 0 {
				b.WriteByte(',')
			}
			if indent != "" {
				b.WriteString("\n" + in)
			}
			if v.kind == jsObject {
				b.WriteString(jsQuote(keys[i]) + ":")
				if indent != "" {
					b.WriteByte(' ')
				}
			}
			val.write(b, indent, in)
		}
		if indent != "" {
			b.WriteString("\n" + at)
		}
		b.WriteString(closing)
	default:
		b.WriteString(v.String())
	}
}

// jsQuote is JSON.stringify of a string.
func jsQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				b.WriteString(`\u00`)
				b.WriteString(strconv.FormatInt(int64(r)>>4, 16))
				b.WriteString(strconv.FormatInt(int64(r)&0xF, 16))
				continue
			}
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
