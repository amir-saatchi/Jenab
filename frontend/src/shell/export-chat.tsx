import * as React from "react"
import { Dialogs } from "@wailsio/runtime"
import { FileJsonIcon, FileTextIcon, TriangleAlertIcon } from "lucide-react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Spinner } from "@/components/ui/spinner"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { num, size } from "@/dev/format"
import { ChatExportFormat, ChatService, type ChatExportResult } from "@/lib/api"
import { showError, toUIError } from "@/lib/errors"
import { safeName } from "@/shell/export-project"

// ExportChat is *Export chat…* from the chat header's menu and the chat
// row's menu (SPEC 2.3, 5.12): one chat as a Markdown file to read, or a
// JSON file that keeps every part. Secrets are replaced in both.
export function ExportChat({
  project,
  chat,
  title,
  open,
  onOpenChange,
}: {
  project: string
  chat: string
  title: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const [format, setFormat] = React.useState<ChatExportFormat>(ChatExportFormat.FormatMarkdown)
  const [running, setRunning] = React.useState(false)
  const [result, setResult] = React.useState<ChatExportResult | null>(null)
  const [error, setError] = React.useState("")

  React.useEffect(() => {
    if (!open) return
    setResult(null)
    setError("")
  }, [open])

  const start = async () => {
    setError("")
    const md = format === ChatExportFormat.FormatMarkdown
    let picked: string
    try {
      picked = await Dialogs.SaveFile({
        Title: "Export chat",
        Filename: chatFileName(title, format),
        CanCreateDirectories: true,
        Filters: [md ? { DisplayName: "Markdown", Pattern: "*.md" } : { DisplayName: "JSON", Pattern: "*.json" }],
      })
    } catch (err) {
      showError(err)
      return
    }
    if (!picked) return
    setRunning(true)
    try {
      setResult(await ChatService.Export(project, chat, { target: chatExportPath(picked, format), format }))
    } catch (err) {
      const ue = toUIError(err)
      if (ue.kind === "invalid" || ue.kind === "not_found") setError(ue.message)
      else showError(err)
    } finally {
      setRunning(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={(o) => !running && onOpenChange(o)}>
      <DialogContent className="sm:max-w-md" showCloseButton={!running}>
        <DialogHeader>
          <DialogTitle>Export chat</DialogTitle>
          <DialogDescription>
            Save{" "}
            <span className="font-medium" dir="auto">
              {title || "New chat"}
            </span>{" "}
            as a file. Keys are replaced with their names.
          </DialogDescription>
        </DialogHeader>

        {result ? (
          <div className="flex flex-col gap-2 text-sm">
            <p>
              Exported {num(result.messages)} {result.messages === 1 ? "message" : "messages"}, {size(result.size)}.
            </p>
            <p className="font-mono text-xs break-all text-muted-foreground" dir="ltr">
              {result.path}
            </p>
          </div>
        ) : (
          <div className="flex flex-col gap-3 text-sm">
            <ToggleGroup
              type="single"
              value={format}
              onValueChange={(v) => v && setFormat(v as ChatExportFormat)}
              variant="outline"
              size="sm"
              aria-label="Export as"
              disabled={running}
            >
              <ToggleGroupItem value={ChatExportFormat.FormatMarkdown}>
                <FileTextIcon />
                Markdown
              </ToggleGroupItem>
              <ToggleGroupItem value={ChatExportFormat.FormatJSON}>
                <FileJsonIcon />
                JSON
              </ToggleGroupItem>
            </ToggleGroup>
            <p className="text-muted-foreground">{formatText(format)}</p>
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
          ) : (
            <>
              <Button variant="outline" onClick={() => onOpenChange(false)} disabled={running}>
                Cancel
              </Button>
              <Button onClick={() => void start()} disabled={running}>
                {running && <Spinner data-icon="inline-start" />}
                Export…
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// formatText says what each format holds.
export function formatText(format: ChatExportFormat) {
  return format === ChatExportFormat.FormatJSON
    ? "Every message part as stored, thinking included, for keeping everything."
    : "The messages to read: tool calls as one line each, without thinking or full tool results."
}

// chatFileName is the save dialog's suggested name: the chat's title as a
// file name, with the format's ending.
export function chatFileName(title: string, format: ChatExportFormat) {
  return safeName(title, "chat") + ending(format)
}

// chatExportPath is the path the export writes: the format's ending is
// added when the name has none.
export function chatExportPath(picked: string, format: ChatExportFormat) {
  const end = ending(format)
  return picked.toLowerCase().endsWith(end) ? picked : picked + end
}

function ending(format: ChatExportFormat) {
  return format === ChatExportFormat.FormatJSON ? ".json" : ".md"
}
