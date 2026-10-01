package main

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// normalizer is applied by Go to text before indexing and to queries.
type normalizer struct {
	name  string
	f     func(string) string
	query func(q string, prefix bool) string // nil: ftsQuery(f(q), prefix)
}

func (n normalizer) build(q string, prefix bool) string {
	if n.query != nil {
		return n.query(q, prefix)
	}
	return ftsQuery(n.f(q), prefix)
}

var normalizers = []normalizer{
	{"none", func(s string) string { return s }, nil},
	{"Go, ZWNJ → space", func(s string) string { return normalize(s, " ") }, nil},
	{"Go, ZWNJ removed", func(s string) string { return normalize(s, "") }, nil},
	{"Go, ZWNJ removed; query term with ZWNJ also tries the split form", func(s string) string { return normalize(s, "") }, ftsQueryZWNJ},
}

// ftsQueryZWNJ is for text indexed with ZWNJ removed. A query term that has a
// ZWNJ matches either the joined form or the two parts side by side:
// بیت‌کوین → ("بیتکوین"* OR "بیت کوین"*).
func ftsQueryZWNJ(q string, prefix bool) string {
	star := ""
	if prefix {
		star = "*"
	}
	quote := func(t string) string { return `"` + strings.ReplaceAll(t, `"`, `""`) + `"` + star }
	var terms []string
	for _, t := range strings.FieldsFunc(normalize(q, "‌"), unicode.IsSpace) {
		if strings.Contains(t, "‌") {
			terms = append(terms, "("+quote(strings.ReplaceAll(t, "‌", ""))+" OR "+quote(strings.ReplaceAll(t, "‌", " "))+")")
		} else {
			terms = append(terms, quote(t))
		}
	}
	// explicit AND: FTS5 has no implicit AND next to a parenthesised group
	return strings.Join(terms, " AND ")
}

// normalize:
//   - NFKC
//   - Arabic yeh/kaf → Persian (ي ى → ی, ك → ک), Arabic heh variants left alone
//   - tatweel removed
//   - Arabic diacritics (harakat, U+064B–U+065F, U+0670) removed
//   - Persian and Arabic-Indic digits → ASCII
//   - ß → ss
//   - ZWNJ (U+200C) → zwnj
func normalize(s, zwnj string) string {
	s = norm.NFKC.String(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == 'ي' || r == 'ى':
			b.WriteRune('ی')
		case r == 'ك':
			b.WriteRune('ک')
		case r == 'ـ': // tatweel
		case r >= 0x064B && r <= 0x065F, r == 0x0670:
		case r >= '۰' && r <= '۹':
			b.WriteRune('0' + (r - '۰'))
		case r >= '٠' && r <= '٩':
			b.WriteRune('0' + (r - '٠'))
		case r == 'ß':
			b.WriteString("ss")
		case r == 'ẞ':
			b.WriteString("SS")
		case r == 0x200C:
			b.WriteString(zwnj)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ftsQuery turns free text into a safe FTS5 query: every whitespace-separated
// term becomes a quoted string (so operators and syntax characters are just
// text), optionally with a prefix star. Terms are ANDed.
func ftsQuery(q string, prefix bool) string {
	var terms []string
	for _, t := range strings.FieldsFunc(q, unicode.IsSpace) {
		t = `"` + strings.ReplaceAll(t, `"`, `""`) + `"`
		if prefix {
			t += "*"
		}
		terms = append(terms, t)
	}
	return strings.Join(terms, " ")
}
