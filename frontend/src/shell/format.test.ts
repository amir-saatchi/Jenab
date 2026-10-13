import { expect, test } from "bun:test"

import { ChatKind, ChatState, MemoryLevel, type ChatItem } from "@/lib/api"
import { initial, initials } from "@/state/projects"
import { pageTitle, shortcut } from "@/shell/app-shell"
import { memoryText, memoryTip, problemText, workText } from "@/shell/bottom-bar"
import { subline } from "@/shell/chat-sidebar"

const it_ = (state: ChatState, over: Partial<ChatItem["chat"]> = {}, last?: string) =>
  ({ chat: { kind: ChatKind.KindChat, role: "", ...over }, state, last_activity: last }) as ChatItem

test("workText", () => {
  expect(workText([it_(ChatState.StateWorking), it_(ChatState.StateWorking), it_(ChatState.StateWaiting), it_(ChatState.StateIdle)])).toBe("2 working · 1 waiting")
  expect(workText([it_(ChatState.StateWaiting)])).toBe("1 waiting")
  expect(workText([it_(ChatState.StateIdle)])).toBe("")
})

test("memoryText", () => {
  const G = 2 ** 30
  const win = { free: 5.4 * G, total: 29 * G, ram: 15.7 * G, level: MemoryLevel.OK }
  expect(memoryText(win)).toBe("Memory 5.4 GB free")
  expect(memoryTip(win)).toBe("What the system can still hand out: 16 GB RAM + 13 GB page file")
  const linux = { free: 2.1 * G, total: 16 * G, ram: 16 * G, level: MemoryLevel.OK }
  expect(memoryTip(linux)).toBe("Free memory, of 16 GB RAM")
  expect(memoryTip({ ...win, free: 0.3 * G, level: MemoryLevel.Critical })).toContain("almost full")
  const mac = { free: 0, total: 16 * G, ram: 16 * G, level: MemoryLevel.OK }
  expect(memoryText(mac)).toBe("Memory 16 GB")
  expect(memoryText({ ...mac, level: MemoryLevel.Low })).toBe("Memory low")
  expect(memoryTip(mac)).toBe("16 GB RAM")
})

test("problemText", () => {
  expect(problemText({ name: "Z.ai", paused_ms: 29_100 } as never)).toBe("Z.ai paused, retry in 30 s")
})

test("subline", () => {
  const now = new Date("2026-10-05T15:00:00")
  expect(subline(it_(ChatState.StateWaiting), now)).toBe("Waiting for you")
  expect(subline(it_(ChatState.StateIdle, { kind: ChatKind.KindMother }), now)).toBe("Project home")
  expect(subline(it_(ChatState.StateIdle, { role: "Track ETH.\nEvery day." }), now)).toBe("Track ETH.")
  expect(subline(it_(ChatState.StateIdle), now)).toBe("No messages yet")
  expect(subline(it_(ChatState.StateIdle, {}, "2026-10-05T14:02:00"), now)).toStartWith("Last active ")
})

test("initials", () => {
  expect(initial("bitcoin")).toBe("B")
  expect(initials("ETH tracker")).toBe("ET")
  expect(initials("Prices")).toBe("Pr")
  expect(initials("قیمت طلا")).toBe("قط")
  expect(initials("  ")).toBe("?")
})

test("shortcut", () => {
  const k = (key: string, mods: Partial<Record<"ctrlKey" | "shiftKey" | "altKey" | "metaKey", boolean>> = {}) => ({
    key, ctrlKey: false, shiftKey: false, altKey: false, metaKey: false, ...mods,
  })
  expect(shortcut(k("Tab", { ctrlKey: true }), false)).toBe("next")
  expect(shortcut(k("Tab", { ctrlKey: true, shiftKey: true }), false)).toBe("previous")
  expect(shortcut(k("Tab"), false)).toBeNull()
  expect(shortcut(k("ArrowLeft", { altKey: true }), false)).toBe("back")
  expect(shortcut(k("ArrowRight", { altKey: true }), false)).toBe("forward")
  expect(shortcut(k("ArrowLeft", { altKey: true }), true)).toBeNull()
  expect(shortcut(k("[", { metaKey: true }), true)).toBe("back")
  expect(shortcut(k("]", { metaKey: true }), true)).toBe("forward")
})

test("pageTitle names the places with a header of their own below 900 px", () => {
  expect(pageTitle(undefined)).toBe("Jenab")
  expect(pageTitle({ view: "settings", section: "Models" })).toBe("Settings")
  expect(pageTitle({ view: "project-settings", project: "p1" })).toBe("Project settings")
  expect(pageTitle({ view: "inspector" })).toBe("Turn inspector")
  expect(pageTitle({ view: "runtime" })).toBe("Runtime")
})
