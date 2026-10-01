# Gate-2 — High fidelity
**Status:** Done ([TASK-002](../../../Tickets/TASK-002-high-fidelity-design.md)). All 12 wireframe screens and a right-to-left check are built, in both themes. Reviewed on 2026-09-29; the decisions are below and in the SPEC.

## Setup
- **App:** `mockups/` at the repo root: Vite, React, TypeScript and Bun, with fake data (the Bitcoin example, SPEC 9). It is thrown away after Gate 2.
- **shadcn:** style `radix-nova`, base **Radix** (decided 2026-09-29), Tailwind v4, lucide icons, Geist and Geist Mono. All 61 components are installed.
- **Colours:** shadcn's neutral theme for now (agreed). They may be tuned later with tweakcn, which only rewrites the CSS variables; the tone check below must pass again after that.
- **Markdown:** `react-markdown` + `rehype-highlight` inside Typeset (SPEC 5.8). Presets: `typeset-chat` (14 px, leading 1.65) and `typeset-page` (15 px, leading 1.7). `remark-gfm` adds tables, strikethrough and task lists.
- **Screens:** one URL each, e.g. `?screen=01&theme=dark`. Screenshots are 1440 × 900 and taken with headless Edge; the two settings pages are taller (07: 1200, 08: 1400), since they scroll.

## Inventory
Every component in the `@shadcn` registry (read with `shadcn search`, 2026-09-29).

| Component | Jenab uses it for |
|---|---|
| Alert | Cloud-sync warning (08), provider errors, page error state (5.7) |
| Alert Dialog | Confirmations: delete chat, *Revert run*, *Free now* |
| Attachment | Files in the composer and in messages; the Files tab |
| Badge | Statuses, counts, tone labels |
| Bubble | User messages; agent text uses the `ghost` variant |
| Button, Button Group | Everywhere; the memory chip (text, *Undo*, *View changes*) |
| Calendar | Date filters on pages (5.9) |
| Card | Approval card, provider card (09), page `stat` and `card` blocks, Settings sections |
| Chart | Page chart blocks (it wraps Recharts) |
| Checkbox | Table row selection for bulk row actions (02) |
| Collapsible | Tool chips, step details |
| Combobox | Filters with many values; the model list in Settings |
| Command | Search across chats, pages and pipelines |
| Dialog | *Create project*, *Add key*, *View YAML* |
| Dropdown Menu | Project switcher, model picker, approval level, "…" menus |
| Empty | Empty states: no pages, no pipelines, a new project |
| Field | Every form: Settings, *Add key*, project settings |
| Input, Input Group, Textarea | The composer (Input Group + Textarea), keys, search |
| Item | Lists: providers, hosts, files, links |
| Kbd | Shortcuts in menus and tooltips |
| Label | Through Field |
| Marker | Cuts, "from Mother", finish notices, date dividers |
| Message, Message Scroller | The chat thread: streaming follow, jump to latest, `content-visibility: auto` on every item |
| Pagination | Large table blocks |
| Popover | Filter pickers, model details |
| Progress | *Refresh memory*, pipeline runs, updates |
| Questionnaire | `ask_user` question form (04) |
| Radio Group | Single choices in Settings |
| Resizable | The dock (02) |
| Scroll Area | YAML view, long lists |
| Select | Default and fast model, time zone |
| Separator | Everywhere |
| Sheet | Narrow windows: sidebars and the chat drawer (11); turn inspector (10) |
| Sidebar | Left sidebar (folds to icons) and right sidebar |
| Skeleton | Loading pages and views |
| Sonner | Notices |
| Spinner | Working status, running tool chips |
| Switch | On/off settings: tray, start at login, web search, catalog models |
| Table | Steps, runs, table blocks (with TanStack Table) |
| Tabs | Right sidebar tabs, Settings sections |
| Toggle, Toggle Group | *Flow / Steps* (06), theme Light / Dark / System (07) |
| Tooltip | Icon buttons, the folded sidebar |

**Not needed:** Accordion, Aspect Ratio, Avatar, Breadcrumb, Carousel, Context Menu (maybe later), Direction (the UI stays left to right; text uses `dir="auto"`), Drawer (Sheet covers it), Hover Card, Input OTP, Menubar (the app menu is native), Native Select, Navigation Menu, Slider.

**Blocks:** `sidebar-07` (folds to icons) is the model for the left sidebar, and `sidebar-15` for two sidebars. `dashboard-01` is a reference for pages. The login and signup blocks aren't needed.

## Mapping
| Wireframe element | Component |
|---|---|
| Project switcher (01) | Sidebar Menu Button + Dropdown Menu |
| Chat list with status (01) | Sidebar Menu; Spinner (working), amber alert icon (waiting), pin (Mother) |
| Chat header (01) | Buttons, ghost: *Refresh memory*, *Session notes*, "…" |
| Messages (01) | Message Scroller, Message, Bubble; agent text through Typeset |
| Tool chip (01) | Collapsible + outline Button (xs); details in a muted block |
| Memory chip (01) | Button Group |
| Turn footer (01) | Ghost Buttons: *Undo turn*, *Copy*, time |
| Composer (01) | Input Group + Textarea; Dropdown Menus for model (by name, e.g. "Gemma 4 31B", SPEC 3.9) and approvals; *Stop* while running |
| Mother (01) | Proposed: a gradient accent border on Mother's chat row and composer. It turns slowly only while Mother is working, and stays still with reduced motion. |
| Right sidebar (01) | Sidebar (right) + Tabs |
| Folded left sidebar (02, 06, 06-A) | Its own design, not shadcn's icon mode (that drew icons off-centre and clipped Mother). A 56 px rail: project mark, *New chat*, *Search*, then one 36 px tile per chat, with the schedules and Settings at the bottom. Mother is a tile filled with the gradient, which turns while Mother works (06-A). Other chats show their initials. A bar on the start edge marks the open chat, a spinner means working, and an amber dot means waiting. Every tile has a tooltip. |
| Page and dock (02) | Resizable (66 % / 34 %, dock at least 320 px); page rows are a 12-column grid of Cards |
| Page header (02) | Title, description, *Close page*; filter (outline Button + Calendar) and *Refresh now* |
| Table block (02) | Table + Pagination; Checkbox selection with a bulk bar (*Mark read*) |
| Background work (03) | Running tool chip with *Stop run*; the finish notice is a Marker separator |
| Subagent chip (03) | Tool chip with the agent's name, open by default while it works |
| Message during a turn (03) | Outline Bubble, dashed, with "Joins at the next step" |
| Approval card (04) | Card: kicker, title, why, risk, Collapsible details, *Allow* / *Deny*, note field |
| Question form (04) | Questionnaire in a Card; the recommended option is marked; Other is an Input |
| Waiting bar (04) | Custom: a thin bar above the composer that jumps to the card |
| Delegation (05) | Tool chips `send_to_chat` and `create_chat` with task ids; finish notice; *Open Reviewer* |
| Target chat (05) | Message with a "from Mother" header; the role under the chat title, with *Edit role* |
| Pipelines tab (06) | Sidebar Menu: last run per pipeline; red flag after 3 failures |
| Pipeline header (06, 06-A) | Toggle Group *Flow / Steps*; *Run now*, *Pause schedule*, *Dry run*, *View YAML* |
| Runs and steps (06) | Tables with status Badges in tones; the selected run has *Revert run* |
| Step output (06, 06-A) | Card with a code preview (secrets redacted) and the full output as a ref |
| Failure notice (06) | Alert (destructive) in Mother's dock, with *Open run log* |
| Flow (06-A) | Custom, plain React and CSS: step boxes, connectors, `for_each` as a dashed frame, a "Touches" column, LLM Badge (purple) |
| Settings (07, 08, 10) | Sidebar (not collapsible) as the page list, *Back to chats*; one scrolling page |
| Providers (07) | Item list: key state, *Models*, *Test*, *Remove*, or *Add key* |
| Default and fast model (07) | Select, with catalog names |
| Limits (07) | Field + Input per value from SPEC 7.6 |
| Other settings (07) | Radio Group (theme, updates); Select + Input Group (search key) |
| Project settings (08) | Label and value rows; Alert (cloud sync); Progress (cache); Tables (lifecycle rules, hosts) |
| First run (09) | Empty (no projects); a Card as the *New project* dialog; starter prompts as outline Buttons |
| Provider card (09) | Card in the chat: Toggle Group of providers, API key Input Group, *Connect* |
| Turn inspector (10) | Custom timeline (lanes Model, Tools, Background, Parts); Tables for requests, tool calls and context blocks |
| Narrow window (11) | Page rows stack below 900 px; the chat is a Sheet from the right; ☰ opens the left sidebar |

**Gaps**, built as custom components on the same tokens:
- the pipeline flow diagram (06-A)
- the turn inspector timeline (10)
- the waiting bar above the composer (04)
- the `stat` block (a Card composition)
- the model picker drawn open in 09 (a static copy of the Dropdown Menu, so it fits in the frame)

## Checks so far
- **Tones (SPEC 5.11):** six tones, each with a text colour and a soft fill for light and dark. Every text colour meets WCAG AA on the background and on its fill, in both themes: the lowest is 5.38:1 (red on its fill, light). All values are inside sRGB.
- **Muted text:** shadcn's default grey text is 4.34:1 on a grey fill in light mode, below AA. The mockups darken it (`--muted-foreground` 0.556 → 0.53), which gives 4.84:1.
- **Focus rings:** Message Scroller items use `content-visibility: auto`, which clips painting at the item's edge, so a focused button on the edge lost part of its ring. Each item gets 4 px of padding (`-m-1 p-1`). The project switcher's ring was hidden under *New chat*; the header menu now has a gap.
- **Right to left (12):** checked with Persian in the chat list, chat header, messages, Markdown lists, inline code, a code block, table cells, badges, an input and the composer.
  - Typeset uses logical properties only. Bubble has one left-aligned rule, for buttons inside a bubble.
  - **Font:** Geist has no Arabic-script glyphs, and the browser's default fallback drew Persian small and thin. The font stack is now `Geist, Vazirmatn, sans-serif`: Latin stays Geist, and Persian uses Vazirmatn on every OS.
  - **Lists:** a Persian item in an English list lost its bullet, because the marker sat outside the list's padding. `dir="auto"` can't fix that, and CSS `:dir()` fails too: Lightning CSS rewrites it into a `:lang()` list. `markdown.tsx` now sets an explicit `dir` on every list, item and table (from the first strong character), and `typeset.css` gives a mixed item room for its marker. A Persian Markdown table is on screen 12.
  - **Known bidi effect:** in an English sentence, `$64,210` next to Persian can show as `64,210$`. That is standard Unicode bidi; the app leaves it.
- **Narrow window (11):** rows stack below 900 px (`max-[900px]:col-span-12` on blocks). The screen shows two real 640 px windows in iframes, so the breakpoints are the real ones.

## Screens
| # | Light | Dark |
|---|---|---|
| 01 | [01-app-shell-light.png](01-app-shell-light.png) | [01-app-shell-dark.png](01-app-shell-dark.png) |
| 02 | [02-page-docked-light.png](02-page-docked-light.png) | [02-page-docked-dark.png](02-page-docked-dark.png) |
| 03 | [03-turn-anatomy-light.png](03-turn-anatomy-light.png) | [03-turn-anatomy-dark.png](03-turn-anatomy-dark.png) |
| 04 | [04-approvals-and-questions-light.png](04-approvals-and-questions-light.png) | [04-approvals-and-questions-dark.png](04-approvals-and-questions-dark.png) |
| 05 | [05-mother-delegates-light.png](05-mother-delegates-light.png) | [05-mother-delegates-dark.png](05-mother-delegates-dark.png) |
| 06 | [06-pipelines-light.png](06-pipelines-light.png) | [06-pipelines-dark.png](06-pipelines-dark.png) |
| 06-A | [06a-pipeline-flow-light.png](06a-pipeline-flow-light.png) | [06a-pipeline-flow-dark.png](06a-pipeline-flow-dark.png) |
| 07 | [07-settings-light.png](07-settings-light.png) | [07-settings-dark.png](07-settings-dark.png) |
| 08 | [08-project-settings-light.png](08-project-settings-light.png) | [08-project-settings-dark.png](08-project-settings-dark.png) |
| 09 | [09-first-run-light.png](09-first-run-light.png) | [09-first-run-dark.png](09-first-run-dark.png) |
| 10 | [10-turn-inspector-light.png](10-turn-inspector-light.png) | [10-turn-inspector-dark.png](10-turn-inspector-dark.png) |
| 11 | [11-narrow-window-light.png](11-narrow-window-light.png) | [11-narrow-window-dark.png](11-narrow-window-dark.png) |
| 12 (RTL) | [12-rtl-check-light.png](12-rtl-check-light.png) | [12-rtl-check-dark.png](12-rtl-check-dark.png) |

## Decisions
Agreed on 2026-09-29 and written into the SPEC.
- **Mother's colour:** a gradient of the purple, blue and green tones. It's a border on Mother's row and composer, and a filled tile in the folded rail. It turns while Mother works and stays still with reduced motion (8.6, 5.12).
- **Folded sidebar:** its own rail, as above (5.12).
- **Markdown:** `remark-gfm` for tables, strikethrough and task lists; lists and tables get an explicit direction (5.8).
- **Persian font:** Vazirmatn bundled after Geist (5.8). The Segoe UI fallback is gone.
- **Settings:** a full view, like the project settings (5.12).
- **Usage:** *Settings → Usage*, tokens per day, chat and model, and the cost from catalog prices (3.9).
- **First run:** Ollama is listed first when it's running on the machine; *Open the example* sits next to *Create project* (3.9).
- **Other models:** models the provider lists but the catalog doesn't know appear under *Other models*, off by default (3.9).
- **Inspect:** with the developer tools on, each turn's footer has *Inspect* (8.4).
- **Dock:** resizable, about a third of the main area, at least 320 px, and the width is saved (5.12).
