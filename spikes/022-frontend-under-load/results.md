# SPIKE-022 results: frontend under load

Test machine and setup:

- **Hardware:** Windows 11 Pro 26200, i5-1235U (10 cores / 12 threads), 16 GB, Iris Xe. The display runs at 60 Hz (idle rAF delta p50 16.7 ms). DPR 1.25, window 1280×800.
- **Runtime:** WebView2 / Edge 153.
- **Stack:** Wails v3.0.0-beta.26, Go 1.26.0 (CGO off), a production bundle embedded in the binary (15 MB exe). React 19.3, Zustand 5.0.15, TanStack Table 9.2.4, @tanstack/react-virtual 3.14, Recharts 3.10.1, react-markdown 10.1 (+ remark-gfm, rehype-highlight), marked 18 (+ highlight.js core, DOMPurify), uPlot 1.6.32 for comparison. Vite 8, TypeScript 7, bun 1.3.2.

How the numbers were taken:

- **Runs:** 3 runs per scenario, each in a fresh app process. Every cell below is **median (max)** over the runs. The raw data is in `out/summary.md` and `out/*.json`.
- **Background load:** other agents were running on the machine throughout. System CPU during the measured phases had a mean of **41–91 %** (per-run values are in `cpu.*` in each JSON). Treat absolute timings as pessimistic.
- **Window state:** the window stayed visible in every run (`hiddenAfterVisible` = false in 105/105), so WebView2 did not throttle it. In 7 runs another window briefly took the foreground, and some of the real key presses were skipped.
- **Frame metric:** ">18 ms" means a missed 60 Hz frame. The literal ">16.7 ms" share is 15–40 % even when idle, because of vsync jitter (16.7–16.9 ms). Both values are in the JSON.
- **Long tasks:** the `longtask` API **did not report the streaming jank**. The work is split into many tasks under 50 ms, and the time goes to style and layout, not script. Long Animation Frames (LoAF) and the rAF deltas do show it, so read the LoAF and frame columns next to the long-task column.

## Q1 Streaming chat

The setup: 200 history messages (≈4,400 DOM nodes), then one reply of 2,015 tokens / 6,925 characters with headings, lists, a GFM table and 3–4 code blocks, sent as one Wails event per token. The window sticks to the bottom while streaming. Real `A` key presses (trusted input) go into the composer every 150 ms. A timer also updates the composer state every 120 ms.

What the labels mean:

- **Batching:**
  - `token`: one Zustand `setState` per event.
  - `sync`: `flushSync` per event.
  - `raf`: buffer the tokens and flush once per animation frame.
  - `t50`: flush at most every 50 ms.
- **Markdown renderers:**
  - `rm`: react-markdown re-parses the whole reply on each render.
  - `rmb`: react-markdown with finished blocks memoised, so only the last block re-parses.
  - `marked`: marked + hljs + DOMPurify, block-memoised.
  - `markedn`: marked, whole reply on each render.

### 100 tok/s (realistic)

| | token rm | sync rm | **raf rm** | token rmb | **raf rmb** | token marked | **raf marked** | raf markedn | t50 rm | t50 rmb |
|---|---|---|---|---|---|---|---|---|---|---|
| fps | 32 (34) | 29 (30) | 59.7 (59.9) | 60 (60) | **60 (60)** | 59 (60) | **60 (60)** | 49 (50) | 60 (60) | 60 (60) |
| frames >18 ms % | 26 (28) | 30 (53) | 0.7 (1.9) | 0.1 (0.2) | **0 (0.1)** | 0.5 (1.3) | **0 (0.3)** | 22 (24) | 0.2 (0.7) | 0 (0.2) |
| frames >33.4 ms % | 20 (22) | 24 (47) | 0.2 (0.4) | 0 (0) | **0 (0)** | 0.3 (0.8) | **0 (0)** | 3.9 (4.8) | 0.1 (0.2) | 0 (0) |
| frame max ms | 117 (133) | 117 (139) | 34 (50) | 19 (20) | 18 (20) | 117 (117) | 17 (29) | 50 (50) | 33 (34) | 18 (33) |
| long tasks >100 ms | 0 | 0 | 0 | 0 | **0** | 0 | **0** | 0 | 0 | 0 |
| LoAF max ms | 116 (122) | 125 (133) | 0 | 0 | **0** | 104 (105) | **0** | 0 | 53 (60) | 0 |
| React commits | 2015 | 2015 | 1159 | 2015 | 1208 | 2015 | 1200 | 902 | 275 | 312 |
| commit p50 / max ms | 8.8 / 27 | 9.1 / 34 | 9.3 / 28 | 1.9 / 9 | 3.3 / 8.5 | 1.9 / 7 | 2.4 / 6 | 10.9 / 31 | 12.9 / 33 | 3.6 / 16 |
| real key → paint p50 ms | 70 (106) | 240 (2819) | 18.5 (21) | 12.4 | **13.8 (15)** | 13.3 | **14.6 (18)** | 20 | 12.9 | 13.0 |
| real key → paint max ms | 2555 (2580) | 4303 (15385) | 36 (55) | 25 (39) | **27 (41)** | 162 (211) | **42 (44)** | 54 (57) | 44 (45) | 46 (46) |
| timer → paint max ms | 3518 | 6437 | 65 | 47 | 48 | 265 | 47 | 107 | 54 | 48 |
| emit → receive p50 / p99 ms | 55 / 2437 | 222 / 4185 | 4.5 / 25 | 0.7 / 6 | **0.9 / 9** | 7.6 / 412 | **1.1 / 15** | 10 / 37 | 1.7 / 33 | 1.0 / 11 |
| stream duration ms (ideal 20,150) | 22,720 | 24,494 | 20,185 | 20,181 | 20,168 | 20,215 | 20,186 | 20,212 | 20,219 | 20,184 |
| total private MB at end | 465 | 464 | 453 | 402 | 395 | 349 | **338** | 409 | 400 | 384 |

### 1,000 tok/s (stress)

| | token rm | sync rm | raf rm | token rmb | **raf rmb** | token marked | **raf marked** | raf markedn | t50 rm | t50 rmb |
|---|---|---|---|---|---|---|---|---|---|---|
| fps | 10.5 | 10.4 | 53 (53) | 14 | **59.7 (59.8)** | 16 | **59.5 (59.6)** | 53 (55) | 59.7 | 59.4 |
| frames >33.4 ms % | 86 | 88 | 4.0 (6.9) | 69 | **0 (0)** | 57 | **0 (0)** | 2.3 (4.1) | 0 | 0 |
| LoAF max ms | 121 | 123 | 0 (53) | 110 | **0** | 106 | **0** | 0 | 0 | 0 |
| key → paint max ms | 19,262 | 21,112 | 48 (60) | 4,062 | **44 (49)** | 2,699 | **32 (45)** | 44 (51) | 29 | 32 |
| emit → receive p99 ms | 18,983 | 20,843 | 33 | 4,087 | **23** | 2,720 | **8.4 (21)** | 35 | 18.5 | 12 |
| stream duration ms (ideal 2,015) | 21,344 | 23,280 | 2,082 | 6,150 | **2,053** | 4,773 | **2,052** | 2,069 | 2,067 | 2,052 |

- **Loss and ordering:** no tokens were missing, duplicated or out of order in any of the 132 chat runs, and the final text matched the source every time. When the page cannot keep up, Wails events queue; they are not dropped. The queue shows up as seconds of latency (per-token rendering at 1,000/s: 19 s p99).
- **History render:** 200 messages take 265–750 ms to reach the first paint, with a longest task of 67–154 ms. This is a one-off when a chat opens, and **it exceeds 100 ms**. The same run with 0 history messages behaves like the 200-message one while streaming. The history cost comes at load time; it does not add to the per-token cost.
- **Target "no long task > 100 ms at 100 tok/s": met** for every variant by the `longtask` API. By LoAF and rAF deltas, only the frame-coalesced variants meet it. Per-token `setState` produces 104–133 ms frames and 20–25 % dropped frames even at 100 tok/s.
- **React batching:** React 19 does not batch per-token updates at 100 tok/s: 2,015 commits for 2,015 events, one event per task.

**Recommendation:**

- **Batching:** buffer tokens outside React and flush once per animation frame with `flushSync` inside `requestAnimationFrame` (`raf`). A 50 ms throttle works about as well.
- **Markdown:** memoise finished Markdown blocks and re-parse only the last one.
  - **react-markdown + rehype-highlight with block memo (`rmb`)** holds 60 fps with no jank at 1,000 tok/s.
  - **marked + highlight.js + DOMPurify with block memo** is equally smooth, uses ~60 MB less memory and is 57 KB gzip smaller (see Q4). It needs explicit sanitising.
  - Either works. react-markdown is safe by default and keeps React elements (good for custom components such as links and code-copy buttons), so it is the default choice. marked is the fallback if bundle size or memory matters.
  - Re-parsing the whole reply on every flush (`raf rm`, `raf markedn`) is just acceptable at 100 tok/s but drops frames at 1,000 tok/s.

## Q2 Table: 10,000 × 12, 500-row pages, TanStack Table v9 + virtual rows

| metric | result | target |
|---|---|---|
| binding call, 500 rows (columns + array rows) | p50 10.1 ms (11.1), max 26 ms; **83 KB** JSON | – |
| Go side of that call | build 1.1 ms + `json.Marshal` 0.5 ms | – |
| same page as objects (`[]map[string]any`) | p50 14.0 ms, **130 KB** | – |
| same page via event (payload > 8 KB goes by reference + fetch) | p50 10.6 ms, p95 12.5 ms | – |
| append page → paint | p50 8.8 ms, max 16–43 ms | – |
| load all 20 pages sequentially | 432 ms (462) | – |
| scroll whole list (200 px per frame, ~27 s) | **59.9 fps (60.0)**, p99 16.9 ms, >18 ms 0.12 % (0.19), >33.4 ms 0 %, max 33 ms, 0 long tasks, no blank viewport | **60 fps: met** |
| sort 10k rows → next paint (number / text / date / datetime) | 32 / 39 / 38 / 41 ms (max 45) | – |
| global filter "a" / "ann" (4 text columns) | 7 / 17 ms | – |
| number-range filter / equals filter | 13 / 14 ms | – |
| sort + filter together | 32 ms; worst operation in a run: 56 ms (60) | – |
| memory at end | total private 633 MB, JS heap 111 MB | – |

Commit time is about 60 % of these paint times; for "sort amount", commit is 20 ms and paint 28 ms. Client-side sort and filter on 10k rows stay under 60 ms, which is one or two dropped frames and no long task.

## Q3 Chart: 10,000 points

| variant | points | first paint ms | longest task ms | fps in the first 3 s | resize → paint ms | hover → tooltip paint p50 / max ms | DOM nodes | private MB |
|---|---|---|---|---|---|---|---|---|
| Recharts line, defaults (animation + dots) | 10k | **1032 (1126)** | 1002 (1093) | 9 | 815 (1036) | 21 / 682 | 10,098 | 914 |
| Recharts line, `isAnimationActive={false}` `dot={false}` | 10k | **282 (371)** | 251 (329) | 54 | 43 (80) | 20 / 34 | 106 | 402 |
| + LTTB → 2,000 | 2k | **126 (132)** | 104 (107) | 57 | 23 (35) | 20 / 35 | 106 | 317 |
| + LTTB → 1,000 | 1k | 149 (173) | 123 (140) | 57 | 21 (35) | 20 / 34 | 106 | 319 |
| Recharts bar, defaults | 10k | **1556 (2155)** | 1502 (2098) | 1.7 | 3429 (4690) | 28 / 1409 | 30,148 | 1313 |
| Recharts bar, no animation | 10k | 1761 (1894) | 1684 (1797) | 25 | 2135 (2423) | 28 / 45 | 30,148 | 1013 |
| Recharts bar, mean per bucket → 1,000 | 1k | **333 (354)** | 303 (318) | 54 | 228 (294) | 21 / 37 | 3,148 | 407 |
| uPlot line (canvas) | 10k | **108 (108)** | 0 | 59 | 22 (52) | 2.1 / 19 | – | 294 |
| uPlot bars (canvas) | 10k | 89 (91) | 0 (LoAF 361) | 53 | 165 (198) | 1.9 / 19 | – | 302 |

- **LTTB cost:** 10k → 1–2k points takes 1.7–2.5 ms.
- **Data fetch:** the 10k-point binding call takes 12 ms and returns 229 KB.
- **Hover sweep** (one move per frame): 60 fps for every line variant, 48–54 fps for the 10k-bar variants.

**Target "first render < 1 s":** Recharts with default props **misses it** for both chart types (line 1.0–1.1 s, bar 1.6–2.2 s, resize up to 4.7 s). The recommended setup:

- **Settings:** always render Recharts with `isAnimationActive={false}` and `dot={false}`.
- **Downsampling:**
  - **Line and scatter:** apply **LTTB above 2,000 points**.
  - **Bars:** aggregate to **≤ 1,000 buckets**. 10k SVG bars stay over 1.5 s even without animation.
- **Result:** with those settings the first paint is 126–333 ms, resize is 21–230 ms, and hover takes one frame.
- **Remaining problem:** the 10k-row SPEC limit still produces a 100–300 ms task on mount.
- **uPlot:** meets every number without downsampling (≈100 ms, no long task, 22 KB gzip). It is the fallback if exact 10k-point rendering is wanted.

## Q4 Memory, start-up, bundle

**Memory.** Figures are private bytes summed over the 6 msedgewebview2 processes in our PID tree plus the Go process, at the end of each test. Working set is also in the JSON; it counts shared DLL pages once per process, which is why it is higher.

| after | WebView2 private MB | total private MB | total working set MB | JS heap MB |
|---|---|---|---|---|
| idle shell | 191 | **245 (248)** | 407 | 3 |
| chat, 200 history + streamed reply (best variants) | ~285–340 | 338–402 | 514–571 | 9–42 |
| chat, react-markdown re-parse per token | ~410 | 465 | 634 | 14–61 |
| table, 10k rows loaded + scrolled + sorted | ~580 | **633** | 809 | 111 |
| chart, Recharts line tuned / LTTB 2k | – | 402 / 317 | 570 / 486 | 76 / 21 |
| chart, Recharts bar 10k defaults | – | **1313** | 1479 | 374 |
| chart, uPlot 10k | – | 294–302 | 461–472 | 6–7 |

**Start-up** (idle scenario; the numbers are 3 runs each):

| | process start → first paint | process start → WebView2 navigation start | navigation → page visible | process start → Wails runtime ready |
|---|---|---|---|---|
| warm WebView2 profile | **831 ms (902)** | 643 ms | 185 ms | 858 ms |
| fresh profile (deleted before each run) | 758 ms (1151) | 592 ms (975) | 168 ms | 779 ms |

- **Where the time goes:** creating the WebView2 environment and controller takes ~600–700 ms of the ~0.8 s. The page itself (≈380 KB decoded JS + CSS from the embedded assets) paints within ~190 ms of navigation start.
- **FCP is not reported:** WebView2 starts the page before the window is visible, so "first paint" here is the first rAF after the page becomes visible.

**Bundle** (production build, gzip -9, each feature's static-import closure; every scenario and Markdown renderer is a dynamic import):

| part | raw KB | gzip KB |
|---|---|---|
| shell: React 19, Zustand, shadcn/ui (radix), Tailwind CSS, Wails glue | 289 | **88.9** |
| react-markdown + remark-gfm + rehype-highlight (lowlight "common", 37 languages) | 315 | **96.7** |
| marked + highlight.js core (7 languages) + DOMPurify | 115 | 39.7 |
| TanStack Table + Virtual | 81 | 23.7 |
| Recharts | 370 | **105.7** |
| uPlot | 51 | 22.2 |
| **app total: shell + chat (react-markdown) + table + Recharts** | 1062 | **317.9** |
| without Recharts | 692 | 212.3 |
| without markdown | 747 | 221.2 |
| without both | 377 | 115.5 |

Geist variable fonts add 76 KB of woff2 (not gzipped); the page uses only the latin subset, 29 KB. The runtime `/wails/runtime.js` is served by Go and is not part of the bundle. Recharts and react-markdown each roughly double the shell, so load both lazily.

## Q5 Wails side

- **Binding round trips:**

  | call | p50 ms | p95 ms | payload |
  |---|---|---|---|
  | `Noop` | 4.2 | 6.8 | – |
  | 1 KB echo | 3.8 | – | – |
  | 500 rows, array rows | **8.2** | 9.8 | 83 KB |
  | 500 rows, object rows | 14.0 | – | 130 KB |
  | 5,000 rows | 41 | – | 840 KB |
  | 200 history messages | 8.5 | – | 95 KB |
  | 10k chart points | 12 | – | 229 KB |

  - **Parallel calls:** 10 concurrent 500-row calls take 27 ms in total.
  - **Where the time goes:** of the ~8 ms for 500 rows, Go spends ~1.6 ms (build + marshal) and a fixed ~4 ms is round-trip overhead. The rest is transfer and `JSON.parse`.
  - **Payload shape:** column names + array rows cost 36 % less than objects.
- **Events** (small payload, JS handler only counts):

  | rate/s | received/s | missing | out of order | latency p50 / p99 ms | lag after last emit |
  |---|---|---|---|---|---|
  | 100 | 100 | 0 | 0 | 0.7 / 8 | 0.7 ms |
  | 1,000 | 1,001 | 0 | 0 | 0.6 / 2.0 (10) | 0.6 ms |
  | 5,000 | 5,000 | 0 | 0 | 0.7 / 2.8 | 1.7 ms |
  | 10,000 | 10,000 | 0 | 0 | 0.8 / 5.9 (13) | 0.8 ms (12) |
  | 20,000 | **13,800 (15,400)** | 0 | 0 | 442 / 889 | **0.9 s (1.2)** |
  | unthrottled (20k events) | 16,100 (16,700) | 0 | 0 | 591 / 1221 | 1.2 s |

  - **Sustained limit:** **~10k events/s** on this machine (under background load) before lag builds. The ceiling is ~14–16k/s: every event is one `ExecuteScript` on the UI thread.
  - **Ordering and loss:** ordering held and nothing was dropped at any rate. That matches the source: one ordered per-window queue and one drainer.
  - **`Emit` never blocks:** its maximum call time was ~1 ms even when unthrottled. `Emit` hands off to an unbounded mailbox, and the 64-deep window queue's backpressure only stalls that mailbox goroutine. A producer faster than the UI therefore grows memory and latency without bound instead of being slowed down.
- **GoStream** (`app.HandleStream` / `JSONStream`, new in beta.26, held-poll over the asset server):

  | rate/s | received/s | missing | out of order | latency p50 / p99 ms |
  |---|---|---|---|---|
  | 1,000 | 1,000 | 0 | 0 | 3.5 / 6.6 |
  | 20,000 | 20,000 | 0 | 0 | 4.1 / 9.7 |
  | unthrottled | **51k (56k)** | 0 | 0 | 7.3 / 14 |

  - **Trade-off:** latency starts higher than events (~4 ms vs ~0.6 ms, because it polls) but throughput is 3–5× higher. `Send` applies real backpressure (buffers bounded to 256 frames / 8 MB), and it avoids the eval path.
  - **When it matters:** for token streaming at LLM rates (≤ 1,000/s), events are fine and simpler. GoStream is worth keeping in mind for high-rate or bulk channels such as logs or run output.

## Surprises

- The `longtask` API showed **0 long tasks** in the worst streaming variants, while frames reached 117–133 ms and the key → paint delay reached 2.5–21 s. Style and layout of a growing message dominate, split across short tasks. Burrow's perf checks should use LoAF or frame deltas, not `longtask`.
- React 19 did not batch token updates arriving as separate Wails events. At 1,000/s, per-token rendering let events queue for up to 21 s. Nothing was lost, but the UI trailed the model by 20 s.
- The 10k-point Recharts default line paints its first frame in ~1 s, but resize takes up to 1 s and hover spikes to 0.7 s. 10k Recharts bars cost 1.3 GB private memory and 30k SVG nodes.
- The 500-row page via event was as fast as the binding (10.6 vs 10.1 ms), even though Wails sends it out-of-line (> 8 KB) through the payload store and a fetch.
- **13 of 105 first attempts exited within 0.1–3 s** with exit status 1 and no stderr, before any page code ran. They clustered around consecutive runs; the likely cause is the WebView2 profile folder still held by the previous run's exiting processes (not confirmed). The runner now retries such starts (`-missing` refilled all 13, each on the first retry) and routes Wails' logger and error handler to `out/<name>.log`.

## Not tested

- Dev mode (Vite dev server and React dev build).
- A virtualised chat history, and `content-visibility: auto` on history messages. These would be the next step for the 265–750 ms history render.
- Several windows, or several chats streaming at once.
- High-DPI or 120 Hz displays.
- ECharts.
- A real LLM token stream with bursty chunking.
- Rendering the Markdown in a Web Worker.
- Server-side (SQL) sort for the table. Only client-side sort was measured, as asked.
- An idle machine: all numbers were taken with 40–90 % background CPU from other agents.
- GoStream reconnect and page-reload behaviour.
