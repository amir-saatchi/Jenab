import { expect, test } from "bun:test"
import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"

import { MemoryNote } from "@/chat/parts"

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
