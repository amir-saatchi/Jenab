import * as React from "react"
import { BrainIcon, ChevronDownIcon, ChevronRightIcon } from "lucide-react"
import { toast } from "sonner"

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { ButtonGroup } from "@/components/ui/button-group"
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Spinner } from "@/components/ui/spinner"
import { ChatService, ChatState, type ChatNotice, type RefreshEstimate } from "@/lib/api"
import { showError } from "@/lib/errors"
import { MemoryEdit, memoryTitle } from "@/chat/cards"
import { modelName } from "@/chat/composer"
import { useSettings } from "@/state/settings"
import { useThreads } from "@/state/thread"

// Refresh memory (SPEC 3.5): the header's button, and the chip of a
// batch.

// RefreshButton is *Refresh memory ▾* in the chat's header: a refresh of
// the turns not reviewed yet, or, from its menu, a Full refresh of the
// whole chat. A long one asks first, with its cost. The chat's status
// shows the progress, and Stop cancels it.
export function RefreshButton({ project, chat }: { project: string; chat: string }) {
  const idle = useThreads((s) => s.threads[chat]?.state === ChatState.StateIdle)
  const groups = useSettings((s) => s.models)
  const [busy, setBusy] = React.useState(false)
  const [ask, setAsk] = React.useState<{ est: RefreshEstimate; full: boolean } | null>(null)
  const start = async (full: boolean) => {
    setBusy(true)
    try {
      const est = await ChatService.RefreshEstimate(project, chat, full)
      if (est.long) setAsk({ est, full })
      else await ChatService.RefreshMemory(project, chat, full)
    } catch (err) {
      showError(err)
    } finally {
      setBusy(false)
    }
  }
  const run = (full: boolean) => {
    setAsk(null)
    ChatService.RefreshMemory(project, chat, full).catch(showError)
  }
  const off = !idle || busy
  return (
    <>
      <ButtonGroup>
        <Button variant="outline" size="sm" disabled={off} onClick={() => void start(false)}>
          {busy ? <Spinner data-icon="inline-start" /> : <BrainIcon data-icon="inline-start" />}
          Refresh memory
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline" size="icon-sm" aria-label="More ways to refresh memory" disabled={off}>
              <ChevronDownIcon />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onSelect={() => void start(true)}>Full refresh: review the whole chat again</DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </ButtonGroup>
      <AlertDialog open={ask !== null} onOpenChange={(o) => !o && setAsk(null)}>
        <AlertDialogContent>
          {ask && (
            <>
              <AlertDialogHeader>
                <AlertDialogTitle>{estimateTitle(ask.est, ask.full)}</AlertDialogTitle>
                <AlertDialogDescription>{estimateText(ask.est, modelName(groups ?? [], ask.est.model))}</AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Cancel</AlertDialogCancel>
                <AlertDialogAction onClick={() => run(ask.full)}>Refresh</AlertDialogAction>
              </AlertDialogFooter>
            </>
          )}
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

const plural = (n: number, one: string, many = `${one}s`) => `${n.toLocaleString()} ${n === 1 ? one : many}`

// estimateTitle and estimateText are the dialog before a long refresh.
export function estimateTitle(e: RefreshEstimate, full: boolean) {
  return `${full ? "Review the whole chat again" : "Refresh memory"}: ${plural(e.turns, "turn")}?`
}

export function estimateText(e: RefreshEstimate, model: string) {
  const saved = e.saved > 0 ? ` ${plural(e.saved, "part")} of ${e.chunks} reviewed before ${e.saved === 1 ? "is" : "are"} kept.` : ""
  return (
    `This makes ${plural(e.calls, "call")} to ${model}, with about ${plural(e.tokens, "token")} of input.${saved}` +
    " Memory changes only when it ends, and Stop cancels it."
  )
}

// undoChangedText is the toast when an undo of a batch did nothing,
// because sections changed since.
export function undoChangedText(sections: string[]) {
  const what = sections.length ? sections.join("; ") : "A section"
  return `${what.charAt(0).toUpperCase()}${what.slice(1)} changed since, so nothing was undone. Edit memory in the settings.`
}

// RefreshChip is a batch's chip: Memory refreshed: 3 added, 1 changed
// [view changes] [undo]. Undo reverts the batch's edits together, and Redo
// puts them back; the backend says whether they are undone, so the chip is
// right after a reload. A chip without edits, such as a failed refresh's,
// is a line.
export function RefreshChip({ project, notice }: { project: string; notice: ChatNotice }) {
  const edits = notice.edits ?? []
  const [undone, setUndone] = React.useState(false)
  const [busy, setBusy] = React.useState(false)
  React.useEffect(() => {
    if (!edits.length) return
    let live = true
    ChatService.RefreshUndone(project, edits)
      .then((u) => live && setUndone(u))
      .catch(() => {}) // the chip keeps Undo; a click says what is wrong
    return () => {
      live = false
    }
    // A notice is replaced, never changed, so it stands for its edits.
  }, [project, notice])
  const flip = async () => {
    setBusy(true)
    try {
      const r = await ChatService.UndoRefresh(project, { edits, redo: undone })
      setUndone(r.undone)
      if (r.changed) toast.info(undoChangedText(r.sections ?? []))
      else if (!r.done) toast.info(r.undone ? "These edits were already undone." : "These edits are already back.")
    } catch (err) {
      showError(err)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Collapsible className="flex flex-col gap-2 not-typeset">
      <div className="flex flex-wrap items-center gap-1.5">
        <Badge variant="outline" className="h-6 w-fit font-normal text-muted-foreground">
          <BrainIcon data-icon="inline-start" />
          <span dir="auto">{notice.text}</span>
          {undone && <span>· undone</span>}
        </Badge>
        <CollapsibleTrigger asChild>
          <Button variant="ghost" size="xs" className="text-muted-foreground">
            <ChevronRightIcon data-icon="inline-start" className="transition-transform group-data-[state=open]/button:rotate-90" />
            View changes
          </Button>
        </CollapsibleTrigger>
        <Button variant="ghost" size="xs" className="text-muted-foreground" disabled={busy} onClick={() => void flip()}>
          {undone ? "Redo" : "Undo"}
        </Button>
      </div>
      <CollapsibleContent className="flex flex-col gap-4 rounded-lg border p-3">
        {edits.map((e) => (
          <MemoryEdit key={e.target} title={memoryTitle(e.target)} before={e.before} after={e.after} />
        ))}
      </CollapsibleContent>
    </Collapsible>
  )
}
