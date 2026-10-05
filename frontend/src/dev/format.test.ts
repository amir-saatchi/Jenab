import { expect, test } from "bun:test"

import { chatOptions, duration, parts, scale, share, size, tick } from "@/dev/format"
import type { TurnItem } from "@/lib/api"

test("times and sizes read short", () => {
  expect([duration(38), duration(1240), duration(4000), duration(184_000), duration(120_000)]).toEqual([
    "38 ms", "1.2 s", "4 s", "3 min 4 s", "2 min",
  ])
  expect([size(64), size(200), size(20_480), size(1_500_000)]).toEqual(["64 B", "0.2 KB", "20.5 KB", "1.5 MB"])
  expect(share(6120, 7412)).toBe("6,120 cached (83 %)")
  expect(share(0, 0)).toBe("")
})

test("the timeline has at most six round ticks", () => {
  expect(scale(3600)).toEqual({ span: 4000, step: 1000, ticks: [0, 1000, 2000, 3000, 4000] })
  expect(scale(0).span).toBe(1)
  const s = scale(130)
  expect([s.step, s.span, tick(s.ticks[1], s.step)]).toEqual([50, 150, "50 ms"])
  for (const t of [1, 7, 99, 1234, 45_000, 600_001]) {
    const s = scale(t)
    expect(s.span).toBeGreaterThanOrEqual(t)
    expect(s.ticks.length).toBeLessThanOrEqual(6)
  }
})

test("each chat is picked once, newest first", () => {
  const t = (project: string, chat: string, turn: number) =>
    ({ project, chat, turn, project_name: "Prices", chat_title: chat === "c1" ? "Mother" : "" }) as TurnItem
  expect(chatOptions([t("p", "c2", 3), t("p", "c1", 2), t("p", "c2", 1)]).map((o) => o.label)).toEqual([
    "Prices · Untitled chat",
    "Prices · Mother",
  ])
  expect(parts(["thinking", "text", "tool_call", "tool_call"])).toBe("thinking · answer · tool call")
})
