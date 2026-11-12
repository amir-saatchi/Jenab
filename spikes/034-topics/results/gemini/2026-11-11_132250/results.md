# SPIKE-034 results

30 runs. Each cell adds up both chats and all reps.

## gemini/gemini-3.5-flash-lite

| | 1 baseline | 2 topics, user opens | 4 subjects | 5 sub-sessions, notices | 6 sub-sessions, summary line |
|---|---|---|---|---|---|
| Runs (failed) | 6 (0) | 6 (0) | 6 (0) | 6 (0) | 6 (0) |
| **Payoff** |  |  |  |  |  |
| Follow-ups after cuts | 9/9 (100%) | 9/9 (100%) | 9/9 (100%) | 9/9 (100%) | 9/9 (100%) |
| Cross-topic questions | 18/18 (100%) | 17/18 (94%) | 17/18 (94%) | 17/18 (94%) | 17/18 (94%) |
| Calls that changed earlier work | 17/18 (94%) | 16/18 (89%) | 17/18 (94%) | 18/18 (100%) | 17/18 (94%) |
| Main thread: decisions made in topics | 6/6 (100%) | 6/6 (100%) | 6/6 (100%) | 6/6 (100%) | 6/6 (100%) |
| ...turns of those that read history | 0/6 | 4/6 | 0/6 | 0/6 | 1/6 |
| **Proposals (3)** |  |  |  |  |  |
| New-topic chances (a subject without a topic) that got one | – | – | – | – | – |
| Returns proposed into their topic | – | – | – | – | – |
| Noise proposals, per run | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 |
| ...proposals on the merge message, per run | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 |
| Duplicate topics proposed, per run | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 |
| Wrong topic proposed, per run | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 |
| Messages answered outside their topic, per run | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 |
| Proposing turns: requests each | – | – | – | – | – |
| Proposing turns: prompt tokens each | – | – | – | – | – |
| Topics / subjects / sub-sessions at the end, per run | 0.0 | 3.7 | 2.5 | 3.0 | 3.0 |
| Subjects for main messages, per run (4) | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 |
| Duplicate subjects, per run (4) | 0.0 | 0.0 | 0.2 | 0.0 | 0.0 |
| **Sub-sessions (5–6)** |  |  |  |  |  |
| Asked: one create_session with the task, no work in the parent | – | – | – | 6/6 (100%) | 6/6 (100%) |
| Finish notice passed on in the parent's answer | – | – | – | 1/6 (17%) | 5/6 (83%) |
| create_session calls nobody asked for, per run | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 |
| Notices to the parent, per run | 0.0 | 0.0 | 0.0 | 12.0 | 1.0 |
| ...their tokens, per run (est.) | 0.0 | 0.0 | 0.0 | 518.5 | 69.2 |
| **Notes and memory** |  |  |  |  |  |
| Waiting work shows as an open item | – | 6/6 (100%) | 6/6 (100%) | 4/6 (67%) | 4/6 (67%) |
| Merge message: one topic / subject | – | 2/6 (33%) | 5/6 (83%) | 6/6 (100%) | 6/6 (100%) |
| Notes or subject writes, per run | 0.0 | 11.2 | 11.2 | 13.0 | 12.8 |
| ...alone in their response | 0/0 | 49/67 | 21/67 | 65/78 | 64/77 |
| Parallel steps run at the same time | 0/6 | 6/6 | 0/6 | 6/6 | 6/6 |
| Project memory has both facts | 9/9 (100%) | 8/9 (89%) | 9/9 (100%) | 9/9 (100%) | 8/9 (89%) |
| Memory revision conflicts (merged and saved again) | 0 (0) | 2 (2) | 0 (0) | 1 (1) | 1 (1) |
| **History and cost** |  |  |  |  |  |
| History reads, per run | 4.5 | 9.3 | 3.8 | 3.8 | 5.7 |
| ...that reach another thread | 0.0 | 9.2 | 0.0 | 3.7 | 5.3 |
| Requests, per run | 45.5 | 61.7 | 47.2 | 60.8 | 64.7 |
| Prompt tokens, per run (k) | 194.7 | 292.2 | 239.8 | 281.7 | 315.3 |
| ...of the main thread or parent (k) | 194.7 | 123.0 | 239.8 | 116.8 | 138.5 |
| Output tokens, per run (k) | 8.7 | 11.8 | 10.0 | 11.0 | 12.0 |
| Cache reads, share of prompt tokens | 29% | 35% | 40% | 34% | 37% |
| Topic's first request: prefix shared with main, of tools + system (est. tokens) | – | 2370 of 2721 | – | 2549 of 2851 | 2567 of 2875 |
| Request errors | 24 | 32 | 23 | 29 | 31 |

## Checks one by one

Passes per check, over the reps.

### gemini/gemini-3.5-flash-lite

| Chat | # | Check | Message | 1 | 2 | 4 | 5 | 6 |
|---|---|---|---|---|---|---|---|---|
| jobs | 3 | memory | When I say a company rejected me, set the status to closed, … | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 6 | asked session | I also want a pipeline that checks Zalando's careers page ev… | – | – | – | 3/3 | 3/3 |
| jobs | 6 | finish notice passed on | I also want a pipeline that checks Zalando's careers page ev… | – | – | – | 1/3 | 2/3 |
| jobs | 6 | waiting | I also want a pipeline that checks Zalando's careers page ev… | – | 3/3 | 3/3 | 1/3 | 2/3 |
| jobs | 9 | call | N26 rejected me. | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 10 | cross | Which of my applications are still waiting, and is anything … | 3/3 | 2/3 | 3/3 | 2/3 | 2/3 |
| jobs | 11 | call | Make follow_up run on Fridays instead, and use 10 days inste… | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 13 | memory | Remember this for the whole project: job alerts should only … | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 14 | merge | follow_up and the board are really one thing for me: my Frid… | – | 1/3 | 3/3 | 3/3 | 3/3 |
| jobs | 15 | cross | What day does follow_up run now, and after how many days doe… | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 15 | reach | What day does follow_up run now, and after how many days doe… | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 16 | call | Here's the Zalando link: https://jobs.zalando.com/en/jobs?se… | 2/3 | 1/3 | 2/3 | 3/3 | 2/3 |
| jobs | 18 | follow-up | What happened with my N26 application, and why does it say c… | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 19 | cross | What have we set up so far, and is anything still waiting on… | 3/3 | 3/3 | 2/3 | 3/3 | 3/3 |
| sqlite | 6 | call | In orders, user_id must point to users.id. | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 8 | asked session | I need stock counts too: a table inventory with sku and quan… | – | – | – | 3/3 | 3/3 |
| sqlite | 8 | finish notice passed on | I need stock counts too: a table inventory with sku and quan… | – | – | – | 0/3 | 3/3 |
| sqlite | 8 | waiting | I need stock counts too: a table inventory with sku and quan… | – | 3/3 | 3/3 | 3/3 | 2/3 |
| sqlite | 9 | cross | How do users and orders relate? | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 10 | call | Add a column discount to products, 0 by default. | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 11 | call | Change of mind on orders: use a plain INTEGER key after all.… | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 13 | memory | Remember this for the whole project: money is stored as whol… | 3/3 | 2/3 | 3/3 | 3/3 | 2/3 |
| sqlite | 14 | cross | Did we settle the key for orders? | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 14 | reach | Did we settle the key for orders? | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 15 | merge | Products and inventory are one thing for me: my catalog. Kee… | – | 1/3 | 2/3 | 3/3 | 3/3 |
| sqlite | 17 | follow-up | Why did we make email unique in users? | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 18 | follow-up | Is the restock check done? | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 19 | cross | What's still open everywhere? | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
