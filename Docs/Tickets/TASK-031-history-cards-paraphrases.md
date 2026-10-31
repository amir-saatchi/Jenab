# TASK-031 — History cards for paraphrased follow-ups
**Type:** Task
**Status:** Open (later)
**Gate:** 3
**Needs:** TASK-027

## Goal
History cards (TASK-027) find the earlier turn when a follow-up repeats its words, but mostly miss it when the follow-up says the same thing in other words (2 of 8 offline). Find a rule that catches more paraphrases without more false cards, the same for every language.

## Today
- The rule in SPEC 3.6: idf over shared words, with common words not counted.
- TASK-027's ideas, not tested live: a shared word in at most 1 of the chat's turns ("anchor"); a bigger margin. Semantic recall (SPIKE-033) is the other route, once embeddings are set up.

## Scope
1. **Dev set:** TASK-027's set (24 follow-ups, 8 chats, English and Persian), SPIKE-028's H1–H5, and TASK-027's gemma and ministral transcripts.
2. **A new held-out set,** written and committed before any scoring: 6–8 long chats with 3 follow-ups each, with German besides English and Persian; hard controls that repeat an old turn's content words but need only the window; paraphrases that share at most one rare word with their turn. Checks that read JSON-escaped arguments, and that check the answer, not only the call.
3. Run on at least two models, since gemma and ministral took wrong cards differently.

## Done when
- On the held-out set, more paraphrases get the needed card than with TASK-027's rule, repeats stay as good, and controls come to no harm on the reference models.

## Result
