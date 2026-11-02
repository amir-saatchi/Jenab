// Files attached in the composer (TASK-021). Each is sent to the upload
// route as it is picked and stored under uploads/<name>; the message then
// carries its jenab:// link, which the agent reads like a key.

import { formatSize } from "@/lib/files"

// MAX_UPLOAD is the largest file the app stores (internal/store.MaxUpload).
export const MAX_UPLOAD = 100 << 20

// Uploaded is the route's answer (internal/app.Uploaded).
export interface Uploaded {
  key: string
  link: string
  mime: string
  size: number
  name: string
  // note says why a document can't be read; the file is stored anyway.
  note?: string
}

export interface Attached {
  id: number
  name: string
  size: number
  state: "uploading" | "done" | "error"
  file?: Uploaded
  error?: string
}

// tooLarge is why a file can't be attached, or "".
export function tooLarge(size: number) {
  return size > MAX_UPLOAD ? `It is larger than ${formatSize(MAX_UPLOAD)}.` : ""
}

// upload stores a file in the project's bucket.
export async function upload(project: string, file: File): Promise<Uploaded> {
  const why = tooLarge(file.size)
  if (why) throw new Error(why)
  let res: Response
  try {
    res = await fetch(`/upload/${project}`, {
      method: "POST",
      headers: { "X-Jenab-Upload-Name": encodeURIComponent(file.name), "Content-Type": "application/octet-stream" },
      body: file,
    })
  } catch {
    throw new Error("The file could not be sent to the app.")
  }
  const body = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(sentence(body.error ?? `The upload failed (${res.status}).`))
  return body as Uploaded
}

// withLinks is the text sent for a message with attached files: the
// user's words, then each file's link on its own line.
export function withLinks(text: string, files: Attached[]) {
  const links = files.flatMap((f) => (f.state === "done" && f.file ? [f.file.link] : []))
  return [text.trim(), links.join("\n")].filter(Boolean).join("\n\n")
}

// describe is the line under an attached file's name.
export function describe(f: Attached) {
  if (f.state === "error") return f.error ?? "Not attached."
  if (f.file?.note) return `Attached, but it can't be read: ${f.file.note}.`
  return formatSize(f.file?.size ?? f.size)
}

// sentence starts a message from the route with a capital letter and ends
// it with a full stop.
function sentence(s: string) {
  s = s.trim()
  if (!s) return s
  s = s[0].toUpperCase() + s.slice(1)
  return /[.!?]$/.test(s) ? s : s + "."
}
