import { create } from "zustand"

import { ChatKind, ChatService, ChatState, type ChatItem, type ChatStatus, type WaitingItem } from "@/lib/api"
import { useThreads } from "@/state/thread"

// withStatus applies a chat:status event to a project's chat list. Events
// for chats not in the list are dropped: the next List has them.
export function withStatus(items: ChatItem[], s: ChatStatus): ChatItem[] {
  let changed = false
  const next = items.map((it) => {
    if (it.chat.id !== s.chat || it.state === s.state) return it
    changed = true
    return { ...it, state: s.state }
  })
  return changed ? next : items
}

// withWaiting applies a chat:status event to the Waiting list of every
// project: a wait is added or replaced, and removed once the chat moves on.
export function withWaiting(ws: WaitingItem[], s: ChatStatus, title: string): WaitingItem[] {
  const rest = ws.filter((w) => w.chat !== s.chat || w.project !== s.project)
  if (s.state !== ChatState.StateWaiting || !s.waiting) return rest.length === ws.length ? ws : rest
  return [...rest, { project: s.project, chat: s.chat, title, waiting: s.waiting }]
}

// visible is the chat list as shown: Mother first, then the others by
// their last activity, newest first; archived chats are left out.
export function visible(items: ChatItem[]): ChatItem[] {
  const time = (it: ChatItem) => (it.last_activity ? Date.parse(it.last_activity) : Date.parse(it.chat.created_at))
  return items
    .filter((it) => !it.chat.archived)
    .sort((a, b) => {
      const ma = a.chat.kind === ChatKind.KindMother, mb = b.chat.kind === ChatKind.KindMother
      if (ma !== mb) return ma ? -1 : 1
      return time(b) - time(a)
    })
}

interface ChatsState {
  byProject: Record<string, ChatItem[]>
  waiting: WaitingItem[]
  load(project: string): Promise<void>
  loadWaiting(): Promise<void>
  create(project: string): Promise<string>
  status(s: ChatStatus): void
}

export const useChats = create<ChatsState>((set, get) => ({
  byProject: {},
  waiting: [],
  load: async (project) => {
    const items = (await ChatService.List(project)) ?? []
    set((s) => ({ byProject: { ...s.byProject, [project]: items } }))
    // A chat no longer listed was deleted: its thread goes too.
    useThreads.getState().gone(project, items.map((it) => it.chat.id))
  },
  loadWaiting: async () => {
    set({ waiting: (await ChatService.Waiting()) ?? [] })
  },
  create: async (project) => {
    const c = await ChatService.Create(project, "", "")
    await get().load(project)
    return c.id
  },
  status: (st) =>
    set((s) => {
      const items = s.byProject[st.project]
      const title =
        items?.find((it) => it.chat.id === st.chat)?.chat.title ??
        s.waiting.find((w) => w.chat === st.chat)?.title ??
        ""
      return {
        byProject: items ? { ...s.byProject, [st.project]: withStatus(items, st) } : s.byProject,
        waiting: withWaiting(s.waiting, st, title),
      }
    }),
}))
