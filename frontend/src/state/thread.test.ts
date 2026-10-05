import { expect, test } from "bun:test"

import {
  ChatKind,
  ChatState,
  NoticeKind,
  PartKind,
  Role,
  type ChatSnapshot,
  type ChatStatus,
  type Delta,
  type Message,
  type PartDone,
} from "@/lib/api"
import { copyText, firstRows, lastNotice, results, rows } from "@/chat/rows"
import { addDelta, fromLive, overlap } from "@/state/stream"
import { fromSnapshot, share, withPart, withStatus, type Thread } from "@/state/thread"

const chat = {
  id: "c1", kind: ChatKind.KindChat, title: "Coins", title_fixed: false, role: "", skills: [], model: "default",
  default_page: "", created_by: "user" as never, created_at: "2026-10-01T10:00:00Z", archived: false,
}

function msg(id: string, turn: number, role: Role, ...texts: string[]): Message {
  return {
    id, chat: "c1", turn, role, usage: { input: 0, output: 0, cache_read: 0, cache_write: 0 },
    created_at: "2026-10-01T10:00:00Z", parts: texts.map((text) => ({ kind: PartKind.PartText, text: { text } })),
  }
}

function snap(messages: Message[], seq: number, from = 1, live: Partial<ChatSnapshot["live"]> = {}): ChatSnapshot {
  return { chat, messages, from, seq, live: { running: false, turn: 1, seq, ...live } }
}

const part = (message: string, index: number, seq: number, text = "hi", kind = PartKind.PartText): PartDone => ({
  project: "p1", chat: "c1", seq, message, index,
  part: kind === PartKind.PartText ? { kind, text: { text } } : kind === PartKind.PartToolResult
    ? { kind, tool_result: { call_id: "k1", text } } : { kind, notice: { kind: NoticeKind.NoticeTurnFailed, text } },
})

const status = (seq: number, state: ChatState, streaming = ""): ChatStatus =>
  ({ project: "p1", chat: "c1", seq, state, tasks: 0, streaming }) as ChatStatus

test("a snapshot sets the turn state from Live", () => {
  const t = fromSnapshot(undefined, "p1", snap([msg("m1", 1, Role.RoleUser, "hi")], 4, 1, { running: true, streaming: "a1" }))
  expect(t.state).toBe(ChatState.StateWorking)
  expect(t.streaming).toBe("a1")
  expect(t.streamed).toEqual(["a1"])
  expect(t.seq).toBe(4)
})

test("parts already in the snapshot are dropped; a gap asks for a new one", () => {
  const t = fromSnapshot(undefined, "p1", snap([msg("m1", 1, Role.RoleUser, "hi")], 4))
  expect(withPart(t, part("m1", 0, 4))).toBe(t)
  expect(withPart(t, part("m2", 0, 6))).toBeNull()
  const next = withPart(t, part("m2", 0, 5))!
  expect(next.messages.map((m) => m.id)).toEqual(["m1", "m2"])
  expect(next.seq).toBe(5)
  // Parts of one write share its number.
  const two = withPart(next, part("m2", 1, 5, "more"))!
  expect(two.messages[1].parts?.map((p) => p.text?.text)).toEqual(["hi", "more"])
})

test("a part with the same index replaces the old one", () => {
  const t = fromSnapshot(undefined, "p1", snap([msg("m1", 1, Role.RoleUser, "a", "b")], 1))
  const next = withPart(t, part("m1", 1, 2, "B"))!
  expect(next.messages[0].parts?.map((p) => p.text?.text)).toEqual(["a", "B"])
  expect(next.messages[0]).not.toBe(t.messages[0])
})

test("a new message's role is guessed from the stream and its part", () => {
  let t = fromSnapshot(undefined, "p1", snap([], 1))
  t = withStatus(t, status(1, ChatState.StateWorking, "a1"))
  t = withPart(t, part("a1", 0, 2))!
  t = withPart(t, part("t1", 0, 3, "ok", PartKind.PartToolResult))!
  t = withPart(t, part("n1", 0, 4, "failed", PartKind.PartNotice))!
  expect(t.messages.map((m) => m.role)).toEqual([Role.RoleAssistant, Role.RoleTool, Role.RoleUser])
})

test("a new try ends the joining messages; old statuses are dropped", () => {
  let t = fromSnapshot(undefined, "p1", snap([], 5, 1, { running: true, streaming: "a1" }))
  t = { ...t, joining: ["u2"] }
  expect(withStatus(t, status(4, ChatState.StateIdle))).toBe(t)
  expect(withStatus(t, status(5, ChatState.StateWorking, "a1")).joining).toEqual(["u2"])
  const next = withStatus(t, status(6, ChatState.StateWorking, "a2"))
  expect(next.joining).toEqual([])
  expect(next.streamed).toEqual(["a1", "a2"])
})

test("a new snapshot keeps older turns and unchanged messages", () => {
  const old = [msg("m1", 1, Role.RoleUser, "one"), msg("m2", 2, Role.RoleUser, "two")]
  const prev: Thread = { ...fromSnapshot(undefined, "p1", snap(old.slice(1), 3, 2)), messages: old, from: 1 }
  const next = fromSnapshot(prev, "p1", snap([msg("m2", 2, Role.RoleUser, "two"), msg("m3", 3, Role.RoleUser, "x")], 5, 2))
  expect(next.messages.map((m) => m.id)).toEqual(["m1", "m2", "m3"])
  expect(next.messages[1]).toBe(old[1])
  expect(next.from).toBe(1)
  expect(share(old, [{ ...old[0], parts: [] }])[0]).not.toBe(old[0])
})

const delta = (message: string, p: number, text: string, kind = PartKind.PartText): Delta =>
  ({ project: "p1", chat: "c1", seq: 1, message, part: p, kind, text })

test("deltas build the answer part by part", () => {
  let s = addDelta(null, delta("a1", 0, "Let me ", PartKind.PartThinking))
  s = addDelta(s, delta("a1", 0, "think"))
  expect(s.kind).toBe(PartKind.PartText)
  expect(s.text).toBe("think")
  s = addDelta(s, delta("a1", 0, " more"))
  s = addDelta(s, delta("a1", 2, "Done"))
  expect(s.parts.map((p) => p.text?.text)).toEqual(["think more"])
  expect([s.index, s.text]).toEqual([2, "Done"])
  // Another answer starts over.
  expect(addDelta(s, delta("a2", 0, "x")).parts).toEqual([])
})

test("a snapshot's stream starts after its finished parts", () => {
  expect(fromLive({ running: true, turn: 1, seq: 1 })).toBeNull()
  const s = fromLive({ running: true, turn: 1, seq: 1, streaming: "a1", parts: [{ kind: PartKind.PartText, text: { text: "a" } }], kind: PartKind.PartText, text: "b" })!
  expect([s.index, s.text]).toEqual([1, "b"])
})

test("overlap finds the deltas a snapshot already has", () => {
  expect(overlap("Hello wor", ["wor", "ld"])).toBe(1)
  expect(overlap("Hello world", ["wor", "ld"])).toBe(2)
  expect(overlap("Hello", ["wor", "ld"])).toBe(0)
  expect(overlap("", [])).toBe(0)
})

test("rows group the agent's work and mark turn ends", () => {
  const call = { ...msg("a1", 1, Role.RoleAssistant), parts: [{ kind: PartKind.PartToolCall, tool_call: { id: "k1", name: "query", args: {} as never } }] }
  const tool = { ...msg("t1", 1, Role.RoleTool), parts: [{ kind: PartKind.PartToolResult, tool_result: { call_id: "k1", text: "1 row" } }] }
  const notice = { ...msg("n1", 2, Role.RoleUser), parts: [{ kind: PartKind.PartNotice, notice: { kind: NoticeKind.NoticeTaskFinished, text: "done" } }] }
  const ms = [msg("u1", 1, Role.RoleUser, "go"), call, tool, msg("a2", 1, Role.RoleAssistant, "Done."), notice, msg("a3", 2, Role.RoleAssistant, "News")]
  const rs = rows(ms)
  expect(rs.map((r) => r.kind)).toEqual(["user", "agent", "notice", "agent"])
  expect(rs.map((r) => (r.kind === "agent" ? r.last : null))).toEqual([null, true, null, true])
  if (rs[1].kind !== "agent") throw new Error("not agent")
  expect(results(rs[1].messages).get("k1")?.text).toBe("1 row")
  expect(copyText(rs[1].messages)).toBe("Done.")
  expect(lastNotice(ms)).toBeNull()
  expect(lastNotice(ms.slice(0, 5))?.kind).toBe(NoticeKind.NoticeTaskFinished)
})

test("a chat opens with about two screens of rows", () => {
  const long = Array.from({ length: 30 }, (_, i) => `line ${i}`).join("\n")
  const ms = Array.from({ length: 40 }, (_, i) => msg(`m${i}`, i + 1, Role.RoleUser, i % 2 ? long : "hi"))
  expect(firstRows(rows(ms))).toBe(5)
  expect(firstRows(rows(ms.filter((_, i) => i % 2 === 0)))).toBe(20)
  expect(firstRows(rows(ms.filter((_, i) => i % 2 === 0)), 1000)).toBe(20)
  expect(firstRows(rows([msg("a", 1, Role.RoleUser, long + long + long)]))).toBe(1)
  expect(firstRows([])).toBe(0)
})
