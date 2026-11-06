import { expect, test } from "bun:test"
import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"

import { MemoryNote, ToolChip, desktopArgs } from "@/chat/parts"

const note = (n?: string) => renderToStaticMarkup(createElement(MemoryNote, { note: n }))

test("a memory chip's note shows under it, and reads in its own direction", () => {
  const html = note("From a chat that read web pages, files or MCP results")
  expect(html).toContain('<span dir="auto">From a chat that read web pages, files or MCP results</span>')
  expect(html).toStartWith("<p ")
})

test("a chip saved without a note shows as before", () => {
  expect(note()).toBe("")
  expect(note("")).toBe("")
})

test("a desktop step shows the lengths of its texts, not the texts (SPEC 8.10)", () => {
  expect(desktopArgs({ id: "2:e7", action: "set_value", text: "my secret sentence" })).toBe("2:e7 · set_value · text: 18 characters")
  expect(desktopArgs('{"full":true}')).toBe("full true")
  // A macro's params are values typed into the window: only their count (SPEC 8.11).
  expect(desktopArgs({ name: "site_search", params: ["query=blue folders"] })).toBe("site_search · params: 1")
  const html = renderToStaticMarkup(
    createElement(ToolChip, {
      call: { id: "c1", name: "desktop_snapshot", args: {} },
      result: { call_id: "c1", text: "Window \"Notes\" (snapshot 1)\n- button \"Save\" [1:e1]" },
      running: false,
    }),
  )
  expect(html).toContain("desktop_snapshot")
  expect(html).toContain("50 characters")
  expect(html).not.toContain("Save")
})
