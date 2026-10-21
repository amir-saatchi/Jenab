# SPIKE-034 — Topics: threads inside a chat, with their own notes and history
**Type:** Spike
**Status:** Open
**Gate:** 3

## Question
A chat with a lasting role, such as a *SQLite expert*, gets many subjects over time: the Users table, then Orders, then a question about both. In one chat they share one history window. After a few cuts (SPEC 3.6), the agent finds earlier work on a subject only through `search_history`, and the work on other subjects fills the window in between.

SPIKE-029 tried to fix this inside one chat: the agent kept a list of **subjects**. Keeping the list worked, but the agent's grouping was the weak step. It named subjects after the step it had just taken, so a change to existing work made a duplicate. A subject could grow and merge, but it couldn't split. **Topics** make the split visible and let the user approve it.

Two questions:
1. **Does the agent propose a topic at the right moments?** That is, for a new subject the user will come back to. Not for a plain question, and not a second time for a subject that already has a topic.
2. **Do topics beat one long chat** that has cuts and `search_history`? Measured on follow-ups after many turns, on questions that span topics, asked in the main thread, and on cost.

The design to test (proposed on 2026-10-20):
- **A topic is one level below a chat.** Any chat can have topics, Mother included. A chat's **main thread** stays a normal chat: small questions, overview, and questions that span topics. A topic can't have topics of its own.
- **A chat is who, a topic is what.** A chat has a role (8.6). Its topics share that role and differ only in their subject.
- **A topic's context:** block 1 with the parent's role, user memory, project memory, the project card, the parent's session notes, the topic's own notes, and the topic's own history window. The main thread and the other topics aren't in its context. It reaches them through `search_history` and `read_messages`.
- **The topic index** sits in the main thread's context, one line per topic, e.g. `t2 · Users table · 1 open`. Like the chat list in Mother (8.6), it is built at cuts. A topic's context doesn't have the index.
- **Topic notes** follow SPIKE-029 round 3's proposed decisions: a short summary, notes that are only added to, and open items instead of a status. `merge_topics` joins two topics when the user says they are one thing.
- **Who opens a topic:** the user can always open one. The agent only **proposes** one, a new topic or an existing one, with `propose_topic(title | topic_id)`. When the user accepts, their message moves into that topic, and the turn runs there. The agent never opens a topic on its own.
- **Writing upward:** a topic may write its parent's session notes directly, under the revision rule of memory (3.3).
- **Parallel turns:** topics of the same chat can run turns at the same time. Revision checks catch conflicts on the parent's notes.
- **Storage, if adopted:** a row in `chats` with a `parent_chat_id`, so history, notes, *Refresh memory*, *Clear*, undo and search are reused. A topic isn't in the sidebar's chat list; it shows inside its chat.

**Relation to SPIKE-029:** if topics win, they replace subjects, and P2-10 (session notes) depends on this spike instead of SPIKE-029. SPIKE-029's status stays as it is until this spike decides. Condition 4 below runs SPIKE-029's proposed combined design, the round 4 it never ran, so the two can be compared on the same chats.

**Out of scope: report-up.** This is a chat or topic telling its parent, or Mother, that something changed:
- a setting, *Mother hears*, with the values off, notice and wake
- a `tell_mother(message)` tool
- the star shape: only Mother sends to other chats
- a hop limit for chains of turns the app starts

It is a separate question, about cost and loops rather than about context. It depends on `create_chat` and `send_to_chat`, which land in Phase 5. It is a possible later round, once topics are decided.

## Conditions
1. **Baseline:** one chat with today's loop, cuts and `search_history`.
2. **Topics, opened by the user:** the scripted user opens a topic at the first message of each subject and writes each later message on that subject in its topic. The agent doesn't route anything. This measures what topics are worth when the split is perfect (question 2).
3. **Topics, proposed by the agent:** the scripted user writes in the thread they are in, and the agent has `propose_topic`. The user accepts a proposal that matches the message's label and declines any other. This measures question 1 and the end-to-end result.
4. **Subjects:** one chat with SPIKE-029's proposed design: one subject per piece of work, notes only added to, open items, `merge_subjects`, and the index. This is the comparison that decides whether topics replace subjects.

## Tests
- **Two scripted chats of about 18 messages**, run with a low `history_max_turns` so cuts happen during the chat, as in SPIKE-029.
  - A *SQLite expert* chat: Users, Orders and Products tables.
  - A second chat on another theme. SPIKE-029's jobs chat can be reused, with its applications table, follow_up pipeline and Zalando watcher.
- **Each message has a label:** its subject, or `main`. The label decides where the scripted user writes in condition 2, and which proposals they accept in condition 3.
- **Each chat mixes:**
  - **work** on three or four subjects, interleaved
  - **returns** to a subject after several turns on others
  - **changes of mind** on an earlier decision
  - **plain questions** that must not get a topic
  - **cross-topic questions** asked in the main thread, e.g. "how do Users and Orders relate?" or "what's still open everywhere?"
  - **a message on an existing subject sent from the main thread**, to catch duplicate topics
  - **a merge message:** "Products and Inventory are one thing for me"
- **Follow-ups after cuts** that need an older turn or reason, e.g. "why did we make email unique?" or "is the Orders table done?".
- **The scripted user in condition 3:**
  - They write where they are.
  - They go back to the main thread themselves for `main` messages.
  - They never open or switch to a topic on their own, so every move into a topic, new or existing, comes from a proposal.
  - A message whose label differs from the current thread and gets no proposal is answered where it is. That counts as a miss.
- **One parallel step:** two messages to two topics of the same chat, sent at once. Both turns write the parent's notes.

## Measures
- **Proposals (condition 3):**
  - new subjects that got a proposal
  - proposals for plain questions or `main` messages
  - duplicate topics: a new topic proposed when one already exists for that subject
  - returns proposed into the right existing topic
  - the cost of a proposal: requests and tokens in the turn that only proposes
- **Payoff:** follow-ups after cuts answered right, and cross-topic questions answered right, per condition.
- **History reads:** `search_history` and `read_messages` calls per run, and how many of them reach another thread.
- **Notes:** whether waiting work shows as an open item, and whether the parent's notes stay right after the parallel step: revision conflicts and what the agent did with them.
- **Cost:** requests and prompt tokens per run. Cache reads, where the provider reports them. The length of the beginning that a topic's requests share with the main thread's, as an upper bound on cache reuse where the provider reports nothing (SPIKE-029: Ollama Cloud reports no cache reads).

## Models
The reference models (DEVELOPMENT.md): Gemini `gemini-3.5-flash-lite` and Ollama Cloud `gemma4:31b`.
- The prompts and tools are the same for every model (SPEC 1).
- Which keys to use, including whether to use the reserve keys (`OLLAMA_API_KEY_II`, `Z_API_KEY_II`), is confirmed with the user before any run.
- A free tier's failures (rate limits, quota) are recorded and that model stops; there are no workarounds for them.

## Setup
- **Harness:** `spikes/034-topics/`, built from SPIKE-029's (scripted chats, a fake project world, scoring, a transcript per run). It runs the app's real orchestrator (`internal/agent`) with the real `search_history` and `read_messages`, and fakes for the tools the app doesn't have yet.
- **Topics in the harness:** each topic is a chat in the same project in the store. The harness keeps the parent link.
  - On an accepted proposal, the user's message is sent again in the topic. The turn that only proposed stays in the thread it was sent in, and its cost counts.
  - `propose_topic`, `update_topic`, `merge_topics` and the notes tools are spike code.
- **Per-chat context:**
  - `Deps.Card` is per project, but a topic needs its parent's notes, its own notes, and, in the main thread, the index. These must go into a block that is chosen per chat.
  - Like SPIKE-032's `Grounding`, a spike-only hook in `Deps` may be needed. That changes app code, unlike SPIKE-029, so it is agreed with the user before the harness is written.
- **Safety:** keys come from `.env` and stay in memory. Only synthetic text is sent.

## Done when
Each condition has results per model, with 3 reps per cell: 2 chats × 4 conditions × 2 models × 3 reps = 48 runs. There is a decision on:
- topics: yes or no, and whether the agent proposes them or only the user opens them
- whether topics replace subjects (SPIKE-029), and P2-10's dependency
- the topic index: its line format and where it sits in the context
- the parent's notes: whether topics write them directly or only report up
- whether report-up gets its own round

## Result
