# SPIKE-023 — Requests with several parts, and background work
**Type:** Spike
**Status:** Done
**Gate:** 3

## Question
Does one model-neutral prompt rule (SPEC 8.3) make the test models handle requests with several parts well, when one part needs slow work? And does the Mother chat delegate well (SPEC 8.6)?

The rule:
1. Answer the parts that need no tools first.
2. Start slow work in the background.
3. Say what is still running.
4. Give the rest in the finish turn, without repeating what was already answered.

The tests:
1. **Quick part first:** a request with a quick question and a slow task (a background subagent or a pipeline run with a fake 30 s delay). Is the quick part answered before the slow part finishes?
2. **Finish turn:** when the finish notice arrives, does the agent give the rest without repeating itself or starting the work again?
3. **Message during a turn:** the user writes while a tool is running ("also add ETH", or "stop, use EUR"). Does the agent take it into account at the next step?
4. **No false background work:** a simple request must not be split, and must not start background tasks.
5. **Cost:** tokens per scenario with and without the rule.
6. **Delegation (Mother chat):** Mother sees a chat list with a *Reviewer* and an *ETH tracker* chat, and gets mixed requests. Does it:
   - send work to the right existing chat, with enough context
   - create a new role chat only for a lasting job, and use a subagent for one-off work
   - answer itself when no delegation is needed
   - pass on the reply from the finish notice without repeating it or sending the work again

Models: the SPIKE-018 development models, with the same prompt for all (SPEC 1, model-neutral). The harness is a first version of the scenario runner ([TASK-001](TASK-001-scenario-runner.md)), with fake tools.

## Done when
Each test has pass rates per model with and without the rule. There is a decision on the prompt rule, on the SPEC 8.3 design (finish notices, system-started turns, messages during a turn) and on Mother's delegation guidance (8.6).

## Result
Run on 2026-09-28/29 with the scenario runner in `spikes/023-multi-part-requests/` (`results.md`, raw runs in `results/runs.jsonl`).

- **Models:** nemotron-3-ultra and gemma4:31b (Ollama Cloud), glm-4.5-flash (Z.ai). Same prompts for all; temperature 0.2. No provider errors.
- **Runs:** 20 scenarios × 2 conditions (with and without the rule) × 3 models × 2 reps = 240 runs, plus 22 follow-up runs with a second delegation wording. n = 2 per cell, so single cells are anecdotes; the totals are the useful figures.
- **Harness:** fake tools with virtual time (e.g. a pipeline run takes 30 s, `send_to_chat` 45 s), finish notices that start system turns, mid-turn messages at the next step boundary, a cap of 8 requests per turn.
- **Budget:** 2.09M prompt tokens.

Pass rates, with the rule / without it:

| Test | nemotron | gemma | glm | All |
|---|---|---|---|---|
| 1 Quick part first | 8/8 / 6/8 | 8/8 / 8/8 | 8/8 / 8/8 | 100 % / 92 % |
| 2 Finish turn | 7/8 / 4/7 | 8/8 / 8/8 | 6/8 / 3/8 | 88 % / 65 % |
| 3 Message during a turn | 8/8 / 8/8 | 8/8 / 8/8 | 8/8 / 8/8 | 100 % / 100 % |
| 4 No false background work | 8/8 / 8/8 | 8/8 / 8/8 | 8/8 / 6/8 | 100 % / 92 % |
| 5 Mean prompt tokens per run | 7,484 / 9,034 | 4,923 / 4,679 | 7,199 / 10,682 | −20 % |
| 6 Delegation (Mother) | 15/16 / 12/16 | 16/16 / 16/16 | 11/16 / 11/16 | 88 % / 81 % |

Findings:
- **The rule helps and pays for itself.** It never made a test worse in total. It costs about 125 prompt tokens per request, but it avoids runaway runs, so the mean cost per run fell by 20 %. Example: glm on "watch gold daily" used 42.9k tokens without it and 9.4k with it.
- **gemma passes almost everything either way.** nemotron and glm gain the most: in the finish turn (test 2), and glm in not treating a definition as research (test 4).
- **Finish notices and system-started turns worked** on all three models.
- **Messages during a turn** were delivered at the next step and acted on in every run (ETH added, EUR price, 7 days instead of 30). The boundary has to be after the tool results of the current response: the OpenAI message order has no place for a user message between a tool call and its result.
- **Main failure: polling.** In 10 of 240 runs, a model called `run_status` / `task_status` 6–7 times in a row and filled the turn's request cap. In 7 of them, the last poll saw the finished status, so no notice came, and the turn ended before the model could answer: the user never got the rest. Without those 7 runs, test 2 is 21/22 with the rule and 15/18 without it. This failure is partly made by the harness (the cap, and dropping the notice after a poll).
- **Mother's delegation guidance v1** fixed the choice between one-off and lasting work: a background subagent for 20 pages (nemotron and glm fetched them one by one in 1 of 2 runs without it), a new role chat for "watch gold daily", and no delegation for simple questions (100 %).
- **Routing to an existing role chat stays weak** for nemotron and glm, with or without guidance. Asked to "check the new pipeline", they read it and reviewed it themselves instead of sending it to *Reviewer* (right target: nemotron 5/6, glm 3/6, gemma 6/6). A stronger wording ("even if you could do it yourself") did not help (nemotron 0/2, glm 1/2).

## Decision
**Decided (2026-09-29):**
1. **Prompt rule (SPEC 8.3):** adopt it as written, for every model.
2. **Polling:**
   - The `run_status` and `task_status` descriptions say that a notice follows, so there is no need to poll.
   - From the second status call for the same ID in a turn, the orchestrator answers "still running; a notice will follow" without checking again.
   - The finish notice is dropped only if the model has already answered after seeing the final status.
3. **Request cap per turn:** new setting `turn_max_requests` (25 for user turns, 8 for turns the app starts; starting values, tuned with the benchmark). When a turn reaches it, one last request goes out without tools, so the user always gets an answer.
4. **Messages during a turn:** they join after the tool results of the current response. Written into SPEC 8.3.
5. **Notices due together** share one system-started turn.
6. **Mother (SPEC 8.6):** use delegation guidance v1 (in `prompts/delegation.md`). Don't add more wording for routing to an existing role chat. Mother doing a job itself is a cost issue, not a safety one. Recheck before Phase 5 with a frontier model and with each chat's full role in the chat list.
7. **Scenario runner (TASK-001):** the SPIKE-023 scenarios become a standing set; `no_repeat` and a polling check become standing assertions.

Doc changes (made in SPEC v0.6): 7.6 (`turn_max_requests`, `system_turn_max_requests`), 8.1 (status tool descriptions), 8.3 (no longer a draft), 8.6 (delegation guidance v1).
