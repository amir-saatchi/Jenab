# TASK-005 — What an open chat holds in memory
**Type:** Task
**Status:** Done
**Gate:** 3
**Requirements:** N-52

## Goal
Find what an open chat holds in WebView2's renderer, cut it if that is cheap, and propose a target for it.

## Why
P1-18's bench found that opening the 200-message chat adds about 170 MB: the renderer grows from 48 to 183 MB and the GPU process by about 30 MB. The JS heap stays near 12 MB, so it is the page, not the data. The chat shows only its last 30 turns (60 rows). SPIKE-022 rendered all 200 messages in 245 MB in total. Most of it comes back when the chat closes; 41 MB stays.

## Scope
- **Measure by content.** The bench seeds the same 30 turns in a few kinds: plain prose, Markdown without code, and the current mix with code blocks and tables. For each it measures the renderer's private bytes, the JS heap, and the DOM counters (nodes, documents, listeners). This needs no change to the app.
- **Look at the parts:** how many DOM nodes a row makes, whether rows off screen are fully rendered, and what highlight.js adds to a code block.
- **Fix if cheap,** for example `content-visibility` for rows off screen, or highlighting code only when it is on screen. Each fix is measured with the bench, and streaming (N-01) and opening (N-02) must not get worse.
- **Propose a target** for an open chat, for Gate 1.

## Done when
The cost of an open chat is explained by part, cheap fixes are in and measured, and a target is proposed.

## Result
The open chat costs 120–170 MB, grows with the number of elements, and comes back when the chat closes, except for about 40 MB that the renderer keeps for reuse. It is not a leak. No cheap fix is worth taking. Proposed target: under 500 MB with the 200-message chat open (in N-52).

- **By content.** The bench's new `-history` flag seeds the 30 turns as short answers, prose, the mix, or code. The renderer grows with the elements, and code costs no more than the mix, so highlight.js is not the cost:

  | Kind | Elements | Renderer grows by | Renderer after GC |
  |---|---|---|---|
  | short | 834 | 46 MB | 82 MB |
  | prose | 1,123 | 72 MB | 83 MB |
  | mixed | 2,000 | 114 MB | 81 MB |
  | code | 2,353 | 101 MB | 83 MB |

  What stays after closing is the same for every kind, so it is not the content.
- **By part.** Chromium's memory dumps (the bench's `-dumps` flag), renderer, idle → chat open:
  - painted tiles (`cc/tile_memory`) 8 → 31 MB
  - malloc 18 → 33 MB
  - V8 6 → 11 MB, Oilpan (`blink_gc`) 3 → 6 MB, Blink objects 2 → 3 MB
  - the GPU process grows 20–30 MB, mostly malloc

  The named parts cover about 50 MB of the renderer's growth; the dumps don't name the rest.
- **After closing.** About 40 MB stays in the renderer: the tiles (32 MB), Oilpan's pages (28 MB after the load, as Oilpan keeps freed pages) and some malloc. Opening and leaving the chat ten times, with GC after each, holds memory flat at 379–389 MB, so the memory is reused, not leaked.
- **Fixes tried.**
  - Rows off screen already use `content-visibility: auto`.
  - Without the scroll fade at the bottom of the chat, tiles drop from 29 to 19 MB, but the whole chat only by about 8 MB, inside the noise. The fade stays.
  - Highlighting code only on screen: not tried, since code costs no more than the mix.
- **The bench** now also counts DOM nodes with the chat open, runs the ten open-and-close cycles, and has a row for the open chat against 500 MB. With `-dumps`, a tracing process of about 10 MB joins the count; numbers for Gate 1 come from runs without it.
