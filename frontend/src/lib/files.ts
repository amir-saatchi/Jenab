// Keys, folders and links of a project's bucket (SPEC 4.1, 4.5), for the
// Files panel and the chat's jenab:// links.

// CACHE is the app's own folder: fetched pages and long tool outputs. The
// panel lists it apart, as *App cache* (SPEC 5.12).
export const CACHE = "cache/"

const SCHEME = "jenab://"
// A project ID is a ULID in upper case; a key is segments of
// [A-Za-z0-9._-] separated by "/" (SPEC 4.1).
const PROJECT = "[0-9A-HJKMNP-TV-Z]{26}"
const linkRe = new RegExp(`^${SCHEME}(${PROJECT})/([A-Za-z0-9._/-]+)$`)

// LINKS finds jenab:// links in text. A trailing "." or "/" is left out,
// as the end of a sentence.
export const LINKS = new RegExp(`${SCHEME}${PROJECT}/[A-Za-z0-9._/-]*[A-Za-z0-9_-]`, "g")

export function validKey(key: string) {
  if (key.length === 0 || key.length > 512) return false
  return key.split("/").every((s) => /^[A-Za-z0-9._-]+$/.test(s) && s !== "." && s !== "..")
}

// parseLink splits jenab://<project_id>/<key>, or returns null.
export function parseLink(s: string): { project: string; key: string } | null {
  const m = linkRe.exec(s)
  if (!m || !validKey(m[2])) return null
  return { project: m[1], key: m[2] }
}

export function linkOf(project: string, key: string) {
  return `${SCHEME}${project}/${key}`
}

// parentOf is the folder a key is in: "" for the top, else ending in "/".
export function parentOf(key: string) {
  const i = key.replace(/\/$/, "").lastIndexOf("/")
  return i < 0 ? "" : key.slice(0, i + 1)
}

// nameOf is the last segment of a key or folder.
export function nameOf(key: string) {
  const k = key.replace(/\/$/, "")
  return k.slice(k.lastIndexOf("/") + 1)
}

export interface Crumb {
  label: string
  prefix: string
}

// crumbs is the path bar for a folder: Files, then App cache when inside
// cache/, then each folder.
export function crumbs(prefix: string): Crumb[] {
  const out: Crumb[] = [{ label: "Files", prefix: "" }]
  let at = ""
  for (const seg of prefix.split("/").filter(Boolean)) {
    at += seg + "/"
    out.push({ label: at === CACHE ? "App cache" : seg, prefix: at })
  }
  return out
}

// shownFolders leaves cache/ out of the top folder; it has its own entry.
export function shownFolders(prefix: string, folders: string[]) {
  return prefix === "" ? folders.filter((f) => f !== CACHE) : folders
}

export type FileKind = "markdown" | "text" | "image" | "audio" | "video" | "other"

// fileKind is how the panel shows a type, from the MIME type detected when
// the file was stored (SPEC 4.7). HTML, XML and SVG are shown as text, as
// the objects route sends them; an SVG is a picture only through <img>.
export function fileKind(mime: string): FileKind {
  const base = mime.split(";")[0].trim().toLowerCase()
  if (base === "text/markdown") return "markdown"
  if (base === "image/svg+xml") return "image"
  if (base.startsWith("image/")) return shownImages.has(base) ? "image" : "other"
  if (base.startsWith("audio/") || base === "application/ogg") return "audio"
  if (base.startsWith("video/")) return "video"
  if (base.startsWith("text/") || base === "application/json" || base === "application/xml" || base.endsWith("+xml")) return "text"
  return "other"
}

const shownImages = new Set([
  "image/png",
  "image/jpeg",
  "image/gif",
  "image/webp",
  "image/avif",
  "image/bmp",
  "image/x-icon",
  "image/vnd.microsoft.icon",
])

// typeName is a MIME type for people: its base, without parameters.
export function typeName(mime: string) {
  return mime.split(";")[0].trim() || "unknown"
}

export function formatSize(n: number) {
  if (n < 1024) return `${n} B`
  const units = ["KB", "MB", "GB"]
  let v = n / 1024
  let u = 0
  while (v >= 1024 && u < units.length - 1) {
    v /= 1024
    u++
  }
  return `${v < 10 ? v.toFixed(1) : Math.round(v)} ${units[u]}`
}

// whoText is a created_by source (SPEC 2.5) as the panel says it.
export function whoText(source: string) {
  const kind = source.split(":")[0]
  switch (kind) {
    case "user":
      return "you"
    case "message":
    case "auto":
      return "the agent"
    case "run":
      return "a pipeline"
    case "app":
      return "the app"
  }
  return kind || "unknown"
}

// splitLinks cuts text at its jenab:// links: strings stay text, the
// others are links.
export function splitLinks(text: string): (string | { link: string })[] {
  const out: (string | { link: string })[] = []
  let at = 0
  for (const m of text.matchAll(LINKS)) {
    if (!parseLink(m[0])) continue
    if (m.index > at) out.push(text.slice(at, m.index))
    out.push({ link: m[0] })
    at = m.index + m[0].length
  }
  if (at < text.length) out.push(text.slice(at))
  return out
}
