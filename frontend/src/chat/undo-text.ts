import { ConflictReason, type TurnConflict, type TurnUndoState, type UndoItem } from "@/lib/api"

// The internal targets of an undo, as store names them.
const objectItem = "_jenab_objects" // key object:<key>
const roleItem = "_jenab_chat_role" // store.RoleItem; key the chat ID
const projectMemoryItem = "_jenab_memory" // key memory:project/<section>
const userMemoryItem = "_jenab_user_memory" // store.UserMemoryItem; key the section

// isolate keeps a name's direction from reordering the sentence around it,
// as a Persian table name in English text would (U+2068 FSI ... U+2069 PDI).
export function isolate(s: string): string {
  return `⁨${s}⁩`
}

// keyText is a row's primary key, stored in SQLite's json_array form, as
// the user reads it: "[2]" is 2, "[1,\"a\"]" is 1, a. Each part is isolated.
function keyText(key: string): string {
  try {
    const parts = JSON.parse(key) as unknown
    if (Array.isArray(parts)) return parts.map((p) => isolate(typeof p === "string" ? p : JSON.stringify(p))).join(", ")
  } catch {
    // shown as stored
  }
  return isolate(key)
}

// itemName names what an undo would revert, for the conflict dialog. A
// key of "" is the whole table, as a later migration's conflict has it.
export function itemName(it: UndoItem): string {
  if (it.table === objectItem) return `the file ${isolate(it.key.replace(/^object:/, ""))}`
  if (it.table === roleItem) return "the chat's role"
  if (it.table === projectMemoryItem) return `the project memory section ${isolate(it.key.replace(/^memory:project\//, ""))}`
  if (it.table === userMemoryItem) return `the user memory section ${isolate(it.key)}`
  if (it.key === "") return `the table ${isolate(it.table)}`
  return `row ${keyText(it.key)} of ${isolate(it.table)}`
}

// changedBy says who made a later change. A message whose chat isn't in
// this project is, for user memory, a chat of another project.
function changedBy(c: TurnConflict): string {
  const src = c.conflict.source ?? ""
  if (c.chat_title) return ` in the chat “${isolate(c.chat_title)}”`
  if (src === "user") return " by you"
  if (src.startsWith("message:")) return c.conflict.item.table === userMemoryItem ? " by a chat in another project" : " by another chat"
  if (src.startsWith("undo:")) return " by an undo"
  return src ? ` by ${src}` : ""
}

// conflictText says why an item stands in the undo's way.
export function conflictText(c: TurnConflict): string {
  const x = c.conflict
  switch (x.reason) {
    case ConflictReason.ConflictChanged: {
      return `${itemName(x.item)} changed since${changedBy(c)}`
    }
    case ConflictReason.ConflictCascade:
      return x.reached ? `undoing ${itemName(x.item)} would also change ${itemName(x.reached)}` : `undoing ${itemName(x.item)} would change other rows`
    case ConflictReason.ConflictReferenced:
      return x.reached
        ? `${itemName(x.reached)} refers to ${itemName(x.item)}`
        : `${itemName(x.item)} refers to a row that is gone`
    case ConflictReason.ConflictSchema:
      return x.item.key === ""
        ? `a later migration changed ${itemName(x.item)}`
        : `the table or a column of ${itemName(x.item)} is gone`
    case ConflictReason.ConflictConstraint:
      return `putting back ${itemName(x.item)} breaks a rule of its table${x.detail ? `: ${x.detail}` : ""}`
    default:
      return `${itemName(x.item)} can't be undone${x.detail ? `: ${x.detail}` : ""}`
  }
}

// skipAll adds the conflicts' items to skip, each once.
export function skipAll(skip: UndoItem[], cs: TurnConflict[]): UndoItem[] {
  const out = [...skip]
  const has = new Set(skip.map((it) => `${it.table}\u0000${it.key}`))
  for (const c of cs) {
    const k = `${c.conflict.item.table}\u0000${c.conflict.item.key}`
    if (!has.has(k)) {
      has.add(k)
      out.push(c.conflict.item)
    }
  }
  return out
}

// Partly is a turn partly undone, for the footer: its chip text, and
// which way the rest is still to go. That is the way of the turn's last
// undo or redo, whose items left are the other side's (TurnPlan.left):
// "undo" after an undo, "redo" after a redo. It is "" when that can't be
// told; then both ways are the rest.
export type Partly = { text: string; rest: "undo" | "redo" | "" }

// partly reads the turn's state: "partly undone: 2 items not undone"
// after an undo that skipped two, or that undid one of the turn's changes
// alone; "partly redone: 1 item not redone" after such a redo. It is null
// when the turn is all in effect or all undone, or past the undo window,
// where it is history.
export function partly(st: TurnUndoState | null): Partly | null {
  if (!st || st.undo.changes === 0 || st.redo.changes === 0 || st.undo.past || st.redo.past) return null
  const items = (n: number) => `${n} ${n === 1 ? "item" : "items"}`
  if (st.undo.left > 0) return { text: `partly undone: ${items(st.undo.left)} not undone`, rest: "undo" }
  if (st.redo.left > 0) return { text: `partly redone: ${items(st.redo.left)} not redone`, rest: "redo" }
  return { text: "partly undone", rest: "" }
}

// isRest says whether an undo (redo false) or a redo of a partly undone
// turn is its rest: the way it still has to go.
export function isRest(p: Partly | null, redo: boolean): boolean {
  return p !== null && (p.rest === "" || p.rest === (redo ? "redo" : "undo"))
}
