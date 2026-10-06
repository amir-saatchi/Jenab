# SPIKE-029 — Subjects: a chat's work, kept as objects
**Type:** Spike
**Status:** Open (before Phase 2)
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
