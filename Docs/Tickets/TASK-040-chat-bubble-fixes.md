# TASK-040 — Chat bubble fixes: links in the user's message, wide answers
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** none

## Goal
Two small chat display bugs found in TASK-021 and TASK-019.

## Scope
1. **`jenab://` links in the user's own message** show as plain text (TASK-021's follow-up), for example after attaching a file. In answers they open the file in the dock (TASK-015). Make them links in the user's bubble too, opening the same way, without turning the user's text into Markdown: only bare `jenab://` links become links, the rest stays as typed. Right-to-left text around a link keeps its direction.
2. **Answer width:** an answer bubble is only as wide as its longest line. So in a mixed answer, a Persian paragraph is right-aligned within that narrow width, not at the right edge of the chat column (TASK-019). Make an answer take the column's width (as other chat apps do), so a right-to-left paragraph lines up on the right edge and a left-to-right one on the left. Short answers may stay narrow if that looks better, as long as mixed-direction answers line up; say which in the report.

## Rules
- Frontend only, unless a link needs a Go change. Use the existing link handling from TASK-015 and the chat primitives (shadcn `Message`, `Bubble`); no new dependencies.
- Check both by hand in a server-mode build with scratch data folders, at 1280 and 760 px, with English, Persian and mixed text.

## Done when
- Frontend tests: a `jenab://` link in a user message is a link and opens the file; other text in the user message is unchanged (no Markdown); an answer with a Persian paragraph lines up on the right edge.
- Screenshots of both, before and after, in the report.

## Result
Done on 2026-11-06. SPEC 4.5 and 5.8 describe it.

- **Links in the user's message:** bare `jenab://` links become links that open the file in the dock, as in answers; the rest stays as typed (no Markdown, and `https://` links stay text). A link runs left to right on its own, so Persian around it keeps its direction.
- **Answer width:** answers take the column's width, short ones too: the ghost bubble has no background, so English looks the same, and a short Persian answer now sits on the right. A *Reply* flash on an answer now spans the column.
- **Checked:** frontend tests (5 new), and by hand in a server build with a fake model at 1280 and 760 px: links open the file, and every Persian paragraph ends at the same right edge.
- **Open:** while the Markdown renderer loads, for a moment, a mixed answer is one paragraph in the answer's first direction.

The session's report: [reports/TASK-040-report.md](reports/TASK-040-report.md).
