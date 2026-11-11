# SPIKE-045 — Subscribe and publish between agents
**Type:** Spike
**Status:** Open (Phase 5)
**Gate:** 3
**Needs:** SPIKE-044 (agents are built the way it decides); SPIKE-034 round 2 (sub-sessions and the `Deps.Notices` hook, on branch `claude/spike-034-topics-1fc440` until merged)

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
