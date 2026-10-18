import { ConflictReason, type TurnConflict, type UndoItem } from "@/lib/api"

// The internal targets of an undo, as store names them.
const objectItem = "_jenab_objects" // key object:<key>
const roleItem = "_jenab_chat_role" // store.RoleItem; key the chat ID

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
  if (it.key === "") return `the table ${isolate(it.table)}`
  return `row ${keyText(it.key)} of ${isolate(it.table)}`
}

// conflictText says why an item stands in the undo's way.
export function conflictText(c: TurnConflict): string {
  const x = c.conflict
  switch (x.reason) {
    case ConflictReason.ConflictChanged: {
      const by = c.chat_title ? ` in the chat “${isolate(c.chat_title)}”` : x.source === "user" ? " by you" : x.source ? ` by ${x.source}` : ""
      return `${itemName(x.item)} changed since${by}`
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
