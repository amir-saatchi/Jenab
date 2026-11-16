# SPIKE-048 Part B report: the chat slice on MyGo

**Branch:** `claude/spike-048-part-b-ff0909`, on top of Part A. Commits: 0a7bab9 (B1, MyGo v0.9.1), 79b1481 (B2, the kit's frame and chat pieces), 17fa09d (B3, the chat's pure logic), 7bc5daa (B4, the frame, the thread and the composer), then B5 (the bench, and the fixes the checks found) and the commit with this report. Nothing is pushed or merged.
**MyGo:** `github.com/egoist/mygo v0.9.1`, from the local module cache, as the user chose. No other dependency changed.
**Downloads:** none.
**Model requests:** one, to `gemini-3.5-flash-lite` with `GEMINI_API_KEY`. Every measurement used the fake model.
**Machine:** the Windows 11 PC that ran SPIKE-022, at 125 % scale. Every run pointed `LOCALAPPDATA`, `APPDATA`, `USERPROFILE`, `TEMP`, `TMP` and Jenab's data folder at a new temporary folder.

## 1. What was built

1. **B1, MyGo v0.9.1.** The kit builds against it unchanged. MyGo now draws tabs ([#1217](https://github.com/egoist/mygo/pull/1217)), so the kit's `ExpandTabs` is gone. `Root` sets tab stops every 4 columns, and code blocks copy `b.Code()` again.
2. **B2, the kit's frame and chat pieces.** Rail, tiles, bar, main area and the virtual list. Message, bubble, marker, card, record, strip, disclosure, radios, the attachment and quote chips, and the composer's input group. Each has a headless test in light and dark, by keyboard and label, with Persian text.
3. **B3, the chat's pure logic in Go** (`internal/native/chat/logic`, 47 tests). Ported from `frontend/src/chat` and `settings`, with their TypeScript tests:
   - the line diff;
   - the rewind and undo texts;
   - pick, rows, parts, cards and notes;
   - refresh;
   - the composer;
   - the provider form's choices.
4. **B4, items 6 to 8.**
   - **The shell** draws the rail, the chat list and the main area (SPEC 5.12), with no dock. Closing the window only hides it, so a tray icon shows the window again and has *Quit*. The rail also has *Quit Jenab*.
   - **The chat screen** shows the thread in a virtualized list:
     - Markdown that streams;
     - thinking parts, tool chips, approval and question cards, and marker rows;
     - *Stop*, *Retry*, *Rewind*, *Edit and resend* and *Undo*.
   - **The composer** has the quote chip, attachments (through the upload route, called in-process), the model picker, and *Send* and *Stop*.
   - **Tests:** `statetest` has in-memory services, so the shell and chat screens are tested headless.
5. **B5, the checks.**
   - **The bench build.** `cmd/desktop-native` built with `-tags bench` (`bench.go`, `bench_shot.go`):
     - it stamps its log in microseconds;
     - it adds marks to the log (projects loaded, open, opened, key);
     - it serves a loopback port that opens chats, sends, types keys into the window, resizes it, takes screenshots (`PrintWindow`) and quits.
     - It keeps secrets in memory, taken from `JENAB_BENCH_KEY_<PROVIDER>`, so it never reads or writes Credential Manager. `main.go` gained a `newKeyring` hook for this.
     - The release build has no bench code.
   - **`cmd/jenab-bench`** gained:
     - **`-native`:** the same seed, user folder and fake model as the Wails runs. MyGo's `MYGO_FRAME_STATS=all` gives the frames.
     - **`-samples`:** the 20 text samples as chats, pictured in both apps at 1280×800, in the thread and the composer.
     - **`-hold`:** keeps the native app open for UI Automation and live checks.
     - **`-gemini`:** connects one Gemini model, with `-hold` only.
     - **A fix for the Wails runs:** the bench now creates `USERPROFILE\AppData\Local`. Without it, WebView2 opened no DevTools port, which is why every Wails run failed with "no DevTools page".
   - **Kit fixes the checks found:**
     - **Code blocks put tab stops every 2 columns**, as the frontend's `typeset.css` does (`tab-size: 2`). The kit's comment had said 4. The composer and other text stay at 4, as in the frontend.
     - **Ten icons had lost their first path** in B2: `send`, `pencil`, `sparkles`, `bell`, `file`, `file-text`, `folder-cog`, `package`, `pin` and `shield-check`. Send drew a slash, and *Edit and resend* drew a tick. They are now regenerated from `lucide-react` 1.48.0, the frontend's version. The Part A icons were already exact.
     - **A selection now runs across messages.** Each Markdown view had kept a selection of its own, so a drag across the thread copied the questions and times but skipped every answer. `kit.Markdown` gained `ShareSelection`, and the thread sets it (MyGo's `Options.ShareSelection`).

### The checks (item 9)

Native and Wails both ran from the same commit with the same seed, the fake model and the same 200-message chat (mixed history). Each app showed 60 of the 200 messages at first. Medians are over the runs shown. The Wails values come from the bench's DevTools probes, and the native values come from MyGo's frame log.

| Check | Native (MyGo) | Wails | Target |
|---|---|---|---|
| Start, process to first frame | 241, 125, 130 ms (100 tok/s run); 278, 171, 155 ms (1,000) | — | — |
| Start, process to the shell drawn | 540, 177, 185 ms; 613, 233, 210 ms | first paint 823, 760, 805 ms; 819, 905, 788 ms | no slower than Wails |
| Opening the 200-message chat, first rows | 12–30 ms (median 14 and 16) | 44–110 ms (median 56 and 50) | no slower than Wails |
| Opening it, all rows built | 12–292 ms (median 37 and 39) | 172–273 ms (median 196 and 186) | no slower than Wails |
| Longest frame or task while opening | 2–16 ms | LoAF 64–66 ms on the first open, then 0 | — |
| Streaming at 100 tok/s | 59.6 fps; frame work p95 4.0 ms, max 62.9 ms (one frame); 0.1 % over 33 ms | 59.9 fps; frames p95 16.9 ms, max 17.6 ms | under 33 ms |
| Streaming at 1,000 tok/s | 59.8 fps; frame work p95 6.9 ms, max 17.8 ms; gaps max 32.4 ms; 0 % over 33 ms | 59.9 fps; frames p95 16.9 ms, max 18.7 ms; 0 % over 33 ms | under 33 ms |
| Key to screen, 100 tok/s | p50 12.0, p95 17.1, max 76.9 ms | p50 15.5, p95 26.6, max 33.9 ms | under 50 ms |
| Key to screen, 1,000 tok/s | p50 11.7, p95 18.2, max 24.1 ms (upper bound p95 33.3) | p50 23.0, p95 35.4, max 51.1 ms | under 50 ms |
| Private memory, idle | 177 MB; 183 MB | 351 MB; 365 MB | — |
| Private memory, the 200-message chat open | **186 MB; 191 MB** | **531 MB; 551 MB** | at most half of Wails |
| After streaming in it | 209 MB; 303 MB | 550 MB; 702 MB | — |
| Peak, 10 answers at once | 225 MB; 418 MB | 654 MB; 770 MB | — |
| After leaving for an empty chat | 224 MB; 406 MB | 459 MB; 495 MB (406 and 453 after memory pressure) | — |
| Ten opens and leaves | 353–405 MB (1,000 tok/s run) | 484–495 MB | — |

Notes on the table:
- **Memory** is private bytes summed over all of the app's processes. That is 2 for native and 8 for Wails (WebView2).
- **Start-up.** "Cold" is the first start in a new user folder. A start after a reboot or with the file cache flushed was not run: flushing needs admin rights or a reboot.
- **Key to screen.** Native measures from the `WM_KEYDOWN` post to the end of the first frame after it. The "upper bound" adds one more frame, for the present. Wails measures from the key event to the next paint.
- **Native memory at 1,000 tok/s.** It grows with the 20,000-token answers (303 MB after streaming, 406 MB after leaving), and Go keeps the heap. The ten open-and-leave cycles stay flat, so it is not a leak. The app sends no memory-pressure signal, unlike the Wails bench.

**Text, 20 samples.** Both apps were pictured at 1280×800, at 125 % scale. Chromium means headless Edge for the font checks and the Wails app's WebView2 for the samples. Lines wrap at different places only because the Wails app shows the dock.

- **Same as Chromium (16):**
  - 1 to 10: Persian, English, mixed, digits, ZWNJ, brackets, the heading, quotes;
  - 11: Go code with tabs, after the tab fix;
  - 12 and 13: inline code and the English table;
  - 17 and 18: the nested list, and links with an email;
  - 20: emphasis with an emoji.

  Every composer matches too.
- **Different (4):**
  - **14, a Persian table:** the columns run left to right. Chromium runs them right to left, since the frontend gives a table the direction of its first strong character (`dir={dirOf(node)}`).
  - **15, a Persian bullet list, and 16, a mixed numbered list:** the markers stay on the left of Persian items. The frontend sets `dir` on each `li`, so Chromium puts them on the right.
  - **19, Arabic harakat:** MyGo drops some marks: the fatha on ک, the sukun on ل after alef, and the kasra under ر. Tanvin (ـًا) and shadda draw. Headless Edge draws the kit's own Vazirmatn TTF correctly at weights 100, 400 and 900, so the font is fine and the fault is in MyGo's DirectWrite path.
- **Style only:**
  - the quote's text is muted;
  - links are blue rather than underlined;
  - the code block shows a language label;
  - the table borders differ;
  - the time shows as 13:11 rather than 01:11 PM;
  - the native composer has no *Approvals* picker.

**Narrator.** This was checked through UI Automation, not by listening. In the native window's UIA tree:
- The rail is a "Projects" group of named buttons, the chat list a "Chats" group.
- The thread is a "Messages" list. Each item is "You said" or "Jenab said" and holds the message text, with its buttons named *Copy*, *Reply*, *Edit and resend* and *Rewind to here*.
- The composer is an Edit named "Message", with the Value and Text patterns, followed by *Attach files*, *Model: bench-model* and *Send*.
- When an answer finishes, "Answer finished" appears in MyGo's polite live region at the end of the tree. MyGo raises `LiveRegionChanged` there.
- A `System.Windows.Automation` listener in PowerShell got no event. That client also can't read `LiveSetting` ("Unsupported Property"), so this check can't tell whether Narrator would hear it. MyGo chose the live region over notification events because those "reached no client in tests", in its comment.
- Nobody listened to Narrator itself.

**Copying.** A drag across messages copies them as plain text, as a browser does. The headless test `TestASelectionCopiesAcrossMessages` covers it: "first question / the first answer / 13:31 / second question / the … answer". An answer's *Copy* button copies its Markdown, as in the Wails app. A browser selection also carries HTML. MyGo's own Markdown selection would carry HTML and Markdown, but a selection shared across messages copies text only.

**Live checks, on the bench build, in a temporary user folder:**
- **Close:** `WM_CLOSE` hid the window. The process kept running and answered on its port. *Show* brought the window back.
- **Size and place:** the window, moved to 213,117 at 1111×733, came back there after a quit and a new start (`window-state.json` under `APPDATA\Jenab`).
- **Weights:** DirectWrite draws the semibold heading, the bold run and the regular text as Chromium does (sample 8).
- **A notification click:** not checked. It needs a person to click the toast. The code shows the window and sends `app:open`, as the Wails app does.
- **Gemini:** one answer from `gemini-3.5-flash-lite` in 3.1 s: "پایتخت ایران تهران است.", shown under "Gemini 3.5 Flash-Lite". The key went from `.env` into the bench app's memory only.

**Tests:** `go vet` is clean, and `go test ./...` passes. In `frontend`, `bun run typecheck` and `bun test` pass. The frontend is unchanged.

## 2. What MyGo lacked or got wrong

1. **Arabic harakat drop on Windows** (sample 19).
   - **What's missing:** some marks don't draw, among them the fatha on ک, the sukun on ل and the kasra under ر.
   - **Where it fails:** both the headless tester and the real window use DirectWrite (`internal/text/dwrite_windows.go`).
   - **Not the font:** go-text shapes the same marks with offsets (fatha on ک: y offset 9 at 100 px), and Edge draws the same TTF correctly.
   - **Impact:** Persian text rarely carries harakat, but Arabic quotes, the Quran and taught Persian do.
   - **Fix:** the kit can't fix this.
2. **Markdown lists and tables have no direction.**
   - **What MyGo does:** `ui/markdown` lays a list item out as a row of marker and content, and a table in column order. Both always run left to right.
   - **No direct option:** `Options` has nothing for it.
   - **Kit route:** the kit could build lists and tables itself through `Options.Block`, which MyGo asks first for each block. That is a fix for the kit, not yet made.
3. **The tester draws through DirectWrite on Windows.** The headless tests therefore check the same text path as the window, which is good. The harakat bug, though, shows only in pixels: the tester's `Texts` and access names have every mark.
4. **Frame stats have no input timestamps.** `MYGO_FRAME_STATS` logs each frame's duration and nothing else. Key-to-screen time needs the app's own marks plus the log's timestamps, so it has a one-frame upper bound.
5. **From Part A, not rechecked on v0.9.1:**
   - the sidebar's selected-item colors;
   - text-field values missing from `Tester.Texts`.

These were mistakes in the kit, not in MyGo, and are fixed in B5:
- the code blocks' tab size;
- the ten clipped icons;
- the thread's selection (MyGo has `ShareSelection` for exactly this).

## 3. Upstream issues worth filing

None was filed: posting to a public tracker needs the user's OK.

1. **Harakat on Windows.**
   - **Report:** some Arabic marks don't draw in `ui.Text` on Windows (DirectWrite), while go-text shapes them.
   - **Attach:** sample 19, the screenshot pair, and the go-text offsets.
2. **Direction for Markdown lists and tables.** Each list item and table could take the direction of its first strong character, as `dir="auto"` does, or an option could set it.
3. **Input timestamps in frame stats.** Optional: a frame stat could carry the input event that started the frame. Measuring input latency would then need no app code.
4. **From Part A,** if still present in v0.9.1:
   - the sidebar's selected-item colors;
   - text-field text in the tester.

## 4. Hours

Part B took about 1.7 hours of agent time: from Part A's last commit (13:01) to this report (14:45). B1 to B4, the building, took about an hour. The checks and their fixes took about 0.7 hour, and most of that went into the bench rather than into the app:
- the native mode and the samples;
- finding why WebView2 opened no DevTools port;
- the UI Automation scripts.

Parts A and B together took about 2.5 hours. The user answered one question (the MyGo version).

**Size of the slice:**

| | Code lines | Test lines | Tests |
|---|---|---|---|
| Kit (`internal/native/kit`), 109 exported functions, 41 icons | 1,391 | 957 | 28 |
| Chat screen (`internal/native/chat`) | 1,786 | 343 | 8 |
| Chat logic (`internal/native/chat/logic`) | 1,598 | 1,068 | 47 |
| Shell (`internal/native/shell`) | 426 | 196 | 4 |
| Stores (`internal/native/state`) | 2,574 | 1,371 | 54 |
| `cmd/desktop-native` (with the bench build) | 718 | 47 | — |
| `cmd/jenab-bench` (with the native mode) | 2,787 | — | — |

The slice stands in for about 4,700 lines of the frontend: `chat` (3,569), the stores (1,149) and the shell's frame.

**Estimate for the rest.** What's left in the frontend:
- `settings` (1,973 lines);
- most of `shell` (about 2,000 of 3,011: the files panel, the file view, the dock, project pages, the welcome page);
- `dev` (936).

That is about 4,900 lines, the same size as the slice. The kit also needs:
- a chart (usage);
- a tree (files);
- tables;
- the dock's panels.

At this slice's pace, porting the screens is 5 to 8 hours of agent time across the seven tickets after a go. This estimate doesn't include:
- pages and pipelines (5.9, 6.10), which are new work, not a port;
- the updater and the installer on MyGo;
- macOS and Linux, while CI is blocked;
- the user's own live checks.

Each ticket also needs a person's review and a live run.

## 5. What's next

1. **Kit: direction for lists and tables** through `Options.Block` (samples 14 to 16), then picture the samples again. With that done, 19 of 20 match.
2. **Harakat:** file the upstream issue (with the OK) and wait for MyGo, or accept the gap. The kit can't draw them.
3. **A person's checks:**
   - ten minutes of Narrator in the chat: reading messages, buttons and the composer, and hearing "Answer finished";
   - one notification click.
4. **Small gaps found on the way:**
   - the composer's *Approvals* picker;
   - the time in the user's locale format (01:11 PM) rather than 24-hour.
5. **The go decision** is the user's. The spike doc's Result has the criteria and my recommendation.
