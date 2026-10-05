import * as React from "react"

import { PartKind, type Delta, type Live as LiveTurn, type Part } from "@/lib/api"

// Stream is an answer still streaming (Status.Streaming): its finished
// parts and the text of the part being streamed. It lives outside React,
// and deltas are applied once per frame, so a fast model costs one render
// per frame (SPEC 5.8, SPIKE-022).
export interface Stream {
  message: string
  parts: Part[]
  index: number // the part being streamed
  kind: PartKind | ""
  text: string
}

// fromLive is the stream a snapshot found, or null when nothing streams.
export function fromLive(l: LiveTurn): Stream | null {
  if (!l.streaming) return null
  const parts = l.parts ?? []
  return { message: l.streaming, parts, index: parts.length, kind: l.kind ?? "", text: l.text ?? "" }
}

// addDelta applies one delta, as the Go side builds the answer: a higher
// part index finishes the part before, and a new kind at the same index
// starts its text again.
export function addDelta(s: Stream | null, d: Delta): Stream {
  if (!s || s.message !== d.message) s = { message: d.message, parts: [], index: d.part, kind: d.kind, text: "" }
  else s = { ...s }
  if (d.part > s.index) {
    if (s.text) s.parts = [...s.parts, textPart(s.kind, s.text)]
    s.index = d.part
    s.kind = d.kind
    s.text = ""
  } else if (d.kind !== s.kind) {
    s.kind = d.kind
    s.text = ""
  }
  s.text += d.text
  return s
}

function textPart(kind: PartKind | "", text: string): Part {
  return kind === PartKind.PartThinking
    ? { kind: PartKind.PartThinking, thinking: { text } }
    : { kind: PartKind.PartText, text: { text } }
}

// overlap is how many of chunks a snapshot's text already holds: deltas
// sent before the snapshot read the answer may arrive after it. It is the
// most chunks whose text ends the snapshot's text.
export function overlap(text: string, chunks: string[]): number {
  for (let n = chunks.length; n > 0; n--) {
    if (text.endsWith(chunks.slice(0, n).join(""))) return n
  }
  return 0
}

const streams = new Map<string, Stream | null>()
const pending = new Map<string, Delta[]>()
const listeners = new Map<string, Set<() => void>>()
let frame = 0

function notify(chat: string) {
  listeners.get(chat)?.forEach((f) => f())
}

function flush() {
  frame = 0
  for (const [chat, ds] of pending) {
    let s = streams.get(chat) ?? null
    for (const d of ds) s = addDelta(s, d)
    streams.set(chat, s)
    notify(chat)
  }
  pending.clear()
}

// push queues a delta for the next frame.
export function push(d: Delta) {
  const q = pending.get(d.chat)
  if (q) q.push(d)
  else pending.set(d.chat, [d])
  if (!frame) frame = requestAnimationFrame(flush)
}

// set replaces a chat's stream, as a snapshot has it.
export function set(chat: string, s: Stream | null) {
  pending.delete(chat)
  streams.set(chat, s)
  notify(chat)
}

// end drops the chat's stream when it is message, or any stream when
// message is empty: the answer was stored, or its try failed.
export function end(chat: string, message = "") {
  const s = streams.get(chat)
  if (!s || (message && s.message !== message)) return
  pending.delete(chat)
  streams.set(chat, null)
  notify(chat)
}

export function get(chat: string): Stream | null {
  return streams.get(chat) ?? null
}

// useStream is the chat's stream, rendered at most once per frame.
export function useStream(chat: string): Stream | null {
  const subscribe = React.useCallback(
    (f: () => void) => {
      let set = listeners.get(chat)
      if (!set) listeners.set(chat, (set = new Set()))
      set.add(f)
      return () => void set.delete(f)
    },
    [chat],
  )
  return React.useSyncExternalStore(subscribe, () => get(chat))
}
