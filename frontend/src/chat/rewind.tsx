import * as React from "react"
import { PencilIcon, UndoIcon } from "lucide-react"

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
import { Marker, MarkerContent } from "@/components/ui/marker"
import { Spinner } from "@/components/ui/spinner"
import { ChatService, type Message, type RewindInfo, type RewindState } from "@/lib/api"
import { showError } from "@/lib/errors"
import { markText, rewindLines, turnsText } from "@/chat/rewind-text"
import { UndoTurn } from "@/chat/undo"
import { messageText, useQuotes } from "@/state/quote"
import { useFill, useThreads, type Thread } from "@/state/thread"

// Rewind (TASK-018): *Rewind to here* and *Edit and resend* on the
// message that starts a turn, between turns. The later turns leave the
// chat; what they changed in the project stays, and the dialog says so.
// The rewind's mark offers *Undo rewind* while the chat has not gone on,
// and Undo for the changes of the turns it took out.

// RewindActions are the two actions on a user message that starts turn.
// later: the chat has turns after it, so *Rewind to here* has work.
export function RewindActions({ thread, message, later }: { thread: Thread; message: Message; later: boolean }) {
  // ask stays while the dialog closes, so its text doesn't go blank.
  const [ask, setAsk] = React.useState<{ edit: boolean; state: RewindState } | null>(null)
  const [open, setOpen] = React.useState(false)
  const [busy, setBusy] = React.useState<"" | "rewind" | "edit">("")
  const project = thread.project
  const chat = thread.chat.id

  // confirm asks what the rewind would do, for its dialog. *Edit and
  // resend* takes the chat back to before the message's turn.
  const confirm = async (edit: boolean) => {
    setBusy(edit ? "edit" : "rewind")
    try {
      const state = await ChatService.RewindState(project, chat, edit ? message.turn - 1 : message.turn)
      setAsk({ edit, state })
      setOpen(true)
    } catch (err) {
      showError(err)
    } finally {
      setBusy("")
    }
  }

  const run = async () => {
    if (!ask) return
    const { edit, state } = ask
    setBusy(edit ? "edit" : "rewind")
    try {
      await ChatService.Rewind(project, chat, state.turn)
      if (edit) {
        useFill.getState().fill(chat, messageText(message))
        // A quote of a turn that stays goes back with the message.
        const q = message.parts?.find((p) => p.quote)?.quote
        useQuotes.getState().set(chat, q && q.turn <= state.turn ? q : null)
      }
      setOpen(false)
      await useThreads.getState().reload(chat)
    } catch (err) {
      setOpen(false)
      showError(err)
    } finally {
      setBusy("")
    }
  }

  return (
    <>
      {later && (
        <Button variant="ghost" size="xs" disabled={busy !== ""} onClick={() => void confirm(false)}>
          {busy === "rewind" && !open ? <Spinner data-icon="inline-start" /> : <UndoIcon data-icon="inline-start" />}
          Rewind to here
        </Button>
      )}
      <Button variant="ghost" size="xs" disabled={busy !== ""} onClick={() => void confirm(true)}>
        {busy === "edit" && !open ? <Spinner data-icon="inline-start" /> : <PencilIcon data-icon="inline-start" />}
        Edit and resend
      </Button>
      <AlertDialog open={open} onOpenChange={(o) => !o && busy === "" && setOpen(false)}>
        <AlertDialogContent>
          {ask && (
            <>
              <AlertDialogHeader>
                <AlertDialogTitle>
                  {ask.edit ? "Edit and resend?" : `Take ${turnsText(ask.state.turn + 1, ask.state.last_turn)} out?`}
                </AlertDialogTitle>
                <AlertDialogDescription asChild>
                  <div className="grid gap-2">
                    {rewindLines(ask.state, ask.edit).map((l, i) => (
                      <p key={i} dir="auto">
                        {l}
                      </p>
                    ))}
                  </div>
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel disabled={busy !== ""}>Cancel</AlertDialogCancel>
                <AlertDialogAction
                  disabled={busy !== ""}
                  onClick={(e) => {
                    e.preventDefault() // closes when the rewind is done
                    void run()
                  }}
                >
                  {busy !== "" && <Spinner data-icon="inline-start" />}
                  {ask.edit ? "Edit and resend" : "Rewind"}
                </AlertDialogAction>
              </AlertDialogFooter>
            </>
          )}
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

function clock(at: string) {
  return new Date(at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
}

// RewindMark marks where a rewind took turns out, with *Undo rewind*
// while it can be undone, and Undo for the changes of the turns it took
// out while there are any. running: the chat is in a turn, when neither
// can run.
export function RewindMark({ thread, rewind, running }: { thread: Thread; rewind: RewindInfo; running: boolean }) {
  const [busy, setBusy] = React.useState(false)
  const chat = thread.chat.id
  const undo = async () => {
    setBusy(true)
    try {
      await ChatService.UndoRewind(thread.project, chat, rewind.id)
      await useThreads.getState().reload(chat)
    } catch (err) {
      showError(err)
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="grid gap-1">
      <Marker variant="separator">
        <MarkerContent>{`${markText(rewind)} · ${clock(rewind.at)}`}</MarkerContent>
      </Marker>
      {!running && (
        <div className="flex justify-center gap-1 text-xs text-muted-foreground">
          {rewind.undoable && (
            <Button variant="ghost" size="xs" disabled={busy} onClick={() => void undo()}>
              {busy && <Spinner data-icon="inline-start" />}
              Undo rewind
            </Button>
          )}
          <UndoTurn
            project={thread.project}
            chat={chat}
            turn={0}
            seen={thread.messages.length}
            rewind={rewind.id}
            label={turnsText(rewind.turn + 1, rewind.last_turn)}
          />
        </div>
      )}
    </div>
  )
}
