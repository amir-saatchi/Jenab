package main

import (
	"fmt"
	"strings"
	"time"
)

// runCalcProbe explains a lost lock seen in an agent run: Calculator is started cold, "Four" is
// clicked, and the frame window's existence, its owner process and list-windows are watched.
// No model calls.
func runCalcProbe(cfg *Config) error {
	for rep := 1; rep <= 2; rep++ {
		app, err := launchCalculator(cfg.CLI)
		if err != nil {
			return err
		}
		h := hwndOf(app.Win.ID)
		start := time.Now()
		look := func(what string) {
			ws, _ := cfg.CLI.ListWindows("CalculatorApp")
			var ids []string
			for _, w := range ws {
				ids = append(ids, fmt.Sprintf("%s pid=%d inst=%s", w.ID, w.PID, w.ProcessInstance))
			}
			fmt.Printf("rep %d %-14s +%5.1fs  %s alive=%v owner=%d  list=[%s]\n", rep, what, time.Since(start).Seconds(),
				app.Win.ID, isWindow(h), windowPID(h), strings.Join(ids, "; "))
		}
		look("launched")
		snap, _, _, err := cfg.CLI.snapshot(app.Win.ID, true)
		if err != nil {
			fmt.Println("  snapshot:", err)
		} else if ref := findRef(snap.Tree, "button", "Four"); ref != "" {
			look("snapshot")
			_, _, took, err := cfg.CLI.Run(15*time.Second, "click", ref)
			fmt.Printf("  click Four: %v (%.0f ms)\n", err, took.Seconds()*1000)
		}
		for _, d := range []time.Duration{0, time.Second, 3 * time.Second, 10 * time.Second, 20 * time.Second, 30 * time.Second} {
			time.Sleep(d)
			look("after click")
		}
		app.Close()
		time.Sleep(2 * time.Second)
	}
	return nil
}
