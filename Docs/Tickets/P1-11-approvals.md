# P1-11 — Approvals and questions
**Type:** Feature
**Status:** Done
**Gate:** 3 (Phase 1)
**Needs:** P1-10
**Requirements:** R-18, R-19, R-117

## Goal
The approval card, the question form and the approval levels, on the Go side (SPEC 8.8). The UI is in P1-15.

## Scope
- **Parts:** `approval` and `question`. The turn waits without holding a transaction.
- **Closing:** if the user writes instead, the card closes as *Deny* with the message as the note; *Stop* also closes it.
- **Records:** in `_jenab_approvals`, with the answer and its source (`user` or `auto`).
- **Levels:** Strict, Standard and Auto per project in `_jenab_meta`; the default comes from `config.yaml`; a change applies from the next tool call and adds a notice.
- **`approve(t, spec, needs)`** in `agent` (CODE-OUTLINE 7).
- **`ask_user`:** 1–4 questions with options and *Other*; the answer returns to the agent.
- **Later:** no Phase 1 tool needs an approval yet (migrations come in Phase 2, hosts in Phase 4), so the tests use a test tool that does.

## Done when
Tests cover:
- wait, approve, and deny with a note
- deny by writing a message, and *Stop* while waiting
- each level on the test tool
- an `ask_user` answer reaching the agent

## Result
- **Parts:** an approval card has a kind, a target, the ask, options with a grant each (*Once*, *Always* or *Deny*; a *Deny* option is required), then the answer, note, who answered and when. A card or form closed by *Stop* is marked `stopped`. Both are parts of the tool message, written before the call's result; the answer is written into the same part (`ChatsDB.SetPart`) and sent as a `PartDone` with the same index.
- **Waiting:** the turn waits on a channel with no transaction held. `chat:status` is `waiting` with what waits; `Live.Waiting`, the project activity and `ChatStatus` ("waiting for the user") show it too. `Orchestrator.Answer` checks the answer against the card or form (`ErrNotWaiting`, `ErrBadAnswer`). A message sent while waiting closes a card as *Deny* with the message as the note, or answers a form. *Stop* and the project's close mark the part stopped, record nothing, and the result is "cancelled by user".
- **Records:** `_jenab_approvals` gets every decision with its source, `user` or `auto`. The newest decision for a kind and target counts, so a later *Deny* revokes an *Always*. A denied call's result is "not run: the user denied it (…)", with the note.
- **Levels:** Strict, Standard and Auto in `_jenab_meta`; Standard when none is stored. `project.Deps.Level` gives a new project's level (the app passes `approvals.default_level`). `Orchestrator.SetLevel` adds the notice "[the approval level changed from X to Y]"; a notice after a failed turn doesn't hide *Retry*. The rules table in `agent` has hosts (Auto approves unless the chat read untrusted content since the user's last message) and Starlark (Auto approves); every other kind asks at every level until its phase adds it. Auto approvals are recorded and shown as an answered card (the chip).
- **`approve`:** a runner method, not a function of the turn: it drops what is already approved and splits the rest into asks and auto approvals. `runTool` runs the preflight, the approvals, then sets `PrivateHosts` for network tools and the untrusted mark after untrusted ones.
- **`fetch_page` hosts:** the user's decision was to ask per new host. At Strict and Standard the first fetch from a host shows a card; *Allow for this project* approves it for every chat. This also wires `tool.Env.ChatStatus`, `PrivateHosts`, the preflight and the untrusted mark that P1-10 left open.
- **`ask_user`:** in `agent` (`agent.Tools()`), 1–4 questions with 2–4 options each; the UI adds *Other*. The answer goes back as JSON by header, with any note. Without `Env.Ask` (subagents later) it tells the agent to put the question in its result. Tools that ask the user have no timeout.
- **Tests:** every Done-when case, plus the activity and status while waiting, an answer for the wrong card, the level notice and *Retry* after it, private hosts reaching the tool, `fetch_page` asking for its host, records and revocation in `store`, `SetPart`, levels and private hosts in `project`, card validation in `chat`.
- **Mutation checks:** 55 on the new and changed code in `agent`, `store` and `project`; 50 made the tests fail at first. Of the other 5:
  - 3 were broken mutants; redone, all make the tests fail.
  - 1 got a test (private hosts are sorted before duplicates are dropped).
  - 1 changed nothing, so the code was simplified (lower case before punycode, which lowers it anyway).
- **Not done:**
  - The *revoke* link on the auto-approved chip, the badge in the chat list and the desktop notification are UI work (P1-13, P1-15).
  - A card left open by a crash stays unanswered in the history; the UI shows it as closed.
  - An answer that arrives at the same moment as *Stop* may be lost; the turn stops either way.
