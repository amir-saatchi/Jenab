# Gate-3 — Readiness
**Status:** In progress

## Purpose
What must be true before Phase 1 coding starts.

## Content
- **Phase 1 tickets:** P1-01 to P1-18, plus TASK-001, in the [ticket list](../Tickets/README.md#phase-1). Each has a goal, a scope, "done when" and the requirements it covers.
- **Dev environment:** [DEVELOPMENT.md](../DEVELOPMENT.md).
- **Repository and CI:** `go.mod`, the first window and `.github/workflows/ci.yml` ([P1-01](../Tickets/P1-01-repository-and-ci.md)). The box is ticked when the first CI run passes on all three platforms.

## Exit criteria
- [x] Phase 1 tickets written, each with "done when" (P1-01 to P1-18, TASK-001)
- [ ] Repository, build and CI set up for Windows, macOS and Linux (beta); `-race` runs on Linux and macOS
- [x] Blocking open questions closed (Wails version: SPIKE-003; Phase 1 providers: decided 2026-09-29, SPEC 3.9)
- [x] License decided before the repository goes public or takes an outside contribution: Apache 2.0, 2026-10-01 (PROPOSAL §10); `LICENSE` and `NOTICE` added with the repository
- [x] Project name checked and chosen before the repository is created: Jenab, 2026-10-01 (PROPOSAL §13); formal trademark search before the public release
- [x] Dev environment setup documented ([DEVELOPMENT.md](../DEVELOPMENT.md))

## Open items
- ~~[SPIKE-023](../Tickets/SPIKE-023-multi-part-requests.md)~~: done; decided 2026-09-29 (SPEC 8.3, 8.6)
- ~~[SPIKE-025](../Tickets/SPIKE-025-skills.md)~~: done; decided 2026-09-29 (SPEC 8.9)
- [TASK-001](../Tickets/TASK-001-scenario-runner.md): scenario runner, built in Phase 1 (SPEC 8.4)
