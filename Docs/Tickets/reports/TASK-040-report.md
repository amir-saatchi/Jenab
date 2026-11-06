# TASK-040 report: chat bubble fixes

Branch `task-040-chat-bubbles`, head `17918bfbe2f6055f4e0447ab1bc3324bbf8448fc` (one commit on `8d9d286`). Frontend only, no Go change, no new dependencies.

## What changed

1. **`jenab://` links in the user's message** (`frontend/src/chat/parts.tsx`, `thread-view.tsx`)
   - New `UserText` renders the user's text through the existing `splitLinks` (TASK-015). Only a bare, valid `jenab://` link becomes an `<a>`. Everything else is a plain text node, so `*stars*`, `` `ticks` ``, `#`, `[a](b)`, `- items` and other schemes (`https://…`) all stay exactly as typed. It is not Markdown.
   - Clicking or middle-clicking a link opens the file in the dock and shows its folder in Files, just as links in answers do. The answer's click handler moved from `markdown-impl.tsx` to `shell/open-file.ts` as `openAnchor`, and both places now use it. The lazy Markdown chunk is unchanged.
   - Each link gets `dir="ltr"`, which makes it an isolate. Two effects:
     - The link reads left to right inside Persian text.
     - The bubble's `dir="auto"` skips the link and goes by the words. So `لطفاً فایل jenab://… را بخوان` stays right to left, and a message that starts with a link takes its direction from the first word after it.
   - Link style matches Typeset's answer links: inherited colour, weight 500, an underline at 30% that is full on hover, and a ring-coloured outline on focus. All Tailwind with semantic tokens, no `dark:` overrides.
2. **Answer width** (`AgentText` in `parts.tsx`)
   - The ghost `Bubble` and its `BubbleContent` get `w-full` in place of `w-fit` (via `cn`/tailwind-merge). An answer now takes the column's width.
   - Each Markdown paragraph already had `dir="auto"` and `text-align: start`. So a Persian paragraph lines up on the column's right edge, and an English one on the left.
   - **Short answers also take the full width.** The ghost bubble has no background, so a short English answer looks exactly as before. A short Persian answer now sits on the right, which is where it belongs; narrow, it was stuck on the left. Answers while streaming use the same component.
   - One side effect: a *Reply* jump's flash ring on an answer now spans the column, not the longest line.

## Checks

- `bun run typecheck`: clean.
- `bun test`: 132 pass, 0 fail. That includes the new `src/chat/bubbles.test.ts` (5 tests):
  - a link in the user's message is an `<a href dir="ltr">`
  - the rest of the text is unchanged, with no Markdown, and invalid links stay text
  - Persian words around a link keep their order
  - clicking the link opens the file in the dock, shows `uploads/` in Files and switches to the Files tab (`BucketService.Head` is mocked)
  - an answer's bubble and content are `w-full`, and its Persian paragraph is a `<p dir="auto">` (rendered with `react-dom/static` `prerender`, so the lazy Markdown loads)
- `bun run build`: OK (only the existing chunk-size warning).
- Go: unchanged, so no Go checks. A server-mode build (`go build -tags server ./cmd/desktop`) compiled for the hand check.
- **By hand:** a server-mode build with `LOCALAPPDATA` and `USERPROFILE` pointed at the scratchpad, and a fake local Ollama on 127.0.0.1 (no keys). I used the Browser pane at 1280 and 760 px. Two files were uploaded through `/upload/<project>`, then I sent three messages: English with two links and Markdown characters, Persian with a link in the middle, and English.
  - User links are underlined links. A click on the link inside the Persian message opened `prices.csv` in the dock with `uploads/` in Files.
  - The Persian bubble stays right to left (computed `direction: rtl`). The English ones stay left to right.
  - All three answers are 621.6 px wide, the column's width. Every Persian paragraph ends at the same right edge (x = 957 at 1280 px), including the short `قیمت بالا رفت.` that sat at the left before.
  - No console errors.
- The screenshots show the scratchpad path in the sidebar, which has the user name in it. They are not committed.

## Screenshots (kept outside the repo, not committed)

Before:
- `before-1280-links-and-persian.jpg`: user links are plain text
- `before-1280-short-mixed.jpg`: the short mixed answer has its Persian line near the left, and the short Persian answer is on the left
- `before-760.jpg`

After:
- `after-1280-links-and-persian.jpg`
- `after-1280-short-mixed.jpg`
- `after-1280-link-opened-in-dock.jpg`: the link in the Persian message clicked, with the file in the dock
- `after-760-links-and-persian.jpg`
- `after-760-short-mixed.jpg`

## Proposed SPEC text

SPEC 5.8, under **Right-to-left text**, after the quote bullet:

> - An answer takes the chat column's width, so each paragraph lines up on its own start edge: a right-to-left one on the right, a left-to-right one on the left (TASK-040).
> - The user's own message is shown as typed, not as Markdown. Only bare `jenab://` links in it become links, and they open the file as in answers (4.5). A link runs left to right on its own (`dir="ltr"`), so it doesn't change the direction of the text around it (TASK-040).

SPEC 4.5 could add "and in the user's own messages" where it says `jenab://` links open in the dock.

## Open

- The Markdown fallback, shown for a moment while the renderer loads, is still one `<p>` for the whole answer. During that moment a Persian paragraph in a mixed answer follows the answer's first direction.
- Only the jenab:// scheme is linked in user text, as the ticket asks. `https://` links the user types stay text.
