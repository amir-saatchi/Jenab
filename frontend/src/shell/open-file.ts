import type * as React from "react"
import { Browser } from "@wailsio/runtime"
import { toast } from "sonner"

import { BucketService } from "@/lib/api"
import { showError, toUIError } from "@/lib/errors"
import { CACHE, linkOf, nameOf, parentOf, parseLink } from "@/lib/files"
import { useDock } from "@/state/dock"
import { useFiles } from "@/state/files"
import { current, useNav } from "@/state/nav"
import { useUI } from "@/state/ui"

// openFile opens a file in the main area with the chat in the dock, and
// shows its folder in the Files panel (SPEC 5.12).
export function openFile(project: string, chat: string, key: string) {
  useFiles.getState().setFolder(project, parentOf(key))
  useUI.getState().set({ rightTab: "files", rightOverlay: false })
  useDock.getState().show(project, chat, { kind: "file", key })
}

// openHere opens a file of the open chat's project, such as a tool's
// stored output.
export function openHere(key: string) {
  const place = current(useNav.getState())
  if (place?.view === "chat") openFile(place.project, place.chat, key)
}

type Head = (project: string, key: string) => Promise<unknown>

// openLink opens a jenab:// link from the chat in the open chat (SPEC
// 4.5). A link to another project, or to a file that is gone, says so.
export async function openLink(href: string, head: Head = (p, k) => BucketService.Head(p, k)) {
  const l = parseLink(href)
  const place = current(useNav.getState())
  if (!l || place?.view !== "chat") return
  if (l.project !== place.project) {
    toast.info("This link is to a file in another project.")
    return
  }
  try {
    await head(l.project, l.key)
  } catch (err) {
    const e = toUIError(err)
    if (e.kind === "not_found") toast.info(`${nameOf(l.key)} no longer exists. It was deleted or moved.`)
    else showError(err)
    return
  }
  // The place may have changed while the file was looked up.
  if (current(useNav.getState()) === place) openFile(place.project, place.chat, l.key)
}

// openAnchor is the click of a link in the chat, in an answer's Markdown
// or in the user's own text. Links open in the browser, not in the app's
// window; a middle click too. A jenab:// link opens the file in the Files
// panel (SPEC 4.5).
export function openAnchor(e: React.MouseEvent<HTMLAnchorElement>) {
  if (e.type === "auxclick" && e.button !== 1) return
  const raw = e.currentTarget.getAttribute("href") ?? ""
  const href = e.currentTarget.href
  e.preventDefault()
  if (raw.startsWith("jenab://")) void openLink(raw)
  else if (/^(https?|mailto):/i.test(href)) void Browser.OpenURL(href)
}

export function copyLink(project: string, key: string) {
  navigator.clipboard.writeText(linkOf(project, key)).then(
    () => toast.success("Link copied"),
    (err) => showError(err),
  )
}

// deleteFile deletes a file as the user; the toast's Undo puts it back
// while the change is in the undo window (SPEC 4.4).
export async function deleteFile(project: string, key: string) {
  try {
    const d = await BucketService.Delete(project, key)
    useDock.getState().closeFile(project, key)
    useFiles.getState().changed(project)
    toast(`Deleted ${nameOf(key)}`, {
      description: key.startsWith(CACHE) ? "From the app cache." : undefined,
      duration: 10_000,
      action: {
        label: "Undo",
        onClick: () =>
          void BucketService.UndoDelete(project, d.change).then(
            () => {
              useFiles.getState().changed(project)
              toast.success(`${nameOf(key)} is back`)
            },
            showError,
          ),
      },
    })
  } catch (err) {
    showError(err)
  }
}

export async function openWithSystem(project: string, key: string) {
  try {
    await BucketService.OpenWithSystem(project, key)
  } catch (err) {
    showError(err)
  }
}
