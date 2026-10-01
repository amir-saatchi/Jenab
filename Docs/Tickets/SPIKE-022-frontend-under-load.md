# SPIKE-022 — Frontend under load
**Type:** Spike
**Status:** Done
**Gate:** 2

## Question
Does the chosen frontend stack stay smooth inside Wails v3's webview (WebView2) with realistic Jenab data? The stack is React + TypeScript, shadcn/ui, Zustand, TanStack Table and Recharts, built with Bun.

1. **Streaming chat:** tokens sent from Go through Wails events at about 100 per second (and a 1,000-per-second stress run), rendered as Markdown with code blocks, in a chat of 200 messages. Frame times, long tasks, input delay while streaming.
2. **Table:** 10,000 rows × 12 columns with TanStack Table and virtual scrolling. Sorting, filtering and scrolling, loaded in 500-row pages as in SPEC 5.3.
3. **Chart:** 10,000 points (the SPEC 5.4 limit) as a line chart and a bar chart in Recharts. Render time and hover responsiveness. If it is too slow, what are the options?
4. **Memory and start-up:** webview memory after each test, cold start to first paint, bundle size.
5. **Wails side:** event throughput and ordering, and bindings for page loads (Go → JS with 500 rows).

## Done when
Each point has measured numbers against a target: 60 fps scrolling, no long task over 100 ms while streaming, a chart under 1 s. There is a decision on any library or limit changes (for example the Markdown renderer, or the chart point limit).

## Result
Run on 2026-09-28 on the i5-1235U laptop (Iris Xe, 60 Hz):

- **Build:** a production build in Wails v3 beta.26 (WebView2 153).
- **Runs:** every scenario ran 3 times in a fresh process; 105/105 runs have results. Numbers are median (max).
- **Load:** other spikes loaded the CPU to 41–91 % throughout, so the timings are pessimistic.
- **Where:** code and data are in `spikes/022-frontend-under-load/` (`results.md`, `out/summary.md`).

**The targets hold only with the right settings.** With default settings, streaming stutters and Recharts misses 1 s.

| Test | Result | Target |
|---|---|---|
| **Streaming, 100 tokens/s**: one `setState` per token | 32 fps, 20 % of frames over 33 ms, key → paint up to 2.6 s | missed |
| Streaming, 100 tokens/s: tokens buffered and flushed once per frame, finished Markdown blocks memoised | 60 fps, 0 % over 33 ms, key → paint 14 ms (max 41 ms) | met |
| Streaming, 1,000 tokens/s: same settings | 59.7 fps, 0 % over 33 ms, key → paint max 44 ms | met |
| Opening a chat with 200 messages | 265–750 ms, one task of 67–154 ms | over 100 ms (one-off) |
| **Table**, 10,000 × 12, virtual rows: scroll | 59.9 fps, 0 long tasks | met |
| Table: sort / filter 10,000 rows in the browser | 32–41 ms / 7–17 ms | – |
| Table: 500-row page from Go (binding) | 8–10 ms, 83 KB as column names + array rows (130 KB as objects) | – |
| **Chart**, 10,000 points, Recharts defaults | line 1.03 s (1.13), bar 1.56 s (2.16), resize up to 4.7 s, 1.3 GB for bars | missed |
| Chart: Recharts without animation and dots | line 282 ms; bar still 1.76 s | line met |
| Chart: line downsampled (LTTB) to 2,000 points / bars aggregated to 1,000 | 126 ms / 333 ms; hover in one frame | met |
| Chart: uPlot (canvas), full 10,000 points, for comparison | about 100 ms, no long tasks | met |
| **Start-up** to first paint | 0.83 s (0.90); 0.6–0.7 s of it is WebView2 setup | – |
| Memory (private, WebView2 + Go) | idle 245 MB; chat 338–402 MB; table 633 MB; tuned chart 317–402 MB | – |
| Bundle, gzip | 318 KB in total: shell 89, react-markdown + highlighting 97, Recharts 106, table 24 | – |
| **Wails events** | up to about 10,000/s with 0.8 ms latency; above that they queue (never dropped or reordered) | – |
| Wails binding call | 4 ms empty, 8 ms for 500 rows, 41 ms for 5,000 rows | – |

Findings:
- **No drops or reordering:**
  - No token was lost, duplicated or out of order in any run.
  - When the page falls behind, events queue. At 1,000/s with per-token rendering, the UI fell 20 s behind the model.
- **Wails `Emit` never blocks:** its buffer is unbounded, so a producer that is too fast grows memory and latency instead of slowing down. GoStream (new in beta.26) has real backpressure and about 51,000 frames/s. It fits high-rate channels such as logs; events are enough for tokens.
- **React 19 doesn't batch the updates:** 2,015 token events gave 2,015 renders.
- **The browser's `longtask` API missed all the streaming jank:** the time goes to style and layout, split into short tasks. Frame times and Long Animation Frames show it.
- **Markdown renderers:** with block memoisation, react-markdown and marked + DOMPurify are equally smooth. marked uses about 60 MB less memory and 57 KB less bundle, but it needs explicit sanitising.
- **Failed starts:** 13 of 105 first attempts exited at start-up with no output. The likely cause is the WebView2 profile still being held by the previous run. All succeeded on retry.

## Decision
**Decided (2026-09-28):**
1. **Streaming chat:**
   - Buffer tokens outside React and render once per animation frame.
   - Parse only the last Markdown block, and memoise the finished ones.
   - Keep react-markdown + rehype-highlight: it's safe by default and keeps React components for links and code-copy buttons. marked + DOMPurify is the fallback if memory or bundle size matters.
   - Tokens go through Wails events.
2. **Chat history:** opening a long chat must not block. Render the history virtualised, or with `content-visibility: auto`. To be measured in Phase 1.
3. **Charts (SPEC 5.4):**
   - Keep Recharts.
   - Up to 1,000 points it keeps its normal animation and dots. Above 1,000 points, animation and dots are off.
   - The runtime decides from the data size; it isn't a view-config field. (The spike turned animation and dots off together, and didn't measure small charts; the Phase 1 perf tests confirm the 1,000 line.)
   - Line charts over 2,000 points are downsampled with LTTB in Go before sending.
   - Bar charts show at most 1,000 bars; above that they show a notice to aggregate in the query, like the 10,000-row notice.
   - The 10,000-row query limit stays. Scatter wasn't measured, so it gets the same 2,000-point rule until it is.
   - uPlot is the fallback if exact 10,000-point charts are needed later.
4. **Tables (SPEC 5.3):**
   - Pages go from Go as column names plus array rows, not objects.
   - Virtual rows.
   - SQL pagination and sorting stay as in the SPEC; the browser could also sort 10,000 rows (under 60 ms).
5. **Loading:** Recharts and the Markdown renderer load lazily. The shell without them is 89 KB gzip.
6. **Wails:**
   - Events for tokens and progress.
   - GoStream is kept in mind for logs and run output.
   - Since `Emit` never blocks, Jenab's own sender coalesces tokens: at most one event per ~16 ms per stream.
7. **Perf checks** in CI and the Phase 1 tests use frame times and Long Animation Frames, not the `longtask` API.

Doc changes (made in SPEC v0.5): SPEC 5.3, 5.4, the new 5.8 (frontend rules), the changelog, and the PROPOSAL Frontend row.
