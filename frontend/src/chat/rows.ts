import { NoticeKind, PartKind, Role, type Message, type Part, type ToolResult } from "@/lib/api"

// Row is one item of the thread: a user message, the agent's work between
// two user messages, or a note from the app.
export type Row =
  | { kind: "user"; id: string; message: Message }
  | { kind: "agent"; id: string; messages: Message[]; last: boolean }
  | { kind: "notice"; id: string; message: Message; part: Part }

// aside are the notices added to a turn after it ended (agent.aside), so
// they don't say how the turn ended.
const aside: (NoticeKind | undefined)[] = [NoticeKind.NoticeApprovalLevel, NoticeKind.NoticeTurnUndone, NoticeKind.NoticeMemoryRefreshed]

// isNotice tells whether a user message is a note from the app.
function isNotice(m: Message) {
  return m.role === Role.RoleUser && (m.parts ?? []).every((p) => p.kind === PartKind.PartNotice)
}

// refreshCard tells whether a message is Refresh memory's card (SPEC
// 3.5). It is a user message, so the model never reads it, but it shows
// with the agent's work, as cards do; and like an aside notice it doesn't
// say how the turn ended (agent.refreshCard).
export function refreshCard(m: Message) {
  const ps = m.parts ?? []
  return ps.length > 0 && ps.every((p) => p.approval?.target.startsWith("refresh:"))
}

// rows groups messages for the thread. Assistant and tool messages in a row
// are one agent item; last marks the one that ends a turn, which gets the
// turn's footer. Turns of messages not yet read from the store are a guess
// until the turn ends. Notices added after a turn ended, such as an
// undo's, sit in the chat's last turn but don't take its footer away.
export function rows(messages: Message[]): Row[] {
  const out: Row[] = []
  let group: Message[] | null = null
  let i = 0
  const close = (next?: Message) => {
    if (next && isAside(next)) next = messages.slice(i + 1).find((m) => !isAside(m))
    if (group) out.push({ kind: "agent", id: group[0].id, messages: group, last: !next || next.turn !== group[group.length - 1].turn })
    group = null
  }
  for (; i < messages.length; i++) {
    const m = messages[i]
    if (m.role !== Role.RoleUser || refreshCard(m)) {
      if (group) group.push(m)
      else group = [m]
      continue
    }
    if (isNotice(m)) {
      close(m)
      ;(m.parts ?? []).forEach((p, i) => out.push({ kind: "notice", id: `${m.id}:${i}`, message: m, part: p }))
      continue
    }
    close(m)
    out.push({ kind: "user", id: m.id, message: m })
  }
  close()
  return out
}

// isAside tells whether a message holds only notices added after a turn
// ended.
function isAside(m: Message) {
  return isNotice(m) && (m.parts ?? []).every((p) => aside.includes(p.notice?.kind))
}

// splitJoining splits off the user messages at the end that join the
// running turn at its next step. They show last, below the answer
// streaming, and the agent's work before them is still the turn's last row.
export function splitJoining(rs: Row[], joining: string[]): [Row[], Row[]] {
  let i = rs.length
  while (i > 0 && rs[i - 1].kind === "user" && joining.includes(rs[i - 1].id)) i--
  return [rs.slice(0, i), rs.slice(i)]
}

// results are a group's tool results by call ID.
export function results(messages: Message[]): Map<string, ToolResult> {
  const out = new Map<string, ToolResult>()
  for (const m of messages) for (const p of m.parts ?? []) if (p.tool_result) out.set(p.tool_result.call_id, p.tool_result)
  return out
}

// lastNotice is the newest notice of the chat, for the provider card: a
// turn that failed because no model is set up. As on the Go side, messages
// that only note a change of the approval level, an undo or a refresh, and
// a refresh's card, don't count.
export function lastNotice(messages: Message[]): { message: Message; kind: NoticeKind; text: string } | null {
  const m = messages.findLast((m) => !refreshCard(m) && (m.parts ?? []).some((p) => !aside.includes(p.notice?.kind)))
  const p = m?.parts?.[m.parts.length - 1]
  return m && p?.notice ? { message: m, kind: p.notice.kind, text: p.notice.text } : null
}

// toolTurn tells whether the turn called a tool, so it may have changed
// the project and has an *Undo turn*.
export function toolTurn(messages: Message[], turn: number): boolean {
  return messages.some((m) => m.turn === turn && (m.parts ?? []).some((p) => p.kind === PartKind.PartToolCall))
}

// copyText is an agent item's text, for *Copy*.
export function copyText(messages: Message[]): string {
  return messages
    .filter((m) => m.role === Role.RoleAssistant)
    .flatMap((m) => (m.parts ?? []).flatMap((p) => (p.text?.text ? [p.text.text] : [])))
    .join("\n\n")
}

// lines is a rough height of a row in lines of text.
function lines(r: Row): number {
  const ms = r.kind === "agent" ? r.messages : [r.message]
  let n = 1
  for (const m of ms)
    for (const p of m.parts ?? []) {
      const t = p.text?.text
      n += t ? t.split("\n").length + Math.floor(t.length / 80) : 1
    }
  return n
}

// firstRows is how many rows, from the end, a chat renders when it opens:
// about two screens, so the opening frame stays short (N-02).
export function firstRows(rs: Row[], budget = 80, max = 24): number {
  let n = 0
  for (let used = 0; n < rs.length && n < max && used < budget; n++) used += lines(rs[rs.length - 1 - n])
  return Math.max(n, Math.min(2, rs.length))
}
