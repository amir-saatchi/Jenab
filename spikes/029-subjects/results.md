# SPIKE-029 — Results

Round 1 is below; [round 2](#round-2) is at the end.

Round 1 ran on 2026-10-08 on Ollama Cloud (reserve key) with gemma4:31b and gpt-oss:120b: 2 scenarios × 4 conditions × 3 reps, 48 runs of 14 messages each. Four runs stopped during a 70-minute provider stall and were run again with `-resume`. The full tables, the checks one by one and a transcript per run are in [`results/2026-10-08_204530/`](results/2026-10-08_204530/results.md).

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

## Round 2

Round 2 tests a fix for finding 2. The rule (`v2` in `subjects.go`) and the `update_subject` description now say:
- a subject is the work itself, such as a table, a pipeline or a decision, named with a noun ("BTC price pipeline"), never after a step
- a change, a fix or a cancellation updates the work's subject by its id, and never creates a second one

It ran on 2026-10-09 on the same models, chats and settings, conditions 2 and 3 only, 3 reps: 24 runs, with no runs lost. The tables and transcripts are in [`results/2026-10-09_120732/`](results/2026-10-09_120732/results.md).

### Round 1 → round 2

| | gemma4:31b, tool | gemma4:31b, tool + index | gpt-oss:120b, tool | gpt-oss:120b, tool + index |
|---|---|---|---|---|
| **Changes that updated the existing subject** | 28% → **72%** | 61% → **76%** | 24% → 24% | 41% → **88%** |
| Duplicates, in 6 runs | 12 → 4 | 5 → **0** | 11 → 12 | 7 → 3 |
| Subjects at the end, per run | 7.5 → 6.8 | 6.5 → 5.8 | 7.2 → 8.0 | 6.7 → 6.7 |
| Work turns that updated a subject | 93% → 96% | 94% → **85%** | 91% → 91% | 83% → 91% |
| Subject status right | 78% → 93% | 96% → 81% | 78% → 67% | 74% → 81% |
| Follow-ups right | 100% → 100% | 100% → 100% | 83% → 83% | 92% → 83% |
| Calls that needed earlier turns right | 100% → 100% | 100% → 94% | 83% → 78% | 83% → 94% |
| Requests per run | 32.5 → 32.3 | 32.3 → 31.8 | 39.3 → 44.8 | 39.7 → 44.3 |
| Prompt tokens per run | 105k → 105k | 97k → 103k | 91k → 123k | 95k → 113k |

With the index, both models together updated the existing subject in 28 of 34 changes (82%). The target was 80%.

### Findings

8. **The new rule fixes the naming.** Subjects are now named after the work ("daily_btc pipeline", "applications table"), and changes update them. With the index, gemma made no duplicates in 6 runs, and gpt-oss went from 41% to 88% of changes. gemma got most of the gain even without the index (28% → 72%).

9. **gpt-oss still needs the index.** Without it, gpt-oss didn't look the subject up and created a new one for each change, as in round 1 (24%). It even named one after the rule's example, "BTC price pipeline", next to its own "daily_btc pipeline". So the index stays needed, and the rule's example should not come from a test chat's domain.

10. **gpt-oss now links the Zalando URL to its blocked subject.** In round 1 it put the link into the applications table in 3 runs out of 3. With the index it now finishes the Zalando pipeline in 2 of 3.

11. **The rule narrowed what counts as work.** "Such as a table, a pipeline or a decision" made both models skip two kinds of work turn:
    - a convention saved to project memory ("rejected means closed; remember this"): no subject in 10 of 12 runs, up from 6 of 12 in round 1
    - a row change ("N26 rejected me"): gemma with the index skipped it in 3 of 3

    That is all of gemma's drop in compliance with the index (94% → 85%), and most of its drop in correct statuses, since the status check needs a subject. Whether these turns need a subject is open. The convention is already in project memory, which the card shows. The row change belongs to the table's subject, so it would only update that subject's outcome.

12. **gpt-oss costs more.** It still sends every `update_subject` call alone (114 of 114). It also made more of them and read subjects more often, so it used about 5 more requests a run than in round 1 (+12–14%) and 19–35% more prompt tokens. gemma's cost didn't change.

### Decisions (proposed)

- **The rule: v2**, with the example taken out of any test chat's domain, and one more line saying a convention or a row change updates the subject it belongs to.
- **Subjects become the structure of the session notes, next to a short free-text part, and don't replace it.** With the index they reach the target, but 9–15% of work turns still get no subject, and gpt-oss still makes a few duplicates. A free-text part holds what a subject misses, and *Refresh memory* (3.5) merges duplicates.
- **The index in block 5: yes.** Round 2 shows it is needed for gpt-oss.
- **The app's check: no.** Round 2 needed none to reach the target.
- **SPIKE-028:** no change.
