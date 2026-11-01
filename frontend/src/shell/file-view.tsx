import * as React from "react"
import { ExternalLinkIcon, FileIcon, LinkIcon, Trash2Icon, TriangleAlertIcon } from "lucide-react"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { Item, ItemContent, ItemDescription, ItemGroup, ItemTitle } from "@/components/ui/item"
import { Separator } from "@/components/ui/separator"
import { Skeleton } from "@/components/ui/skeleton"
import { BucketService, type FileObject, type ObjectVersion } from "@/lib/api"
import { toUIError } from "@/lib/errors"
import { fileKind, formatSize, nameOf, parentOf, typeName, whoText } from "@/lib/files"
import { Markdown } from "@/chat/markdown"
import { useFiles } from "@/state/files"
import { DockControls, IconButton } from "@/shell/dock"
import { copyLink, deleteFile, openWithSystem } from "@/shell/open-file"

// TEXT_LIMIT is how much of a text file is shown; the rest opens with the
// system app.
export const TEXT_LIMIT = 1 << 20

export function objectURL(project: string, o: Pick<FileObject, "key" | "sha256">) {
  // The query changes with the content, so a new version isn't taken from
  // the cache; the route ignores it.
  return `/objects/${project}/${o.key}?v=${o.sha256.slice(0, 12)}`
}

// FileView is a file opened in the main area, with the chat in the dock
// (SPEC 5.12): text and Markdown as in the chat, pictures, and for other
// types their name, type and size with *Open with the system app*. Its
// versions are listed under it.
export function FileView({ project, chat, fileKey }: { project: string; chat: string; fileKey: string }) {
  const rev = useFiles((s) => s.rev[project] ?? 0)
  const [obj, setObj] = React.useState<FileObject | null>(null)
  const [error, setError] = React.useState<{ kind: string; message: string } | null>(null)
  React.useEffect(() => {
    let live = true
    BucketService.Head(project, fileKey).then(
      (o) => {
        if (!live) return
        setObj(o)
        setError(null)
      },
      (err) => live && setError(toUIError(err)),
    )
    return () => {
      live = false
    }
  }, [project, fileKey, rev])

  const folder = parentOf(fileKey)
  return (
    <div className="flex h-full min-h-0 flex-col">
      <header className="flex h-14 shrink-0 items-center gap-2 border-b px-3">
        <FileIcon className="size-4 shrink-0 text-muted-foreground" />
        <div className="grid min-w-0 leading-tight">
          <span className="truncate font-medium">{nameOf(fileKey)}</span>
          <span className="truncate text-xs text-muted-foreground" dir="auto">
            {obj?.title || (folder ? <bdi>{folder}</bdi> : "Files")}
          </span>
        </div>
        <div className="ms-auto flex shrink-0 items-center gap-1">
          {obj && (
            <>
              <IconButton label="Copy link" onClick={() => copyLink(project, fileKey)}>
                <LinkIcon />
              </IconButton>
              {obj.opens && (
                <IconButton label="Open with the system app" onClick={() => void openWithSystem(project, fileKey)}>
                  <ExternalLinkIcon />
                </IconButton>
              )}
              <IconButton label="Delete" onClick={() => void deleteFile(project, fileKey)}>
                <Trash2Icon />
              </IconButton>
              <Separator orientation="vertical" className="mx-1 h-5" />
            </>
          )}
          <DockControls project={project} chat={chat} closeLabel="Close the file" />
        </div>
      </header>
      <div className="min-h-0 flex-1 overflow-auto">
        <div className="mx-auto flex w-full max-w-4xl flex-col gap-6 p-6">
          {error ? (
            <Empty>
              <EmptyHeader>
                <EmptyTitle>{error.kind === "not_found" ? "This file no longer exists" : "This file can't be shown"}</EmptyTitle>
                <EmptyDescription>{error.kind === "not_found" ? "It was deleted or moved." : error.message}</EmptyDescription>
              </EmptyHeader>
            </Empty>
          ) : !obj ? (
            <div className="flex flex-col gap-3">
              <Skeleton className="h-4 w-2/3" />
              <Skeleton className="h-4 w-1/2" />
              <Skeleton className="h-4 w-3/5" />
            </div>
          ) : (
            <>
              <Content project={project} obj={obj} />
              <Versions project={project} obj={obj} />
            </>
          )}
        </div>
      </div>
    </div>
  )
}

function Content({ project, obj }: { project: string; obj: FileObject }) {
  const kind = fileKind(obj.mime)
  const src = objectURL(project, obj)
  switch (kind) {
    case "markdown":
    case "text":
      return <TextContent src={src} size={obj.size} markdown={kind === "markdown"} />
    case "image":
      return <img src={src} alt={obj.title || nameOf(obj.key)} className="max-h-[70vh] w-fit max-w-full rounded-lg border object-contain" />
    case "audio":
      return <audio src={src} controls className="w-full" />
    case "video":
      return <video src={src} controls className="max-h-[70vh] w-full rounded-lg border" />
  }
  return <OtherContent project={project} obj={obj} />
}

// TextContent loads the first TEXT_LIMIT bytes of a text file. A cut in
// the middle of a character shows as one replacement character.
function TextContent({ src, size, markdown }: { src: string; size: number; markdown: boolean }) {
  const [text, setText] = React.useState<string | null>(null)
  const [failed, setFailed] = React.useState("")
  React.useEffect(() => {
    let live = true
    setText(null)
    fetch(src, { headers: size > TEXT_LIMIT ? { Range: `bytes=0-${TEXT_LIMIT - 1}` } : {} })
      .then((r) => {
        if (!r.ok) throw new Error(r.status === 404 ? "The file no longer exists." : `The file couldn't be read (${r.status}).`)
        return r.text()
      })
      .then((t) => live && setText(t), (err: Error) => live && setFailed(err.message))
    return () => {
      live = false
    }
  }, [src, size])
  if (failed) {
    return (
      <Alert variant="destructive">
        <TriangleAlertIcon />
        <AlertTitle>Can't show this file</AlertTitle>
        <AlertDescription>{failed}</AlertDescription>
      </Alert>
    )
  }
  if (text === null) return <Skeleton className="h-24 w-full" />
  return (
    <div className="flex flex-col gap-3">
      {size > TEXT_LIMIT && (
        <Alert>
          <TriangleAlertIcon />
          <AlertTitle>Only the first {formatSize(TEXT_LIMIT)} is shown</AlertTitle>
          <AlertDescription>The file is {formatSize(size)}. Open it with the system app to see all of it.</AlertDescription>
        </Alert>
      )}
      {markdown ? (
        <Markdown preset="page">{text}</Markdown>
      ) : (
        <pre dir="auto" className="overflow-x-auto rounded-lg border bg-muted/40 p-4 text-start font-mono text-sm whitespace-pre-wrap break-words">
          {text}
        </pre>
      )}
    </div>
  )
}

function OtherContent({ project, obj }: { project: string; obj: FileObject }) {
  return (
    <Empty className="border">
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <FileIcon />
        </EmptyMedia>
        <EmptyTitle>{nameOf(obj.key)}</EmptyTitle>
        <EmptyDescription>
          {typeName(obj.mime)} · {formatSize(obj.size)}
        </EmptyDescription>
      </EmptyHeader>
      <EmptyContent>
        {obj.opens ? (
          <Button variant="outline" onClick={() => void openWithSystem(project, obj.key)}>
            <ExternalLinkIcon data-icon="inline-start" />
            Open with the system app
          </Button>
        ) : (
          <p className="text-sm text-muted-foreground">
            This type isn't opened outside the app, since it could run a program.
          </p>
        )}
      </EmptyContent>
    </Empty>
  )
}

// Versions lists the file's versions, the newest first (SPEC 4.3).
function Versions({ project, obj }: { project: string; obj: FileObject }) {
  const [vs, setVs] = React.useState<ObjectVersion[] | null>(null)
  React.useEffect(() => {
    let live = true
    BucketService.Versions(project, obj.key).then(
      (v) => live && setVs(v ?? []),
      () => live && setVs([]),
    )
    return () => {
      live = false
    }
  }, [project, obj.key, obj.version, obj.sha256])
  if (!vs || vs.length === 0) return null
  return (
    <section className="flex flex-col gap-2">
      <Separator />
      <h2 className="text-sm font-medium">Versions</h2>
      <ItemGroup>
        {vs.map((v) => (
          <Item key={v.version} size="sm" variant="outline">
            <ItemContent>
              <ItemTitle>
                Version {v.version}
                {v.version === obj.version && <Badge variant="secondary">Current</Badge>}
              </ItemTitle>
              <ItemDescription>
                {formatSize(v.size)} · by {whoText(v.created_by)} · {new Date(v.created_at).toLocaleString([], { dateStyle: "medium", timeStyle: "short" })}
              </ItemDescription>
            </ItemContent>
          </Item>
        ))}
      </ItemGroup>
    </section>
  )
}
