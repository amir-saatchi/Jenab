# TASK-002 — High-fidelity design with shadcn/ui
**Type:** Task
**Status:** Done (2026-09-29; reviewed, decisions in the SPEC; see [high-fidelity/](../Gates/Gate-2-Design/high-fidelity/README.md))
**Gate:** 2 (after the low-fidelity review)

## Goal
High-fidelity mockups of the agreed low-fidelity screens ([Gate-2 Design](../Gates/Gate-2-Design/README.md)), built from shadcn/ui components, in light and dark (SPEC 5.11).

## Steps
1. **Inventory:** read every shadcn/ui component available at the time, including charts and blocks. For each: what Jenab would use it for, or "not needed".
   - Use the shadcn CLI (`bunx shadcn search`, `docs`, `view`) or the shadcn MCP server, not the website by hand.
   - The shadcn skill for Claude Code is installed in the repo (`.claude/skills/`).
   - **Base library — Decided (2026-09-29):** Radix (`radix-nova` style), as most examples and blocks use it. React Aria stays the fallback if the checks in step 5 show gaps in keyboard, screen-reader or right-to-left support.
2. **Mapping:** map each wireframe element to a component, e.g.:
   - chat thread → Message Scroller, Message and Bubble; cuts and notices → Marker; files → Attachment. Their streaming follow and jump-to-latest are built in, but they must still meet SPEC 5.8: 60 fps at 1,000 tokens/s, and a 200-message chat opening without blocking (virtualised or `content-visibility: auto`).
   - chat list → Sidebar
   - tool chip → Collapsible and Badge
   - approval card → Card and Button
   - question form → Radio Group
   - dock → Resizable
   - notices → Sonner
   - data tables → Data Table (TanStack)

   Gaps get a custom component on the same tokens and are listed.
3. **Mockups:** a throwaway React app with Bun, fake data and real components, so the mockups can't drift from what the code can build. Screenshots of each screen, in both themes, go in `Gates/Gate-2-Design/high-fidelity/`.
4. **Typeset:** set up the Markdown presets (SPEC 5.8): `typeset-chat` for messages and a roomier one for page text blocks, with `not-typeset` on chat parts and `typeset-scroll` on wide tables.
5. **Checks:**
   - tones meet WCAG AA in both themes
   - right-to-left text shows correctly in the chat, inputs and tables (R-116), including inside Typeset (aligned to `start`)
   - code blocks from `rehype-highlight` look right inside Typeset
   - a streamed reply doesn't shift earlier blocks
   - the narrow-window layout

## Done when
Every agreed low-fidelity screen has a high-fidelity screenshot in both themes. The inventory and the mapping are written down, with the gaps listed. The low-fidelity open questions are answered.
