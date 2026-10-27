import { expect, test } from "bun:test"

import type { SessionNote } from "@/lib/api"
import { notesLine } from "@/chat/notes"

test("notesLine", () => {
  const now = new Date(2026, 9, 27, 15, 0)
  const n = (revision: number, at: Date) => ({ chat: "c", content: "", summary: "", revision, updated_at: at.toISOString() }) as SessionNote
  expect(notesLine(n(0, now), now)).toBe("What this chat keeps on its current work.")
  expect(notesLine(n(3, new Date(2026, 9, 27, 9, 5)), now)).toStartWith("Revision 3 · updated ")
  expect(notesLine(n(3, new Date(2026, 9, 27, 9, 5)), now)).not.toContain("2026")
  expect(notesLine(n(4, new Date(2026, 9, 20, 9, 5)), now)).toContain("2026")
})
