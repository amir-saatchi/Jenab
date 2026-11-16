# TASK-061 proof report: Phase 2 acceptance on the native app's code

Part 1 of [TASK-061](../TASK-061-remove-wails.md), scope item 1 ("Proof first"). Branch `claude/task-061-proof-af1da9`, from main at `ae309c1`. Runs on 2026-11-16 (UTC), Windows 11, i5-1235U.

**Status:** the acceptance runs pass on both models the user picked, and the Go test passes. The native release is built and starts on a scratch profile. **The manual run of the native app is waiting for the user** (steps below). Nothing was removed or renamed; items 2 to 4 wait for the manual run.

## What changed since P2-14

P2-14 ran on 2026-11-14 and 15. Since then, main got:
- TASK-048: the runner stops a model when its quota is gone.
- TASK-050 to 052: the findings fixes, including the `query` tool's text (TASK-051).
- TASK-053 to 059 and TASK-062: the native screens, the native updater and installer, `build:native`, and a lifecycle lock fix.

The services didn't change for TASK-061, so the scenario runs check that nothing regressed in them. The native UI is checked by the manual run.

## The acceptance runs

`scenarios/phase2-acceptance`, `P2_schema_memory`, `-reps 3`, as P2-14 ran it:

```bash
jenab-scenarios run -models <scratch>/models.yaml -env <main checkout>/.env -model <provider/id> -reps 3 -usage-log <scratch>/usage-<model>.jsonl scenarios/phase2-acceptance
```

- The main checkout's `scenarios/models.yaml` lists neither model, so a scratch models file outside the repo named them: `anthropic` (kind `anthropic`, `ANTHROPIC_API_KEY`, `paid: true`) with `claude-haiku-5-5` at 1,000,000, and `gemini` (kind `gemini`, `GEMINI_API_KEY`) with `gemini-3.5-flash-lite` at 1,048,576. No keys in it; nothing committed.
- Same set, prompts and settings for both models.

| Model | Passed | Requests a run (chat + schema agent) | Time a run | Prompt tokens a run | Cost |
|---|---|---|---|---|---|
| anthropic/claude-haiku-5-5 | **3/3** | 14+2, 14+2, 14+2 | 28.5 s, 26.4 s, 27.5 s | 91,941 (83 % cache reads) | **$0.015** for the 3 reps |
| gemini/gemini-3.5-flash-lite | **3/3** | 14+2, 13+2, 16+2 | 82 s, 39 s, 149 s | 67,407 (28 % cache reads) | free |

P2-14 for comparison: Haiku 3/3, 16–17 requests, about 30 s, $0.017; flash-lite 3/3, 21–24 requests, 71–110 s.

### Haiku 5.5

- **3/3**, `results/20261116-205547`: 14+2 requests a run (16 to the provider), 28.5 s, 26.4 s and 27.5 s.
- **Cost:** from the usage log, 48 requests: input 174, cache read 252,478, cache write 52,404, output 12,496 tokens. At $0.10 / $0.01 / $0.125 / $0.50 a million (as P2-14), that is **$0.015** for the 3 reps, under the $0.10 cap. 83 % of prompt tokens were cache reads (P2-14: 78 %). The runner reported 12 thinking blocks dropped and 2 cache breaks; no request failed.
- **Turn 1:** each run made one table (`btc_daily_closes`, `btc_daily_prices`, `btc_daily_close`) and saved the goal to project memory.
- **Turn 3:** each run asked on the migration card ("Apply a schema change that deletes data?"), approved once, and kept the 3 rows in `prices`.
- **Turns 4–6:** prices from `query`, the goal from memory, no history tool.
- **Turn 5:** every run still called `describe_table` for `prices`, which the card lists. **No run tried `sqlite_master`** (P2-14: 7 of 9 runs on the three models did). That fits TASK-051's new `query` text.
- **Undo:** in every run, `Undo turn 7: done, 3 rows in prices`, with the notice `[turn 4 was undone: 3 rows in prices]`. Turn 8's rows and the Bitcoin rows stayed. Turn 9 named only 2026-11-13 = 3,251.
- Info, not a failure: in r1, turn 9's answer also named the undone prices, saying they were removed (the check `answer_lacks` is info).

### Flash-lite

- **3/3**, `results/20261116-205749`, on `GEMINI_API_KEY`: 49 requests in all, no quota or rate-limit error, so the reserve key wasn't needed. This work used 49 of the key's 500 a day.
- r3 took 149 s, 112 s of it waiting on requests (the slowest 19.5 s); it also wrote session notes in turn 3. That is the provider's speed, not a failure.
- **Turn 1:** each run made one table (`bitcoin_prices`, `daily_prices` twice) with one `apply_migration` each, and saved the goal to project memory. r3 saved it as a two-item list; the goal check still passed.
- **Turn 3:** each run asked on the migration card ("Apply a schema change that changes a primary key?"), approved once, and kept the 3 rows.
- **Turns 4–6:** prices from `query`, the goal from memory, no history tool.
- **Turn 5:** r2 and r3 answered from the card with no tool call; r1 called `describe_table`. **No run tried `sqlite_master`** (P2-14: every flash-lite rep did).
- **Undo:** in every run, `Undo turn 7: done, 3 rows in prices`, with the notice `[turn 4 was undone: 3 rows in prices]`; turn 8's and the Bitcoin rows stayed, and turn 9 named only 2026-11-13 = 3,251.

### Differences from P2-14

- **`sqlite_master`:** 0 of 6 runs tried it (P2-14: 7 of 9). TASK-051's new `query` text fits this.
- **`describe_table` in turn 5:** Haiku still called it in every run. Flash-lite skipped it in 2 of 3. So SPEC 3.2's "a wasted request" is now Haiku's habit only, here.
- **Fewer requests:** Haiku 16 a run (P2-14: 16–17); flash-lite 15–18 (P2-14: 21–24), mostly from the dropped `sqlite_master` and `describe_table` calls.
- **Haiku's migration card:** "deletes data" in all 3 runs (it made `prices` new, copied the 3 rows and dropped the old table). P2-14's live runs all showed "changes a primary key" (a widen); its manual run with Haiku showed "deletes data", as here. Both ask, both kept every row.
- Nothing regressed: every real check passed in all 6 runs.

## Go test

`go test ./internal/scenario -run TestPhase2Acceptance -count=1`: **pass** (0.5 s), on main's code at `ae309c1`.

## The native release

- `wails3 task build:native` (the Taskfile runs through `wails3 task`; `task` itself isn't installed). MyGo 0.9.1, 27 s. NSIS 3.13 was already in `%LOCALAPPDATA%\mygo` from TASK-059, so **nothing was downloaded**.
- Output, git-ignored, in this worktree:
  - `D:\DEV\Burrow\.claude\worktrees\task-061-proof-af1da9\bin\native\windows-amd64\Jenab.exe` (39.3 MB; version info: Jenab 0.1.0-dev, publisher Jenab; SHA-256 `0F11DE16BFEBA8B7EE8D6A79CB9D6A7FB457DDAB96C4FC78330CA51624515DF3`)
  - `D:\DEV\Burrow\.claude\worktrees\task-061-proof-af1da9\bin\native\windows-amd64\jenab-setup-0.1.0-dev-amd64.exe` (10.2 MB; SHA-256 `9D8D7FA84A19D6F79E4D7C9E0EB4FFC0BB3BB1CF36788E6ABC9626F994988D0F`)
  - The worktree goes when its session is archived. Then build it again from main with `wails3 task build:native`; it lands in `bin\native\windows-amd64` of that checkout.
- **Smoke check (bench build):** `go build -tags bench ./cmd/desktop-native` from the same commit, started with `LOCALAPPDATA`, `APPDATA` and `USERPROFILE` in a scratch folder and `JENAB_BENCH_PORT` set. `GET /state` answered at once with `loaded: true`, no projects, the window shown and focused, and the updater `unavailable` (no update key in `mygo.json`, as TASK-059 says). A screenshot through `/shot` showed the welcome screen ("Welcome to Jenab", *Create project*). `POST /quit` closed it. Its data (`registry.db`, `logs`, `window-state.json`) went into the scratch folder only. No system dialog opened.
- The release exe itself was not started here: the user's manual run does that.

## Manual run in the native app (for the user)

This follows P2-14's "Built app" run, in the native app. Use Haiku 5.5 and your own data folder. Take the three screenshots P2-14 asked for: the memory row, the migration card and *Undo turn*. Keep them with you if they show the data folder's path.

1. Install with `jenab-setup-0.1.0-dev-amd64.exe` (per user, Start menu entry "Jenab"), or start `Jenab.exe` straight from `bin\native\windows-amd64`. If the Wails app is open, quit it first.
2. *Settings › Models*: if Anthropic isn't there, *Add provider* (Anthropic) with your key, and pick `claude-haiku-5-5`.
3. *Create project* "Bitcoin tracker". In its chat, send:
   > New project. I want to track the daily Bitcoin closing price in USD, which I'll type in by hand for now. My goal: find out whether buying a fixed 50 USD every Monday beats buying only after a 5 % drop. Make the table I need, and keep that goal in mind for every chat in this project.

   Check: the schema change row (the table made) and the memory row (brain icon, the goal saved to project memory). **Screenshot 1: the memory row.** P2-14's chip had *Undo* and *Edit*; note what the native row offers.
4. Send: `Here are the first closes: 2026-11-10 91,250; 2026-11-11 93,480; 2026-11-12 92,115. Please add them.` Check: the turn's footer offers *Undo turn*.
5. Send:
   > Also track Ethereum. Move the prices into one general table prices(coin, date, price), keeping every Bitcoin row (coin 'BTC'), and drop the old table once nothing needs it.

   A migration card asks ("Apply a schema change that deletes data?"), with the steps. **Screenshot 2: the migration card.** Press *Apply*.
6. Quit the app completely (*Quit Jenab* in the rail, not just closing the window), then start it again.
7. Open the project, start a *New chat*, and send these one at a time:
   - `What was the Bitcoin close on 2026-11-12, and which day had the highest Bitcoin close so far?` Expect 92,115, and 93,480 on 2026-11-11, from `query`.
   - `Which tables does this project have, and what is the key of the prices table?` Expect `prices`, keyed `(coin, date)`.
   - `Remind me, what is the goal of this project?` Expect the goal, from memory.

   Check: no history tool (`search_history`, `read_messages`, `list_chats`) shows in these turns.
8. Send these one at a time (not as one message):
   - `Add the first Ethereum closes: 2026-11-10 3,140; 2026-11-11 3,205; 2026-11-12 3,188.`
   - `And 2026-11-13: Bitcoin 94,020, Ethereum 3,251.`
9. On the first of those two turns, press *Undo turn* in its footer. **Screenshot 3: the footer before pressing, or the notice after.** Check: it takes out 3 rows in `prices`, and the next turn shows the notice "[turn 4 was undone: 3 rows in prices]".
10. Send `Which Ethereum days are in the table now, with their closes?` Expect only 2026-11-13 at 3,251. Bitcoin keeps its 4 days.

Pass: every check above holds. Anything that looks wrong or missing in the native UI is a finding. Haiku spent $0.005 a run in the scenario, so the manual run costs about a cent.

## For items 2 to 4 (not done here)

- Removing Wails removes `wails3 task`, which runs the Taskfile today (`Docs/DEVELOPMENT.md` says "No separate Task install is needed"). `build:native` and the other tasks then need another runner, such as Task as a `go tool`, or the docs give `go tool mygo build` directly.

## Notes

- Only the two models the user picked, on `ANTHROPIC_API_KEY` and `GEMINI_API_KEY`. `GEMINI_API_KEY_III` wasn't needed. No other model or key.
- Synthetic data only: the scenario's made-up prices.
- No key values were read or printed. No raw provider errors are pasted here.
- The results folders (`scenarios/phase2-acceptance/results/`) are git-ignored and not committed.
- The PC was kept awake during the runs.
