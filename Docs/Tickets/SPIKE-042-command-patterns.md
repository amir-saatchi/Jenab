# SPIKE-042 — Commands: patterns, approvals and output
**Type:** Spike
**Status:** Open
**Gate:** 3 (Phase 6)
**Needs:** —

## Question
SPEC 8.5 says `run_command` comes "only after its own spike", and 8.8 offers *Always for this pattern* without saying what a pattern is. What is a pattern, which commands can never get *Always*, and how does Jenab tell them apart on Windows without failing open?

## Scope
1. **Pattern:** the program plus its leading literal words (`go test`, `bun run build`, `git status`). Compound commands (`&&`, `;`, `|`) are split, and each part must match.
2. **Fail closed:** redirection, substitution, script blocks or variables are covered by no pattern and ask *Once*. A Go tokenizer for cmd and PowerShell, with no cgo or new parser dependency. How much syntax can it accept before failing closed?
3. **Never *Always*:** shells and interpreters with inline code (`cmd /c`, `pwsh -Command`, `python -c`, `node -e`), bare `git`, and delete commands (`rm`, `del`, `Remove-Item`).
4. **Dangerous list:** forced recursive deletes, ShellExecute and URL launches. They ask every time, at every level.
5. **Fields:** `why` (shown on the card) and a proposed pattern that the user can accept or narrow. Rules are stored as data in `_jenab_approvals`.
6. **Breaker:** after 3 denials in a row in a turn, the turn ends with a notice. Should it apply to every card (8.8), not only commands?
7. **Measure** on real commands the reference models write for the Phase 6 "fix a failing test" task: how many ask, how many are covered, and any command a pattern let through that it shouldn't have.
8. **Output and environment** (SPEC note B): check them on Windows: code pages, PowerShell output, ANSI colours.

## Rules
- The same tool, description and rules for every model (SPEC 1). Lines that depend on the OS are allowed in the description; lines for a model are not.
- Stricter than today, never looser: after untrusted content every command asks (8.8).
- Windows first; bash (Git Bash, WSL) after.

## Done when
- The pattern rules, the never-*Always* list, the dangerous list and the breaker are decided, with the measurements.
- The decision is written as SPEC 8.5 and 8.8 text, ready for the `run_command` ticket.
