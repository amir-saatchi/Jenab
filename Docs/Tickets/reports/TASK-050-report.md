# TASK-050 report: find Project settings and the project's memory

Branch: `claude/interesting-gates-086691`. Frontend only; no Go and no `Docs/` changes. What the model is sent is unchanged.

## What changed

1. **Project settings from Settings.** Settings' page list has a second group under the main pages: the open project's name as its label, with one entry, *Project settings*, that opens that project's settings. With no project open the group isn't there.
   - "The open project" is the project of the chat used last (`mru[0]`), where *Back to chats* returns. A project that no longer exists counts as none. On the first-run screen there is none. (`useOpenProject` and `openProjectOf` in `frontend/src/shell/open.ts`.)
   - `SettingsLayout` takes `more`: more sidebar groups under the page list.
   - The project name's menu keeps *Project settings*. It and the new entry both call `openProjectSettings(project, section?)`, which also closes the narrow-window overlay as the menu did.
   - The title of *Project settings* is now the project's name (`<span dir="auto">`). The line under it reads "Project settings · created …". With the project not loaded yet it says "Project settings" as before. (`ProjectSettingsTitle` in `project-settings.tsx`; `SettingsTitle` takes a node.)
2. **Settings → Memory says whose memory it is.** A line under the section's description (`MemoryScope` in `frontend/src/settings/memory.tsx`):
   - With a project open: "This memory is for every project. ‹name› has its own memory too, in **Project settings › Memory**." The link is a shadcn `Button variant="link"` that opens that project's settings at its *Memory* section. The name is `dir="auto"`, so a Persian name keeps its own direction inside the English sentence.
   - With no project open: "This memory is for every project. Each project has its own memory too, in its Project settings." No link.
3. **The schema tool's row.** In the collapsed row, a result whose first line is `[<the tool's own name>: <state> — ref: …]` (or without the ref) shows only `<state>`, so `request_schema_change` reads "applied", "rejected", "denied" or "failed". Other tools' first lines are shown as before (`[query result — … — ref: …]` doesn't match, since it isn't headed by the tool's name). The expanded row is unchanged: the whole result, ref included, and the stored-output link. `internal/agent/schema.go` is unchanged. (`resultLine` in `frontend/src/chat/parts.tsx`.)

## Checks

- `bun test` (frontend): **144 pass, 0 fail**. New tests:
  - `src/shell/settings-view.test.ts`:
    - the open project rule (`openProjectOf`): a project, none, a project that is gone
    - the page-list group renders the name (`dir="auto"`) and *Project settings* with a project, and nothing with none
    - the entry opens `project-settings` and closes the overlay
    - the title is the project's name, with "Project settings" without one
  - `src/settings/memory.test.ts`: the line with a project (name, link, the link opens *Memory*), and without one (no link).
  - `src/chat/parts.test.ts`:
    - `resultLine` for applied with a ref, denied without one, and other tools' lines unchanged
    - a rendered `request_schema_change` row shows `applied` and no `ref:`
  - A static render can't size the window, so the sidebar test stubs `@/hooks/use-mobile` with `mock.module`, as `bubbles.test.ts` does for the API.
- `bun run build` (`tsc -b` + Vite): OK, only the existing chunk-size warning. `bun run typecheck`: OK.
- Go: not touched. A server-mode build (`go build -tags server ./cmd/desktop`) compiled for the hand check. `go.mod`/`go.sum` unchanged.
- **By hand**, in a server-mode build:
  - Setup: `LOCALAPPDATA`, `APPDATA` and `USERPROFILE` pointed at the scratchpad. A fake Ollama (a small Python script in the scratchpad, no keys) on `127.0.0.1:11434`, connected through the *Ollama on this computer* preset. The Browser pane at 1280×800, light and dark.
  - Settings with no project: no project group, and the Memory line has no link.
  - I created a project with a Persian name (`قیمت‌ها`). Settings then shows the group under that name. The entry and the Memory link both open its settings: the link at *Memory*, which is active in the list. The `h1` reads `قیمت‌ها`.
  - In a chat, the fake model called `request_schema_change`. The schema agent, the same fake, applied a real `create_table` migration through `apply_migration`. The collapsed row reads `request_schema_change applied`, and the expanded row has the full result with `ref: subagents/…` and the stored-output link.
  - An earlier run, where the fake reported too few input tokens, ended `failed`. Its row read `request_schema_change failed`, with the full error and ref in the expanded row. The failure came from my fake model, not from the app.
  - No console errors. I stopped the server and the fake model afterwards.

## Notes for Main-Session

- The open project behind Settings is the one of the chat used last. That's my reading of "the open project" while Settings is showing. It is the same project *Back to chats* goes to.
- The collapsed rule is keyed to the tool's own name, not to `request_schema_change` alone. No other tool uses that header today, so only the schema tool changes. If the subagent tools adopt the same header later, they get the same short row.
- Seen in passing, not changed: a "Skill loaded: migrations" chip shows in the parent chat after a schema change. It is the schema agent's skill, not the chat's. This may be intended.
- The screenshots show the scratchpad path in the sidebar and in Settings → General, and that path has the Windows user name in it. They are not committed.

## Screenshots (kept outside the repo, not committed)

Folder: `%LOCALAPPDATA%\Temp\claude\42bf34a8ff589872\fa0fe044-9325-41f0-a6d5-4cbc5a312e42\scratchpad\screenshots\`

- `1-settings-no-project-dark.jpg`, `2-settings-no-project-light.jpg`: the page list without a project group; the Memory line without a link
- `3-settings-with-project-dark.jpg`, `4-settings-with-project-light.jpg`: the `قیمت‌ها` group with *Project settings*; the Memory line with its link
- `5-schema-row-light.jpg`, `7-schema-row-dark.jpg`: the collapsed `request_schema_change applied` row
- `6-schema-row-expanded-light.jpg`, `8-schema-row-expanded-dark.jpg`: the expanded row with the full result and ref
- `9-project-settings-title-dark.jpg`, `10-project-settings-title-light.jpg`: *Project settings* titled with the project's name, opened at *Memory* from the link
- `extra-schema-row-failed-expanded-light.jpg`: a `failed` row from the earlier fake run

## Proposed Result paragraph

> **Result:** Done (TASK-050). Settings' page list has the open project, which is the project of the chat used last, under its name, with *Project settings*. With no project open the group isn't shown. The project name's menu keeps *Project settings*, and the project's name is now the title of *Project settings*. Settings → Memory has a line saying the memory is for every project and that the open project has its own memory, with a link to its *Project settings › Memory*. With no project open the line has no link. The collapsed `request_schema_change` row shows its state ("applied", "rejected", "denied", "failed") instead of the `ref: subagents/…` header. The expanded row and what the model gets are unchanged. Each fix has a frontend test, `bun test` and `bun run build` pass, and a server-mode hand check with a fake local model showed all three in light and dark.
