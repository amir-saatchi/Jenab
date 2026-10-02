package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"
	"sync"
	"time"
)

// ---- size: state size and where the deciding text sits ----

const needle = "Hackers drain $80 million from the Ronin bridge used by a blockchain game."

// sizedState builds a news list of about `tokens` tokens (estimated at 4 characters per token,
// English only) with the needle at the start, middle or end.
func sizedState(tokens int, pos string) string {
	var lines []string
	chars := 0
	for i := 0; chars < tokens*4; i++ {
		n := newsItems[i%len(newsItems)]
		if n.ID == "c08" { // the exchange hack is too close to the needle
			continue
		}
		l := fmt.Sprintf("Item %d (2026-09-%02d): %s", i+1, 1+i%28, n.EN)
		lines = append(lines, l)
		chars += len(l) + 1
	}
	nl := "Item X (2026-09-30): " + needle
	switch pos {
	case "start":
		lines = append([]string{nl}, lines...)
	case "middle":
		m := len(lines) / 2
		lines = append(lines[:m], append([]string{nl}, lines[m:]...)...)
	default:
		lines = append(lines, nl)
	}
	return strings.Join(lines, "\n")
}

func sizeItems() []*Item {
	var out []*Item
	for _, t := range []int{1000, 8000, 30000, 60000, 90000} {
		for _, pos := range []string{"start", "middle", "end"} {
			out = append(out, &Item{Set: "size", ID: fmt.Sprintf("size_%dk_%s", t/1000, pos), Lang: "en",
				State: sizedState(t, pos),
				Qs: map[string]Q{
					"ronin":    {Type: "noul", Instructions: "Does the list contain an item about a hack of the Ronin bridge?"},
					"football": {Type: "noul", Instructions: "Does the list contain an item about a football cup final?"},
				},
				Want: map[string]Want{"ronin": {Noul: yes()}, "football": {Noul: yes()}},
			})
		}
	}
	return out
}

// cutoffItems put the needle at the end of lists of growing length, to find how much of
// the state the model reads.
func cutoffItems() []*Item {
	var out []*Item
	for _, c := range []int{4000, 5000, 6000, 7000, 7500, 8000, 8500, 9000, 10000, 12000, 16000} {
		out = append(out, &Item{Set: "cutoff", ID: fmt.Sprintf("cutoff_%d", c), Lang: "en",
			State: sizedState(c/4, "end"),
			Qs:    map[string]Q{"ronin": {Type: "noul", Instructions: "Does the list contain an item about a hack of the Ronin bridge?"}},
			Want:  map[string]Want{"ronin": {Noul: yes()}},
		})
	}
	for _, c := range []int{4000, 8000, 16000, 40000} {
		var recs []map[string]string
		for _, l := range strings.Split(sizedState(c/4, "end"), "\n") {
			recs = append(recs, map[string]string{"news": l})
		}
		out = append(out, &Item{Set: "cutoff", ID: fmt.Sprintf("cutoff_%d_array", c), Lang: "en",
			State: recs,
			Qs:    map[string]Q{"ronin": {Type: "noul", Instructions: "Does the list contain an item about a hack of the Ronin bridge?"}},
			Want:  map[string]Want{"ronin": {Noul: yes()}},
		})
	}
	return out
}

// ---- question and option counts ----

var subjects = []string{"bitcoin", "ether", "solana", "dogecoin", "tether", "apple", "volkswagen", "nvidia", "siemens", "coinbase", "gold", "oil", "euro", "dollar", "dax", "bayern"}
var events = []string{"price rise", "price fall", "fund flows (money into or out of funds)", "hack or theft", "outage", "regulation", "earnings report", "product launch", "lawsuit", "partnership", "network upgrade", "layoffs", "record high", "merger", "sponsorship", "interest rates"}

// options returns n options (subject × event) that always include `right`.
func options(n int, right string) map[string]string {
	all := map[string]string{}
	var ids []string
	for _, s := range subjects {
		for j, e := range events {
			id := fmt.Sprintf("%s_%d", s, j)
			all[id] = fmt.Sprintf("%s: %s", s, e)
			ids = append(ids, id)
		}
	}
	out := map[string]string{right: all[right]}
	for i := 0; len(out) < n && i < len(ids); i++ {
		// spread the picks over the whole list so every subject appears
		k := ids[(i*37)%len(ids)]
		out[k] = all[k]
	}
	if n > len(ids) {
		for i := len(ids); len(out) < n; i++ {
			out[fmt.Sprintf("extra_%d", i)] = fmt.Sprintf("other topic %d", i)
		}
	}
	return out
}

func countItems() []*Item {
	var out []*Item
	state := newsItems[0].EN // c01: spot bitcoin ETF inflows
	for _, k := range []int{1, 8, 32, 64} {
		qs := map[string]Q{}
		want := map[string]Want{}
		for i := 0; i < k; i++ {
			s := subjects[i%len(subjects)]
			e := events[(i/len(subjects))%len(events)]
			id := fmt.Sprintf("q%02d", i)
			qs[id] = Q{Type: "noul", Instructions: fmt.Sprintf("Is this news about %s, and about %s?", s, e)}
			want[id] = Want{Noul: &[]bool{s == "bitcoin" && strings.HasPrefix(e, "price rise")}[0]}
		}
		out = append(out, &Item{Set: "questions", ID: fmt.Sprintf("questions_%d", k), Lang: "en", State: state, Qs: qs, NoLLM: true})
		_ = want // the answers are not scored: this measures time only
	}
	for _, n := range []int{10, 50, 100, 255} {
		for _, st := range []struct{ id, text, right string }{
			{"c01", newsItems[0].EN, "bitcoin_2"},
			{"c07", newsItems[6].EN, "ether_2"},
		} {
			out = append(out, &Item{Set: "options", ID: fmt.Sprintf("options_%d_%s", n, st.id), Lang: "en", State: st.text,
				Qs:   map[string]Q{"topic": {Type: "choice", Instructions: "What is this news item about?", Criteria: options(n, st.right)}},
				Want: map[string]Want{"topic": {Choice: []string{st.right}}}, NoLLM: true})
		}
	}
	return out
}

// ---- requests outside the schema, to read the error bodies (no retries) ----

type errCase struct {
	ID   string
	Body func() any
	Key  string // "" = the real token
}

func errCases() []errCase {
	q := func(n int) map[string]Q {
		m := map[string]Q{}
		for i := 0; i < n; i++ {
			m[fmt.Sprintf("q%02d", i)] = Q{Type: "noul", Instructions: "Is this about bitcoin?"}
		}
		return m
	}
	b := func(model string, state any, qs any) map[string]any {
		m := map[string]any{"model": model, "questions": qs}
		if state != nil {
			m["state"] = state
		}
		return m
	}
	st := newsItems[0].EN
	return []errCase{
		{"questions_65", func() any { return b("clef-flash", st, q(65)) }, ""},
		{"options_256", func() any {
			return b("clef-flash", st, map[string]Q{"t": {Type: "choice", Instructions: "Topic?", Criteria: options(256, "bitcoin_2")}})
		}, ""},
		{"score_11_levels", func() any {
			return b("clef-flash", st, map[string]Q{"s": {Type: "score", Instructions: "How big?", Criteria: []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}}})
		}, ""},
		{"choice_1_option", func() any {
			return b("clef-flash", st, map[string]Q{"t": {Type: "choice", Instructions: "Topic?", Criteria: map[string]string{"crypto": "Crypto"}}})
		}, ""},
		{"choice_no_criteria", func() any {
			return b("clef-flash", st, map[string]Q{"t": {Type: "choice", Instructions: "Topic?"}})
		}, ""},
		{"bad_type", func() any { return b("clef-flash", st, map[string]Q{"t": {Type: "rank", Instructions: "Topic?"}}) }, ""},
		{"bad_question_id", func() any {
			return b("clef-flash", st, map[string]Q{"a b/c": {Type: "noul", Instructions: "Bitcoin?"}})
		}, ""},
		{"no_questions", func() any { return b("clef-flash", st, map[string]Q{}) }, ""},
		{"no_state", func() any { return b("clef-flash", nil, q(1)) }, ""},
		{"empty_state", func() any { return b("clef-flash", "", q(1)) }, ""},
		{"wrong_model_in_body", func() any { return b("clef", st, q(1)) }, ""},
		{"unknown_model_in_body", func() any { return b("clef-pro", st, q(1)) }, ""},
		{"remote_image_url", func() any {
			m := b("clef-flash", st, q(1))
			m["images"] = []string{"https://example.com/chart.png"}
			return m
		}, ""},
		{"bad_token", func() any { return b("clef-flash", st, q(1)) }, "BOGUS"},
	}
}

// ---- images: simple charts drawn here ----

func chartPNG(kind string) string {
	const w, h = 480, 300
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)
	grey := color.RGBA{120, 120, 120, 255}
	for x := 40; x < w-20; x++ {
		img.Set(x, h-40, grey)
	}
	for y := 20; y < h-40; y++ {
		img.Set(40, y, grey)
	}
	blue := color.RGBA{30, 90, 220, 255}
	thick := func(x, y int, c color.Color) {
		for dx := -2; dx <= 2; dx++ {
			for dy := -2; dy <= 2; dy++ {
				img.Set(x+dx, y+dy, c)
			}
		}
	}
	switch kind {
	case "up", "down", "flat":
		for x := 50; x < w-30; x++ {
			t := float64(x-50) / float64(w-80)
			wig := 8 * float64((x/23)%3-1)
			var y float64
			switch kind {
			case "up":
				y = float64(h-60) - t*float64(h-110) + wig
			case "down":
				y = 40 + t*float64(h-110) + wig
			default:
				y = float64(h)/2 + wig
			}
			thick(x, int(y), blue)
		}
	default: // bars: "bars3" = the third bar is tallest, "bars1" = the first
		heights := map[string][]int{"bars3": {120, 90, 210, 150}, "bars1": {220, 160, 100, 130}}[kind]
		for i, bh := range heights {
			x0 := 70 + i*100
			for x := x0; x < x0+60; x++ {
				for y := h - 41 - bh; y < h-41; y++ {
					img.Set(x, y, blue)
				}
			}
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func imageItems() []*Item {
	trend := Q{Type: "choice", Instructions: "Which way does the line in the chart go from left to right?", Criteria: map[string]string{"up": "Up", "down": "Down", "flat": "Flat, no clear trend"}}
	bars := Q{Type: "choice", Instructions: "Which bar in the chart is the tallest, counting from the left?", Criteria: map[string]string{"first": "The first bar", "second": "The second bar", "third": "The third bar", "fourth": "The fourth bar"}}
	var out []*Item
	for _, k := range []string{"up", "down", "flat"} {
		out = append(out, &Item{Set: "image", ID: "image_" + k, Lang: "en", State: "A chart from the price_chart view.", Images: []string{chartPNG(k)},
			Qs: map[string]Q{"trend": trend}, Want: map[string]Want{"trend": {Choice: []string{k}}}, NoLLM: true})
	}
	for k, right := range map[string]string{"bars3": "third", "bars1": "first"} {
		out = append(out, &Item{Set: "image", ID: "image_" + k, Lang: "en", State: "A bar chart of news items per day.", Images: []string{chartPNG(k)},
			Qs: map[string]Q{"tallest": bars}, Want: map[string]Want{"tallest": {Choice: []string{right}}}, NoLLM: true})
	}
	return out
}

// ---- burst: many calls at once, no retries, to see the rate limits ----

func runBurst(ctx context.Context, model string, conc, n int) {
	it := classifyItems()[0]
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < n; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			rec, ans := askClef(ctx, model, clefBody{Model: model, State: it.State, Questions: it.Qs}, false)
			rec.Run, rec.Set, rec.Item, rec.Lang, rec.Rep = "burst", "burst", fmt.Sprintf("burst_c%d", conc), "en", i
			rec.Answers = ans
			if ans != nil {
				score(it, &rec)
			}
			save(rec)
		}(i)
	}
	wg.Wait()
	logf("burst %s c=%d n=%d took %s", model, conc, n, time.Since(start).Round(time.Millisecond))
}
