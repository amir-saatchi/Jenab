package kit

import "testing"

// The cases of the old frontend's lib/dir.test.ts, then scripts that the
// first version took as neither direction (TASK-069).
func TestRightToLeft(t *testing.T) {
	for s, want := range map[string]bool{
		"سلام, hello":  true,
		"Hello سلام":   false,
		"۸ صبح":        true,
		"۶۴ BTC":       false,
		"١٢ BTC":       false,
		"12 %":         false,
		"- خرید نان":   true,
		"| نام | سن |": true,
		"":             false,
		"שלום world":   true,
		// Strong left-to-right letters outside Latin, Greek and Cyrillic.
		"Đà Nẵng سلام":   false, // Vietnamese, Latin Extended Additional
		"ẞ سلام":         false,
		"你好 سلام":        false,
		"こんにちは سلام":     false,
		"안녕 سلام":        false,
		"สวัสดี سلام":    false,
		"नमस्ते سلام":    false,
		"გამარჯობა سلام": false,
		"Բարեւ سلام":     false,
		// Signs and digits are passed over.
		"× سلام":     true,
		"÷ سلام":     true,
		"3 × 4 = 12": false,
		"(۱۲) نان":   true,
	} {
		if got := RightToLeft(s); got != want {
			t.Errorf("RightToLeft(%q) = %v", s, got)
		}
	}
}
