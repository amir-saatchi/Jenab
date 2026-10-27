// A line diff for a memory edit's card (SPEC 3.3): the lines the edit
// adds and removes, so a planted line can't hide in a long section.

export type LineOp = { kind: "same" | "add" | "del"; text: string }

// A changed line on the card; blanks counts a run of blank lines shown as
// one row, so blank lines can't push a changed line out of view.
export type Change = { kind: "add" | "del"; text: string; blanks: number }

function lines(s?: string): string[] {
  return s ? s.split("\n") : []
}

// lineDiff is before and after as kept, removed and added lines, in order.
export function lineDiff(before?: string, after?: string): LineOp[] {
  const a = lines(before)
  const b = lines(after)
  let start = 0
  while (start < a.length && start < b.length && a[start] === b[start]) start++
  let ea = a.length
  let eb = b.length
  while (ea > start && eb > start && a[ea - 1] === b[eb - 1]) {
    ea--
    eb--
  }
  const mid: LineOp[] = []
  align(a.slice(start, ea), b.slice(start, eb), mid)
  return [
    ...a.slice(0, start).map((text) => ({ kind: "same" as const, text })),
    ...mid,
    ...a.slice(ea).map((text) => ({ kind: "same" as const, text })),
  ]
}

// changes are the diff's added and removed lines, with each run of blank
// ones of a kind as one row.
export function changes(ops: LineOp[]): Change[] {
  const out: Change[] = []
  for (const op of ops) {
    if (op.kind === "same") continue
    const blank = op.text.trim() === ""
    const last = out[out.length - 1]
    if (blank && last && last.kind === op.kind && last.blanks > 0) last.blanks++
    else out.push({ kind: op.kind, text: blank ? "" : op.text, blanks: blank ? 1 : 0 })
  }
  return out
}

// align appends the ops that turn x into y, keeping a longest common run
// of lines (Hirschberg): exact for any size, in memory linear in the
// lines, so no section is too long to diff.
function align(x: string[], y: string[], out: LineOp[]) {
  if (x.length === 0) {
    y.forEach((text) => out.push({ kind: "add", text }))
    return
  }
  if (y.length === 0) {
    x.forEach((text) => out.push({ kind: "del", text }))
    return
  }
  if (x.length === 1) {
    const j = y.indexOf(x[0])
    if (j < 0) {
      out.push({ kind: "del", text: x[0] })
      y.forEach((text) => out.push({ kind: "add", text }))
      return
    }
    y.slice(0, j).forEach((text) => out.push({ kind: "add", text }))
    out.push({ kind: "same", text: x[0] })
    y.slice(j + 1).forEach((text) => out.push({ kind: "add", text }))
    return
  }
  const half = x.length >> 1
  const head = lcsRow(x.slice(0, half), y)
  const tail = lcsRow(x.slice(half).reverse(), [...y].reverse())
  let k = 0
  for (let j = 1; j <= y.length; j++) {
    if (head[j] + tail[y.length - j] > head[k] + tail[y.length - k]) k = j
  }
  align(x.slice(0, half), y.slice(0, k), out)
  align(x.slice(half), y.slice(k), out)
}

// lcsRow is, for each j, the longest common run of x and y's first j lines.
function lcsRow(x: string[], y: string[]): Uint32Array {
  let prev = new Uint32Array(y.length + 1)
  let cur = new Uint32Array(y.length + 1)
  for (const line of x) {
    for (let j = 1; j <= y.length; j++) {
      cur[j] = line === y[j - 1] ? prev[j - 1] + 1 : Math.max(prev[j], cur[j - 1])
    }
    ;[prev, cur] = [cur, prev]
  }
  return prev
}
