import * as React from "react"
import { Dialogs } from "@wailsio/runtime"
import { FolderInputIcon, FolderOpenIcon, InfoIcon, TriangleAlertIcon } from "lucide-react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field"
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from "@/components/ui/input-group"
import { Spinner } from "@/components/ui/spinner"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { SettingsService, type FolderWarning as Warning, type MoveResult } from "@/lib/api"
import { showError, toUIError } from "@/lib/errors"
import { useSettings } from "@/state/settings"
import { FolderWarning } from "@/shell/folder-warning"
import { levels } from "@/shell/project-settings"
import { FactRow, SettingsSection } from "@/shell/settings-layout"

// GeneralSection is *Settings → General* (SPEC 3.9): the data folder and
// the approval level for new projects.
export function GeneralSection() {
  const view = useSettings((s) => s.view)
  const started = useSettings((s) => s.started)
  const edit = useSettings((s) => s.edit)
  const [choosing, setChoosing] = React.useState(false)
  const saved = view?.settings.data_folder || started?.default_data_folder || ""
  const pending = !!started && !samePath(saved, started.data_folder)
  const moved = started?.moved

  return (
    <SettingsSection title="General">
      <FactRow
        label="Data folder"
        action={
          <Button variant="outline" size="sm" onClick={() => setChoosing(true)} disabled={!started}>
            Change…
          </Button>
        }
      >
        <span className="block truncate font-mono text-xs" dir="ltr" title={started?.data_folder}>
          {started?.data_folder}
        </span>
      </FactRow>
      {pending && (
        <Alert>
          <InfoIcon />
          <AlertDescription>
            <span>
              At the next start, the projects move to{" "}
              <span className="font-mono text-xs break-all" dir="ltr">
                {saved}
              </span>
              . Settings and logs stay where they are.
            </span>
          </AlertDescription>
        </Alert>
      )}
      {moved && (moved.moved > 0 || (moved.failed ?? []).length > 0) && <MovedNote moved={moved} />}
      <FolderWarning />
      <FactRow label="Approval level">
        <ToggleGroup
          type="single"
          variant="outline"
          size="sm"
          value={view?.settings.approvals.default_level ?? ""}
          disabled={!view}
          onValueChange={(v) =>
            v &&
            edit((s) => {
              s.approvals.default_level = v
            }).catch(showError)
          }
        >
          {levels.map((l) => (
            <ToggleGroupItem key={l.value} value={l.value}>
              {l.label}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
      </FactRow>
      <p className="text-xs text-muted-foreground">
        The approval level for new projects. Each project can change its own in its settings.
      </p>
      {started && (
        <ChooseFolder
          open={choosing}
          onOpenChange={setChoosing}
          current={saved}
          fallback={started.default_data_folder}
        />
      )}
    </SettingsSection>
  )
}

function MovedNote({ moved }: { moved: MoveResult }) {
  const n = moved.moved
  const failed = moved.failed ?? []
  return (
    <Alert>
      {failed.length > 0 ? <TriangleAlertIcon className="text-tone-amber" /> : <InfoIcon />}
      <AlertDescription>
        {n > 0 && <span>{n === 1 ? "1 project was" : `${n} projects were`} moved to the new data folder at this start. </span>}
        {failed.length > 0 && (
          <span>
            Not moved, still opened from the old folder: <span dir="auto">{failed.join(", ")}</span>. Jenab tries
            again at the next start; the log has the reason.
          </span>
        )}
      </AlertDescription>
    </Alert>
  )
}

// ChooseFolder picks a new data folder. It is saved now and used at the
// next start, when the projects move (SPEC 2.1).
function ChooseFolder({
  open,
  onOpenChange,
  current,
  fallback,
}: {
  open: boolean
  onOpenChange: (o: boolean) => void
  current: string
  fallback: string
}) {
  const edit = useSettings((s) => s.edit)
  const [dir, setDir] = React.useState(current)
  const [warning, setWarning] = React.useState<Warning | null>(null)
  const [error, setError] = React.useState("")
  const [busy, setBusy] = React.useState(false)

  React.useEffect(() => {
    if (open) {
      setDir(current)
      setError("")
      setWarning(null)
    }
  }, [open, current])

  // The warning follows the typed path, checked once typing pauses.
  React.useEffect(() => {
    const d = dir.trim()
    if (!open || !d) return
    let live = true
    const t = setTimeout(() => {
      SettingsService.CheckFolder(d)
        .then((w) => live && (setWarning(w), setError("")))
        .catch((err) => live && (setWarning(null), setError(toUIError(err).message)))
    }, 300)
    return () => {
      live = false
      clearTimeout(t)
    }
  }, [dir, open])

  const browse = async () => {
    try {
      const picked = await Dialogs.OpenFile({
        Title: "Choose the data folder",
        CanChooseDirectories: true,
        CanChooseFiles: false,
        CanCreateDirectories: true,
        Directory: dir || fallback,
      })
      if (picked) setDir(picked)
    } catch (err) {
      showError(err)
    }
  }

  const save = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      const d = dir.trim()
      await edit((s) => {
        s.data_folder = samePath(d, fallback) ? "" : d
      })
      onOpenChange(false)
    } catch (err) {
      setError(toUIError(err).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <form onSubmit={save} className="flex flex-col gap-4">
          <DialogHeader>
            <DialogTitle>Data folder</DialogTitle>
            <DialogDescription>
              Where projects are kept. The change applies at the next start, when the projects move there.
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field data-invalid={error ? true : undefined}>
              <FieldLabel htmlFor="data-folder">Folder</FieldLabel>
              <InputGroup>
                <InputGroupAddon>
                  <FolderOpenIcon />
                </InputGroupAddon>
                <InputGroupInput
                  id="data-folder"
                  dir="ltr"
                  className="font-mono text-xs"
                  value={dir}
                  aria-invalid={error ? true : undefined}
                  onChange={(e) => setDir(e.target.value)}
                />
                <InputGroupAddon align="inline-end">
                  <InputGroupButton onClick={browse}>
                    <FolderInputIcon data-icon="inline-start" />
                    Browse…
                  </InputGroupButton>
                </InputGroupAddon>
              </InputGroup>
              {error ? (
                <FieldError>{error}</FieldError>
              ) : (
                <FieldDescription>
                  Use a local disk, not a network drive or a synced folder. The default is{" "}
                  <span className="font-mono" dir="ltr">
                    {fallback}
                  </span>
                  .
                </FieldDescription>
              )}
            </Field>
          </FieldGroup>
          {warning && (
            <Alert>
              <TriangleAlertIcon className="text-tone-amber" />
              <AlertDescription>{warning.text}</AlertDescription>
            </Alert>
          )}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={busy || !dir.trim() || !!error || samePath(dir.trim(), current)}>
              {busy && <Spinner data-icon="inline-start" />}
              Save
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

// samePath compares folders the way Windows and macOS do: without case
// and a trailing separator.
function samePath(a: string, b: string) {
  const norm = (s: string) => s.replace(/[\\/]+$/, "").toLowerCase()
  return norm(a) === norm(b)
}
