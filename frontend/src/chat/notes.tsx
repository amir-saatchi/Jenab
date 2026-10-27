import * as React from "react"
import { NotebookTextIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { Spinner } from "@/components/ui/spinner"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { ChatService, type SessionNote } from "@/lib/api"
import { showError } from "@/lib/errors"
import { Markdown } from "@/chat/markdown"
import { useThreads } from "@/state/thread"

// NotesButton opens the chat's session notes (SPEC 3.4), read-only: the
// chat writes them as it works, and Refresh memory writes the summary.
// While open they are read again on each change to the chat.
export function NotesButton({ project, chat }: { project: string; chat: string }) {
  const [open, setOpen] = React.useState(false)
  const [notes, setNotes] = React.useState<SessionNote | null>(null)
  const seq = useThreads((s) => s.threads[chat]?.seq)
  React.useEffect(() => setNotes(null), [project, chat])
  React.useEffect(() => {
    if (!open) return
    let live = true
    ChatService.Notes(project, chat)
      .then((n) => live && setNotes(n))
      .catch(showError)
    return () => {
      live = false
    }
  }, [open, project, chat, seq])

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="ghost" size="icon-sm" aria-label="Session notes" onClick={() => setOpen(true)}>
            <NotebookTextIcon />
          </Button>
        </TooltipTrigger>
        <TooltipContent>Session notes</TooltipContent>
      </Tooltip>
      <SheetContent side="right" className="gap-0 sm:max-w-md">
        <SheetHeader className="border-b">
          <SheetTitle>Session notes</SheetTitle>
          <SheetDescription>{notes ? notesLine(notes) : "What this chat keeps on its current work."}</SheetDescription>
        </SheetHeader>
        <div className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto p-4">
          {!notes ? (
            <div className="flex flex-1 items-center justify-center">
              <Spinner className="text-muted-foreground" />
            </div>
          ) : !notes.content && !notes.summary ? (
            <Empty>
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <NotebookTextIcon />
                </EmptyMedia>
                <EmptyTitle>No notes yet</EmptyTitle>
                <EmptyDescription>The chat writes its task, plan and open questions here as it works.</EmptyDescription>
              </EmptyHeader>
            </Empty>
          ) : (
            <>
              {notes.summary && <NotesSection title="Summary" text={notes.summary} />}
              {notes.content && <NotesSection title={notes.summary ? "Notes" : ""} text={notes.content} />}
            </>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}

function NotesSection({ title, text }: { title: string; text: string }) {
  return (
    <section className="flex flex-col gap-2">
      {title && <h3 className="text-xs font-medium text-muted-foreground">{title}</h3>}
      <Markdown>{text}</Markdown>
    </section>
  )
}

// notesLine is the sheet's description: the revision and when the notes
// last changed.
export function notesLine(n: SessionNote, now = new Date()) {
  if (!n.revision) return "What this chat keeps on its current work."
  const t = new Date(n.updated_at)
  const today = t.toDateString() === now.toDateString()
  const when = today ? t.toLocaleTimeString([], { timeStyle: "short" }) : t.toLocaleDateString([], { dateStyle: "medium" })
  return `Revision ${n.revision} · updated ${when}`
}
