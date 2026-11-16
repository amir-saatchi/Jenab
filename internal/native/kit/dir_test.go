package kit

import "testing"

// The cases of the frontend's lib/dir.test.ts.
func TestRightToLeft(t *testing.T) {
	for s, want := range map[string]bool{
		"سلام, hello": true,
		"Hello سلام":  false,
		"۸ صبح":       true,
		"۶۴ BTC":      false,
		"12 %":        false,
		"- خرید نان":  true,
		"| نام | سن |": true,
	} {
		if got := RightToLeft(s); got != want {
			t.Errorf("RightToLeft(%q) = %v", s, got)
		}
	}
}
