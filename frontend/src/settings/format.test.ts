import { expect, test } from "bun:test"

import type { UsageLine, VersionInfo } from "@/lib/api"
import { buildLine, cost, limitDefault, placeholders, readNumber, tokens, versionLine } from "@/settings/format"

const line = (c: number, unpriced = 0): UsageLine =>
  ({ input: 0, output: 0, cache_read: 0, cache_write: 0, cost: c, unpriced }) as UsageLine

test("token counts and costs read short", () => {
  expect([tokens(950), tokens(12500), tokens(1_200_000)]).toEqual(["950", "12.5K", "1.2M"])
  expect([cost(line(0)), cost(line(4.575)), cost(line(0.004)), cost(line(1, 10)), cost(line(0, 10))]).toEqual([
    "$0.00", "$4.58", "<$0.01", "$1.00+", "—",
  ])
})

test("a local Ollama gets one background call", () => {
  expect(limitDefault("ollama", "", 8)).toBe(1)
  expect(limitDefault("ollama", "http://127.0.0.1:11434/", 8)).toBe(1)
  expect(limitDefault("ollama", "https://ollama.com/", 8)).toBe(8)
  expect(limitDefault("gemini", "", 8)).toBe(8)
})

test("base URL placeholders are found", () => {
  expect(placeholders("https://api.cloudflare.com/client/v4/accounts/{account_id}/ai/v1/")).toEqual(["account_id"])
  expect(placeholders(undefined)).toEqual([])
})

test("readNumber: the same, bad, or the value to save", () => {
  expect(readNumber(" 4 ", "4", 1)).toBe("same")
  expect(readNumber("8", "4", 1)).toEqual({ value: 8 })
  expect(readNumber("0", "4", 1)).toBe("bad")
  expect(readNumber("0", "4", 0)).toEqual({ value: 0 })
  expect(readNumber("2.5", "4", 1)).toBe("bad")
  expect(readNumber("", "4", 0)).toBe("bad") // empty is no zero
  expect(readNumber("", "4", 1, true)).toEqual({ value: null })
  expect(readNumber("", "", 1, true)).toBe("same")
})

test("the About line names the version and the commit", () => {
  const v: VersionInfo = { version: "0.1.0-dev", commit: "d449b7e", modified: false, go: "go1.26.10", wails: "v3.0.0", os: "windows/amd64" }
  expect(versionLine(v)).toBe("Jenab 0.1.0-dev (d449b7e)")
  expect(versionLine({ ...v, modified: true })).toBe("Jenab 0.1.0-dev (d449b7e, modified)")
  expect(versionLine({ ...v, version: "dev", commit: "" })).toBe("Jenab dev")
  expect(buildLine(v)).toBe("Go 1.26.10, Wails v3.0.0, windows/amd64")
  expect(buildLine({ ...v, wails: "" })).toBe("Go 1.26.10, windows/amd64")
})
