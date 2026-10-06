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
import { copyText, firstRows, lastNotice, results, rows, splitJoining } from "@/chat/rows"
import { addDelta, fromLive, type Stream } from "@/state/stream"
import { fresh, fromSnapshot, share, withPart, withStatus, type Event, type Thread } from "@/state/thread"

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
  t = withPart(t, part("a2", 0, 3, "ok", PartKind.PartToolResult))!
  t = withPart(t, part("a3", 0, 4, "failed", PartKind.PartNotice))!
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

const delta = (message: string, p: number, text: string, kind = PartKind.PartText, offset = 0): Delta =>
  ({ project: "p1", chat: "c1", seq: 1, message, part: p, kind, text, offset })

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

test("fresh drops the text a snapshot already has, by offset", () => {
  const live: Stream = { message: "a1", parts: [], index: 0, kind: PartKind.PartText, text: "Hello wor" }
  const d = (text: string, offset: number): Event => ({ delta: delta("a1", 0, text, PartKind.PartText, offset) })
  const texts = (q: Event[]) => fresh(q, live).map((e) => ("delta" in e ? e.delta.text : ""))
  expect(texts([d("Hello ", 0), d("wor", 6), d("ld", 9)])).toEqual(["ld"])
  expect(texts([d("wo", 6), d("rld", 8)])).toEqual(["ld"]) // half held: the rest is kept
  // A repeated end is a real token when its offset is past the snapshot.
  expect(texts([d("r", 9)])).toEqual(["r"])
  expect(fresh([d("x", 0)], null)).toHaveLength(1)
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

// IDs below grow in the order they were made, as ULIDs do.

test("a late delta for an earlier part leaves the stream alone", () => {
  let s = addDelta(null, delta("a1", 0, "One"))
  s = addDelta(s, delta("a1", 1, "Two"))
  const late = addDelta(s, delta("a1", 0, " more"))
  expect(late).toBe(s)
  expect([late.index, late.text, late.parts.map((p) => p.text?.text)]).toEqual([1, "Two", ["One"]])
})

test("a message sent during a turn sorts after the answer and results made before it", () => {
  let t = fromSnapshot(undefined, "p1", snap([msg("00U1", 1, Role.RoleUser, "go")], 3, 1, { running: true, streaming: "01A1" }))
  // The user writes while the answer streams; it joins.
  t = withPart(t, part("01U2", 0, 4, "also this"))!
  t = { ...t, joining: ["01U2"] }
  expect(t.messages.map((m) => [m.id, m.turn])).toEqual([["00U1", 1], ["01U2", 1]])
  // The answer and the tool message got their IDs when the answer started.
  const call: PartDone = { ...part("01A1", 0, 5), part: { kind: PartKind.PartToolCall, tool_call: { id: "k1", name: "query", args: {} as never } } }
  t = withPart(t, call)!
  t = withPart(t, part("01A2", 0, 6, "1 row", PartKind.PartToolResult))!
  expect(t.messages.map((m) => m.id)).toEqual(["00U1", "01A1", "01A2", "01U2"])
  const rs = rows(t.messages)
  expect(rs.map((r) => r.kind)).toEqual(["user", "agent", "user"])
  if (rs[1].kind !== "agent") throw new Error("not agent")
  expect(results(rs[1].messages).get("k1")?.text).toBe("1 row")
  // The joining message shows last; the agent's work stays the turn's last row.
  const [shown, joined] = splitJoining(rs, t.joining)
  expect(shown.map((r) => r.id)).toEqual(["00U1", "01A1"])
  expect(joined.map((r) => r.id)).toEqual(["01U2"])
  // Once the next try takes it, it is an ordinary message.
  expect(splitJoining(rs, [])[1]).toEqual([])
})

test("a new message's turn comes from PartDone, or is guessed", () => {
  const ms = [msg("01U3", 3, Role.RoleUser, "third"), msg("01A3", 3, Role.RoleAssistant, "Done.")]
  const idle = fromSnapshot(undefined, "p1", snap(ms, 7))
  // Sent while idle: the next turn, so turn 3 keeps its footer.
  const sent = withPart(idle, part("01U4", 0, 8, "fourth"))!
  expect(sent.messages.at(-1)?.turn).toBe(4)
  expect(rows(sent.messages).map((r) => (r.kind === "agent" ? r.last : null))).toEqual([null, true, null])
  // A notice while idle, such as a new approval level, stays in the last turn.
  expect(withPart(idle, part("01N4", 0, 8, "x", PartKind.PartNotice))!.messages.at(-1)?.turn).toBe(3)
  // Sent during a turn: it joins that turn.
  const busy = withStatus(idle, status(7, ChatState.StateWorking))
  expect(withPart(busy, part("01U4", 0, 8, "fourth"))!.messages.at(-1)?.turn).toBe(3)
  // The turn PartDone carries wins.
  const known = { ...part("01U4", 0, 8, "fourth"), turn: 9 } as PartDone
  expect(withPart(busy, known)!.messages.at(-1)?.turn).toBe(9)
})

test("an approval level notice doesn't hide the failed turn", () => {
  const notice = (id: string, kind: NoticeKind, text: string): Message => ({
    ...msg(id, 1, Role.RoleUser),
    parts: [{ kind: PartKind.PartNotice, notice: { kind, text } }],
  })
  const failed = notice("01N1", NoticeKind.NoticeTurnFailed, "no model")
  const level = notice("01N2", NoticeKind.NoticeApprovalLevel, "[level]")
  const ms = [msg("01U1", 1, Role.RoleUser, "go"), failed, level]
  expect(lastNotice(ms)?.kind).toBe(NoticeKind.NoticeTurnFailed)
  expect(lastNotice(ms)?.message).toBe(failed)
  expect(lastNotice([...ms, msg("01U2", 2, Role.RoleUser, "again")])).toBeNull()
  expect(lastNotice([level])).toBeNull()
})

test("a status sent between reading Live and the messages is kept", () => {
  // Live was read at 4; a status came at 4 and a write at 5 before the
  // messages were read.
  const t = fromSnapshot(undefined, "p1", snap([msg("01U1", 1, Role.RoleUser, "go")], 5, 1, { running: true, seq: 4 }))
  expect(t.streaming).toBe("")
  expect(withStatus(t, status(4, ChatState.StateWorking, "01A1")).streaming).toBe("01A1")
  expect(withStatus(t, status(3, ChatState.StateIdle))).toBe(t)
})

// replay runs a read's replay on the pure parts: the snapshot, then the
// queued events less the deltas it holds.
function replay(s: ChatSnapshot, queue: Event[]): { thread: Thread; stream: Stream | null } {
  let thread = fromSnapshot(undefined, "p1", s)
  let st = fromLive(s.live)
  for (const e of fresh(queue, st)) {
    if ("delta" in e) {
      if (thread.streaming === e.delta.message) st = addDelta(st, e.delta)
    } else if ("part" in e) thread = withPart(thread, e.part) ?? thread
    else thread = withStatus(thread, e.status)
  }
  return { thread, stream: st }
}

test("a read replays what came meanwhile, less what the snapshot has", () => {
  const live = {
    running: true, seq: 4, streaming: "01A1", parts: [{ kind: PartKind.PartText, text: { text: "One" } }],
    kind: PartKind.PartText, text: "foo\n",
  }
  const queue: Event[] = [
    { delta: delta("01A1", 0, "late") }, // for the finished part
    { delta: delta("01A1", 1, "\n", PartKind.PartText, 3) }, // already in Live's text
    { status: status(4, ChatState.StateWorking, "01A1") },
    { part: part("01U1", 0, 5, "go") }, // in the snapshot
    { delta: delta("01A1", 1, "bar", PartKind.PartText, 4) },
  ]
  const s = snap([msg("01U1", 1, Role.RoleUser, "go")], 5, 1, live)
  const { thread, stream } = replay(s, queue)
  expect(thread.messages.map((m) => m.id)).toEqual(["01U1"])
  expect(thread.streaming).toBe("01A1")
  expect([stream?.index, stream?.text, stream?.parts.map((p) => p.text?.text)]).toEqual([1, "foo\nbar", ["One"]])
  // A new line after the snapshot is a real token: its offset is past it.
  expect(replay(s, [{ delta: delta("01A1", 1, "\n", PartKind.PartText, 4) }]).stream?.text).toBe("foo\n\n")
  // A status older than Live is dropped; a newer one replaces Live's state.
  expect(replay(s, [{ status: status(3, ChatState.StateIdle) }]).thread.state).toBe(ChatState.StateWorking)
  expect(replay(s, [{ status: status(5, ChatState.StateWaiting) }]).thread.state).toBe(ChatState.StateWaiting)
})

test("a snapshot mid-turn knows the messages that join the answer streaming", () => {
  // IDs grow in the order the messages were made.
  const messages = [
    msg("m1", 1, Role.RoleUser, "go"),
    msg("m2", 1, Role.RoleAssistant, "first step"),
    msg("m3", 1, Role.RoleUser, "sent before the second step"),
    msg("m5", 1, Role.RoleUser, "sent while it streams"),
  ]
  const t = fromSnapshot(undefined, "p1", snap(messages, 5, 1, { running: true, streaming: "m4" }))
  expect(t.joining).toEqual(["m5"])
  expect(fromSnapshot(undefined, "p1", snap(messages, 5, 1, { running: true })).joining).toEqual(["m3", "m5"])
  expect(fromSnapshot(undefined, "p1", snap(messages, 5)).joining).toEqual([])
})
