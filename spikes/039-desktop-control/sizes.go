package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// sizes snapshots each test app (full tree and interactive-only), prints the compact form, and
// counts tokens. Gemini's countTokens is used for the editor, Calculator and the Edge window
// (synthetic content only). The Explorer window shows the user's own folders in its navigation
// pane, so its text never leaves the machine: its tokens are estimated from the others' ratios.

type SizeRow struct {
	App, Form    string
	Bytes, Lines int
	Nodes, Refs  int
	Tokens       int
	Estimated    bool
	Ms           int64
	Complete     bool
}

func runSizes(cfg *Config) error {
	if err := loadKey(cfg.Env); err != nil {
		return err
	}
	g := &Gemini{Model: "gemini-3.5-flash-lite", OutDir: cfg.Out}
	cli := cfg.CLI
	dir := filepath.Join(cfg.Out, "sizes")
	os.MkdirAll(dir, 0o755)
	var rows []SizeRow

	type target struct {
		name    string
		launch  func() (*App, error)
		private bool
		depth   string
		prepare func(*App)
	}
	targets := []target{
		{name: "editor", launch: func() (*App, error) { return launchEditor(cli, "SPIKE-039 Test Editor") }, depth: "10",
			prepare: func(a *App) {
				if s, _, _, err := cli.snapshot(a.Win.ID, true); err == nil {
					if n := findNode(s.Tree, func(n SnapNode) bool {
						return n.Ref != "" && (n.Role == "textfield" || n.Role == "document" || n.Role == "textarea")
					}); n != nil {
						cli.Run(10*time.Second, "set-value", n.Ref, "A short paragraph so the box is not empty.\nSecond line.")
					}
				}
			}},
		{name: "calculator", launch: func() (*App, error) { return launchCalculator(cli) }, depth: "10"},
		{name: "explorer", launch: func() (*App, error) { return launchExplorer(cli, cfg.Scratch) }, private: true, depth: "10"},
		{name: "edge", launch: func() (*App, error) { return launchEdge(cli, cfg.Scratch) }, depth: "45"},
	}
	ratio := map[string][2]int{} // form -> (tokens, bytes) over counted apps
	for _, t := range targets {
		fmt.Println("==", t.name)
		app, err := t.launch()
		if err != nil {
			return fmt.Errorf("%s: %w", t.name, err)
		}
		time.Sleep(3 * time.Second) // let it settle
		if t.name == "edge" {
			// Chromium reads thin until its renderer accessibility is on: wait for the page.
			for end := time.Now().Add(20 * time.Second); time.Now().Before(end); time.Sleep(time.Second) {
				if s, _, _, err := cli.snapshot(app.Win.ID, true, "--max-depth", t.depth, "--timeout-ms", "15000"); err == nil {
					if d := pageDoc(s.Tree); d != nil && len(d.Children) > 0 {
						break
					}
				}
			}
		}
		if t.prepare != nil {
			t.prepare(app)
		}
		for _, interactive := range []bool{false, true} {
			form := "full"
			if interactive {
				form = "interactive"
			}
			s, raw, took, err := cli.snapshot(app.Win.ID, interactive, "--max-depth", t.depth, "--timeout-ms", "15000")
			if err != nil {
				app.Close()
				return fmt.Errorf("%s %s: %w", t.name, form, err)
			}
			all, refs := countNodes(s.Tree)
			c := compact(s, 1)
			for _, v := range []struct {
				form, text string
				lines      int
			}{{form + " json", string(raw), 0}, {form + " compact", c.Text, c.Lines}} {
				row := SizeRow{App: t.name, Form: v.form, Bytes: len(v.text), Lines: v.lines, Nodes: all, Refs: refs,
					Ms: took.Milliseconds(), Complete: s.Complete}
				if !t.private {
					n, err := g.CountTokens(v.text)
					if err != nil {
						app.Close()
						return err
					}
					row.Tokens = n
					r := ratio[v.form]
					ratio[v.form] = [2]int{r[0] + n, r[1] + len(v.text)}
					os.WriteFile(filepath.Join(dir, t.name+"-"+strings.ReplaceAll(v.form, " ", "-")+ext(v.form)), []byte(v.text), 0o644)
				}
				rows = append(rows, row)
			}
			// Edge: how much is the browser frame and how much the page?
			if t.name == "edge" {
				if doc := pageDoc(s.Tree); doc != nil {
					page := *s
					page.Tree = *doc
					pc := compact(&page, 1)
					n, err := g.CountTokens(pc.Text)
					if err == nil {
						rows = append(rows, SizeRow{App: "edge page only", Form: form + " compact", Bytes: len(pc.Text), Lines: pc.Lines, Tokens: n, Complete: s.Complete})
						os.WriteFile(filepath.Join(dir, "edge-page-"+form+"-compact.txt"), []byte(pc.Text), 0o644)
					}
				}
			}
		}
		app.Close()
	}
	for i, r := range rows {
		if r.App == "explorer" {
			q := ratio[r.Form]
			if q[1] > 0 {
				rows[i].Tokens = r.Bytes * q[0] / q[1]
				rows[i].Estimated = true
			}
		}
	}
	fmt.Printf("\n%-16s %-20s %8s %6s %6s %5s %8s %6s\n", "app", "form", "bytes", "lines", "nodes", "refs", "tokens", "ms")
	for _, r := range rows {
		est := ""
		if r.Estimated {
			est = "~"
		}
		fmt.Printf("%-16s %-20s %8d %6d %6d %5d %7s%d %6d %v\n", r.App, r.Form, r.Bytes, r.Lines, r.Nodes, r.Refs, est, r.Tokens, r.Ms, map[bool]string{true: "", false: "INCOMPLETE"}[r.Complete])
	}
	return writeJSON(filepath.Join(cfg.Out, "sizes.json"), rows)
}

func ext(form string) string {
	if strings.HasSuffix(form, "json") {
		return ".json"
	}
	return ".txt"
}
