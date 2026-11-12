# SPIKE-034 round 2 results — visible sub-sessions

Run on 2026-10-27 on `ollama-cloud/gemma4:31b` and
`mistral/ministral-14b-latest`: 2 chats × conditions 5 and 6 × 3 reps per
model, 24 runs, none lost. ministral met Mistral's rate limit 42 times; the
app waited out each one. Gemini `gemini-3.5-flash-lite` ran on 2026-11-11
and 12: 12 runs, none lost (finding 8), with round 1's conditions 1, 2 and 4
in [`results/gemini/2026-11-11_132250/`](results/gemini/2026-11-11_132250/).
The other runs are in [`results/2026-10-27_022209/`](results/2026-10-27_022209/)
(gemma) and [`results/mistral/2026-10-27_022209/`](results/mistral/2026-10-27_022209/)
(ministral), with a transcript each. The tables below put them next to
round 1's runs of the same models ([`results.md`](results.md)): conditions
1 (one chat) and 2 (topics the user opens) are the baselines.

## Findings

1. **Asked, both models start a sub-session; unasked, almost never.** At the
   message that asks ("Do this in its own session: …"), both made one
   `create_session` call with the whole task in its message, and did no
   work in the parent, in 12 of 12 runs. In 24 runs there was one call
   nobody asked for: ministral, on the `jobs` merge message, started a
   "Friday Review" sub-session instead of joining the two. That sub-session
   then tried `create_session` twice and was refused (one level).
2. **With a notice after every sub-session turn (condition 5), the parent
   stays current.** Cross-topic questions in the parent: gemma 15/18, as in
   one chat (15/18) and against 9/18 with round 1's topics; ministral 17/18,
   against 15/18 in one chat and 6/18 with topics. Decisions made in a
   sub-session and asked in the parent: 6/6 on both, without reading
   history. The question round 1 failed, "is anything automated for them
   yet?" (`jobs` 10), passed 3/3 on both models: the follow_up
   sub-session's replies were in the parent's history as notices.
3. **The summary line (condition 6) doesn't keep it current.** Cross-topic
   questions: gemma 11/18, ministral 12/18; `jobs` 10 passed 0/3 and 1/3.
   gemma wrote the follow_up sub-session's notes at message 5 in every run
   and still answered "nothing is automated" at message 10: the parent's
   block 5, and the line in it, was built at its first turn (message 4), and
   the parent had too few turns for a cut before message 10. The line is
   only as current as the parent's last cut; a weaker model also writes few
   notes (ministral: 1.7 note writes a run), so its line is often empty.
4. **Work in a sub-session holds up as in round 1's topics, or better.**
   Follow-ups after cuts: gemma 9/9, ministral 6/9 (one chat: 8/9 each).
   Calls that changed earlier work: gemma 18/18, ministral 18/18 in
   condition 5 (one chat: 17/18 and 15/18). ministral acted on the Zalando
   link in its sub-session 3/3 in condition 5, against 0/3 in one chat.
   Waiting work as an open item: gemma 6/6 and 5/6; ministral 0/6 and 2/6,
   since it rarely writes notes, as in round 1.
5. **The parent rarely passes on the finish notice unasked.** The notice
   reaches the parent's next turn, which answers an unrelated `main`
   message; the answer named the sub-session's work in 0/6 and 2/6 runs
   (gemma) and 2/6 and 3/6 (ministral). In `jobs` the sub-session's first
   reply is "waiting for the link", so there is little to pass on. The user
   sees the sub-session anyway; Mother's finish turn (8.6) is the tested way
   to pass a reply on.
6. **Merging works with `merge_sessions`:** one sub-session after the merge
   message in 6/6 and 5/6 runs (gemma), 5/6 and 4/6 (ministral), against
   1/6 with round 1's `merge_topics`. Not looked into why.
7. **Cost.** Prompt tokens per run in condition 5: gemma 149k, ministral
   146k, against 135k and 122k in one chat (+11% and +20%); the parent's
   own share is 62k and 52k. The notices add about 440 (gemma) and 1,060
   (ministral) estimated tokens a run: 12 notices, 150 and 360 characters
   on average, the longest 952; none reached the 1,000-character cut.
   Requests per run stay about 40. Parallel turns in two sub-sessions gave
   3–4 memory revision conflicts per model, each merged and saved again;
   memory had both facts in every run (one chat on ministral: 5/9).
8. **Gemini: sub-sessions work, and both ways up score the same.** Asked,
   it made one `create_session` with the task and did no work in the
   parent in 12 of 12 runs, with no call nobody asked for;
   `merge_sessions` joined the two in 12 of 12. Cross-topic questions in
   the parent: 17/18 with notices and 17/18 with the summary line (one
   chat: 18/18); decisions made in a sub-session, 6/6 in both. Unlike
   gemma's, its parent searched the sub-sessions (3.7 and 5.3 history reads
   a run that reached another chat), so `jobs` 10 passed 2/3 with the
   summary line too. The two misses there: with notices, one parent said
   no automation was set up; with the summary line, one said "no pipelines
   have been created". The finish notice was passed on 1/6 with notices
   and 5/6 with the summary line, where it is the only notice the parent
   gets; ministral leaned the same way (2/6 and 3/6). Waiting work as an
   open item: 4/6 in both, against 6/6 with topics, mostly the Zalando
   sub-session in `jobs`. Prompt tokens per run against one chat's 195k:
   +45% with notices, +62% with the summary line; the parent's own 117k
   and 139k; the notices about 520 estimated tokens a run. Requests: 61 and
   65 a run, against 45.

## What this proposes (the user to decide)

- **Sub-sessions: yes,** started with `create_session` when the user asks,
  with the rule as tested: lasting work the user comes back to, plain
  questions answered in the parent, one level.
- **The way up: a notice after every sub-session turn,** cut to 1,000
  characters, written into the parent's next turn without starting one.
  Not the summary line.
- **The list line:** round 1's (`s2 · Orders table · 1 open · chat <id>`);
  the summary adds tokens and is stale.
- **SPEC 8.6** is the user's call: a separate `create_session` (a child
  tied to its parent: the list, the notices, one level) or `create_chat`
  beyond Mother.
- **Gemini doesn't change this.** On Gemini the two ways up score the same,
  because its parent searches the sub-sessions; the notice is what keeps a
  parent current on gemma and ministral, and the summary line costs more
  tokens on all three.

## Tables

102 runs. Each cell adds up both chats and all reps.

### ollama-cloud/gemma4:31b

| | 1 baseline | 2 topics, user opens | 3 topics, agent proposes | 4 subjects | 5 sub-sessions, notices | 6 sub-sessions, summary line |
|---|---|---|---|---|---|---|
| Runs (failed) | 6 (0) | 6 (0) | 6 (0) | 6 (0) | 6 (0) | 6 (0) |
| **Payoff** |  |  |  |  |  |  |
| Follow-ups after cuts | 8/9 (89%) | 9/9 (100%) | 9/9 (100%) | 9/9 (100%) | 9/9 (100%) | 9/9 (100%) |
| Cross-topic questions | 15/18 (83%) | 9/18 (50%) | 15/18 (83%) | 15/18 (83%) | 15/18 (83%) | 11/18 (61%) |
| Calls that changed earlier work | 17/18 (94%) | 18/18 (100%) | 18/18 (100%) | 18/18 (100%) | 18/18 (100%) | 18/18 (100%) |
| Main thread: decisions made in topics | 6/6 (100%) | 3/6 (50%) | 6/6 (100%) | 6/6 (100%) | 6/6 (100%) | 6/6 (100%) |
| ...turns of those that read history | 0/6 | 4/6 | 0/6 | 0/6 | 0/6 | 0/6 |
| **Proposals (3)** |  |  |  |  |  |  |
| New-topic chances (a subject without a topic) that got one | – | – | 1/70 (1%) | – | – | – |
| Returns proposed into their topic | – | – | 1/2 (50%) | – | – | – |
| Noise proposals, per run | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 |
| ...proposals on the merge message, per run | 0.0 | 0.0 | 1.0 | 0.0 | 0.0 | 0.0 |
| Duplicate topics proposed, per run | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 |
| Wrong topic proposed, per run | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 |
| Messages answered outside their topic, per run | 0.0 | 0.0 | 11.7 | 0.0 | 0.0 | 0.0 |
| Proposing turns: requests each | – | – | 3.0 | – | – | – |
| Proposing turns: prompt tokens each | – | – | 12404.5 | – | – | – |
| Topics / subjects / sub-sessions at the end, per run | 0.0 | 3.8 | 0.2 | 3.5 | 3.0 | 3.2 |
| Subjects for main messages, per run (4) | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 |
| Duplicate subjects, per run (4) | 0.0 | 0.0 | 0.0 | 0.5 | 0.0 | 0.0 |
| **Sub-sessions (5–6)** |  |  |  |  |  |  |
| Asked: one create_session with the task, no work in the parent | – | – | – | – | 6/6 (100%) | 6/6 (100%) |
| Finish notice passed on in the parent's answer | – | – | – | – | 0/6 (0%) | 2/6 (33%) |
| create_session calls nobody asked for, per run | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 |
| Notices to the parent, per run | 0.0 | 0.0 | 0.0 | 0.0 | 12.0 | 1.0 |
| ...their tokens, per run (est.) | 0.0 | 0.0 | 0.0 | 0.0 | 439.2 | 55.0 |
| **Notes and memory** |  |  |  |  |  |  |
| Waiting work shows as an open item | – | 5/6 (83%) | 6/6 (100%) | 6/6 (100%) | 6/6 (100%) | 5/6 (83%) |
| Merge message: one topic / subject | – | 1/6 (17%) | 0/6 (0%) | 3/6 (50%) | 6/6 (100%) | 5/6 (83%) |
| Notes or subject writes, per run | 0.0 | 9.3 | 8.5 | 9.7 | 10.2 | 10.5 |
| ...alone in their response | 0/0 | 8/56 | 8/51 | 9/58 | 6/61 | 8/63 |
| Parallel steps run at the same time | 0/6 | 6/6 | 0/6 | 0/6 | 6/6 | 6/6 |
| Project memory has both facts | 9/9 (100%) | 9/9 (100%) | 9/9 (100%) | 9/9 (100%) | 9/9 (100%) | 9/9 (100%) |
| Memory revision conflicts (merged and saved again) | 0 (0) | 3 (3) | 0 (0) | 0 (0) | 3 (3) | 4 (4) |
| **History and cost** |  |  |  |  |  |  |
| History reads, per run | 5.5 | 6.0 | 3.3 | 3.0 | 3.2 | 3.8 |
| ...that reach another thread | 0.0 | 6.0 | 1.7 | 0.0 | 3.2 | 3.8 |
| Requests, per run | 41.3 | 40.2 | 37.8 | 38.8 | 38.8 | 40.0 |
| Prompt tokens, per run (k) | 134.5 | 147.5 | 152.8 | 152.7 | 149.2 | 156.8 |
| ...of the main thread or parent (k) | 134.5 | 63.5 | 150.5 | 152.7 | 61.5 | 68.5 |
| Output tokens, per run (k) | 8.5 | 9.8 | 7.2 | 7.7 | 8.7 | 9.7 |
| Cache reads, share of prompt tokens | 0% | 0% | 0% | 0% | 0% | 0% |
| Topic's first request: prefix shared with main, of tools + system (est. tokens) | – | 2365 of 2718 | 2562 of 3129 | – | 2550 of 2865 | 2552 of 2863 |
| Request errors | 1 | 1 | 0 | 0 | 0 | 0 |

### mistral/ministral-14b-latest

| | 1 baseline | 2 topics, user opens | 3 topics, agent proposes | 4 subjects | 5 sub-sessions, notices | 6 sub-sessions, summary line |
|---|---|---|---|---|---|---|
| Runs (failed) | 6 (0) | 6 (0) | 6 (0) | 6 (0) | 6 (0) | 6 (0) |
| **Payoff** |  |  |  |  |  |  |
| Follow-ups after cuts | 8/9 (89%) | 6/9 (67%) | 4/9 (44%) | 5/9 (56%) | 6/9 (67%) | 6/9 (67%) |
| Cross-topic questions | 15/18 (83%) | 6/18 (33%) | 10/18 (56%) | 12/18 (67%) | 17/18 (94%) | 12/18 (67%) |
| Calls that changed earlier work | 15/18 (83%) | 15/18 (83%) | 15/18 (83%) | 16/18 (89%) | 18/18 (100%) | 16/18 (89%) |
| Main thread: decisions made in topics | 6/6 (100%) | 3/6 (50%) | 5/6 (83%) | 6/6 (100%) | 6/6 (100%) | 5/6 (83%) |
| ...turns of those that read history | 0/6 | 0/6 | 0/6 | 0/6 | 0/6 | 0/6 |
| **Proposals (3)** |  |  |  |  |  |  |
| New-topic chances (a subject without a topic) that got one | – | – | 0/72 (0%) | – | – | – |
| Returns proposed into their topic | – | – | – | – | – | – |
| Noise proposals, per run | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 |
| ...proposals on the merge message, per run | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 |
| Duplicate topics proposed, per run | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 |
| Wrong topic proposed, per run | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 |
| Messages answered outside their topic, per run | 0.0 | 0.0 | 12.0 | 0.0 | 0.0 | 0.0 |
| Proposing turns: requests each | – | – | – | – | – | – |
| Proposing turns: prompt tokens each | – | – | – | – | – | – |
| Topics / subjects / sub-sessions at the end, per run | 0.0 | 3.0 | 0.0 | 2.0 | 3.0 | 3.3 |
| Subjects for main messages, per run (4) | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 |
| Duplicate subjects, per run (4) | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 |
| **Sub-sessions (5–6)** |  |  |  |  |  |  |
| Asked: one create_session with the task, no work in the parent | – | – | – | – | 6/6 (100%) | 6/6 (100%) |
| Finish notice passed on in the parent's answer | – | – | – | – | 2/6 (33%) | 3/6 (50%) |
| create_session calls nobody asked for, per run | 0.0 | 0.0 | 0.0 | 0.0 | 0.0 | 0.2 |
| Notices to the parent, per run | 0.0 | 0.0 | 0.0 | 0.0 | 12.0 | 1.2 |
| ...their tokens, per run (est.) | 0.0 | 0.0 | 0.0 | 0.0 | 1063.0 | 128.5 |
| **Notes and memory** |  |  |  |  |  |  |
| Waiting work shows as an open item | – | 0/6 (0%) | 0/6 (0%) | 3/6 (50%) | 0/6 (0%) | 2/6 (33%) |
| Merge message: one topic / subject | – | 1/6 (17%) | 0/6 (0%) | 3/6 (50%) | 5/6 (83%) | 4/6 (67%) |
| Notes or subject writes, per run | 0.0 | 1.3 | 0.0 | 6.2 | 1.7 | 1.7 |
| ...alone in their response | 0/0 | 7/8 | 0/0 | 34/37 | 10/10 | 8/10 |
| Parallel steps run at the same time | 0/6 | 6/6 | 0/6 | 0/6 | 6/6 | 6/6 |
| Project memory has both facts | 5/9 (56%) | 9/9 (100%) | 4/9 (44%) | 3/9 (33%) | 9/9 (100%) | 9/9 (100%) |
| Memory revision conflicts (merged and saved again) | 1 (1) | 6 (6) | 0 (0) | 0 (0) | 4 (4) | 3 (3) |
| **History and cost** |  |  |  |  |  |  |
| History reads, per run | 3.2 | 0.2 | 1.3 | 1.0 | 0.0 | 0.8 |
| ...that reach another thread | 0.0 | 0.2 | 1.3 | 0.0 | 0.0 | 0.8 |
| Requests, per run | 40.3 | 35.8 | 36.8 | 44.7 | 39.2 | 42.5 |
| Prompt tokens, per run (k) | 121.8 | 118.3 | 142.5 | 161.2 | 146.0 | 163.3 |
| ...of the main thread or parent (k) | 121.8 | 36.8 | 142.5 | 161.2 | 51.7 | 69.5 |
| Output tokens, per run (k) | 2.8 | 3.3 | 2.8 | 3.2 | 4.3 | 4.3 |
| Cache reads, share of prompt tokens | 16% | 15% | 16% | 20% | 14% | 16% |
| Topic's first request: prefix shared with main, of tools + system (est. tokens) | – | 2371 of 2722 | – | – | 2575 of 2862 | 2569 of 2872 |
| Request errors | 24 | 19 | 20 | 27 | 18 | 24 |

### gemini/gemini-3.5-flash-lite

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

### Checks one by one

Passes per check, over the reps.

#### ollama-cloud/gemma4:31b

| Chat | # | Check | Message | 1 | 2 | 3 | 4 | 5 | 6 |
|---|---|---|---|---|---|---|---|---|---|
| jobs | 1 | new topic | Create a table applications with company, role, applied (a d… | – | – | 0/3 | – | – | – |
| jobs | 2 | new topic | Add these three: Zalando, Data Analyst, applied 2026-09-20, … | – | – | 0/3 | – | – | – |
| jobs | 3 | memory | When I say a company rejected me, set the status to closed, … | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 3 | new topic | When I say a company rejected me, set the status to closed, … | – | – | 0/3 | – | – | – |
| jobs | 5 | new topic | Add a pipeline follow_up that runs every Monday at 09:00 and… | – | – | 0/3 | – | – | – |
| jobs | 6 | asked session | I also want a pipeline that checks Zalando's careers page ev… | – | – | – | – | 3/3 | 3/3 |
| jobs | 6 | finish notice passed on | I also want a pipeline that checks Zalando's careers page ev… | – | – | – | – | 0/3 | 2/3 |
| jobs | 6 | new topic | I also want a pipeline that checks Zalando's careers page ev… | – | – | 1/3 | – | – | – |
| jobs | 6 | waiting | I also want a pipeline that checks Zalando's careers page ev… | – | 2/3 | 3/3 | 3/3 | 3/3 | 2/3 |
| jobs | 8 | new topic | Make a view board that groups the applications by status. | – | – | 0/3 | – | – | – |
| jobs | 9 | call | N26 rejected me. | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 9 | new topic | N26 rejected me. | – | – | 0/3 | – | – | – |
| jobs | 10 | cross | Which of my applications are still waiting, and is anything … | 3/3 | 0/3 | 3/3 | 3/3 | 3/3 | 0/3 |
| jobs | 11 | call | Make follow_up run on Fridays instead, and use 10 days inste… | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 11 | new topic | Make follow_up run on Fridays instead, and use 10 days inste… | – | – | 0/3 | – | – | – |
| jobs | 12 | new topic | Remember this for the whole project: I only apply to jobs in… | – | – | 0/3 | – | – | – |
| jobs | 13 | memory | Remember this for the whole project: job alerts should only … | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 13 | new topic | Remember this for the whole project: job alerts should only … | – | – | 0/2 | – | – | – |
| jobs | 13 | return | Remember this for the whole project: job alerts should only … | – | – | 0/1 | – | – | – |
| jobs | 14 | merge | follow_up and the board are really one thing for me: my Frid… | – | 1/3 | 0/3 | 3/3 | 3/3 | 2/3 |
| jobs | 15 | cross | What day does follow_up run now, and after how many days doe… | 3/3 | 2/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 15 | reach | What day does follow_up run now, and after how many days doe… | 3/3 | 2/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 16 | call | Here's the Zalando link: https://jobs.zalando.com/en/jobs?se… | 2/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 16 | new topic | Here's the Zalando link: https://jobs.zalando.com/en/jobs?se… | – | – | 0/2 | – | – | – |
| jobs | 16 | return | Here's the Zalando link: https://jobs.zalando.com/en/jobs?se… | – | – | 1/1 | – | – | – |
| jobs | 18 | follow-up | What happened with my N26 application, and why does it say c… | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 18 | new topic | What happened with my N26 application, and why does it say c… | – | – | 0/3 | – | – | – |
| jobs | 19 | cross | What have we set up so far, and is anything still waiting on… | 3/3 | 3/3 | 3/3 | 2/3 | 3/3 | 2/3 |
| sqlite | 1 | new topic | Create a table users with id, email, name and created_at. id… | – | – | 0/3 | – | – | – |
| sqlite | 2 | new topic | Add three users: Ana (ana@example.org), Ben (ben@example.org… | – | – | 0/3 | – | – | – |
| sqlite | 3 | new topic | Next, a table orders with id, user_id, total and created_at.… | – | – | 0/3 | – | – | – |
| sqlite | 5 | new topic | Create a table products with sku, name and price. sku is the… | – | – | 0/3 | – | – | – |
| sqlite | 6 | call | In orders, user_id must point to users.id. | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 6 | new topic | In orders, user_id must point to users.id. | – | – | 0/3 | – | – | – |
| sqlite | 8 | asked session | I need stock counts too: a table inventory with sku and quan… | – | – | – | – | 3/3 | 3/3 |
| sqlite | 8 | finish notice passed on | I need stock counts too: a table inventory with sku and quan… | – | – | – | – | 0/3 | 0/3 |
| sqlite | 8 | new topic | I need stock counts too: a table inventory with sku and quan… | – | – | 0/3 | – | – | – |
| sqlite | 8 | waiting | I need stock counts too: a table inventory with sku and quan… | – | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 9 | cross | How do users and orders relate? | 0/3 | 1/3 | 0/3 | 1/3 | 0/3 | 0/3 |
| sqlite | 10 | call | Add a column discount to products, 0 by default. | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 10 | new topic | Add a column discount to products, 0 by default. | – | – | 0/3 | – | – | – |
| sqlite | 11 | call | Change of mind on orders: use a plain INTEGER key after all.… | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 11 | new topic | Change of mind on orders: use a plain INTEGER key after all.… | – | – | 0/3 | – | – | – |
| sqlite | 12 | new topic | Remember this for the whole project: all timestamps are stor… | – | – | 0/3 | – | – | – |
| sqlite | 13 | memory | Remember this for the whole project: money is stored as whol… | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 13 | new topic | Remember this for the whole project: money is stored as whol… | – | – | 0/3 | – | – | – |
| sqlite | 14 | cross | Did we settle the key for orders? | 3/3 | 1/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 14 | reach | Did we settle the key for orders? | 3/3 | 1/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 15 | merge | Products and inventory are one thing for me: my catalog. Kee… | – | 0/3 | 0/3 | 0/3 | 3/3 | 3/3 |
| sqlite | 17 | follow-up | Why did we make email unique in users? | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 17 | new topic | Why did we make email unique in users? | – | – | 0/3 | – | – | – |
| sqlite | 18 | follow-up | Is the restock check done? | 2/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 18 | new topic | Is the restock check done? | – | – | 0/3 | – | – | – |
| sqlite | 19 | cross | What's still open everywhere? | 3/3 | 2/3 | 3/3 | 3/3 | 3/3 | 3/3 |

#### mistral/ministral-14b-latest

| Chat | # | Check | Message | 1 | 2 | 3 | 4 | 5 | 6 |
|---|---|---|---|---|---|---|---|---|---|
| jobs | 1 | new topic | Create a table applications with company, role, applied (a d… | – | – | 0/3 | – | – | – |
| jobs | 2 | new topic | Add these three: Zalando, Data Analyst, applied 2026-09-20, … | – | – | 0/3 | – | – | – |
| jobs | 3 | memory | When I say a company rejected me, set the status to closed, … | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 3 | new topic | When I say a company rejected me, set the status to closed, … | – | – | 0/3 | – | – | – |
| jobs | 5 | new topic | Add a pipeline follow_up that runs every Monday at 09:00 and… | – | – | 0/3 | – | – | – |
| jobs | 6 | asked session | I also want a pipeline that checks Zalando's careers page ev… | – | – | – | – | 3/3 | 3/3 |
| jobs | 6 | finish notice passed on | I also want a pipeline that checks Zalando's careers page ev… | – | – | – | – | 0/3 | 1/3 |
| jobs | 6 | new topic | I also want a pipeline that checks Zalando's careers page ev… | – | – | 0/3 | – | – | – |
| jobs | 6 | waiting | I also want a pipeline that checks Zalando's careers page ev… | – | 0/3 | 0/3 | 3/3 | 0/3 | 1/3 |
| jobs | 8 | new topic | Make a view board that groups the applications by status. | – | – | 0/3 | – | – | – |
| jobs | 9 | call | N26 rejected me. | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 9 | new topic | N26 rejected me. | – | – | 0/3 | – | – | – |
| jobs | 10 | cross | Which of my applications are still waiting, and is anything … | 2/3 | 0/3 | 0/3 | 1/3 | 3/3 | 1/3 |
| jobs | 11 | call | Make follow_up run on Fridays instead, and use 10 days inste… | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 11 | new topic | Make follow_up run on Fridays instead, and use 10 days inste… | – | – | 0/3 | – | – | – |
| jobs | 12 | new topic | Remember this for the whole project: I only apply to jobs in… | – | – | 0/3 | – | – | – |
| jobs | 13 | memory | Remember this for the whole project: job alerts should only … | 1/3 | 3/3 | 1/3 | 0/3 | 3/3 | 3/3 |
| jobs | 13 | new topic | Remember this for the whole project: job alerts should only … | – | – | 0/3 | – | – | – |
| jobs | 14 | merge | follow_up and the board are really one thing for me: my Frid… | – | 1/3 | 0/3 | 0/3 | 2/3 | 1/3 |
| jobs | 15 | cross | What day does follow_up run now, and after how many days doe… | 3/3 | 0/3 | 2/3 | 3/3 | 3/3 | 2/3 |
| jobs | 15 | reach | What day does follow_up run now, and after how many days doe… | 3/3 | 0/3 | 2/3 | 3/3 | 3/3 | 2/3 |
| jobs | 16 | call | Here's the Zalando link: https://jobs.zalando.com/en/jobs?se… | 0/3 | 0/3 | 0/3 | 1/3 | 3/3 | 1/3 |
| jobs | 16 | new topic | Here's the Zalando link: https://jobs.zalando.com/en/jobs?se… | – | – | 0/3 | – | – | – |
| jobs | 18 | follow-up | What happened with my N26 application, and why does it say c… | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 18 | new topic | What happened with my N26 application, and why does it say c… | – | – | 0/3 | – | – | – |
| jobs | 19 | cross | What have we set up so far, and is anything still waiting on… | 1/3 | 0/3 | 0/3 | 0/3 | 2/3 | 2/3 |
| sqlite | 1 | new topic | Create a table users with id, email, name and created_at. id… | – | – | 0/3 | – | – | – |
| sqlite | 2 | new topic | Add three users: Ana (ana@example.org), Ben (ben@example.org… | – | – | 0/3 | – | – | – |
| sqlite | 3 | new topic | Next, a table orders with id, user_id, total and created_at.… | – | – | 0/3 | – | – | – |
| sqlite | 5 | new topic | Create a table products with sku, name and price. sku is the… | – | – | 0/3 | – | – | – |
| sqlite | 6 | call | In orders, user_id must point to users.id. | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 6 | new topic | In orders, user_id must point to users.id. | – | – | 0/3 | – | – | – |
| sqlite | 8 | asked session | I need stock counts too: a table inventory with sku and quan… | – | – | – | – | 3/3 | 3/3 |
| sqlite | 8 | finish notice passed on | I need stock counts too: a table inventory with sku and quan… | – | – | – | – | 2/3 | 2/3 |
| sqlite | 8 | new topic | I need stock counts too: a table inventory with sku and quan… | – | – | 0/3 | – | – | – |
| sqlite | 8 | waiting | I need stock counts too: a table inventory with sku and quan… | – | 0/3 | 0/3 | 0/3 | 0/3 | 1/3 |
| sqlite | 9 | cross | How do users and orders relate? | 3/3 | 3/3 | 2/3 | 3/3 | 3/3 | 1/3 |
| sqlite | 10 | call | Add a column discount to products, 0 by default. | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 10 | new topic | Add a column discount to products, 0 by default. | – | – | 0/3 | – | – | – |
| sqlite | 11 | call | Change of mind on orders: use a plain INTEGER key after all.… | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 11 | new topic | Change of mind on orders: use a plain INTEGER key after all.… | – | – | 0/3 | – | – | – |
| sqlite | 12 | new topic | Remember this for the whole project: all timestamps are stor… | – | – | 0/3 | – | – | – |
| sqlite | 13 | memory | Remember this for the whole project: money is stored as whol… | 1/3 | 3/3 | 0/3 | 0/3 | 3/3 | 3/3 |
| sqlite | 13 | new topic | Remember this for the whole project: money is stored as whol… | – | – | 0/3 | – | – | – |
| sqlite | 14 | cross | Did we settle the key for orders? | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 14 | reach | Did we settle the key for orders? | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 15 | merge | Products and inventory are one thing for me: my catalog. Kee… | – | 0/3 | 0/3 | 3/3 | 3/3 | 3/3 |
| sqlite | 17 | follow-up | Why did we make email unique in users? | 3/3 | 3/3 | 1/3 | 1/3 | 3/3 | 3/3 |
| sqlite | 17 | new topic | Why did we make email unique in users? | – | – | 0/3 | – | – | – |
| sqlite | 18 | follow-up | Is the restock check done? | 2/3 | 0/3 | 0/3 | 1/3 | 0/3 | 0/3 |
| sqlite | 18 | new topic | Is the restock check done? | – | – | 0/3 | – | – | – |
| sqlite | 19 | cross | What's still open everywhere? | 3/3 | 0/3 | 3/3 | 2/3 | 3/3 | 3/3 |

#### gemini/gemini-3.5-flash-lite

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
