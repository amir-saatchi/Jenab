# P3-10 — Pages in the shell: the Pages tab, the dock and the layout per chat
**Type:** Feature
**Status:** Open
**Gate:** 3 (Phase 3)
**Needs:** P3-09, P3-03
**Requirements:** R-46, R-49

## Goal
Pages are listed in the right sidebar, open in the main area with the chat in the dock, and each chat remembers its own layout (SPEC 5.9, 5.12).

## Scope
- **Pages tab:** the list of pages (title, description), live as they change. A click opens the page in the main area with the chat in the dock, as a file does now (`kit.Split`). Saved views aren't listed.
- **Dock:** left or right, hide, resize (width saved with the side), close the page for the full chat back. Below 900 px the chat is a drawer over the page (5.12).
- **Layout per chat (5.9):** `chats.ui_state` keeps the open page or file, its filter values, the dock side and width, the right-sidebar tab and whether the chat list shows. Switching chats restores it. Opening a project opens its last active chat (Mother the first time). A new chat starts full width or with its `default_page`. The same page can be open in several chats with their own filter values. A deleted page closes and the chat goes back to full width.
- **The agent knows the open page:** opening a page or changing its filters adds a notice (3.1), e.g. `[user opened page bitcoin: from_date = 2026-07-01]`, batched as other notices are.
- **`open_page`:** the tool from P3-03 opens the page in the agent's own chat.
- **Rail and shortcuts (5.12):** while a page is open the chat list moves to the rail as tiles; *Ctrl+Tab* and *Alt+←/→* include pages.
- **Pipelines tab:** its icon stays, with an empty state until Phase 4.

## Done when
- Headless tests: open and close a page, dock side and width, restore on chat switch and after a restart, `default_page`, the notice text, `open_page`, a deleted page, the narrow drawer.
