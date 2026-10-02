# SPIKE-026 — How many commands can run at once?
**Type:** Spike
**Status:** Done on Windows (2 and 3 cold builds at once not measured, see Result; light commands still to measure, see Open)
**Gate:** 3

## Question
`run_command` (SPEC 8.5, Phase 6) will run builds and tests on the user's machine. Unlike LLM calls, they use the machine itself. How many can run at once, and what keeps the machine and the app usable?

## Setup
- **Machine:** a normal work laptop in daily use: Intel i5-1235U (10 cores, 12 threads), 16 GB of memory, NVMe SSD, Windows 11. WSL, two IDEs, several chat apps and Defender were running, as they would for a user.
- **Harness:** `spikes/026-command-limits/`. It starts K copies of a command at once. Each copy runs in its own Job Object inside one shared job, so the jobs see the whole process tree: peak committed memory and CPU time per command and for all K. While it runs, a normal-priority thread sleeps 5 ms in a loop and records how late it wakes, as a stand-in for the app's UI thread.
- **Commands:**
  - `go-cold`: `go build -a` of this app (std, Wails and ours), like a first build in a fresh checkout. Its own `GOCACHE`, so the user's cache isn't touched.
  - `go-test`: `go test -count=1 ./internal/...`, warm cache.
  - `vite`: the mockups' production build (84 screens, Tailwind, React).
  - `tsc`: a full type check of the mockups.
- **Safety stop:** if the memory Windows can still hand out (commit headroom) falls below 1.5 GB, the level is killed, so the user's apps never run out.

## Result
Run on 2026-10-01. Data in `spikes/026-command-limits/results/`.

1. **One command at a time:**

   | Command | Time | CPU time | Peak memory | Processes |
   |---|---|---|---|---|
   | `go-cold` | 94 s | 92 s | 1,045 MB | 344 |
   | `go-test` | 16 s | 30 s | 927 MB | 194 |
   | `vite` | 24 s | 10 s | 962 MB | 6 |
   | `tsc` | 18 s | 22 s | 406 MB | 3 |

2. **Memory runs out, not CPU.**
   - Two and three type checks at once took 17 s and 16 s, the same as one: 12 threads had room.
   - Memory didn't. Commit headroom was 4.3 GB before the run and 2.3 GB after it, with none of our commands running, because the user's own apps grew. From the third level on, two commands together (about 1.1–1.2 GB) took it below 1.5 GB and the safety stop fired. So 2 and 3 cold builds at once were not measured to the end, and weren't needed: one heavy command can take half of what a busy 16 GB laptop has left.
3. **Priority:** commands at below-normal priority kept the app thread on time.

   | | Late, p99 | Late, max |
   |---|---|---|
   | Normal priority (12 levels) | 4–20 ms | 9–133 ms |
   | Below normal (2 levels) | 1.3–3 ms | 3–16 ms |

   A 133 ms wake-up is a visible stutter in the UI; 16 ms is one frame. Levels the safety stop killed were short, so these are small samples, but every normal level had a higher p99 than both below-normal ones.

## Decision (proposed, SPEC 8.5)
Memory is the limit, and it differs a lot between commands: a build takes about 1 GB, `git pull` almost nothing. A low count set by the heaviest case would block the light ones, so the agent decides with real numbers, and the app guards the machine.
- **The agent states the need:** `run_command(cmd, needs_memory_mb)`, default 256. The tool's description gives rough sizes.
- **The app checks at the start:** if too little is free, it checks again after 5, 15 and 30 s, with no LLM calls, and the chat shows the wait with *Run now* and *Cancel*. After the third check the agent gets `low_memory` with the needed, free and total memory and the time waited. It can ask the user, lower the need, or wait longer with `wait_for_memory_s` (at most 600).
- **Every result has the numbers:** `peak_memory_mb`, `free_memory_mb`, `total_memory_mb`, so the agent learns what its commands use.
- **Hard stop:** below 500 MB free while commands run, the newest command using at least 256 MB is stopped with its process tree, and the agent gets `out_of_memory`. The agent can't change this rule.
- **Count limits as a safety net:** `max_parallel_commands: 100` across the app and `max_commands_per_chat: 20`, editable, and stated in the tool's description.
- **Priority:** commands run at below-normal priority, set on their Job Object, so the app stays smooth during builds.
- **N-54:** builds need about 1 GB each on top of the minimum system.

## Open
- macOS and Linux: the same check with their memory pressure signals, when those machines are available.
- Repeat 2 and 3 cold builds at once on a machine with more headroom, to see how much parallel builds slow each other.
- Light and long-running commands (`git`, `docker`, `bun install`, a dev server): check the 256 MB default, the 500 MB stop and whether 100 commands at once are safe. Before Phase 6.
