import { expect, mock, test } from "bun:test"

import * as api from "@/lib/api"
import { ChatKind, ChatState, PartKind, Role, type ChatSnapshot, type ChatStatus, type Delta, type PartDone } from "@/lib/api"

// The store reads chats through ChatService.Snapshot; here each call takes
// the next answer from snapshots.
const snapshots: (() => Promise<ChatSnapshot>)[] = []
mock.module("@/lib/api", () => ({
  ...api,
  ChatService: { ...api.ChatService, Snapshot: () => snapshots.shift()!() },
}))
globalThis.requestAnimationFrame = (f: FrameRequestCallback) => setTimeout(() => f(0), 0) as unknown as number

const { drafts, useThreads } = await import("@/state/thread")
const stream = await import("@/state/stream")

const chat = (id: string) => ({
  id, kind: ChatKind.KindChat, title: id, title_fixed: false, role: "", skills: [], model: "default",
  default_page: "", created_by: "user" as never, created_at: "2026-10-01T10:00:00Z", archived: false,
})

const snap = (id: string, seq: number, live: Partial<ChatSnapshot["live"]> = {}): ChatSnapshot => ({
  chat: chat(id), from: 1, seq,
  messages: [{ id: "01U1", chat: id, turn: 1, role: Role.RoleUser, usage: { input: 0, output: 0, cache_read: 0, cache_write: 0 },
    created_at: "2026-10-01T10:00:00Z", parts: [{ kind: PartKind.PartText, text: { text: "go" } }] }],
  live: { running: false, turn: 1, seq, ...live },
})

const status = (c: string, seq: number, state: ChatState, streaming = ""): ChatStatus =>
  ({ project: "p1", chat: c, seq, state, tasks: 0, streaming }) as ChatStatus
const delta = (c: string, message: string, part: number, text: string, offset = 0): Delta =>
  ({ project: "p1", chat: c, seq: 1, message, part, kind: PartKind.PartText, text, offset })
const part = (c: string, message: string, seq: number): PartDone =>
  ({ project: "p1", chat: c, seq, message, index: 0, part: { kind: PartKind.PartText, text: { text: "go" } } })

const frame = () => new Promise((r) => setTimeout(r, 5))

test("open replays the events that came during the read", async () => {
  let done!: (s: ChatSnapshot) => void
  snapshots.push(() => new Promise((r) => (done = r)))
  const opening = useThreads.getState().open("p1", "r1")
  // Live was read at 4, the messages at 5: the status at 4 came between.
  useThreads.getState().status(status("r1", 4, ChatState.StateWorking, "01A1"))
  useThreads.getState().part(part("r1", "01U1", 5))
  useThreads.getState().delta(delta("r1", "01A1", 0, "Hello"))
  useThreads.getState().delta(delta("r1", "01A1", 0, " there", 5))
  done(snap("r1", 5, { running: true, seq: 4 }))
  await opening
  const t = useThreads.getState().threads.r1
  expect(t.state).toBe(ChatState.StateWorking)
  expect(t.streaming).toBe("01A1")
  expect(t.messages.map((m) => m.id)).toEqual(["01U1"])
  await frame()
  expect(stream.get("r1")?.text).toBe("Hello there")
})

test("a read again knows which deltas the snapshot holds from their offsets", async () => {
  let done!: (s: ChatSnapshot) => void
  snapshots.push(() => new Promise((r) => (done = r)))
  useThreads.getState().delta(delta("r1", "01A1", 0, "\n", 11)) // waits for a frame
  useThreads.getState().part(part("r1", "01X1", 9)) // a gap: the chat is read again
  useThreads.getState().delta(delta("r1", "01A1", 0, "\n", 12)) // came after the snapshot read Live
  done(snap("r1", 9, { running: true, seq: 9, streaming: "01A1", kind: PartKind.PartText, text: "Hello there\n" }))
  await frame()
  expect(stream.get("r1")?.text).toBe("Hello there\n\n")
})

test("open tries a failed first read again", async () => {
  snapshots.push(() => Promise.reject(new Error("busy")), () => Promise.resolve(snap("r2", 1)))
  await useThreads.getState().open("p1", "r2")
  expect(useThreads.getState().threads.r2?.seq).toBe(1)
  expect(useThreads.getState().failed.r2).toBeUndefined()
})

test("joined skips a message the turn already took", () => {
  const base = { ...useThreads.getState().threads.r1, state: ChatState.StateWorking, streamed: ["01B1"], joining: [] }
  useThreads.setState((s) => ({ threads: { ...s.threads, j1: { ...base, chat: chat("j1") } } }))
  useThreads.getState().joined("j1", "01A9") // sent before the try now streaming started
  expect(useThreads.getState().threads.j1.joining).toEqual([])
  useThreads.getState().joined("j1", "01C1")
  expect(useThreads.getState().threads.j1.joining).toEqual(["01C1"])
  useThreads.setState((s) => ({ threads: { ...s.threads, j1: { ...s.threads.j1, state: ChatState.StateIdle, joining: [] } } }))
  useThreads.getState().joined("j1", "01D1") // the turn ended first
  expect(useThreads.getState().threads.j1.joining).toEqual([])
})

test("a chat gone from its list is forgotten, but the one shown", () => {
  const t = useThreads.getState().threads.r1
  useThreads.setState((s) => ({ threads: { ...s.threads, g1: { ...t, chat: chat("g1") }, g2: { ...t, project: "p2", chat: chat("g2") } } }))
  drafts.set("g1", "half a thought")
  // r2 was opened last, so it is shown.
  useThreads.getState().gone("p1", ["r1"])
  const ids = Object.keys(useThreads.getState().threads).sort()
  expect(ids).toEqual(["g2", "r1", "r2"])
  expect(drafts.has("g1")).toBe(false)
  expect(stream.get("g1")).toBeNull()
})
