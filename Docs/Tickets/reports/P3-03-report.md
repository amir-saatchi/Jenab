# P3-03 report: Agent tools for views and pages

**Base:** `main` at 496591f. **Branch:** `claude/kind-swanson-2e0d93`.
**Model requests:** 183 to providers: 100 Gemini, 83 NVIDIA (see the live-run table). Only the synthetic prompts of `scenarios/views-and-pages` went to providers.
**Keys:** `GEMINI_API_KEY` (main key only) and `NVIDIA_API_KEY`, read by the runner from the main checkout's `.env`; no reserve keys; Haiku not run.

## What was built

### Tools (SPEC 8.1), `internal/tool/config.go`

Six tools in `ConfigTools()`, added to `Builtin` after the data tools. All have `Areas: Views` (block 1, as the data tools) and `When: WhenTables`; the two saves and `delete_config` write the database, the others read it.

| Tool | Arguments | Result |
|---|---|---|
| `save_view` | `config` (YAML), optional `revision` | Validates (P3-02's `view.Validator`), saves only without errors; what the view uses is checked again in the write. "Saved view X, revision N." / "No change: …" plus warnings; on errors "Not saved: N errors:" and every issue with its line. A chip notice `config_saved` ("View saved: X"). |
| `save_page` | `config` (YAML), optional `revision` | The same for pages ("Page saved: X"). |
| `validate` | `config` | The same checks, nothing saved: "The view X is valid. Nothing was saved." / "… has errors" / "… valid, with warnings". |
| `read_config` | `id` | "view X (type), revision N:" or "page X, revision N:" and the stored YAML. An unknown id: "No view or page "X": the project has none saved." or "… Saved: a, b." |
| `open_page` | `id`, `filters` | Checks the page exists, each filter is one of its params ("The page X has no filter a. Its filters: …") and each value fits (see Review fixes, 7); a view's id is told it is not a page. Adds an `open_page` notice (Target `page:X`, Details = the filters as JSON) and says "Asked the app to open the page X; the user opens it from the chip." |
| `delete_config` | `id` | Refuses a view another config uses, naming them: "Not deleted: the view X is used by page bitcoin and view y. Take it off them first if the user wants it gone, or leave it." The check runs in the delete's write. Otherwise deletes through the change log (op `delete`, expected revision = the current one), "Deleted view X. Undo turn brings it back.", and a chip `config_deleted` ("View deleted: X"). No approval card. |

Tool descriptions as the model sees them:

- **save_view:** Save a view: a table, chart, stat or form that pages show. Pass the whole view as YAML in config; its id names it, and saving an id that exists replaces that view. The view is validated first and saved only when it has no errors; the result lists every error with its line, so fix them all and save again. To change a view you read, pass the revision read_config showed. Load the config-guide skill before your first view or page.
- **save_page:** Save a page: rows of views, headings, text, filters and buttons the user opens from the sidebar. Pass the whole page as YAML in config; its id names it, and saving an id that exists replaces that page. Every view a page shows must be saved first. The page is validated first and saved only when it has no errors; the result lists every error with its line. To change a page you read, pass the revision read_config showed.
- **validate:** Check a view or a page without saving it: the same checks as save_view and save_page, with every error and warning and its line.
- **read_config:** Read a saved view or page by its id: its YAML as stored, with its revision. Read one before you change it, then save the whole changed config.
- **open_page:** Ask the app to open a saved page for the user, with filter values if you give them (filters: the page's filter params and their values). The user sees a chip with Open in the chat.
- **delete_config:** Delete a saved view or page by its id. Only when the user asked to remove it: taking a view off a page doesn't need a delete. A view still on a page, or opened by a page's button or a table's row action, is not deleted; the result names what uses it. Undo turn brings it back.

### Repair loop (SPEC 10)

- Failed saves are counted per turn, per tool and config id (`repairs`, keyed by chat, turn and subagent run, cleared at the turn's or run's end through `Env.AtTurnEnd`). The id is the parsed YAML's top-level `id`, in any style; the `id:` line only when the YAML doesn't parse. A successful save of that config starts its count again.
- The fifth failure in a row (the first plus 4 more attempts) adds: "That was the last try: 5 saves of this config failed this turn. Don't save it again; show the user the remaining errors and ask how to go on." A sixth save is refused without validating: "save_view X: 5 saves of this config failed this turn, which is the limit; don't try again. Show the user the remaining errors and ask how to go on." `RepairAttempts = 4`.
- **Early-stop notice:** the first failed save of a turn registers a check through `Env.BeforeAnswer`. When the turn is about to end with a config whose last save failed and wasn't retried (and isn't at the limit, which was already told), the agent sends, once a turn, e.g. "save_view btc_daily failed validation (3 errors) and was not saved. Fix and save it, or tell the user why not." as an `early_stop` notice. Same text for every model.

### Project card (3.2)

`store.CardWith(ctx, at, refs)` (with `view.Card` passing `view.CardRefs`) adds:

- per table: `- btc_prices (2 rows): …; views btc_price_chart` and `; views keyword_list; forms edit_keyword`;
- `Views:` lines `- id (type): "title"`, `Pages:` lines `- id: "title"; views a, b`;
- in the short card (over its budget): "Views (N; read_config shows one): ids." and "Pages (N): ids.".

`store.Card` keeps its signature (no config lines) for callers that have no view package. The desktop, the scenario runner and the app test now build the card with `view.Card`.

**Rebuilds on a config change:** `ConfigStamp` (the sum of the revisions in `_jenab_views` and `_jenab_pages`; rows are never removed, so it only grows) joins the schema version in the agent's card version (`cardVer`). A config change in a turn is told at the next turn's first request; a schema change still at once (P2-08).

### config-guide skill

`internal/skill/builtin/config-guide/SKILL.md` rewritten: `load_with: [save_view, save_page, delete_config]`, five examples (chart with filters, stat, update form, table with badge and row actions, page with a card) and a section "Changing and removing" (read, edit, save the whole config; taking a view off a page isn't a delete; delete only on the user's ask and when the card shows no other users; deleting a page and then its views no other page shows; removing views leaves tables and data alone, added after the live runs; Undo turn). `internal/view/guide_test.go` validates and saves every YAML example in order on a project with the tables they use.

### Chat chip

`configChip` in `internal/native/chat/chips.go` for `config_saved`, `config_deleted` and `open_page` notices: the text ("View saved: X", "Page saved: X", "Page deleted: X", "Page: X"), the pages-tab icon, neutral tone. No undo of its own: when the turn is undone it reads "… · undone", like the memory chip. A page's chip has *Open* when its kind is `config_saved` or `open_page` and the turn isn't undone; `config_deleted` has none. The text is never read. *Open* docks the page next to the chat (`state.Docked{Kind: "page", Key, Filters}`) and switches the right sidebar to Pages.

`project:configs` with a page deleted (by the agent, the user or an undo) now closes that page wherever it is docked in the project (`Shell.ClosePage`, from `Stores.Apply`).

### Scenarios

`scenarios/views-and-pages`: the Bitcoin project with prices, news and three saved views (`btc_price_chart`, `btc_last_price`, `btc_news_list`) on the page `bitcoin`, written straight into `_jenab_views` and `_jenab_pages` by `data.sql`; the project card is the real one.

- **V1_build_page:** a page "BTC moves" with a bar chart of the daily change and a stat of the biggest rise. Checks config-guide loaded before the first save, two new views live and on a new page, the chart is a bar chart, a new stat.
- **V2_change_view:** make the price chart a bar chart called "Daily close". Checks the view changed in place (no second view) and the page still shows it.
- **V3_fix_error:** save a table view as written (no `default_sort`, and an `ORDER BY` the validator warns about); the first save is refused, the fixed one saved.
- **V4_open_page:** open the page from September 25. Checks `open_page` with `bitcoin` and `2026-09-25`, its notice, and no save or delete.
- **V5_remove_view:** the news list on the page is not wanted any more. Checks the page saved without it, the view deleted, the other two views and the page kept, and the `btc_news` table still there with no schema change.

`_offline.yaml` plays a passing run through the real tools (5/5 pass). The runner (`internal/scenario/tools.go`) treats the config tools as a group: a set that fakes one of them, or a data tool, gets none of the real ones (`skill-loading`, `role-skills`, `grounding` and `multi-part` fake `save_view`).

## Decisions and deviations

1. **The chip's *Open* has no visible page yet.** It docks a `page` entry and opens the Pages tab; the shell draws pages from P3-10. Until then nothing new is drawn in the dock for a page.
2. **delete_config refuses more than the ticket lists:** a form a page's action or button opens (`open_form`), not only a row action's, and a page another page's action or button opens (`open_page`). A dangling target would fail the next validation of the config that has it.
3. **Subagents have the repair limit** (changed in review): each subagent run counts on its own, with the same early-stop notice before it answers.
4. **A config change reaches the card at the next turn's first request,** not mid-turn: the save's result already tells the model what it saved, and rebuilding block 4 mid-turn would cost the prompt cache for every save. A schema change still rebuilds at once.
5. **`Env.BeforeAnswer` takes the notice kind** (`func(kind chat.NoticeKind, check func() string)`), so the early-stop notice is an `early_stop` notice and the desktop's stays a reminder.
6. **All six tools have `When: WhenTables`**, as the data tools: a project without tables has no views to show.
7. **The skill's examples avoid `run_pipeline`** buttons and pipelines (Phase 4).
8. **The scenario fixture writes configs straight into the tables,** without change-log entries, so an undo in a run can't reach them. The runner needs no change for seeded configs.

## Checks

- `go build ./...`, `go vet ./...`, `go test -p 2 -short -count=1 ./...` with `GOPROXY=off GOMEMLIMIT=1500MiB`: all pass, also after the review fixes.
- gofmt: the changed and new Go files are clean (checked with CRs stripped).
- `go run ./cmd/jenab-scenarios check scenarios`: all sets ok, `views-and-pages` 5 scenarios.
- Offline: `jenab-scenarios run -model scripted:scenarios/views-and-pages/_offline.yaml scenarios/views-and-pages`: 5/5.

New tests: `internal/tool/config_test.go` (`TestConfigToolSpecs`, `TestSaveView`, `TestSaveViewErrors`, `TestSavePage`, `TestValidate`, `TestReadConfig`, `TestOpenPage`, `TestDeleteConfig` with refusals naming the users and a delete with its undo, `TestRepairLimit`, `TestRepairResetOnSave`, `TestEarlyStop`, `TestConfigCard`), `internal/agent/card_test.go` `TestCardAfterConfigSave`, `internal/view/guide_test.go` `TestConfigGuideExamples`, `internal/native/chat/configchip_test.go` (`TestConfigChips`, `TestOpenPageChip`: light and dark, keyboard, accessible name, a Persian page id, undo and redo), `internal/native/state/stores_test.go` `TestADeletedPageCloses`.

## Review fixes

Main-Session's review, all nine points. The live runs below are from before the fixes; the offline run and every test were run again after them.

1. **Reference checks are atomic.** `store.ConfigSave` and `store.ConfigDelete` take a `Check func(all []Config) error` that runs inside the writer transaction with every live view and page, before the write; its error refuses the write, wrapped in `ErrInvalid`, so the writer goes on. `delete_config` checks who uses the config there (`view.UsersOf`), so a page saved meanwhile with the view keeps it ("Not deleted: … is used by page bitcoin."). `save_view` and `save_page` check there that the views, forms and pages they use still exist (`view.MissingRefs`, the same wording as validation: "no view btc_price_chart; save the view first", "no form edit_keyword; …", "no page bitcoin"); a refused save counts as a failed save for the repair limit and the early-stop notice. Tests use a hook between the checks and the write (`testBeforeWrite`) for the other chat's write: `TestDeleteRacesSave`, `TestSaveRacesDelete` (view, row action's form, button's page), and `store.TestConfigCheck`.
2. **Card rebuild tests:** `TestCardConfigTiming` (a config change from another chat gives a card note at the chat's next turn's first request; a config change mid-turn gives none; a schema change mid-turn after a config save gives one at once, with the config in it) and `store.TestConfigStampAfterUndo` (the stamp rises after each undo and redo, including the undo that deletes the view). `TestCardAfterConfigSave` stays.
3. **Optional `revision` on `save_view` and `save_page`** (the number `read_config` shows), passed as the save's expected revision. A mismatch is refused: "Not saved: the view X changed after you read revision 2; it is at revision 3 now. Read it again with read_config and make your change on that." A config deleted since: "… was deleted after you read revision 4. Ask the user before saving it again." Left out, a save replaces whatever is there, as before. A refused revision isn't a failed save. One sentence in each description, the same for every model. Size: the two tools grew by about 370 characters (about 90 tokens); the six config tools are now 3,089 characters of name, description and schema. `validate` keeps a schema without `revision`. The repo has no tool-list size test (the tool-index test checks the hook only), so this was measured with a temporary test. Test: `TestSaveRevision`.
4. **`validate` stays unlimited.** Its bound is the request cap: `llm.turn_max_requests` (25 by default) for a turn, `subMaxRequests` (15) for a subagent run. The tool's code comment says so.
5. **The repair key comes from the parsed YAML:** the top-level `id` when the YAML parses, whatever its style (`"id": x`, `id : x`, flow style); `(no id)` when it parses without a string id; the `id:` line only when it doesn't parse. Test: `TestRepairKeyParsed`, including a run of failures that changes the id's style and still reaches the limit.
6. **Subagents get the same repair limit,** counted per subagent run. `tool.Env.Run` names the run (`helper:<id>`), and the subagent's Env now has `Turn`, `AtTurnEnd` (run when the run ends) and `BeforeAnswer` (checked when the model answers without calls; the first reminder is added as a notice and the run gets one more request, as a turn's). Tests: `TestSubagentTurnHooks` (agent) and `TestRepairPerRun` (tool).
7. **`open_page` checks its values:** one scalar each (string, number, true/false, null); a `date` filter takes `YYYY-MM-DD` or a relative date (`today-30d`, `start_of_month`…), a `number` filter a JSON number, a `checkbox` true or false, a `select` a string or a number; `text` takes any scalar. The values are at most 4 KiB as JSON. Each refusal names the filter and what it takes, e.g. "The filter min_price of bitcoin is a number: give it as a JSON number, not "90000"." Test: `TestOpenPageValues`.
8. **The chip's *Open* comes from the notice kind.** A delete now has its own kind, `config_deleted`; *Open* is on `config_saved` and `open_page` notices of a page, never decided by the text. Tests: `TestConfigChipKinds` (tool) and the chip tests, whose delete notice now has the new kind.
9. **The card note is sent again in full after a saving turn** (accepted as designed). Cost: the card is capped at `CardTokens` = 2,000 tokens, so at most about 2,000 tokens, once, at the next turn's first request after a turn that changed a config.

Also: `TestDeleteFormOfPageButton`, a form opened only by a page's button can't be deleted.

## Live runs

Same prompts and settings for both models; `-reps 3`, `-usage-log`. No quota or rate errors. Haiku was not run.

| Run | Model | Passed | Requests to providers | Wall time | Notes |
|---|---|---|---|---|---|
| All 5 scenarios | gemini-3.5-flash-lite | 15/15 | 88 | 476 s (32 s a run) | 43 % of prompt tokens from cache |
| All 5 scenarios | mistral-large-4-1t-a52b (NVIDIA) | 15/15 | 68 | 1469 s (98 s a run) | 77 % from cache; NVIDIA's endpoint is slow (up to 4 min for 6 requests) |
| V5 again, after the fix below | gemini-3.5-flash-lite | 3/3 | 12 | 34 s | |
| V5 again, after the fix below | mistral-large-4-1t-a52b (NVIDIA) | 3/3 | 15 | 138 s | |

What the transcripts show:

- **V1:** both models loaded config-guide first. Gemini tried its queries with `query` first (9–13 requests); Mistral read the saved views with `read_config` and then saved (5–6 requests). Both opened the new page with `open_page`. No save failed.
- **V3:** in all 6 runs the first save was refused (no `default_sort`) and the second was saved. No model needed more than one repair, so the live runs never reached the repair limit or the early-stop notice. The tool tests cover those.
- **V5, first version: a failure of the scenario, found and fixed.** The message said "I don't need the news table any more. Take it off the Bitcoin page and get rid of it." In all 6 runs on both models, the chat took the view off the page, deleted it, and then asked `request_schema_change` to drop the `btc_news` table and its rows. The runner approves schema cards, so the table was dropped (the checks passed, since they didn't look at the data). In the app this goes through an approval card that says it deletes data, but the user asked to remove a view, not the data. Two changes, the same for every model:
  - the message now says "the news list on the Bitcoin page … get rid of that view", and the scenario checks that `btc_news` is still there and that `request_schema_change` isn't called;
  - config-guide's removing section has a new line: "Deleting views and pages leaves the tables and their data as they are. Drop a table only when the user asks for the data itself to go."

  After the change, all 6 V5 runs took the view off the page and deleted it, and nothing else. One Gemini run in the first version called `delete_config` first, was refused ("used by page bitcoin"), and then saved the page without the view. Only V5 was run again. The new skill line is in the removing section, which the other four scenarios don't use.

## SPEC lines Main-Session should change

- **8.1:** list `delete_config(id)` with the tools, and that `open_page` records the request as a chip with *Open* (the dock draws it from P3-10). `save_view` and `save_page` take an optional `revision`, the one `read_config` shows, and refuse a config changed or deleted since. What a save uses and what uses a deleted config are checked in the write. `open_page` takes one scalar value a filter, fitting its control, at most 4 KiB in all.
- **8.6:** a subagent run is a turn of its own for per-turn tool limits: the repair limit counts per run, and its early-stop notice comes before the run answers.
- **10:** the limit counts per tool and config id within a turn, or within a subagent run; the id is the parsed YAML's `id`; a successful save starts it again; the fifth failure says it was the last try and a sixth save is refused unchecked. `validate` has no limit of its own; the request cap bounds it.
- **3.2:** the card's config lines as built (`; views …; forms …` per table, `Views:` and `Pages:` sections, the short form); a config change reaches the card at the next turn's first request.
- **8.9:** config-guide's `load_with` is `[save_view, save_page, delete_config]`.
- **5.9:** a deleted page closes in every chat where it is docked (`project:configs`), as built here; P3-10 draws the dock.
- **Scenarios (or wherever SPEC lists them):** `views-and-pages`. Also worth a line: removing a view never touches its tables; the runner approves schema cards, so a set that can change the schema should check the data as well.

## Proposed Result paragraph

Built: `save_view`, `save_page`, `validate`, `read_config`, `open_page` and `delete_config` in block 1 with the data tools. Saves validate first and return every error with its line, take the revision the model read, and check their references again in the write. The repair loop allows 4 more tries per config in a turn or subagent run, then tells the model to stop, and the early-stop notice comes when a turn ends after a failed save that wasn't retried. `delete_config` refuses, in its write, a view or page other configs use, naming them; a delete is undone with its turn. `open_page` checks its filter values. The project card lists views, pages and, per table, the views and forms that use it, and is rebuilt at the next turn after a config change. config-guide has five tested examples and a removing section. Saved and deleted views and pages show as chips; a page's chip has *Open*, which docks the page (drawn from P3-10), and a deleted page closes everywhere. `scenarios/views-and-pages` passes 15/15 on gemini-3.5-flash-lite and on mistral-large-4. The first version of its remove scenario led both models to drop the news table too; the message now names the view, and config-guide says removing views leaves tables and data alone.
