# SPIKE-034 results

4 runs. Each cell adds up both chats and all reps.

## ollama-cloud/gemma4:31b

| | 1 baseline | 2 topics, user opens | 3 topics, agent proposes | 4 subjects |
|---|---|---|---|---|
| Runs (failed) | 1 (0) | 1 (0) | 1 (0) | 1 (0) |
| **Payoff** |  |  |  |  |
| Follow-ups after cuts | 2/2 (100%) | 2/2 (100%) | 2/2 (100%) | 2/2 (100%) |
| Cross-topic questions | 2/3 (67%) | 2/3 (67%) | 2/3 (67%) | 2/3 (67%) |
| Calls that changed earlier work | 2/3 (67%) | 2/3 (67%) | 2/3 (67%) | 2/3 (67%) |
| Main thread: decisions made in topics | 1/1 (100%) | 1/1 (100%) | 1/1 (100%) | 1/1 (100%) |
| ...turns of those that read history | 0/1 | 1/1 | 0/1 | 0/1 |
| **Proposals (3)** |  |  |  |  |
| New subjects proposed | – | – | 1/12 (8%) | – |
| Returns proposed into their topic | – | – | – | – |
| Noise proposals, per run | 0.0 | 0.0 | 0.0 | 0.0 |
| ...proposals on the merge message, per run | 0.0 | 0.0 | 1.0 | 0.0 |
| Duplicate topics proposed, per run | 0.0 | 0.0 | 0.0 | 0.0 |
| Wrong topic proposed, per run | 0.0 | 0.0 | 0.0 | 0.0 |
| Messages answered outside their topic, per run | 0.0 | 0.0 | 11.0 | 0.0 |
| Proposing turns: requests each | – | – | 3.0 | – |
| Proposing turns: prompt tokens each | – | – | 12427.0 | – |
| Topics / subjects at the end, per run | 0.0 | 4.0 | 1.0 | 4.0 |
| Subjects for main messages, per run (4) | 0.0 | 0.0 | 0.0 | 0.0 |
| Duplicate subjects, per run (4) | 0.0 | 0.0 | 0.0 | 1.0 |
| **Notes and memory** |  |  |  |  |
| Waiting work shows as an open item | – | 1/1 (100%) | 0/1 (0%) | 1/1 (100%) |
| Merge message: one topic / subject | – | 0/1 (0%) | 0/1 (0%) | 0/1 (0%) |
| Notes or subject writes, per run | 0.0 | 8.0 | 8.0 | 9.0 |
| ...alone in their response | 0/0 | 0/8 | 0/8 | 2/9 |
| Parallel steps run at the same time | 0/1 | 1/1 | 0/1 | 0/1 |
| Project memory has both facts | 1/1 (100%) | 1/1 (100%) | 1/1 (100%) | 1/1 (100%) |
| Memory revision conflicts (merged and saved again) | 0 (0) | 1 (1) | 0 (0) | 0 (0) |
| **History and cost** |  |  |  |  |
| History reads, per run | 7.0 | 4.0 | 3.0 | 3.0 |
| ...that reach another thread | 0.0 | 4.0 | 3.0 | 0.0 |
| Requests, per run | 40.0 | 34.0 | 36.0 | 38.0 |
| Prompt tokens, per run (k) | 114.0 | 113.0 | 139.0 | 132.0 |
| Output tokens, per run (k) | 8.0 | 7.0 | 8.0 | 6.0 |
| Cache reads, share of prompt tokens | 0% | 0% | 0% | 0% |
| Topic's first request: prefix shared with main, of tools + system (est. tokens) | – | 2349 of 2700 | 2526 of 3191 | – |
| Request errors | 0 | 0 | 0 | 0 |

## Checks one by one

Passes per check, over the reps.

### ollama-cloud/gemma4:31b

| Chat | # | Check | Message | 1 | 2 | 3 | 4 |
|---|---|---|---|---|---|---|---|
| sqlite | 1 | new topic | Create a table users with id, email, name and created_at. id… | – | – | 0/1 | – |
| sqlite | 2 | new topic | Add three users: Ana (ana@example.org), Ben (ben@example.org… | – | – | 0/1 | – |
| sqlite | 3 | new topic | Next, a table orders with id, user_id, total and created_at.… | – | – | 0/1 | – |
| sqlite | 5 | new topic | Create a table products with sku, name and price. sku is the… | – | – | 0/1 | – |
| sqlite | 6 | call | In orders, user_id must point to users.id. | 0/1 | 0/1 | 0/1 | 0/1 |
| sqlite | 6 | new topic | In orders, user_id must point to users.id. | – | – | 0/1 | – |
| sqlite | 8 | new topic | I need stock counts too: a table inventory with sku and quan… | – | – | 1/1 | – |
| sqlite | 8 | waiting | I need stock counts too: a table inventory with sku and quan… | – | 1/1 | 0/1 | 1/1 |
| sqlite | 9 | cross | How do users and orders relate? | 0/1 | 0/1 | 0/1 | 0/1 |
| sqlite | 10 | call | Add a column discount to products, 0 by default. | 1/1 | 1/1 | 1/1 | 1/1 |
| sqlite | 10 | new topic | Add a column discount to products, 0 by default. | – | – | 0/1 | – |
| sqlite | 11 | call | Change of mind on orders: use a plain INTEGER key after all.… | 1/1 | 1/1 | 1/1 | 1/1 |
| sqlite | 11 | new topic | Change of mind on orders: use a plain INTEGER key after all.… | – | – | 0/1 | – |
| sqlite | 12 | new topic | Remember this for the whole project: all timestamps are stor… | – | – | 0/1 | – |
| sqlite | 13 | memory | Remember this for the whole project: money is stored as whol… | 1/1 | 1/1 | 1/1 | 1/1 |
| sqlite | 13 | new topic | Remember this for the whole project: money is stored as whol… | – | – | 0/1 | – |
| sqlite | 14 | cross | Did we settle the key for orders? | 1/1 | 1/1 | 1/1 | 1/1 |
| sqlite | 14 | reach | Did we settle the key for orders? | 1/1 | 1/1 | 1/1 | 1/1 |
| sqlite | 15 | merge | Products and inventory are one thing for me: my catalog. Kee… | – | 0/1 | 0/1 | 0/1 |
| sqlite | 17 | follow-up | Why did we make email unique in users? | 1/1 | 1/1 | 1/1 | 1/1 |
| sqlite | 17 | new topic | Why did we make email unique in users? | – | – | 0/1 | – |
| sqlite | 18 | follow-up | Is the restock check done? | 1/1 | 1/1 | 1/1 | 1/1 |
| sqlite | 18 | new topic | Is the restock check done? | – | – | 0/1 | – |
| sqlite | 19 | cross | What's still open everywhere? | 1/1 | 1/1 | 1/1 | 1/1 |
