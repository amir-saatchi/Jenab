package main

import (
	"encoding/json"
	"os"
	"testing"
)

// The probe's saved results page (out/ is not committed, so this skips without a probe run).
func TestGoogleResultsOnProbePage(t *testing.T) {
	b, err := os.ReadFile("out/chrome-probe/3-after-reject.json")
	if err != nil {
		t.Skip("no probe page:", err)
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	json.Unmarshal(b, &env)
	snap, err := parseSnap(env.Data)
	if err != nil {
		t.Fatal(err)
	}
	doc := pageNode(snap.Tree)
	if doc == nil {
		t.Fatal("no page")
	}
	got := googleResults(*doc)
	for i, r := range got {
		t.Logf("%d %q %q %q", i+1, r.Title, r.Site, r.Cite)
	}
	if len(got) < 5 {
		t.Fatalf("%d results", len(got))
	}
	answer, cites := "", 0
	for _, r := range got[:5] {
		answer += "1. " + r.Title + " - " + r.Cite + "\n"
		if r.Cite != "" {
			cites++ // a Reddit thread shows no address in its link
		}
	}
	if m, h := gradeResults(got[:5], answer); m != 5 || h != cites {
		t.Errorf("own list graded %d titles, %d hosts of %d", m, h, cites)
	}
	swapped := "1. " + got[1].Title + "\n2. " + got[0].Title + "\n3. " + got[2].Title + "\n4. " + got[3].Title + "\n5. " + got[4].Title
	if m, _ := gradeResults(got[:5], swapped); m == 5 {
		t.Error("a swapped order passed")
	}
}
