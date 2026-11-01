import { beforeEach, expect, test } from "bun:test"

import type { FileObject } from "@/lib/api"
import { CACHE, crumbs, fileKind, formatSize, linkOf, nameOf, parentOf, parseLink, shownFolders, splitLinks, whoText } from "@/lib/files"
import { chatKey, clampWidth, useDock } from "@/state/dock"
import { useFiles } from "@/state/files"
import { useNav } from "@/state/nav"
import { useUI } from "@/state/ui"
import { listFolder } from "@/shell/files-panel"
import { openFile, openLink } from "@/shell/open-file"

const P = "01JABCDEFGHJKMNPQRSTVWXYZ0"
const Q = "01JZZZZZZZZZZZZZZZZZZZZZZZ"

beforeEach(() => {
  useNav.setState({ history: { places: [{ view: "chat", project: P, chat: "c1" }], index: 0 }, mru: [] })
  useDock.setState({ open: {}, chatHidden: {}, drawer: {}, side: {}, width: {}, listShown: false })
  useFiles.setState({ folder: {}, rev: {} })
  useUI.setState({ rightTab: "pages", rightOverlay: false })
})

test("folder navigation: the path bar, parents and names", () => {
  expect(crumbs("")).toEqual([{ label: "Files", prefix: "" }])
  expect(crumbs("docs/2026/")).toEqual([
    { label: "Files", prefix: "" },
    { label: "docs", prefix: "docs/" },
    { label: "2026", prefix: "docs/2026/" },
  ])
  expect(crumbs("cache/pages/")).toEqual([
    { label: "Files", prefix: "" },
    { label: "App cache", prefix: "cache/" },
    { label: "pages", prefix: "cache/pages/" },
  ])
  expect(parentOf("a.txt")).toBe("")
  expect(parentOf("docs/2026/a.txt")).toBe("docs/2026/")
  expect(parentOf("docs/2026/")).toBe("docs/")
  expect(nameOf("docs/2026/a.txt")).toBe("a.txt")
  expect(nameOf("docs/2026/")).toBe("2026")
})

const obj = (key: string) => ({ key, mime: "text/plain", size: 1 }) as FileObject

// A fake List with pages of two entries, like the Go side's pages of 100.
function fakeList(all: Record<string, { folders: string[]; objects: string[] }>) {
  const calls: string[] = []
  const list = async (_p: string, prefix: string, after: string) => {
    calls.push(`${prefix}|${after}`)
    const f = all[prefix] ?? { folders: [], objects: [] }
    const names = [...f.folders, ...f.objects].sort()
    const from = after ? names.indexOf(after) + 1 : 0
    const page = names.slice(from, from + 2)
    return {
      folders: page.filter((n) => n.endsWith("/")),
      objects: page.filter((n) => !n.endsWith("/")).map(obj),
      next: from + 2 < names.length ? page[page.length - 1] : "",
    }
  }
  return { list, calls }
}

test("folder navigation: a long folder loads page by page", async () => {
  const { list, calls } = fakeList({ "docs/": { folders: ["docs/old/"], objects: ["docs/a.txt", "docs/b.txt", "docs/c.txt"] } })
  let f = await listFolder(list, P, "docs/", null)
  expect(f.objects.map((o) => o.key)).toEqual(["docs/a.txt", "docs/b.txt"])
  expect(f.next).toBe("docs/b.txt")
  f = await listFolder(list, P, "docs/", f, f.next)
  expect(f.objects.map((o) => o.key)).toEqual(["docs/a.txt", "docs/b.txt", "docs/c.txt"])
  expect(f.folders).toEqual(["docs/old/"])
  expect(f.next).toBe("")
  expect(calls).toEqual(["docs/|", "docs/|docs/b.txt"])
})

test("cache/ is hidden from the top folder only", async () => {
  expect(shownFolders("", ["a/", CACHE, "z/"])).toEqual(["a/", "z/"])
  expect(shownFolders("x/", ["x/cache/"])).toEqual(["x/cache/"])
  expect(shownFolders(CACHE, ["cache/pages/", "cache/tool/"])).toEqual(["cache/pages/", "cache/tool/"])
  const { list } = fakeList({
    "": { folders: ["a/", CACHE], objects: [] },
    [CACHE]: { folders: ["cache/pages/"], objects: [] },
  })
  expect((await listFolder(list, P, "", null)).folders).toEqual(["a/"])
  expect((await listFolder(list, P, CACHE, null)).folders).toEqual(["cache/pages/"])
})

test("links: parsing and finding them in text", () => {
  expect(parseLink(`jenab://${P}/docs/a.txt`)).toEqual({ project: P, key: "docs/a.txt" })
  expect(linkOf(P, "docs/a.txt")).toBe(`jenab://${P}/docs/a.txt`)
  for (const bad of [`jenab://${P}/`, `jenab://${P}/../a`, `jenab://${P}/a//b`, `jenab://nope/a.txt`, `https://x/a`, `jenab://${P}/a b`]) {
    expect(parseLink(bad)).toBeNull()
  }
  const link = `jenab://${P}/docs/report.pdf`
  expect(splitLinks(`See ${link}. Done`)).toEqual(["See ", { link }, ". Done"])
  expect(splitLinks(`فایل ${link}`)).toEqual(["فایل ", { link }])
  expect(splitLinks("no links")).toEqual(["no links"])
})

test("opening a link opens the file next to the chat and shows its folder", async () => {
  const asked: string[] = []
  await openLink(`jenab://${P}/docs/2026/a.txt`, async (p, k) => void asked.push(`${p}/${k}`))
  expect(asked).toEqual([`${P}/docs/2026/a.txt`])
  expect(useDock.getState().open[chatKey(P, "c1")]).toEqual({ kind: "file", key: "docs/2026/a.txt" })
  expect(useFiles.getState().folder[P]).toBe("docs/2026/")
  expect(useUI.getState().rightTab).toBe("files")
})

test("a link to another project or to a missing file opens nothing", async () => {
  let asked = 0
  await openLink(`jenab://${Q}/a.txt`, async () => void asked++)
  expect(asked).toBe(0)
  await openLink(`jenab://${P}/gone.txt`, async () => {
    throw { cause: { kind: "not_found", message: "It no longer exists." } }
  })
  await openLink("jenab://bad", async () => void asked++)
  expect(asked).toBe(0)
  expect(useDock.getState().open[chatKey(P, "c1")]).toBeUndefined()
  expect(useUI.getState().rightTab).toBe("pages")
})

test("the dock: open, close, and a deleted file closes", () => {
  openFile(P, "c1", "a.txt")
  openFile(P, "c2", "a.txt")
  openFile(P, "c3", "b.txt")
  useDock.getState().closeFile(P, "a.txt")
  expect(useDock.getState().open[chatKey(P, "c1")]).toBeUndefined()
  expect(useDock.getState().open[chatKey(P, "c2")]).toBeUndefined()
  expect(useDock.getState().open[chatKey(P, "c3")]).toEqual({ kind: "file", key: "b.txt" })
  useDock.getState().swap(P)
  expect(useDock.getState().side[P]).toBe("left")
  useDock.getState().setChatHidden(P, "c3", true)
  useDock.getState().close(P, "c3")
  expect(useDock.getState().open[chatKey(P, "c3")]).toBeUndefined()
  expect(useDock.getState().chatHidden[chatKey(P, "c3")]).toBe(false)
  openFile(P, "c3", "b.txt")
  useDock.getState().setListShown(true)
  openFile(P, "c3", "c.txt")
  expect(useDock.getState().listShown).toBe(true)
  openFile(P, "c1", "c.txt")
  expect(useDock.getState().listShown).toBe(false)
  expect(clampWidth(100, 1200)).toBe(320)
  expect(clampWidth(1000, 1200)).toBe(880)
  expect(clampWidth(500, 1200)).toBe(500)
  expect(clampWidth(500, 500)).toBe(320)
})

test("fileKind, sizes and who", () => {
  expect(fileKind("text/markdown; charset=utf-8")).toBe("markdown")
  expect(fileKind("text/html; charset=utf-8")).toBe("text")
  expect(fileKind("application/json")).toBe("text")
  expect(fileKind("image/svg+xml")).toBe("image")
  expect(fileKind("image/tiff")).toBe("other")
  expect(fileKind("application/pdf")).toBe("other")
  expect(fileKind("audio/mpeg")).toBe("audio")
  expect(fileKind("application/octet-stream")).toBe("other")
  expect(formatSize(900)).toBe("900 B")
  expect(formatSize(1536)).toBe("1.5 KB")
  expect(formatSize(5 * 2 ** 20)).toBe("5.0 MB")
  expect(whoText("user")).toBe("you")
  expect(whoText("message:01J")).toBe("the agent")
  expect(whoText("run:01J")).toBe("a pipeline")
})
