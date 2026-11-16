import { beforeEach, expect, test } from "bun:test"
import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"

import type { ProjectItem } from "@/lib/api"
import { current, useNav } from "@/state/nav"
import { MemoryScope } from "@/settings/memory"
import { openProjectSettings } from "@/shell/open"

// Settings → Memory says it is for every project, and where the open
// project's own memory is (TASK-050).

const P = "01JABCDEFGHJKMNPQRSTVWXYZ0"
const item = { id: P, name: "قیمت‌ها", folder: "D:/Projects/prices", pinned: false, created_at: "2026-11-01T10:00:00Z" } as ProjectItem

beforeEach(() => {
  useNav.setState({ history: { places: [{ view: "settings", section: "Memory" }], index: 0 }, mru: [{ project: P, chat: "c1" }] })
})

const html = (project?: ProjectItem) => renderToStaticMarkup(createElement(MemoryScope, { project }))

test("with a project open, the line links to its memory in its settings", () => {
  const h = html(item)
  expect(h).toContain("This memory is for every project.")
  expect(h).toContain('<span dir="auto">قیمت‌ها</span> has its own memory too, in')
  expect(h).toMatch(/<button[^>]*>Project settings › Memory<\/button>/)
  // The link opens that page at its Memory section.
  openProjectSettings(P, "Memory")
  expect(current(useNav.getState())).toEqual({ view: "project-settings", project: P, section: "Memory" })
})

test("with no project open, the line has no link", () => {
  const h = html()
  expect(h).toContain("This memory is for every project. Each project has its own memory too, in its Project settings.")
  expect(h).not.toContain("<button")
})
