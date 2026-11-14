# SPIKE-034 results

12 runs. Each cell adds up both chats and all reps.

## ollama-cloud/gemma4:31b

| | 5 sub-sessions, notices | 6 sub-sessions, summary line |
|---|---|---|
| Runs (failed) | 6 (0) | 6 (0) |
| **Payoff** |  |  |
| Follow-ups after cuts | 9/9 (100%) | 9/9 (100%) |
| Cross-topic questions | 15/18 (83%) | 11/18 (61%) |
| Calls that changed earlier work | 18/18 (100%) | 18/18 (100%) |
| Main thread: decisions made in topics | 6/6 (100%) | 6/6 (100%) |
| ...turns of those that read history | 0/6 | 0/6 |
| **Proposals (3)** |  |  |
| New-topic chances (a subject without a topic) that got one | – | – |
| Returns proposed into their topic | – | – |
| Noise proposals, per run | 0.0 | 0.0 |
| ...proposals on the merge message, per run | 0.0 | 0.0 |
| Duplicate topics proposed, per run | 0.0 | 0.0 |
| Wrong topic proposed, per run | 0.0 | 0.0 |
| Messages answered outside their topic, per run | 0.0 | 0.0 |
| Proposing turns: requests each | – | – |
| Proposing turns: prompt tokens each | – | – |
| Topics / subjects / sub-sessions at the end, per run | 3.0 | 3.2 |
| Subjects for main messages, per run (4) | 0.0 | 0.0 |
| Duplicate subjects, per run (4) | 0.0 | 0.0 |
| **Sub-sessions (5–6)** |  |  |
| Asked: one create_session with the task, no work in the parent | 6/6 (100%) | 6/6 (100%) |
| Finish notice passed on in the parent's answer | 0/6 (0%) | 2/6 (33%) |
| create_session calls nobody asked for, per run | 0.0 | 0.0 |
| Notices to the parent, per run | 12.0 | 1.0 |
| ...their tokens, per run (est.) | 439.2 | 55.0 |
| **Notes and memory** |  |  |
| Waiting work shows as an open item | 6/6 (100%) | 5/6 (83%) |
| Merge message: one topic / subject | 6/6 (100%) | 5/6 (83%) |
| Notes or subject writes, per run | 10.2 | 10.5 |
| ...alone in their response | 6/61 | 8/63 |
| Parallel steps run at the same time | 6/6 | 6/6 |
| Project memory has both facts | 9/9 (100%) | 9/9 (100%) |
| Memory revision conflicts (merged and saved again) | 3 (3) | 4 (4) |
| **History and cost** |  |  |
| History reads, per run | 3.2 | 3.8 |
| ...that reach another thread | 3.2 | 3.8 |
| Requests, per run | 38.8 | 40.0 |
| Prompt tokens, per run (k) | 149.2 | 156.8 |
| ...of the main thread or parent (k) | 61.5 | 68.5 |
| Output tokens, per run (k) | 8.7 | 9.7 |
| Cache reads, share of prompt tokens | 0% | 0% |
| Topic's first request: prefix shared with main, of tools + system (est. tokens) | 2550 of 2865 | 2552 of 2863 |
| Request errors | 0 | 0 |

## Checks one by one

Passes per check, over the reps.

### ollama-cloud/gemma4:31b

| Chat | # | Check | Message | 5 | 6 |
|---|---|---|---|---|---|
| jobs | 3 | memory | When I say a company rejected me, set the status to closed, … | 3/3 | 3/3 |
| jobs | 6 | asked session | I also want a pipeline that checks Zalando's careers page ev… | 3/3 | 3/3 |
| jobs | 6 | finish notice passed on | I also want a pipeline that checks Zalando's careers page ev… | 0/3 | 2/3 |
| jobs | 6 | waiting | I also want a pipeline that checks Zalando's careers page ev… | 3/3 | 2/3 |
| jobs | 9 | call | N26 rejected me. | 3/3 | 3/3 |
| jobs | 10 | cross | Which of my applications are still waiting, and is anything … | 3/3 | 0/3 |
| jobs | 11 | call | Make follow_up run on Fridays instead, and use 10 days inste… | 3/3 | 3/3 |
| jobs | 13 | memory | Remember this for the whole project: job alerts should only … | 3/3 | 3/3 |
| jobs | 14 | merge | follow_up and the board are really one thing for me: my Frid… | 3/3 | 2/3 |
| jobs | 15 | cross | What day does follow_up run now, and after how many days doe… | 3/3 | 3/3 |
| jobs | 15 | reach | What day does follow_up run now, and after how many days doe… | 3/3 | 3/3 |
| jobs | 16 | call | Here's the Zalando link: https://jobs.zalando.com/en/jobs?se… | 3/3 | 3/3 |
| jobs | 18 | follow-up | What happened with my N26 application, and why does it say c… | 3/3 | 3/3 |
| jobs | 19 | cross | What have we set up so far, and is anything still waiting on… | 3/3 | 2/3 |
| sqlite | 6 | call | In orders, user_id must point to users.id. | 3/3 | 3/3 |
| sqlite | 8 | asked session | I need stock counts too: a table inventory with sku and quan… | 3/3 | 3/3 |
| sqlite | 8 | finish notice passed on | I need stock counts too: a table inventory with sku and quan… | 0/3 | 0/3 |
| sqlite | 8 | waiting | I need stock counts too: a table inventory with sku and quan… | 3/3 | 3/3 |
| sqlite | 9 | cross | How do users and orders relate? | 0/3 | 0/3 |
| sqlite | 10 | call | Add a column discount to products, 0 by default. | 3/3 | 3/3 |
| sqlite | 11 | call | Change of mind on orders: use a plain INTEGER key after all.… | 3/3 | 3/3 |
| sqlite | 13 | memory | Remember this for the whole project: money is stored as whol… | 3/3 | 3/3 |
| sqlite | 14 | cross | Did we settle the key for orders? | 3/3 | 3/3 |
| sqlite | 14 | reach | Did we settle the key for orders? | 3/3 | 3/3 |
| sqlite | 15 | merge | Products and inventory are one thing for me: my catalog. Kee… | 3/3 | 3/3 |
| sqlite | 17 | follow-up | Why did we make email unique in users? | 3/3 | 3/3 |
| sqlite | 18 | follow-up | Is the restock check done? | 3/3 | 3/3 |
| sqlite | 19 | cross | What's still open everywhere? | 3/3 | 3/3 |
