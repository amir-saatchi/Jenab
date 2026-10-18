import * as React from "react"
import { Dialogs, Events, System } from "@wailsio/runtime"
import { FileArchiveIcon, FolderIcon, TriangleAlertIcon } from "lucide-react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Field, FieldContent, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Progress } from "@/components/ui/progress"
import { Spinner } from "@/components/ui/spinner"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { num, size } from "@/dev/format"
import {
  ExportFormat,
  ExportStep,
  ProjectService,
  type ExportEstimate,
  type ExportProgress,
  type ExportResult,
} from "@/lib/api"
import { showError, toUIError } from "@/lib/errors"

// ExportProject is *Export* from the project's menu (SPEC 2.1): a copy of
// the project as it is now, as one zip file or a folder, while the
// project stays in use. It shows the size first, then the progress.
export function ExportProject({
  project,
  name,
  open,
  onOpenChange,
}: {
  project: string
  name: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const [format, setFormat] = React.useState<ExportFormat>(ExportFormat.ExportZip)
  const [approvals, setApprovals] = React.useState(false)
  const approvalsId = React.useId()
  const [estimate, setEstimate] = React.useState<ExportEstimate | null>(null)
  const [progress, setProgress] = React.useState<ExportProgress | null>(null)
  const [result, setResult] = React.useState<ExportResult | null>(null)
  const [error, setError] = React.useState("")
  const [running, setRunning] = React.useState(false)
  const [stopping, setStopping] = React.useState(false)
  // own is set while this dialog's own Export call runs; that call's
  // failure is shown from its error, not from the last event.
  const own = React.useRef(false)

  React.useEffect(() => {
    if (!open) return
    setEstimate(null)
    setProgress(null)
    setResult(null)
    setError("")
    setApprovals(false) // asked again for every export
    let live = true
    let ended = false
    // One export runs per project at a time, so the project tells its
    // events. The last one says how it ended.
    const off = Events.On("project:export", (e) => {
      const pr = e.data
      if (!live || pr.project !== project) return
      if (pr.step === ExportStep.StepDone && pr.result) {
        ended = true
        setResult(pr.result)
        setRunning(false)
        setStopping(false)
      } else if (pr.step === ExportStep.StepFailed) {
        ended = true
        if (own.current) return
        setError(pr.stopped ? stoppedText : `The export failed: ${pr.error}`)
        setRunning(false)
        setStopping(false)
      } else {
        setProgress(pr)
      }
    })
    // An export may still run from before, e.g. if the dialog went away
    // with the sidebar: follow it.
    ProjectService.ExportStatus(project)
      .then((pr) => {
        if (!live || !pr || ended || own.current) return
        setProgress(pr)
        setRunning(true)
      })
      .catch((err) => live && showError(err))
    ProjectService.ExportEstimate(project)
      .then((e) => live && setEstimate(e))
      .catch((err) => live && showError(err))
    return () => {
      live = false
      off()
    }
  }, [open, project])

  const start = async () => {
    setError("")
    const zip = format === ExportFormat.ExportZip
    let picked: string
    try {
      picked = await Dialogs.SaveFile({
        Title: "Export project",
        Filename: fileName(name, format),
        CanCreateDirectories: true,
        Filters: zip ? [{ DisplayName: "Zip file", Pattern: "*.zip" }] : undefined,
      })
    } catch (err) {
      showError(err)
      return
    }
    if (!picked) return
    const target = exportPath(picked, format)
    // Windows and macOS ask before a save dialog picks a file that is
    // there; GTK may not, so on Linux an existing file is refused.
    const replace = zip && target === picked && !System.IsLinux()
    setRunning(true)
    setProgress(null)
    own.current = true
    try {
      setResult(await ProjectService.Export(project, { target, format, replace, include_approvals: approvals }))
    } catch (err) {
      const ue = toUIError(err)
      // Stop makes the call fail as cancelled, after the backend has
      // removed what it wrote; so does Jenab shutting down.
      if (ue.kind === "closing") setError(stoppedText)
      else if (ue.kind === "invalid" || ue.kind === "busy") setError(ue.message)
      else showError(err)
    } finally {
      own.current = false
      setRunning(false)
      setStopping(false)
    }
  }

  const stop = () => {
    setStopping(true)
    ProjectService.StopExport(project).catch((err) => {
      setStopping(false)
      showError(err)
    })
  }

  return (
    <Dialog open={open} onOpenChange={(o) => !running && onOpenChange(o)}>
      <DialogContent className="sm:max-w-md" showCloseButton={!running}>
        <DialogHeader>
          <DialogTitle>Export project</DialogTitle>
          <DialogDescription>
            A copy of{" "}
            <span className="font-medium" dir="auto">
              {name}
            </span>{" "}
            as it is now: its data, chats and files. You can keep working while it runs.
          </DialogDescription>
        </DialogHeader>

        {result ? (
          <div className="flex flex-col gap-2 text-sm">
            <p>
              Exported {size(result.size)} in {seconds(result.ms)}.
            </p>
            <p className="font-mono text-xs break-all text-muted-foreground" dir="ltr">
              {result.path}
            </p>
            {result.missing > 0 && (
              <Alert>
                <TriangleAlertIcon className="text-tone-amber" />
                <AlertDescription>
                  {num(result.missing)} {result.missing === 1 ? "file was" : "files were"} missing from the project and
                  {result.missing === 1 ? " is" : " are"} not in the export.
                </AlertDescription>
              </Alert>
            )}
            <p className="text-muted-foreground">
              Copied into the projects folder, the export opens as a new project next to this one.
            </p>
          </div>
        ) : running ? (
          <div className="flex flex-col gap-2 text-sm">
            <div className="flex items-center justify-between gap-2">
              <span>{stopping ? "Stopping…" : stepText(progress?.step)}</span>
              <span className="text-muted-foreground tabular-nums" dir="ltr">
                {progress ? `${size(progress.done)} / ${size(progress.total)}` : ""}
              </span>
            </div>
            <Progress value={percent(progress)} aria-label="Export progress" />
          </div>
        ) : (
          <div className="flex flex-col gap-3 text-sm">
            <ToggleGroup
              type="single"
              value={format}
              onValueChange={(v) => v && setFormat(v as ExportFormat)}
              variant="outline"
              size="sm"
              aria-label="Export as"
            >
              <ToggleGroupItem value={ExportFormat.ExportZip}>
                <FileArchiveIcon />
                Zip file
              </ToggleGroupItem>
              <ToggleGroupItem value={ExportFormat.ExportFolder}>
                <FolderIcon />
                Folder
              </ToggleGroupItem>
            </ToggleGroup>
            <Field orientation="horizontal">
              <Checkbox id={approvalsId} checked={approvals} onCheckedChange={(c) => setApprovals(c === true)} />
              <FieldContent>
                <FieldLabel htmlFor={approvalsId}>Include approvals (for my own backup)</FieldLabel>
                <FieldDescription>
                  The hosts, scripts and tools you allowed, the approval level and the private-network exceptions.
                  Without them, the copy asks again like a new project.
                </FieldDescription>
              </FieldContent>
            </Field>
            <p className="text-muted-foreground">
              {estimate ? (
                <>
                  About {size(estimate.bytes)} before compression, with {num(estimate.objects)}{" "}
                  {estimate.objects === 1 ? "file" : "files"}.
                </>
              ) : (
                <Spinner />
              )}
            </p>
          </div>
        )}

        {error && (
          <Alert>
            <TriangleAlertIcon className="text-tone-amber" />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        <DialogFooter>
          {result ? (
            <Button onClick={() => onOpenChange(false)}>Done</Button>
          ) : running ? (
            <Button variant="outline" onClick={stop} disabled={stopping}>
              Stop
            </Button>
          ) : (
            <>
              <Button variant="outline" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button onClick={() => void start()} disabled={!estimate}>
                Export…
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

const stoppedText = "The export was stopped. Nothing was left behind."

// fileName is the save dialog's suggested name: the project's name
// without characters a file name can't hold.
export function fileName(project: string, format: ExportFormat) {
  const base = project.replace(/[\\/:*?"<>|\u0000-\u001f]+/g, "-").replace(/^[\s.-]+|[\s.-]+$/g, "") || "project"
  return format === ExportFormat.ExportZip ? `${base}.zip` : base
}

// exportPath is the path the export writes: a zip file gets .zip when the
// name has no such ending.
export function exportPath(picked: string, format: ExportFormat) {
  return format === ExportFormat.ExportZip && !/\.zip$/i.test(picked) ? `${picked}.zip` : picked
}

// stepText says what the export is copying.
export function stepText(step: ExportStep | undefined) {
  switch (step) {
    case ExportStep.StepProject:
      return "Copying the project's data…"
    case ExportStep.StepChats:
      return "Copying the chats…"
    case ExportStep.StepObjects:
      return "Copying the files…"
    case ExportStep.StepSkills:
      return "Copying the skills…"
    case ExportStep.StepDone:
      return "Finishing…"
    default:
      return "Starting…"
  }
}

// percent is the progress bar's value.
export function percent(p: ExportProgress | null) {
  if (!p || p.total <= 0) return 0
  return Math.min(100, (100 * p.done) / p.total)
}

function seconds(ms: number) {
  return ms < 1000 ? "under a second" : `${(ms / 1000).toFixed(1)} s`
}
