# SPIKE-034 results

Run on 2026-10-27 on `ollama-cloud/gemma4:31b` (`OLLAMA_API_KEY`) and
`mistral/ministral-14b-latest` (`MISTRAL_API_KEY`): 2 chats × 4 conditions ×
3 reps per model, 48 runs, none lost. Gemini `gemini-3.5-flash-lite` ran
conditions 1, 2 and 4 with round 2 on 2026-11-11 and 12: 18 runs, none lost
(findings 10 and 11), in
[`results/gemini/2026-11-11_132250/`](results/gemini/2026-11-11_132250/).
The other runs are in
[`results/2026-10-27_010643/`](results/2026-10-27_010643/) (gemma) and
[`results/mistral/2026-10-27_010844/`](results/mistral/2026-10-27_010844/)
(ministral), with a transcript each; the smoke run is in
[`results/2026-10-27_005749_smoke/`](results/2026-10-27_005749_smoke/).

## Findings

1. **The agent doesn't propose topics.** With `propose_topic` and its rule,
   gemma proposed a new topic at 1 of 70 chances (a message on a subject
   that had no topic) and a return once; ministral proposed none in 72.
   gemma's one new topic came after it had done the work in the main thread,
   so the waiting item stayed in main's notes. gemma did propose a topic on
   the merge message in all 6 runs ("Catalog", "Friday review"), which the
   scripted user declines. Condition 3 ends up as one chat.
2. **A perfect split doesn't beat one chat.** In condition 2 the scripted
   user opens every topic and writes each message in the right one.
   Follow-ups after cuts: gemma 9/9 against 8/9 in one chat; ministral 6/9
   against 8/9.
3. **Topics cost the main thread its reach.** Cross-topic questions asked in
   main: gemma 9/18 against 15/18 in one chat, ministral 6/18 against 15/18.
   Decisions made in a topic and asked in main: 3/6 against 6/6 for both.
   Two causes, both in the transcripts:
   - An index line (`t2 · Follow-up pipeline · 0 open · chat <id>`) names a
     topic but doesn't say what was done there. For "is anything automated
     yet?", gemma read the applications topic and answered "no" in 3 of 3
     runs, twice adding "there are no pipelines", although the follow_up
     topic had made one.
   - Blocks 2–5 are rebuilt only at the thread's own cuts, and the main
     thread has few turns, so its project card is old. ministral answered
     "Monday, 14 days" after the follow_up topic had changed it to Friday
     and 10 days, from main's card. In one chat, the change is in the
     history window instead.
   gemma searched the topics in 4 of the 6 decision turns and still missed
   later changes: it found ORD-0001 and not the switch to INTEGER.
4. **Subjects (condition 4, SPIKE-029's combined design) hold up on gemma
   and not on ministral.** gemma: follow-ups 9/9, cross-topic 15/18, waiting
   work as an open item 6/6, merge 3/6, 0.5 duplicate subjects a run.
   ministral: follow-ups 5/9, cross-topic 12/18, waiting 3/6, merge 3/6; 34
   of its 37 subject writes were sent alone, so it pays a request for each.
5. **Merging works better on subjects than on topics:** the merge message
   ended in one subject in 3 of 6 runs per model, and in one topic in 1 of 6.
6. **The parallel step is safe.** Two topics writing project memory at once
   gave 3 (gemma) and 6 (ministral) revision conflicts. Each was merged and
   saved again, and memory had both facts in every run. ministral lost a
   fact in 15 of 27 memory checks outside condition 2: it passes the right
   `expected_revision` but writes only the new fact over the old.
7. **Cost.** Requests per run were about the same in every condition
   (gemma 38–41, ministral 36–45). Prompt tokens, against one chat: gemma
   +10% (2), +14% (3), +14% (4); ministral −3% (2), +17% (3), +32% (4). A
   topic's first request shares about 2,370 of 2,720 estimated tokens of
   tools and system text with the main thread's, so a provider cache could
   reuse most of it. Mistral reports cache reads (15–20% of prompt tokens in
   every condition); Ollama Cloud reports none.
8. **Free tiers.** Ollama Cloud had one transport error (EOF), retried by
   the app. Mistral's limited tier (30 requests a minute for 14b) answered
   with 429 88 times; the app waited for `retry_after` each time and
   no run failed.
9. **Harness findings for the design:**
   - The store keeps chat titles unique per project, and proposed topic
     titles often matched another chat's. Topics as `chats` rows need a
     title rule (the harness prefixes `tN · `).
   - The app adds a notice mid-turn when project memory changes under a
     running turn. A topic's parallel turn sees it.
   - The per-chat block needed an app hook, `agent.Deps.Notes` (block 5),
     left nil by the app.
10. **Gemini answers across topics where the other two didn't, and topics
    still don't beat one chat.** In condition 2 its main thread answered
    cross-topic questions 17/18 (one chat: 18/18) and every decision made
    in a topic (6/6), against 9/18 and 3/6 (gemma) and 6/18 and 3/6
    (ministral). It read the topics' history in 4 of the 6 decision turns,
    9.2 reads a run that reached another thread. Once in 3 it still answered
    "nothing is automated yet" from the index (`jobs` 10). Follow-ups after
    cuts were 9/9 in every condition, so topics didn't help there. Subjects
    (condition 4): cross-topic 17/18, waiting work as an open item 6/6,
    merge 5/6, 0.2 duplicate subjects a run. Merging topics: 2/6. Prompt
    tokens per run against one chat's 195k: +50% with topics, +23% with
    subjects; requests 45, 62 and 47 a run.
11. **Gemini's free tier.** Its 429s say `gemini-3.5-flash-lite` allows 500
    requests a day and 15 a minute; the plan assumed about 1,500 a day. A
    run takes 39–73 requests, so one key holds about 8 runs a day. The 30
    Gemini runs (these 18 and round 2's 12) used `GEMINI_API_KEY` on both
    days and, with the user's OK, the reserve keys `GEMINI_API_KEY_III` and
    `_IV` on the first; `_II` has no credit (402). Three runs passed the
    40-minute limit while the PC was in Modern Standby (Kernel-Power 506)
    and ran again with `-resume`. The first of them crashed the transcript
    writer on the messages it never sent; it now marks them.

## Tables

66 runs. Each cell adds up both chats and all reps.

## ollama-cloud/gemma4:31b

| | 1 baseline | 2 topics, user opens | 3 topics, agent proposes | 4 subjects |
|---|---|---|---|---|
| Runs (failed) | 6 (0) | 6 (0) | 6 (0) | 6 (0) |
| **Payoff** |  |  |  |  |
| Follow-ups after cuts | 8/9 (89%) | 9/9 (100%) | 9/9 (100%) | 9/9 (100%) |
| Cross-topic questions | 15/18 (83%) | 9/18 (50%) | 15/18 (83%) | 15/18 (83%) |
| Calls that changed earlier work | 17/18 (94%) | 18/18 (100%) | 18/18 (100%) | 18/18 (100%) |
| Main thread: decisions made in topics | 6/6 (100%) | 3/6 (50%) | 6/6 (100%) | 6/6 (100%) |
| ...turns of those that read history | 0/6 | 4/6 | 0/6 | 0/6 |
| **Proposals (3)** |  |  |  |  |
| New-topic chances (a subject without a topic) that got one | – | – | 1/70 (1%) | – |
| Returns proposed into their topic | – | – | 1/2 (50%) | – |
| Noise proposals, per run | 0.0 | 0.0 | 0.0 | 0.0 |
| ...proposals on the merge message, per run | 0.0 | 0.0 | 1.0 | 0.0 |
| Duplicate topics proposed, per run | 0.0 | 0.0 | 0.0 | 0.0 |
| Wrong topic proposed, per run | 0.0 | 0.0 | 0.0 | 0.0 |
| Messages answered outside their topic, per run | 0.0 | 0.0 | 11.7 | 0.0 |
| Proposing turns: requests each | – | – | 3.0 | – |
| Proposing turns: prompt tokens each | – | – | 12404.5 | – |
| Topics / subjects at the end, per run | 0.0 | 3.8 | 0.2 | 3.5 |
| Subjects for main messages, per run (4) | 0.0 | 0.0 | 0.0 | 0.0 |
| Duplicate subjects, per run (4) | 0.0 | 0.0 | 0.0 | 0.5 |
| **Notes and memory** |  |  |  |  |
| Waiting work shows as an open item | – | 5/6 (83%) | 6/6 (100%) | 6/6 (100%) |
| Merge message: one topic / subject | – | 1/6 (17%) | 0/6 (0%) | 3/6 (50%) |
| Notes or subject writes, per run | 0.0 | 9.3 | 8.5 | 9.7 |
| ...alone in their response | 0/0 | 8/56 | 8/51 | 9/58 |
| Parallel steps run at the same time | 0/6 | 6/6 | 0/6 | 0/6 |
| Project memory has both facts | 9/9 (100%) | 9/9 (100%) | 9/9 (100%) | 9/9 (100%) |
| Memory revision conflicts (merged and saved again) | 0 (0) | 3 (3) | 0 (0) | 0 (0) |
| **History and cost** |  |  |  |  |
| History reads, per run | 5.5 | 6.0 | 3.3 | 3.0 |
| ...that reach another thread | 0.0 | 6.0 | 1.7 | 0.0 |
| Requests, per run | 41.3 | 40.2 | 37.8 | 38.8 |
| Prompt tokens, per run (k) | 134.5 | 147.5 | 152.8 | 152.7 |
| Output tokens, per run (k) | 8.5 | 9.8 | 7.2 | 7.7 |
| Cache reads, share of prompt tokens | 0% | 0% | 0% | 0% |
| Topic's first request: prefix shared with main, of tools + system (est. tokens) | – | 2365 of 2718 | 2562 of 3129 | – |
| Request errors | 1 | 1 | 0 | 0 |

## mistral/ministral-14b-latest

| | 1 baseline | 2 topics, user opens | 3 topics, agent proposes | 4 subjects |
|---|---|---|---|---|
| Runs (failed) | 6 (0) | 6 (0) | 6 (0) | 6 (0) |
| **Payoff** |  |  |  |  |
| Follow-ups after cuts | 8/9 (89%) | 6/9 (67%) | 4/9 (44%) | 5/9 (56%) |
| Cross-topic questions | 15/18 (83%) | 6/18 (33%) | 10/18 (56%) | 12/18 (67%) |
| Calls that changed earlier work | 15/18 (83%) | 15/18 (83%) | 15/18 (83%) | 16/18 (89%) |
| Main thread: decisions made in topics | 6/6 (100%) | 3/6 (50%) | 5/6 (83%) | 6/6 (100%) |
| ...turns of those that read history | 0/6 | 0/6 | 0/6 | 0/6 |
| **Proposals (3)** |  |  |  |  |
| New-topic chances (a subject without a topic) that got one | – | – | 0/72 (0%) | – |
| Returns proposed into their topic | – | – | – | – |
| Noise proposals, per run | 0.0 | 0.0 | 0.0 | 0.0 |
| ...proposals on the merge message, per run | 0.0 | 0.0 | 0.0 | 0.0 |
| Duplicate topics proposed, per run | 0.0 | 0.0 | 0.0 | 0.0 |
| Wrong topic proposed, per run | 0.0 | 0.0 | 0.0 | 0.0 |
| Messages answered outside their topic, per run | 0.0 | 0.0 | 12.0 | 0.0 |
| Proposing turns: requests each | – | – | – | – |
| Proposing turns: prompt tokens each | – | – | – | – |
| Topics / subjects at the end, per run | 0.0 | 3.0 | 0.0 | 2.0 |
| Subjects for main messages, per run (4) | 0.0 | 0.0 | 0.0 | 0.0 |
| Duplicate subjects, per run (4) | 0.0 | 0.0 | 0.0 | 0.0 |
| **Notes and memory** |  |  |  |  |
| Waiting work shows as an open item | – | 0/6 (0%) | 0/6 (0%) | 3/6 (50%) |
| Merge message: one topic / subject | – | 1/6 (17%) | 0/6 (0%) | 3/6 (50%) |
| Notes or subject writes, per run | 0.0 | 1.3 | 0.0 | 6.2 |
| ...alone in their response | 0/0 | 7/8 | 0/0 | 34/37 |
| Parallel steps run at the same time | 0/6 | 6/6 | 0/6 | 0/6 |
| Project memory has both facts | 5/9 (56%) | 9/9 (100%) | 4/9 (44%) | 3/9 (33%) |
| Memory revision conflicts (merged and saved again) | 1 (1) | 6 (6) | 0 (0) | 0 (0) |
| **History and cost** |  |  |  |  |
| History reads, per run | 3.2 | 0.2 | 1.3 | 1.0 |
| ...that reach another thread | 0.0 | 0.2 | 1.3 | 0.0 |
| Requests, per run | 40.3 | 35.8 | 36.8 | 44.7 |
| Prompt tokens, per run (k) | 121.8 | 118.3 | 142.5 | 161.2 |
| Output tokens, per run (k) | 2.8 | 3.3 | 2.8 | 3.2 |
| Cache reads, share of prompt tokens | 16% | 15% | 16% | 20% |
| Topic's first request: prefix shared with main, of tools + system (est. tokens) | – | 2371 of 2722 | – | – |
| Request errors | 24 | 19 | 20 | 27 |

## gemini/gemini-3.5-flash-lite

| | 1 baseline | 2 topics, user opens | 4 subjects |
|---|---|---|---|
| Runs (failed) | 6 (0) | 6 (0) | 6 (0) |
| **Payoff** |  |  |  |
| Follow-ups after cuts | 9/9 (100%) | 9/9 (100%) | 9/9 (100%) |
| Cross-topic questions | 18/18 (100%) | 17/18 (94%) | 17/18 (94%) |
| Calls that changed earlier work | 17/18 (94%) | 16/18 (89%) | 17/18 (94%) |
| Main thread: decisions made in topics | 6/6 (100%) | 6/6 (100%) | 6/6 (100%) |
| ...turns of those that read history | 0/6 | 4/6 | 0/6 |
| **Proposals (3)** |  |  |  |
| New-topic chances (a subject without a topic) that got one | – | – | – |
| Returns proposed into their topic | – | – | – |
| Noise proposals, per run | 0.0 | 0.0 | 0.0 |
| ...proposals on the merge message, per run | 0.0 | 0.0 | 0.0 |
| Duplicate topics proposed, per run | 0.0 | 0.0 | 0.0 |
| Wrong topic proposed, per run | 0.0 | 0.0 | 0.0 |
| Messages answered outside their topic, per run | 0.0 | 0.0 | 0.0 |
| Proposing turns: requests each | – | – | – |
| Proposing turns: prompt tokens each | – | – | – |
| Topics / subjects / sub-sessions at the end, per run | 0.0 | 3.7 | 2.5 |
| Subjects for main messages, per run (4) | 0.0 | 0.0 | 0.0 |
| Duplicate subjects, per run (4) | 0.0 | 0.0 | 0.2 |
| **Sub-sessions (5–6)** |  |  |  |
| Asked: one create_session with the task, no work in the parent | – | – | – |
| Finish notice passed on in the parent's answer | – | – | – |
| create_session calls nobody asked for, per run | 0.0 | 0.0 | 0.0 |
| Notices to the parent, per run | 0.0 | 0.0 | 0.0 |
| ...their tokens, per run (est.) | 0.0 | 0.0 | 0.0 |
| **Notes and memory** |  |  |  |
| Waiting work shows as an open item | – | 6/6 (100%) | 6/6 (100%) |
| Merge message: one topic / subject | – | 2/6 (33%) | 5/6 (83%) |
| Notes or subject writes, per run | 0.0 | 11.2 | 11.2 |
| ...alone in their response | 0/0 | 49/67 | 21/67 |
| Parallel steps run at the same time | 0/6 | 6/6 | 0/6 |
| Project memory has both facts | 9/9 (100%) | 8/9 (89%) | 9/9 (100%) |
| Memory revision conflicts (merged and saved again) | 0 (0) | 2 (2) | 0 (0) |
| **History and cost** |  |  |  |
| History reads, per run | 4.5 | 9.3 | 3.8 |
| ...that reach another thread | 0.0 | 9.2 | 0.0 |
| Requests, per run | 45.5 | 61.7 | 47.2 |
| Prompt tokens, per run (k) | 194.7 | 292.2 | 239.8 |
| ...of the main thread or parent (k) | 194.7 | 123.0 | 239.8 |
| Output tokens, per run (k) | 8.7 | 11.8 | 10.0 |
| Cache reads, share of prompt tokens | 29% | 35% | 40% |
| Topic's first request: prefix shared with main, of tools + system (est. tokens) | – | 2370 of 2721 | – |
| Request errors | 24 | 32 | 23 |

## Checks one by one

Passes per check, over the reps.

### ollama-cloud/gemma4:31b

| Chat | # | Check | Message | 1 | 2 | 3 | 4 |
|---|---|---|---|---|---|---|---|
| jobs | 1 | new topic | Create a table applications with company, role, applied (a d… | – | – | 0/3 | – |
| jobs | 2 | new topic | Add these three: Zalando, Data Analyst, applied 2026-09-20, … | – | – | 0/3 | – |
| jobs | 3 | memory | When I say a company rejected me, set the status to closed, … | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 3 | new topic | When I say a company rejected me, set the status to closed, … | – | – | 0/3 | – |
| jobs | 5 | new topic | Add a pipeline follow_up that runs every Monday at 09:00 and… | – | – | 0/3 | – |
| jobs | 6 | new topic | I also want a pipeline that checks Zalando's careers page ev… | – | – | 1/3 | – |
| jobs | 6 | waiting | I also want a pipeline that checks Zalando's careers page ev… | – | 2/3 | 3/3 | 3/3 |
| jobs | 8 | new topic | Make a view board that groups the applications by status. | – | – | 0/3 | – |
| jobs | 9 | call | N26 rejected me. | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 9 | new topic | N26 rejected me. | – | – | 0/3 | – |
| jobs | 10 | cross | Which of my applications are still waiting, and is anything … | 3/3 | 0/3 | 3/3 | 3/3 |
| jobs | 11 | call | Make follow_up run on Fridays instead, and use 10 days inste… | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 11 | new topic | Make follow_up run on Fridays instead, and use 10 days inste… | – | – | 0/3 | – |
| jobs | 12 | new topic | Remember this for the whole project: I only apply to jobs in… | – | – | 0/3 | – |
| jobs | 13 | memory | Remember this for the whole project: job alerts should only … | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 13 | new topic | Remember this for the whole project: job alerts should only … | – | – | 0/2 | – |
| jobs | 13 | return | Remember this for the whole project: job alerts should only … | – | – | 0/1 | – |
| jobs | 14 | merge | follow_up and the board are really one thing for me: my Frid… | – | 1/3 | 0/3 | 3/3 |
| jobs | 15 | cross | What day does follow_up run now, and after how many days doe… | 3/3 | 2/3 | 3/3 | 3/3 |
| jobs | 15 | reach | What day does follow_up run now, and after how many days doe… | 3/3 | 2/3 | 3/3 | 3/3 |
| jobs | 16 | call | Here's the Zalando link: https://jobs.zalando.com/en/jobs?se… | 2/3 | 3/3 | 3/3 | 3/3 |
| jobs | 16 | new topic | Here's the Zalando link: https://jobs.zalando.com/en/jobs?se… | – | – | 0/2 | – |
| jobs | 16 | return | Here's the Zalando link: https://jobs.zalando.com/en/jobs?se… | – | – | 1/1 | – |
| jobs | 18 | follow-up | What happened with my N26 application, and why does it say c… | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 18 | new topic | What happened with my N26 application, and why does it say c… | – | – | 0/3 | – |
| jobs | 19 | cross | What have we set up so far, and is anything still waiting on… | 3/3 | 3/3 | 3/3 | 2/3 |
| sqlite | 1 | new topic | Create a table users with id, email, name and created_at. id… | – | – | 0/3 | – |
| sqlite | 2 | new topic | Add three users: Ana (ana@example.org), Ben (ben@example.org… | – | – | 0/3 | – |
| sqlite | 3 | new topic | Next, a table orders with id, user_id, total and created_at.… | – | – | 0/3 | – |
| sqlite | 5 | new topic | Create a table products with sku, name and price. sku is the… | – | – | 0/3 | – |
| sqlite | 6 | call | In orders, user_id must point to users.id. | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 6 | new topic | In orders, user_id must point to users.id. | – | – | 0/3 | – |
| sqlite | 8 | new topic | I need stock counts too: a table inventory with sku and quan… | – | – | 0/3 | – |
| sqlite | 8 | waiting | I need stock counts too: a table inventory with sku and quan… | – | 3/3 | 3/3 | 3/3 |
| sqlite | 9 | cross | How do users and orders relate? | 0/3 | 1/3 | 0/3 | 1/3 |
| sqlite | 10 | call | Add a column discount to products, 0 by default. | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 10 | new topic | Add a column discount to products, 0 by default. | – | – | 0/3 | – |
| sqlite | 11 | call | Change of mind on orders: use a plain INTEGER key after all.… | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 11 | new topic | Change of mind on orders: use a plain INTEGER key after all.… | – | – | 0/3 | – |
| sqlite | 12 | new topic | Remember this for the whole project: all timestamps are stor… | – | – | 0/3 | – |
| sqlite | 13 | memory | Remember this for the whole project: money is stored as whol… | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 13 | new topic | Remember this for the whole project: money is stored as whol… | – | – | 0/3 | – |
| sqlite | 14 | cross | Did we settle the key for orders? | 3/3 | 1/3 | 3/3 | 3/3 |
| sqlite | 14 | reach | Did we settle the key for orders? | 3/3 | 1/3 | 3/3 | 3/3 |
| sqlite | 15 | merge | Products and inventory are one thing for me: my catalog. Kee… | – | 0/3 | 0/3 | 0/3 |
| sqlite | 17 | follow-up | Why did we make email unique in users? | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 17 | new topic | Why did we make email unique in users? | – | – | 0/3 | – |
| sqlite | 18 | follow-up | Is the restock check done? | 2/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 18 | new topic | Is the restock check done? | – | – | 0/3 | – |
| sqlite | 19 | cross | What's still open everywhere? | 3/3 | 2/3 | 3/3 | 3/3 |

### mistral/ministral-14b-latest

| Chat | # | Check | Message | 1 | 2 | 3 | 4 |
|---|---|---|---|---|---|---|---|
| jobs | 1 | new topic | Create a table applications with company, role, applied (a d… | – | – | 0/3 | – |
| jobs | 2 | new topic | Add these three: Zalando, Data Analyst, applied 2026-09-20, … | – | – | 0/3 | – |
| jobs | 3 | memory | When I say a company rejected me, set the status to closed, … | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 3 | new topic | When I say a company rejected me, set the status to closed, … | – | – | 0/3 | – |
| jobs | 5 | new topic | Add a pipeline follow_up that runs every Monday at 09:00 and… | – | – | 0/3 | – |
| jobs | 6 | new topic | I also want a pipeline that checks Zalando's careers page ev… | – | – | 0/3 | – |
| jobs | 6 | waiting | I also want a pipeline that checks Zalando's careers page ev… | – | 0/3 | 0/3 | 3/3 |
| jobs | 8 | new topic | Make a view board that groups the applications by status. | – | – | 0/3 | – |
| jobs | 9 | call | N26 rejected me. | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 9 | new topic | N26 rejected me. | – | – | 0/3 | – |
| jobs | 10 | cross | Which of my applications are still waiting, and is anything … | 2/3 | 0/3 | 0/3 | 1/3 |
| jobs | 11 | call | Make follow_up run on Fridays instead, and use 10 days inste… | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 11 | new topic | Make follow_up run on Fridays instead, and use 10 days inste… | – | – | 0/3 | – |
| jobs | 12 | new topic | Remember this for the whole project: I only apply to jobs in… | – | – | 0/3 | – |
| jobs | 13 | memory | Remember this for the whole project: job alerts should only … | 1/3 | 3/3 | 1/3 | 0/3 |
| jobs | 13 | new topic | Remember this for the whole project: job alerts should only … | – | – | 0/3 | – |
| jobs | 14 | merge | follow_up and the board are really one thing for me: my Frid… | – | 1/3 | 0/3 | 0/3 |
| jobs | 15 | cross | What day does follow_up run now, and after how many days doe… | 3/3 | 0/3 | 2/3 | 3/3 |
| jobs | 15 | reach | What day does follow_up run now, and after how many days doe… | 3/3 | 0/3 | 2/3 | 3/3 |
| jobs | 16 | call | Here's the Zalando link: https://jobs.zalando.com/en/jobs?se… | 0/3 | 0/3 | 0/3 | 1/3 |
| jobs | 16 | new topic | Here's the Zalando link: https://jobs.zalando.com/en/jobs?se… | – | – | 0/3 | – |
| jobs | 18 | follow-up | What happened with my N26 application, and why does it say c… | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 18 | new topic | What happened with my N26 application, and why does it say c… | – | – | 0/3 | – |
| jobs | 19 | cross | What have we set up so far, and is anything still waiting on… | 1/3 | 0/3 | 0/3 | 0/3 |
| sqlite | 1 | new topic | Create a table users with id, email, name and created_at. id… | – | – | 0/3 | – |
| sqlite | 2 | new topic | Add three users: Ana (ana@example.org), Ben (ben@example.org… | – | – | 0/3 | – |
| sqlite | 3 | new topic | Next, a table orders with id, user_id, total and created_at.… | – | – | 0/3 | – |
| sqlite | 5 | new topic | Create a table products with sku, name and price. sku is the… | – | – | 0/3 | – |
| sqlite | 6 | call | In orders, user_id must point to users.id. | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 6 | new topic | In orders, user_id must point to users.id. | – | – | 0/3 | – |
| sqlite | 8 | new topic | I need stock counts too: a table inventory with sku and quan… | – | – | 0/3 | – |
| sqlite | 8 | waiting | I need stock counts too: a table inventory with sku and quan… | – | 0/3 | 0/3 | 0/3 |
| sqlite | 9 | cross | How do users and orders relate? | 3/3 | 3/3 | 2/3 | 3/3 |
| sqlite | 10 | call | Add a column discount to products, 0 by default. | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 10 | new topic | Add a column discount to products, 0 by default. | – | – | 0/3 | – |
| sqlite | 11 | call | Change of mind on orders: use a plain INTEGER key after all.… | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 11 | new topic | Change of mind on orders: use a plain INTEGER key after all.… | – | – | 0/3 | – |
| sqlite | 12 | new topic | Remember this for the whole project: all timestamps are stor… | – | – | 0/3 | – |
| sqlite | 13 | memory | Remember this for the whole project: money is stored as whol… | 1/3 | 3/3 | 0/3 | 0/3 |
| sqlite | 13 | new topic | Remember this for the whole project: money is stored as whol… | – | – | 0/3 | – |
| sqlite | 14 | cross | Did we settle the key for orders? | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 14 | reach | Did we settle the key for orders? | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 15 | merge | Products and inventory are one thing for me: my catalog. Kee… | – | 0/3 | 0/3 | 3/3 |
| sqlite | 17 | follow-up | Why did we make email unique in users? | 3/3 | 3/3 | 1/3 | 1/3 |
| sqlite | 17 | new topic | Why did we make email unique in users? | – | – | 0/3 | – |
| sqlite | 18 | follow-up | Is the restock check done? | 2/3 | 0/3 | 0/3 | 1/3 |
| sqlite | 18 | new topic | Is the restock check done? | – | – | 0/3 | – |
| sqlite | 19 | cross | What's still open everywhere? | 3/3 | 0/3 | 3/3 | 2/3 |

### gemini/gemini-3.5-flash-lite

| Chat | # | Check | Message | 1 | 2 | 4 |
|---|---|---|---|---|---|---|
| jobs | 3 | memory | When I say a company rejected me, set the status to closed, … | 3/3 | 3/3 | 3/3 |
| jobs | 6 | waiting | I also want a pipeline that checks Zalando's careers page ev… | – | 3/3 | 3/3 |
| jobs | 9 | call | N26 rejected me. | 3/3 | 3/3 | 3/3 |
| jobs | 10 | cross | Which of my applications are still waiting, and is anything … | 3/3 | 2/3 | 3/3 |
| jobs | 11 | call | Make follow_up run on Fridays instead, and use 10 days inste… | 3/3 | 3/3 | 3/3 |
| jobs | 13 | memory | Remember this for the whole project: job alerts should only … | 3/3 | 3/3 | 3/3 |
| jobs | 14 | merge | follow_up and the board are really one thing for me: my Frid… | – | 1/3 | 3/3 |
| jobs | 15 | cross | What day does follow_up run now, and after how many days doe… | 3/3 | 3/3 | 3/3 |
| jobs | 15 | reach | What day does follow_up run now, and after how many days doe… | 3/3 | 3/3 | 3/3 |
| jobs | 16 | call | Here's the Zalando link: https://jobs.zalando.com/en/jobs?se… | 2/3 | 1/3 | 2/3 |
| jobs | 18 | follow-up | What happened with my N26 application, and why does it say c… | 3/3 | 3/3 | 3/3 |
| jobs | 19 | cross | What have we set up so far, and is anything still waiting on… | 3/3 | 3/3 | 2/3 |
| sqlite | 6 | call | In orders, user_id must point to users.id. | 3/3 | 3/3 | 3/3 |
| sqlite | 8 | waiting | I need stock counts too: a table inventory with sku and quan… | – | 3/3 | 3/3 |
| sqlite | 9 | cross | How do users and orders relate? | 3/3 | 3/3 | 3/3 |
| sqlite | 10 | call | Add a column discount to products, 0 by default. | 3/3 | 3/3 | 3/3 |
| sqlite | 11 | call | Change of mind on orders: use a plain INTEGER key after all.… | 3/3 | 3/3 | 3/3 |
| sqlite | 13 | memory | Remember this for the whole project: money is stored as whol… | 3/3 | 2/3 | 3/3 |
| sqlite | 14 | cross | Did we settle the key for orders? | 3/3 | 3/3 | 3/3 |
| sqlite | 14 | reach | Did we settle the key for orders? | 3/3 | 3/3 | 3/3 |
| sqlite | 15 | merge | Products and inventory are one thing for me: my catalog. Kee… | – | 1/3 | 2/3 |
| sqlite | 17 | follow-up | Why did we make email unique in users? | 3/3 | 3/3 | 3/3 |
| sqlite | 18 | follow-up | Is the restock check done? | 3/3 | 3/3 | 3/3 |
| sqlite | 19 | cross | What's still open everywhere? | 3/3 | 3/3 | 3/3 |
