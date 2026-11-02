package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// bench times the two ways to run agent-desktop on the same windows, and checks which window
// each one reaches when two windows share one app name.

type BenchResult struct {
	Timings    map[string]Stats  `json:"timings"`
	Notes      map[string]string `json:"notes"`
	Foreground string            `json:"foreground"`
	Targeting  map[string]string `json:"targeting"`
}

func findRef(n SnapNode, role, name string) string {
	if n.Role == role && n.Name == name && n.Ref != "" {
		return n.Ref
	}
	for _, c := range n.Children {
		if r := findRef(c, role, name); r != "" {
			return r
		}
	}
	return ""
}

func findNode(n SnapNode, pred func(SnapNode) bool) *SnapNode {
	if pred(n) {
		return &n
	}
	for _, c := range n.Children {
		if r := findNode(c, pred); r != nil {
			return r
		}
	}
	return nil
}

func (c *CLI) snapshot(winID string, interactive bool, extra ...string) (*SnapData, []byte, time.Duration, error) {
	args := []string{"snapshot", "--window-id", winID}
	if interactive {
		args = append(args, "-i")
	}
	args = append(args, extra...)
	env, raw, took, err := c.Run(30*time.Second, args...)
	if err != nil {
		return nil, raw, took, err
	}
	s, err := parseSnap(env.Data)
	return s, raw, took, err
}

func runBench(cfg *Config) error {
	cli := cfg.CLI
	res := BenchResult{Timings: map[string]Stats{}, Notes: map[string]string{}, Targeting: map[string]string{}}
	rec := func(name string, ds []time.Duration) {
		res.Timings[name] = stats(ds)
		fmt.Printf("%-34s %s\n", name, res.Timings[name])
	}
	fg0 := foregroundWindow()

	var ds []time.Duration
	for range 20 {
		_, _, took, err := cli.Run(10*time.Second, "version")
		if err != nil {
			return err
		}
		ds = append(ds, took)
	}
	rec("cli version (process start)", ds)

	calc, err := launchCalculator(cli)
	if err != nil {
		return err
	}
	defer calc.Close()
	time.Sleep(2 * time.Second)

	ds = nil
	for range 10 {
		_, _, took, err := cli.Run(10*time.Second, "list-windows", "--app", "CalculatorApp")
		if err != nil {
			return err
		}
		ds = append(ds, took)
	}
	rec("cli list-windows --app", ds)

	ds = nil
	var snap *SnapData
	for range 10 {
		s, _, took, err := cli.snapshot(calc.Win.ID, true)
		if err != nil {
			return err
		}
		snap, ds = s, append(ds, took)
	}
	rec("cli snapshot calculator -i", ds)

	ds = nil
	var fgSteals int
	for range 10 {
		ref := findRef(snap.Tree, "button", "Seven")
		fg := foregroundWindow()
		_, _, took, err := cli.Run(10*time.Second, "click", ref)
		if err != nil {
			return fmt.Errorf("cli click: %w", err)
		}
		if foregroundWindow() != fg {
			fgSteals++
		}
		ds = append(ds, took)
	}
	rec("cli click (headless)", ds)
	res.Notes["cli click foreground changes"] = fmt.Sprint(fgSteals, " of 10")
	if s, _, _, err := cli.snapshot(calc.Win.ID, true); err == nil {
		if d := findNode(s.Tree, func(n SnapNode) bool { return strings.HasPrefix(n.Name, "Display is") }); d != nil {
			res.Notes["calculator display after 10 cli clicks on Seven"] = d.Name
		}
		cli.Run(10*time.Second, "click", findRef(s.Tree, "button", "Clear"))
	}

	// The library, loaded into this process.
	t0 := time.Now()
	dll, err := LoadDLL(cfg.DLLPath, cli.Home)
	if err != nil {
		return fmt.Errorf("dll: %w", err)
	}
	res.Notes["dll load + ad_init + adapter"] = time.Since(t0).Round(time.Millisecond).String()
	fmt.Println("dll load:", res.Notes["dll load + ad_init + adapter"])

	ds = nil
	var calcWin ExactWindow
	for range 10 {
		t := time.Now()
		l, err := dll.ListWindowsExact("CalculatorApp")
		if err != nil {
			return err
		}
		ds = append(ds, time.Since(t))
		for _, w := range l.Wins {
			if w.ID == calc.Win.ID {
				calcWin = w
			}
		}
		if calcWin.ptr == nil {
			l.Close()
			return fmt.Errorf("dll did not list the calculator window")
		}
		defer l.Close()
	}
	rec("dll ad_list_windows_exact", ds)

	ds = nil
	var dllSnap string
	for range 10 {
		t := time.Now()
		s, r := dll.Snapshot("CalculatorApp", true, 10)
		if r != 0 {
			return fmt.Errorf("ad_snapshot: %d %s", r, clip(s, 300))
		}
		ds, dllSnap = append(ds, time.Since(t)), s
	}
	rec("dll ad_snapshot calculator -i", ds)

	ds = nil
	var nodes []TreeNode
	for range 10 {
		t := time.Now()
		nodes, err = dll.TreeExact(calcWin, true, 10)
		if err != nil {
			return err
		}
		ds = append(ds, time.Since(t))
	}
	rec("dll ad_get_tree_exact -i", ds)
	res.Notes["dll tree nodes"] = fmt.Sprint(len(nodes))

	var env Envelope
	json.Unmarshal([]byte(dllSnap), &env)
	ds2, _ := parseSnap(env.Data)
	ds = nil
	for range 10 {
		ref := findRef(ds2.Tree, "button", "Seven")
		t := time.Now()
		out, r := dll.ClickRef(ref)
		if r != 0 {
			return fmt.Errorf("ad_execute_by_ref: %d %s", r, clip(out, 300))
		}
		ds = append(ds, time.Since(t))
	}
	rec("dll ad_execute_by_ref click", ds)

	ds = nil
	for range 10 {
		t := time.Now()
		if _, err := dll.ClickExact(calcWin, "button", "Seven"); err != nil {
			return err
		}
		ds = append(ds, time.Since(t))
	}
	rec("dll find_exact + action (pinned)", ds)
	if s, _, _, err := cli.snapshot(calc.Win.ID, true); err == nil {
		if d := findNode(s.Tree, func(n SnapNode) bool { return strings.HasPrefix(n.Name, "Display is") }); d != nil {
			res.Notes["calculator display after 20 dll clicks on Seven"] = d.Name
		}
	}

	// Two windows of one app (two editors, both spike039.exe): which one does each path reach?
	a, err := launchEditor(cli, "SPIKE-039 Editor A")
	if err != nil {
		return err
	}
	defer a.Close()
	b, err := launchEditor(cli, "SPIKE-039 Editor B")
	if err != nil {
		return err
	}
	defer b.Close()
	time.Sleep(time.Second)
	if s, _, _, err := cli.snapshot(a.Win.ID, true); err == nil {
		res.Targeting["cli --window-id A"] = s.Window.Title
	} else {
		res.Targeting["cli --window-id A"] = err.Error()
	}
	if s, r := dll.Snapshot("spike039", true, 10); r == 0 {
		json.Unmarshal([]byte(s), &env)
		d, _ := parseSnap(env.Data)
		res.Targeting["dll ad_snapshot(app) with A and B open, B in front"] = d.Window.Title
	} else {
		res.Targeting["dll ad_snapshot(app)"] = fmt.Sprint(r, " ", clip(s, 200))
	}
	if l, err := dll.ListWindowsExact("spike039"); err == nil {
		for _, w := range l.Wins {
			if w.ID == a.Win.ID {
				if n, err := dll.TreeExact(w, true, 10); err == nil && len(n) > 0 {
					res.Targeting["dll ad_get_tree_exact(A)"] = fmt.Sprintf("%s, %d nodes", n[0].Name, len(n))
				}
				if _, err := dll.ClickExact(w, "button", "Upper case"); err != nil {
					res.Targeting["dll ClickExact(A, Upper case)"] = err.Error()
				} else {
					res.Targeting["dll ClickExact(A, Upper case)"] = "ok"
				}
			}
		}
		l.Close()
	}
	for k, v := range res.Targeting {
		fmt.Printf("%-50s %s\n", k, v)
	}
	if foregroundWindow() == fg0 {
		res.Foreground = "unchanged by the headless calls"
	} else {
		res.Foreground = "changed (the launches bring windows to the front)"
	}
	return writeJSON(cfg.Out+"/bench.json", res)
}
