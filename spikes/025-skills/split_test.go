package main

import "testing"

// The five split skills plus the intro give guide v2 back byte for byte, so condition A has
// exactly the guide text that B/C can load.
func TestSplitIsGuideV2(t *testing.T) {
	loadGuides()
	loadSkills()
	g, want := rebuildGuide(), guides["v2"]
	if g == want {
		return
	}
	i := 0
	for i < len(g) && i < len(want) && g[i] == want[i] {
		i++
	}
	lo := i - 80
	if lo < 0 {
		lo = 0
	}
	hi := func(s string) int {
		if i+80 < len(s) {
			return i + 80
		}
		return len(s)
	}
	t.Fatalf("differs at byte %d (rebuilt %d, guide %d):\nrebuilt: %q\nguide:   %q", i, len(g), len(want), g[lo:hi(g)], want[lo:hi(want)])
}
