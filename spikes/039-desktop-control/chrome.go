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

// launchChrome starts Chrome with its own profile folder (a new one per run, so no consent
// cookie carries over) on about:blank. The window is the one owned by the process started here;
// close kills that process tree only. The user's own Chrome (another profile) is never touched.
func launchChrome(cli *CLI, profile string) (*App, error) {
	cmd := exec.Command(chromePath, "--user-data-dir="+profile, "--no-first-run", "--no-default-browser-check",
		"--disable-search-engine-choice-screen", "--disable-sync", "--new-window", "about:blank")
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

// runChromeProbe searches Google once without a model, to see the page's tree: the omnibox is
// set and Return pressed (the window comes to the front), a consent page is answered only with
// its reject button, and full snapshots are saved to out/chrome-probe.
func runChromeProbe(cfg *Config, focus bool) error {
	dir := filepath.Join(cfg.Out, "chrome-probe")
	os.MkdirAll(dir, 0o755)
	app, err := launchChrome(cfg.CLI, filepath.Join(cfg.Scratch, "chrome-probe-"+time.Now().Format("150405")))
	if err != nil {
		return err
	}
	defer app.Close()
	time.Sleep(2 * time.Second)
	// save snapshots until ready says the page is there (20 s at most).
	save := func(name string, ready func(*SnapData) bool) *SnapData {
		var snap *SnapData
		var raw []byte
		start := time.Now()
		for {
			var err error
			snap, raw, _, err = cfg.CLI.snapshot(app.Win.ID, false, "--timeout-ms", "15000", "--max-depth", "40", "--compact")
			if err != nil {
				fmt.Println(name, "snapshot:", err)
				return nil
			}
			if ready(snap) || time.Since(start) > 20*time.Second {
				break
			}
			time.Sleep(time.Second)
		}
		fmt.Printf("%s after %.1f s: ", name, time.Since(start).Seconds())
		os.WriteFile(filepath.Join(dir, name+".json"), raw, 0o644)
		c := compact(snap, 1)
		os.WriteFile(filepath.Join(dir, name+".txt"), []byte(c.Text), 0o644)
		fmt.Printf("%s: %d lines, title %q\n", name, c.Lines, snap.Window.Title)
		return snap
	}
	isReject := func(n SnapNode) bool { return n.Ref != "" && n.Role == "button" && reReject.MatchString(n.Name) }
	isResult := func(n SnapNode) bool {
		return n.Role == "link" && findNode(n, func(h SnapNode) bool { return h.Role == "heading" }) != nil
	}
	loaded := func(s *SnapData) bool {
		d := pageDoc(s.Tree)
		return d != nil && (findNode(*d, isReject) != nil || findNode(*d, isResult) != nil)
	}
	s := save("1-blank", func(*SnapData) bool { return true })
	if s == nil {
		return nil
	}
	omni := findNode(s.Tree, func(n SnapNode) bool { return n.Ref != "" && strings.Contains(n.Name, "Address and search") })
	if omni == nil {
		return fmt.Errorf("no omnibox in the snapshot")
	}
	if focus {
		fg := foregroundWindow()
		_, _, took, err := cfg.CLI.Run(15*time.Second, "focus", "--headed", omni.Ref)
		fmt.Printf("focus omnibox: %v (%.0f ms), foreground changed: %v\n", err, took.Seconds()*1000, foregroundWindow() != fg)
	}
	_, _, took, err := cfg.CLI.Run(15*time.Second, "set-value", omni.Ref, "Wails v3 Go desktop framework")
	fmt.Printf("set-value omnibox: %v (%.0f ms)\n", err, took.Seconds()*1000)
	time.Sleep(2 * time.Second)
	if s, _, _, err := cfg.CLI.snapshot(app.Win.ID, true); err == nil {
		if n := findNode(s.Tree, func(n SnapNode) bool { return strings.Contains(n.Name, "Address and search") }); n != nil {
			fmt.Printf("omnibox now %q %v\n", clip(n.Value, 60), n.States)
		}
	}
	// Return after a set value reaches the omnibox only sometimes; try up to three times.
	for try := 1; try <= 3; try++ {
		_, _, took, err = cfg.CLI.Run(15*time.Second, "press", "return", "--window-id", app.Win.ID)
		time.Sleep(3 * time.Second)
		ws, _ := cfg.CLI.ListWindows("chrome")
		title := ""
		for _, w := range ws {
			if w.ID == app.Win.ID {
				title = w.Title
			}
		}
		fmt.Printf("press return %d: %v (%.0f ms), title %q\n", try, err, took.Seconds()*1000, title)
		if !strings.HasPrefix(title, "about:blank") {
			break
		}
	}
	s = save("2-after-search", loaded)
	if s == nil {
		return nil
	}
	if reject := findNode(s.Tree, isReject); reject != nil {
		_, _, _, err := cfg.CLI.Run(15*time.Second, "click", reject.Ref)
		fmt.Printf("click %q: %v\n", reject.Name, err)
		save("3-after-reject", func(s *SnapData) bool { d := pageDoc(s.Tree); return d != nil && findNode(*d, isResult) != nil })
	}
	return nil
}

// Consent: only a reject button may be clicked. The accept list is the safety rule (the same for
// every model); the reject list is only for the no-model probe.
var (
	reAccept = regexp.MustCompile(`(?i)\b(accept|agree|akzeptier|zustimm|einverstanden|allow all|alle zulassen|accepter|accetta|aceptar)`)
	reReject = regexp.MustCompile(`(?i)^(reject all|alle ablehnen)$`)
)
