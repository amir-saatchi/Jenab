package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const chromePath = `C:\Program Files\Google\Chrome\Application\chrome.exe`

// ChromeOpts: the profile folder (in the scratch folder; a new one per run gives Google's consent
// dialog, a reused one may not) and the languages the page is asked for.
type ChromeOpts struct {
	Profile    string
	AcceptLang string // e.g. "de-DE,de" or "en-US,en"; empty: Chrome's default
	StartURL   string // empty: about:blank
	UILang     string // Chrome's own language; empty: en-US
}

// launchChrome starts Chrome with its own profile folder on about:blank, with its UI in English
// (the address bar's name is part of a macro). The window is the one owned by the process started
// here; close kills that process tree only. The user's own Chrome and profile are never touched.
func launchChrome(cli *CLI, o ChromeOpts) (*App, error) {
	args := []string{"--user-data-dir=" + o.Profile, "--no-first-run", "--no-default-browser-check",
		"--disable-search-engine-choice-screen", "--disable-sync"}
	ui := "en-US"
	if o.UILang != "" {
		ui = o.UILang
	}
	args = append(args, "--lang="+ui)
	if o.AcceptLang != "" {
		args = append(args, "--accept-lang="+o.AcceptLang)
	}
	start := "about:blank"
	if o.StartURL != "" {
		start = o.StartURL
	}
	args = append(args, "--new-window", start)
	cmd := exec.Command(chromePath, args...)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	pid := cmd.Process.Pid
	go cmd.Wait()
	w, err := cli.WaitWindow("chrome", 30*time.Second, func(w Window) bool { return w.PID == pid })
	if err != nil {
		killTree(pid)
		return nil, err
	}
	return &App{Name: "chrome", Win: w, close: func() {
		closeWindow(hwndOf(w.ID), 5*time.Second)
		time.Sleep(time.Second)
		killTree(pid)
	}}, nil
}

// Consent: only a reject button may be clicked. The accept list is the safety rule (the same for
// every model and every replay).
var (
	reAccept = regexp.MustCompile(`(?i)\b(accept|agree|akzeptier|zustimm|einverstanden|allow all|alle zulassen|accepter|accetta|aceptar)`)
	reReject = regexp.MustCompile(`(?i)^(reject all|alle ablehnen)$`)
)

// runChromeProbe searches Google once per language without a model and prints the consent
// dialog's buttons and the first result links.
func runChromeProbe(cfg *Config, langs []string, profile string) error {
	for _, lang := range langs {
		p := profile
		if p == "" {
			p = cfg.Scratch + `\chrome-probe-` + time.Now().Format("150405")
		}
		app, err := launchChrome(cfg.CLI, ChromeOpts{Profile: p, AcceptLang: lang, StartURL: os.Getenv("START"), UILang: os.Getenv("UILANG")})
		if err != nil {
			return err
		}
		time.Sleep(time.Duration(envInt("STARTWAIT", 2000)) * time.Millisecond)
		show := func(label string) *SnapData {
			snap, raw, _, err := cfg.CLI.snapshot(app.Win.ID, true, browserSnapArgs...)
			os.WriteFile(filepath.Join(cfg.Out, "probe-"+label+".json"), raw, 0o644)
			if err != nil {
				fmt.Println(label, "snapshot:", err)
				return nil
			}
			d := pageNode(snap.Tree)
			page := ""
			if d != nil {
				page = origin(d.Value)
			}
			fmt.Printf("[%s %s] page %s, title %q\n", lang, label, page, snap.Window.Title)
			if d != nil {
				c := compactNodes([]SnapNode{*d}, 1)
				lines := strings.Split(c.Text, "\n")
				for i, l := range lines {
					if i < 40 || strings.Contains(l, "button") && i < 120 {
						fmt.Println("   ", l)
					}
				}
			}
			return snap
		}
		s := show("blank")
		if s != nil && os.Getenv("REJECT") != "" {
			if r := findNode(s.Tree, func(n SnapNode) bool { return n.Ref != "" && n.Role == "button" && reReject.MatchString(n.Name) }); r != nil {
				_, _, _, err := cfg.CLI.Run(15*time.Second, "click", r.Ref)
				fmt.Printf("clicked %q: %v\n", r.Name, err)
				time.Sleep(4 * time.Second)
				s = show("after reject")
			}
		}
		var omni *SnapNode
		if s != nil {
			omni = omnibox(s.Tree)
		}
		if omni == nil {
			app.Close()
			return fmt.Errorf("no omnibox")
		}
		fmt.Printf("omnibox role %q name %q\n", omni.Role, omni.Name)
		fg := foregroundWindow()
		if os.Getenv("FOCUS") != "" {
			_, _, _, err := cfg.CLI.Run(15*time.Second, "focus", omni.Ref)
			fmt.Println("focus:", err)
		}
		_, _, _, err = cfg.CLI.Run(15*time.Second, "set-value", omni.Ref, "SQLite WAL mode")
		fmt.Println("set-value:", err)
		for try := 1; try <= envInt("TRIES", 3); try++ {
			time.Sleep(time.Duration(envInt("PRESSWAIT", 1000)) * time.Millisecond)
			if f := focusedNode(cfg.CLI, app.Win.ID); f != nil {
				fmt.Printf("focused before press: %s %q\n", f.Role, f.Name)
			}
			_, _, _, err = cfg.CLI.Run(15*time.Second, "press", "return", "--window-id", app.Win.ID)
			time.Sleep(3 * time.Second)
			ws, _ := cfg.CLI.ListWindows("chrome")
			title := ""
			for _, w := range ws {
				if w.ID == app.Win.ID {
					title = w.Title
				}
			}
			fmt.Printf("press %d: %v, title %q, fg changed %v\n", try, err, title, foregroundWindow() != fg)
			if !strings.HasPrefix(title, "about:blank") {
				break
			}
		}
		time.Sleep(time.Duration(envInt("AFTER", 2000)) * time.Millisecond)
		show("after search")
		app.Close()
	}
	return nil
}

// focusedNode reads the window again and returns the last element in tree order with the
// keyboard focus (a page's document can say focused next to the element inside it).
func focusedNode(cli *CLI, winID string) *SnapNode {
	snap, _, _, err := cli.snapshot(winID, true, browserSnapArgs...)
	if err != nil {
		return nil
	}
	var last *SnapNode
	var walk func(n SnapNode)
	walk = func(n SnapNode) {
		for _, st := range n.States {
			if st == "focused" {
				m := n
				last = &m
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(snap.Tree)
	return last
}

// paceGoogle keeps live Google searches at least a minute apart (one CAPTCHA came after ~10
// searches in an hour), across runs, with a time stamp file in out/.
func paceGoogle(out string) {
	f := filepath.Join(out, "last-google.txt")
	if b, err := os.ReadFile(f); err == nil {
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(b))); err == nil {
			if wait := time.Minute - time.Since(t); wait > 0 {
				fmt.Printf("   pacing Google: %.0f s\n", wait.Seconds())
				time.Sleep(wait)
			}
		}
	}
	os.WriteFile(f, []byte(time.Now().Format(time.RFC3339)), 0o644)
}

// out/captchas.txt keeps every CAPTCHA, one per line, across runs: two stop all Google runs.
func noteCaptcha(out, why string) {
	f, err := os.OpenFile(filepath.Join(out, "captchas.txt"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, time.Now().Format(time.RFC3339), why)
}

func captchaCount(out string) int {
	b, err := os.ReadFile(filepath.Join(out, "captchas.txt"))
	if err != nil {
		return 0
	}
	return len(strings.Fields(strings.ReplaceAll(string(b), " ", "_")))
}
