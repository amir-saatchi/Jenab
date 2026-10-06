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
import { showError, toUIError } from "@/lib/errors"
import * as stream from "@/state/stream"

// Thread is an open chat's messages and turn state (Q32). Events with a
// sequence number up to snapSeq are already in messages; a write that skips
// a number means one was missed, and the chat is read again. The turn state
// was read before the messages, at liveSeq: a status carries the whole
// state, so one from liveSeq on is newer than the snapshot's.
export interface Thread {
  project: string
  chat: Chat
  messages: Message[]
  from: number // the first turn loaded; 1 means the whole chat
  seq: number // the last write applied
  snapSeq: number
  liveSeq: number
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
    liveSeq: live.seq,
    state: live.waiting ? ChatState.StateWaiting : live.running ? ChatState.StateWorking : ChatState.StateIdle,
    waiting: live.waiting ?? null,
    retry: live.retry ?? null,
    streaming: live.streaming ?? "",
    streamed: live.streaming ? [live.streaming] : [],
    joining: live.running ? [...new Set([...(prev?.joining ?? []), ...joiningIn(messages, live)])] : [],
  }
}

// joiningIn is the user messages of the running turn sent after its last
// answer: they join at its next request, so they show below the answer
// still streaming, as the store will sort them once it is saved.
function joiningIn(messages: Message[], live: ChatSnapshot["live"]): string[] {
  const turn = messages.filter((m) => m.turn === live.turn)
  let after = live.streaming ?? ""
  for (const m of turn) if (m.role === Role.RoleAssistant && m.id > after) after = m.id
  return turn
    .slice(1)
    .filter((m) => m.role === Role.RoleUser && m.id > after && (m.parts ?? []).some((p) => p.kind !== PartKind.PartNotice))
    .map((m) => m.id)
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

// turn is a new message's turn, as PartDone gives it. Without one it is
// guessed: a message the user sends while the chat is idle starts the next
// turn, and anything else belongs to the last one.
function turn(t: Thread, e: PartDone, r: Role): number {
  if (e.turn > 0) return e.turn
  const last = t.messages[t.messages.length - 1]?.turn ?? 0
  if (r === Role.RoleUser && e.part.kind !== PartKind.PartNotice && t.state === ChatState.StateIdle) return last + 1
  return Math.max(last, 1)
}

// sortsBefore tells whether message a comes before b, as the store sorts
// them: by turn, then by ID, and IDs grow in the order they were made. A
// message sent during a turn sorts after the answer and results that were
// being made when it came.
function sortsBefore(a: Message, b: Message) {
  return a.turn !== b.turn ? a.turn < b.turn : a.id < b.id
}

// withPart applies a finished part: it replaces the part at its index or
// adds it, in a new message if needed, at its place in the store's order.
// It returns null on a gap.
export function withPart(t: Thread, e: PartDone): Thread | null {
  if (e.seq <= t.snapSeq) return t
  if (e.seq > t.seq + 1) return null
  const messages = [...t.messages]
  let i = messages.findLastIndex((m) => m.id === e.message)
  if (i < 0) {
    const r = role(t, e)
    const m: Message = {
      id: e.message,
      chat: t.chat.id,
      turn: turn(t, e, r),
      role: r,
      usage: { input: 0, output: 0, cache_read: 0, cache_write: 0 },
      created_at: new Date().toISOString(),
      parts: [],
    }
    i = messages.length
    while (i > 0 && sortsBefore(m, messages[i - 1])) i--
    messages.splice(i, 0, m)
  }
  const parts = [...(messages[i].parts ?? [])]
  parts[e.index] = e.part
  messages[i] = { ...messages[i], parts }
  return { ...t, messages, seq: Math.max(t.seq, e.seq) }
}

// withStatus applies a chat:status event. A new try takes the messages
// written during the turn, so they no longer join.
export function withStatus(t: Thread, s: ChatStatus): Thread {
  if (s.seq < t.liveSeq) return t
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

export type Event = { delta: Delta } | { part: PartDone } | { status: ChatStatus }

// fresh is the events that came while a snapshot was read, less the text
// its stream already holds: a delta's offset says where its text starts in
// the part, so what lies within the snapshot's text is dropped. Parts and
// statuses the snapshot has are dropped by withPart and withStatus.
export function fresh(queue: Event[], live: stream.Stream | null): Event[] {
  if (!live) return queue
  const held = live.text.length
  const out: Event[] = []
  for (const e of queue) {
    if (!("delta" in e) || e.delta.message !== live.message || e.delta.part !== live.index || e.delta.kind !== live.kind) {
      out.push(e)
      continue
    }
    const d = e.delta
    if (d.offset + d.text.length <= held) continue
    out.push(d.offset >= held ? e : { delta: { ...d, text: d.text.slice(held - d.offset), offset: held } })
  }
  return out
}

interface ThreadsState {
  threads: Record<string, Thread>
  failed: Record<string, string> // chats whose first read failed, with the error
  open(project: string, chat: string): Promise<void>
  older(chat: string): Promise<void>
  joined(chat: string, message: string): void
  setChat(c: Chat): void
  delta(d: Delta): void
  part(e: PartDone): void
  status(s: ChatStatus): void
  forget(chat: string, deleted?: boolean): void
  gone(project: string, chats: string[]): void
}

// Reads in flight, by chat: events that come meanwhile wait in queue.
const reading = new Map<string, { queue: Event[]; again: boolean }>()
const olderLoading = new Set<string>()

// Turns read per scroll up.
const OLDER_TURNS = 10

// Waits before a failed first read is tried again, in ms.
const RETRIES = [500, 2000]

// Threads kept: the chats opened last. Only the one opened last is shown.
const KEPT = 20
let recent: string[] = []

// drafts are the composer's text per chat, kept while another chat is open.
export const drafts = new Map<string, string>()

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))

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
        const live = stream.fromLive(snap.live)
        set((s) => ({ threads: { ...s.threads, [chat]: fromSnapshot(s.threads[chat], project, snap) } }))
        stream.set(chat, live)
        for (const e of fresh(run.queue.splice(0), live)) apply(e)
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
    failed: {},
    // open reads a chat the first time it is shown, trying again a few
    // times; threads of chats not opened lately are dropped.
    open: async (project, chat) => {
      recent = [...recent.filter((c) => c !== chat), chat]
      const old = recent.slice(0, -KEPT)
      recent = recent.slice(-KEPT)
      for (const c of old) get().forget(c)
      if (get().threads[chat]) return
      set((s) => (s.failed[chat] ? { failed: omit(s.failed, chat) } : s))
      for (let i = 0; ; i++) {
        try {
          return await read(project, chat)
        } catch (err) {
          if (i >= RETRIES.length) {
            set((s) => ({ failed: { ...s.failed, [chat]: toUIError(err).message } }))
            throw err
          }
          await sleep(RETRIES[i])
          if (get().threads[chat]) return
        }
      }
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
    // joined marks a message sent during a turn. Send returns after the
    // events it caused, so the turn may have ended, or a new try taken the
    // message, by then: message IDs grow, and a try's answer made after the
    // message took it.
    joined: (chat, message) =>
      set((s) => {
        const t = s.threads[chat]
        if (!t || t.state === ChatState.StateIdle || t.joining.includes(message) || t.streamed.some((m) => m > message)) return s
        return { threads: { ...s.threads, [chat]: { ...t, joining: [...t.joining, message] } } }
      }),
    setChat: (c) =>
      set((s) => (s.threads[c.id] ? { threads: { ...s.threads, [c.id]: { ...s.threads[c.id], chat: c } } } : s)),
    delta: (d) => event(d.chat, { delta: d }),
    part: (e) => event(e.chat, { part: e }),
    status: (st) => event(st.chat, { status: st }),
    // forget drops a chat's thread and stream; a deleted chat's draft too.
    forget: (chat, deleted = false) => {
      stream.drop(chat)
      if (deleted) {
        drafts.delete(chat)
        recent = recent.filter((c) => c !== chat)
      }
      set((s) => (s.threads[chat] || s.failed[chat] ? { threads: omit(s.threads, chat), failed: omit(s.failed, chat) } : s))
    },
    // gone forgets the project's chats a new chat list no longer has, but
    // the one shown, which the list may predate.
    gone: (project, chats) => {
      const keep = new Set(chats)
      const shown = recent[recent.length - 1]
      for (const [id, t] of Object.entries(get().threads)) {
        if (t.project === project && !keep.has(id) && id !== shown) get().forget(id, true)
      }
    },
  }
})

function omit<T>(r: Record<string, T>, key: string): Record<string, T> {
  const { [key]: _, ...rest } = r
  return rest
}
