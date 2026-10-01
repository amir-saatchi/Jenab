# SPIKE-022: frontend under load

Throwaway test app for SPIKE-022. It is a Wails v3 (v3.0.0-beta.26) window with a
React + TypeScript frontend built by Vite through bun. The stack: shadcn/ui,
Zustand 5.0.15, TanStack Table 9.2.4 with `@tanstack/react-virtual`, Recharts 3.10.1,
react-markdown and marked, plus uPlot as a chart comparison. Each run starts one app
process that runs one scenario on its own, writes `out/<scenario>-rN.json` and exits.
Results are in [results.md](results.md).

```bash
go run ./cmd/runner            # bun install/build, go build, all scenarios x 3 runs (~35 min), out/summary.md
```

Other ways to run it:

```bash
go run ./cmd/runner -only '^chat-100' -runs 1    # a subset
go run ./cmd/runner -summary                     # re-aggregate out/*.json only
bin/spike022.exe -q "s=chat&rate=100&mode=raf&md=rmb" -manual   # one scenario; the window stays open
```

Keep the window visible and don't use the machine during a run. The chat scenario
sends real `A` key presses, and only while its own window is in front.

## Layout

- `main.go`: the Wails app and the `Spike` service. The page calls it by name
  (`main.Spike.X`), so no bindings generator is needed. It covers:
  - token and flood events, with Go-side pacing and time spent in `Emit`
  - row pages and chart data
  - key presses (only when our window is in front)
  - `Report`, which writes the result file with memory and CPU data
- `stream.go`: a GoStream (`HandleStream`) flood, used to compare with events.
- `data.go`: deterministic test data: a ~2,000-token Markdown reply, 200
  history messages, 10k x 12 rows and 10k chart points.
- `win.go`: memory of the WebView2 process tree (PID tree below our process), CPU
  sampling (system and our process tree), foreground check and `keybd_event`.
- `frontend/src/scenarios/`: `idle`, `chat` (+ `md-rm`, `md-marked`), `table`,
  `chart`, `wails`. `lib/metrics.ts` holds rAF frame deltas, `longtask`,
  `long-animation-frame`, Event Timing and after-paint timing.
- `cmd/runner`: builds, measures the bundle (gzip -9 over each chunk's
  static-import closure), runs the scenarios and aggregates median (max).

The page loads the Wails runtime from `/wails/runtime.js`, which the Go asset
server serves. That keeps it out of the bundle and always at the Go version.
The WebView2 profile goes to `out/wv-data`, and to `out/wv-fresh` for the
fresh-profile start-up runs (it is deleted before each of those runs). Both are
throwaway and git-ignored with the rest of `out/`. Starts that fail within
seconds are retried, and `-missing` re-runs only the runs that have no result.
