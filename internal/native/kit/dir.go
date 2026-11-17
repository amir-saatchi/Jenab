package kit

import "golang.org/x/text/unicode/bidi"

// RightToLeft reports whether s reads right to left: whether its first
// strong character, by its Unicode bidi class, is right to left (R or AL:
// Hebrew, Arabic, Persian…), as dir="auto" finds it. A strong left-to-right
// character first (L: Latin, Greek, Cyrillic, Chinese, Japanese, Thai,
// Devanagari…) makes it left to right. Digits, Arabic and Persian ones
// too, punctuation and signs such as × and ÷ are weak or neutral and are
// passed over.
func RightToLeft(s string) bool {
	for _, r := range s {
		p, _ := bidi.LookupRune(r)
		switch p.Class() {
		case bidi.R, bidi.AL:
			return true
		case bidi.L:
			return false
		}
	}
	return false
}
