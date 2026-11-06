import { expect, test } from "bun:test"

import { NoticeKind, PartKind, Role, type Message, type RewindInfo, type RewindState, type TurnPlan } from "@/lib/api"
import { markText, marks, rewindLines, turnStarts, turnsText } from "@/chat/rewind-text"

function msg(id: string, turn: number, role: Role, notice = false): Message {
  return {
    id, chat: "c1", turn, role, usage: { input: 0, output: 0, cache_read: 0, cache_write: 0 }, created_at: "2026-10-01T10:00:00Z",
    parts: [notice ? { kind: PartKind.PartNotice, notice: { kind: NoticeKind.NoticeTurnUndone, text: "n" } } : { kind: PartKind.PartText, text: { text: id } }],
  }
}

const rw = (id: number, turn: number, last_turn: number, undoable = false): RewindInfo => ({ id, turn, last_turn, at: "2026-10-01T10:00:00Z", undoable })

const plan = (over: Partial<TurnPlan> = {}): TurnPlan =>
  ({ changes: 0, rows: [], summary: "", display: "", past: false, entries: 0, too_big: false, left: 0, ...over }) as TurnPlan

test("turnsText", () => {
  expect(turnsText(4, 4)).toBe("turn 4")
  expect(turnsText(4, 6)).toBe("turns 4–6")
})

// Only the user message that starts a turn gets the actions: not one
// that joined the turn, nor a notice.
test("turnStarts", () => {
  const ms = [
    msg("u1", 1, Role.RoleUser), msg("a1", 1, Role.RoleAssistant), msg("j1", 1, Role.RoleUser),
    msg("n2", 2, Role.RoleUser, true), msg("a2", 2, Role.RoleAssistant),
    msg("u3", 3, Role.RoleUser),
  ]
  expect([...turnStarts(ms)]).toEqual([["u1", 1], ["u3", 3]])
})

// A later rewind to an earlier turn moves the marks of the rewinds
// before it up to its own place.
test("marks", () => {
  const a = rw(1, 5, 6), b = rw(2, 2, 4), c = rw(3, 3, 3, true)
  expect(marks([c, a, b]).map((m) => [m.rewind.id, m.at])).toEqual([[1, 2], [2, 2], [3, 3]])
  expect(markText(a)).toBe("Turn 6 taken out")
  expect(markText(b)).toBe("Turns 3–4 taken out")
})

test("rewindLines", () => {
  const st: RewindState = { turn: 1, last_turn: 3, changes: plan({ changes: 1, display: "2 rows in prices" }), notes: true, unload: ["sql"], load: [] }
  expect(rewindLines(st, false)).toEqual([
    "Turns 2–3 leave this chat and what the model knows of it.",
    "What they changed in the project stays: 2 rows in prices. Undo them from the rewind's mark.",
    "The session notes go back to how they were.",
    "Skills unloaded: sql.",
    "Undo rewind brings the turns back, until the chat goes on.",
  ])
  const one: RewindState = { turn: 2, last_turn: 3, changes: plan(), notes: false, unload: null as never, load: ["views"] }
  expect(rewindLines(one, true)).toEqual([
    "Turn 3 leaves this chat and what the model knows of it. Your message goes back in the box, to edit and send again.",
    "Skills loaded again: views.",
    "Undo rewind brings the turns back, until the chat goes on.",
  ])
})
