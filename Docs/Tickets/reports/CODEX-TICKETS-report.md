# Codex lessons: which ideas get a ticket

**Date:** 2026-11-11 · **Input:** [CODEX-LESSONS-report](CODEX-LESSONS-report.md) (2026-11-09) · **Checked against:** SPEC v0.7, PROPOSAL §9, Gates 1–4 and Tickets/README at commit ba55571.

**Method.** I checked each idea in the report against today's SPEC and tickets. Where a verdict rests on code, I read the code:
- `internal/agent/turn.go` `runTools` runs calls one after another. `prompts/base.md` line 5 asks for "calls that don't need each other's results in the same message". So the report's premise for parallel calls holds.
- `runTool` returns "cancelled by user" when a tool fails after *Stop*, whatever the tool did before.
- In the Codex tree I spot-checked: the matching passes (`apply-patch/src/seek_sequence.rs`), the forced child environment (`core/src/unified_exec/process_manager.rs`, which also sets `LANG`/`LC_*` to `C.UTF-8`), the PowerShell UTF-8 prefix (`shell-command/src/powershell.rs`), the banned prefixes (`core/src/exec_policy.rs`), the 3 and 10-in-50 breaker (`ext/guardian-reviewer/src/circuit_breaker.rs`), hook trust by hash (`hooks/src/engine/discovery.rs`) and `WRITE_RESTRICTED` in `windows-sandbox-rs/src/token.rs`. All match the report.

No code was copied. No models were called.

**Numbers.** The last IDs in Tickets/README are SPIKE-040 and TASK-043. The report's TASK-042 and TASK-043 are taken by the shared-failure work. The drafts below use **SPIKE-041 to SPIKE-043** and **TASK-044 to TASK-045**. Before you file them, check the numbers again: another session may have taken some.

**Where we are.** Phase 1 and Phase 2 acceptance are still open (P1-18, P2-14), and the Gate 3 TASKs run alongside them. Phase 6 (editing and commands) comes after the public release. Most of the report's value is for Phase 6, so most tickets wait for it.

---

## Summary

### The top 8

| # | Idea | Today | Verdict | ID |
|---|---|---|---|---|
| 1 | Edit tool: matching passes, refusing several matches, check everything before writing, errors that show the lines | **Partly**: 8.5 names `edit_file(path, old, new)`, the before-copy and undo by hash. No matching, errors or protected paths | **Phase 6** (editing): the spike before the editing ticket | SPIKE-041 |
| 2 | A definition of "pattern" for *Always for this pattern* | **Partly**: 8.8 has the card and the levels; "pattern" is never defined | **Phase 6** (commands): this is the core of the command spike that 8.5 already asks for | SPIKE-042 |
| 3 | Windows write sandbox without admin rights | **New**: 8.5 has Job Objects, priority and memory checks, no file sandbox | **Phase 6**, after SPIKE-042: worth knowing, but large and only useful once commands exist | SPIKE-043 |
| 4 | Command output: head and tail, short header, forced environment, PowerShell UTF-8 | **Partly**: 8.5 says "output to the bucket as preview and ref" | **Phase 6**: a SPEC note now so the command ticket carries it; no ticket of its own | SPEC note B |
| 5 | Run read-only calls of one answer in parallel | **New**: SPEC is silent; the code is serial; the prompt already asks for calls together | **Now** (Gate 3), measure first: cheap, the same for every model, and the prompt already pays for it | TASK-044 |
| 6 | Skills trusted by hash | **Partly**: 8.9 uses only skills the user wrote or accepted, but says nothing about a file changed on disk or a skill in an imported project | **Phase 5** (user and project skills): a SPEC note now | SPEC note D |
| 7 | Honest interrupts and a net diff per turn | **Partly**: 8.3 closes open calls as "cancelled by user"; 8.5 says "a diff per file" | Split: **now** for the wording (desktop actions can already half-run); **Phase 6** for the diff (SPEC note) | TASK-045; SPEC note C |
| 8 | A breaker for denial loops | **New** | **Phase 6**, inside SPIKE-042: no denial loops seen with today's cards; commands are where they would hurt | SPIKE-042 |

### Smaller adopts

| Idea | Today | Verdict | ID |
|---|---|---|---|
| LM Studio preset | **New**: 3.9 lists 9 presets, LM Studio is not one. PROPOSAL §5 names LM Studio | **Now**, no ticket: one preset line (kind `openai_compatible`, `http://localhost:1234/v1`) with the next provider change | SPEC note F |
| LM Studio at first run "when it answers" | **New** | **No**: a second probe at first run for little gain; the preset is enough | – |
| MCP: separate startup and call timeouts | **Partly**: 8.7 has 60 s per call, no startup limit | **Phase 5** MCP ticket | SPEC note E |
| MCP: errors that name the setting | **New** for MCP (6.7 already does it for private hosts) | **Phase 5** MCP ticket | SPEC note E |
| MCP: tool list cached for the project card | **Partly**: 3.2 and 8.7 put tool names on the card, but don't say where they come from before the first start | **Phase 5** MCP ticket | SPEC note E |
| MCP: decline elicitation by default | **Partly**: 8.7's "Not yet" list doesn't name elicitation | **Phase 5**: add it to the "Not yet" line | SPEC note E |
| Secrets over 2,560 bytes (encrypted file, key in the keychain) | **Not needed**: 8.7 MCP auth is a header key; OAuth is not in v1 (11) | **No**, until MCP OAuth is planned | – |
| Protected paths for edits | **Partly**: 8.5 blocks credential files for reading and skips `.git/` | **Phase 6**, inside SPIKE-041 | SPIKE-041 |
| Safe internal git calls | **Not needed**: Jenab never runs git itself | **No** | – |
| Commands' network is not controlled | **New** | **SPEC note now** (doc only) | SPEC note A |

### The area table (36 rows)

I agree with the report's verdict on every row. Where a row leads somewhere, it is listed above.

| Rows | Report's verdict | My check |
|---|---|---|
| 1 turn loop, 4 retries, 9 token counting, 11 memories, 26 skills, 28 roles, 29 providers, 32 storage | Already better | Agree: 8.3, 3.8, 3.9, 3.3–3.5, 8.9, 8.6 and 2.3 cover them |
| 6 history, 8 caching, 12 approval levels, 25 MCP core, 35 project trust | Already have | Agree: 3.6, 3.7, 3.1, 8.8, 8.7, 8.5 |
| 2 tools in parallel | Adopt (not streaming dispatch) | Agree: TASK-044. Streaming dispatch would break 8.3's "tools only after a complete answer", which makes retries safe |
| 3 stop | Adopt (small) | TASK-045 now; the process-tree grace is SPEC note B |
| 13 exec policy, 14 dangerous commands, 20 shell tool | Adopt | SPIKE-042 |
| 16 Windows sandbox | Adopt unelevated only | SPIKE-043 |
| 17 network from commands | SPEC note | SPEC note A |
| 18 edit tool, 19 protected paths | Adopt | SPIKE-041 |
| 23 turn diff | Adopt | SPEC note C (Phase 6 editing ticket) |
| 30 local models | LM Studio preset | SPEC note F |
| 31 secrets | Adopt later | No, until MCP OAuth is planned |
| 21 long-running processes | Later | Agree; after the first `run_command`. No ticket now |
| 22 remote exec | Not for us; keep a small Go interface | Agree; a design hint for the Phase 6 tickets, not a ticket |
| 10 AGENTS.md | Not as instructions; maybe *Import as project skill* | **No** for now: Jenab's users are mostly not in code repositories; revisit with Phase 6 |
| 5 steering, 7 compaction, 15 guardian, 24 git undo, 27 hooks, 33 telemetry, 34 code mode, 36 config layers | Not for us | Agree |

---

## Ticket drafts

### TASK-044 — Run read-only tool calls of one answer together

**Type:** Task
**Status:** Open
**Gate:** 3
**Needs:** –

#### Goal
Block 1 asks the model to make calls that don't need each other's results in the same message (`prompts/base.md`, TASK-033). `runTools` (`internal/agent/turn.go`) still runs them one after another. So the saved requests don't save wall time. Measure how much time that costs, and if it is worth it, run read-only calls of one message together.

#### Scope
1. **Measure first.** From the reference suite's traces (`ToolTrace.Took`), count the messages with two or more read-only calls and the tool time they spend. Go on only if tool time is a real share of turn time, for example in `fetch_page` or `web_search` runs.
2. **A flag:** `Parallel bool` on `tool.Spec`. Candidates: `query`, `describe_table`, `read_ref`, `search_ref`, `search_history`, `read_messages`, `fetch_page`, `web_search`, `read_feed`, `call_api`, `bucket_read`, `bucket_list`, `list_files`, `read_file`, `search_code`, `mcp_describe`. Not flagged: anything that writes, `load_skill`, `ask_user`, `subagent`, and the `desktop_*` tools.
3. **Order:**
   - Checks and approval cards run first, one call at a time, in call order. A call's check counts the untrusted mark of every earlier call in the message as already set (from `Effects`), so the Auto rule (8.8) asks exactly as it does today.
   - Then the approved flagged calls run together, up to a small cap (start with 4). Other calls run alone, in their place.
   - Results are stored in call order, so the history and the cache stay the same as today.
4. **Stop and limits:** *Stop* cancels them all; each call keeps its own timeout; the runtime panel shows each call.
5. **SPEC 8.3:** a short paragraph with the rule.

#### Rules
- The same tools, prompts and order for every model (SPEC 1). No change to what the model sees.
- No loosening of approvals: a message runs the same cards in the same order as now.
- No streaming dispatch: tools still run only after a complete answer (8.3).

#### Done when
- The measurement is in the ticket's result, with the decision to build or not.
- If built: Go tests for call-order results, a card in the middle of a message, the untrusted mark across siblings, *Stop* during parallel calls, and a panic in one call.
- Before and after wall time on the scenarios with several fetches, on both reference models, and `jenab-scenarios check scenarios` passes.

---

### TASK-045 — Say when a stopped call may have run

**Type:** Task
**Status:** Open
**Gate:** 3
**Needs:** –

#### Goal
After *Stop*, every open call gets "cancelled by user" (8.3). For a call that was already running, that can be wrong. A `desktop_act` may have reached the app before agent-desktop was killed, and later MCP calls and commands will be the same. A model that reads "cancelled" and the user's "go on" may repeat the action. Tell the model what is known.

#### Scope
1. **List the tools** that can have an effect before they return: desktop actions and replays, and any write whose result is lost after *Stop*. Check what an `insert` taken by the writer returns after *Stop* (7.3 says it finishes).
2. **Three wordings:**
   - not started: "cancelled by user before it ran";
   - running, no effect outside Jenab's undo: "cancelled by user";
   - running, may have acted: "cancelled by user while it ran; it may have partly run. Check before repeating it." For a desktop step, also name the window and the step.
3. **Mark** such tools in `tool.Spec` (or reuse `Effects`), so MCP and commands get the same wording later.
4. **SPEC 8.3:** the three wordings, in the *Stop* list.

#### Rules
- The same wording for every model (SPEC 1).
- Keep the result short; it stays in the window.

#### Done when
- Go tests for each wording, including a desktop action stopped mid-call (with a fake runner).
- SPEC 8.3 has the rule.

---

### SPIKE-041 — Edit tool: matching, shape and protected paths

**Type:** Spike
**Status:** Open
**Gate:** 3 (Phase 6)
**Needs:** –

#### Question
How should `edit_file` (8.5) find the text to replace, and what should it say when it can't? Is `edit_file(path, old, new)` with an edit list better than a patch anchored on context lines, on the reference models?

#### Scope
1. **Matching passes**, in order; the first pass with exactly one match wins:
   1. exact;
   2. ignoring trailing whitespace;
   3. ignoring leading and trailing whitespace;
   4. with typographic dashes, quotes and spaces folded to ASCII.
2. **Never fold** letters, ZWNJ, bidi marks or Arabic and Persian letter forms (such as ي/ی, ك/ک). Test with Persian files.
3. **Errors:** several matches name the lines and ask for more context; no match shows the closest region.
4. **Writing:** check every edit of a call before writing anything. Keep the file's line endings and BOM. Write to a temp file and rename it.
5. **Shapes compared:** `old`/`new` pairs against a context-anchored patch, on the reference models, with real edits from a fixture repository: success rate, retries and tokens.
6. **Protected paths** for writes: at least `.git/`, Jenab's data and project folders, and the credential patterns already blocked for reading (8.5). Check whether git hooks and similar files that run code need more.
7. **A cancelled edit** names the files already written.

#### Rules
- One tool for every model (SPEC 1). A JSON function, not a grammar-constrained free-form tool.
- Write the matching fresh; copy no Codex code (Apache-2.0 would need NOTICE and change statements).
- Windows first; file paths and line endings as Windows has them.

#### Done when
- Each question has a measured answer on the reference models.
- The decision is written as SPEC 8.5 text: the tool, its errors and the protected paths.

---

### SPIKE-042 — Commands: patterns, approvals and output

**Type:** Spike
**Status:** Open
**Gate:** 3 (Phase 6)
**Needs:** –

#### Question
SPEC 8.5 says `run_command` comes "only after its own spike", and 8.8 offers *Always for this pattern* without saying what a pattern is. What is a pattern, which commands can never get *Always*, and how does Jenab tell them apart on Windows without failing open?

#### Scope
1. **Pattern:** the program plus its leading literal words (`go test`, `bun run build`, `git status`). Compound commands (`&&`, `;`, `|`) are split, and each part must match.
2. **Fail closed:** redirection, substitution, script blocks or variables are covered by no pattern and ask *Once*. A Go tokenizer for cmd and PowerShell, with no cgo or new parser dependency. How much syntax can it accept before failing closed?
3. **Never *Always*:** shells and interpreters with inline code (`cmd /c`, `pwsh -Command`, `python -c`, `node -e`), bare `git`, and delete commands (`rm`, `del`, `Remove-Item`).
4. **Dangerous list:** forced recursive deletes, ShellExecute and URL launches. They ask every time, at every level.
5. **Fields:** `why` (shown on the card) and a proposed pattern that the user can accept or narrow. Rules are stored as data in `_jenab_approvals`.
6. **Breaker:** after 3 denials in a row in a turn, the turn ends with a notice. Should it apply to every card (8.8), not only commands?
7. **Measure** on real commands the reference models write for the Phase 6 "fix a failing test" task: how many ask, how many are covered, and any command a pattern let through that it shouldn't have.
8. **Output and environment** (SPEC note B): check them on Windows: code pages, PowerShell output, ANSI colours.

#### Rules
- The same tool, description and rules for every model (SPEC 1). Lines that depend on the OS are allowed in the description; lines for a model are not.
- Stricter than today, never looser: after untrusted content every command asks (8.8).
- Windows first; bash (Git Bash, WSL) after.

#### Done when
- The pattern rules, the never-*Always* list, the dangerous list and the breaker are decided, with the measurements.
- The decision is written as SPEC 8.5 and 8.8 text, ready for the `run_command` ticket.

---

### SPIKE-043 — Windows write sandbox without admin rights

**Type:** Spike
**Status:** Open
**Gate:** 3 (Phase 6)
**Needs:** SPIKE-042

#### Question
Can Jenab run commands so that they can write only inside the linked folder, with no admin rights, from Go (`golang.org/x/sys/windows`)?

#### Scope
1. **Token:** a write-restricted token with a random capability SID for each workspace, and an allow-write ACE for that SID on the folder. Deny-write on protected folders (`.git/`, Jenab's own).
2. **Tools still work:** `go build` (cache under `%LOCALAPPDATA%`), `bun install`, `git` (`index.lock`), temp folders. Which extra write roots are needed?
3. **Cleanup:** remove the ACEs when the folder is unlinked, and after a crash.
4. **Fit:** with the Job Object, below-normal priority and the memory checks (8.5).
5. **Consent:** the ACEs change permissions on the user's folder. How is that shown and agreed?
6. **Limits to document:** reads are not isolated; the network is not controlled (SPEC note A).

#### Rules
- No admin rights, no new users, services or firewall rules.
- Write it fresh from the idea; copy no Codex code.
- Windows only; macOS and Linux are out of scope.

#### Done when
- A spike program shows the four checks above on Windows 11, with results.
- A go or no-go decision, and if go, SPEC 8.5 text. Whether the sandbox lets Auto skip any command card is **not** decided here; that would need its own spike (8.8).

---

## SPEC notes (no ticket)

**A. SPEC 8.5, Commands, planned rules.** Add:
> - **Network:** a command's own connections are not controlled. The host and private-network rules (6.7) cover Jenab's fetches only. The guard is that after a turn has read untrusted content, every command asks (8.8).

**B. SPEC 8.5, Commands, planned rules.** Replace "output to the bucket as preview and ref" with:
> - **Output:** stored in the bucket with a ref; the preview keeps the start and the end of the output, with a marker for what was left out in between, since a build's error is usually at the end. A header gives the exit code (or that it was stopped), the time and the full size.
> - **Environment:** `NO_COLOR=1`, `TERM=dumb`, `PAGER=cat` and `GIT_PAGER=cat`. PowerShell scripts start by setting UTF-8 output, and output is decoded from the console's code page, so Persian text survives.
> - **Stop:** the process tree gets a short grace period, then is killed. A stopped command's result says it may have partly run, and how long it ran.

**C. SPEC 8.5, Editing.** Replace "The chat shows a diff per file" with:
> - The chat shows a diff per file, built from the before-copy in the bucket and the content written: one net diff per file per turn, never from git.

**D. SPEC 8.9, Trust.** Add (Phase 5, with user and project skills):
> - The hash of each accepted user or project skill is stored with it. A skill whose files change on disk, or that arrives in an imported project, is left out of the skill list until the user accepts it again in the import card.

**E. SPEC 8.7** (Phase 5, for the MCP ticket):
- *Processes:* "A start that doesn't finish within 30 s fails, separately from the call limit."
- *Processes:* "Start and call errors name the setting to change, e.g. the server's time limit."
- *Tools in the context:* "The tool names and descriptions on the card come from the server's last start, kept in `registry.db`, so the card lists them before the server runs."
- *Not yet:* add "elicitation (declined unless the user answers)".

**F. SPEC 3.9, Providers and keys.** Add LM Studio to the preset list:
> … OpenRouter, Z.ai and LM Studio (*OpenAI-compatible* with `http://localhost:1234/v1`, no key).

When it is added, check that a cold model load answers within the 2-minute first-event limit (3.8). If it doesn't, record it; don't key a timeout on the URL.

---

## Left out, and why

- **Report's TASK-042 (`run_command` output) as a ticket:** there is no `run_command` ticket to attach it to yet. It becomes SPEC note B and part of SPIKE-042.
- **Report's TASK-045 (net diff per turn) as a ticket:** it is how 8.5's diff is built, not separate work. SPEC note C.
- **Report's TASK-044 (skills by hash) as a ticket:** user and project skills arrive in Phase 5. SPEC note D, so the Phase 5 skills ticket includes it.
- **Report's TASK-046 (LM Studio preset) as a ticket:** one line. SPEC note F.
- **The denial breaker now:** no loops seen with today's cards. It is decided in SPIKE-042, which also asks whether it should apply to every card.
- **LM Studio probe at first run:** little gain for another network probe.
- **Encrypted secret file for large tokens:** nothing in v1 needs secrets over 2,560 bytes.
- **AGENTS.md as a project skill:** low value for a data app; revisit with Phase 6.
- **Long-running processes, remote exec, git safety:** after the first `run_command`, or never needed.
- **Streaming tool dispatch, steering that interrupts, automatic compaction, LLM approver, elevated sandbox, network proxy, hooks, code mode, telemetry, config layers:** agree with the report; they go against 8.3, 8.8, 3.5 or local-first.
