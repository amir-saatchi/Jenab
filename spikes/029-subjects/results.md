# SPIKE-029 — Round 1 results

Run on 2026-10-08 on Ollama Cloud (reserve key) with gemma4:31b and gpt-oss:120b: 2 scenarios × 4 conditions × 3 reps, 48 runs of 14 messages each. Four runs stopped during a 70-minute provider stall and were run again with `-resume`. The full tables, the checks one by one and a transcript per run are in [`results/2026-10-08_204530/`](results/2026-10-08_204530/results.md).

Each cell is small: 12 follow-ups, 18 calls and 27 status checks per model and condition. A difference of one or two is noise.

## Summary

| | gemma4:31b | gpt-oss:120b |
|---|---|---|
| Work turns that updated a subject on their own (conditions 2–4) | 87–94% | 83–93% |
| ...with the app's check | 98% | 96% |
| Subjects made for plain questions, in 18 runs | 2 (both after the app's check) | 1 |
| Changes that updated the existing subject, not a new one: tool → tool + index | 28% → 61% | 24% → 41% |
| Subject status right: tool → tool + index | 78% → 96% | 78% → 74% |
| Follow-ups right: baseline → tool + index | 75% → 100% | 83% → 92% |
| Requests per run: baseline → tool + index | 30.2 → 32.3 (+7%) | 31.5 → 39.7 (+26%) |
| Prompt tokens per run: baseline → tool + index | 75k → 97k (+29%) | 60k → 95k (+60%) |

## Findings

1. **The agent keeps subjects when a prompt rule tells it to.** Both models updated a subject after 83–94% of work turns without help, and almost never made one for a plain question. The rule's "skip it for plain questions" works.

2. **Subjects become a log of actions, not a list of subjects.** This is the main failure. Both models name a subject after the step they just took ("Create daily_btc pipeline", "Update daily_btc schedule", "Handle N26 rejection"), so a later change to the same work gets a new subject. Without the index, only about a quarter of changes updated the existing subject. A few were exact duplicates ("Zalando jobs pipeline" twice).

3. **The index in the context helps, but doesn't fix it.** With the index, gemma updated the existing subject in 61% of changes instead of 28%, and its statuses were right in 96% of checks. gpt-oss went from 24% to 41%, and its statuses didn't improve: a new subject left the old one with its old status. In the jobs chat, gpt-oss didn't link the Zalando URL to the subject "Zalando data jobs pipeline · blocked", even though that line was in its context. In 3 runs out of 3 it put the link into the applications table instead. So `get_subject` alone, without the index, is not enough. The index is needed, but it isn't sufficient.

4. **Follow-ups got better.** In the baseline, gemma answered "why did we stop collecting weekend prices?" by restating the schedule, without the reason. Asked "what did we set up?", it listed only what was still in the window. With subjects, it answered all 12 follow-ups right, in all three subject conditions. gpt-oss went from 83% to 92%. Calls that needed earlier turns ("do the same for ETH", "N26 rejected me") were right as often without subjects as with them. The project card and `get_config` already cover them.

5. **Subjects don't replace history search.** Reads of `search_history` and `read_messages` didn't go down with subjects (gemma 3.2 per run in the baseline, 3.7–4.7 with subjects). The models use both.

6. **The app's check adds little.** It closed most of the remaining gaps (gemma 87% → 98%), but it doesn't touch the main failure (2), and it fired on writes nobody asked for. In two runs gemma made a view while answering a plain question, and the check then made it create a subject for that view. In these runs the check was a separate message; in the app it would be one more request in the turn.

7. **The cost depends on how a model places its calls.** gemma usually sent `update_subject` together with its last real call, so subjects cost it about 2 requests a run. gpt-oss sent every `update_subject` call alone in its response (155 of 155), so each one cost a request: about 8 more a run, +26%. Prompt tokens grew 29–60%. That comes from the extra requests, the two tool schemas and the index; Ollama Cloud reports no cache reads. Per SPEC 1 there is no prompt for one model: this is recorded as gpt-oss behaviour.

## Decisions (proposed)

- **Subjects: yes, as the structure of the session notes** (3.4), with the index in block 5. The decision on whether they replace the notes' free text waits for round 2, which tests the fix for finding 2.
- **The index in the context: yes.** Without it, the model rarely finds the subject to update.
- **The app's check: no, for now.** It gains little, adds noise and costs a request. It may be looked at again if round 2 shows low compliance.
- ***Refresh memory* writes subjects:** untested here. Its merge step (3.5) is a natural place to merge duplicates, which is the main failure, so round 2 or the Phase 2 work should try it.
- **SPIKE-028:** nothing here changes its plan. A request list per turn and subjects across turns still fit together.

## Round 2 (proposed)

Change the rule and the tool description so a subject is the work itself, named with a noun ("BTC price tracking"), and so a change to existing work updates its subject: "never create a subject for a change to work that already has one". Then run conditions 2 and 3 again on the same models, with 3 reps. Success: at least 80% of changes update the existing subject, with no loss in compliance.
