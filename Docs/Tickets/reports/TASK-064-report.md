# TASK-064 report: links in Markdown open only safe addresses

**Base:** `main` at eb1031d, with MyGo v0.9.1 pinned.
**Model requests:** none. The tests use `statetest`'s fake services. The live check seeded a chat on disk and sent no message.
**Downloads:** none. **New dependencies:** none.
**Real data:** the live check used a scratch profile, deleted after the run. No system dialogs were opened.

## 1. The cause

`kit.Markdown.prepare` set MyGo's `markdown.Options.OnLink` whenever the screen set `Own`. MyGo's `link()` gives every click to `OnLink` when it is set, before its own `#` scroll and its filter (`linkAllowed`, which ends in `ui.SafeToOpen`). The kit's handler then gave every link `Own` declined to `c.OpenURL`, and on Windows `OpenURL` ends in `ShellExecuteW`. The chat and the file view both set `Own`, so `[docs](calc.exe)`, `file:///…` or `javascript:` in an answer or a Markdown file opened or ran with one click.

## 2. One rule in the kit

- **`kit.SafeLink(url)`** (`kit/safelink.go`) is MyGo's `ui.SafeToOpen`. It allows only http, https and mailto, in any case. It refuses control characters, DEL and backslashes.
- **`Markdown.click`** (`kit/markdown.go`) now handles every click in every `kit.Markdown`, with or without `Own`. It runs the same order for answers, user messages, files and pages:
  1. A `#heading` link scrolls to its heading, as MyGo's default does (section 3).
  2. A link `Own` takes goes to the app (`jenab://`, SPEC 4.5).
  3. A link `SafeLink` refuses opens nothing.
  4. A safe link goes to `OnLink` if the screen set one, or else to `c.OpenURL`.
- **`Own` is asked before `SafeLink`,** so `Own` must take only links it knows. Both `own` functions take only `state.IsLink` links. The chat's `own` (`chat/links.go`) also passes safe links to `Screen.OnLink` when that is set. It declines anything `SafeLink` refuses, so the kit drops those.
- **`Markdown.OnLink`'s meaning changed.** It now gets only links `SafeLink` allows. Before, with `Own` unset, it got every link, and MyGo's filter never ran.

## 3. `#heading` links

MyGo scrolls to a heading through `Options.Anchor` and `AnchorVersion`. One `kit.Markdown` can show many texts, since the chat shares one for the whole thread. So a click records the anchor with the text of the view it came from (`building`, the text `prepare` saw last). The next `prepare` for that text passes the anchor once, then clears it. When the text is still streaming, a text that starts with the recorded one also counts. The other views get no anchor, so a row coming back into the virtual list doesn't scroll again. `c.After(0)` asks for the frame that scrolls. A heading that doesn't exist does nothing.

MyGo scrolls the heading just into view, at the bottom edge of the scroll, rather than to the top.

## 4. Every place in `internal/native` that opens something from data

| Place | What it opens | Guard |
|---|---|---|
| `kit/markdown.go`, `Markdown.click` | Links in answers, the chat's welcome text, Markdown files | `#` scrolls, then `Own`, then `SafeLink`, before `OnLink` or `c.OpenURL`. **This is the only `c.OpenURL` call in `internal/native`.** |
| `kit/linktext.go`, `LinkText` and `LinkBubble` | Nothing in the system. `ui.Link` gets an empty URL, so MyGo opens nothing, and the caller's function runs. | The callers: |
| ↳ `chat/links.go`, `bubble` | Bare `jenab://` links in the user's message, through `st.OpenLink`, in the app | `state.SplitLinks` marks only links `ParseLink` accepts. Other text in the message is never a link. |
| ↳ `chat/links.go`, `storedAs` | A tool output's bucket key, through `st.OpenHere`, in the app | A key, not a URL |
| `files/view.go`, `files/panel.go` | Markdown files use `kit.Markdown` with `Own: s.own` (jenab only), so the first row applies. *Copy link* copies text (`kit.CopyText`). | — |
| *Open with the system app* (`files/view.go`) | `BucketService.OpenWithSystem`, then `mygo.Shell.OpenPath` (`cmd/desktop/main.go`, `OpenFile`) | SPEC 4.7, unchanged: types on an allow list only, a read-only temp copy with the detected type's extension, marked as downloaded. It never opens a URL. |
| `kit/link.go` `Link`, `kit/textlink.go` `TextLink`, `kit/linkbutton.go` `LinkButton` | Nothing. These are buttons that report `Clicked`, used for in-app navigation (settings, the inspector). | — |
| Toasts (`kit/toast.go`, `kit/toastaction.go`), tooltips, desktop notices | Nothing. A toast's action runs a function. No tooltip or notice opens a URL. | — |

`chat.Screen.OnLink` isn't set anywhere in the app, so safe links in answers open in the system.

## 5. Tests

All are headless and run in light and dark.
- **`kit/safelink_test.go`:**
  - `TestSafeLink`: https, HTTPS, http on loopback and mailto are safe. These are not: `file:`, `javascript:`, `calc`, `calc.exe`, `docs/a.md`, `./a.md`, `\\host\share\x.exe`, `//host/share/x.exe`, `https:\\host\x`, a control character (`\x01`, a newline, DEL), a leading space, `ms-settings:`, `jenab://` (the app's, not the system's) and the empty string.
  - `TestMarkdownOpensOnlySafeLinks`: a text with a link of each kind is shown three ways: without `Own`, with `Own`, and with `Own` and `OnLink`. Each link is clicked. Only https and mailto open, through `OnLink` when it is set. The `jenab://` link goes to `Own`. The `\\host` link and the link with a control character can't be Markdown destinations, so they come from inline code through `CodeLink`. That is the path a `jenab://` link in code takes.
  - `TestAHeadingLinkScrolls`: two scroll views share one `Markdown`. A link to a missing heading does nothing. *Go to the end* brings the heading at the bottom of a long text into view. The other view doesn't move, and nothing opens.
- **`chat/links_test.go`, `TestAnAnswerOpensOnlySafeLinks`:**
  - An answer holds web, mail, `calc.exe`, `file:`, `javascript:`, a relative path and a `//host` path. The user's message holds `calc.exe`, `file:///…` and `\\host\share`.
  - Only the answer's seven words are links. Clicking each opens only the https and mailto ones.
  - With `Screen.OnLink` set, it gets only the https link, and the system opens nothing more.
- **`kit.Links(tt)`** (new, `kit/tester.go`) lists a screen's links for screen tests, which can't import MyGo's `ui`. `kit.HasRole` knows `"link"`.

## 6. The live check

- **Setup:** the bench build (`go build -tags bench ./cmd/desktop`) ran with `LOCALAPPDATA`, `APPDATA`, `USERPROFILE` and `TEMP` in a scratch folder. A throwaway seed, not committed, made a project with one chat:
  - The user message reads `Open calc.exe or file:///C:/Windows/notepad.exe`.
  - The answer has links to `https://example.com/`, `calc.exe`, `file:///C:/Windows/notepad.exe`, `javascript:alert(1)`, `docs/a.md`, `//localhost/c$/Windows/notepad.exe` and `ms-settings:display`. A *Go to the end* link points to a heading 40 paragraphs down.
- **Driving:** `POST /open` opened the chat, and UI Automation found the links. The words in the user's message weren't links. The answer had eight.
- **Clicks:** each unsafe link got a real mouse click. Afterwards:
  - Jenab had no child process.
  - No Notepad, Calculator, Explorer or Settings process had started.
  - Jenab kept the focus.
- **The heading:** *Go to the end* moved "The end" from below the window (y 2125) into view (y 822), at the bottom of the thread.
- **The log** had no warnings or errors.
- **Not clicked live:** the https link, which would open the browser on this computer. The tests cover it.
- **Screenshots** stay outside the repo.

## 7. Checks

`go build ./...`, `go vet ./...` and `go test ./...` pass at the repo root.
