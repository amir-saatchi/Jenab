import { expect, test } from "bun:test"

import { ConflictReason, type TurnConflict, type UndoConflict } from "@/lib/api"
import { conflictText, isolate as i, itemName, skipAll } from "@/chat/undo-text"

function conflict(over: Partial<UndoConflict>, chat_title = ""): TurnConflict {
  return { conflict: { item: { table: "prices", key: "[2]" }, reason: ConflictReason.ConflictChanged, ...over }, chat_title }
}

test("itemName", () => {
  expect(itemName({ table: "prices", key: "[2]" })).toBe(`row ${i("2")} of ${i("prices")}`)
  expect(itemName({ table: "pairs", key: '[1,"BTC"]' })).toBe(`row ${i("1")}, ${i("BTC")} of ${i("pairs")}`)
  expect(itemName({ table: "_jenab_objects", key: "object:reports/a.txt" })).toBe(`the file ${i("reports/a.txt")}`)
  expect(itemName({ table: "_jenab_chat_role", key: "01CHAT" })).toBe("the chat's role")
  expect(itemName({ table: "prices", key: "" })).toBe(`the table ${i("prices")}`)
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
