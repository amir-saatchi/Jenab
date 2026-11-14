# SPIKE-045 results

Subscribe and publish between agents: what does subscribing mean, do models
subscribe to what their job needs, and do they publish when another chat
needs the result? 30 good runs on the two reference models, 2026-11-14.
Every number below is in the generated tables:
[`results/all.md`](results/all.md) (both models), and one `results.md` per
model under `results/<model>/<stamp>/`, with a transcript per run.

## Runs

| Model | Runs | Of | Requests | Left |
|---|---|---|---|---|
| Gemini `gemini-3.5-flash-lite` | 15 | 15 | 918 | none |
| NVIDIA `mistralai/mistral-large-4-1t-a52b` | 15 | 15 | 1,228 | none |

15 runs a model: conditions 1–5 × 3 reps. Each run is the whole scripted day:
6 agents, 14 messages (`script.go`).

**Keys.**
- `GEMINI_API_KEY` ran Gemini's first 7 runs before its 500-a-day quota ran
  out. With the user's OK (asked in this session), `GEMINI_API_KEY_III` ran
  the other 8 (`models-gemini-iii.yaml`).
- NVIDIA's free endpoint took 28–45 minutes a run at 2 runs in parallel,
  with no hangs.

**Lost runs.**
- **Laptop closed.** The user had to close the laptop at about 16:45. That
  stopped 9 NVIDIA runs, which ran again with `-resume` from 18:33.
- **Modern Standby.** The PC slept from 21:19 to 23:04 (Kernel-Power
  506/507), even with the keep-awake hold, because the session counted as
  idle while it waited on the batch. NVIDIA's condition 5 rep 3 hit its time
  limit and ran again with `-resume`.
- **Smoke runs.** The smokes (one condition-3 run per model) used run IDs
  numbered `r_100` to `r_103`. Both models found the failed run by guessing
  `r_101` from yesterday's `r_100`, which would have let condition 1 find it
  without a notice. The runs then got IDs that can't be guessed (`r_q7m2`),
  and the smokes aren't counted.

**Not errors.** The `Error:` lines under some turns in the transcripts are
single requests that the provider layer retried (15 a minute on Gemini's
free tier, 429s on NVIDIA). No turn stopped on one.

## Findings

### 1. Without delivery, agents miss what the card doesn't push into their task

Condition 1 has no subscriptions, so only the project card shows the change.
The card also gets a card-change note when the schema changes.

| | Docs (7) shows `volume` | Reviewer (8) looks at the failed run | Docs (13) shows `eth_prices` |
|---|---|---|---|
| Gemini, 1 none | 0/3 | 0/3 | 0/3 |
| NVIDIA, 1 none | 0/3 | 0/3 | 0/3 |
| Gemini, 2 user | 3/3 | 3/3 | 3/3 |
| NVIDIA, 2 user | 3/3 | 3/3 | 3/3 |

- **The docs agent:** at messages 7 and 13 the user asks for something else
  (an intro, a "Last reviewed" line). The card has the new column, and block
  2 has the card-change note. Without a notice, no run on either model added
  it.
  - With the notice (`[schema_changed by chat "Schema agent": btc_prices +
    column volume REAL]`), every run updated the docs in the same turn,
    unasked.
- **The reviewer:** the card doesn't show runs. Without the `pipeline_run`
  notice, the reviewer answered message 8 ("does weekly_summary still look
  fine?") about weekly_summary only, and never found daily_btc's failure.
  - With the notice, every run looked at `r_q7m2` (with `run_status`, or by
    naming it or the 503).
- **The reporter** got today's 6.4% drop in every condition (3/3 everywhere),
  because today's price is in `btc_prices` and a query finds it. That measure
  doesn't separate the conditions.

### 2. Notices cost less than the search they replace

- **Notices' share:** 0.10–0.31% of the prompt tokens per run (244–954
  estimated tokens). Each notice is one line.
- **Cheaper runs with notices.** Without a notice, the reviewer searched for
  what happened, with `search_history`, `list_chats`, `run_status` on guessed
  IDs and `query`.
  - Message 8 alone cost the reviewer 315K prompt tokens on NVIDIA in
    condition 1, against 87K in condition 2. On Gemini it was 85K against 29K.
  - Over the whole day, NVIDIA's condition 1 cost 461K prompt tokens a run,
    and condition 2 250K. Gemini's were 289K and 251K.
- **Block 5:** the subscriptions list adds 2–3K tokens a run, summed over
  every request. Condition 5's channel list adds about 7–9K more (2–3% of
  the prompt tokens).

### 3. Agents don't subscribe themselves

Condition 3 gives every chat `subscribe` and `unsubscribe`. The description
names the two events and their filters, and says "Every notice costs tokens,
so subscribe only to what this chat's job needs". The roles say what each job
needs ("You look into every pipeline run that fails"), without naming the
tool.

| | Needed made (recall) | Right filter | Unneeded | At creation | Later |
|---|---|---|---|---|---|
| Gemini | 2/6 (33%) | 1/2 | 0 | 0 | 2 |
| NVIDIA | 2/6 (33%) | 0/2 | 0 | 0 | 2 |

- **Never at creation.** In its first turn each agent did the task it was
  given and stopped. The 4 subscriptions made came later:
  - the reviewer at message 8, when the user asked about weekly_summary:
    `pipeline_run (any)` on Gemini, `pipeline = weekly_summary` on NVIDIA
    (twice);
  - the docs agent at message 7, after the `volume` change it then missed
    (Gemini, once).
- **The filters followed the message, not the job.** NVIDIA's reviewer
  subscribed to the pipeline the user had just named, not to failures.
- **Precision was perfect** (0 unneeded, no duplicates, no unsubscribes).
  Data entry, the price watcher and the reporter never subscribed. But that
  is because almost nothing was made.
- **Use ≈ condition 1:** 0/3 on docs (7) and the reviewer (8) for both
  models. Only the one early docs subscription paid off (Gemini, docs (13)
  1/3).

### 4. Publishing needs the channel list; the prompt section raises recall and noise

Part 2 gives every chat `publish` and `read_report`, and the key's
subscriptions (the reporter subscribes to `alerts`). The needed report is
the price watcher's, at message 9, when BTC fell 6.4%. Its role says that a
move over 5% "matters to other chats", without naming the tool or the
channel.

| | Published (recall) | Right channel | Title | Routine turns with a report | Extra | Refused |
|---|---|---|---|---|---|---|
| Gemini, 4 tool | 2/3 | 2/2 | 2/2 | 0/30 | 1 | 3 |
| Gemini, 5 tool + section + list | 3/3 | 3/3 | 3/3 | 4/30 (13%) | 9 | 0 |
| NVIDIA, 4 tool | 0/3 | – | – | 0/30 | 0 | 0 |
| NVIDIA, 5 tool + section + list | 3/3 | 3/3 | 3/3 | 1/30 (3%) | 2 | 0 |

- **Tool only (4):** the description says when to publish, but the
  channels are only in the refusal.
  - Gemini's first `publish` named a channel that doesn't exist every time
    (`general`, `updates`, `bitcoin`: 3 of 3). The refusal lists the
    channels, and each time the next call used the right one.
  - NVIDIA never published. Its watcher wrote "worth flagging to the weekly
    report" in its reply, and stopped there.
- **Section and list (5):** every needed report was made, on `alerts`, with
  a title that names BTC and the move (e.g. "BTC dropped 6.40% on 2026-11-12
  (crossed 5%)"). No refusals.
  - **Noise** came from chats with nothing to publish:
    - the reviewer, on its routine first look at yesterday's run (both
      models);
    - the docs agent, after its own edits (Gemini, 2 reports).
  - **Extra** reports were useful, but nobody subscribed to them:
    - the schema agent's two changes on `data` (Gemini, every run), which
      `schema_changed` already delivers;
    - the reviewer's failed-run report on `reviews` (both models).
- **The reporter used the report:** after the `alerts` notice it called
  `read_report` (Gemini 3/3 in condition 5, NVIDIA 3/3) and put the drop
  in the draft. It would also have found the drop with a query (finding 1).

### 5. Delivery behaved as defined

- **Once per chat:** each matching event became one notice per subscribed
  chat (3 a run in condition 2; 4 in condition 5 with the `alerts` report).
- **Own events:** the schema agent never subscribes to `schema_changed`, so
  no own event was skipped in a run. `TestDelivery` covers it.
- **No turn started by a notice:** every notice waited for the chat's next
  scripted message.
- **No loops:** with notices only, a publish never started another turn.

## Decision

The proposed SPEC 8.6 text is in the [ticket's Result](../../Docs/Tickets/SPIKE-045-subscribe-and-publish.md#result). It waits for the user's decision.

## Limits

- **Two models:** both reference models, 3 reps each. They agree on every
  finding, but 3 reps can't tell 67% from 100%.
- **The reporter's use measure** can't separate the conditions (finding 1).
- **No wake-up turns:** they weren't tested; see the ticket.
- **Not tested:** Mother setting subscriptions with `create_chat`. This spike
  gave the key's subscriptions as if the user had set them in the panel.
- **Publish for every chat:** Part 2 gave `publish` to every chat. Every
  noise and extra report came from a chat whose job publishes nothing. So
  giving `publish` only to chats with a channel to publish on would have
  removed them all in these runs, but that wasn't run.
