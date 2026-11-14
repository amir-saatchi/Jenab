# SPIKE-045 — Subscribe and publish between agents
**Type:** Spike
**Status:** Done (decided 2026-11-15). The user accepted the proposed SPEC 8.6 text. It is in SPEC 8.6, with 2.3, 3.1, 8.1 and 8.9 updated to match.
**Gate:** 3
**Needs:** SPIKE-044 (agents are built the way it decides, now SPEC 8.6) and SPIKE-034 round 2 (sub-sessions and the `Deps.Notices` hook). Both are merged into `main`.

## Question
Agents in a project need to hear about what happens elsewhere: a schema change, a pipeline run, a result another agent found. Three questions:
1. **What does subscribing mean?** A definition precise enough to become SPEC text.
2. **Do models subscribe correctly?** To what their job needs, and not to what it doesn't.
3. **Do models publish correctly?** When another chat needs the result, and not on routine turns.

## Background
- **Turn ends aren't events.** A chat's finish notice goes only up the tree: a sub-session to its parent (SPIKE-034 round 2), an agent to Mother (8.6). Subscriptions are separate, and any chat can have them.
- **Publishing is a choice:** an agent publishes with an explicit call, not after every turn. Most turns are routine.
- **Notices only** (decided with the user, 2026-11-11). A delivered event starts no turn, so nothing can loop. Wake-up turns come later, with their own guards: own events skipped, a hop limit, a budget per chain.
- SPIKE-034 round 2 found that a notice after every sub-session turn kept the parent current on gemma and ministral, and a summary line built at cuts didn't. Parents seldom passed a notice on to the user unprompted (0–3 of 6).

## Definition under test
**A subscription is a standing rule held by the app: when an event of type E, matching filter F, happens, it is delivered to chat C as a notice.**
- **Delivery:** the notice is written into C's history when C's next turn starts, like a memory-changed notice (3.6). At most 1,000 characters. Example: `[schema_changed by chat "Schema agent": orders + column paid_at]`.
- **Own events:** an event C caused itself isn't delivered to C.
- **Two sources:**
  - **App events:** the app publishes them, so no model chooses to. In this spike:
    - `schema_changed`, filtered by table;
    - `pipeline_run`, filtered by pipeline and outcome.

    The project card already shows the new schema. The notice adds what changed and who changed it, which is what lets an agent act on it.
  - **Channels:** agents publish with `publish(channel, title, body)`.
    - Channels are a fixed list per project, made in the setup.
    - Calls to a channel that doesn't exist are refused and counted.
    - `read_report(id)` gets the body.
- **Who subscribes:** the user in the *Customize* panel (SPIKE-044), or the agent with `subscribe(event_or_channel, filter)` and `unsubscribe(id)`.

## Conditions
**Part 1: subscribe.** Only app events, so publishing plays no part.
1. **None:** no subscriptions, only the project card. This is the baseline.
2. **User:** subscriptions from the answer key, as if set in the panel.
3. **Agent:** agents subscribe themselves, from their job, with `subscribe`.

**Part 2: publish.** Channels, with subscriptions from the answer key.
4. **Tool only:** `publish`, with a description that says when to use it.
5. **Tool and prompt section:**
   - the section says: "Before you end a turn whose work changed something another chat depends on, publish it on its channel. Don't publish routine work."
   - block 5 lists the channels with their last 20 titles, built at cuts.

## Scenario
A project with agents set up the way SPIKE-044 decides, and a scripted user over one day: about 12 messages. Some cause events, some are routine.
- **Schema agent** (change data): makes the schema changes the user asks for.
- **Docs agent:** "keep `docs/schema.md` in line with the tables". Needs `schema_changed`.
- **Reviewer:** "look into every pipeline run that fails". Needs `pipeline_run`, with outcome = failed.
- **Price watcher:** publishes on `alerts` when BTC moves more than 5%.
- **Reporter:** subscribes to `alerts` and writes the weekly summary.
- **Data entry:** its job needs no subscriptions. It checks for subscriptions nobody needed.

## Measures
- **Subscribe** (condition 3):
  - needed subscriptions made (recall);
  - unneeded ones (precision, the number that matters most: each one costs on every event);
  - the right filter;
  - made when the agent was created, or later.
- **Publish** (conditions 4 and 5):
  - published when a subscriber needed it (recall);
  - published on routine turns (noise);
  - the right channel;
  - a title that says what happened.
- **Use:** at the subscriber's next turn after a notice, does it act on the event? For example, the docs show the new column, and the reviewer looks at the failed run. Compared with condition 1, where only the card has the change.
- **Cost:** prompt tokens per run, and the notices' share of them.

## Rules
- The same prompts and tool descriptions for every model (SPEC 1). Decided on the reference models ([DEVELOPMENT](../DEVELOPMENT.md#reference-models)). Failures on other models are recorded, not worked around.
- Spike code only: the subscription store, `subscribe`, `publish`, `read_report` and delivery sit behind spike hooks, on `Deps.Notices`. The app leaves them nil.
- A scripted user, as in SPIKE-034. Gemini's free tier allows 500 requests a day, so plan the runs with `-resume`.

## Done when
- Each measure has a number per condition and model.
- The decision is written as SPEC 8.6 text:
  - the definition of a subscription;
  - the v1 app events and their filters;
  - who can subscribe: the user, the agent, or both;
  - the publish rule, and whether the prompt section and the channel list go in;
  - what's left for wake-up turns.

## Result
Ran on 2026-11-14: 30 good runs, 15 a model (conditions 1–5 × 3 reps). The models were Gemini `gemini-3.5-flash-lite` and NVIDIA `mistralai/mistral-large-4-1t-a52b`. Each run is the whole scripted day: 6 agents, 14 messages, one of them in Persian.

- **Gemini:** `GEMINI_API_KEY` ran 7 runs before its 500-a-day quota ran out. With the user's OK, `GEMINI_API_KEY_III` ran the other 8. 918 requests in all.
- **NVIDIA:** 28–45 minutes a run, 1,228 requests. 9 runs were stopped when the laptop had to close, and 1 by Modern Standby (21:19–23:04). All ran again with `-resume`.
- **The smokes didn't count:** both models found the failed run by guessing `r_101` from yesterday's `r_100`. The runs then got IDs that can't be guessed.

The harness is in [`spikes/045-pubsub/`](../../spikes/045-pubsub/README.md). The findings are in its [`results.md`](../../spikes/045-pubsub/results.md), and every table is in [`results/all.md`](../../spikes/045-pubsub/results/all.md).

The spike adds one app hook, `agent.Deps.Section`: text added to block 1 of a chat, for condition 5's publish rule. The app leaves it nil. Everything else sits behind the hooks that were already there: `Notices` (delivery), `Notes` (block 5) and `ChatTools`.

**Part 1, subscribe** (Gemini / NVIDIA):

| | 1 none | 2 user | 3 agent |
|---|---|---|---|
| Needed subscriptions made (recall) | – | key | 2/6 / 2/6 |
| Right filter | – | key | 1/2 / 0/2 |
| Unneeded (precision) | – | key | 0 / 0 (100%) |
| Made at creation / later | – | – | 0, 2 / 0, 2 |
| Notices per run | 0 / 0 | 3.0 / 3.0 | 0.3 / 0 |
| Docs (7) show the new column | 0/3 / 0/3 | 3/3 / 3/3 | 0/3 / 0/3 |
| Reviewer (8) looks at the failed run | 0/3 / 0/3 | 3/3 / 3/3 | 0/3 / 0/3 |
| Docs (13) show the new table | 0/3 / 0/3 | 3/3 / 3/3 | 1/3 / 0/3 |
| Prompt tokens per run | 289K / 461K | 251K / 250K | 322K / 447K |
| Notices' share of them | – | 0.10% / 0.23% | 0.01% / – |

**Part 2, publish** (Gemini / NVIDIA, with the key's subscriptions):

| | 4 tool | 5 tool + section + list |
|---|---|---|
| Published when the reporter needed it (recall) | 2/3 / 0/3 | 3/3 / 3/3 |
| Right channel | 2/2 / – | 3/3 / 3/3 |
| Title says what happened | 2/2 / – | 3/3 / 3/3 |
| Routine turns with a report (noise) | 0/30 / 0/30 | 4/30 / 1/30 |
| Extra reports (useful, but nobody subscribes) | 1 / 0 | 9 / 2 |
| Calls to a channel that doesn't exist | 3 / 0 | 0 / 0 |
| Use at the next turn (docs, reviewer) | 100% / 100% | 100% / 100% |
| Prompt tokens per run | 359K / 262K | 325K / 309K |
| Notices' share / block 5 per run | 0.13%, 2.7K / 0.25%, 2.9K | 0.15%, 10.0K / 0.31%, 11.1K |

The reporter's use measure (the draft has today's 6.4% drop) was 100% in every condition, because a query on `btc_prices` finds it, so it's left out above.

- **Without delivery, agents miss what the card doesn't push into their task.**
  - With only the card and its card-change note, the docs agent never added the new column or table: 0 of 6 checks on each model.
  - The reviewer never found the failed run: the card doesn't show runs.
  - With the key's subscriptions, every check passed. The docs agent updated the file unasked in the same turn as the notice.
- **Notices cost less than the search they replace.**
  - They are 0.1–0.3% of the prompt tokens.
  - Without the failed-run notice, the reviewer searched (with `search_history`, `list_chats`, `run_status` on guessed IDs). Its message-8 turn cost 315K prompt tokens on NVIDIA against 87K with the notice, and 85K against 29K on Gemini.
- **Agents don't subscribe themselves.**
  - With `subscribe`, both models made 2 of the 6 needed subscriptions, none at creation. In its first turn each agent did its task and stopped.
  - The ones made followed the message, not the job: NVIDIA's reviewer subscribed to weekly_summary, the pipeline the user had just named, instead of failures.
  - Precision was perfect only because almost nothing was made. Use was the same as condition 1.
- **Publishing needs the channel list.**
  - With the tool alone, Gemini's first publish named a channel that doesn't exist every time (`general`, `updates`, `bitcoin`). The refusal's list fixed each one.
  - NVIDIA never published with the tool alone. Its watcher wrote "worth flagging to the weekly report" in its reply, and stopped there.
- **The prompt section raises recall and noise.**
  - With the section and the list, every needed report was made, on `alerts`, with a good title.
  - Every noise and extra report came from a chat whose job publishes nothing: the reviewer, the docs agent and the schema agent.
  - The schema agent's reports repeat what `schema_changed` already delivers.
- **Delivery behaved as defined:** once per chat, no turn started by a notice, no loops. No own event came up in a run (the schema agent doesn't subscribe to its own changes); `TestDelivery` covers it.

**Not tested:**
- Mother setting subscriptions in `create_chat`.
- The *Customize* panel.
- Giving `publish` only to chats with a channel to publish on.
- Wake-up turns.

**Decision** (2026-11-15): the user accepted the text below. It is now in SPEC 8.6 as *Subscriptions and channels*. Writing it in also settled these points:

- **8.1, 8.6 and 8.9:** `create_chat` gains `subscriptions` and `publishes`. 8.1's tool table gains `publish` and `read_report`, which come with channels, not with a group.
- **8.6:**
  - The *Customize* panel holds the chat's subscriptions and channels.
  - *Only Mother sends* still holds: a report reaches subscribers as a notice, which starts no turn.
  - The chat comparison says a chat can publish on its channels.
  - The text adds two short lines on why, from the results.
- **3.1:** block 5 holds the chat's subscriptions and channels, and the notes example gains a `schema_changed` notice.
- **2.3:** `chats` gains `publishes`. New tables: `subscriptions`, `channels` and `reports`.
- **Phase 5:** subscriptions and channels.

**SPEC 8.6 text** (proposed, then accepted):

> **Subscriptions and channels** (SPIKE-045)
>
> A **subscription** is a standing rule held by the app: when an event of type E, matching filter F, happens, it is delivered to chat C as a notice.
>
> - **Delivery.**
>   - The notice is written into C's history when C's next turn starts, like a memory-changed notice (3.6). It is one line, at most 1,000 characters, and says what changed and who changed it.
>   - Each event reaches a chat once, and never the chat that caused it.
>   - A notice starts no turn in v1, so nothing can loop.
> - **App events (v1).** The app publishes them; no model chooses to.
>   - `schema_changed`, filtered by table, e.g. `[schema_changed by chat "Schema agent": btc_prices + column volume REAL]`.
>   - `pipeline_run`, filtered by pipeline and by outcome (`success` or `failed`), e.g. `[pipeline_run by app: daily_btc failed at step price: HTTP 503 from api.example.com, run r_q7m2 at 10:00]`.
>   - A filter left out matches any.
> - **Who subscribes.**
>   - **The user,** in the *Customize* panel, next to the groups: an event and its filter, or a channel.
>   - **Mother,** with `create_chat`'s `subscriptions`, from the job: e.g. a reviewer gets `pipeline_run` with outcome `failed`.
>   - Subscriptions can change later in the panel. Unlike groups, they leave no tool calls in the history.
>   - Agents have no `subscribe` tool: in SPIKE-045 they made a third of the needed subscriptions, late and with the wrong filters.
> - **Block 5** lists the chat's subscriptions, built at cuts.
> - **Channels.** A fixed list per project, each with a one-line description, made in the setup or in the project settings. Agents publish reports on them:
>   - **The tools:** `publish(channel, title, body)` and `read_report(id)`.
>     - A chat has `publish` only when it publishes on a channel. That is set like a subscription: by the user in the panel, or by Mother in `create_chat`'s `publishes`.
>     - A chat subscribed to a channel has `read_report`.
>   - **Unknown channels:** a call to a channel that doesn't exist is refused, with the channel list.
>   - **The notice:** subscribers get the title, e.g. `[alerts by chat "Price watcher": BTC dropped 6.40% on 2026-11-12 · report rp_4, read it with read_report]`.
>   - **The prompt section,** only with `publish`: "Before you end a turn whose work changed something another chat depends on, publish it on its channel. Don't publish routine work."
>   - **Block 5** lists the chat's channels, with the last 20 titles of each, built at cuts.
> - **Later: wake-up turns.** A notice that starts a turn, for events that can't wait (a failed run for a reviewer). It needs its own guards before it is built:
>   - own events skipped (already the rule);
>   - a hop limit on chains, where a turn started by a notice publishes and wakes another chat;
>   - a token budget per chain;
>   - a per-subscription *wake* switch, off by default.
>
>   Turns started this way are system-started turns (8.3). They count against the background tasks and `system_turn_max_tokens`.

Changes from the definition under test:

- **No agent `subscribe` or `unsubscribe`:** condition 3 made a third of the needed subscriptions, late, and use was no better than with none.
- **Mother sets subscriptions in `create_chat`:** not tested. SPIKE-044 found she picks groups well, and subscriptions are the same kind of choice. Check it in the Phase 5 scenario runs.
- **The channel list and the prompt section go in,** but `publish` goes only to chats that publish on a channel. In the runs, every noise and extra report came from a chat whose job publishes nothing. Without the list, Gemini guessed channel names and NVIDIA never published.
