package main

// Part A.3, second look: other ways to take the text where `get html`
// plus readable text finds nothing (shadow DOM, iframes, lazy sections,
// canvas).

import (
	"context"
	"encoding/json"
	"strconv"
	"time"
)

func partExtract(ctx context.Context) {
	e, err := newEnv()
	if err != nil {
		p("setup: %v", err)
		return
	}
	b := browsers()[0]
	p("## 3c. Other ways to take the text (%s)\n", b.ID)
	p("Characters and phrases found by each method after open + network idle + settle. html = `get html html` through readable text (as in 3); text = `get text body` (the page's innerText); snapshot = `snapshot` (the accessibility tree agent-browser gives agents); scrolled = html after `scroll down 4000` three times.\n")
	p("| Page | html | text | snapshot | snapshot -c | scrolled html |")
	p("|---|---|---|---|---|---|")
	a := e.ab(b, "extract")
	a.Setup()
	defer func() { a.Close(ctx); waitGone(10 * time.Second) }()
	for _, id := range jsMain {
		pg := jsPages[id]
		u := e.S.Base() + "/js/" + id + pg.Hash
		tm, r := visit(ctx, a, u)
		if !r.Success {
			p("| %s | open failed: %s |||||", id, clip(r.Error, 80))
			continue
		}
		cell := func(s string) string {
			return strconv.Itoa(chars(s)) + " · " + phrasesHit(s, pg.Phrases)
		}
		tx := a.Run(ctx, 20*time.Second, "get", "text", "body").field("text")
		sn := snapText(a.Run(ctx, 20*time.Second, "snapshot"))
		sc := snapText(a.Run(ctx, 20*time.Second, "snapshot", "-c"))
		for i := 0; i < 3; i++ {
			a.Run(ctx, 10*time.Second, "scroll", "down", "4000")
			time.Sleep(700 * time.Millisecond)
		}
		_, sh, _ := rendered(ctx, a, u)
		p("| %s | %s | %s | %s | %s | %s |", id, cell(tm.text), cell(tx), cell(sn), cell(sc), cell(sh))
	}
}

// snapText is the snapshot's text, whatever field it comes in.
func snapText(r Result) string {
	if !r.Success {
		return ""
	}
	if s := r.field("snapshot"); s != "" {
		return s
	}
	var v any
	json.Unmarshal(r.Data, &v)
	b, _ := json.Marshal(v)
	return string(b)
}
