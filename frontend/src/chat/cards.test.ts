import { expect, test } from "bun:test"
import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"

import { ApprovalCard } from "@/chat/cards"
import { Grant, type Approval } from "@/lib/api"

// The window card of desktop control (SPEC 8.10): one button per window
// that can be picked, then Deny; an answered one says what was picked.
const pick = (answer?: string): Approval =>
  ({
    id: "a1",
    kind: "desktop_window",
    target: "desktop",
    ask: "Pick a window for the agent to use",
    why: "to write your sentence",
    risk: "The agent can read the window",
    options: [
      { label: "Notes — notepad", grant: "pick:w-1" },
      { label: "Search — chrome", grant: "pick:w-2" },
      { label: "Deny", grant: Grant.GrantDeny },
    ],
    answer,
    answered_at: answer ? "2026-11-03T10:14:00Z" : undefined,
  }) as Approval

const render = (a: Approval) =>
  renderToStaticMarkup(createElement(ApprovalCard, { approval: a, project: "p", chat: "c", message: "m", index: 0, live: true }))

test("a pick card lists the windows and Deny", () => {
  const html = render(pick())
  expect(html).toContain("Other app&#x27;s window")
  expect(html).toContain("Notes — notepad")
  expect(html).toContain("Search — chrome")
  expect(html).toContain(">Deny<")
  expect(html).not.toContain("Once")
  expect(html.indexOf("Notes — notepad")).toBeLessThan(html.indexOf(">Deny<"))
})

test("an answered pick card says which window", () => {
  const html = render(pick("pick:w-2"))
  expect(html).toContain("Picked Search — chrome")
  expect(html).not.toContain("Notes — notepad")
})

// The agent named an app (SPEC 8.10): its windows come first.
const named = (ask: string, groups: string[]): Approval =>
  ({
    ...pick(),
    ask,
    options: [
      ...groups.map((g, i) => ({ label: `Window ${i + 1} — app${i + 1}`, grant: `pick:w-${i + 1}`, group: g })),
      { label: "Deny", grant: Grant.GrantDeny },
    ],
  }) as Approval

test("one window of the app: Allow, Deny and Show all windows", () => {
  const html = render(named("Let the agent use Window 1 — app1?", ["match", "other", "other"]))
  expect(html).toContain("Let the agent use Window 1 — app1?")
  expect(html).toContain(">Allow<")
  expect(html).toContain(">Deny<")
  expect(html).toContain("Show all windows")
  expect(html).toContain("Note for the agent")
  // No window list: the other windows wait behind Show all windows.
  expect(html).not.toContain("Window 2 — app2")
  expect(html).not.toContain("Other windows")
  expect(html.indexOf(">Allow<")).toBeLessThan(html.indexOf(">Deny<"))
})

test("several windows of the app: the rest fold under Other windows", () => {
  const html = render(named("Pick a window of app for the agent to use", ["match", "match", "other"]))
  expect(html).toContain("Window 1 — app1")
  expect(html).toContain("Window 2 — app2")
  expect(html).toContain("Other windows (1)")
  expect(html).not.toContain("Window 3 — app3")
  expect(html).not.toContain(">Allow<")
  expect(html).not.toContain("Show all windows")
  // The list scrolls inside the card.
  expect(html).toContain("overflow-y-auto")
})

test("no window of the app: the card says so and lists the rest", () => {
  const html = render(named("No window of calc is open: pick another for the agent to use", ["other", "other"]))
  expect(html).toContain("No window of calc is open")
  expect(html).toContain("Window 1 — app1")
  expect(html).toContain("Window 2 — app2")
  expect(html).not.toContain("Other windows")
})
