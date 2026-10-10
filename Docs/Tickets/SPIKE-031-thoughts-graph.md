# SPIKE-031 — Thoughts-Graph: working out a how-to task as a graph of model calls
**Type:** Spike
**Status:** Done
**Gate:** 3

## Question
When a user asks how to do something, can the model work it out as a **graph of thoughts** instead of one answer: first a problem definition, then solutions as branches, then steps, critiques and merges under them? Each expansion is one model call, and every thought is a row in SQLite, so the whole search is kept and can be read, replayed or steered later.

Is the answer from the graph better than one plain call, and what does it cost? Does it work with the same prompts on every model (SPEC 1)?

The pattern (proposed on 2026-10-06):
- **A thought** has a `kind` (`problem`, `solution`, `step`, `critique`, `merge`), a short `title`, its `content`, a `weight` from 0 to 1, a one-sentence `rationale` for the weight, and a status (`open`, `done`, `dead_end`). The controller adds `expanded` once a thought has children.
- **Edges** say how a thought follows from its parent: `branches_to` (solution), `followed_by` (step), `critiques`, `merges`. A merge has more than one parent, which is what makes it a graph and not a tree.
- **The index:** every call sees the whole graph as titles only, one line per thought (`#5 [step] Profile the slow queries · w 0.80 · open`), indented under its first parent. With it the model knows which paths were already tried, and can avoid duplicates, critique an earlier thought or merge two of them. Only the path from #1 to the thought being expanded is sent in full.
- **Self-weighting, no judge:** the model weighs each new thought against the problem as #1 defines it, on an anchored scale (0.2 won't work, 0.5 workable with downsides, 0.8 strong, 1.0 solves it), and against its siblings. In the same call it may **reweight** thoughts already in the index, so later calls correct earlier weights with more context.
- **The model picks the next step:** each call also returns `next`, the thought to expand after this one, or `finish`. If `next` names something that can't be expanded, the controller takes the open thought with the highest weight.
- **Structured output by a tool:** the model answers by calling `record_thoughts`, whose parameters are the schema. Go checks the arguments with the app's schema checker and sends the error back once. JSON in the text instead of a call is taken and counted. This is SPIKE-030's *submit tool* method; `provider.Request` has no field to require the call, so the prompt asks for it.

## Method
1. **Define** (one call): the problem thought, #1.
2. **Expand** (one call each): the thought in focus gets new thoughts under it, weights may change, and the model names `next`. This repeats until the model says `finish` (ignored while there is no solution yet), no thought can be expanded, or the budget (`-max-thoughts`, default 16) is used. Thoughts at `-max-depth` (default 4) can't be expanded.
3. **Conclude** (one call): the index plus the two strongest paths in full, with the critiques under them, become the final answer, which ends with a `Used: #3, #7` line.
4. **Baseline** (one call): the same task, answered directly, for comparison.

Two modes:
- **siblings:** up to `-k` (default 3) thoughts per call, weighed against each other.
- **one:** exactly one thought per call, as in the original idea. The index is what keeps the next sibling different from the ones before.

## Tasks
Five synthetic how-to tasks (`-list`): a slow report page, moving a 200 GB database with at most 5 minutes down, a plan for the German B1 exam, a family move from Berlin to Munich (German), and starting a small home bakery in Tehran (Persian).

## Models
gemma4:31b (Ollama Cloud) and glm-4.5-flash (Z.ai), thinking off, confirmed with the user on 2026-10-06. They are listed in the spike's own `models.yaml` and use the second keys, `OLLAMA_API_KEY_II` and `Z_API_KEY_II`, so the runs don't clash with SPIKE-030, which runs at the same time on the first keys. One call at a time per provider.

## Setup
- **Harness:** `spikes/031-thoughts-graph/`. It calls the app's own provider registry and backends, and the app's schema checker. The fake provider tests the loop (`go test`).
- **Storage:** `results/graphs.db` with the tables `graphs`, `thoughts`, `edges`, `reweights` and `calls`. Every call is kept with its prompt, its reply, its tries and failures, its tokens and time.
- **Safety:** keys come from `.env` and stay in memory, inside the provider registry. Only synthetic text is sent. A quota error stops that model.

## Measures
- **Shape:** thoughts by kind, dead ends, depth reached, merges.
- **Index use:** duplicate titles (pairs with mostly the same words), how often `next` named a thought that could be expanded, early `finish`, the index size in the last call.
- **Weights:** the spread of sibling weights (mean standard deviation), and how many reweights the later calls made.
- **Output:** define and expand calls valid on the first try, after the one retry, or not at all; failures by kind (`no_tool_call`, `text_json`, `bad_json`, `schema`, `check`, `max_tokens`).
- **Cost:** calls, tokens and time per graph, compared with the baseline.
- **Answer:** the graph's answer against the baseline, read side by side in each graph's report.

## Done when
Both modes have run on every task on at least two models. There is a decision:
- whether a Thoughts-Graph mode is worth building, and for which requests
- siblings or one thought per call
- whether self-weighting with reweights is enough, or a separate judge call is needed
- what the controller decides and what the model decides (`next`, `finish`)
- what to keep for the app: the tables, the index format, how the turn inspector could show the graph

## Result
### Round 1 (2026-10-06)
20 graphs: 5 tasks × 2 modes × gemma4:31b and glm-4.5-flash, one rep, `-max-thoughts 16 -max-depth 4 -k 3`. Every graph finished; the per-graph reports are in `spikes/031-thoughts-graph/results/<task>/`, the table in `results/summary.md`, the analysis (`go run . -analyze`) in `results/analysis.txt`.

**Answer quality is not scored yet.** The graph's answers read well and are well structured. In 14 of 20 they are shorter than the one-call baseline, but length says nothing about quality. Until a blind comparison is done, nothing here shows that the graph gives better answers.

**What worked**
- **The tool answer is reliable.** 195 of 206 define and expand calls were valid on the first try and 8 after the retry; 3 failed both tries (graphs 3, 9 and 14). The usual failure was gemma answering in text instead of calling the tool (10 times, all in *one* mode; GLM did it once). Z.ai twice sent tool arguments that weren't valid JSON; the provider layer reports that as a transport error, and the next try worked.
- **The index is cheap.** It was at most 1,579 characters (about 400 tokens) at 16 thoughts.
- **Few duplicates.** 0 to 2 pairs of near-identical titles per graph. With no index-off run to compare, this doesn't show that the index is what prevents them.

**What didn't**
- ***One* mode collapses into a chain.** Every *one* graph has exactly one solution. The model always named the thought it had just added as `next`, never #1, so it never wrote a second solution. When the chain reached `-max-depth`, the controller took over (57 invalid `next` values named a thought at max depth, 3 a `done` one), but its fallback skips #1, so it couldn't branch either. This is partly a controller flaw, but the model's own choices were a chain.
- ***Siblings* mode branches only once.** Always exactly 3 solutions under #1, then steps. Of 3 solutions, typically one or two were never expanded (84 thoughts left `open`): the model kept following its top branch.
- **The model never stops.** In *siblings* mode `finish` never came; all 10 graphs used the whole budget. Only 2 *one* graphs ended on `finish` (5 and 8 thoughts, single chains).
- **The weights barely carry information.** Half of all thoughts are weighted 0.80–0.89, and the mean spread between siblings is 0.00–0.10. There was 1 reweight in 20 graphs, 1 dead end and 1 merge. Status `done` was used (41 times), `dead_end` almost never.
- **Weight against use is circular.** Thoughts weighted ≥ 0.90 were named on the answer's `Used:` line 88 % of the time, against 46–60 % below. But the conclude call sees the highest-weighted paths in full, so this partly measures the prompt, not the weights.

**Cost** (mean per graph, against the baseline answer)

| Model | Mode | Graph: tokens in / out, time | Baseline: tokens in / out, time |
|---|---|---|---|
| gemma4:31b | siblings | 9,671 / 2,750, 24 s | 87 / 1,065, 8 s |
| gemma4:31b | one | 22,361 / 3,368, 47 s | 87 / 1,044, 13 s |
| glm-4.5-flash | siblings | 10,224 / 5,835, 229 s | 76 / 1,865, 63 s |
| glm-4.5-flash | one | 28,134 / 12,998, 420 s | 76 / 2,227, 64 s |

*Siblings* costs about 3–4× the baseline in time, and far more in input tokens. *One* costs about twice *siblings* and gives worse graphs.

**Found in the provider layer** (not in the spike)
- **A false `too_large` error.** `provider.EstimateTokens` counts thinking text, but the Chat Completions backend doesn't send thinking back (`internal/provider/openai/chat.go`). After a long `reasoning_content`, the next request in the same conversation can be flagged as a silent truncation. It happened once here (graph 14: the provider read 1,730 tokens of an estimated 20,205). The agent loop sends thinking back too, so the app can hit this.
- **Thinking is probably not turned off for Z.ai.** The Chat Completions backend sends nothing when `Request.Thinking` is false. GLM's output was 2–4× gemma's and its calls took minutes, which looks like hidden reasoning. Not verified yet.

### Round 2 (2026-10-08): is the answer better?
Round 1 measured the graph's shape but not its answers. Round 2 scores them. The round-1 code and results are kept in `round1/` and `results/round1/`; round 2 writes to `results/round2/`.

**What changed**
- **Structured output by schema in the prompt.** Following SPIKE-030's decision, the answer is one JSON object in the text, and its schema closes the system prompt. There are no tools. Go checks the object against the schema and the graph rules, and sends the errors back once (at most 8, with their paths). Failures are counted as `no_json`, `schema`, `check` and `max_tokens`.
- **Builders:** gemini-3.5-flash-lite (native Gemini backend, `GEMINI_API_KEY`) and gemma4:31b (Ollama Cloud, `OLLAMA_API_KEY_II`), the reference models. Thinking is off in every graph call. GLM is dropped as a builder (round 1: minutes per call).
- **Four answers per task and builder:**
  - `index`: the round-1 *siblings* graph. The model sees the index, reweights and names `next`.
  - `path`: the same graph, but the model sees only the path to the thought it expands, as in Tree of Thoughts. There is no `next` and no reweight; the controller picks. Conclude is the same in both.
  - `plain`: one call, thinking off.
  - `thinking`: one call, thinking on. If no thinking comes back, it is kept but not valid.
  *one* mode is dropped (round 1: it always made a chain).
- **Controller** (`pick`, the fallback in `index` and the policy in `path`), breadth first:
  1. Back to #1 while fewer than 2 solutions are alive.
  2. Then a solution not yet expanded, highest weight first.
  3. Then the open thought with the highest weight.
  4. Then the best expandable thought.

  This fixes round 1's fallback, which could never go back to #1. The `index` prompt also asks, before going deeper, whether an unexpanded solution could be better. So `index` vs `path` tests the index *with* the model steering against no index with the controller steering. The two can't be separated here.
- **Judges:** gpt-oss:120b and nemotron-3-ultra on Ollama Cloud (`OLLAMA_API_KEY_II`), thinking on. Neither model builds an answer.
  - Pairs: `index` vs `plain`, `index` vs `thinking`, `index` vs `path`, `path` vs `plain`.
  - Each pair is judged in both orders. A verdict counts only when both orders agree; otherwise it is a *split*, which shows position bias.
  - The judge writes a reason, then a winner (A, B or tie) and a score from 1 to 10 for each answer. A winner with the lower score goes back once.
  - The prompt tells the judge not to prefer length or structure.
- **A person reads blind:** `-blind 10` writes 10 pairs (`results/round2/blind/pair-NN.md`). Each pairs the `index` answer with whichever baseline the judges scored higher, in a random order. The key is in `key.json`.
- **Storage:** two new tables. `answers` holds every answer of each kind, with its graph or its call. `judgments` holds one verdict per judge and order. Calls outside a graph (baselines, judges) are in `calls` too, with their model and the thinking that came back.

**Found while setting up**
- **Gemini "thinking on" barely thinks.** With thinking on, the native backend sends `thinking_summaries: auto` but no `thinking_level`, so the model's default applies. For flash-lite that looks like minimal: 1,064 output tokens with thought tokens against 1,013 with thinking off, the same 4 s, and no thought summaries. Gemini's first `thinking` answers were marked not valid. gemma's thinking baseline worked (about 1,900 characters of thinking). Fixed on 2026-10-09 by commit e4a7c49, which sends a thinking level when thinking is on. Gemini's thinking answers were then made again and judged.

**Result** (2026-10-08)

All 20 graphs reached 16 thoughts, and none ended on `finish`. gemma failed 5 expand calls on both tries:
- 4 times it wrapped its answer in `{"properties": {...}}`, copying the schema's shape, and did so again on the retry.
- Once its merge named a thought from the same call, which the check rejects. The check may be too strict.

Each failed call was skipped and the graph went on. The judges made 160 verdicts, all valid on the first try. That count includes Gemini's thinking pairs, judged on 2026-10-09 after the backend fix: commit e4a7c49 sends a thinking level, and the harness now also counts thought tokens from the usage. Gemini's new thinking answers used 800–1,230 thought tokens each. gpt-oss:120b took 2 s a verdict, nemotron-3-ultra 34 s. Details are in `results/round2/judges.md`, `analysis.txt` and `summary.md`.

| X vs Y | gpt-oss:120b: X / Y / split | nemotron-3-ultra: X / Y / split | Mean score X – Y |
|---|---|---|---|
| index vs plain | 1 / 9 / 0 | 1 / 8 / 1 | 7.2–7.5 vs 8.7–8.8 |
| index vs thinking | 0 / 8 / 2 | 0 / 7 / 3 | 7.1–7.5 vs 8.8 |
| index vs path | 4 / 2 / 4 | 4 / 4 / 2 | 8.1–8.2 vs 7.7–8.1 |
| path vs plain | 0 / 9 / 1 | 0 / 10 / 0 | 7.0 vs 8.8–9.0 |

- **One plain call beats both graphs**, by both judges, in 9 of 10 pairs or more. The one graph win is gemma on db-move.
- **The index doesn't clearly help.** Index against path is near even, with many splits. With gemma, index won 3 of 5 for both judges. With Gemini, path did as well or better.
- **The judges agree** on 33 of the 40 pairs both resolved. There is little position bias, apart from gpt-oss on index vs path (70 % first answer).
- **But length decides almost every verdict.** The longer answer won 69 of 80 verdicts for gpt-oss and 70 of 80 for nemotron, although the prompt says not to prefer length. The graph's answers are always shorter: 2.0–3.5k characters, against 3.3–5.1k for a plain call and 3.6–6.9k for thinking. Index and path are about the same length, and that is the comparison that comes out even.

  So these verdicts don't tell "the graph is worse" from "the graph is shorter". The conclude prompt (approach, steps, risks) gives compact answers, while a plain call writes a full guide.
- **Cost:** a graph takes 3–6× the time of a plain call; Gemini's times include waits for its 15-requests-a-minute free tier. It writes 2–6× the output tokens. Its input is 13–21k tokens for index and about 7k for path, against under 100 for a plain call.

So far there is no evidence that the graph gives better answers, and some that it gives worse ones for far more cost. That holds unless length is the only reason.

**The blind read** (2026-10-10). The user read some pairs. They found the answers good overall, but couldn't judge the German ones well, and suspected these tasks can be answered in one shot. Ten Claude readers then each read one pair blind, told not to prefer length. Their verdicts were matched to the key afterwards (`results/round2/blind/verdicts.md`).
- **The graph lost 10 of 10**, by a mean of 5.3 against 7.4: 5 of 5 against Gemini's plain call and 5 of 5 against gemma's thinking call.
- **The longer answer won every pair, but every reader said length didn't decide it.** The reasons are content.
  - The graph's answer is an outline where the one-call answer is a procedure: no commands for db-move, no budget split for bakery, no SQL or Go for report-page.
  - It is narrow where the task needs breadth. For umzug it covers booking movers but not the address registration, school or Kita.
  - On the plus side, it had fewer factual errors in 6 of 10 pairs and often the better risk section.

  So the length gap is not an artefact. It is the content the graph loses.

**Why: the answer is decided at the first expansion.** The user suspected that the model settles its answer at once and only writes it out as a path. The graphs support that. In all 10 `index` graphs:
- The solution weighted highest when #1 was first expanded is in the answer.
- It got most of the later thoughts in 8 of 10 graphs, tied in 1, and got fewer only in Gemini's report-page graph.
- In 6 of 10 graphs the answer uses only that one solution.

The rest of the search changes almost nothing. There were 20 reweights across the 10 `index` graphs, and 3 dead ends in 300 thoughts, so nothing is pruned. Then conclude, which sees only the two strongest paths, compresses 16 thoughts into a summary. A single call has no such step, and writes the full procedure it already knew.

These tasks have well-known answers that the model can write in one pass. Tree of Thoughts gained on tasks where one pass fails and a wrong step must be undone: Game of 24, crosswords, constrained planning. On open how-to tasks there is nothing to search: the first idea is good enough, and splitting it up only loses detail.

**Not done:** a goal checklist from #1 as the `finish` signal, relative ranking of siblings, and embeddings (path-to-goal cosine against the answer scores). None of them can fix an answer that was settled in the first call, so they were dropped.

## Decision
**Decided (2026-10-10):**
- **No Thoughts-Graph mode.** On how-to requests, one call beats the graph:
  - Two model judges: 160 verdicts in both orders.
  - Ten blind readers: the graph lost 10 of 10.

  The graph costs 3–6× the time and 2–6× the output tokens. The model settles its answer in the first expansion, and the graph only splits it up and loses detail. If anyone tries the idea again, it must be on tasks where one call is measurably wrong and a wrong step has to be undone, as in Tree of Thoughts. The first test there would be whether the graph corrects a wrong first answer.
- **Siblings or one per call:** siblings. *One* per call always made a chain (round 1).
- **Self-weighting is not enough** to steer a search. Half the weights sat at 0.80–0.89, and the first weight decided the answer in all 10 `index` graphs. There were 20 reweights in round 2, against 1 in round 1, but they changed nothing. A separate judge call would not fix this either, because the problem is that the tasks need no search.
- **Controller and model:** the model's `next` and `finish` are not reliable. `finish` never came in round 1's *siblings* graphs or round 2's `index` graphs, 20 in all. Only 2 single-chain *one* graphs in round 1 ended on it. A controller would have to own both the budget and the order. The breadth-first `pick` is the one to start from.
- **Kept for the app:**
  - The judge method for comparing two answers: both orders, *split* when the order changes the verdict, a reason before the verdict, and a check on the lengths. Length predicted 86–88 % of the model verdicts.
  - Blind reading with a key opened only afterwards.
  - The finding that a one-line instruction ("don't prefer length") doesn't stop model judges preferring length.
- **Not kept:** the tables, the index format and a turn-inspector view of the graph. Nothing in the app needs them.
- **Found in the provider layer and fixed:**
  - the false `too_large` after thinking (f1321fd)
  - Gemini not thinking when thinking is on (e4a7c49)
- **Recorded, not worked around:**
  - gemma sometimes wraps its JSON in `{"properties": ...}`, the shape of the schema (4 calls)
  - GLM's minutes-long calls (round 1)
