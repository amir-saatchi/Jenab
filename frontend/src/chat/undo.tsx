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
import { conflictText, isRest, partly as partlyOf, skipAll } from "@/chat/undo-text"

// Step is the dialog open now: the confirmation with what the undo or
// redo reverts (partly: the turn is partly undone; rest: this is the way
// it still has to go), or the conflicts that stopped it.
type Step =
  | { kind: "confirm"; redo: boolean; partly: boolean; rest: boolean; plan: TurnPlan; migrations: number }
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
// stand in its way. Skipped items stay as they are: the turn is then
// partly undone, and the footer says so. After an undo it has *Undo the
// rest*, which tries the skipped items again, and *Redo turn*, which puts
// back only what was undone; after a redo, *Redo the rest* and *Undo
// turn*. A crash between the turn's databases leaves it partly undone
// too, the way its newest database went; with no way known, both are the
// rest. The chat then gets a notice, so the model knows; it arrives with
// the chat's events, and seen, the chat's message count, makes the footer
// ask again.
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

  // open confirms an undo, or with redo a redo; with neither, whichever
  // the turn has.
  const open = async (redo?: boolean) => {
    setBusy(true)
    try {
      const st: TurnUndoState = await ChatService.UndoState(project, chat, turn)
      setState(st)
      const p = partlyOf(st)
      const confirm = (redo: boolean, plan: TurnPlan, migrations: number) =>
        setStep({ kind: "confirm", redo, partly: p !== null, rest: isRest(p, redo), plan, migrations })
      if (redo !== true && st.undo.changes > 0) confirm(false, st.undo, st.migrations)
      else if (redo !== false && st.redo.changes > 0) confirm(true, st.redo, 0)
      else if (redo === undefined) toast("This turn changed nothing that can be undone.")
      else toast(redo ? "Nothing of this turn is undone now." : "Nothing of this turn is in effect now.")
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
      if (!r.done && !r.conflicts?.length && r.skipped > 0) toast(`Nothing was ${redo ? "redone" : "undone"}: the skipped items stay as they are.`)
    } catch (err) {
      setStep(null)
      showError(err)
    } finally {
      setBusy(false)
      ChatService.UndoState(project, chat, turn).then(setState, () => {})
    }
  }

  const partly = partlyOf(state)
  const redoOnly = state !== null && state.undo.changes === 0 && state.redo.changes > 0
  const verb = step?.redo ? "Redo" : "Undo"
  // A partly undone turn has a button each way: the rest first, the way
  // it still has to go, then the turn the other way; with no way, as a
  // crash leaves it, the rest both ways.
  const ways = partly ? (partly.rest === "redo" ? [true, false] : [false, true]) : []
  return (
    <>
      {partly && <span className="self-center">{partly.text}</span>}
      {ways.map((redo, i) => (
        <Button key={i} variant="ghost" size="xs" disabled={busy} onClick={() => void open(redo)}>
          {busy && !step && i === ways.length - 1 && <Spinner data-icon="inline-start" />}
          {`${redo ? "Redo" : "Undo"} ${isRest(partly, redo) ? "the rest" : "turn"}`}
        </Button>
      ))}
      {!partly && (
        <Button variant="ghost" size="xs" disabled={busy} onClick={() => void open()}>
          {busy && !step && <Spinner data-icon="inline-start" />}
          {redoOnly ? "Redo turn" : "Undo turn"}
        </Button>
      )}
      <AlertDialog open={step !== null} onOpenChange={(o) => !o && !busy && setStep(null)}>
        <AlertDialogContent>
          {step?.kind === "confirm" && (
            <>
              <AlertDialogHeader>
                <AlertDialogTitle>
                  {step.rest ? `${verb} the rest of turn ${turn}?` : `${verb} turn ${turn}?`}
                </AlertDialogTitle>
                <AlertDialogDescription>
                  {refused(step.plan, verb.toLowerCase()) ||
                    `${step.redo ? "This puts back" : "This reverts"} ${step.plan.display || "the turn's changes"}.`}
                  {!refused(step.plan, "") && step.partly && (step.rest ? (step.redo ? " The rest is in effect already." : " The rest is undone already.") : step.redo ? " The items still in effect stay." : " The items still undone stay.")}
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
                    {verb} {step.rest ? "the rest" : "turn"}
                  </AlertDialogAction>
                )}
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
