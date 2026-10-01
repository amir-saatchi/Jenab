# Gate-2 — Design
**Status:** Low fidelity: reviewed on 2026-09-29; the agreed proposals are in the SPEC. High fidelity: done, reviewed on 2026-09-29 ([TASK-002](../../Tickets/TASK-002-high-fidelity-design.md), [high-fidelity/](high-fidelity/README.md)).

## Low fidelity
Grayscale wireframes, one SVG per screen, in `low-fidelity/`.
- **Numbered notes:** each screen explains its parts, lists its open questions and names the requirements it covers (Gate-1).
- **Dashed outline:** something proposed here that isn't in the SPEC yet.
- **Content:** the Bitcoin example from SPEC 9.

| # | Screen | Covers |
|---|---|---|
| 01 | [App shell, full chat](low-fidelity/01-app-shell.svg) | Sidebars, chat list with statuses, tool chips, composer |
| 02 | [Page open, chat docked](low-fidelity/02-page-docked.svg) | Page header, grid rows, `stat`, `card`, bulk row action, dock |
| 03 | [Anatomy of a turn](low-fidelity/03-turn-anatomy.svg) | Quick part first, background work, finish turn, message during a turn |
| 04 | [Approval card and question form](low-fidelity/04-approvals-and-questions.svg) | Host, migration and connection approvals, `ask_user`, badge, notification |
| 05 | [Mother delegates](low-fidelity/05-mother-delegates.svg) | `send_to_chat`, `create_chat`, "from Mother", finish notice |
| 06 | [Pipelines and run log](low-fidelity/06-pipelines.svg) | Pipelines tab; the Steps view: runs, steps, *Revert run*, failure flag |
| 06-A | [Pipeline as a flow](low-fidelity/06a-pipeline-flow.svg) | The same page as a read-only flow: trigger, steps, `for_each` frame, what each step touches, results of the selected run |
| 07 | [Settings](low-fidelity/07-settings.svg) | Providers and catalog models, default and fast model, limits, theme, updates, search, tray menu |
| 08 | [Project settings](low-fidelity/08-project-settings.svg) | Folder warning, workspace, storage and undo size, hosts, export |
| 09 | [First run](low-fidelity/09-first-run.svg) | No wizard: *Create project*, then a provider card in the chat, then the model picker |
| 10 | [Turn inspector](low-fidelity/10-turn-inspector.svg) | Timeline, tokens and cache per request, context blocks |
| 11 | [Narrow window](low-fidelity/11-narrow-window.svg) | Stacked rows, sidebars and chat as overlays |

**Agreed and added to the SPEC:**
- A **Pipelines** tab and a pipeline page, with a read-only Flow view (the default) and a Steps view, and *Pause schedule* (06, 06-A; SPEC 6.10).
- A model picker per chat in the composer (01; SPEC 3.9).
- *Refresh memory* in the chat header (01; SPEC 3.5).
- First run without a wizard, with starter prompts in a new Mother chat (09; SPEC 3.9).
- The left sidebar folds to icons while a page is open (02; SPEC 5.12).
- Project settings opened from the project switcher (08; SPEC 5.12).
- A bar above the composer while an approval waits out of view (04; SPEC 8.8).
- A minimum window size of 640 × 480, and overlays in narrow windows (11; SPEC 5.12).

Nothing proposed is left to review.

**Main open questions:** all decided in the high-fidelity review on 2026-09-29 and written into the SPEC (3.9, 5.8, 5.12, 8.4, 8.6). See [high-fidelity/](high-fidelity/README.md#decisions).

## High fidelity
Next, in [TASK-002](../../Tickets/TASK-002-high-fidelity-design.md): read every available shadcn/ui component first, then mock up the agreed screens with them, in light and dark. Results, the inventory and the mapping are in [high-fidelity/](high-fidelity/README.md).
