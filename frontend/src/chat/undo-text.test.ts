import { expect, test } from "bun:test"

import { ConflictReason, type TurnConflict, type TurnPlan, type TurnUndoState, type UndoConflict } from "@/lib/api"
import { conflictText, isolate as i, isRest, itemName, partly, skipAll } from "@/chat/undo-text"

function conflict(over: Partial<UndoConflict>, chat_title = ""): TurnConflict {
  return { conflict: { item: { table: "prices", key: "[2]" }, reason: ConflictReason.ConflictChanged, ...over }, chat_title }
}

test("itemName", () => {
  expect(itemName({ table: "prices", key: "[2]" })).toBe(`row ${i("2")} of ${i("prices")}`)
  expect(itemName({ table: "pairs", key: '[1,"BTC"]' })).toBe(`row ${i("1")}, ${i("BTC")} of ${i("pairs")}`)
  expect(itemName({ table: "_jenab_objects", key: "object:reports/a.txt" })).toBe(`the file ${i("reports/a.txt")}`)
  expect(itemName({ table: "_jenab_chat_role", key: "01CHAT" })).toBe("the chat's role")
  expect(itemName({ table: "prices", key: "" })).toBe(`the table ${i("prices")}`)
  expect(itemName({ table: "_jenab_memory", key: "memory:project/goals" })).toBe(`the project memory section ${i("goals")}`)
  expect(itemName({ table: "_jenab_user_memory", key: "ترجیحات" })).toBe(`the user memory section ${i("ترجیحات")}`)
})

test("conflictText names who changed memory", () => {
  const user = { table: "_jenab_user_memory", key: "preferences" }
  expect(conflictText(conflict({ item: user, source: "message:01J" as never }))).toBe(
    `the user memory section ${i("preferences")} changed since by a chat in another project`,
  )
  expect(conflictText(conflict({ item: user, source: "undo:user_memory:4" as never }))).toBe(
    `the user memory section ${i("preferences")} changed since by an undo`,
  )
})

// A Persian table name and key stay whole, each in its own isolate, so
// the English sentence around them keeps its order.
test("itemName isolates right-to-left names", () => {
  expect(itemName({ table: "قیمت‌ها", key: '["تهران",2]' })).toBe("row ⁨تهران⁩, ⁨2⁩ of ⁨قیمت‌ها⁩")
})

test("conflictText", () => {
  const row = `row ${i("2")} of ${i("prices")}`
  const order = `row ${i("7")} of ${i("orders")}`
  expect(conflictText(conflict({ source: "message:01M" as never }, "Other"))).toBe(`${row} changed since in the chat “${i("Other")}”`)
  expect(conflictText(conflict({ source: "user" as never }))).toBe(`${row} changed since by you`)
  expect(conflictText(conflict({ reason: ConflictReason.ConflictReferenced, reached: { table: "orders", key: "[7]" } }))).toBe(
    `${order} refers to ${row}`,
  )
  expect(conflictText(conflict({ reason: ConflictReason.ConflictCascade, reached: { table: "orders", key: "[7]" } }))).toBe(
    `undoing ${row} would also change ${order}`,
  )
  expect(conflictText(conflict({ reason: ConflictReason.ConflictSchema, item: { table: "prices", key: "" } }))).toBe(
    `a later migration changed the table ${i("prices")}`,
  )
})

test("skipAll adds each item once", () => {
  const cs = [conflict({}), conflict({}), conflict({ item: { table: "prices", key: "[3]" } })]
  expect(skipAll([{ table: "prices", key: "[3]" }], cs)).toEqual([
    { table: "prices", key: "[3]" },
    { table: "prices", key: "[2]" },
  ])
})

test("partly follows the side the last undo or redo left", () => {
  const plan = (changes: number, rows: TurnPlan["rows"], left = 0): TurnPlan => ({
    changes,
    rows,
    left,
    summary: "",
    display: "",
    past: false,
    entries: 0,
    too_big: false,
  })
  const state = (undo: TurnPlan, redo: TurnPlan): TurnUndoState => ({ turn: 1, undo, redo, migrations: 0 })
  const two = [{ table: "prices", rows: 2 }]
  const one = [{ table: "prices", rows: 1 }]
  const undone = partly(state(plan(1, two, 2), plan(2, one)))
  expect(undone).toEqual({ text: "partly undone: 2 items not undone", rest: "undo" })
  expect([isRest(undone, false), isRest(undone, true)]).toEqual([true, false])
  const redone = partly(state(plan(2, two), plan(1, one, 1)))
  expect(redone).toEqual({ text: "partly redone: 1 item not redone", rest: "redo" })
  expect([isRest(redone, false), isRest(redone, true)]).toEqual([false, true])
  // No side known: both ways are the rest.
  const crash = partly(state(plan(1, one), plan(1, one)))
  expect(crash).toEqual({ text: "partly undone", rest: "" })
  expect([isRest(crash, false), isRest(crash, true)]).toEqual([true, true])
  // Past the undo window the turn is history: no partly state.
  expect(partly(state({ ...plan(1, one), past: true }, { ...plan(1, one, 1), past: true }))).toBeNull()
  expect(partly(state(plan(1, two), plan(0, null)))).toBeNull()
  expect(partly(state(plan(0, null), plan(1, two)))).toBeNull()
  expect(partly(null)).toBeNull()
  expect(isRest(null, false)).toBe(false)
})
