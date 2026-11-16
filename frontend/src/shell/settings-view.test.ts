import { beforeEach, expect, mock, test } from "bun:test"
import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"

import type { ProjectItem } from "@/lib/api"
import { current, useNav } from "@/state/nav"
import { useUI } from "@/state/ui"
import { openProjectOf, openProjectSettings } from "@/shell/open"

// Settings' page list has the open project, under its name, with its
// Project settings; Project settings has the project's name as its title
// (TASK-050).

// The sidebar asks the window's width, which a static render has none of.
mock.module("@/hooks/use-mobile", () => ({ NARROW_WIDTH: 900, useIsMobile: () => false }))
const { SidebarProvider } = await import("@/components/ui/sidebar")
const { OpenProjectPages } = await import("@/shell/settings-view")
const { ProjectSettingsTitle } = await import("@/shell/project-settings")

const P = "01JABCDEFGHJKMNPQRSTVWXYZ0"
const item = { id: P, name: "قیمت‌ها", folder: "D:/Projects/prices", pinned: false, created_at: "2026-11-01T10:00:00Z" } as ProjectItem

beforeEach(() => {
  useNav.setState({ history: { places: [{ view: "settings" }], index: 0 }, mru: [{ project: P, chat: "c1" }] })
  useUI.setState({ leftOverlay: true })
})

const pages = (project?: ProjectItem) =>
  renderToStaticMarkup(createElement(SidebarProvider, null, createElement(OpenProjectPages, { project })))

test("the open project is the one of the chat used last, if it is still there", () => {
  expect(openProjectOf(P, [item])).toBe(item)
  expect(openProjectOf(undefined, [item])).toBeUndefined()
  expect(openProjectOf("gone", [item])).toBeUndefined()
})

test("with a project open, the page list has its Project settings under its name", () => {
  const html = pages(item)
  expect(html).toContain(`<span class="truncate" dir="auto">قیمت‌ها</span>`)
  expect(html).toContain("<span>Project settings</span>")
})

test("with no project open, the page list has no project entry", () => {
  expect(pages()).not.toContain("Project settings")
  expect(pages()).not.toContain("data-sidebar=\"group\"")
})

test("the entry opens the project's settings", () => {
  openProjectSettings(P)
  expect(current(useNav.getState())).toEqual({ view: "project-settings", project: P, section: undefined })
  expect(useUI.getState().leftOverlay).toBe(false)
})

test("Project settings has the project's name as its title", () => {
  const html = renderToStaticMarkup(createElement(ProjectSettingsTitle, { item }))
  expect(html).toMatch(/<h1[^>]*><span dir="auto">قیمت‌ها<\/span><\/h1>/)
  expect(html).toContain("Project settings · created")
  expect(renderToStaticMarkup(createElement(ProjectSettingsTitle, {}))).toMatch(/<h1[^>]*>Project settings<\/h1>/)
})
