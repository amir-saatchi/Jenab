import { expect, test } from "bun:test"

import { MoveGuard, pickView, pickWaiting } from "@/chat/pick"
import type { Approval, Message, Waiting } from "@/lib/api"

const opt = (n: number, group?: string) => ({ label: `W${n}`, grant: `pick:w-${n}`, group })
const deny = { label: "Deny", grant: "deny" }

test("pickView: no app named is one plain list", () => {
  const v = pickView([opt(1), opt(2), deny] as NonNullable<Approval["options"]>, false)
  expect(v).toEqual({ kind: "list", first: [opt(1), opt(2)], other: [], folded: false } as never)
})

test("pickView: one window of the app, then Show all windows", () => {
  const options = [opt(1, "match"), opt(2, "other"), deny] as NonNullable<Approval["options"]>
  expect(pickView(options, false)).toEqual({ kind: "one", window: opt(1, "match") } as never)
  // Show all windows opens the full list in the same card, unfolded.
  expect(pickView(options, true)).toEqual({ kind: "list", first: [opt(1, "match")], other: [opt(2, "other")], folded: false } as never)
})

test("pickView: several windows fold the rest; none show the rest open", () => {
  const several = pickView([opt(1, "match"), opt(2, "match"), opt(3, "other")] as NonNullable<Approval["options"]>, false)
  expect(several).toMatchObject({ kind: "list", folded: true })
  const none = pickView([opt(1, "other"), opt(2, "other")] as NonNullable<Approval["options"]>, false)
  expect(none).toMatchObject({ kind: "list", first: [], folded: false })
  expect(pickView([deny] as NonNullable<Approval["options"]>, false)).toBeNull()
})

const msg = (approval: Partial<Approval>): Message[] =>
  [{ id: "m", parts: [{ kind: "approval", approval: { kind: "desktop_window", ...approval } }] }] as unknown as Message[]
const wait = { message: "m", index: 0 } as Waiting

test("pickWaiting: only an open window card stops auto-scroll", () => {
  expect(pickWaiting(msg({}), wait)).toBe(true)
  expect(pickWaiting(msg({}), null)).toBe(false)
  expect(pickWaiting(msg({ answer: "pick:w-1" } as Partial<Approval>), wait)).toBe(false)
  expect(pickWaiting(msg({ stopped: true }), wait)).toBe(false)
  expect(pickWaiting(msg({ kind: "desktop_key" }), wait)).toBe(false)
})

test("MoveGuard: a click right after the card moves is ignored", () => {
  const g = new MoveGuard(500)
  g.seen(10, 100, 0) // the card appears
  expect(g.steady(200)).toBe(false)
  expect(g.steady(500)).toBe(true)
  g.seen(10, 100, 900) // same place: still steady
  expect(g.steady(900)).toBe(true)
  g.seen(10, 40, 1000) // content above pushed it up
  expect(g.steady(1300)).toBe(false)
  expect(g.steady(1500)).toBe(true)
  g.moved(2000) // it resized in place
  expect(g.steady(2100)).toBe(false)
})
