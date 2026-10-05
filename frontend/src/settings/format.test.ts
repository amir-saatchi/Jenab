import { expect, test } from "bun:test"

import type { UsageLine } from "@/lib/api"
import { cost, limitDefault, placeholders, tokens } from "@/settings/format"

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
