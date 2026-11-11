# SPIKE-041 — Edit tool: matching, shape and protected paths
**Type:** Spike
**Status:** Open
**Gate:** 3 (Phase 6)
**Needs:** —

## Question
How should `edit_file` (8.5) find the text to replace, and what should it say when it can't? Is `edit_file(path, old, new)` with an edit list better than a patch anchored on context lines, on the reference models?

## Scope
1. **Matching passes**, in order; the first pass with exactly one match wins:
   1. exact;
   2. ignoring trailing whitespace;
   3. ignoring leading and trailing whitespace;
   4. with typographic dashes, quotes and spaces folded to ASCII.
2. **Never fold** letters, ZWNJ, bidi marks or Arabic and Persian letter forms (such as ي/ی, ك/ک). Test with Persian files.
3. **Errors:** several matches name the lines and ask for more context; no match shows the closest region.
4. **Writing:** check every edit of a call before writing anything. Keep the file's line endings and BOM. Write to a temp file and rename it.
5. **Shapes compared:** `old`/`new` pairs against a context-anchored patch, on the reference models, with real edits from a fixture repository: success rate, retries and tokens.
6. **Protected paths** for writes: at least `.git/`, Jenab's data and project folders, and the credential patterns already blocked for reading (8.5). Check whether git hooks and similar files that run code need more.
7. **A cancelled edit** names the files already written.

## Rules
- One tool for every model (SPEC 1). A JSON function, not a grammar-constrained free-form tool.
- Write the matching fresh; copy no Codex code (Apache-2.0 would need NOTICE and change statements).
- Windows first; file paths and line endings as Windows has them.

## Done when
- Each question has a measured answer on the reference models.
- The decision is written as SPEC 8.5 text: the tool, its errors and the protected paths.
