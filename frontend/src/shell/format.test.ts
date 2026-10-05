import { expect, test } from "bun:test"

import { ChatKind, ChatState, MemoryLevel, type ChatItem } from "@/lib/api"
import { initial, initials } from "@/state/projects"
import { shortcut } from "@/shell/app-shell"
import { memoryText, modelText, problemText, workText } from "@/shell/bottom-bar"
import { subline } from "@/shell/chat-sidebar"

const it_ = (state: ChatState, over: Partial<ChatItem["chat"]> = {}, last?: string) =>
  ({ chat: { kind: ChatKind.KindChat, role: "", ...over }, state, last_activity: last }) as ChatItem

test("workText", () => {
  expect(workText([it_(ChatState.StateWorking), it_(ChatState.StateWorking), it_(ChatState.StateWaiting), it_(ChatState.StateIdle)])).toBe("2 working · 1 waiting")
  expect(workText([it_(ChatState.StateWaiting)])).toBe("1 waiting")
  expect(workText([it_(ChatState.StateIdle)])).toBe("")
})

test("memoryText", () => {
  expect(memoryText({ free: 2.1 * 2 ** 30, total: 16 * 2 ** 30, level: MemoryLevel.OK })).toBe("RAM 2.1 / 16 GB")
  expect(memoryText({ free: 0, total: 16 * 2 ** 30, level: MemoryLevel.Low })).toBe("RAM low") // macOS
})

test("modelText", () => {
  expect(modelText("default", { default: "google/gemma-4-31b" })).toBe("gemma-4-31b")
  expect(modelText("", { default: "p/m1" })).toBe("m1")
  expect(modelText("zai/glm-4.6", {})).toBe("glm-4.6")
  expect(modelText("default", {})).toBe("No model")
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
