package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// runProbe shows what agent-desktop's find returns, on the test editor and Calculator (no model).
func runProbe(cfg *Config) error {
	if os.Getenv("PROBE") == "prep" {
		return runPrep(cfg, os.Getenv("PROFILE"), os.Getenv("ENGLISH") != "")
	}
	if os.Getenv("PROBE") == "chrome" {
		return runChromeProbe(cfg, strings.Split(os.Getenv("LANGS"), ";"), os.Getenv("PROFILE"))
	}
	ed, err := launchEditor(cfg.CLI, "SPIKE-040 Test Editor")
	if err != nil {
		return err
	}
	defer ed.Close()
	time.Sleep(time.Second)
	for _, args := range [][]string{
		{"find", "--window-id", ed.Win.ID, "--role", "textfield"},
		{"find", "--window-id", ed.Win.ID, "--role", "button", "--name", "Clear", "--first"},
		{"find", "--window-id", ed.Win.ID, "--text", "characters"},
	} {
		_, raw, took, err := cfg.CLI.Run(15*time.Second, args...)
		fmt.Printf("%v (%v, %.0f ms):\n%s\n\n", args[1:], err, took.Seconds()*1000, clip(string(raw), 1500))
	}
	snap, _, _, err := cfg.CLI.snapshot(ed.Win.ID, true)
	if err == nil {
		fmt.Println(compact(snap, 1).Text)
	}
	calc, err := launchCalculator(cfg.CLI)
	if err != nil {
		return err
	}
	defer calc.Close()
	time.Sleep(1500 * time.Millisecond)
	for _, args := range [][]string{
		{"find", "--window-id", calc.Win.ID, "--role", "button", "--name", "Four", "--exact"},
		{"find", "--window-id", calc.Win.ID, "--role", "button", "--name", "One"},
		{"find", "--window-id", calc.Win.ID, "--text", "Display is"},
	} {
		_, raw, took, err := cfg.CLI.Run(15*time.Second, args...)
		fmt.Printf("%v (%v, %.0f ms):\n%s\n\n", args[1:], err, took.Seconds()*1000, clip(string(raw), 1500))
	}
	return nil
}

func envInt(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil {
		return v
	}
	return def
}

// runPrep readies a Chrome profile without a model: one Google search, consent declined with
// "Alle ablehnen"/"Reject all", and with english, the results page's "Change to English" link.
// Later runs in that profile get no consent dialog (and, with english, an English page).
func runPrep(cfg *Config, profile string, english bool) error {
	app, err := launchChrome(cfg.CLI, ChromeOpts{Profile: profile})
	if err != nil {
		return err
	}
	defer app.Close()
	time.Sleep(2 * time.Second)
	s := &Session{cli: cfg.CLI, Win: app.Win, owner: windowPID(hwndOf(app.Win.ID))}
	omni, err := s.findOne("textfield", "Address and search bar", 10*time.Second)
	if omni == nil {
		return fmt.Errorf("no address bar (%v)", err)
	}
	cfg.CLI.Run(15*time.Second, "set-value", omni.Ref, "weather")
	time.Sleep(time.Second)
	cfg.CLI.Run(15*time.Second, "press", "return", "--window-id", app.Win.ID)
	if b, _ := s.findOne("button", "Alle ablehnen", 20*time.Second); b != nil {
		_, _, _, err := cfg.CLI.Run(15*time.Second, "click", b.Ref)
		fmt.Println("clicked Alle ablehnen:", err)
	} else if b, _ := s.findOne("button", "Reject all", 3*time.Second); b != nil {
		_, _, _, err := cfg.CLI.Run(15*time.Second, "click", b.Ref)
		fmt.Println("clicked Reject all:", err)
	}
	if english {
		if l, _ := s.findOne("link", "Change to English", 20*time.Second); l != nil {
			_, _, _, err := cfg.CLI.Run(15*time.Second, "click", l.Ref)
			fmt.Println("clicked Change to English:", err)
			time.Sleep(4 * time.Second)
		} else {
			fmt.Println("no Change to English link")
		}
	}
	time.Sleep(3 * time.Second)
	o, path := s.pageOrigin()
	fmt.Println("page:", o, clip(path, 80), "title:", app.Win.Title)
	return nil
}
