import * as React from "react"
import {
  CheckIcon,
  ChevronDownIcon,
  ClockIcon,
  CopyIcon,
  DatabaseIcon,
  EyeIcon,
  FlaskConicalIcon,
  GlobeIcon,
  PauseIcon,
  PlayIcon,
  RepeatIcon,
  TriangleAlertIcon,
  Undo2Icon,
  XIcon,
  FileCodeIcon,
} from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { cn } from "@/lib/utils"

// Pipeline page (SPEC 6.10). Read-only: changes go through the agent.

export type RunStatus = "ok" | "partial" | "failed"

const tone: Record<RunStatus, string> = {
  ok: "bg-tone-green-soft text-tone-green",
  partial: "bg-tone-amber-soft text-tone-amber",
  failed: "bg-tone-red-soft text-tone-red",
}

export function StatusBadge({ status }: { status: RunStatus }) {
  return <Badge className={tone[status]}>{status}</Badge>
}

export function PipelineHeader({ view }: { view: "flow" | "steps" }) {
  return (
    <header className="flex flex-col gap-3 px-6 pt-5 pb-4">
      <div className="flex items-start gap-3">
        <div className="grid min-w-0 flex-1 gap-0.5">
          <h1 className="font-mono text-xl font-semibold tracking-tight">daily_btc</h1>
          <p className="text-sm text-muted-foreground">
            Pipeline · every day at 08:00 (Europe/Berlin) · next run tomorrow 08:00
          </p>
        </div>
        <ToggleGroup type="single" value={view} variant="outline" size="sm">
          <ToggleGroupItem value="flow">Flow</ToggleGroupItem>
          <ToggleGroupItem value="steps">Steps</ToggleGroupItem>
        </ToggleGroup>
        <Button variant="ghost" size="icon-sm" aria-label="Close">
          <XIcon />
        </Button>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <Button size="sm">
          <PlayIcon data-icon="inline-start" />
          Run now
        </Button>
        <Button variant="outline" size="sm">
          <PauseIcon data-icon="inline-start" />
          Pause schedule
        </Button>
        <Button variant="outline" size="sm">
          <FlaskConicalIcon data-icon="inline-start" />
          Dry run
        </Button>
        <Button variant="outline" size="sm">
          <FileCodeIcon data-icon="inline-start" />
          View YAML
        </Button>
      </div>
    </header>
  )
}

export const runs: { started: string; trigger: string; status: RunStatus; rows: number; time: string }[] = [
  { started: "Today 08:00", trigger: "schedule", status: "ok", rows: 6, time: "4.1 s" },
  { started: "Yesterday 09:12", trigger: "catch-up", status: "ok", rows: 6, time: "3.8 s" },
  { started: "27 Sep 14:30", trigger: "chat", status: "partial", rows: 3, time: "6.0 s" },
  { started: "26 Sep 08:00", trigger: "schedule", status: "failed", rows: 0, time: "2.2 s" },
  { started: "25 Sep 08:00", trigger: "schedule", status: "ok", rows: 5, time: "3.9 s" },
]

export function RunsTable({ compact }: { compact?: boolean }) {
  const list = compact ? runs.slice(0, 4) : runs
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Started</TableHead>
          <TableHead>Trigger</TableHead>
          <TableHead>Status</TableHead>
          {!compact && <TableHead className="text-end">Rows</TableHead>}
          <TableHead className="text-end">Time</TableHead>
          {!compact && <TableHead className="w-24" />}
        </TableRow>
      </TableHeader>
      <TableBody>
        {list.map((r, i) => (
          <TableRow key={r.started} data-state={i === 0 ? "selected" : undefined}>
            <TableCell>{r.started}</TableCell>
            <TableCell className="text-muted-foreground">{r.trigger}</TableCell>
            <TableCell>
              <StatusBadge status={r.status} />
            </TableCell>
            {!compact && <TableCell className="text-end tabular-nums">{r.rows}</TableCell>}
            <TableCell className="text-end tabular-nums">{r.time}</TableCell>
            {!compact && (
              <TableCell className="py-0 text-end">
                {i === 0 && (
                  <Button variant="ghost" size="xs">
                    <Undo2Icon data-icon="inline-start" />
                    Revert run
                  </Button>
                )}
              </TableCell>
            )}
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

type StepResult = "ok" | "warn" | "failed" | "running"

export const steps: {
  n: number
  name: string
  type: string
  detail: string
  result: StepResult
  resultText: string
  time: string
  llm?: string
  touches?: { icon: "host" | "table" | "feeds"; label: string; sub: string }
}[] = [
  {
    n: 1, name: "price", type: "http.get", detail: "GET /simple/price", result: "ok", resultText: "1 item", time: "0.4 s",
    touches: { icon: "host", label: "api.coingecko.com", sub: "approved host" },
  },
  {
    n: 2, name: "save_price", type: "db.upsert", detail: "key (coin, date)", result: "ok", resultText: "1 row", time: "0.1 s",
    touches: { icon: "table", label: "btc_prices", sub: "shown on page Bitcoin" },
  },
  {
    n: 3, name: "news", type: "feed.read", detail: "${{ item.url }}", result: "ok", resultText: "38 items", time: "1.2 s",
    touches: { icon: "feeds", label: "3 feed hosts", sub: "no key needed" },
  },
  {
    n: 4, name: "pick", type: "llm.select", detail: "keep: moves the price · max 5", result: "warn", resultText: "5 kept · 12 dropped", time: "2.3 s", llm: "fast",
  },
  {
    n: 5, name: "save_news", type: "db.upsert", detail: "key url", result: "ok", resultText: "5 rows", time: "0.1 s",
    touches: { icon: "table", label: "news", sub: "shown on page Bitcoin" },
  },
]

export function ResultIcon({ result, className }: { result: StepResult; className?: string }) {
  if (result === "warn") return <TriangleAlertIcon className={cn("size-3.5 text-tone-amber", className)} />
  if (result === "failed") return <XIcon className={cn("size-3.5 text-tone-red", className)} />
  return <CheckIcon className={cn("size-3.5 text-tone-green", className)} />
}

export function StepsTable({ selected }: { selected: number }) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead className="w-8">#</TableHead>
          <TableHead>Step</TableHead>
          <TableHead>Type</TableHead>
          <TableHead>Result</TableHead>
          <TableHead className="text-end">Time</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {steps.map((s) => (
          <TableRow key={s.n} data-state={s.n === selected ? "selected" : undefined}>
            <TableCell className="text-muted-foreground tabular-nums">{s.n}</TableCell>
            <TableCell className="font-medium">{s.name}</TableCell>
            <TableCell className="font-mono text-xs text-muted-foreground">{s.type}</TableCell>
            <TableCell>
              <span className="flex items-center gap-1.5">
                <ResultIcon result={s.result} />
                {s.resultText}
              </span>
            </TableCell>
            <TableCell className="text-end tabular-nums">{s.time}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

export function OutputPreview({ title, meta, children }: { title: string; meta?: string; children: React.ReactNode }) {
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        {meta && (
          <CardAction>
            <span className="text-xs text-muted-foreground">{meta}</span>
          </CardAction>
        )}
      </CardHeader>
      <CardContent className="flex flex-col gap-3">{children}</CardContent>
    </Card>
  )
}

export function CodePreview({ children }: { children: string }) {
  return (
    <pre className="overflow-hidden rounded-lg bg-muted/60 p-3 font-mono text-xs leading-5 whitespace-pre-wrap">
      {children}
    </pre>
  )
}

export function RefLine({ children }: { children: React.ReactNode }) {
  return <p className="text-xs text-muted-foreground">{children}</p>
}

export function RunActions() {
  return (
    <div className="flex gap-2">
      <Button variant="outline" size="sm">
        <Undo2Icon data-icon="inline-start" />
        Revert run
      </Button>
      <Button variant="outline" size="sm">
        <CopyIcon data-icon="inline-start" />
        Copy log
      </Button>
    </div>
  )
}

// ---- Flow view (06-A): plain React and CSS, top to bottom. ----

function Connector() {
  return (
    <div className="flex flex-col items-center text-border">
      <div className="h-3 w-px bg-border" />
      <ChevronDownIcon className="-mt-1.5 size-3.5 text-muted-foreground/60" />
    </div>
  )
}

function Pill({ children }: { children: React.ReactNode }) {
  return (
    <div className="mx-auto flex w-fit items-center gap-1.5 rounded-full border bg-card px-3 py-1 text-xs text-muted-foreground">
      {children}
    </div>
  )
}

function StepNode({ s, selected }: { s: (typeof steps)[number]; selected?: boolean }) {
  return (
    <div
      className={cn(
        "flex flex-col gap-1 rounded-lg border bg-card px-3 py-2 text-sm",
        selected && "border-primary ring-2 ring-primary/20"
      )}
    >
      <div className="flex items-center gap-2">
        <span className="font-mono text-xs text-muted-foreground">{s.type}</span>
        <span className="font-medium">{s.name}</span>
        {s.llm && <Badge className="bg-tone-purple-soft text-tone-purple">LLM · {s.llm}</Badge>}
      </div>
      <div className="flex items-center gap-2 text-xs">
        <span className="truncate font-mono text-muted-foreground">{s.detail}</span>
        <span className="ms-auto flex shrink-0 items-center gap-1 tabular-nums">
          <ResultIcon result={s.result} />
          {s.resultText} · {s.time}
        </span>
      </div>
    </div>
  )
}

const touchIcon = { host: GlobeIcon, table: DatabaseIcon, feeds: RepeatIcon }

function Touch({ t }: { t?: (typeof steps)[number]["touches"] }) {
  if (!t) return <div />
  const Icon = touchIcon[t.icon]
  return (
    <div className="flex items-center gap-2">
      <div className="h-px w-4 shrink-0 border-t border-dashed" />
      <div className="flex min-w-0 flex-col rounded-md border border-dashed bg-muted/40 px-2 py-1 text-xs">
        <span className="flex items-center gap-1.5 truncate">
          <Icon className="size-3 text-muted-foreground" />
          {t.label}
        </span>
        <span className="truncate text-muted-foreground">{t.sub}</span>
      </div>
    </div>
  )
}

function FlowRow({ s, selected }: { s: (typeof steps)[number]; selected?: boolean }) {
  return (
    <div className="grid grid-cols-[1fr_11rem] items-center gap-0">
      <StepNode s={s} selected={selected} />
      <Touch t={s.touches} />
    </div>
  )
}

export function Flow({ selected }: { selected: number }) {
  const [price, savePrice, news, pick, saveNews] = steps
  return (
    <div className="flex flex-col">
      <div className="grid grid-cols-[1fr_11rem] pb-1 text-xs font-medium text-muted-foreground">
        <span>Flow</span>
        <span className="ps-6">Touches</span>
      </div>
      <div className="grid grid-cols-[1fr_11rem]">
        <Pill>
          <ClockIcon className="size-3.5" />
          every day 08:00 · catch-up on
        </Pill>
      </div>
      <div className="grid grid-cols-[1fr_11rem]">
        <Connector />
      </div>
      <FlowRow s={price} selected={selected === 1} />
      <div className="grid grid-cols-[1fr_11rem]">
        <Connector />
      </div>
      <FlowRow s={savePrice} selected={selected === 2} />
      <div className="grid grid-cols-[1fr_11rem]">
        <Connector />
      </div>
      <div className="grid grid-cols-[1fr_11rem] items-center">
        <div className="flex flex-col gap-2 rounded-xl border border-dashed p-2">
          <div className="flex items-center justify-between px-1 text-xs">
            <span className="font-mono text-muted-foreground">for_each feed</span>
            <span className="text-muted-foreground">× 3 feeds</span>
          </div>
          <StepNode s={news} selected={selected === 3} />
        </div>
        <Touch t={news.touches} />
      </div>
      <div className="grid grid-cols-[1fr_11rem]">
        <Connector />
      </div>
      <FlowRow s={pick} selected={selected === 4} />
      <div className="grid grid-cols-[1fr_11rem]">
        <Connector />
      </div>
      <FlowRow s={saveNews} selected={selected === 5} />
      <div className="grid grid-cols-[1fr_11rem]">
        <Connector />
      </div>
      <div className="grid grid-cols-[1fr_11rem]">
        <Pill>end · 6 rows written · 4.1 s</Pill>
      </div>
      <p className="pt-4 text-xs text-muted-foreground">
        Read-only: the flow is drawn from the saved YAML. Changes go through the agent, like pages.
      </p>
    </div>
  )
}

export function StepSettings({ rows }: { rows: [string, string][] }) {
  return (
    <dl className="grid grid-cols-[5rem_1fr] gap-x-4 gap-y-1 text-sm">
      {rows.map(([k, v]) => (
        <React.Fragment key={k}>
          <dt className="text-muted-foreground">{k}</dt>
          <dd className="font-mono text-xs leading-5">{v}</dd>
        </React.Fragment>
      ))}
    </dl>
  )
}

export function ViewStepYaml() {
  return (
    <Button variant="outline" size="sm" className="w-fit">
      <EyeIcon data-icon="inline-start" />
      View step YAML
    </Button>
  )
}

export { CardDescription }
