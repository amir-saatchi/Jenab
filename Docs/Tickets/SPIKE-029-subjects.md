# SPIKE-029 — Subjects: a chat's work, kept as objects
**Type:** Spike
**Status:** In progress (rounds 1–3 done; decisions proposed)
**Gate:** 3

## Question
After a cut (SPEC 3.6), the agent only sees `This chat has 38 earlier turns. Use search_history or read_messages.` To find what it already did, it has to guess search words. Session notes (3.4) are free text, and *Refresh memory* (3.5) only runs when the user starts it.

Can the agent keep a list of **subjects** next to the real history: one object per piece of work, with its status and outcome? Does it keep the list up to date when a prompt rule tells it to, and does the list help it answer follow-ups after a cut?

The pattern (proposed on 2026-10-06):
- **A subject** has `id`, `subject` (a short title), `status` (`open`, `in_progress`, `blocked`, `done`, `dropped`), `outcome` (what was done or decided, and why), `open` (questions still open), `source` (message IDs in the history), `updated_at` and `revision`. Subjects are stored per chat in `chats.db`. The history itself is never rewritten.
- **`update_subject(id?, subject, status, outcome, open, expected_revision)`** creates a subject when `id` is missing and updates it otherwise. It follows the same revision rule as memory (3.3).
- **`get_subject(id)`** returns the whole object. Its `source` IDs lead to `read_messages`.
- **The index**, one line per subject (`id · subject · status`), sits in block 5 next to the session notes (3.1). Like blocks 2–5, it is rewritten only at a cut, so the provider's cache holds. Updates made between cuts come back as a short notice in the current turn (3.1).
- **When to call it:** the agent calls `update_subject` in the same response as its last real tool call, not after the answer. A call made after the answer would cost one more request every turn.
- **The app's check:** if a turn wrote changes (they are in the change log) but updated no subject, the app sends one more request asking for it.

The prompt rule, the same for all models (SPEC 1):
> When your work creates, changes or decides something, call `update_subject`: update the matching subject from the list, or create one if none matches. Call it together with your last tool call. Skip it for plain questions.

**Relation to SPIKE-028:** 028's request list holds the parts of one request within a turn. Subjects last for the whole chat. A request that ends `done`, `blocked` or `dropped` may update or create a subject. The two designs are decided together, so they don't drift apart.

## Conditions
1. **Baseline:** today's loop, no subjects.
2. **Tool:** `update_subject`, `get_subject` and the prompt rule.
3. **Tool + index:** condition 2, plus the index in block 5.
4. **Tool + index + check:** condition 3, plus the app's check for turns that wrote changes.

## Tests
- Long chats of 10–15 turns, run with a low `history_max_turns` so cuts happen during the chat.
- Each chat mixes:
  - **work:** tables, pipelines, views, memory edits
  - **plain questions** that must not create a subject
  - **changes of mind:** a decision that is reversed later, which should update the subject instead of creating a new one
  - **work left unfinished** (`blocked` or `open`)
- Follow-ups after a cut that need an earlier turn: "is the BTC pipeline done?", "do the same for ETH", "why did we choose EUR?".
- A message that only touches an existing subject, to catch duplicates.

## Measures
- **Compliance:** the share of work turns that created or updated a subject, and how often only the app's check made it happen (condition 4).
- **Noise:** subjects created for plain questions, and duplicates of an existing subject.
- **Correctness:** whether each subject's status and outcome match what really happened in the turn.
- **Cost:** extra requests and tokens per turn, the size of the index, and the share read from the provider's cache where the provider reports it.
- **Payoff:** follow-ups after a cut answered correctly, and `search_history` and `read_messages` calls compared with condition 1.

Models: the SPIKE-018 development models, with the same prompts for all (SPEC 1). The harness is TASK-001's scenario runner, with fakes for the tools the app doesn't have yet.

## Done when
Each condition has results per model, with at least 3 reps per cell. There is a decision:
- whether subjects replace the free text of the session notes (3.4) or are added to them
- on the index in block 5 (3.1)
- on the app's check (8.3)
- whether *Refresh memory* (3.5) writes subjects in its merge step
- on how subjects relate to SPIKE-028's request list

## Result
**Round 1** ran on 2026-10-08 on gemma4:31b and gpt-oss:120b (Ollama Cloud, reserve key): 2 chats of 14 messages × 4 conditions × 3 reps, 48 runs. The harness is in [`spikes/029-subjects/`](../../spikes/029-subjects/README.md), the findings in its [`results.md`](../../spikes/029-subjects/results.md). It changes no app code: the rule and the index go into block 4 through `agent.Deps.Card`, and the app's check is sent as the next message.

- **Compliance is high.** 83–94% of work turns updated a subject without help, and the app's check raised it to 96–98%. A subject was almost never made for a plain question (3 in 36 runs, 2 of them caused by the check).
- **The main failure: subjects become a log of actions.** Models name a subject after the step they just took ("Update daily_btc schedule"), so a change to existing work gets a new subject. Changes that updated the existing subject: 24–28% without the index, 41–61% with it.
- **The index is needed but not enough.** It raised gemma's correct statuses from 78% to 96%. In 3 runs out of 3, gpt-oss missed that a URL belonged to a `blocked` subject that was listed in its context.
- **The payoff is in follow-ups.** Follow-ups answered right: gemma 75% in the baseline and 100% with subjects; gpt-oss 83% and 92%. Calls that needed earlier turns were right as often without subjects, since the card and `get_config` cover them. History reads did not go down.
- **The cost depends on how a model places its calls.** With the index: +7% requests for gemma, which sends `update_subject` together with its last real call, and +26% for gpt-oss, which sends every one alone. Prompt tokens grew 29–60%.

**Proposed decisions after round 1:**
- Subjects become the structure of the session notes, with the index in block 5.
- No app check for now.
- *Refresh memory* (3.5) is the place to merge duplicates.
- SPIKE-028 stays as planned.

**Round 2** ran on 2026-10-09: a rule that names a subject after the work itself and says a change updates its subject, on conditions 2 and 3, 24 runs.

- **The target is met with the index.** Changes that updated the existing subject went from 61% to 76% on gemma and from 41% to 88% on gpt-oss, 82% for both together. gemma made no duplicates.
- **Without the index, gpt-oss didn't improve** (24%). The index is needed.
- **The rule narrowed what counts as work.** Both models skipped a convention saved to memory, and gemma skipped a row change, so gemma's compliance with the index fell from 94% to 85%.
- **gpt-oss costs more:** +12–14% requests and +19–35% prompt tokens over round 1, since it sends every `update_subject` call alone and now makes more of them.

**Proposed decisions** (they replace round 1's):
- the round 2 rule, with an example from outside the test chats and one more line for conventions and row changes
- subjects become the structure of the session notes, next to a short free-text part, and don't replace it
- the index in block 5
- no app check
- *Refresh memory* (3.5) merges duplicates
- SPIKE-028 stays as planned

**Round 3** (planned on 2026-10-09) tests a broader subject. A subject is one goal of the user, and it can grow. Its text is added to instead of replaced, two subjects can be merged, and in one variant `status` is replaced by the list of what is still waiting.

All three variants run on condition 3 (tool + index), on the same models, 3 reps:

| | Subjects | Text | State | Merge |
|---|---|---|---|---|
| **A** (round 2's design, the control) | one per table, pipeline or decision | `outcome`, replaced | `status` | – |
| **B** | one per goal, can be widened and renamed | `summary` (replaced) + `notes` (only added to) | `status` | `merge_subjects` |
| **C** | like B | like B | `open` items only, no `status` | like B |

The tools in B and C:
- `update_subject(id?, subject?, status?, summary?, note?, open?)`. A note is added at the end with the message it came from. The summary is the current state, and the index doesn't show it.
- `merge_subjects(ids, subject, summary?, status?)`. The model writes the merged title. The app joins the notes in time order and the open items, so nothing is rewritten. Merged ids lead to the kept subject from now on, and the index shows only that one. If the call has no summary, the old summaries are joined. A later rewrite belongs to *Refresh memory* (3.5).
- In C the index line is `s2 · BTC price tracking · 1 open` instead of the status.

The chats get one more message each, after a cut, where two pieces of earlier work turn out to be one thing for the user: Bitcoin and Ethereum tracking in the crypto chat, and the follow_up pipeline and the board view in the jobs chat. The chats change, so A runs again instead of reusing round 2's runs. That is 36 runs.

**Measures:** what round 2 measured, plus:
- whether the two pieces of work end up in one subject after the merge message
- merges made, read one by one to judge whether they were right
- whether the waiting state shows: `open` or `blocked` in A and B, an open item in C
- follow-ups that need an older reason ("why weekends?", "why closed?"), which notes should keep

The status check doesn't apply to C. A goal with several parts has no single status, so C is judged by the waiting check and the follow-ups.

**Round 3 result** (2026-10-09, 36 runs, no runs lost; findings 13–18 in [`results.md`](../../spikes/029-subjects/results.md#round-3)):
- **One subject per goal makes a chat one subject:** 1.5–1.8 subjects per chat in B and C, against 6–7 in A. The index then says little.
- **Merging works between whole subjects.** All 10 crypto merges were right, and in A neither model joined the work by hand. In the jobs chat, the board view was already inside "Job applications tracking", so all 7 merges renamed the whole job search "Friday review". A subject can't split, so a grouping inside a goal has no place.
- **One status for a goal fails:** B's statuses were right in 41–59% of checks, against 67–93% in A. In C the open items showed waiting work in 12 of 12 checks.
- **The notes keep reasons, but gpt-oss answered "why weekends?" from the card without reading them,** and once made up a reason. gemma answered every follow-up right in all variants.
- **Cost:** 6–17% more prompt tokens than A, and the same number of requests.

**Proposed decisions after round 3:**
- Keep round 2's size: one subject per piece of work.
- Add notes that are only added to, open items instead of a status, and `merge_subjects`. Merges make larger subjects when the user or *Refresh memory* (3.5) asks for them.
- The other round 2 decisions stand.

This combined design hasn't been run as a whole. A round 4 on condition 3 would be 12 runs.
