import * as React from "react"

import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { showError, toUIError } from "@/lib/errors"
import { useChats } from "@/state/chats"
import { useProjects } from "@/state/projects"
import { useUI } from "@/state/ui"
import { openChat } from "@/shell/open"

// NewProject asks for a name and opens the project's Mother chat (SPEC 3.9).
export function NewProject() {
  const open = useUI((s) => s.newProject)
  const set = useUI((s) => s.set)
  const [name, setName] = React.useState("")
  const [error, setError] = React.useState("")
  const [busy, setBusy] = React.useState(false)

  const close = (o: boolean) => {
    if (o) return
    set({ newProject: false })
    setName("")
    setError("")
  }

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      const op = await useProjects.getState().create(name)
      await useChats.getState().load(op.id)
      openChat(op.id, op.mother)
      close(false)
    } catch (err) {
      const ue = toUIError(err)
      if (ue.kind === "invalid") setError(ue.message)
      else showError(err)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent className="sm:max-w-sm">
        <form onSubmit={submit} className="flex flex-col gap-4">
          <DialogHeader>
            <DialogTitle>New project</DialogTitle>
            <DialogDescription>A project is one folder with its data, chats and files.</DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field data-invalid={error ? true : undefined}>
              <FieldLabel htmlFor="new-project-name">Name</FieldLabel>
              <Input
                id="new-project-name"
                value={name}
                onChange={(e) => {
                  setName(e.target.value)
                  setError("")
                }}
                dir="auto"
                autoFocus
                aria-invalid={error ? true : undefined}
              />
              {error && <FieldError>{error}</FieldError>}
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => close(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={busy || !name.trim()}>
              {busy && <Spinner data-icon="inline-start" />}
              Create
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
