import { expect, test } from "bun:test"

import { splitBlocks } from "@/lib/blocks"

test("splits at blank lines", () => {
  expect(splitBlocks("# Title\n\nOne\ntwo\n\n\nThree")).toEqual(["# Title", "One\ntwo", "Three"])
})

test("keeps fenced code whole", () => {
  const code = "```go\nfunc a() {\n\n}\n```"
  expect(splitBlocks(`Before\n\n${code}\n\nAfter`)).toEqual(["Before", code, "After"])
  expect(splitBlocks("~~~\na\n\n```\nb\n~~~")).toEqual(["~~~\na\n\n```\nb\n~~~"])
})

test("an unclosed fence runs to the end", () => {
  expect(splitBlocks("Text\n\n```\ncode\n\nmore")).toEqual(["Text", "```\ncode\n\nmore"])
})

test("an indented line continues its block", () => {
  expect(splitBlocks("1. one\n\n   more of one\n2. two\n\nEnd")).toEqual(["1. one\n\n   more of one\n2. two", "End"])
})

test("nothing for empty text", () => {
  expect(splitBlocks("")).toEqual([])
  expect(splitBlocks("\n\n")).toEqual([])
})
