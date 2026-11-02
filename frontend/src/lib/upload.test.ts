import { afterEach, expect, test } from "bun:test"

import { describe, MAX_UPLOAD, tooLarge, upload, withLinks, type Attached } from "@/lib/upload"

const P = "01J0000000000000000000000A"
const done = (name: string, note?: string): Attached => ({
  id: 1,
  name,
  size: 2048,
  state: "done",
  file: { key: `uploads/${name}`, link: `jenab://${P}/uploads/${name}`, mime: "text/plain", size: 2048, name, note },
})

test("a message carries the links of the files that were stored", () => {
  const files: Attached[] = [
    done("a.docx"),
    { id: 2, name: "b.pdf", size: 1, state: "uploading" },
    { id: 3, name: "c.zip", size: 1, state: "error", error: "Too large." },
    done("d.txt"),
  ]
  expect(withLinks("  Summarise these.\n", files)).toBe(
    `Summarise these.\n\njenab://${P}/uploads/a.docx\njenab://${P}/uploads/d.txt`,
  )
  expect(withLinks("", [done("a.docx")])).toBe(`jenab://${P}/uploads/a.docx`)
  expect(withLinks("Hi", [])).toBe("Hi")
})

test("the line under a file says its size, or why it can't be read", () => {
  expect(describe(done("a.docx"))).toBe("2.0 KB")
  expect(describe(done("s.docx", "it is password-protected"))).toBe("Attached, but it can't be read: it is password-protected.")
  expect(describe({ id: 1, name: "x", size: 5, state: "error", error: "It is larger than 100 MB." })).toBe("It is larger than 100 MB.")
  expect(describe({ id: 1, name: "x", size: 5, state: "uploading" })).toBe("5 B")
})

test("files over 100 MB are refused before they are sent", async () => {
  expect(tooLarge(MAX_UPLOAD)).toBe("")
  expect(tooLarge(MAX_UPLOAD + 1)).toBe("It is larger than 100 MB.")
  const big = { name: "big.bin", size: MAX_UPLOAD + 1 } as File
  await expect(upload(P, big)).rejects.toThrow("It is larger than 100 MB.")
})

const realFetch = globalThis.fetch
afterEach(() => {
  globalThis.fetch = realFetch
})

test("upload posts the file with its name and reads the answer", async () => {
  let sent: { url: string; init?: RequestInit } | undefined
  globalThis.fetch = (async (url: string, init?: RequestInit) => {
    sent = { url, init }
    return new Response(JSON.stringify({ key: "uploads/file.docx", link: "l", mime: "m", size: 3, name: "نامه.docx" }))
  }) as typeof fetch
  const f = new File(["abc"], "نامه.docx")
  const up = await upload(P, f)
  expect(up.key).toBe("uploads/file.docx")
  expect(sent?.url).toBe(`/upload/${P}`)
  expect(sent?.init?.method).toBe("POST")
  expect((sent?.init?.headers as Record<string, string>)["X-Jenab-Upload-Name"]).toBe(encodeURIComponent("نامه.docx"))

  globalThis.fetch = (async () =>
    new Response(JSON.stringify({ error: "the file is larger than 100 MB" }), { status: 413 })) as unknown as typeof fetch
  await expect(upload(P, f)).rejects.toThrow("The file is larger than 100 MB.")
  globalThis.fetch = (async () => {
    throw new TypeError("network")
  }) as unknown as typeof fetch
  await expect(upload(P, f)).rejects.toThrow("The file could not be sent to the app.")
})
