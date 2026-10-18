import * as React from "react"
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
import { Button } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"
import { ChatService, type TurnConflict, type TurnPlan, type TurnUndoState, type UndoItem } from "@/lib/api"
import { showError } from "@/lib/errors"
import { conflictText, skipAll } from "@/chat/undo-text"

// Step is the dialog open now: the confirmation with what the undo or
// redo reverts, the choice for a turn that is half undone, or the
// conflicts that stopped it.
type Step =
  | { kind: "confirm"; redo: boolean; plan: TurnPlan; migrations: number }
  | { kind: "half"; undo: TurnPlan; redo: TurnPlan; migrations: number }
  | { kind: "conflicts"; redo: boolean; conflicts: TurnConflict[]; skip: UndoItem[] }

// refused says why a plan can't run, or "" when it can.
function refused(p: TurnPlan, verb: string): string {
  if (p.past) return "This turn is past the undo window. Its changes are kept as history only."
  if (p.too_big) return `This turn changed ${p.entries.toLocaleString()} rows, more than ${verb} can revert at once.`
  return ""
}

// UndoTurn is *Undo turn* in a turn's footer (SPEC 2.6), or *Redo turn*
// once the turn is undone. It asks the Go side what each would revert,
// confirms with that summary, and lists the conflicts if later changes
// stand in its way. A turn left half undone, by a crash or by skipped
// conflicts, offers both. The chat then gets a notice, so the model knows;
// it arrives with the chat's events, and seen, the chat's message count,
// makes the footer ask again.
export function UndoTurn({ project, chat, turn, seen }: { project: string; chat: string; turn: number; seen: number }) {
  const [state, setState] = React.useState<TurnUndoState | null>(null)
  const [step, setStep] = React.useState<Step | null>(null)
  const [busy, setBusy] = React.useState(false)

  React.useEffect(() => {
    let live = true
    ChatService.UndoState(project, chat, turn).then(
      (st) => live && setState(st),
      () => {}, // the button still works; open() shows the error
    )
    return () => {
      live = false
    }
  }, [project, chat, turn, seen])

  const open = async () => {
    setBusy(true)
    try {
      const st: TurnUndoState = await ChatService.UndoState(project, chat, turn)
      setState(st)
      if (st.undo.changes > 0 && st.redo.changes > 0) setStep({ kind: "half", undo: st.undo, redo: st.redo, migrations: st.migrations })
      else if (st.undo.changes > 0) setStep({ kind: "confirm", redo: false, plan: st.undo, migrations: st.migrations })
      else if (st.redo.changes > 0) setStep({ kind: "confirm", redo: true, plan: st.redo, migrations: 0 })
      else toast("This turn changed nothing that can be undone.")
    } catch (err) {
      showError(err)
    } finally {
      setBusy(false)
    }
  }

  const run = async (redo: boolean, skip: UndoItem[]) => {
    setBusy(true)
    try {
      const r = await ChatService.UndoTurn(project, chat, { turn, redo, skip })
      if (r.conflicts?.length) setStep({ kind: "conflicts", redo, conflicts: r.conflicts, skip })
      else setStep(null)
    } catch (err) {
      setStep(null)
      showError(err)
    } finally {
      setBusy(false)
      ChatService.UndoState(project, chat, turn).then(setState, () => {})
    }
  }

  const redoOnly = state !== null && state.undo.changes === 0 && state.redo.changes > 0
  const verb = step?.kind !== "half" && step?.redo ? "Redo" : "Undo"
  return (
    <>
      <Button variant="ghost" size="xs" disabled={busy} onClick={() => void open()}>
        {busy && !step && <Spinner data-icon="inline-start" />}
        {redoOnly ? "Redo turn" : "Undo turn"}
      </Button>
      <AlertDialog open={step !== null} onOpenChange={(o) => !o && !busy && setStep(null)}>
        <AlertDialogContent>
          {step?.kind === "confirm" && (
            <>
              <AlertDialogHeader>
                <AlertDialogTitle>
                  {verb} turn {turn}?
                </AlertDialogTitle>
                <AlertDialogDescription>
                  {refused(step.plan, verb.toLowerCase()) ||
                    `${step.redo ? "This puts back" : "This reverts"} ${step.plan.display || "the turn's changes"}.`}
                  {!refused(step.plan, "") && step.migrations > 0 && " Table changes from migrations stay, since migrations are never undone."}
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel disabled={busy}>{refused(step.plan, "") ? "Close" : "Cancel"}</AlertDialogCancel>
                {!refused(step.plan, "") && (
                  <AlertDialogAction
                    disabled={busy}
                    onClick={(e) => {
                      e.preventDefault() // stays open for the conflicts
                      void run(step.redo, [])
                    }}
                  >
                    {busy && <Spinner data-icon="inline-start" />}
                    {verb} turn
                  </AlertDialogAction>
                )}
              </AlertDialogFooter>
            </>
          )}
          {step?.kind === "half" && (
            <>
              <AlertDialogHeader>
                <AlertDialogTitle>Turn {turn} is partly undone</AlertDialogTitle>
                <AlertDialogDescription>
                  {`Undone: ${step.redo.display || "some of its changes"}. Still in effect: ${step.undo.display || "the rest"}.`}
                  {(refused(step.undo, "undo") || refused(step.redo, "redo")) && ` ${refused(step.undo, "undo") || refused(step.redo, "redo")}`}
                  {!refused(step.undo, "") && step.migrations > 0 && " Table changes from migrations stay, since migrations are never undone."}
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel disabled={busy}>Cancel</AlertDialogCancel>
                <AlertDialogAction
                  variant="outline"
                  disabled={busy || !!refused(step.redo, "")}
                  onClick={(e) => {
                    e.preventDefault()
                    void run(true, [])
                  }}
                >
                  Redo the rest
                </AlertDialogAction>
                <AlertDialogAction
                  disabled={busy || !!refused(step.undo, "")}
                  onClick={(e) => {
                    e.preventDefault()
                    void run(false, [])
                  }}
                >
                  {busy && <Spinner data-icon="inline-start" />}
                  Undo the rest
                </AlertDialogAction>
              </AlertDialogFooter>
            </>
          )}
          {step?.kind === "conflicts" && (
            <>
              <AlertDialogHeader>
                <AlertDialogTitle>Later changes are in the way</AlertDialogTitle>
                <AlertDialogDescription>
                  Nothing was changed. Leave these as they are and {verb.toLowerCase()} the rest, or cancel.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <ul className="max-h-60 list-disc overflow-y-auto ps-5 text-sm">
                {step.conflicts.map((c, i) => (
                  <li key={i} dir="auto">
                    {conflictText(c)}
                  </li>
                ))}
              </ul>
              <AlertDialogFooter>
                <AlertDialogCancel disabled={busy}>Cancel</AlertDialogCancel>
                <AlertDialogAction
                  disabled={busy}
                  onClick={(e) => {
                    e.preventDefault()
                    void run(step.redo, skipAll(step.skip, step.conflicts))
                  }}
                >
                  {busy && <Spinner data-icon="inline-start" />}
                  Skip these, {verb.toLowerCase()} the rest
                </AlertDialogAction>
              </AlertDialogFooter>
            </>
          )}
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
