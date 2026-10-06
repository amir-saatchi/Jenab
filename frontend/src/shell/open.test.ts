import { beforeEach, expect, test } from "bun:test"

import type { OpenedProject } from "@/lib/api"
import { useChats } from "@/state/chats"
import { current, useNav } from "@/state/nav"
import { useProjects } from "@/state/projects"
import { openProject } from "@/shell/open"

// open answers when let go, so two openings can overlap.
const answers: Record<string, () => void> = {}
const opened = (id: string) => ({ id, mother: `${id}-mother`, level: "standard" }) as OpenedProject

beforeEach(() => {
  useNav.setState({ history: { places: [], index: -1 }, mru: [], cycle: null, cycleFrom: null })
  useChats.setState({ byProject: { p1: [], p2: [] } } as never)
  useProjects.setState({
    open: (id: string) => new Promise<OpenedProject>((resolve) => (answers[id] = () => resolve(opened(id)))),
  })
})

const settle = () => Bun.sleep(1)

test("only the project asked for last opens", async () => {
  const first = openProject("p1")
  const second = openProject("p2")
  answers.p2()
  await second
  answers.p1() // the slower first answer comes back last
  await first
  expect(current(useNav.getState())).toEqual({ view: "chat", project: "p2", chat: "p2-mother" })
})

test("an opening doesn't move away from a place opened meanwhile", async () => {
  const p = openProject("p1")
  useNav.getState().go({ view: "settings" })
  answers.p1()
  await p
  await settle()
  expect(current(useNav.getState())).toEqual({ view: "settings" })
})

test("an opening with nothing in between moves", async () => {
  const p = openProject("p1", "c1")
  answers.p1()
  await p
  expect(current(useNav.getState())).toEqual({ view: "chat", project: "p1", chat: "c1" })
})
