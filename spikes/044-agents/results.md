# SPIKE-044 results

Specialized agents: does Mother pick an agent's tool groups and write its
custom prompt well, and can the agent then do its job? 217 good runs on three
models, 2026-11-12/13. Every number below is in the generated tables:
[`results/all.md`](results/all.md) (every model), and one `results.md` per
model under `results/<model>/<stamp>/`, with a transcript per run.

## Runs

| Model | Runs | Of | Requests | Left |
|---|---|---|---|---|
| Gemini `gemini-3.5-flash-lite` | 89 | 91 | 1,281 | 2: the write scenario, rep 3, conditions 3 and 4, stopped by both keys' daily quota |
| NVIDIA `mistralai/mistral-large-4-1t-a52b` | 23 | 91 | 268 | 68: stopped by choice; the free endpoint took 10–25 minutes a run |
| Anthropic `claude-haiku-5-5` | 105 | 105 | 763 | none |

91 runs a model: 7 scenarios × conditions 1–4 × 3 reps, plus condition 0
(every group, the token control) once per scenario. Haiku ran condition 0 in
all 3 reps.

**Models and keys.**

- **Haiku 5.5 instead of gemma4:31b.** `OLLAMA_API_KEY` returned 402 (no
  credit) on 2026-11-12, and the user chose Haiku 5.5 for this spike. So the
  Ollama backend wasn't tested here.
  - Haiku cost about $0.30 at list prices (assuming cache reads cost 10% of
    input): 5.54M prompt tokens, 4.73M of them cache reads, and 346K output.
    A check run before the batch cost under $0.01.
- **Gemini's reserve keys.** `GEMINI_API_KEY`'s free quota was already used
  up that day, and `GEMINI_API_KEY_II` has no prepaid credit (402). With the
  user's OK, `GEMINI_API_KEY_III` ran rep 1 and the reruns, and `_IV` ran
  reps 2–3. Both reached the 500-a-day limit.
- **NVIDIA covers only part of the scenarios:** the reviewer, entry and docs
  scenarios in every condition, and ETH and research in most. Persian and
  write have no good NVIDIA run. Read NVIDIA's numbers as a sanity check, not
  a measure.

**Lost runs.** The PC went into Modern Standby twice (Kernel-Power 506/507):
on 2026-11-12 from 18:12 to 19:09, and again until 22:37 on 2026-11-13.
Every run in flight hit its time limit. Right after waking, Google and NVIDIA
returned HTML 403 pages for a minute or two. Those runs failed and ran again
with `-resume`; the report leaves failed runs out.

## Findings

### 1. Mother picks groups well; a narrowed chat never strays

- **Recall and precision.** In the asked and unasked conditions (2–4), the
  agent had 0.90–1.00 of the groups its job needs, and 0.72–0.83 of what it
  got was needed.
- **Forbidden groups.** An agent got a group it must not have in 3 of
  61 Gemini agents (5%), and none on Haiku or NVIDIA.
- **Every group.** Mother gave an agent every group in 4 of 61 Gemini agents,
  all for the ETH tracker, which needs 4 of the 6 groups anyway. Haiku and
  NVIDIA never did.
- **The usual over-picks** are defensible:
  - Views and pipelines for the docs writer, to read the configs it documents
    (every model, nearly every run).
  - Files for the reviewer, to keep review notes.
  - Read data for the researcher.
- **The usual misses:**
  - Memory for the researcher: Haiku left it out in all 9 runs, and the
    notes went into a file instead, which passed.
  - Read data for Gemini's reviewer: it picked `views_pipelines+files`, so it
    couldn't check the tables, and its task still passed.
- **The follow-up task passed in 95–100% of asked runs on every model.** The
  agent did the job with what Mother picked about as well as with the answer
  key (100%).
- **No agent ever called a tool outside its groups:** 0 refusals in 217 runs.
  A chat only sees the tools it has, so it doesn't try others. The refusal is
  a safety net for histories and odd models, not a path the reference models
  take.

### 2. Groups, not single tools

- **Groups and single tools picked equally well:** within 0.04 for recall and
  precision on every model.
- **But a list of single tools made Mother try tools she doesn't have.**
  - Every refusal in the spike was Mother's, and all came in condition 3:
    Gemini 7 (`read_config` ×5, `insert`, `request_schema_change`), Haiku 2
    (`insert`).
  - With `insert` named in `create_chat`'s schema, Haiku tried the write
    itself in 2 of 3 write runs: an `INSERT` through `query`, then `insert`.
    Once it wrote and deleted a junk file `cache/noop` as a probe.
  - With groups (conditions 2 and 4), Mother never tried a write.
- **Haiku's prompts repeated a core rule more often with single tools:** 65%
  against 24% with groups.
- **The refusal text worked when it fired.** After it, Mother handed the work
  to a new chat with `create_chat` (Haiku r1 and r3). In one run she told the
  user she can't write, and offered to start a chat.

### 3. Mother needs to read configs

- Mother has no `read_config`, though it only reads.
- On NVIDIA she started a whole chat to answer "What does the daily_btc
  pipeline do?" (the one plain-message creation in the spike).
- Gemini's 5 refused `read_config` calls came from the same gap.
- Reading a view's or pipeline's config, a run's status, or opening a page
  writes nothing, and Mother needs them to direct work.

### 4. Unasked: Mother delegates when work comes, never on a question

- **On a plain question** ("What does the daily_btc pipeline do?"), Mother
  created a chat in 1 of 46 runs, NVIDIA's case above.
- **When the user described a lasting job without saying "agent",** Mother
  created one at once in 67–80% of runs.
  - Haiku asked first in 3 of 18 ("Should I create it?") and created it when
    the work came.
  - Gemini created it when the task came in 4 of 18 runs.
- **The Persian digest needs only read data and files, which Mother has.**
  Haiku did it herself in all 3 runs, and passed. Gemini made an agent each
  time, twice without read data, and failed all 3.
- **The write with no agent yet (scenario 7):**
  - Delegated in every run with groups (conditions 2 and 4).
  - Mother never pretended the keywords were added: 0 of 16.

### 5. The custom prompt

- **Length.** Every prompt stayed under 500 tokens.
  - Haiku: 158–216 tokens on average.
  - NVIDIA: 197–303.
  - Gemini: 40–64, mostly one line ("Weekly Bitcoin News Summarizer: reads
    btc_news …").
- **What to report.** Gemini's short prompts seldom said what to report: 10–43%,
  against 95–100% on Haiku and NVIDIA. The tasks passed anyway, since the
  message carried the details.
- **Core rules.** Haiku and NVIDIA repeated a core rule in 20–67% of prompts
  ("Do not invent facts", "be brief"); Gemini never did. That's wasted
  tokens, not harm.
- **Language.** The custom prompt's language carries into the agent's output.
  - For the Persian request, Haiku wrote the prompt in English in all 6 asked
    runs. It named Persian as the output language in 5; once it said "Write a
    short summary in English".
  - Gemini wrote it in Persian in 3 of 6. With an English prompt, Gemini's
    agent wrote the digest in English and failed the check; with a Persian
    prompt, it wrote Persian and passed.
  - `create_chat`'s schema should say to write the role in the user's
    language.

### 6. Tokens

- **Tool definitions.** A narrowed agent sends about 1,500–2,100 fewer tokens
  of tool definitions per request than a chat with every tool: about 1,800
  of 3,650, half.
  - With every group, the usual gating (TASK-033) already drops about 500.
- **Measured prompt tokens per request,** the key's groups against every
  group, for the same task and role:

| Model | Every group | Key's groups | Saved |
|---|---|---|---|
| Gemini | 5,136 | 3,493 | 32% |
| NVIDIA | 5,057 | 4,392 | 13% (5 runs) |
| Haiku | 7,473 | 5,069 | 32% |

## What the spike doesn't show

- **The prompt measures are keyword checks** (`score.go`), a rough guide.
- **Persian has 3 runs a cell,** and NVIDIA none.
- **Ollama wasn't tested.**
- **Agents never hit the refusal,** so how a narrowed agent copes with a
  missing tool is untested on the reference models.
- **Every scenario is a single agent;** SPIKE-045 covers notices between
  agents.

## Proposed SPEC 8.6 text

In the ticket's Result section. The decision is the user's.
