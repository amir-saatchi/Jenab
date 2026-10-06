import { beforeEach, expect, test } from "bun:test"

import { current, push, samePlace, step, used, useNav, type Place } from "@/state/nav"

const chat = (c: string, project = "p1"): Place => ({ view: "chat", project, chat: c })

beforeEach(() => useNav.setState({ history: { places: [], index: -1 }, mru: [], cycle: null, cycleFrom: null }))

test("push and step", () => {
  let h = push({ places: [], index: -1 }, chat("a"))
  h = push(h, chat("a")) // the same place again is no step
  h = push(h, chat("b"))
  expect(h.places.length).toBe(2)
  h = step(h, -1)
  expect(h.places[h.index]).toEqual(chat("a"))
  expect(step(h, -1)).toBe(h) // nothing further back
  h = push(h, { view: "settings" }) // drops the forward places
  expect(h.places).toEqual([chat("a"), { view: "settings" }])
  expect(samePlace({ view: "project-settings", project: "p1" }, { view: "project-settings", project: "p2" })).toBe(false)
})

test("used", () => {
  const m = used(used(used([], { project: "p1", chat: "a" }), { project: "p1", chat: "b" }), { project: "p1", chat: "a" })
  expect(m).toEqual([
    { project: "p1", chat: "a" },
    { project: "p1", chat: "b" },
  ])
})

test("Ctrl+Tab walks back through the chats used last", () => {
  const nav = useNav.getState
  for (const c of ["a", "b", "c"]) nav().go(chat(c))
  nav().nextChat(1) // Ctrl+Tab
  expect(current(nav())).toEqual(chat("b"))
  nav().nextChat(1) // Tab again while holding Ctrl
  expect(current(nav())).toEqual(chat("a"))
  nav().endCycle() // Ctrl let go
  expect(nav().mru.map((m) => m.chat)).toEqual(["a", "c", "b"])
  // One cycle is one step in the history, whatever it passed on the way.
  expect(nav().history.places).toEqual([chat("a"), chat("b"), chat("c"), chat("a")])
  nav().back()
  expect(current(nav())).toEqual(chat("c"))
  nav().forward()
  nav().nextChat(1)
  expect(current(nav())).toEqual(chat("c"))
  nav().endCycle()
  nav().nextChat(-1) // Ctrl+Shift+Tab goes the other way round
  expect(current(nav())).toEqual(chat("b"))
})

test("back and forward count as use", () => {
  const nav = useNav.getState
  nav().go(chat("a"))
  nav().go(chat("b"))
  nav().go({ view: "settings" })
  nav().back()
  nav().back()
  expect(current(nav())).toEqual(chat("a"))
  expect(nav().mru[0].chat).toBe("a")
  nav().forward()
  expect(current(nav())).toEqual(chat("b"))
})

test("a turn in the inspector and a section of Settings are places of their own", () => {
  const at = (turn: number) => ({ view: "inspector" as const, project: "p", chat: "c", turn })
  expect(samePlace(at(1), at(1))).toBe(true)
  expect(samePlace(at(1), at(2))).toBe(false)
  expect(samePlace({ view: "settings" }, { view: "settings", section: "Models" })).toBe(false)
  expect(samePlace({ view: "runtime" }, { view: "runtime" })).toBe(true)
})

test("Ctrl+Tab away from the chat used last opens that chat first", () => {
  const nav = useNav.getState
  for (const c of ["a", "b"]) nav().go(chat(c))
  nav().go({ view: "settings" })
  nav().nextChat(1)
  expect(current(nav())).toEqual(chat("b"))
  nav().nextChat(1)
  expect(current(nav())).toEqual(chat("a"))
  nav().endCycle()
  expect(nav().history.places).toEqual([chat("a"), chat("b"), { view: "settings" }, chat("a")])
  nav().go({ view: "runtime" })
  nav().nextChat(-1) // the other way round starts at the far end
  expect(current(nav())).toEqual(chat("b"))
  nav().endCycle()
})

test("Ctrl+Tab round to where it started adds no step", () => {
  const nav = useNav.getState
  for (const c of ["a", "b"]) nav().go(chat(c))
  nav().nextChat(1)
  nav().nextChat(1)
  expect(current(nav())).toEqual(chat("b"))
  nav().endCycle()
  expect(nav().history.places).toEqual([chat("a"), chat("b")])
})

test("one chat: Ctrl+Tab opens it from another place, and does nothing on it", () => {
  const nav = useNav.getState
  nav().go(chat("a"))
  nav().nextChat(1)
  expect(nav().cycle).toBeNull()
  nav().go({ view: "settings" })
  nav().nextChat(1)
  expect(current(nav())).toEqual(chat("a"))
  nav().endCycle()
})
