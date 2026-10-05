import { expect, test } from "bun:test"

import { ChatKind, ChatState, type ChatItem, type ChatStatus } from "@/lib/api"
import { useChats, visible, withStatus, withWaiting } from "@/state/chats"

function item(id: string, over: Partial<ChatItem["chat"]> = {}, last?: string): ChatItem {
  return {
    chat: {
      id, kind: ChatKind.KindChat, title: id, title_fixed: false, role: "", skills: [], model: "default",
      default_page: "", created_by: "user" as never, created_at: "2026-10-01T10:00:00Z", archived: false, ...over,
    },
    state: ChatState.StateIdle,
    last_activity: last,
  }
}

const status = (chat: string, state: ChatState, waiting?: ChatStatus["waiting"]): ChatStatus =>
  ({ project: "p1", chat, seq: 1, state, tasks: 0, waiting }) as ChatStatus

const wait = { message: "m1", index: 0, kind: "question", text: "Which currency?" } as NonNullable<ChatStatus["waiting"]>

test("visible: Mother first, then newest, archived left out", () => {
  const list = visible([
    item("old", {}, "2026-10-02T10:00:00Z"),
    item("gone", { archived: true }),
    item("new", {}, "2026-10-04T10:00:00Z"),
    item("mother", { kind: ChatKind.KindMother }),
  ])
  expect(list.map((it) => it.chat.id)).toEqual(["mother", "new", "old"])
})

test("withStatus", () => {
  const items = [item("a"), item("b")]
  const next = withStatus(items, status("b", ChatState.StateWorking))
  expect(next[1].state).toBe(ChatState.StateWorking)
  expect(next[0]).toBe(items[0])
  expect(withStatus(next, status("b", ChatState.StateWorking))).toBe(next) // no change, same list
  expect(withStatus(items, status("x", ChatState.StateWorking))).toBe(items)
})

test("withWaiting", () => {
  let ws = withWaiting([], status("a", ChatState.StateWaiting, wait), "Prices")
  expect(ws).toEqual([{ project: "p1", chat: "a", title: "Prices", waiting: wait }])
  ws = withWaiting(ws, status("a", ChatState.StateWaiting, { ...wait, index: 1 }), "Prices") // the next card
  expect(ws.length).toBe(1)
  expect(ws[0].waiting.index).toBe(1)
  const same = withWaiting(ws, status("b", ChatState.StateIdle), "")
  expect(same).toBe(ws)
  expect(withWaiting(ws, status("a", ChatState.StateWorking), "Prices")).toEqual([])
})

test("status keeps the title for Waiting", () => {
  useChats.setState({ byProject: { p1: [item("a", { title: "Prices" })] }, waiting: [] })
  useChats.getState().status(status("a", ChatState.StateWaiting, wait))
  expect(useChats.getState().waiting[0].title).toBe("Prices")
  expect(useChats.getState().byProject.p1[0].state).toBe(ChatState.StateWaiting)
})
