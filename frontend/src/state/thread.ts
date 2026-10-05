import { create } from "zustand"

import {
  ChatService,
  ChatState,
  PartKind,
  Role,
  type Chat,
  type ChatSnapshot,
  type ChatStatus,
  type Delta,
  type Message,
  type PartDone,
  type Retry,
  type Waiting,
} from "@/lib/api"
import { showError } from "@/lib/errors"
import * as stream from "@/state/stream"

// Thread is an open chat's messages and turn state (Q32). Events with a
// sequence number up to snapSeq are already in messages; a write that skips
// a number means one was missed, and the chat is read again.
export interface Thread {
  project: string
  chat: Chat
  messages: Message[]
  from: number // the first turn loaded; 1 means the whole chat
  seq: number // the last write applied
  snapSeq: number
  state: ChatState
  waiting: Waiting | null
  retry: Retry | null
  streaming: string // the answer streaming now, not yet stored
  streamed: string[] // answers streamed since the chat was read: the assistant's
  joining: string[] // sent during a turn; they join at its next request
}

// share keeps the old object of every message that didn't change, so
// reading a chat again re-renders only what is new.
export function share(old: Message[], next: Message[]): Message[] {
  const byID = new Map(old.map((m) => [m.id, m]))
  return next.map((m) => {
    const o = byID.get(m.id)
    return o && JSON.stringify(o) === JSON.stringify(m) ? o : m
  })
}

// fromSnapshot is the thread a snapshot gives. Older turns loaded before
// are kept.
export function fromSnapshot(prev: Thread | undefined, project: string, snap: ChatSnapshot): Thread {
  const messages = snap.messages ?? []
  const older = prev ? prev.messages.filter((m) => m.turn < snap.from) : []
  const live = snap.live
  return {
    project,
    chat: snap.chat,
    messages: [...older, ...share(prev?.messages ?? [], messages)],
    from: prev && older.length ? prev.from : snap.from,
    seq: snap.seq,
    snapSeq: snap.seq,
    state: live.waiting ? ChatState.StateWaiting : live.running ? ChatState.StateWorking : ChatState.StateIdle,
    waiting: live.waiting ?? null,
    retry: live.retry ?? null,
    streaming: live.streaming ?? "",
    streamed: live.streaming ? [live.streaming] : [],
    joining: live.running ? (prev?.joining ?? []) : [],
  }
}

// role is a new message's role, guessed from its first part until the chat
// is read again: answers were streamed, tool messages hold results and
// cards, and the app's notices go in user messages.
function role(t: Thread, e: PartDone): Role {
  if (t.streamed.includes(e.message)) return Role.RoleAssistant
  switch (e.part.kind) {
    case PartKind.PartToolResult:
    case PartKind.PartApproval:
    case PartKind.PartQuestion:
      return Role.RoleTool
    case PartKind.PartThinking:
    case PartKind.PartToolCall:
      return Role.RoleAssistant
  }
  return Role.RoleUser
}

// withPart applies a finished part: it replaces the part at its index or
// adds it, in a new message if needed. It returns null on a gap.
export function withPart(t: Thread, e: PartDone): Thread | null {
  if (e.seq <= t.snapSeq) return t
  if (e.seq > t.seq + 1) return null
  const messages = [...t.messages]
  let i = messages.findLastIndex((m) => m.id === e.message)
  if (i < 0) {
    const last = messages[messages.length - 1]
    messages.push({
      id: e.message,
      chat: t.chat.id,
      turn: last?.turn ?? 1,
      role: role(t, e),
      usage: { input: 0, output: 0, cache_read: 0, cache_write: 0 },
      created_at: new Date().toISOString(),
      parts: [],
    })
    i = messages.length - 1
  }
  const parts = [...(messages[i].parts ?? [])]
  parts[e.index] = e.part
  messages[i] = { ...messages[i], parts }
  return { ...t, messages, seq: Math.max(t.seq, e.seq) }
}

// withStatus applies a chat:status event. A new try takes the messages
// written during the turn, so they no longer join.
export function withStatus(t: Thread, s: ChatStatus): Thread {
  if (s.seq < t.snapSeq) return t
  const streaming = s.streaming ?? ""
  const started = streaming !== "" && streaming !== t.streaming
  return {
    ...t,
    state: s.state,
    waiting: s.waiting ?? null,
    retry: s.retry ?? null,
    streaming,
    streamed: started ? [...t.streamed, streaming] : t.streamed,
    joining: started || s.state === ChatState.StateIdle ? [] : t.joining,
  }
}

type Event = { delta: Delta } | { part: PartDone } | { status: ChatStatus }

interface ThreadsState {
  threads: Record<string, Thread>
  open(project: string, chat: string): Promise<void>
  older(chat: string): Promise<void>
  joined(chat: string, message: string): void
  setChat(c: Chat): void
  delta(d: Delta): void
  part(e: PartDone): void
  status(s: ChatStatus): void
}

// Reads in flight, by chat: events that come meanwhile wait in queue.
const reading = new Map<string, { queue: Event[]; again: boolean }>()
const olderLoading = new Set<string>()

// Turns read per scroll up.
const OLDER_TURNS = 10

export const useThreads = create<ThreadsState>((set, get) => {
  // read reads the chat again and replays the events that came meanwhile.
  async function read(project: string, chat: string) {
    const r = reading.get(chat)
    if (r) {
      r.again = true
      return
    }
    const run = { queue: [] as Event[], again: false }
    reading.set(chat, run)
    try {
      do {
        run.again = false
        const snap = await ChatService.Snapshot(project, chat)
        set((s) => ({ threads: { ...s.threads, [chat]: fromSnapshot(s.threads[chat], project, snap) } }))
        const live = stream.fromLive(snap.live)
        const queue = run.queue.splice(0)
        // Deltas for the part the snapshot has may already be in its text.
        const same = queue.flatMap((e) =>
          "delta" in e && live && e.delta.message === live.message && e.delta.part === live.index ? [e.delta.text] : [],
        )
        let skip = live ? stream.overlap(live.text, same) : 0
        stream.set(chat, live)
        for (const e of queue) {
          if ("delta" in e && live && e.delta.message === live.message && e.delta.part === live.index && skip > 0) {
            skip--
            continue
          }
          apply(e)
        }
      } while (run.again)
    } finally {
      reading.delete(chat)
    }
  }

  function apply(e: Event) {
    if ("delta" in e) {
      const t = get().threads[e.delta.chat]
      if (t && t.streaming === e.delta.message) stream.push(e.delta)
      return
    }
    if ("part" in e) {
      const t = get().threads[e.part.chat]
      if (!t) return
      const next = withPart(t, e.part)
      if (!next) return void read(t.project, t.chat.id).catch(showError)
      stream.end(e.part.chat, e.part.message)
      set((s) => ({ threads: { ...s.threads, [e.part.chat]: next } }))
      return
    }
    const t = get().threads[e.status.chat]
    if (!t) return
    const next = withStatus(t, e.status)
    if (next === t) return
    if (!next.streaming) stream.end(t.chat.id)
    set((s) => ({ threads: { ...s.threads, [t.chat.id]: next } }))
    // The turn's end: read the stored messages, with their turn, model and
    // time.
    if (t.state !== ChatState.StateIdle && next.state === ChatState.StateIdle) read(t.project, t.chat.id).catch(showError)
  }

  function event(chat: string, e: Event) {
    const r = reading.get(chat)
    if (r) r.queue.push(e)
    else apply(e)
  }

  return {
    threads: {},
    open: async (project, chat) => {
      if (get().threads[chat]) return
      await read(project, chat)
    },
    older: async (chat) => {
      const t = get().threads[chat]
      if (!t || t.from <= 1 || olderLoading.has(chat)) return
      olderLoading.add(chat)
      try {
        const from = Math.max(1, t.from - OLDER_TURNS)
        const ms = (await ChatService.Messages(t.project, chat, from, t.from - 1)) ?? []
        set((s) => {
          const cur = s.threads[chat]
          if (!cur || cur.from !== t.from) return s
          return { threads: { ...s.threads, [chat]: { ...cur, messages: [...ms, ...cur.messages], from } } }
        })
      } finally {
        olderLoading.delete(chat)
      }
    },
    // joined marks a message sent during a turn.
    joined: (chat, message) =>
      set((s) => {
        const t = s.threads[chat]
        return t ? { threads: { ...s.threads, [chat]: { ...t, joining: [...t.joining, message] } } } : s
      }),
    setChat: (c) =>
      set((s) => (s.threads[c.id] ? { threads: { ...s.threads, [c.id]: { ...s.threads[c.id], chat: c } } } : s)),
    delta: (d) => event(d.chat, { delta: d }),
    part: (e) => event(e.chat, { part: e }),
    status: (st) => event(st.chat, { status: st }),
  }
})
