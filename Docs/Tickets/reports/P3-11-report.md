# P3-11 report: the Links tab

Branch `claude/bold-bose-fa21cb`.

## What I built

The right sidebar's Links tab now shows a project's saved links. Each row has its title, the URL's host, its note, its tags, and who saved it and when. The newest link comes first. A search box matches titles, hosts and tags. The list updates as soon as any link is written.

Each layer:

- **Store** (`internal/store/links.go`)
  - The table:

    ```sql
    _jenab_links(id PK, url UNIQUE NOT NULL, title, note, tags JSON array, created_by, created_at) WITHOUT ROWID
    ```

    It has an index on `created_at`. It is added as **project.db format step 7** (`linkTables`), after the views and pages step.
  - `SaveLink`: with no ID it adds a link; with an ID it edits one. `LinkSave.Merge` turns a duplicate URL into an update; that is for the agent's future `save_link`.
  - `DeleteLink`, `Links` and `LinkByID`.
  - Each write goes through the writer queue (`doThen`) and the change log (`rowEntry`, before and after revisions).
  - A delete is a real `DELETE`. Its change-log entry keeps the row, so Undo puts it back.
  - Undo has a reverter for `_jenab_links`. If the link was edited since, that is a conflict. If another link now has the same URL, that is a constraint conflict.
  - `UndoResult.Links` makes Undo, Undo turn and Redo fire the links hook too.
  - Limits: URL 2048 characters, title 300, note 2000, 20 tags of up to 50 characters each.
  - `CleanLink` trims the fields and drops repeated tags, ignoring case. It requires a URL with a scheme and a host, an opaque part or a path.
  - A refusal is a `*store.LinkError` (it matches `ErrInvalid`) whose message the user can read.
- **Events**:
  - `project.LinksChanged` and `Publisher.Links`.
  - The app event is `project:links` (`app.EventLinks`).
  - It fires on every committed link write, undo included.
- **Service**: `app.LinkService` (`Bound.Links`), wired in `cmd/desktop/main.go`.
  - Methods: `List`, `Add`, `Edit`, `Delete` and `UndoDelete`. Writes are made as `user`.
  - Errors map to `UIError`: invalid, "This URL is saved already.", not found, and two busy messages for Undo.
- **State**: `state.Links` in `internal/native/state/savedlinks.go`, named so because `links.go` is already the chat's `jenab://` links.
  - It keeps one list per project. A list is re-read lazily after `project:links`, and the old list stays on screen until the new one is in.
  - It holds the search text per project.
  - The add and edit dialog's state (`LinkEditor`): a refusal stays in the dialog, a successful save closes it.
  - `Stores.DeleteLink` shows a "Deleted …" toast with **Undo**, then "… is back", the same way file deletes work.
  - Helpers:
    - `LinkHost`: the host without `www.`, a mailto address, or else `scheme:`.
    - `LinkTitle`.
    - `SplitTags`: splits at `,` and the Persian comma `،`.
    - `JoinTags`.
    - `FilterLinks`.
- **Kit**:
  - `kit.SavedLink(c, SavedLinkFace)` draws a row: title, host, note (two lines at most), tag badges and the "Saved by" line.
  - A link SafeLink refuses gets an amber **Doesn't open** badge, with a tooltip explaining why.
  - `kit.OpenLink(c, url)` is the one way to open a link that comes from data. It goes through `SafeLink` (http, https and mailto) before `c.OpenURL`.
  - `HasRole` now knows `treeitem`.
- **Screen**: `internal/native/links`, put into the shell's right sidebar in place of the empty Links tab.
  - The head has the search box and an **Add link** button.
  - The rows are a `kit.Tree` named "Links": arrow keys, Home/End and Enter work, and right-click opens a menu.
  - A click or Enter opens the link. A link that isn't safe opens nothing and shows a toast saying why.
  - The menu has **Open** (only for safe links), **Copy link**, **Edit**, and **Delete** with Undo.
  - The dialog has fields for URL (always left to right), Title, Note and Tags.
  - Empty states: "No links yet" and "No links match".
- **Test fake**: `statetest.FakeLinks` and `Fake.AddLink`, included in `Fake.API()`.

## Decisions

- **No `save_link` tool for the agent.**
  - It is in SPEC 8.1 but nothing implements it yet, and the ticket doesn't own it.
  - Adding it would change every model's tool list (the built-in specs test, scenarios, skills).
  - The store side is ready: `SaveLink` with `Merge` and a `message:` source. `project:links` fires on any committed write, so the tab will update live once the tool exists.
- **Any URL scheme is stored; only http, https and mailto open.** Other links show a "Doesn't open" badge and can still be copied. This keeps what the agent or the user saved, and opening stays safe.
- **The card's "Saved links: N." line now shows only when N > 0.** Without this, every project's card read "Saved links: 0." once the table existed.
- **Search** covers title, host and tags, as the ticket says, not the note.
  - It folds text with `store.FoldText`, the chat search's fold (SPEC 2.3): case, NFKC presentation forms, Arabic ي and ك as ی and ک, Persian and Arabic digits as Latin, and harakat, tatweel and ZWNJ dropped. Direction marks are dropped too.
  - A space in the query also matches a ZWNJ, so «می خواهم» and «میخواهم» both find «می‌خواهم».
- **Direction**:
  - Title and note each take the direction of their first strong letter, so a Persian title starts on the right.
  - Host and URL always run left to right.
  - The dialog's text fields already set their own direction (`kit.TextInput`), except the URL field, which is forced left to right.
- **Delete is a hard delete** with Undo from the change log, as the ticket asks. It is not a trash.

## Review fixes (second commit)

Main-Session's review asked for ten things. All are done, with tests:

1. **The dialog's lifetime.** `links.Screen` notes which project's tab it drew each frame; `EndFrame`, called after each `Shell.View`, detaches the dialog when its tab wasn't drawn (another tab, the sidebar hidden, another project). The dialog draws only in its own project's tab. A save that runs when the dialog detaches goes on: a refusal then shows as an error toast, a success reads the list again. `StartAdd` and `StartEdit` refuse while a save runs, so its answer isn't lost. The dialog is modal, so these changes come from shortcuts, a link that shows a file, or a project opened elsewhere.
2. **Undo text.** `ItemName` names a links item "a saved link". `UndoConflict` has a new `Name`, which the store fills for links conflicts with the title or host (from the link now, or from its last change when it's gone). `ConflictText` then says "the link «title»", isolated.
3. **Commas in tags.** The store refuses a tag with "," or "،": "A tag has no comma: commas separate tags."
4. **Merge keeps fields.** With `Merge`, an empty title, note or tag list keeps the old value.
5. **Try again, and freeing lists.** "Links can't be shown" has a Try again button; the read again shows the spinner. `Links.Keep(p)` drops other projects' lists, searches and counts when a project opens (`OpenProject`).
6. **Search folding** as above. `CleanLink` de-duplicates tags with the same fold.
7. **Bidi controls.** A URL with a space, a control character or a format character (U+202A–202E, U+2066–2069, marks) is refused. The title, note and tags lose the overrides and isolates (ZWNJ and the marks stay: Persian needs them). `kit.SavedLink` draws the title, host, note and tags inside FSI…PDI.
8. **Normalized URLs.** `NormalLinkURL` lowercases the scheme and host (userinfo kept), and gives an http(s) URL with no path "/". So `HTTPS://Example.COM` and `https://example.com/` are one link. A URL that doesn't parse says "This URL can't be read. Check its host and its characters."
9. **UndoDelete messages.** A constraint conflict says another link has the URL now; any other says "The link changed after it was deleted, so the delete can't be undone."
10. **New tests:**
    - The step-7 backup file exists with user_version 6, and an older format refuses the new file (`TestLinkFormatUpdate`).
    - `project:links` reaches the Publisher through `project.Manager` for a save and its undo (`TestLinkWritesArePublished`).
    - The dialog after a tab switch, the sidebar hidden and a project switch (`TestTheLinkDialogClosesWithItsTab`, `TestPanelDialogFollowsItsTab`, `TestLinksEditorDetach`).
    - The comma tag (`TestCleanLinkTagsAndDirection`).
    - A redo onto a taken URL (`TestLinkRedoOntoATakenURL`).
    - `internal/native/links/panel_test.go`: the list, keyboard, the dialog, Try again.
    - Also `TestNormalLinkURL`, `TestLinkMergeKeepsFields`, `TestLinksRetry`, `TestLinksKeep`, `TestUndoDeleteConflictText`, and more `TestFilterLinks` and `TestConflictText` cases.

## Deviations

- I added no new dependencies. `golang.org/x/text` was already a direct dependency.
- `kit.SavedLink` and `kit.OpenLink` are new kit pieces, so the screen package doesn't import MyGo's `ui`.

## Tests

- **Store** (`internal/store/links_test.go`):
  - Save, list and delete; a URL saved only once and Merge.
  - `CleanLink` cases.
  - Undo of add, edit and delete, including the conflict and constraint cases.
  - The format update from step 6 to 7, with the card line.
- **Service** (`internal/app/links_test.go`):
  - List, add, edit and delete; refusals with their messages; Undo of a delete and a second Undo.
  - Five `project:links` events.
- **State** (`internal/native/state/savedlinks_test.go`):
  - `LinkHost`, `SplitTags` and `FilterLinks`, including the Persian folding.
  - Store tests: one read and then a re-read after `project:links`; a refusal in the dialog; add and edit; delete with its Undo toast; a second Undo refused; no service.
- **Kit** (`internal/native/kit/savedlink_test.go`), light and dark:
  - Every text shows; the tree and each row are named.
  - Keyboard: Tab, Enter, End, Enter. The safe link opens and `javascript:` doesn't.
  - Tab leaves the list.
  - Pixel check: the Persian title and note have their ink on the right, the English ones on the left.
- **Shell** (`internal/native/shell/links_test.go`), light and dark:
  - The list: newest first, host, note, tags, "Saved by" lines, tree and row roles, and the Persian title's ink on the right.
  - Search by tag, host and a Persian title with Arabic yeh; a note doesn't match.
  - A safe link opens; an unsafe one doesn't and shows its toast; the unsafe link's menu has no Open.
  - Copy link.
  - Add: a URL without a scheme is refused, then a Persian title and tags are added. Edit.
  - Delete, then Undo from the toast.
  - Live: an agent-saved link appears after `project:links`.
  - The empty state.

Final checks, all with `GOPROXY=off GOMEMLIMIT=1500MiB`:

- `go build ./...` passes.
- `go vet ./...` passes.
- `go test -p 2 -short -count=1 ./...`: every package passes.
- `gofmt -l` on the changed files, with CR stripped: clean.
- `go run ./cmd/jenab-scenarios check scenarios`: everything ok.

Headless screenshots are in my scratchpad, not in the repo: the tab, the add dialog, a delete with its toast, and a search with no match, each in light and dark.

## MyGo bugs found

None.

One thing to know: `Tester.Find(s)` returns the first label equal to `s`. A tree row's accessible name is its title, so `Find(title)` gives the whole row's box, not the title text's. The tests measure only the title's line for that reason. This is not a bug.

## SPEC lines for Main-Session

- **4.6**: "Shown next to the bucket in the **Resources** panel" should be "Shown in the right sidebar's **Links** tab (5.12)". Add:
  - Newest first, searchable by title, host and tag.
  - Only http, https and mailto links open, through SafeLink; others show *Doesn't open* and can be copied.
  - A delete is a change by `user` with Undo from its notice.
  - Every committed write, undo or redo sends `project:links` (P3-11).
  - `_jenab_links` came in project.db format step 7.
- **2.3** or 4.6: the Links tab's search uses the same fold as the chat search, and a space also matches a ZWNJ.
- **4.6**: tags have no commas; a URL is stored normalized (lower-case scheme and host, "/" for an empty http path) and may hold no spaces, controls or direction characters.
- **8.1**, the `save_link` row: "Save a link to the Resources panel" should be "Save a link to the Links tab". The tool itself is still missing and needs a ticket. The store's `SaveLink` with `Merge: true` and a `message:` source is ready for it.
- **5.12**, the right sidebar: the Links tab could get a line saying what it shows: title, host, note, tags, who saved it and when, search, and the row menu (Open, Copy link, Edit, Delete).
