package main

import (
	"fmt"
	"slices"
	"strings"
)

// Targets from Gate 1 (N-01, N-02, N-07, N-52, N-54).
const (
	targetFPS        = 58   // 60 fps, with vsync jitter
	targetKeyPaintMs = 50   // key → paint while streaming
	targetBlockMs    = 100  // opening a 200-message chat
	targetStartMs    = 1500 // cold start to first paint
	targetIdleMB     = 350  // idle memory, private
	targetAppIdleMB  = 100  // the app's own process, idle
	targetOpenMB     = 500  // with the 200-message chat open (proposed)
	targetLoadMB     = 1500 // every limit in use (proposed)
)

func mark(ok bool) string {
	if ok {
		return "pass"
	}
	return "**fail**"
}

func (rep *report) markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Bench: %s\n\nThe built app `%s`, a fake model at %.0f tokens a second (answers of %d tokens), a history of kind %s. Took %.0f s.\n\n",
		rep.Started.Local().Format("2006-01-02 15:04"), rep.App, rep.Rate, rep.Reply, rep.History, rep.Took)

	b.WriteString("## Summary\n\n| Requirement | Measured | Target | Result |\n|---|---|---|---|\n")
	s := rep.Stream
	fmt.Fprintf(&b, "| N-01 streaming | %.1f fps; key → paint p50 %.0f ms, max %.0f ms | 60 fps; under %d ms | %s |\n",
		s.FPS, s.KeyPaintP50Ms, s.KeyPaintMaxMs, targetKeyPaintMs, mark(s.FPS >= targetFPS && s.KeyPaintMaxMs < targetKeyPaintMs && s.Keys > 0))
	// As SPIKE-022 and P1-15: input waits for the longest task.
	var task, frame float64
	for _, o := range rep.Opens {
		task, frame = max(task, o.LongTaskMaxMs), max(frame, o.LoafMaxMs)
	}
	fmt.Fprintf(&b, "| N-02 opening the chat | longest task %.0f ms (longest frame %.0f ms) | %d ms | %s |\n", task, frame, targetBlockMs, mark(len(rep.Opens) > 0 && task <= targetBlockMs))
	var starts []int64
	late := false
	for _, st := range rep.Starts {
		starts = append(starts, st.FirstPaintMs)
		late = late || st.Late
	}
	slices.Sort(starts)
	if len(starts) > 0 {
		note := ""
		if late {
			note = " (a probe came late, so this is an upper bound)"
		}
		fmt.Fprintf(&b, "| N-07 start to first paint | median %d ms, max %d ms%s | %d ms | %s |\n", starts[len(starts)/2], starts[len(starts)-1], note, targetStartMs, mark(starts[len(starts)-1] < targetStartMs))
	}
	fmt.Fprintf(&b, "| N-52 idle memory | %.0f MB private, the app %.0f MB | %d MB, the app %d MB | %s |\n", rep.Idle.PrivateMB, rep.Idle.AppPrivateMB,
		targetIdleMB, targetAppIdleMB, mark(rep.Idle.PrivateMB < targetIdleMB && rep.Idle.AppPrivateMB < targetAppIdleMB))
	fmt.Fprintf(&b, "| N-52 with the chat open | %.0f MB private | %d MB (proposed) | %s |\n", rep.AfterOpens.PrivateMB, targetOpenMB, mark(rep.AfterOpens.PrivateMB < targetOpenMB))
	fmt.Fprintf(&b, "| N-54 memory under load | peak %.0f MB private, %d answers at once | %d MB (proposed) | %s |\n",
		rep.Load.Peak.PrivateMB, rep.Load.Streams, targetLoadMB, mark(rep.Load.Peak.PrivateMB < targetLoadMB))

	b.WriteString("\n## Start-up\n\nFrom the process start to the shell's first paint. Starts where WebView2 opened no DevTools port are not counted; the first start with a new profile is always one of them.\n\n| Start | Navigation ms | First paint ms |\n|---|---|---|\n")
	for i, st := range rep.Starts {
		late := ""
		if st.Late {
			late = " (late)"
		}
		fmt.Fprintf(&b, "| %d | %d | %d%s |\n", i+1, st.NavigationMs, st.FirstPaintMs, late)
	}

	b.WriteString("\n## Opening the 200-message chat\n\n| Open | Rows | First rows ms | All rows ms | Long tasks | Longest task ms | LoAFs | Longest LoAF ms | LoAF blocking ms |\n|---|---|---|---|---|---|---|---|---|\n")
	for i, o := range rep.Opens {
		fmt.Fprintf(&b, "| %d | %d | %.0f | %.0f | %d | %.0f | %d | %.0f | %.0f |\n", i+1, o.Rows, o.FirstRowsMs, o.AllRowsMs, o.LongTasks, o.LongTaskMaxMs, o.Loafs, o.LoafMaxMs, o.LoafMaxBlockingMs)
	}
	d := rep.OpenDOM
	fmt.Fprintf(&b, "\nWith the chat open, after garbage collection: %d elements in the page, %d rows (%.0f elements a row). The renderer counts %d DOM nodes, %d documents and %d event listeners.\n",
		d.Elements, d.Rows, float64(d.Elements)/float64(max(1, d.Rows)), d.Nodes, d.Documents, d.Listeners)

	fmt.Fprintf(&b, "\n## Streaming\n\n%d frames in %.1f s: %.1f fps, p95 %.1f ms, max %.1f ms; %.1f %% over 18 ms, %.1f %% over 33 ms. %d keys: key → paint p50 %.1f, p95 %.1f, max %.1f ms. Long tasks %d (longest %.0f ms), LoAFs %d (longest %.0f ms).\n",
		s.Frames, s.Seconds, s.FPS, s.FrameP95Ms, s.FrameMaxMs, s.Over18MsPct, s.Over33MsPct, s.Keys, s.KeyPaintP50Ms, s.KeyPaintP95Ms, s.KeyPaintMaxMs, s.LongTasks, s.LongTaskMaxMs, s.Loafs, s.LoafMaxMs)

	b.WriteString("\n## Memory\n\nPrivate bytes summed over the app and its WebView2 processes, as in SPIKE-022; the working set counts shared pages once per process.\n\n| When | Private MB | App MB | WebView2 MB | Working set MB | JS heap MB | Processes |\n|---|---|---|---|---|---|---|\n")
	row := func(name string, m memory) {
		fmt.Fprintf(&b, "| %s | %.0f | %.0f | %.0f | %.0f | %.0f | %d |\n", name, m.PrivateMB, m.AppPrivateMB, m.WebviewPrivateMB, m.WorkingSetMB, m.JSHeapMB, m.Processes)
	}
	rows := []struct {
		name string
		m    memory
	}{
		{"idle", rep.Idle},
		{"after opening the 200-message chat", rep.AfterOpens},
		{"after streaming in it", rep.AfterStream},
		{fmt.Sprintf("peak, %d answers streaming", rep.Load.Streams), rep.Load.Peak},
		{"after the load", rep.Load.After},
		{"10 s after leaving for an empty chat", rep.Settle.Left},
		{"after garbage collection", rep.Settle.Collected},
		{"after a memory-pressure signal", rep.Settle.Pressure},
	}
	for _, r := range rows {
		row(r.name, r.m)
	}

	// WebView2 by process type.
	var types []string
	for _, r := range rows {
		for t := range r.m.ByTypeMB {
			if !slices.Contains(types, t) {
				types = append(types, t)
			}
		}
	}
	slices.Sort(types)
	b.WriteString("\n### WebView2 by process type, private MB\n\n| When |")
	for _, t := range types {
		fmt.Fprintf(&b, " %s |", t)
	}
	b.WriteString("\n|---|" + strings.Repeat("---|", len(types)) + "\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "| %s |", r.name)
		for _, t := range types {
			fmt.Fprintf(&b, " %.0f |", r.m.ByTypeMB[t])
		}
		b.WriteString("\n")
	}
	if c := rep.Settle.Cycles; len(c) > 0 {
		fmt.Fprintf(&b, "\nThen the 200-message chat opened and left %d times, with garbage collection after each: private MB", len(c))
		for i, v := range c {
			sep := ","
			if i == 0 {
				sep = ""
			}
			fmt.Fprintf(&b, "%s %.0f", sep, v)
		}
		b.WriteString(". Memory that keeps rising here is a leak.\n")
	}
	rep.dumpTables(&b)
	return b.String()
}

// dumpTables writes the memory-infra dumps: per process, the allocators
// of 1 MB or more, largest first. A partition's size is part of
// partition_alloc's.
func (rep *report) dumpTables(b *strings.Builder) {
	if len(rep.Dumps) == 0 {
		return
	}
	var procs []string
	for _, d := range rep.Dumps {
		for p := range d.Dump {
			if !slices.Contains(procs, p) {
				procs = append(procs, p)
			}
		}
	}
	slices.Sort(procs)
	b.WriteString("\n## Memory by allocator\n\nFrom Chromium's memory-infra dumps, effective MB.\n")
	for _, p := range procs {
		peak := map[string]float64{}
		for _, d := range rep.Dumps {
			for name, v := range d.Dump[p] {
				peak[name] = max(peak[name], v)
			}
		}
		var names []string
		for name, v := range peak {
			if v >= 1 {
				names = append(names, name)
			}
		}
		if len(names) == 0 {
			continue
		}
		slices.SortFunc(names, func(x, y string) int { return int(peak[y]*100 - peak[x]*100) })
		fmt.Fprintf(b, "\n### %s\n\n| Allocator |", p)
		for _, d := range rep.Dumps {
			fmt.Fprintf(b, " %s |", d.When)
		}
		b.WriteString("\n|---|" + strings.Repeat("---|", len(rep.Dumps)) + "\n")
		for _, name := range names {
			fmt.Fprintf(b, "| %s |", name)
			for _, d := range rep.Dumps {
				fmt.Fprintf(b, " %.1f |", d.Dump[p][name])
			}
			b.WriteString("\n")
		}
	}
}
