# SPIKE-034 results

24 runs. Each cell adds up both chats and all reps.

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

## Checks one by one

Passes per check, over the reps.

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
