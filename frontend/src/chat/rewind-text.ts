import { PartKind, Role, type Message, type RewindInfo, type RewindState } from "@/lib/api"

// turnsText is "turn 4" or "turns 4–6", as agent.turnsText.
export function turnsText(first: number, last: number): string {
  return first === last ? `turn ${first}` : `turns ${first}–${last}`
}

function upper(s: string) {
  return s.charAt(0).toUpperCase() + s.slice(1)
}

// turnStarts maps the ID of each user message that starts a turn to its
// turn: the turns *Rewind to here* and *Edit and resend* work on
// (TASK-018). A message sent during a turn joins it, so it starts none.
export function turnStarts(messages: Message[]): Map<string, number> {
  const out = new Map<string, number>()
  let turn = -1
  for (const m of messages) {
    if (m.turn === turn) continue
    turn = m.turn
    if (m.role === Role.RoleUser && (m.parts ?? []).some((p) => p.kind !== PartKind.PartNotice)) out.set(m.id, m.turn)
  }
  return out
}

// Mark is a rewind's mark in the chat, after turn at.
export interface Mark {
  rewind: RewindInfo
  at: number
}

// marks places each rewind after the turn it took the chat back to. A
// later rewind to an earlier turn took that place out too, so the mark
// moves up to the later rewind's: the turns it names were gone before
// the chat went on from there.
export function marks(rewinds: RewindInfo[]): Mark[] {
  const byID = [...rewinds].sort((a, b) => a.id - b.id)
  const out: Mark[] = []
  let at = Infinity
  for (let i = byID.length - 1; i >= 0; i--) {
    at = Math.min(at, byID[i].turn)
    out.push({ rewind: byID[i], at })
  }
  return out.reverse()
}

// markText is a mark's line, such as "Turns 4–6 taken out".
export function markText(r: RewindInfo): string {
  return `${upper(turnsText(r.turn + 1, r.last_turn))} taken out`
}

// rewindLines say what a rewind would do, for its dialog: the turns that
// leave, the changes that stay, and the notes and skills that go back.
// edit is *Edit and resend*, whose message goes back in the box.
export function rewindLines(st: RewindState, edit: boolean): string[] {
  const turns = turnsText(st.turn + 1, st.last_turn)
  const lines = [
    edit
      ? `${upper(turns)} ${st.turn + 1 === st.last_turn ? "leaves" : "leave"} this chat and what the model knows of it. Your message goes back in the box, to edit and send again.`
      : `${upper(turns)} ${st.turn + 1 === st.last_turn ? "leaves" : "leave"} this chat and what the model knows of it.`,
  ]
  if (st.changes.changes > 0) {
    const what = st.changes.display || st.changes.summary || "their changes"
    lines.push(`What ${st.turn + 1 === st.last_turn ? "it" : "they"} changed in the project stays: ${what}. Undo them from the rewind's mark.`)
  }
  if (st.notes) lines.push("The session notes go back to how they were.")
  if (st.unload?.length) lines.push(`Skills unloaded: ${st.unload.join(", ")}.`)
  if (st.load?.length) lines.push(`Skills loaded again: ${st.load.join(", ")}.`)
  lines.push("Undo rewind brings the turns back, until the chat goes on.")
  return lines
}
