import { beforeEach, expect, mock, test } from "bun:test"
import { createElement, isValidElement, type ReactElement, type ReactNode } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { prerender } from "react-dom/static"

import { chatKey, useDock } from "@/state/dock"
import { useFiles } from "@/state/files"
import { useNav } from "@/state/nav"
import { useUI } from "@/state/ui"

// The chat's bubbles (TASK-040): jenab:// links in the user's own text,
// and answers as wide as the column.

const P = "01JABCDEFGHJKMNPQRSTVWXYZ0"
const link = `jenab://${P}/uploads/prices.csv`

// Head stands in for the Go side: the file exists.
const heads: string[] = []
const api = await import("@/lib/api")
mock.module("@/lib/api", () => ({
  ...api,
  BucketService: { ...api.BucketService, Head: async (p: string, k: string) => void heads.push(`${p}/${k}`) },
}))
const { AgentText, UserText } = await import("@/chat/parts")

beforeEach(() => {
  heads.length = 0
  useNav.setState({ history: { places: [{ view: "chat", project: P, chat: "c1" }], index: 0 }, mru: [] })
  useDock.setState({ open: {}, chatHidden: {}, drawer: {}, side: {}, width: {}, listShown: false })
  useFiles.setState({ folder: {}, rev: {} })
  useUI.setState({ rightTab: "pages", rightOverlay: false })
})

const html = (text: string) => renderToStaticMarkup(createElement(UserText, { text }))

test("a jenab:// link in the user's message is a link, running left to right", () => {
  expect(html(`Read this:\n\n${link}`)).toBe(
    `Read this:\n\n<a href="${link}" dir="ltr" class="${anchorClass(`x ${link}`)}">${link}</a>`,
  )
})

test("the rest of the user's message stays as typed: no Markdown", () => {
  const typed = "Keep *stars*, `ticks`, # hashes and [a](https://example.com) as typed.\n- not a list"
  expect(html(typed)).toBe(
    "Keep *stars*, `ticks`, # hashes and [a](https://example.com) as typed.\n- not a list",
  )
  // Only valid links: another scheme or a bad key stays text.
  expect(html("jenab://bad and https://example.com")).toBe("jenab://bad and https://example.com")
})

test("Persian text around a link keeps its words and order", () => {
  const out = html(`لطفاً فایل ${link} را بخوان.`)
  expect(out).toStartWith("لطفاً فایل <a ")
  expect(out).toEndWith("</a> را بخوان.")
})

test("clicking a link in the user's message opens the file next to the chat", async () => {
  const a = anchor(UserText({ text: `see ${link}` }))
  let prevented = false
  a.props.onClick({
    type: "click",
    button: 0,
    currentTarget: { getAttribute: () => link, href: link },
    preventDefault: () => void (prevented = true),
  })
  expect(prevented).toBe(true)
  await new Promise((r) => setTimeout(r, 0))
  expect(heads).toEqual([`${P}/uploads/prices.csv`])
  expect(useDock.getState().open[chatKey(P, "c1")]).toEqual({ kind: "file", key: "uploads/prices.csv" })
  expect(useFiles.getState().folder[P]).toBe("uploads/")
  expect(useUI.getState().rightTab).toBe("files")
})

test("an answer takes the column's width, so a Persian paragraph lines up on the right edge", async () => {
  const text = "Prices went up.\n\nقیمت بالا رفت.\n\nThat is all."
  const { prelude } = await prerender(createElement(AgentText, { text }))
  const out = await new Response(prelude).text()
  // The bubble and its content fill the column, not the longest line.
  const bubble = /<div data-slot="bubble"[^>]*class="([^"]*)"/.exec(out)?.[1] ?? ""
  const content = /<div data-slot="bubble-content"[^>]*class="([^"]*)"/.exec(out)?.[1] ?? ""
  expect(bubble.split(" ")).toContain("w-full")
  expect(bubble.split(" ")).not.toContain("w-fit")
  expect(content.split(" ")).toContain("w-full")
  expect(content.split(" ")).not.toContain("w-fit")
  // Each paragraph goes by its own words; typeset aligns it to its start.
  expect(out).toContain('<p dir="auto">Prices went up.</p>')
  expect(out).toContain('<p dir="auto">قیمت بالا رفت.</p>')
})

// anchor finds the first link among rendered nodes.
function anchor(nodes: ReactNode): ReactElement<{ onClick: (e: unknown) => void; className: string }> {
  const list = Array.isArray(nodes) ? nodes : [nodes]
  const a = list.find((n) => isValidElement(n) && n.type === "a")
  if (!a) throw new Error("no link")
  return a as ReactElement<{ onClick: (e: unknown) => void; className: string }>
}

function anchorClass(text: string) {
  return anchor(UserText({ text })).props.className
}
