# TASK-032 — Truncation check without tool definitions
**Type:** Task
**Status:** Open
**Gate:** 3
**Needs:** —

## Goal
Jenab's silent-truncation check (SPEC 3.8) compares the input tokens the provider read with Jenab's estimate, and the estimate counts the tool definitions. Some Ollama chat templates send the tools only when the last message is the user's; llama3.2:3b's is one. After a tool result the model reads about 600 tokens of an estimated 3,400, and Jenab stops the turn as `too_large`. In SPIKE-038, llama3.2:3b broke this way in all 8 basic runs.

## Scope
1. Leave tool definitions out of the estimate this check compares with: a real cut drops old messages (SPIKE-017), not tools. `EstimateTokens` for the context budget keeps them. The same for every kind.
2. Tests: a request with large tools and a small reported input is not flagged; a request whose messages were cut still is.
3. Run SPIKE-038's `local-basic` set on llama3.2:3b, 3 reps, recorded as it is.

## Done when
- llama3.2:3b's `local-basic` runs no longer break with `too_large`, and the truncation tests still pass.

## Result
