/**
 * Splits Markdown into top-level blocks at blank lines outside fenced code, so
 * finished blocks can be memoised while a reply streams and only the last
 * (growing) block is parsed again.
 */
export function splitBlocks(src: string): string[] {
  const out: string[] = [];
  let cur: string[] = [];
  let fence: string | null = null;
  for (const line of src.split("\n")) {
    const m = /^\s*(```|~~~)/.exec(line);
    if (m) {
      if (fence === null) fence = m[1];
      else if (m[1] === fence) fence = null;
    }
    if (fence === null && line.trim() === "") {
      if (cur.length) { out.push(cur.join("\n")); cur = []; }
      continue;
    }
    cur.push(line);
  }
  if (cur.length) out.push(cur.join("\n"));
  return out;
}
