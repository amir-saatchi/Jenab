# SPIKE-037 results: a live answer, and earlier turns as answers only

Run on 2026-10-26 on the app's real orchestrator, with the three chosen
models: Ollama Cloud `gemma4:31b` (reference), Ollama Cloud `gpt-oss:120b`,
Mistral `ministral-14b-latest`. Same prompts and tools for every model.

- **Part A:** 8 tests × 2 conditions × 3 models × 3 reps = **144 runs**, 0 errors
  (`results/2026-10-26_182459_A/`).
- **Part B:** 2 chats × 3 conditions × 3 models × 3 reps = **54 runs**, 0 errors
  (`results/2026-10-26_183034_B/`).
- Every run has a transcript in its folder's `transcripts/`; the full
  tables are in each folder's `results.md`.
- No model ran out of quota. Mistral returned some 429s (30 requests a
  minute); the provider layer retried them, and no run failed.

## How the spike differs from the ticket

- **Finish notices are simulated.** The orchestrator has no finish turns
  yet (8.3 is specified but not built). The harness sends
  `[task t_N finished: …]` as a user message once the chat is idle, in each
  test's order. So the model sees what a finish turn would show it, but
  the notice is a user-role message, not a notice part.
- **Part A's hook only changes the prompt.** `Deps.LiveAnswer` swaps item 4
  of the rule. The request stays append-only, as the design says. Where
  each answer lands in the user's view (in place, or below) is computed
  by the scorer using the design's rules.
- **Part B uses one window setting per chat.** Running both settings on
  both chats would double the 54 runs. So the jobs chat uses today's sizes
  (8 turns: 2 cuts a run) and the trip chat a low `history_max_turns`
  (min 2, max 4: 4 cuts a run).
- **Pages are small.** The fake pages are 30–200 tokens, while real
  previews reach 1,500. Part B's window condition is therefore cheaper here
  than it would be in real use, which understates what answers-only saves.
- **Not enforced:** the 3-task background limit (`a_table` starts 4).
- **gpt-oss runs on Ollama Cloud.** Groq's free tier (8,000 tokens a
  minute) is too small for these runs.
- **Ollama reports no cache reads,** so cache figures come from Mistral only.

## Part A: the live answer

| cond | model | finish turns | whole | rest | earlier facts kept | dropped / turn | view right | last answer right | answers in view | output / update | cache read |
|---|---|---|---|---|---|---|---|---|---|---|---|
| stacked | ministral-14b | 69 | 67% | 33% | 75% | 0.7 | 95% | 83% | 4.1 | 199 | 35% |
| stacked | gemma4:31b | 72 | 79% | 21% | 95% | 0.0 | 97% | 97% | 4.1 | 357 | – |
| stacked | gpt-oss:120b | 72 | 79% | 21% | 93% | 0.2 | 97% | 95% | 4.1 | 284 | – |
| live | ministral-14b | 69 | 86% | 14% | 94% | 0.1 | 97% | 97% | 1.4 | 252 | 31% |
| live | gemma4:31b | 72 | 86% | 14% | 99% | 0.0 | 98% | 98% | 1.4 | 388 | – |
| live | gpt-oss:120b | 69 | 91% | 9% | 98% | 0.1 | 98% | 96% | 1.4 | 318 | – |

**How the measures are defined:**
- **whole:** the finish answer keeps at least 60% of the facts that
  arrived before it. For the first finish turn, which has no earlier
  facts, it must repeat at least 40% of the first answer's longer words.
- **rest:** neither of those.
- **appended** (the old answer word for word, plus a new block) and
  **none** (no text): neither happened in any run, so they're not in the
  table.
- **view right:** facts of every task that arrived, found anywhere in what
  the user reads at the end.
- **last answer right:** the same facts, found in the last answer alone.
- **requests per update:** 1.0 on both Ollama models; 1.5 (stacked) and
  1.7 (live) on Mistral.

**What the numbers say:**

1. **All three models write the whole updated answer.** With the live rule,
   86–91% of finish turns are whole, and they keep 94–99% of the earlier
   facts.
   - No model left a finish turn empty or appended a block to the old
     answer.
   - The "rest" turns are mostly the first arrival in a text answer, where
     the model gives the new part with a short recap.
2. **Today's "give the rest" rule is mostly not followed** when the user
   asks to update a table or a recommendation as results come in.
   - Under the stacked rule, 67–79% of finish turns were already whole.
     So the stacked view repeats the answer, growing, 4 times. In
     `a_table`, that is 5 near-identical tables.
   - Where models did follow the rule, the last stacked answer lacks
     earlier facts (ministral: last answer right 83%).
3. **Clearer for the user, at a modest cost.**
   - The user reads 1.4 answers instead of 4.1, with the same or better
     facts (97–98% vs 95–97%).
   - The cost is output tokens per update: +9% on gemma, +12% on gpt-oss,
     +27% on ministral.
   - Mistral's cache reads go from 35% to 31%.
   - Requests per update don't change.
4. **Edge cases:**
   - **Failed task:** reported in 17/17 runs, and still in the live answer
     at the end.
   - **Empty result:** all 9 live runs rewrote the answer whole, and said
     that Spiegel added nothing. None wiped the answer down to a one-line
     note, which a live answer would turn into lost content. The "no text
     leaves the answer as it is" rule never fired.
   - **Two tasks in one notice:** both results were in one answer in 9/9
     live runs and 8/9 stacked runs.
   - **Late message** ("I can only go on Saturdays"): taken into account in
     8/9 live answers and in every later finish answer.
   - **Persian:** the language held in every revision (lowest share of
     Persian letters outside tables 77–81%; stacked ministral fell below
     50% once).
5. **Risk: a whole rewrite can add detail.** In `a_text`, ministral's live
   answer added "10–12 °C in the morning" and "last train around 20:00",
   which no result said (`a_text_live_mistral-ministral-14b-latest_r1`,
   turn 4). The stacked answers do this too, but every live revision is a
   fresh chance to do it.

**A gap in "in place only while latest" (`a_late`):**
- Once the user sends a message, every later arrival answers below. In
  `a_late` that left 4 answers, three of them the same growing table
  (`a_late_live_ollama-cloud-gemma4-31b_r1`, turns 3–5).
- The scorer's view counts the Saturday-closed shops as missing. In fact
  the models dropped them because the user said Saturday, which is right,
  so `a_late`'s "view right" (88–100%) understates them.

Notable transcripts (in `results/2026-10-26_182459_A/transcripts/`):
- `a_table_stacked_ollama-cloud-gemma4-31b_r1`: the stacked rule ignored;
  five copies of the growing table.
- `a_text_stacked_mistral-ministral-14b-latest_r1` vs
  `a_text_live_mistral-ministral-14b-latest_r1`, turn 4:
  - stacked: "Train Connections Update" plus a recap;
  - live: the whole plan (weather, trail, trains, itinerary, packing).
- `a_empty_live_ollama-cloud-gemma4-31b_r1`, turn 3: the empty Spiegel
  result folded into a whole answer.

## Part B: earlier turns

**Prompt tokens** (as the provider reports them) **per request and per
run**, with the change against today's window:

| chat | model | window / req | answers / req | trace / req | window / run | answers / run | trace / run |
|---|---|---|---|---|---|---|---|
| jobs | ministral-14b | 2,943 | 2,233 (−24%) | 2,369 (−20%) | 106k | 83k (−22%) | 81k (−24%) |
| jobs | gemma4:31b | 3,281 | 3,713 (+13%) | 2,316 (−29%) | 113k | 202k (+79%) | 83k (−26%) |
| jobs | gpt-oss:120b | 2,741 | 2,421 (−12%) | 2,168 (−21%) | 109k | 147k (+35%) | 102k (−6%) |
| trip (low) | ministral-14b | 3,009 | 2,061 (−32%) | 2,120 (−30%) | 104k | 87k (−17%) | 75k (−28%) |
| trip (low) | gemma4:31b | 3,079 | 3,579 (+16%) | 2,830 (−8%) | 119k | 228k (+91%) | 157k (+32%) |
| trip (low) | gpt-oss:120b | 2,495 | 3,392 (+36%) | 2,104 (−16%) | 87k | 192k (+120%) | 98k (+12%) |

**The history part of a turn's first request** (estimated tokens):

| chat | window | answers | trace |
|---|---|---|---|
| jobs | 1,300–2,700 | 490–850 | 630–920 |
| trip | 1,100–1,900 | 260–640 | 400–630 |

**Follow-ups and side effects:**

| chat | cond | follow-ups right (mini / gemma / gpt-oss) | history reads / run | repeated calls / run | wrong names / run | fake calls | trace tokens (whole run) |
|---|---|---|---|---|---|---|---|
| jobs | window | 59% / 78% / 81% | 6.7 / 3.3 / 4.0 | 0.3 / 1.3 / 1.3 | 3.0 / 0 / 0 | 0 | – |
| jobs | answers | 56% / 89% / 78% | 4.7 / 17 / 11 | 0.7 / 4.3 / 7.3 | 1.7 / 0 / 0 | 0 | – |
| jobs | trace | 48% / 70% / 85% | 5.3 / 6.0 / 10 | 0.3 / 1.3 / 1.3 | 1.7 / 0 / 0 | 10 / 8 / 0 | 181–217 |
| trip | window | 67% / 85% / 78% | 5.0 / 4.0 / 3.0 | 0.7 / 3.0 / 1.7 | 0 / 0 / 0 | 0 | – |
| trip | answers | 33% / 93% / 78% | 9.3 / 24 / 18 | 0 / 4.0 / 3.7 | 2.3 / 0 / 0.3 | 0 | – |
| trip | trace | 48% / 81% / 78% | 6.3 / 14 / 8.7 | 0 / 3.3 / 1.7 | 0 / 0 / 0 | 0 / 2 / 0 | 127–216 |

The per-kind table is in `results/2026-10-26_183034_B/results.md`.

**What the numbers say:**

3. **How much smaller:**
   - The history part of a request shrinks by about 50–85% with answers
     only, and 40–75% with the trace.
   - Per request, prompt tokens fall by 12–32% where the model doesn't read
     more. The system prompt and tools are a fixed ~1,300 tokens, and the
     pages here are small.
   - **Per run, answers only costs more on 2 of 3 models.** gemma and
     gpt-oss make up for the missing chain by reading it back: 11–24
     `read_messages`/`search_history` calls a run, against 3–4 with the
     window. They also redo work: gpt-oss fetched pages and searched again
     (7.3 repeated calls a run in jobs).
     - On those two models, the per-run prompt tokens went up 35–120%.
     - Only ministral saved per run, by 17–22%, and it is also the model
       that lost the most (below).
4. **What the model loses with answers only:**
   - **ministral** guesses what it can't see:
     - URLs (`https://www.alfamapatiohotel.com/`, a 404);
     - refs (`ref_56789`);
     - task IDs (`task_5XZ8LmQ7p`, `fado_search_2027`).

     On the trip chat it falls to 33% right (detail, source and ref
     follow-ups 0/3 each). It also claimed booking.com and tripadvisor as
     sources.
   - **gemma and gpt-oss** keep or improve accuracy (gemma 89–93%), but
     only by reading more, as above.
   - **Tool use doesn't drop.** Non-read tool calls per later turn are the
     same as with the window (0.6–1.4), so no model stopped using tools.
   - **A shared failure, in every condition:** the complete list of sources
     in message 13 is almost always wrong. After a cut, models forget the
     background tasks (LinkedIn/StepStone, flights/fado). gpt-oss also made
     up a "€45,000–55,000" range for Zalando (`b_jobs_window_…gpt-oss…_r1`,
     turn 16).
5. **The trace wins back some of it, but it causes fake calls.**
   - **What it wins back:**
     - It brings reads and repeated work back to roughly window levels.
       Ministral's made-up refs and URLs mostly go away on the trip chat:
       0 wrong names, against 2.3 with answers only.
     - It cuts per-run tokens on jobs for all three models (−6 to −26%).
     - On the trip chat it saves only on ministral (−28%). gemma and
       gpt-oss still read a lot there, and their per-run tokens went up
       (+12 to +32%).
     - It costs ~10–15 tokens per earlier turn (127–217 for the whole run).
   - **But models imitate it.**
     - gemma wrote `[search_ref …]` / `[read_ref …]` as text instead of
       calling the tool, in all 3 jobs reps, at "How big is the team for
       the N26 role?". It then answered that the page doesn't say
       (`b_jobs_trace_ollama-cloud-gemma4-31b_r1`, turn 8). Detail
       follow-ups fell to 0/3.
     - ministral wrote `[subagent …]` as text instead of starting the
       background task (jobs r2 and r3, turns 4 and 9), then polled made-up
       task IDs.
     - Fake calls appeared only in the trace condition: 2 of 3 models,
       20 lines in 8 of 18 runs.
   - **Net:** follow-ups right with the trace are below the window for
     ministral (48% vs 59/67%) and for gemma on jobs (70% vs 78%), and
     about level for gpt-oss.
- **Cache (Mistral):** 29–30% of prompt tokens read from cache with the
  window, 33–36% with answers or trace. Reshaping the just-finished turn
  did not hurt the cached prefix.

## Proposed decisions

**The live answer: yes.**
- All three models write the whole updated answer (86–91% of finish
  turns, 94–99% of earlier facts kept). The user reads one answer instead
  of four, at +9–27% output tokens per update.
- **Rules:**
  - **In place only while latest:** keep it, with one change. After a
    user message, the first arrival answers below, and later arrivals of
    the same request update *that* answer while it is the latest, instead
    of each going below. `a_late` shows the cost of the plain rule: three
    copies of one table.
  - **Revisions:** keep every revision, and let the user see earlier ones.
    Whole rewrites occasionally drop a fact (0.0–0.1 per turn) or add an
    unsupported detail (`a_text`), and the earlier revision is how the
    user checks.
  - **A finish turn with no text:** keep "leaves the answer as it is" as a
    safety rule; no model produced one.
  - **A very short finish answer:** worth adding a rule, as a live answer
    would replace a full one with it. Proposal: if a finish answer keeps
    less than about half of the current answer, it goes below as a note
    instead of replacing it. It never happened here, so it can wait for
    real use.

**8.3's prompt rule: the whole answer**, as `liveRule` in
`internal/agent/live.go` words it, together with the live answer.
- Without the live answer, the "rest" rule is mostly ignored for update
  requests (67–79% whole), and stacking repeats the answer.
- SPIKE-023's 65→88% gain came from a different measure (a finish turn
  that answers at all). Here every finish turn answered under both rules.

**Earlier turns: keep today's window. Adopt neither answers only nor
answers and trace now.**
- Answers only shrinks the history, but 2 of 3 models read it back,
  costing more per run. The third (ministral) guesses names and loses
  accuracy.
- The trace fixes most of the guessing, but in its bracketed form it
  causes fake tool calls on 2 of 3 models, including the reference
  model. Its savings are small at these page sizes.
- **"From which turn on":** if it's tried again, apply answers + trace
  only to turns behind a cut, in place of today's stubs, and keep the
  window's turns whole.

**The trace line's format, if it is tried again:**
- Don't put it in the assistant's own message in tool-call syntax. Put it
  in a separate system block ("Steps in earlier turns"), worded as a
  record, not as something to write: `turn 4: fetched
  jobs.example.com/n26-risk (ref …)`.
- Drop the `[full steps: read_messages(…)]` line; `search_history` and
  `read_messages` are already in the tools.
- Retest with real page sizes (1,500-token previews), where the window is
  much larger.

**3.6's window sizes: no change.**
- Today's window kept history at 1–3k tokens per request in these chats,
  far under `history_max_tokens` (24,000). Cuts came from
  `history_max_turns` alone.
- Nothing here argues for retuning while the window stays as it is.

## Open points for the user

- Approve the live answer and the reversed rule item. If yes, the
  orchestrator still needs real finish turns (8.3), and the UI
  (revisions, "updating") needs a mockup.
- Approve the refinement to "in place only while latest": later arrivals
  update the answer below.
- Part B: keep the window, or run a round 2 with the trace as a system
  block, only behind a cut, and with real page sizes.
