package sqlguard

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The lexer of layer 1. It follows SQLite's rules for strings, quoted
// names and comments (tokenize.c), but is stricter everywhere else: a byte
// it doesn't know is refused rather than guessed at, so it can never read
// a query differently from SQLite without saying so. Outside strings,
// quoted names and comments it allows ASCII letters, digits, '_', white
// space, SQLite's operators and punctuation, plain parameters, and other
// characters only inside names (valid UTF-8, never U+FEFF, which SQLite
// takes for white space).

type kind byte

const (
	word   kind = iota // keyword, name or number
	quoted             // "name", `name` or [name]
	str                // 'string'
	param              // ?, ?NNN, :name, @name, $name
	semi               // ;
	punct              // an operator or punctuation, one byte
)

type token struct {
	kind kind
	text string // the name without its quotes; a param with its prefix
	pos  int    // byte offset in the query
}

// isNameByte reports a byte that can be part of a name: an ASCII letter,
// a digit, '_', or a byte of a multi-byte character (checked separately).
func isNameByte(c byte) bool {
	return c == '_' || c >= 0x80 ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

// isPunct reports SQLite's one-byte operators and punctuation; the longer
// ones (||, <=, ->>, …) are made of these.
func isPunct(c byte) bool { return strings.IndexByte("(),.+-*/%<>=!|&~", c) >= 0 }

// lex splits q into tokens. Comments and white space are dropped.
func lex(q string) ([]token, *RejectError) {
	// SQLite reads the query as a C string, so it would stop at a NUL,
	// even inside a string.
	if i := strings.IndexByte(q, 0); i >= 0 {
		return nil, reject(q, 1, i, "the query contains a NUL byte")
	}
	var out []token
	i := 0
	for i < len(q) {
		c := q[i]
		switch {
		case isSpace(c):
			i++
		case c == '-' && i+1 < len(q) && q[i+1] == '-':
			for i < len(q) && q[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(q) && q[i+1] == '*':
			// SQLite accepts a comment left open at the end. It is refused
			// here, because it would hide what Wrap puts after the query.
			j := strings.Index(q[i+2:], "*/")
			if j < 0 {
				return nil, reject(q, 1, i, "unterminated comment")
			}
			i += j + 4
		case c == '\'' || c == '"' || c == '`':
			text, end, ok := unquote(q, i)
			if !ok {
				if c == '\'' {
					return nil, reject(q, 1, i, "unterminated string")
				}
				return nil, reject(q, 1, i, "unterminated quoted name")
			}
			k := quoted
			if c == '\'' {
				k = str
			}
			out = append(out, token{k, text, i})
			i = end
		case c == '[':
			j := strings.IndexByte(q[i+1:], ']')
			if j < 0 {
				return nil, reject(q, 1, i, "unterminated quoted name")
			}
			out = append(out, token{quoted, q[i+1 : i+1+j], i})
			i += j + 2
		case c == '?':
			j := i + 1
			for j < len(q) && isDigit(q[j]) {
				j++
			}
			out = append(out, token{param, q[i:j], i})
			i = j
		case c == ':' || c == '@' || c == '$':
			j, err := nameEnd(q, i+1)
			if err != nil {
				return nil, err
			}
			if j == i+1 {
				return nil, reject(q, 1, i, "`%c` must start a parameter name, like %cname", c, c)
			}
			if j < len(q) && (q[j] == '(' || q[j] == '$' || strings.HasPrefix(q[j:], "::")) {
				return nil, reject(q, 1, i, "parameter names are letters, digits and '_' only")
			}
			// database/sql binds only names that start with a letter, so
			// any other name could never get its value.
			if r, _ := utf8.DecodeRuneInString(q[i+1 : j]); !unicode.IsLetter(r) {
				return nil, reject(q, 1, i, "parameter names start with a letter, like %cname; for a numbered parameter write ?1", c)
			}
			out = append(out, token{param, q[i:j], i})
			i = j
		case isNameByte(c):
			j, err := nameEnd(q, i)
			if err != nil {
				return nil, err
			}
			if j < len(q) && q[j] == '$' {
				return nil, reject(q, 1, j, "`$` can only start a parameter name")
			}
			out = append(out, token{word, q[i:j], i})
			i = j
		case c == ';':
			out = append(out, token{semi, ";", i})
			i++
		case isPunct(c):
			out = append(out, token{punct, q[i : i+1], i})
			i++
		default:
			return nil, reject(q, 1, i, "%s is not allowed here", describe(c))
		}
	}
	return out, nil
}

// nameEnd returns where the name that starts at q[i] ends. A character
// outside ASCII must be valid UTF-8 and not U+FEFF.
func nameEnd(q string, i int) (int, *RejectError) {
	for i < len(q) && isNameByte(q[i]) {
		if q[i] < 0x80 {
			i++
			continue
		}
		r, n := utf8.DecodeRuneInString(q[i:])
		if r == utf8.RuneError {
			return 0, reject(q, 1, i, "the query is not valid UTF-8")
		}
		if r == 0xFEFF {
			return 0, reject(q, 1, i, "the character U+FEFF is not allowed outside strings")
		}
		i += n
	}
	return i, nil
}

func describe(c byte) string {
	if c > ' ' && c < 0x7f {
		return fmt.Sprintf("`%c`", c)
	}
	return fmt.Sprintf("the byte 0x%02X", c)
}

// unquote reads the quoted token that starts at q[i]. A doubled quote
// stands for one.
func unquote(q string, i int) (text string, end int, ok bool) {
	c := q[i]
	j := i + 1
	k := strings.IndexByte(q[j:], c)
	if k < 0 {
		return "", 0, false
	}
	if j+k+1 >= len(q) || q[j+k+1] != c {
		return q[j : j+k], j + k + 1, true // no doubled quote: no copy
	}
	var b strings.Builder
	for {
		if j >= len(q) {
			return "", 0, false
		}
		if q[j] == c {
			if j+1 < len(q) && q[j+1] == c {
				b.WriteByte(c)
				j += 2
				continue
			}
			return b.String(), j + 1, true
		}
		b.WriteByte(q[j])
		j++
	}
}

// lower lowers ASCII letters only, the way SQLite compares names; it
// allocates only when s has an upper-case one.
func lower(s string) string {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c >= 'A' && c <= 'Z' {
			b := []byte(s)
			for j := i; j < len(b); j++ {
				if b[j] >= 'A' && b[j] <= 'Z' {
					b[j] += 'a' - 'A'
				}
			}
			return string(b)
		}
	}
	return s
}
