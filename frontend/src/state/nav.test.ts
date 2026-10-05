import { beforeEach, expect, test } from "bun:test"

import { current, push, samePlace, step, used, useNav, type Place } from "@/state/nav"

const chat = (c: string, project = "p1"): Place => ({ view: "chat", project, chat: c })

beforeEach(() => useNav.setState({ history: { places: [], index: -1 }, mru: [], cycle: null }))

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
