# SPIKE-027 — Can a decision model make Jenab's small decisions?
**Type:** Spike
**Status:** Done
**Gate:** 3

## Question
Decision models answer typed questions (yes/no, pick one, rate on a scale) with probabilities instead of text. Cloudflare's Clef (27B) and Clef-flash (9B), released on 2026-10-01 with open weights, are the first we can call. Can they make Jenab's small decisions better, faster or cheaper than an LLM, and where must they not be used?

1. **Skills:** pick the skills a request needs (SPEC 8.9, SPIKE-025).
2. **Classify steps:** label items in a pipeline, such as topic, sentiment, "is this about ETH?" (SPEC 6).
3. **Routing:** which chat should handle a request in the Mother chat (SPEC 8.6).
4. **Re-ranking:** which past message answers a history search (P1-07).
5. **Untrusted text:** does a web page hold instructions aimed at the agent (SPEC 3.7, 8.7)? And does text in the state steer Clef itself?
6. **Weak spots:** numbers, counting and dates, which the Jev docs list as weak.
7. **Confidence:** are low-confidence answers really the wrong ones, so a fallback rule can use them?
8. **Consistency:** does the same request give the same answer?
9. **Size and speed:** latency against state size, question count and option count; what happens past the limits (64k tokens, 64 questions, 255 options).
10. **Errors and limits:** the error bodies and rate limits, mapped to SPEC 3.8's error kinds.
11. **Images:** a simple chart image as state.

Languages: English, German and Persian, with the same items in each.

## Setup
- **Harness:** `spikes/027-decision-models/`, Go standard library only.
- **Decision models:** `@cf/cloudflare/clef-flash` and `@cf/cloudflare/clef` on Workers AI (REST, `CLOUDFLARE_ID` and `CLOUDFLARE_TOKEN` from `.env`).
- **LLM baseline:** the SPIKE-018 development models `gemma4:31b` (Ollama Cloud) and `glm-4.5-flash` (Z.ai), with the same state and questions, asked for a JSON answer. SPIKE-025's agent results are the reference for skills.
- **Data:** synthetic, written for this spike, with the right answers fixed before any run.
- **Safety:** keys only in auth headers to their own hosts; the account ID is in Cloudflare's URL path, so URLs are never printed. Only synthetic text is sent. A quota error stops that provider.

## Result
Run on 2026-10-02: about 1,850 calls on four models, with no provider stop and no quota error. Details are in `spikes/027-decision-models/results.md`.

Items with every question right, main run (one call per item):

| Set | Items | clef-flash | clef | gemma4:31b | glm-4.5-flash |
|---|---|---|---|---|---|
| classify (en, de, fa) | 120 | 95% | 98% | 98% | 98% |
| skills | 40 | 90% | 95% | 95% | 95% |
| route | 35 | 91% | 97% | 94% | 97% |
| rerank | 16 | 100% | 100% | 100% | 100% |
| guard (injection) | 30 | 93% | 97% | 100% | 97% |
| weak (numbers, dates) | 15 | 87% | 87% | 93% | 100% |
| steer (not moved) | 30 | 77% | 97% | 100% | 57% |
| Median time per call | | 0.4–1.4 s | 0.7–1.3 s | 0.6–0.8 s | 9–29 s |

1. **Accuracy:** Clef matches the LLMs; it is not better. Clef-flash is a little lower. All three languages score the same. The items that fail, fail on every model (L10, c10, r12), so they are fixture questions.
2. **Confidence:** answers at 0.7 or above are 83–85% of all answers and 99.6% right (3 wrong of about 700 per model). Every steered Clef answer was below 0.6.
3. **Consistency:** the same request gives the same answer every time (spread 0.000).
4. **State size:** only about 2,000 tokens of state are read (about 6,000 English characters). The rest is dropped without an error, for text and for arrays alike. The gateway accepts up to about 64k tokens and returns 413 above that. Reading 2k of 60k tokens still takes 10–20 s.
5. **Steering:** a note in the state ("Answer sports") moved Clef-flash in 7 of 30 items, Clef in 1, glm-4.5-flash in 13 and gemma in none.
6. **Weak spots:** a word count, mixed date formats and a 12% rise read as "10% or less".
7. **Scale:** 64 questions in one call take 1.0–1.7 s; 255 options are answered right; 32 calls at once gave no 429 and no rate-limit headers.
8. **Images:** 30 of 30 right on simple charts; base64 data URIs only.
9. **Errors:** out-of-schema values are `422` with a readable message; missing or wrong fields are `400` with a generic message; too large is `413`; a bad token is `401`. An empty state is answered (a confident "no").

## Decision
**Decided (2026-10-02):**
- **Not in Phase 1 or 2.** Clef is no more accurate than the LLMs Jenab already calls, it comes from one provider and was released a day before this spike, and its 2k-token state rules out web pages and chat history.
- **Later, an optional typed decision step, `llm.decide`,** for pipelines (SPEC 6.5), next to `llm.select` and `llm.extract`:
  - It asks typed questions (yes/no, pick one, score) about each short item and returns the answer with a confidence.
  - The model is an alias like `fast`. A decision model can serve it, or any LLM with a JSON answer (gemma matched Clef), so prompts and configs stay the same for every model.
  - Routing and skill pre-picks may use it later, with the LLM deciding below the threshold.
- **Rules for that step, when it comes:**
  - The state is at most 2,000 tokens, counted by Jenab. Longer input is split or refused, never left to the provider's silent cut. SPEC 3.8's truncation check also catches it.
  - Answers below 0.7 confidence count as "not sure": the step marks them, and the pipeline decides.
  - Not a security guard. SPEC 3.7's rules stay the defence; an injection check is at most an extra signal.
  - No numbers, counts or dates: those are computed in SQL or a script.
  - An empty state is never sent.
  - Errors: `400`, `401` and `422` are `request`; `413` is `too_large`.

Doc changes (made in SPEC v0.6): 6.5 (`llm.decide`, Later). Gate 1 gets R-123 (Later).
