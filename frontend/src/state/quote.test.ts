import { expect, test } from "bun:test"
import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"

import { PartKind, Role, type Message } from "@/lib/api"
import { QuoteChip } from "@/chat/quote"
import { QUOTE_MAX, cutQuote, messageText, quoteOf, useQuotes } from "@/state/quote"

const msg = (role: Role, ...texts: string[]): Message => ({
  id: "m1",
  chat: "c1",
  turn: 3,
  role,
  usage: { input: 0, output: 0, cache_read: 0, cache_write: 0 },
  created_at: "2026-11-01T10:00:00Z",
  parts: texts.map((t) => ({ kind: PartKind.PartText, text: { text: t } })),
})

const bytes = (s: string) => new TextEncoder().encode(s).length

test("a short quote is kept, trimmed", () => {
  expect(cutQuote("  سلام دنیا \n")).toEqual({ text: "سلام دنیا", cut: false })
})

test("a long quote is cut at a character, at most QUOTE_MAX bytes, as Go cuts it", () => {
  const { text, cut } = cutQuote("a" + "ب".repeat(QUOTE_MAX / 2))
  expect(cut).toBe(true)
  expect(bytes(text)).toBe(QUOTE_MAX - 1) // the last letter didn't fit
  expect(text.endsWith("ب")).toBe(true)
  // Astral characters are not split either.
  const emoji = cutQuote("😀".repeat(QUOTE_MAX))
  expect(bytes(emoji.text)).toBe(QUOTE_MAX)
  expect(emoji.text).toBe("😀".repeat(QUOTE_MAX / 4))
})

test("a quote names its message, turn and role", () => {
  expect(quoteOf(msg(Role.RoleAssistant, "x"), " the passage ")).toEqual({
    text: "the passage",
    message: "m1",
    turn: 3,
    role: Role.RoleAssistant,
  })
  expect(quoteOf(msg(Role.RoleUser, "x"), "ب".repeat(QUOTE_MAX))?.cut).toBe(true)
})

test("tool messages and empty selections can't be quoted", () => {
  expect(quoteOf(msg(Role.RoleTool, "x"), "x")).toBeNull()
  expect(quoteOf(msg(Role.RoleUser, "x"), " \n ")).toBeNull()
})

test("Reply on a whole message quotes its text parts", () => {
  expect(messageText(msg(Role.RoleAssistant, "first", " ", "second\nline"))).toBe("first\n\nsecond\nline")
})

test("one quote per chat, removable", () => {
  const q = quoteOf(msg(Role.RoleUser, "x"), "x")!
  useQuotes.getState().set("c1", q)
  useQuotes.getState().set("c2", { ...q, text: "y" })
  expect(useQuotes.getState().quotes.c1.text).toBe("x")
  useQuotes.getState().set("c1", null)
  expect(useQuotes.getState().quotes.c1).toBeUndefined()
  expect(useQuotes.getState().quotes.c2.text).toBe("y")
})

test("the chip shows the quote in its own direction, with a remove button", () => {
  const q = { ...quoteOf(msg(Role.RoleAssistant, "x"), "قیمت امروز")!, cut: true }
  const html = renderToStaticMarkup(createElement(QuoteChip, { quote: q, onRemove: () => {} }))
  expect(html).toContain("Replying to the answer")
  expect(html).toMatch(/<span dir="auto"[^>]*>قیمت امروز…<\/span>/)
  expect(html).toContain('aria-label="Remove the quote"')
})
