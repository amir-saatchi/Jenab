# TASK-018 report: rewind a chat to an earlier message

Branch `claude/ecstatic-gauss-e24a6f`. Docs/ is not edited; the SPEC text below is proposed for Main-Session to merge.

Decision taken (Main-Session, scope point 2): **no branches.** The dropped turns go into the undo window like other deletions (4.4). *Undo rewind* brings them back while the chat has not gone on.

---

## 1. Proposed SPEC text

### 2.3 chats.db: new rows in the table list

| Table | Holds |
|---|---|
| `rewinds` | One row per rewind (8.3): chat_id, the turn the chat was taken back to, the last turn that left, the notice it wrote, what it changed in the chat's state (notes, skills, *Refresh memory* progress) so an undo can put it back, and the time |
| `rewound_messages` | The messages a rewind took out, with their parts as JSON, token counts and times. They are out of the chat, the context and `messages_fts` |
| `turn_state` | For each chat and turn, the session notes and the loaded skills as they were before the turn first changed them, so a rewind can put them back |

Add after the "Reply to part of a message" bullet:

- **Rewind** (TASK-018): a rewind moves the later turns' messages from `messages` and `message_parts` to `rewound_messages`, in one transaction. Deleting the parts takes their text out of `messages_fts` (the index trigger), so history search, `read_messages` and the substring scan no longer find them. *Undo rewind* writes them back with their IDs and indexes them again.
  - Usage (*Settings → Usage*) still counts the tokens of rewound messages: they were spent.
  - A change-log source `message:<id>` of a rewound message still resolves to its chat, for conflict lists and *Undo turn*.
  - Rewinds and their messages are dropped with the change log, after `change_keep` (90 days). *Clear* and deleting the chat drop them too.

### 2.6 Undo: add a bullet after "Revert run"

- **Rewound turns:** a rewind (8.3) never undoes the turns' changes to project data. The rewind's mark in the chat has *Undo changes* for them, with the same plan, conflicts, partly-undone states and redo as *Undo turn*, over the changes whose source is a rewound message. Its notice names the turns: `[the changes of turns 4–6, taken out of this chat, were undone: 12 rows in prices]`. After a rewind to the start, no turn is left to hold the notice, so there is none.

### 3.4 Session notes: add a bullet

- **Rewind** (8.3): the notes go back to how they were before the first turn that left changed them, with a new revision, so a model holding the old revision is told to read them again. Edits by *Refresh memory* follow the same rule. *Undo rewind* puts back the notes the rewind replaced, again with a new revision. A chat from before TASK-018 has no record of earlier notes, so a rewind keeps its notes as they are.

### 3.6 History window: add to the **Cut** bullet

- A rewind (8.3) and its undo are a cut too: the window is built again at the next request, so it starts a new cached prefix, and blocks 2–5 are rebuilt with the notes and skills as the rewind left them.

### 4.4 Lifecycle: add to the daily sweep

- Rewound turns (8.3) are kept with the change log, 90 days, and dropped with it. *Undo rewind* is offered only for the chat's newest rewind and only while the chat has not gone on; after that, the rewound turns are kept for their changes' undo and for Usage, not to come back.

### 8.3 Turns and background work: new section after "User messages during a turn"

- **Rewind** (TASK-018): the user can take a chat back to an earlier turn.
  - **Where:** on the user message that starts a turn (not one that joined a running turn), between turns only, like *Clear*: refused while the chat is in a turn, has a queued message or runs *Refresh memory*.
    - *Rewind to here* keeps that turn and takes out every later one. It is shown only when there is a later turn.
    - *Edit and resend* takes out that turn and every later one, and puts the message's text back in the composer. A quote of a turn that stays comes back with it; attached files come back as their links in the text.
  - **Asking first:** a dialog says which turns leave, what they changed in the project and that it stays, whether the session notes go back, which skills are unloaded or loaded again, and that *Undo rewind* brings the turns back until the chat goes on.
  - **What goes:** the later turns leave the chat, the context and the history index (2.3). The chat's next turn has the next number after the kept turn. Session notes (3.4), loaded skills (8.9) and *Refresh memory*'s progress (`memory_reviewed_up_to` and the review chunks of the dropped turns, 3.5) go back to how they were at the end of the kept turn.
  - **Data changes stay:** a rewind never undoes them. When the dropped turns changed project data, the kept turn gets a notice for the model, which the chat doesn't show: `[the user took turns 4–6 out of this chat; what they changed in the project stays: 12 rows in prices]`. It says nothing of what the turns were about. A rewind to the start writes none, like *Clear*. The rewind's mark offers *Undo changes* (2.6).
  - **The mark:** a line after the kept turn, `Turns 4–6 taken out · 14:02`, with *Undo rewind* while it can be undone and *Undo changes* while the turns have changes in effect. A later rewind to an earlier turn moves older marks up to its own place.
  - **Undo rewind:** brings the turns back as they were, with their index entries, notes, skills and refresh progress, and drops the notice. Only the chat's newest rewind, while the chat has not gone on since; within the undo window (4.4).
  - **Cache:** a rewind is a cut (3.6).
  - **Background work:** a rewind can't happen during a turn, so nothing a turn started is still running in it then: subagents run inside their call, and *Stop* ends them. A title still being made from the chat's first turn is dropped when the chat is rewound to its start, as after *Clear*. When background tasks, `create_chat` and `send_to_chat` come (8.6), a rewind stops the tasks the dropped turns started and keeps the chats Mother made or messaged, and says so in its notice; until then there is nothing to stop.

### 8.6 Mother chat: add to *Clear*

- A rewind to the start (8.3) is like *Clear* with undo, but it also puts the notes and skills back to how they were before the first turn. A role's skills stay.

### 8.9 Skills: add to **Staying loaded**

- **Rewind** (8.3): the chat's skills go back to those loaded at the end of the kept turn: skills loaded in the dropped turns are unloaded, and skills that left block 1 at a cut in them are loaded again. The next request is a cut, so block 1 matches. *Undo rewind* puts back the skills the rewind replaced.

---

## 2. What was built

**Store (`internal/store/rewind.go`, `chats.go`)**
- Format step 6: tables `rewinds`, `rewound_messages`, `turn_state`.
- `turn_state` records the notes and skills as they were before each turn first changed them (`SetSkills`, `saveNotesTx`).
- `RewindPreview`, `Rewind`, `UndoRewind`, `Rewinds`, `RewindMessages`, `PurgeRewinds`.
  - `Rewind` does everything in one transaction: moves the messages out, restores notes (revision + 1) and skills, resets `memory_reviewed_up_to` and moves the dropped review chunks, writes the notice, bumps the seq.
  - `UndoRewind` reverses it. Only the newest rewind, and only while the chat's last turn is still the rewind's turn (`ErrRewindMoved`); `ErrRewindGone` once undone or purged.
- `Usage` and `MessageChats` also read `rewound_messages`. *Clear* drops the chat's rewinds and turn_state.

**Agent (`internal/agent/rewind.go`, `undo.go`, `trace.go`, `agent.go`)**
- `RewindState` (for the dialog: turns, kept changes, notes, skills), `Rewind`, `UndoRewind`, `RewindUndoState`.
- Guards like *Clear*: `ErrInTurn`, `ErrRefreshing`.
- After a rewind or its undo, the chat state is read again and the window is reset (a cut), traces of dropped turns go, and a title in flight is dropped on a rewind to 0.
- `UndoRequest.Rewind`: *Undo turn* over a rewind's messages, with its own notice text.
- `NoticeRewound` is an aside notice (added between turns, doesn't take a turn's footer).

**App (`internal/app/chat.go`, `errors.go`) and cleanup (`internal/project/manager.go`)**
- Bound methods `RewindState`, `Rewind`, `UndoRewind`, `RewindUndoState`; `ChatSnapshot.Rewinds`.
- UI messages for the three new errors.
- The daily change-log cleanup also purges rewinds past `change_keep`.

**Frontend**
- `chat/rewind.tsx`: *Rewind to here* and *Edit and resend* in the user message's hover footer, with the confirm dialog, and `RewindMark` with *Undo rewind* and *Undo changes*.
- `chat/rewind-text.ts` (+ tests): `turnStarts`, `marks`, `markText`, `rewindLines`.
- `chat/undo.tsx`: `UndoTurn` takes `rewind` and `label`, and hides itself on a mark when there is nothing to undo or redo.
- `state/thread.ts`: `Thread.rewinds`, `reload(chat)`, and `useFill` for putting text in the composer; `composer.tsx` takes it.
- `chat/rows.ts`: the rewind notice is hidden and is an aside.
- `thread-view.tsx`: one prop and one line in `UserRow`'s footer, plus mark placement in `ThreadView` (kept small because TASK-040 also edits `UserRow`).
- Bindings regenerated.

## 3. Checks

All run in the worktree with `GOPROXY=off`, `GOMEMLIMIT=1500MiB`:
- `go build ./...` (after `bun run build`): ok
- `go vet ./...`: ok
- `go test -p 2 -short ./...`: all ok
- `gofmt`: the changed files are clean once CRLF is stripped. The checkout uses `core.autocrlf=true`, so `gofmt -l` lists every file, touched or not.
- `go run ./cmd/jenab-scenarios check scenarios`: all ok
- bindings check (`wails3 generate bindings … -d bin/bindings-check` + `git diff --no-index`): no diff
- frontend: `bun install --frozen-lockfile` (the worktree had no node_modules; no new packages), `bun run typecheck` ok, `bun test` 131 pass, `bun run build` ok

**New Go tests**
- `internal/store/rewind_test.go`: `TestRewind` (FTS and substring search, notes revision and conflict, skills, Usage, MessageChats, Rewinds, undo restores everything incl. turn_state), `TestRewindRules` (bad turns, notice turn, rewind to 0, newest-only undo, `ErrRewindMoved`, notes after a new turn, purge, Clear), `TestRewindReview`.
- `internal/agent/rewind_test.go`:
  - `TestRewind`: a turn that loads a skill, saves notes and inserts rows is rewound. Checked:
    - the preview
    - the notice text
    - the history index
    - the skills
    - data kept
    - the next request has nothing of the dropped turns (words, notes, skill text)
    - `ErrRewindMoved` after a new turn
    - *Undo changes* via `UndoRequest{Rewind}` and its notice
    - again after a restart (new orchestrator on the same folder)
  - `TestUndoRewind`: the turns, index, skills and notes come back, and the next turn is number 4.
  - `TestRewindBetweenTurns`: `ErrInTurn` while a tool call runs, `ErrNothingToRewind` for the last turn, idle after.
  - `TestRewindToStartDropsTitle`: a title streaming in the background is dropped on a rewind to 0.

**Live check** (server-mode build `-tags server`)
- Setup:
  - `LOCALAPPDATA` and `USERPROFILE` pointed at scratchpad folders.
  - A local fake Ollama (stdlib Go in the scratchpad, no keys). It answers by listing the user messages it receives.
  - Driven in the in-app browser.
- Steps and results:
  - Three turns, then *Rewind to here* on turn 2: the dialog said "Turn 3 leaves…", and the mark "Turn 3 taken out" appeared.
  - *Undo rewind* brought turn 3 back.
  - *Edit and resend* on turn 2 put "second banana" in the composer and showed "Turns 2–3 taken out".
  - The edited message got the answer `I see these user messages: "first apple", "second blueberry"`.
  - After a server restart, the next turn saw only apple, blueberry and the new message.
- Fixed during the check: the dialog went blank while closing.
- Both processes were stopped by PID.
- Not checked live: data changes. The fake model makes no tool calls; this is covered by the Go tests.

One slip: I removed the generated `bin/bindings-check` folder with `rm -rf` instead of the Recycle Bin. It was output I had just generated there, git-ignored.

## 4. Open points

1. **Desktop sessions (internal/desktop, TASK-038's area):** sessions are keyed by turn number and live up to 10 minutes. If the dropped last turn left a session open, a new turn with the same number within those minutes would reuse that window. The same is true after *Clear* today. Suggested fix there: end the chat's session on Clear, rewind and its undo, for example with `Manager.EndChat(p, c)` called from the orchestrator.
2. **After the chat goes on,** dropped turns can't come back. They stay for their changes' undo, Usage and conflict names until purged. That is the "no branches" decision; say if you want *Undo rewind* to work later too (it would need turn renumbering).
3. **Chats from before this change** have no `turn_state` rows, so a rewind there keeps their notes and skills as they are. The dialog then says nothing about notes.
4. **Subagent transcripts** of dropped turns stay in the bucket (`subagents/<chat>/`) until Clear or delete, like a stopped turn's.
5. **Rewind to the start puts skills back to before turn 1,** unlike *Clear*, which keeps loaded skills. A role's skills stay. I think that is right for "as if the turns never happened". Say if it should match Clear instead.
6. **Edit and resend** brings back text and links, not attached files as chips; images come back as their links in the text.

## 5. Scope point 4, item by item

| Item | Handled |
|---|---|
| History search index (2.3) | The parts are deleted, so the FTS trigger unindexes them and the substring scan doesn't see them. *Undo rewind* re-inserts and re-indexes. Tested in store and agent. |
| Session notes (3.4) | `turn_state` keeps the before-state per turn. A rewind restores the first one after the kept turn, with revision + 1; undo restores the replaced notes. Tested. |
| Skills (8.9) | Same mechanism through `SetSkills`. The next request is a cut, so block 1 matches. Tested, including that the skill text is absent from the next request. |
| Refresh memory progress (3.5, not in the list but follows the chat) | `memory_reviewed_up_to` is reset to the last kept message if it pointed into the dropped turns. Dropped review chunks move into the rewind and come back on undo. Tested (`TestRewindReview`). |
| Background tasks and subagents (8.3) | A rewind is refused while a turn runs or is queued. Subagents run inside their call, so none outlive a turn. Background tasks don't exist yet. A title in flight is dropped on a rewind to 0. Tested. The SPEC text says what happens once background tasks come. |
| Chats Mother created or messaged (8.6) | `create_chat` and `send_to_chat` aren't built yet, so nothing to do. The proposed SPEC text says such chats are kept and named in the notice. |
| Data changes (scope 3) | Kept, listed in the dialog, the model is told through the notice, and *Undo changes* is on the mark. Tested. |
| Cache (scope 5) | The window is reset, so the next request cuts. Tested indirectly (the next request has no dropped content, notes or skill text). |
| Not during a turn (scope 6) | `ErrInTurn` / `ErrRefreshing`, and the UI hides the actions while running. Tested. |
