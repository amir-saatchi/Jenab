// splitBlocks splits Markdown into top-level blocks at blank lines outside
// fenced code, so finished blocks can be memoised while an answer streams
// and only the last one is parsed again (SPEC 5.8). An indented line after
// a blank line continues its block, as a list item's second paragraph does.
export function splitBlocks(src: string): string[] {
  const out: string[] = []
  let cur: string[] = []
  let fence: string | null = null
  let blank = false
  for (const line of src.split("\n")) {
    const m = /^\s*(```+|~~~+)/.exec(line)
    if (fence === null && line.trim() === "") {
      blank = cur.length > 0
      continue
    }
    if (blank) {
      blank = false
      if (fence === null && /^[ \t]/.test(line)) cur.push("")
      else {
        out.push(cur.join("\n"))
        cur = []
      }
    }
    if (m) {
      if (fence === null) fence = m[1]
      else if (m[1].startsWith(fence[0]) && m[1].length >= fence.length) fence = null
    }
    cur.push(line)
  }
  if (cur.length) out.push(cur.join("\n"))
  return out
}
