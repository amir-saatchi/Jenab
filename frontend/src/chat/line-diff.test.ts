import { expect, test } from "bun:test"

import { changes, lineDiff } from "@/chat/line-diff"

const kinds = (before?: string, after?: string) => lineDiff(before, after).map((o) => `${o.kind} ${o.text}`)

test("lineDiff", () => {
  expect(kinds("a\nb", "a\nb")).toEqual(["same a", "same b"])
  expect(kinds(undefined, "a\nb")).toEqual(["add a", "add b"])
  expect(kinds("a\nb", "")).toEqual(["del a", "del b"])
  expect(kinds("a\nb\nc", "a\nx\nc")).toEqual(["same a", "del b", "add x", "same c"])
  expect(kinds("a\nb\nc", "a\nb\nc\n\n\nplanted")).toEqual(["same a", "same b", "same c", "add ", "add ", "add planted"])
  expect(kinds("a\nb\nc\nd", "b\nc\nplanted\nd")).toEqual(["del a", "same b", "same c", "add planted", "same d"])
  expect(kinds("قیمت طلا\nb", "قیمت طلا\nb\nپاسخ کوتاه")).toEqual(["same قیمت طلا", "same b", "add پاسخ کوتاه"])
})

test("changes", () => {
  expect(changes(lineDiff("a\nb", "a\n\n\n \nplanted\nb"))).toEqual([
    { kind: "add", text: "", blanks: 3 },
    { kind: "add", text: "planted", blanks: 0 },
  ])
  expect(changes(lineDiff("a\nb", "x"))).toEqual([
    { kind: "del", text: "a", blanks: 0 },
    { kind: "del", text: "b", blanks: 0 },
    { kind: "add", text: "x", blanks: 0 },
  ])
  expect(changes(lineDiff("a", "a"))).toEqual([])
})

test("lineDiff of long sections stays correct", () => {
  const before = Array.from({ length: 300 }, (_, i) => `line ${i}`).join("\n")
  const after = before.replace("line 150", "line 150\nplanted")
  expect(changes(lineDiff(before, after))).toEqual([{ kind: "add", text: "planted", blanks: 0 }])
})

test("lineDiff of a section past a few thousand lines finds a planted line", () => {
  const old = Array.from({ length: 1100 }, (_, i) => `line ${i}`)
  const now = [...old]
  now[0] = "line 0, changed"
  now[now.length - 1] = "line 1099, changed"
  now.splice(550, 0, "planted")
  expect(changes(lineDiff(old.join("\n"), now.join("\n")))).toEqual([
    { kind: "del", text: "line 0", blanks: 0 },
    { kind: "add", text: "line 0, changed", blanks: 0 },
    { kind: "add", text: "planted", blanks: 0 },
    { kind: "del", text: "line 1099", blanks: 0 },
    { kind: "add", text: "line 1099, changed", blanks: 0 },
  ])

  // The most lines the cap allows, 4,000 a side, all of them changed.
  const a = Array.from({ length: 4000 }, (_, i) => `a${i}`)
  const b = Array.from({ length: 4000 }, (_, i) => `b${i}`)
  const ops = lineDiff(a.join("\n"), b.join("\n"))
  expect(ops.filter((o) => o.kind === "del").map((o) => o.text)).toEqual(a)
  expect(ops.filter((o) => o.kind === "add").map((o) => o.text)).toEqual(b)
})
