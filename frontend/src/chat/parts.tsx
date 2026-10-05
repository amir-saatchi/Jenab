import * as React from "react"
import { BrainIcon, CheckIcon, ChevronRightIcon, CopyIcon, SparklesIcon, TriangleAlertIcon, XIcon } from "lucide-react"

import { Bubble, BubbleContent } from "@/components/ui/bubble"
import { Button } from "@/components/ui/button"
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible"
import { Marker, MarkerContent, MarkerIcon } from "@/components/ui/marker"
import { Spinner } from "@/components/ui/spinner"
import { NoticeKind, type ChatNotice, type Part, type ToolCall, type ToolResult } from "@/lib/api"
import { cn } from "@/lib/utils"
import { Markdown } from "@/chat/markdown"

export function AgentText({ text, stopped }: { text: string; stopped?: boolean }) {
  return (
    <Bubble variant="ghost">
      <BubbleContent>
        <Markdown>{text}</Markdown>
        {stopped && <span className="text-xs text-muted-foreground">Stopped</span>}
      </BubbleContent>
    </Bubble>
  )
}

// Thinking is folded; a provider may send it redacted.
export function Thinking({ text, streaming }: { text: string; streaming?: boolean }) {
  return (
    <Collapsible className="flex flex-col gap-1.5 not-typeset">
      <CollapsibleTrigger asChild>
        <Button variant="ghost" size="xs" className="w-fit text-muted-foreground">
          <BrainIcon data-icon="inline-start" />
          <span className={cn(streaming && "shimmer")}>{streaming ? "Thinking" : text ? "Thought" : "Thinking hidden by the provider"}</span>
          <ChevronRightIcon data-icon="inline-end" className="transition-transform group-data-[state=open]/button:rotate-90" />
        </Button>
      </CollapsibleTrigger>
      {text && (
        <CollapsibleContent>
          <p dir="auto" className="ms-3 border-s ps-3 text-start text-sm whitespace-pre-wrap text-muted-foreground">
            {text}
          </p>
        </CollapsibleContent>
      )}
    </Collapsible>
  )
}

// firstLine is a short preview of s.
function firstLine(s: string, max = 120) {
  const line = s.trim().split("\n", 1)[0] ?? ""
  return line.length > max ? line.slice(0, max - 1) + "…" : line
}

function argsText(args: unknown): string {
  try {
    const s = JSON.stringify(typeof args === "string" ? JSON.parse(args) : args, null, 2)
    return s.length > 4000 ? s.slice(0, 4000) + "\n…" : s
  } catch {
    return String(args)
  }
}

// argsPreview is the call's arguments in one line, for its chip.
function argsPreview(args: unknown): string {
  try {
    const v = typeof args === "string" ? JSON.parse(args) : args
    if (v && typeof v === "object") {
      return Object.values(v as Record<string, unknown>)
        .filter((x) => typeof x === "string" || typeof x === "number")
        .map(String)
        .join(" · ")
    }
  } catch {
    // shown in full when unfolded
  }
  return ""
}

// ToolChip is a tool call with its result: a one-line preview, unfolded
// to the arguments and the output.
export function ToolChip({ call, result, running }: { call: ToolCall; result?: ToolResult; running: boolean }) {
  const state = result ? (result.is_error ? "failed" : "done") : running ? "running" : "failed"
  const detail = result ? firstLine(result.text) : running ? argsPreview(call.args) || "running" : "not run"
  return (
    <Collapsible className="flex flex-col gap-1.5 not-typeset">
      <CollapsibleTrigger asChild>
        <Button variant="outline" size="xs" className="w-fit max-w-full justify-start font-normal text-muted-foreground">
          {state === "done" && <CheckIcon data-icon="inline-start" className="text-tone-green" />}
          {state === "running" && <Spinner data-icon="inline-start" />}
          {state === "failed" && <XIcon data-icon="inline-start" className="text-tone-red" />}
          <span className="font-mono text-foreground">{call.name}</span>
          <span dir="auto" className={cn("truncate", state === "running" && "shimmer")}>
            {detail}
          </span>
          <ChevronRightIcon data-icon="inline-end" className="transition-transform group-data-[state=open]/button:rotate-90" />
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="ms-3 flex flex-col gap-1.5 rounded-lg border bg-muted/40 p-2.5 font-mono text-xs">
          <ToolDetail label="input">
            <pre className="whitespace-pre-wrap">{argsText(call.args)}</pre>
          </ToolDetail>
          {result && (
            <ToolDetail label={result.is_error ? "error" : "output"}>
              <pre dir="auto" className="text-start whitespace-pre-wrap">
                {result.text}
              </pre>
              {result.ref && <span className="text-muted-foreground">The full output is stored as {result.ref}.</span>}
            </ToolDetail>
          )}
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}

function ToolDetail({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex gap-3">
      <span className="w-14 shrink-0 text-muted-foreground">{label}</span>
      <div className="flex min-w-0 flex-1 flex-col gap-1 break-words">{children}</div>
    </div>
  )
}

export function SkillChip({ text }: { text: string }) {
  return (
    <Button variant="outline" size="xs" className="pointer-events-none w-fit font-normal text-muted-foreground not-typeset">
      <SparklesIcon data-icon="inline-start" />
      <span dir="auto">{text}</span>
    </Button>
  )
}

// ImagePart is a picture from the bucket, loaded from /objects (Q31).
export function ImagePart({ project, part }: { project: string; part: Part }) {
  const img = part.image
  if (!img) return null
  return (
    <img
      src={`/objects/${project}/${img.ref}`}
      alt={img.alt ?? ""}
      loading="lazy"
      className="max-h-80 w-fit max-w-full rounded-lg border object-contain"
    />
  )
}

// noticeText is a notice as the user reads it: the app writes them for the
// model, in brackets.
export function noticeText(text: string) {
  return text.replace(/^\[(.*)\]$/s, "$1")
}

// NoticeLine is a note from the app between messages. A long one wraps
// instead of sitting between two rules.
export function NoticeLine({ notice, time }: { notice: ChatNotice; time?: string }) {
  const warn =
    notice.kind === NoticeKind.NoticeAnswerCut ||
    notice.kind === NoticeKind.NoticeEarlyStop ||
    notice.kind === NoticeKind.NoticeTurnFailed
  const text = noticeText(notice.text) + (time ? ` · ${time}` : "")
  return (
    <Marker variant={text.length > 70 ? "default" : "separator"}>
      {warn && (
        <MarkerIcon>
          <TriangleAlertIcon className="text-tone-amber" />
        </MarkerIcon>
      )}
      <MarkerContent dir="auto">{text}</MarkerContent>
    </Marker>
  )
}

export function CopyButton({ text }: { text: string }) {
  const [done, setDone] = React.useState(false)
  return (
    <Button
      variant="ghost"
      size="xs"
      className="text-muted-foreground"
      onClick={() =>
        void navigator.clipboard.writeText(text).then(() => {
          setDone(true)
          setTimeout(() => setDone(false), 1500)
        })
      }
    >
      {done ? <CheckIcon data-icon="inline-start" /> : <CopyIcon data-icon="inline-start" />}
      {done ? "Copied" : "Copy"}
    </Button>
  )
}
