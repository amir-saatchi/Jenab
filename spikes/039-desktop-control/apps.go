package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// The spike only drives windows it opened itself: the test editor (its own process), Calculator,
// one File Explorer window on a test folder, and Edge on a local page with a fresh profile.
// close() posts WM_CLOSE to that exact window. It kills a process only when the spike started
// it (the editor, Edge with its own profile, Calculator when none was running before).

type App struct {
	Name  string
	Win   Window
	close func()
}

func (a *App) Close() {
	if a != nil && a.close != nil {
		a.close()
		a.close = nil
	}
}

func ids(ws []Window) map[string]bool {
	m := map[string]bool{}
	for _, w := range ws {
		m[w.ID] = true
	}
	return m
}

func launchEditor(cli *CLI, title string, extra ...string) (*App, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(self, append([]string{"editor", "-title", title}, extra...)...)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	pid := cmd.Process.Pid
	go cmd.Wait()
	w, err := cli.WaitWindow("spike039", 15*time.Second, func(w Window) bool { return w.PID == pid })
	app := &App{Name: "editor", Win: w, close: func() {
		if !closeWindow(hwndOf(w.ID), 3*time.Second) || processAlive(pid) {
			killTree(pid)
		}
	}}
	if err != nil {
		app.Win = Window{}
		killTree(pid)
		return nil, err
	}
	return app, nil
}

func launchCalculator(cli *CLI) (*App, error) {
	before, _ := cli.ListWindows("CalculatorApp")
	seen := ids(before)
	wasRunning := len(before) > 0
	if err := exec.Command("calc.exe").Start(); err != nil {
		return nil, err
	}
	w, err := cli.WaitWindow("CalculatorApp", 20*time.Second, func(w Window) bool { return !seen[w.ID] })
	if err != nil {
		return nil, err
	}
	return &App{Name: "calculator", Win: w, close: func() {
		closeWindow(hwndOf(w.ID), 5*time.Second)
		if !wasRunning && processAlive(w.PID) {
			killTree(w.PID)
		}
	}}, nil
}

// makeTestFolder writes a small folder of dummy files for Explorer.
func makeTestFolder(scratch string) (string, error) {
	dir := filepath.Join(scratch, "spike039-files")
	for _, sub := range []string{"", "Drafts", "Receipts"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return "", err
		}
	}
	for i, name := range []string{"notes.txt", "plan.md", "budget.csv", "photo-list.txt", "readme.txt",
		"Drafts/letter.txt", "Drafts/ideas.md", "Receipts/2026-10.csv"} {
		os.WriteFile(filepath.Join(dir, name), []byte(strings.Repeat("test line\n", i+1)), 0o644)
	}
	return dir, nil
}

func launchExplorer(cli *CLI, scratch string) (*App, error) {
	dir, err := makeTestFolder(scratch)
	if err != nil {
		return nil, err
	}
	before, _ := cli.ListWindows("explorer")
	seen := ids(before)
	if err := exec.Command("explorer.exe", dir).Start(); err != nil {
		return nil, err
	}
	w, err := cli.WaitWindow("explorer", 20*time.Second, func(w Window) bool {
		return !seen[w.ID] && strings.HasPrefix(w.Title, "spike039-files")
	})
	if err != nil {
		return nil, err
	}
	// explorer.exe is the user's shell: only the window is closed, never the process.
	return &App{Name: "explorer", Win: w, close: func() { closeWindow(hwndOf(w.ID), 5*time.Second) }}, nil
}

const edgePath = `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`

// testPage is a local page with the parts a busy page has: navigation, a form, a table, a list.
func testPage() string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="en"><head><meta charset="utf-8"><title>SPIKE-039 Test Page</title></head><body>
<header><nav aria-label="Main"><ul>`)
	for _, s := range []string{"Home", "Products", "Pricing", "Docs", "Blog", "Careers", "Support", "Status", "Contact", "About", "Press", "Partners"} {
		fmt.Fprintf(&b, `<li><a href="#%s">%s</a></li>`, strings.ToLower(s), s)
	}
	b.WriteString(`</ul></nav></header><main><h1>Order a test widget</h1>
<p>This page exists only for the desktop-control spike. Nothing on it is sent anywhere.</p>
<form onsubmit="document.getElementById('out').textContent='Sent: '+this.topic.value;return false">
<label>Topic <input name="topic" type="text"></label>
<label>Message <textarea name="message" rows="3"></textarea></label>
<label>Size <select name="size"><option>Small</option><option>Medium</option><option>Large</option></select></label>
<fieldset><legend>Extras</legend><label><input type="checkbox" name="gift"> Gift wrap</label>
<label><input type="checkbox" name="express"> Express delivery</label><label><input type="checkbox" name="news"> Newsletter</label></fieldset>
<fieldset><legend>Colour</legend><label><input type="radio" name="c" value="red"> Red</label>
<label><input type="radio" name="c" value="green"> Green</label><label><input type="radio" name="c" value="blue"> Blue</label></fieldset>
<button type="submit">Send</button> <button type="reset">Reset</button></form><p id="out" role="status"></p>
<h2>Price list</h2><table><thead><tr><th>Item</th><th>Size</th><th>Price</th><th>Stock</th></tr></thead><tbody>`)
	for i := 1; i <= 15; i++ {
		fmt.Fprintf(&b, "<tr><td>Widget %d</td><td>%s</td><td>%d.00 EUR</td><td>%d</td></tr>", i, []string{"S", "M", "L"}[i%3], 10+i*3, 100-i*4)
	}
	b.WriteString(`</tbody></table><h2>Recent changes</h2><ul>`)
	for i := 1; i <= 10; i++ {
		fmt.Fprintf(&b, "<li>Change %d: adjusted the widget catalogue</li>", i)
	}
	b.WriteString(`</ul></main><footer><a href="#privacy">Privacy</a> <a href="#terms">Terms</a> <a href="#cookies">Cookies</a></footer></body></html>`)
	return b.String()
}

// launchEdge serves the test page from 127.0.0.1 (a file:// URL would carry the user's profile
// path into every snapshot) and opens it in Edge with a fresh profile in the scratch folder.
func launchEdge(cli *CLI, scratch string) (*App, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, testPage())
	})}
	go srv.Serve(ln)
	pageURL := "http://" + ln.Addr().String() + "/test.html"
	profile := filepath.Join(scratch, "edge-profile")
	cmd := exec.Command(edgePath, "--user-data-dir="+profile, "--no-first-run", "--no-default-browser-check",
		"--disable-sync", "--new-window", pageURL)
	if err := cmd.Start(); err != nil {
		srv.Close()
		return nil, err
	}
	pid := cmd.Process.Pid
	go cmd.Wait()
	end := func() {
		time.Sleep(time.Second)
		killTree(pid)
		srv.Close()
	}
	w, err := cli.WaitWindow("msedge", 30*time.Second, func(w Window) bool {
		return w.PID == pid && strings.Contains(w.Title, "SPIKE-039 Test Page")
	})
	if err != nil {
		end()
		return nil, err
	}
	return &App{Name: "edge", Win: w, close: func() {
		closeWindow(hwndOf(w.ID), 5*time.Second)
		end()
	}}, nil
}
