# What Jenab can learn from Codex CLI

**Date:** 2026-11-09 · **Source:** OpenAI Codex CLI source tree (Apache-2.0), read-only, kept git-ignored at `.source-code/codex-main/`. Paths below are relative to `codex-rs/`. **Jenab:** SPEC v0.7, Tickets/README as of commit e4191c4.

**Method.** Five read-only research passes over the Codex tree, one per area. I spot-checked the claims the verdicts rest on against the source: `seek_sequence` tiers, the restricted-token flags, the banned-prefix list, call/output pairing, world-state diffs, removal of Chat Completions, the guardian breaker limits, and Statsig metrics. Nothing was built or run.

**About Codex docs.** `docs/*.md` in the tree are stubs that link to developers.openai.com. Everything below therefore comes from the code itself. The tree is a recent snapshot: the legacy `shell` tool and ghost-commit undo are already gone.

---

## Summary: the top 8 ideas, ranked by value for Jenab

Most of the value is for **Phase 6** (workspace editing and `run_command`, SPEC 8.5). Codex has solved, in production, exactly the problems Jenab's Phase 6 plan leaves open.

1. **An edit tool that forgives whitespace but never guesses** (SPEC 8.5, 2.6). Codex finds the text to replace in four passes:
   1. exact match;
   2. ignoring trailing whitespace;
   3. ignoring leading and trailing whitespace;
   4. with Unicode dashes, quotes and spaces folded to ASCII.

   It also checks the whole edit (reads every file and computes every new version) before writing anything, and its error messages repeat the lines it could not find. For Jenab's planned `edit_file(path, old, new)`:
   - adopt the matching passes;
   - refuse when `old` matches more than once;
   - keep the edit a JSON function, not Codex's grammar-constrained free-form tool, which needs the Responses API.

   **Size M.**
2. **A real definition of "pattern" for "Always for this pattern"** (SPEC 8.5, 8.8). Codex's execpolicy works like this:
   - **Rules:** prefix rules over the argument list, each deciding allow, prompt or forbidden. When several match, the strictest wins.
   - **Splitting:** `cmd /c`, `powershell -Command` and `bash -lc` are split into their sub-commands, and each one is checked.
   - **Fail closed:** a command the parser can't reduce to plain words is never allowed by a rule.
   - **Banned prefixes:** the model can propose a prefix, but over-broad ones are refused (`bash -c`, `python`, `node -e`, `git`, `rm`, `pwsh -Command`, `cmd /c`).
   - **Dangerous commands** (forced recursive delete, ShellExecute/URL launches on Windows) always ask.

   Jenab's plan says "every new command pattern needs approval" but never defines a pattern; this is the missing definition. **Size M (spike, then task).**
3. **A Windows write sandbox that needs no admin rights** (SPEC 8.5, 6.7). Codex's "unelevated" mode has three parts:
   - It starts the command with a **write-restricted** token (`DISABLE_MAX_PRIVILEGE | LUA_TOKEN | WRITE_RESTRICTED`).
   - The restricting SIDs include a random per-workspace "capability SID", and the workspace folder gets an allow-write ACE for that SID.
   - Protected sub-folders (`.git`, the agent's own config) get deny-write ACEs.

   The effect is that writes succeed only inside the workspace. This is all possible from Go with `golang.org/x/sys/windows`, which Jenab already uses for Job Objects in `internal/sqlworker/proc_windows.go`. Limits:
   - reads are not isolated;
   - the network is blocked only through proxy environment variables;
   - the ACEs persist on disk and need cleanup.

   **Size L (spike first).**
4. **Command output the model can use** (SPEC 8.5, 3.7). Codex does four things here:
   - **Head and tail.** It keeps the start and the end of the output and drops the middle, with a marker giving the original size. Jenab's previews show only the start ("showing first …"), but for builds and tests the error is at the end.
   - **Short header.** Exit code (or a running-process id), wall time, original size.
   - **Child environment.** It forces `NO_COLOR=1`, `TERM=dumb`, `PAGER=cat`, `GIT_PAGER=cat`.
   - **PowerShell encoding.** It prefixes PowerShell scripts with a line that sets UTF-8 console output. This matters for Persian text.

   **Size S, as part of the `run_command` ticket.**
5. **Run read-only tool calls of one answer in parallel** (SPEC 8.3, 8.1). Codex has two parts:
   - It starts each tool as soon as its call has streamed in, while the rest of the answer is still arriving.
   - A read/write lock lets tools marked parallel-safe run together, while every other tool runs alone.

   Jenab already asks the model for independent calls in one message (TASK-033's line in block 1), but `runTools` in `internal/agent/turn.go` runs them one after another. So the saving in requests doesn't yet save wall time. Jenab could add a `Parallel` flag on `tool.Spec` for `query`, `fetch_page`, `read_ref`, `search_history` and the like:
   - calls that need an approval card stay serial;
   - results are stored in call order.

   **Size M.**
6. **Trust that ends when the file changes** (SPEC 8.9, 8.7). Codex hashes each hook it trusts. When the content changes, the hook is marked "Modified" and has to be approved again. Jenab trusts "skills the user wrote or accepted". The spec doesn't say what happens when a project skill in `skills/` is edited on disk, or arrives inside an imported project. The same rule fits later for MCP server commands and command rules:
   - store the hash of what was accepted;
   - a changed file is unused until the user accepts it again.

   **Size S.**
7. **Honest interrupts** (SPEC 8.3). On *Stop*, Codex:
   - cancels, waits about 100 ms, then aborts hard;
   - writes a note the model reads: the user interrupted on purpose, tools may have partly run, and background processes may still be running.

   Jenab writes "cancelled by user" into each open call. That is enough today, but once commands and file edits exist, a cancelled call may have half-run. Jenab can add two things:
   - the result should say "may have partly run; check before repeating";
   - Codex's in-memory **net per-turn diff**, built from the edits actually applied rather than from git, fits Jenab's planned "diff per file" and *Undo turn*.

   **Size S.**
8. **A breaker for denial loops** (SPEC 8.8). Codex's automated reviewer stops the turn after 3 denials in a row, or 10 in the last 50. Jenab doesn't need the reviewer (see "not for us"), but the breaker suits human approvals once commands arrive: a model that keeps asking for the same denied thing should end the turn, not wear the user down. **Size S.**

Smaller adopts: an **LM Studio preset** (S); MCP **startup and call timeouts kept apart, with errors that name the setting** (folded into Phase 5); **protected paths** for edits (part of idea 1); **safe internal git calls** (if Jenab ever runs git); **storage for secrets over 2,560 bytes** for MCP OAuth later.

**Already as good or better in Jenab:**
- the request cap (Codex has none);
- the provider error kinds and the pause per provider;
- untrusted-content tracking (Codex has only a coarse "polluted" flag for memories);
- session storage and search (Codex greps JSONL files with ripgrep);
- the history window kept stable for the cache;
- pairing every tool call with a result;
- MCP behind an index, which keeps the tool list stable.

**Not for us:**
- the Responses-API-only design (Codex removed Chat Completions entirely);
- the free-form Lark tools;
- remote compaction;
- the LLM "guardian" that approves in the user's place;
- the elevated sandbox users and firewall rules;
- the MITM network proxy;
- JS "code mode";
- default-on vendor telemetry.

---

## Area-by-area table

| # | Area | Codex | Jenab (SPEC) | Verdict |
|---|---|---|---|---|
| 1 | Turn loop and request limits | Loops while the model calls tools or input is pending. No cap on requests or tool calls; stops only on error, token budget, hooks or interrupt (`core/src/session/turn.rs:543-1012`) | `turn_max_requests` 25; the last request has tool choice `none`; 3 bad-call requests in a row stop the turn (8.3) | **Already better** |
| 2 | Tool execution | Each tool starts when its call item completes, while the stream goes on. A read/write lock runs parallel-safe tools together (`core/src/stream_events_utils.rs:351-484`, `core/src/tools/parallel.rs:292-304`) | Calls run in order after the complete answer (`internal/agent/turn.go` `runTools`); a call is stored before it runs (2.3, 3.8) | **Adopt** parallel read-only calls after the complete answer. Not streaming dispatch: Jenab's "tools only after a complete answer" rule makes retries safe (8.3) |
| 3 | Stop / interrupt | Cancel token, 100 ms grace, then hard abort. Writes a `<turn_aborted>` note the model reads; synthetic "aborted by user after Xs" outputs; `TurnAborted` event with a reason (`core/src/tasks/mod.rs:75, 1047-1179`, `core/src/context/turn_aborted.rs:12`) | Open calls get "cancelled by user"; streamed text kept and marked; subagents cancelled (8.3) | **Adopt** (small) wording for half-run commands and edits; grace then kill the process tree |
| 4 | Retries | Request retries 4, stream retries 5, 200 ms exponential backoff with jitter, honours `Retry-After`; 429 not retried at HTTP level (`model-provider-info/src/lib.rs:73-82, 499-505`, `codex-client/src/retry.rs`, `core/src/responses_retry.rs`) | Error kinds, capped `retry-after`, 10→100 s waits, one pause per provider, 10-minute turn budget, *Retry now* (3.8, 8.3) | **Already better** |
| 5 | Steering (messages during a turn) | Steers join the running turn's queue with a turn-id check. With `InstantInterrupt`, new input cancels the current sampling (`core/src/session/turn_input.rs:953-1022`, `input_queue.rs:322-342`) | Joins at the next step boundary (8.3) | **Not now**: pre-emption throws away paid tokens and partial text; revisit only if users complain about slow reactions |
| 6 | History in the prompt | Tool output cut at record time, head+tail 50/50, about 10k tokens; full output kept in the rollout file. Missing outputs get a synthetic "aborted" output with a deterministic ID; orphan outputs dropped (`core/src/context_manager/normalize.rs:21-169`, `utils/output-truncation`) | Previews (head) plus ref, stubs at cuts, in-turn trimming; app-closed calls get a result (`internal/agent/context.go:205-218`) (3.6, 3.7) | **Already have**; **adopt** head+tail for command output only (idea 4) |
| 7 | Compaction | Automatic LLM summary at 90 % of the window. Keeps up to 20k tokens of recent user messages plus a summary message; re-injects instructions. Remote encrypted compaction for OpenAI only (`core/src/compact.rs`, `compact_remote_v2.rs`, `protocol/src/openai_models.rs:579`) | Deliberately no automatic compaction: cuts plus *Refresh memory* into capped notes (3.5, 3.6, 11) | **Not for us**; Jenab's design was measured (P2-13) |
| 8 | Prompt caching | `prompt_cache_key`; deterministic UUIDv5 IDs for synthetic items; environment and AGENTS.md as "world state" sections that append diffs, never rewrite (`core/src/client.rs:712-735`, `core/src/context/world_state/*.rs`) | Blocks rewritten only at cuts; notices in between; tool-call IDs hashed the same in every request (3.1, 3.8) | **Already have** (same idea, different words) |
| 9 | Token counting | No tokenizer: last reported usage plus bytes/4 for newer items; unknown models get a 272k fallback window (`core/src/context_manager/history.rs:1143-1167`, `models-manager/src/model_info.rs:96-140`) | Own estimate, compared with reported `prompt_tokens` to catch silent truncation; the user enters unknown windows (3.8, 3.9) | **Already better** (the 272k fallback would hide Ollama truncation) |
| 10 | Project instructions (AGENTS.md) | Walks from repo root to cwd: `AGENTS.override.md`, `AGENTS.md`, fallbacks; 32 KiB budget; skipped for untrusted projects (`core/src/agents_md.rs`) | Workspace files are untrusted data (8.5); instructions come from memory, roles and skills | **Not as instructions.** Optional later: *Import as project skill* for a workspace's AGENTS.md, through the existing import approval card (8.9) |
| 11 | Memories | Off by default. A background job extracts from idle sessions with a cheap model, consolidates into `MEMORY.md` and summaries, injects a 2,500-token summary. `disable_on_external_context` marks threads that used web or MCP (`memories/`, `ext/memories/`) | Structured memory with caps, revisions, undo, the untrusted chip line, Strict asks (3.3–3.5) | **Already better** for safety and control. Not adopted: memory from idle sessions with no user action (SPEC 11 lists automatic *Refresh memory* as not in v1) |
| 12 | Approval model | `AskForApproval` = untrusted / on-request / granular / never, kept separate from the sandbox profile (read-only, workspace, full). Decisions: once, for session, persist prefix rule, network rule (`protocol/src/protocol.rs:1167, 4535`, `protocol/src/approvals.rs`) | Strict, Standard, Auto; Once, Always, Deny; newest decision counts; the untrusted-content rule (8.8) | **Already have** the levels; **adopt** the decision types for commands (idea 2) |
| 13 | Exec policy | Starlark `prefix_rule`/`network_rule` files; strictest wins; amendments appended to a rules file; banned over-broad prefixes; shell wrappers split before matching (`execpolicy/src/*`, `core/src/exec_policy.rs:64-153, 746-1001`) | "Always for this pattern", undefined (8.5, 8.8) | **Adopt** the idea, not the Starlark: rules are data in `_jenab_approvals` (idea 2) |
| 14 | Dangerous commands | Heuristics: forced `rm`, recursion through `sudo`, `env`, `bash -lc`. On Windows, tree-sitter parsing of PowerShell and cmd that flags URL/ShellExecute launches (`shell-command/src/command_safety/*`) | Not specified | **Adopt** a short list that always asks and never offers *Always* |
| 15 | LLM approver ("guardian") | A reviewer subagent can replace the human: JSON risk verdict, fails closed, 90 s timeout, breaker after 3 or 10 denials. Only user messages and AGENTS.md can authorize; tool output is evidence (`ext/guardian-reviewer/src/*`, `guardian-context/src/lib.rs`) | The user approves; Auto approves only before untrusted content (8.8) | **Not for us**: it loosens approvals and depends on the model. **Adopt** only the breaker (idea 8) and the authorization rule, which Jenab already follows |
| 16 | Windows sandbox | Unelevated: write-restricted token plus capability SID plus workspace ACEs, network blocked only through environment variables. Elevated: sandbox users, DPAPI, private desktop, firewall plus WFP. Also MXC (`windows-sandbox-rs/src/token.rs:468-525`, `cap.rs`, `acl.rs`, `env.rs:126-177`, `setup_provisioning/*`) | Job Objects, below-normal priority, memory checks; no file sandbox planned (8.5) | **Adopt** unelevated mode via a spike (idea 3); **not for us**: the elevated mode |
| 17 | Network from commands | Local HTTP and SOCKS proxy with an allowlist and MITM, plus proxy environment variables; OS-enforced only in elevated mode (`network-proxy/`, `core/src/tools/network_approval.rs`) | The 6.7 host and private-network rules cover Jenab's own fetches only | **Not for us** (MITM CA, heavy). **Write into SPEC** that commands' network is not controlled, and that "after untrusted content every command asks" is the guard |
| 18 | Edit / patch tool | `apply_patch`: anchored on context lines, the four matching passes, verify everything before writing, partial-apply report, exact error strings. Free-form Lark tool for OpenAI; other models send it through the shell (`apply-patch/src/*`, `core/src/safety.rs:67-125`) | `edit_file(path, old, new)`, `write_file`; before-copy in the bucket; undo by hash (8.5) | **Adopt** the matching and error design (idea 1); **not for us**: the free-form tool and catching patches inside shell commands |
| 19 | Protected paths | `.git`, `.agents`, `.codex`, `.aws` stay read-only inside writable roots (`protocol/src/permissions.rs:43-53`) | Credential files blocked for reading (8.5) | **Adopt** for edits: `.git/`, Jenab's data and project folders, and files that run code later (git hooks) |
| 20 | Shell tool | `cmd` as a string; argv chosen per shell (`pwsh -NoProfile -Command`, `cmd /c`); UTF-8 prefix for PowerShell; forced environment; Windows safety lines in the description; `justification` and `prefix_rule` fields (`core/src/tools/handlers/shell_spec.rs`, `shell-command/src/powershell.rs`) | `run_command(cmd, needs_memory_mb)` (8.5) | **Adopt** `why` (shown on the card), a proposed pattern, the forced environment, the UTF-8 prefix. OS lines in the description are allowed: they depend on the OS, not the model |
| 21 | Long-running processes | `exec_command` returns after a yield time with a session id; `write_stdin` with empty text polls; at most 64 processes, least-recently-used eviction (`core/src/unified_exec/*`) | Not planned (8.5 covers one-shot commands) | **Later**: a dev server or watcher needs it; keep it out of the first `run_command` |
| 22 | Remote exec server | JSON-RPC process and file server, so tools can run elsewhere (`exec-server*`) | One machine (7.7) | **Not for us**; but keep `run_command` and the edits behind a small Go interface |
| 23 | Turn diff | Net per-turn unified diff built in memory from applied edits, 100 ms budget (`core/src/turn_diff_tracker.rs`) | "The chat shows a diff per file" (8.5) | **Adopt** (idea 7), built from the bucket before-copy and the new content |
| 24 | Git, worktrees, undo | Ghost-commit undo **removed**. Worktrees made with git hooks, fsmonitor and filters disabled (`worktree/src/git.rs:1-11`, `features/src/lib.rs:1172`) | Undo through the change log and before-copies; worktrees not planned (8.5) | **Already better** for undo. **Adopt** the safe-git rule if Jenab ever runs git itself |
| 25 | MCP client | stdio plus streamable HTTP; full OAuth with keyring and a file fallback; `mcp__server__tool` names, sanitized and hashed; startup 30 s and call 300 s timeouts; annotation-based approval modes; elicitation; resources; BM25 `tool_search` for deferred tools (`codex-mcp/`, `rmcp-client/`) | Index tools `mcp_describe`/`mcp_call`; approval with hint rules after untrusted content; 60 s per call; no resources yet (8.7) | **Already have** the core (index plus stricter approvals). **Adopt** separate startup and call timeouts, errors naming the setting, and a cached tool list so the card shows tools before the server starts. Elicitation: decline by default when it comes |
| 26 | Skills | Catalog of name, description and path in the prompt (2 % of the window, descriptions cut at 1,024 chars); the model reads SKILL.md with file tools; found in `.agents/skills` from root to cwd (`ext/skills/src/*`, `skills/src/parser.rs`) | `## Skills` list, `load_skill`, `load_with`, caps, measured in SPIKE-025 (8.9) | **Already better**; **adopt** hash-based trust (idea 6) |
| 27 | Hooks | 14 events; command handlers get JSON on stdin; exit 2 blocks; can rewrite tool input; trust by hash (`hooks/`, `protocol/src/hooks.rs`) | "Not planned" (8.5) | **Not for us** (user-written code beside the agent loop); the trust-by-hash idea moves to skills |
| 28 | Agent roles and subagents | Roles are full config layers; `spawn_agent`, `wait_agent`, … ; at most 6 threads, depth 1; the child inherits approvals and sandbox; optional history fork; a notice to the parent when it ends (`agent-roles/`, `core/src/agent/*`) | Roles, Mother, subagents with one level, 5 per chat, finish notices, no polling (8.3, 8.6) | **Already have**; Jenab's no-polling rule is stronger than `wait_agent` |
| 29 | Providers | Responses API only (Chat Completions removed, `model-provider-info/src/lib.rs:110-143`); built-in OpenAI, Bedrock, Vertex, Ollama, LM Studio; `env_key`; command-based tokens | Five kinds incl. Chat Completions and native Ollama; presets fill only kind and base URL (3.8, 3.9) | **Already better** for model-neutrality |
| 30 | Local models | Ollama: version probe (≥ 0.13.4) and automatic `/api/pull`. LM Studio: `lms` download plus a 1-token warm-up request. Neither starts the server (`ollama/`, `lmstudio/`) | Native Ollama with `num_ctx`; `localhost:11434` listed first at first run (3.8, 3.9) | **Adopt** an LM Studio preset (`openai_compatible`, `http://localhost:1234/v1`). **Not for us**: automatic pull (Jenab never downloads) |
| 31 | Secrets | `keyring` crate; on Windows large blobs go in an age-encrypted file whose random key is in the keyring; regex scrubber for `sk-…`, Bearer and AWS keys (`secrets/src/local.rs`, `sanitizer.rs`) | go-keyring, values 1–2,560 bytes, redaction by value (6.7) | **Adopt later** for MCP OAuth tokens over 2,560 bytes: an encrypted file whose key is in the keychain, using the Go standard library or DPAPI (no new dependency). A pattern scrubber for logs is optional |
| 32 | Session storage and resume | JSONL "rollouts" are the source of truth, with SQLite as a rebuildable index; resume scans back to the last compaction; ripgrep search; zstd after 7 days (`rollout/`, `thread-store/`, `state/`) | SQLite `chats.db` with FTS5 for three languages, rewind with undo, export (2.3) | **Already better** |
| 33 | Telemetry and feedback | Statsig metrics on by default in release builds; Sentry upload of logs plus transcript on request (`otel/src/config.rs`, `feedback/`) | Local only; turn inspector (8.4) | **Not for us** |
| 34 | Code mode | Model-written JS in a V8 isolate calls tools (`code-mode*`) | Starlark later for pipelines (6.5) | **Not for us** (new runtime, model-specific use) |
| 35 | Project trust | "Trust this folder"; untrusted projects lose project config, rules, hooks, AGENTS.md (`config/src/loader/mod.rs:1146-1163`) | The user links one folder; its files are untrusted data (8.5) | **Already have** in spirit; the rule matters again once project-local command rules exist |
| 36 | Config layering and feature flags | Numeric layer precedence (system, user, profile, project, session); about 250 staged feature flags (`config/src/config_layer_source.rs`, `features/`) | `config.yaml` plus per-project settings | **Not for us** now |

---

## Notes by area

### Agent loop, Stop, retries, limits (SPEC 8.3, 3.8, 7.6)
- **No request cap in Codex** (`core/src/session/turn.rs:761`). One comment says compaction should keep the loop from running forever. Jenab's SPIKE-023 showed models poll until a cap, so Jenab's cap and its no-polling answer stay; nothing to take here.
- **Stop.** Codex writes a model-readable `<turn_aborted>` marker (`core/src/context/turn_aborted.rs`). Today Jenab tells the model nothing beyond "cancelled by user" results; a stopped turn followed by "go on" works because the history is valid. Once Phase 6 exists:
  - a cancelled `run_command` result should say it may have partly run, and how long it ran;
  - a cancelled edit should name the files already written.

  Codex's patch result carries exactly this (`AppliedPatchDelta`, `apply-patch/src/lib.rs:213-304`).
- **Retries.** Two Codex choices to note but not adopt:
  - retrying `ContentFilter` errors with extra guidance in the history (model-specific steering);
  - switching transport (WebSocket→HTTPS), which is OpenAI-specific.
- **Parallel tools** (idea 5). Codex's lock is a single `RwLock`: parallel-safe tools take a read lock, the rest take the write lock (`core/src/tools/parallel.rs:292-304`). In Jenab:
  - a `Parallel bool` on `tool.Spec`;
  - `runTools` starts the parallel ones together and stores results in call order (the stored order must stay stable for the cache and for undo);
  - any call that may show a card runs alone.

  Writes already queue on the writer (7.3), but keep `insert` and memory edits serial so a failure stays easy to attribute.

### Context (SPEC 3.1, 3.6, 3.7)
- Codex and Jenab reached the same cache rule independently: append-only between resets, with changes sent as small appended messages. Codex's "world state" sections (`core/src/context/world_state/*.rs`) are Jenab's notices folded at cuts.
- **Head+tail cutting.** Codex keeps head and tail at 50/50 with a `…N tokens truncated…` marker (`utils/string/src/truncate.rs`). Jenab's preview shows the head with a ref, which is right for pages and documents. For command output use head plus tail, and keep the full output in the bucket with a ref, so `read_ref` and `search_ref` still work.
- **Compaction** is the main difference in philosophy. Codex summarizes automatically at 90 %. Jenab cuts and lets the user start *Refresh memory*. Jenab's choice is measured (P2-13, TASK-027), so no change.

### Approvals, exec policy, sandbox (SPEC 6.7, 8.5, 8.8, 8.10)
- **Two separate settings in Codex:** *when to ask* (approval policy) and *what the OS enforces* (permission profile). With a working sandbox, Codex lets non-dangerous commands run without asking under its default `on-request`, because the sandbox enforces the writable roots (`core/src/exec_policy.rs:865-950`).
  - Jenab's plan asks for every new pattern even at Auto. That is stricter and right without a sandbox.
  - If the write sandbox (idea 3) lands, a later decision could let Auto skip asking for commands that touch only the workspace, never after untrusted content. Any such loosening needs its own spike and a strong reason (8.8).
- **What a pattern is** (idea 2), proposed for the spike:
  - **Shape:** a pattern is the command's program plus its leading literal words, e.g. `git status`, `go test`, `bun run build`.
  - **Splitting:** compound commands (`&&`, `;`, `|`) are split and each part must match.
  - **Not covered:** a command with redirection, substitution (`$(…)`, backticks), script blocks or variables isn't covered by any pattern and asks *Once*.
  - **Banned:** interpreters with inline code (`python -c`, `node -e`, `pwsh -Command`, `cmd /c`), bare `git`, `rm`/`del`/`Remove-Item` and shells can never get *Always*.
  - **Model's proposal:** the model may propose the pattern in the call (Codex's `prefix_rule`), and the card shows it for the user to accept or narrow.

  Codex parses PowerShell with tree-sitter plus a long-lived real PowerShell parser process (`shell-command/src/command_safety/powershell_parser.rs`). Jenab would need its own Go tokenizer that fails closed, because tree-sitter means cgo or a new dependency. That is a real spike question: how much PowerShell syntax to accept before failing closed.
- **Windows sandbox** (idea 3). Codex's elevated mode creates local users, stores their passwords with DPAPI, adds firewall and WFP rules, and runs a UAC setup helper plus a service. That is far too heavy for a desktop app Jenab ships, and would also be "modifying system settings" on the user's machine. The unelevated mode is the useful part. Spike questions:
  - Does a write-restricted token break common tools (`go build` cache in `%LOCALAPPDATA%`, `bun install` cache, git's `index.lock`)? Codex grants extra write roots for temp folders (`exclude_tmpdir_env_var`).
  - How to remove the persisted ACEs when a workspace is unlinked?
  - Does it combine with the existing Job Object, below-normal priority and memory checks?
- **Guardian.** Codex can let a second LLM approve on the user's behalf. Jenab's rules make the user the approver, and the reviewer's quality depends on the model, so this is not for us. Two parts carry over:
  - **Authorization comes only from the user's messages.** Tool output, skill text and the model's own text are evidence (`ext/guardian-reviewer/src/instructions.rs:12-23`). Jenab's 8.8 untrusted rule already says this.
  - **The breaker.** 3 denials in a row ends the turn (`ext/guardian-reviewer/src/circuit_breaker.rs:3-7`).

### Coding tools (SPEC 8.5)
- **Edit matching** (`apply-patch/src/seek_sequence.rs:12-110`): exact, then `trim_end`, then `trim`, then Unicode punctuation folded. Codex keeps the file's own line endings (`apply-patch/src/file_update.rs:124-152`). For Jenab's `edit_file(path, old, new)`:
  - Find `old` with the passes in order; the first pass that matches exactly once wins.
  - Several matches → error naming the line numbers and asking for more context.
  - No match → error showing the closest region. Codex shows the lines it expected (`Failed to find expected lines in <path>:\n…`).
  - Keep the file's EOL style and BOM; write to a temp file and rename it into place.
  - An edit list (several `old`/`new` pairs in one file) is checked whole before anything is written.
- **Codex has no str_replace tool**: non-OpenAI models edit through `apply_patch` heredocs in the shell. Whether Jenab's reference models do better with `old`/`new` pairs or with a context-anchored patch is an open question for the spike; Jenab's model-neutral rule means the same tool for all.
- **Protected paths.** Codex keeps `.git`, `.agents`, `.codex` and `.aws` read-only inside writable roots, so the agent can't plant a git hook or change its own config. For Jenab, `edit_file` and `write_file` should refuse:
  - `.git/`;
  - any Jenab data or project folder;
  - the credential patterns already blocked for reading.
- **Commands.** Two Codex lines are worth copying in spirit:
  - Codex adds Windows-only lines to the tool description: don't mix shells for deletes, check targets before recursive operations, start hidden processes with `Start-Process -WindowStyle Hidden` (`core/src/tools/handlers/shell_spec.rs:332-337`). These depend on the OS, not the model, so they fit Jenab's rule.
  - The one-shot default timeout is 10 s (`core/src/exec.rs:61`).

### MCP (SPEC 8.7)
- Codex sends MCP tools as native tools, or defers them behind a BM25 `tool_search` (`core/src/mcp_tool_exposure.rs`). Jenab's index (`mcp_describe` and `mcp_call`) was measured in SPIKE-024; keep it.
- Approval modes `auto|prompt|writes|approve` read the hints (`core/src/mcp_tool_call.rs:2754-2785`). Jenab's rules are stricter: no *Always* after untrusted content for destructive or open-world tools. Keep Jenab's.
- Worth folding into the Phase 5 MCP ticket:
  - separate startup (30 s) and call timeouts;
  - startup errors that name the setting to change (`codex-mcp/src/connection_manager/startup.rs:138`);
  - a cached tool catalog so the project card lists tools before the server is started (`startup_readiness = "catalog"`);
  - OAuth tokens kept in the keychain with a fallback;
  - elicitation declined unless the user answers.

### Skills, memories, instructions, roles (SPEC 3.3–3.5, 8.6, 8.9)
- **Skills.** Codex's catalog is budgeted to about 2 % of the window, with long descriptions cut (`ext/skills/src/render.rs:23-33`). Jenab's 200-character description cap and the SPIKE-025 measurements already cover this.
- **Hash-based trust** (idea 6) is the one gap found: SPEC 8.9 doesn't say what happens when a project skill changes on disk or comes with an imported project.
- **Memories.** Codex's memories have a `disable_on_external_context` switch that stops memory generation for threads that used web or MCP (`config/src/types.rs:324-360`). Jenab's per-edit chip line plus *undo* is finer-grained. Nothing to take.
- **Roles and subagents.** Both stop at one level of depth and a small count. Codex's `fork_turns: all` (a child starts with the parent's whole history) costs tokens and breaks Jenab's "subagent gets the task and its inputs" rule; not for us.

### Providers and local models (SPEC 3.8, 3.9)
- Codex removed Chat Completions in favour of the Responses API, and the `ollama-chat` provider with it (`model-provider-info/src/lib.rs:110-143`). Jenab's multi-protocol layer is a deliberate strength; nothing to take.
- **LM Studio** is OpenAI-compatible at `localhost:1234/v1`. Two options, both allowed by "presets fill only kind and base URL":
  - a preset;
  - listing it at first run next to Ollama when it answers.

  Codex warms the model with a 1-token request in the background. Jenab's first-event timeout (10 minutes for Ollama) would need the same allowance for LM Studio; record it, don't guess from the URL.

### Secrets (SPEC 6.7)
- Codex works around the Windows Credential Manager size limit by keeping an encrypted file whose random key sits in the keyring (`secrets/src/local.rs`). Jenab caps secret values at 2,560 bytes, which MCP OAuth tokens (Phase 5) can exceed. When that comes, use the same split with the standard library's AES-GCM or DPAPI (`CryptProtectData` via `x/sys/windows`): no new dependency.

### Storage, resume and the rest
- **Storage.** Codex's JSONL rollouts are the source of truth, with SQLite as an index (`state/`). Jenab's SQLite with FTS5, rewind with undo, and export are ahead of this.
- **Telemetry.** Codex sends Statsig metrics by default in release builds (`otel/src/config.rs`), and feedback uploads logs and the transcript to Sentry. Both are against Jenab's local-first stance.
- **Code mode** (JS in V8 calling tools) is interesting as research next to SPIKE-031's Thoughts-Graph, but it brings a JS runtime and is tuned for specific models. Not for us.

---

## Proposed tickets

Numbers are suggestions; the last ones in Tickets/README are SPIKE-040 and TASK-041.

| ID | Title | Goal | SPEC | Size |
|---|---|---|---|---|
| SPIKE-041 | Edit tool: matching and shape | Compare `edit_file(path, old, new)` (with an edit list) and a context-anchored multi-hunk patch on the reference models. Use Codex-style matching passes, a refusal on several matches, errors that show the closest lines, and check-all-then-write. Decide the tool, its errors and the protected paths | 8.5, 2.6, 8.8 | M |
| SPIKE-042 | Command patterns and approvals | Define a "pattern" (program plus leading literal words); a Go tokenizer for cmd and PowerShell that fails closed; banned prefixes; a dangerous list that always asks; `why` and proposed-pattern fields; a breaker after 3 denials. Measure on real commands from the reference models | 8.5, 8.8 | M |
| SPIKE-043 | Windows write sandbox without admin | Write-restricted token plus a per-workspace capability SID plus workspace ACEs, from Go; check `go`, `bun` and `git` work; ACE cleanup on unlink; fits with the Job Object, priority and memory checks. Document that network and reads aren't isolated | 8.5, 6.7 | L |
| TASK-042 | `run_command` output and environment | Head+tail preview with the full output as a ref; header with exit code, wall time and size; `NO_COLOR`, `TERM=dumb`, `PAGER`/`GIT_PAGER=cat`; PowerShell UTF-8 prefix; "may have partly run" on *Stop*; grace period, then kill the tree. Part of the Phase 6 command ticket | 8.5, 3.7, 8.3 | S |
| TASK-043 | Run read-only tool calls of one answer in parallel | `Parallel` flag on `tool.Spec`; concurrent runs for flagged tools; results stored in call order; card tools serial; scenario runner measures wall time | 8.3, 8.1 | M |
| TASK-044 | Skills trusted by hash | Store the accepted hash of each user and project skill; a changed or imported skill is unused until accepted again in the import card; the same mechanism later for MCP server commands and command rules | 8.9, 8.7 | S |
| TASK-045 | Net diff per turn | Build the turn's per-file diff from the bucket before-copy and the written content, without git; show it in the chat and use it for *Undo turn* conflicts. Part of the Phase 6 editing ticket | 8.5, 2.6 | S |
| TASK-046 | LM Studio preset | `openai_compatible` preset with `http://localhost:1234/v1`; optional first-run listing when it answers; note on first-event timeouts | 3.9 | S |
| (Phase 5 notes) | MCP details from Codex | Separate startup and call timeouts; errors that name the setting; cached tool list for the card; decline elicitation by default; secrets over 2,560 bytes in an encrypted file with its key in the keychain | 8.7, 6.7 | in the MCP ticket |
| (SPEC note) | Commands' network is not controlled | State in 8.5 that 6.7's host and private-network rules can't reach a command's own connections; the guard is "after untrusted content every command asks" | 8.5, 6.7 | doc only |

---

## Licence note

Codex is Apache-2.0. Two pieces would be tempting to port line by line:
- `apply-patch/src/seek_sequence.rs` together with the patch parser;
- the restricted-token and ACL code in `windows-sandbox-rs/src/token.rs`, `cap.rs` and `acl.rs`.

A port would need Codex's NOTICE and attribution, and a statement of changes in each derived file. Jenab already ships agent-desktop's NOTICE, so the mechanism exists. Writing them fresh from the descriptions in this report is cleaner, and none of them is large.

## Not covered

- TUI, app-server and its protocol, cloud tasks, realtime/voice, the `sdk/` and `codex-cli` npm wrapper, Bazel, `responses-api-proxy`, `rollout-trace`, `response-debug-context`, login and ChatGPT auth.
- The message board, the plugin system and `connectors` (ChatGPT Apps) beyond one paragraph each.
- The Linux and macOS sandboxes beyond one line, since Jenab is Windows-first and Linux is a beta.
- The guardian prompt templates and the memory prompts: read for their structure only.
- Line numbers come from the research passes. I spot-checked eight of the claims above against the source; others may have shifted by a few lines.
- No Codex code was run, so the runtime behaviour (e.g. whether the unelevated sandbox breaks common tools) is inferred from the code, not observed.
