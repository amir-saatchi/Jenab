import { expect, test } from "bun:test"

import { memoryTitle } from "@/chat/cards"
import { estimateText, estimateTitle, undoChangedText } from "@/chat/refresh"

test("a long refresh's dialog says what it costs", () => {
  const e = { turns: 30, chunks: 6, saved: 2, calls: 5, tokens: 24000, model: "fast", long: true }
  expect(estimateTitle(e, false)).toBe("Refresh memory: 30 turns?")
  expect(estimateTitle({ ...e, turns: 1 }, true)).toBe("Review the whole chat again: 1 turn?")
  expect(estimateText(e, "Gemma")).toBe(
    "This makes 5 calls to Gemma, with about 24,000 tokens of input. 2 parts of 6 reviewed before are kept." +
      " Memory changes only when it ends, and Stop cancels it.",
  )
  expect(estimateText({ ...e, saved: 0, calls: 1 }, "Gemma")).toStartWith("This makes 1 call to Gemma, with about 24,000 tokens of input. Memory")
})

test("a batch names its sections by memory", () => {
  expect(memoryTitle("memory:user/preferences")).toBe("User memory · preferences")
  expect(memoryTitle("memory:project/a/b")).toBe("Project memory · a/b")
})

test("an undo that did nothing names the sections that changed", () => {
  expect(undoChangedText(["user memory, section preferences"])).toBe(
    "User memory, section preferences changed since, so nothing was undone. Edit memory in the settings.",
  )
  expect(undoChangedText([])).toStartWith("A section changed since")
})
