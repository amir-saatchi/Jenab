package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unsafe"
)

// hang: the test editor blocks its UI thread for 40 s. The same snapshot is asked through the CLI
// (8 s limit, then the process is killed) and through the DLL (the caller gives up after 8 s, but
// the call itself cannot be stopped; we time when it really returns).

func runHang(cfg *Config) error {
	cli := cfg.CLI
	res := map[string]string{}
	app, err := launchEditor(cli, "SPIKE-039 Hang Test", "-freeze-after", "5s", "-freeze-for", "40s")
	if err != nil {
		return err
	}
	defer app.Close()
	dll, err := LoadDLL(cfg.DLLPath, cli.Home)
	if err != nil {
		return err
	}
	l, err := dll.ListWindowsExact("spike039")
	if err != nil {
		return err
	}
	defer l.Close()
	var w ExactWindow
	for _, x := range l.Wins {
		if x.ID == app.Win.ID {
			w = x
		}
	}
	if w.ptr == nil {
		return errors.New("dll did not list the hang editor")
	}
	var clearRef string
	if s, _, took, err := cli.snapshot(app.Win.ID, true); err == nil {
		res["before freeze: cli snapshot"] = took.Round(time.Millisecond).String()
		clearRef = findRef(s.Tree, "button", "Clear")
	}
	dsnap, _ := dll.Snapshot("spike039", true, 10) // refs for the DLL's click below
	var denv Envelope
	json.Unmarshal([]byte(dsnap), &denv)
	dclear := ""
	if d, err := parseSnap(denv.Data); err == nil {
		dclear = findRef(d.Tree, "button", "Clear")
	}
	time.Sleep(7 * time.Second) // frozen from 5 s to 45 s
	frozeAt := time.Now()

	// Through the CLI, with the timeout of its own observation budget and ours.
	s, _, took, err := cli.snapshot(app.Win.ID, true, "--timeout-ms", "3000")
	switch {
	case errors.Is(err, ErrTimeout):
		res["cli snapshot (3 s budget, 30 s kill)"] = "killed after " + took.Round(time.Millisecond).String()
	case err != nil:
		res["cli snapshot (3 s budget, 30 s kill)"] = fmt.Sprintf("error after %s: %v", took.Round(time.Millisecond), err)
	default:
		all, refs := countNodes(s.Tree)
		res["cli snapshot (3 s budget, 30 s kill)"] = fmt.Sprintf("returned after %s, complete=%v, %d nodes, %d refs", took.Round(time.Millisecond), s.Complete, all, refs)
	}
	short := &CLI{Bin: cli.Bin, Home: cli.Home}
	_, _, took, err = short.Run(4*time.Second, "click", "@sbogus0000:e1")
	res["cli click on a bogus ref while frozen (4 s kill)"] = fmt.Sprintf("%s: %v", took.Round(time.Millisecond), err)
	_, _, took, err = short.Run(2*time.Second, "snapshot", "--window-id", app.Win.ID, "-i", "--timeout-ms", "20000")
	res["cli snapshot, 20 s budget, our 2 s kill"] = fmt.Sprintf("%s: %v", took.Round(time.Millisecond), err)

	// A click on a real button of the frozen window, with our 10 s kill.
	_, _, took, err = cli.Run(10*time.Second, "click", clearRef)
	res["cli click Clear while frozen (10 s kill)"] = fmt.Sprintf("%s: %v", took.Round(time.Millisecond), err)

	// Through the DLL: the caller gives up after 8 s; the call goes on until it returns.
	dllCall := func(label string, f func() string) {
		start := time.Now()
		returned := make(chan string, 1)
		err := dll.doTimeout(8*time.Second, func() { returned <- f() })
		if errors.Is(err, ErrTimeout) {
			res[label] = "caller gave up after 8 s; the DLL thread stays blocked"
			select {
			case r := <-returned:
				res[label+": really returned"] = time.Since(start).Round(time.Millisecond).String() + " " + r
			case <-time.After(45 * time.Second):
				res[label+": really returned"] = "not within 53 s"
			}
			return
		}
		r := <-returned
		res[label] = "returned after " + time.Since(start).Round(time.Millisecond).String() + " " + r
	}
	dllCall("dll ad_get_tree_exact", func() string {
		opts := adTreeOptions{MaxDepth: 10, InteractiveOnly: true}
		var t adNodeTree
		r := dll.call("ad_get_tree_exact", dll.adapter, uintptr(unsafe.Pointer(w.ptr)), uintptr(unsafe.Pointer(&opts)), uintptr(unsafe.Pointer(&t)))
		n := t.Count
		dll.call("ad_free_tree", uintptr(unsafe.Pointer(&t)))
		return fmt.Sprintf("code %d, %d nodes, %s", int32(r), n, dll.lastError())
	})
	dllCall("dll ad_execute_by_ref click Clear", func() string {
		act := adAction{Kind: 0}
		var out uintptr
		r := dll.call("ad_execute_by_ref_timeout", dll.adapter, uintptr(unsafe.Pointer(cstr(dclear))),
			uintptr(unsafe.Pointer(&act)), 0, 0, uintptr(unsafe.Pointer(&out)))
		return fmt.Sprintf("code %d %s", int32(r), clip(dll.takeString(out), 160))
	})
	res["time from the freeze to the end"] = time.Since(frozeAt).Round(time.Second).String()
	app.Close()

	// Edge at a cold start (fresh profile): the first reads, through both ways, pinned to the
	// spike's own Edge window.
	edge, err := launchEdge(cli, cfg.Scratch)
	if err != nil {
		return err
	}
	defer edge.Close()
	t0 := time.Now()
	for i := 0; i < 6; i++ {
		s, _, took, err := (&CLI{Bin: cli.Bin, Home: cli.Home}).snapshot(edge.Win.ID, true, "--max-depth", "45", "--timeout-ms", "30000")
		key := fmt.Sprintf("edge cold cli snapshot %d (at +%s)", i+1, time.Since(t0).Round(100*time.Millisecond))
		if err != nil {
			res[key] = fmt.Sprintf("%s: %v", took.Round(time.Millisecond), err)
			continue
		}
		all, refs := countNodes(s.Tree)
		page := 0
		if d := pageDoc(s.Tree); d != nil {
			page, _ = countNodes(*d)
		}
		res[key] = fmt.Sprintf("%s, complete=%v, %d nodes, %d refs, page %d nodes", took.Round(time.Millisecond), s.Complete, all, refs, page)
	}
	if l, err := dll.ListWindowsExact("msedge"); err == nil {
		for _, x := range l.Wins {
			if x.ID == edge.Win.ID {
				dllCall("edge dll ad_get_tree_exact (warm)", func() string {
					opts := adTreeOptions{MaxDepth: 45, InteractiveOnly: true}
					var t adNodeTree
					r := dll.call("ad_get_tree_exact", dll.adapter, uintptr(unsafe.Pointer(x.ptr)), uintptr(unsafe.Pointer(&opts)), uintptr(unsafe.Pointer(&t)))
					n := t.Count
					dll.call("ad_free_tree", uintptr(unsafe.Pointer(&t)))
					return fmt.Sprintf("code %d, %d nodes", int32(r), n)
				})
			}
		}
		l.Close()
	}
	for k, v := range res {
		fmt.Printf("%-48s %s\n", k, v)
	}
	return writeJSON(cfg.Out+"/hang.json", res)
}
