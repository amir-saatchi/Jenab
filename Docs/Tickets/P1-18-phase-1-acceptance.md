# P1-18 — Phase 1 acceptance
**Type:** Task
**Status:** In progress
**Gate:** 3 (Phase 1)
**Needs:** P1-15, P1-16, TASK-001
**Requirements:** N-01, N-02, N-07, N-09, N-52, N-54

## Goal
Prove Phase 1's "done when" (PROPOSAL 9): a user can hold a normal tool-using chat that survives restarts, and the agent finds something said 20 turns earlier through history search.

## Scope
- **Scenario:** a TASK-001 scenario of 25 turns with tool calls and a restart after turn 12. At turn 25 it asks about something from turn 3, and asserts `tool_called: search_history` and `answer_contains`.
- **Restart test:** the same restart as a Go integration test with `provider/fake`, so it also runs in CI.
- **Performance:** the Phase 1 numbers (N-01, N-02, N-07, N-09, N-52), measured on the built app on Windows, and on macOS and Linux when available.
- **Memory under load (N-54):** the app's memory with 8 background LLM calls streaming from `provider/fake` and two busy chats. Pipelines and scripts come later, so the full test, with 4 runs each holding a script near its 256 MB cap, is added to Gate 4 with them. The results confirm or change the 4 GB minimum.
- **Gate 4:** the Phase 1 acceptance tests in Gate 4 are written from this ticket.

## Done when
The scenario passes on two development models, the restart test passes in CI, and the measured numbers are recorded here.

## Progress
- **Scenario:** `scenarios/acceptance`, `P1_restart_history`. 25 turns on the Bitcoin project: queries, web search, the bucket, a pipeline config and plain questions. The app restarts before turn 13, which adds up the list from turn 12. Turn 25 asks what the user paid for their first Bitcoin on turn 3, and on which exchange.
  - The runner got two additions: `restart: true` on a message closes the app (orchestrator, projects, registry) and opens it again from disk, and an assert's `turn` can be a turn number.
  - 2026-10-09, 1 rep, both passed:

    | Model | Turn 13 | Turn 25 | Prompt tokens | Time |
    |---|---|---|---|---|
    | gemini-3.5-flash-lite | queried the sum: 301,500 | `search_history`, then `read_messages`; 48,200 and Bitstamp | 216,744 | 196 s |
    | gemma4:31b | from the window: 301,500 | 4 `search_history` calls (one with a bad `scope`, fixed after the error); 48,200 and Bitstamp | 183,440 | 110 s |
- **Restart test:** `TestAcceptanceRestart` (`internal/scenario`) runs the same set on a scripted model in `go test`. It checks that the restart cut the window (turn 13 sees turns 10–12, not 7), that turn 3 is out of the window at turn 25, and that the answer comes from the search on the reopened project. With the restart switched off, the test fails. CI is blocked by billing, so it runs locally for now.
- **N-09:** `TestSearchSpeed` (`JENAB_PERF_TEST=1`), 100,000 messages: p95 9.7 ms for the project and 11.8 ms for one chat (target 50 ms). Search inside words, which the user turns on, has p95 322 ms.
- **Bench:** `wails3 task bench` builds the app with the `bench` tag and runs `cmd/jenab-bench` on it (DEVELOPMENT.md). The bench gives the app its own user folder with a project of 10 chats, one of them with 200 messages, and a fake model on 127.0.0.1 that streams 100 tokens a second. It drives the app through WebView2's DevTools port with SPIKE-022's measures. The shipped build has no port and no bench code.
  - 2026-10-10, Windows 11, two runs:

    | Requirement | Run 1 | Run 2 | Target |
    |---|---|---|---|
    | N-01 streaming | 59.9 fps; key → paint p50 22 ms, max 37 ms | 59.9 fps; p50 21 ms, max 42 ms | 60 fps; under 50 ms |
    | N-02 opening the 200-message chat | longest task 74 ms (frame 104 ms) | 82 ms (frame 113 ms) | 100 ms |
    | N-07 start to first paint | median 953 ms | median 900 ms | 1.5 s |
    | N-52 idle | 303 MB | 278 MB | 300 MB |
    | N-54 10 answers streaming at once | peak 563 MB | peak 555 MB | 1.5 GB |

  - **N-02:** only the first open has a long task; opens 2–5 have none, and their first rows paint in 55–85 ms. The chat loads its last 30 turns (60 rows), so a longer history doesn't cost more.
  - **N-07** counts from the process start to the shell's first frame, with the WebView2 profile already made. The first start with a new profile opens no DevTools port, so it can't be measured this way.
  - **N-52** is on the line: 278–310 MB over five runs, about 60 MB the app and the rest WebView2's 7 processes. SPIKE-022 measured 245 MB on a smaller page.
  - **Memory after use:** the growth is the open 200-message chat, not streaming, and it comes back when the chat closes. The bench splits WebView2 by process type and, after the load, switches to an empty chat, collects garbage and sends a memory-pressure signal (2026-10-10):

    | When | Total MB | Renderer | GPU | Other WebView2 | App |
    |---|---|---|---|---|---|
    | idle | 290 | 48 | 121 | 63 | 59 |
    | the 200-message chat open | 460 | 183 | 153 | 63 | 61 |
    | after streaming in it | 481 | 218 | 138 | 64 | 62 |
    | 10 answers streaming | 565 | 281 | 153 | 64 | 68 |
    | 10 s after switching to an empty chat | 384 | 114 | 142 | 62 | 66 |
    | after garbage collection | 331 | 89 | 114 | 63 | 66 |

    The open chat costs about 170 MB, mostly in the renderer, though the JS heap is only 12 MB: it is the page (60 rows of Markdown and code), not data. 41 MB stays after closing it, in the renderer. The memory-pressure signal frees nothing more.
  - **N-54** uses 10 chats answering at once in place of 8 background calls and 2 busy chats; Phase 1 has no background calls.
- **Open:**
  - N-52's target: 300 MB, or raise it.
  - An open 200-message chat costs about 170 MB in WebView2's renderer: find what holds it (likely the rendered Markdown and code), and whether a target for an open chat is needed.
  - The restart test in CI, once the billing lock is lifted.
  - macOS and Linux numbers, when available.
