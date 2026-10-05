import type { TurnItem } from "@/lib/api"

const count = new Intl.NumberFormat("en")

// num is a whole number with group separators: 7,412.
export function num(n: number) {
  return count.format(n)
}

// duration is a time span in its natural unit: 38 ms, 1.2 s, 3 min 4 s.
export function duration(ms: number) {
  if (ms < 1000) return `${Math.max(0, Math.round(ms))} ms`
  if (ms < 60_000) return `${trim((ms / 1000).toFixed(1))} s`
  const s = Math.round(ms / 1000)
  return s % 60 ? `${Math.floor(s / 60)} min ${s % 60} s` : `${s / 60} min`
}

// size is a byte count: 312 B, 0.2 KB at 200 bytes and up, 1.5 MB.
export function size(bytes: number) {
  if (bytes < 200) return `${bytes} B`
  if (bytes < 1_000_000) return `${trim((bytes / 1000).toFixed(1))} KB`
  return `${trim((bytes / 1_000_000).toFixed(1))} MB`
}

function trim(s: string) {
  return s.endsWith(".0") ? s.slice(0, -2) : s
}

// Scale is the timeline's span and its ticks: at most six, at 1, 2 or 5
// times a power of ten milliseconds.
export interface Scale {
  span: number
  step: number
  ticks: number[]
}

export function scale(total: number): Scale {
  const t = Math.max(total, 1)
  let step = 1
  for (let p = 1; ; p *= 10) {
    const s = [p, 2 * p, 5 * p].find((s) => t / s <= 5)
    if (s) {
      step = s
      break
    }
  }
  const span = Math.ceil(t / step) * step
  const ticks = Array.from({ length: span / step + 1 }, (_, i) => i * step)
  return { span, step, ticks }
}

export function tick(ms: number, step: number) {
  return step >= 1000 ? `${ms / 1000} s` : `${ms} ms`
}

// ChatOption is a chat with recorded turns, for the inspector's picker.
export interface ChatOption {
  key: string
  project: string
  chat: string
  label: string
}

export function chatKey(project: string, chat: string) {
  return `${project}/${chat}`
}

// chatOptions lists the chats of the turns, the most recent first.
export function chatOptions(turns: TurnItem[]): ChatOption[] {
  const out: ChatOption[] = []
  for (const t of turns) {
    const key = chatKey(t.project, t.chat)
    if (out.some((o) => o.key === key)) continue
    const project = t.project_name || "Closed project"
    out.push({ key, project: t.project, chat: t.chat, label: `${project} · ${t.chat_title || "Untitled chat"}` })
  }
  return out
}

const partNames: Record<string, string> = {
  text: "answer",
  thinking: "thinking",
  tool_call: "tool call",
  image: "image",
}

// parts names what a request got back, once each: "thinking · answer".
export function parts(kinds: string[]) {
  return [...new Set(kinds.map((k) => partNames[k] ?? k.replace(/_/g, " ")))].join(" · ")
}

// share is the part of a prompt read from the cache: "6,120 cached (83 %)".
export function share(cached: number, prompt: number) {
  if (prompt <= 0) return ""
  return `${num(cached)} cached (${Math.round((cached / prompt) * 100)} %)`
}
