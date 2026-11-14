# TASK-047 report: a steady request start

**Branch:** `claude/elastic-heyrovsky-bb6707`. Commits: d380d01 (break counting), 0717ae6 (steady start), 5706bab (table tools as before), and this report.
**Models:** `anthropic/claude-haiku-5-5` (paid) and `nvidia/mistralai/mistral-large-4-1t-a52b` (free reference model).
**Binaries:** "before" is d380d01 (TASK-046) with the new break counting added (section 1, item 6). "After" is 5706bab. Same sets and settings, one run of each.

## 1. What changed

All of it is in `internal/agent` and is the same for every model. Nothing in the provider layer changed.

1. **The system blocks stay the same between cuts.** Blocks 1–5 are built at a cut and not touched until the next one. Before, a migration rebuilt block 4 (the card) mid-window, and a new day rebuilt block 1 at the next cut.
2. **Changes come as notes at the end.** A new project card, a new date and the last-request line are added as a short message at the end of the history. The message stays in its place in every later request until the next cut. Then the cut folds what it said into the blocks, and the note goes. Notes aren't stored: a restart always starts with a cut, so nothing is lost. Memory edits from other chats were already notices (3.3) and stay so.
3. **The tool list is fixed at the cut.** The cut takes every tool that is on and whose conditions hold (tables, chats, history, bucket), and keeps that list until the next cut. One exception stays: `query` and `describe_table` join at the migration that makes the project's first table, as before. That happens once in a project's life. A Settings switch (8.10) now takes effect at the next turn, with a cut. In the middle of a turn, a tool that was switched off stays in the list, but a call to it is refused as before.
   - I first offered those two tools from the cut even with no tables, so that no migration would change the list. On mistral, missing-features F6 then failed: asked to "save a note in my files", it called `query`, saw "no tables yet" and built a notes table instead of calling `bucket_put`. The before binary passed F6 with `bucket_put`. So I went back to the old rule (commit 5706bab), and all "after" numbers below come from that final code.
4. **No idle-only cuts when nothing changed.** After 5 minutes idle, the cut happens only if it would build other blocks or another tool list (date aside). Memory, a new skill, Mother's chat list, a new schema version or a tool change still cut. The turn and token caps work as before.
5. **Thinking before the cut is left out.** At a cut, the thinking parts of the turns before the cut leave the window, for every model. The tool calls and texts stay. This is valid for all providers: Anthropic drops it at the cut anyway; OpenAI Responses calls don't refer to reasoning items; Gemini calls carry their own signature.
6. **Breaks, not entries.** The runner and the report now count a "break": a request that drops thinking at a new place. One stale block that is dropped again in every later request counts once. The place is the request start (a hash of the system blocks and tool names) plus the first dropped block's path.

What I left out, and why:
- **In-turn trim and series stubs** still change earlier messages in the current turn when a turn grows very large. It is rare, and `drop_block` handles it. Changing it would mean a new design for trimming. Open point 1.
- **The plain harness** keeps a new day as a note too, so its block 1 stays put. Before, it rebuilt block 1 each new day. Plain is a stand-in for a simple chat app; I changed it so the comparison is about the window, not about the date line. Open point 3.

## 2. The notice texts

They are the same for every model. Each is a user-role notice part (the same kind as memory notices).

```
[the date changed: Today is Tuesday, 2026-10-06, in the user's time zone (UTC+03:30).]
[the project card changed; this one replaces the one before]
<the new card, as block 4 would hold it>
[the project card changed: there is none now]
[this is the last request of this turn: answer with what you have, without tools]
```

The last line is the old `lastRequest` text, unchanged. It is now kept as a note, so the next turn's start is the same as this request's.

## 3. Before and after

One run of each set per binary. "Read share" is cache read / (cache read + cache write + uncached input). "Dropped thinking" is blocks / breaks (section 1, item 6). Cost uses the brief's prices.

The plain "after" rows come from commit 0717ae6. Plain requests always carry the full tool list, so the table-tool change in 5706bab doesn't touch them. All jenab "after" rows come from 5706bab.

### Haiku 5.5

| Set | Cache read | Cache write | Read share | Dropped thinking | Passed | Requests | Time | Cost |
|---|---|---|---|---|---|---|---|---|
| bench-bitcoin, jenab, before | 390,505 | 170,060 | 69.6 % | 235 / 8 | 1/1 | 48 | 4m45s | $0.0388 |
| bench-bitcoin, jenab, after | 506,783 | 95,933 | 84.1 % | 4 / 1 | 1/1 | 44 | 1m54s | $0.0279 |
| bench-bitcoin, plain, before | 251,263 | 114,833 | 68.6 % | 232 / 9 | 1/1 | 30 | 1m40s | $0.0206 |
| bench-bitcoin, plain, after | 392,472 | 23,752 | 94.3 % | 0 / 0 | 1/1 | 30 | 1m16s | $0.0112 |
| bench-bitcoin-long, jenab, before | 614,621 | 251,600 | 70.9 % | 316 / 13 | 0/1 | 71 | 5m10s | $0.0571 |
| bench-bitcoin-long, jenab, after | 1,137,453 | 175,042 | 86.6 % | 2 / 1 | 1/1 | 73 | 3m13s | $0.0515 |
| bench-bitcoin-long, plain, before | 535,295 | 238,827 | 69.1 % | 661 / 14 | 1/1 | 45 | 3m13s | $0.0418 |
| bench-bitcoin-long, plain, after | 811,453 | 32,911 | 96.1 % | 0 / 0 | 1/1 | 45 | 1m59s | $0.0181 |
| acceptance, before | 228,379 | 37,549 | 85.8 % | 40 / 4 | 1/1 | 41 | 1m48s | $0.0102 |
| acceptance, after | 232,541 | 37,527 | 86.1 % | 0 / 0 | 1/1 | 42 | 60s | $0.0100 |
| session-notes, before | 46,168 | 12,352 | 78.8 % | 3 / 1 | 1/1 | 11 | 27s | $0.0038 |
| session-notes, after | 61,541 | 13,742 | 81.7 % | 0 / 0 | 1/1 | 13 | 27s | $0.0046 |
| session-notes, before (2nd run) | 50,413 | 13,210 | 79.2 % | 3 / 1 | 1/1 | 11 | 36s | $0.0044 |
| session-notes, after (2nd run, 0717ae6) | 55,729 | 8,849 | 86.3 % | 0 / 0 | 1/1 | 11 | 29s | $0.0034 |

- **Read share** went from 69–86 % to 82–96 %. In the bench sets it gained 14–27 points.
- **Cache writes** fell by 30–44 % with jenab and by 79–86 % with plain. Jenab still writes at each cut (by turns or tokens), as designed.
- **Dropped thinking** went from 3–661 blocks in 1–14 breaks to 0–4 blocks in 0–1 breaks. The one break left in each jenab bench set is the project's first table, when `query` and `describe_table` join (section 1, item 3).
- **Cost** fell by 10–57 % in the bench sets and stayed the same in acceptance. Session-notes is too small to say: +$0.0008 in one pair (13 requests instead of 11) and −$0.0010 in the other.
- **Pass rate:** bench-bitcoin-long with jenab failed before (wrong ETH start date) and passed after. With one run each, that is not a proven gain. Every other set passed both times.
- The jenab "before" break counts come from a binary that keyed breaks by path only, so they are a lower bound. The plain "before" runs were redone with the full key.
- **Total Haiku spend for the ticket: $0.4610 of the $1.00 cap**, over 20 set runs (including reruns). No 402, 429 or billing error.

### mistral-large-4 on NVIDIA (free)

NVIDIA reports cache reads but no cache writes, and mistral never drops thinking. So the useful numbers are the read share and the uncached input.

| Set | Cache read | Uncached input | Read share | Passed | Requests | Time |
|---|---|---|---|---|---|---|
| bench-bitcoin, jenab, before | 260,544 | 99,135 | 72.4 % | 1/1 | 51 | 16m40s |
| bench-bitcoin, jenab, after | 316,992 | 49,034 | 86.6 % | 0/1 | 58 | 58m39s |
| bench-bitcoin, jenab, after (rerun) | 289,856 | 52,008 | 84.8 % | 1/1 | 44 | 9m7s |
| bench-bitcoin, plain, before | 191,744 | 87,080 | 68.8 % | 1/1 | 30 | 12m5s |
| bench-bitcoin, plain, after | 273,216 | 19,091 | 93.5 % | 1/1 | 30 | 25m4s |
| bench-bitcoin-long, jenab, before | 375,808 | 146,469 | 72.0 % | 1/1 | 74 | 32m24s |
| bench-bitcoin-long, jenab, after | 734,144 | 88,733 | 89.2 % | 1/1 | 90 | 24m39s |
| bench-bitcoin-long, plain, before | 398,144 | 193,023 | 67.3 % | 1/1 | 46 | 27m15s |
| bench-bitcoin-long, plain, after | 885,824 | 48,635 | 94.8 % | 1/1 | 45 | 50m12s |
| acceptance, before | 311,424 | 36,362 | 89.5 % | 1/1 | 64 | 17m36s |
| acceptance, after | 304,192 | 35,311 | 89.6 % | 1/1 | 62 | 8m39s |
| session-notes, before | 34,368 | 8,299 | 80.5 % | 1/1 | 13 | 5m45s |
| session-notes, after | 33,344 | 8,044 | 80.6 % | 1/1 | 13 | 2m42s |

- **Read share** in the bench sets went from 67–72 % to 85–95 %. Uncached input fell by 39–78 %. Acceptance and session-notes didn't change: they are short enough that they rarely cut, sit idle or change the card.
- In bench-bitcoin-long with jenab, mistral made 90 requests instead of 74. Its uncached input still fell by 39 %.
- **Times can't be compared.** NVIDIA's speed changed a lot over the two days (a request took 1 s to 4 min), and the after runs had stalls, half an hour of 404s and a 503 outage. The first after runs were also hit by Modern Standby; I threw those away and ran them again.
- **bench-bitcoin with jenab failed the first after run, and passed the rerun.** In turn 16, seven requests in a row stalled or got "Service temporarily unavailable" from NVIDIA. The turn ended after the page was fetched, before the day was saved. In turn 17, the model saw the missing day and saved it. But it ended the turn with the insert's answer ("Comparison below.") and never gave the comparison, so 2 `answer_contains` checks failed. That is the provider outage, not the change.

## 4. Regressions on mistral

"Before" is the before binary where I ran it; otherwise the REFERENCE-SUITE baseline on mistral (earlier code, same model). "0717ae6" is the first after commit, run before the F6 fix.

| Set | Before | 0717ae6 | After (5706bab) |
|---|---|---|---|
| acceptance | 1/1 | 1/1 | 1/1 |
| session-notes | 1/1 (suite: 3/3) | 1/1 | 1/1 |
| memory | not run | 2/2 | 2/2 |
| project-card | not run | 2/2 | 2/2 |
| refresh-memory R1 | suite: 0/1, rerun 0/1 | 1/1 | 0/2 |
| local-refresh | suite: 1/1 | 1/1 | 1/1 |
| schema-design | suite: 3/3; before binary 2/3 | 1/1 | 1/3 |
| schema-widen | suite: 1/3, rerun 0/1 | 1/1 | 1/1 |
| schema-widen-check | suite: 1/3, rerun 0/1 | 1/1 | 1/1 |
| missing-features | 5/6 (F4 cut at 40 min) | 5/6 (F6 failed) | 6/6 |
| bench-bitcoin, jenab | 1/1 | 1/1 | 0/1, rerun 1/1 |
| bench-bitcoin, plain | 1/1 | 1/1 | same code as 0717ae6 |
| bench-bitcoin-long, jenab | 1/1 | 1/1 | 1/1 |
| bench-bitcoin-long, plain | 1/1 | 1/1 | same code as 0717ae6 |

The three sets that failed on the final code:
- **bench-bitcoin with jenab:** an NVIDIA outage in turn 16 (section 3). The rerun passed in 9 minutes. (A rerun before that was cut by Modern Standby after 2 requests; I don't count it.)
- **schema-design:** asked for "the reason it matters", the model named the `btc_news` column `why`, and the check wants `reason`. The other columns, keys and tables were right. This is the first migration in a new project, in turn 2: no window cut, card note or tool change takes part. I ran it twice more on the final binary and three times on the before binary. The before binary failed the same way once (`why_matters`). The final binary named it `why`, `why_it_matters` and `reason`. In a new project's turn 2, both binaries send the same blocks and tools, so I count this as the model's naming, not a drop.
- **refresh-memory R1:** the memory refresh failed. The merge answer hit the output limit of 8192 tokens (mistral used about 40K thinking tokens in this set). With memory empty, the new chat searched the old chat instead, so the 3 `tool_not_called` checks and the 2 `refreshed` checks failed. Its answers were still right (Monday, the parents' group). The refresh step makes its own requests, outside the chat window this ticket changed. The REFERENCE-SUITE baseline failed the same way twice (no JSON, then a time-out). On a rerun the refresh worked ("6 added"), and turns 31 and 32 were answered from memory. In turn 33 (the plant list), the model searched the old chat for an existing table and the Latin-name rule, though memory held the rule. So the 3 `tool_not_called` checks failed again. The new chat is 3 turns long with one cut at its start and no notes, so its requests are the same as they would be with the before code. I count it as the model's choice.

The F6 failure on 0717ae6 is in section 1, item 3. With the old table-tool rule back, F6 passed (`bucket_put`, 2 requests).

On Haiku, all 6 sets passed after (section 3).

`go run ./cmd/jenab-scenarios check scenarios` passes. `go build ./...`, `go vet ./...` and `go test -p 2 -short ./...` pass, and gofmt is clean.

## 5. Proposed SPEC text

Changed lines only; the rest of each section stays.

**3.1, after the list of blocks** (replaces the paragraph with "There is one exception where correctness wins"):

> Provider prompt caches reuse the longest unchanged beginning of a request, and newer Claude models drop earlier thinking when it changed (3.8). The rule that follows: **between window cuts, a request only grows at the end.** Blocks 1–5 and the tool list are built at a cut and kept until the next one (TASK-047). The one exception is the tool list at the project's first table (below). *Refresh memory* (3.5) ends with a cut.
>
> Anything that changes in between is added at the end as a short note and folded into its block at the next cut:
> - another chat edits memory, or this one does (3.3): `[memory updated by another chat: preferences]` and the new text;
> - a schema, view or pipeline change: `[the project card changed; this one replaces the one before]` and the new card, or `[the project card changed: there is none now]`;
> - a new day: `[the date changed: Today is Tuesday, 2026-10-06, in the user's time zone (UTC+03:30).]`;
> - the turn's last request (8.3): `[this is the last request of this turn: answer with what you have, without tools]`.
>
> A note stays in its place in every request until the cut, so the next request still starts the same. Card, date and last-request notes aren't stored: a restart starts with a cut, which builds the blocks fresh.

**3.1, Block 1, Date:**

> **Date:** today's date and the user's time zone, as a UTC offset, since Windows has no IANA name for it. It has no time, so it changes only on a new day. It is built at each cut; a new day between cuts comes as a note.

**3.1, Tool list** (replaces its first line and the `query` line):

> **Tool list** (TASK-033, TASK-047): a tool is in the requests only once the project or the chat has what it works on. This is decided at a cut and kept until the next one. The one exception is the project's first table: `query` and `describe_table` join at the migration that makes it, like the project card. Offered before, `query` led a model to make a table for a note.
> - A tool switched on or off in Settings (8.10) is added or left out from the next turn, which cuts. Within a turn the list stays; a call to a tool that is off is refused.

**3.6, the idle bullet:**

> - If the chat was idle long enough for the provider's cache to expire (5 minutes), the next turn cuts only when a cut would build other blocks 1–5 (the date aside) or another tool list: changed memory, a new schema version, another chat list in Mother, a loaded skill or a tool switch. Otherwise the window stays as it is, and a new day comes as a note (3.1). The first turn after the app starts always cuts.

**3.6, new bullet after "Cut":**

> - **Thinking:** at a cut, the thinking parts of the turns before it leave the window, for every model. Their texts and tool calls stay. Every provider takes this: Anthropic would drop them anyway (3.8), OpenAI's function calls don't refer to reasoning items, and Gemini's calls carry their own signature.

**3.8, Changed context** (replaces the second sentence and the last bullet):

> Jenab changes them only at cuts, where it has already left out the thinking before the cut (3.6), once when the project's first table adds its tools (3.1), and when it trims the current turn or makes stubs of a series (3.7). So adaptive thinking still carries `block_binding.prefix_mismatch_behavior: "drop_block"` …
>
> - The `thinking_dropped` entries of `input_transformations` in `message_start` are counted as the request's dropped thinking blocks, with the first one's path, in the trace and the scenario runner, not stored. The runner also counts **breaks**: requests that drop thinking at a new place, keyed by the system blocks, the tool names and the first dropped path. One stale block dropped again in every later request is one break. Other types and reasons are ignored, and no error text is matched.

## 6. Open points

1. **In-turn trim and series stubs** still change earlier messages of the current turn when a turn grows very large (3.7). That can still drop thinking with `drop_block`. To keep it steady, trimming would have to wait for the next cut or only add at the end; that is a design of its own.
2. **The first table** still changes the tool list once in a project's life: 1 break, 2–4 dropped blocks on Haiku. Offering `query` from the start removes it, but it made mistral build a table for a note (F6). I kept correctness.
3. **The plain harness** now also takes a new day as a note. If plain should stay a stand-in for a chat app that rebuilds its system prompt every day, that one line goes back; it only matters in runs that cross midnight.
4. **Settings switches (8.10)** now take effect at the next turn, which cuts. Within a turn a switched-off tool stays in the list, and a call to it is refused. SPEC 8.10 may want a line on this.
5. **The card's counts can stay older now.** The idle check compares the card by schema version, so plain writes don't count as a change. Before, an idle cut built the card again with new row counts; now the counts are "as of" the last cut until the turn or token cap cuts. The model can always `query` for exact counts.
6. **Notes aren't stored.** A restart always cuts, so nothing is lost, but the first request after a restart writes the whole start again.
7. **Few runs per set.** Most pass rates and times come from one run per binary; only bench-bitcoin, R1 and schema-design were run again on mistral. mistral's times can't be compared (NVIDIA speed, stalls, 404s, a 503 outage, and standby in the first after runs).
8. **Before break counts for jenab** on Haiku are a lower bound (path-only key). The after counts and the plain before counts use the full key.
