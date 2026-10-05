import * as React from "react"
import { SearchIcon } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import { Table, TableBody, TableCell, TableFooter, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { DevPage } from "@/dev/dev-page"
import { chatKey, chatOptions, duration, num, parts, scale, share, size, tick } from "@/dev/format"
import { DevService, type RequestView, type ToolView, type TurnItem, type TurnView } from "@/lib/api"
import { toUIError } from "@/lib/errors"
import { cn } from "@/lib/utils"

interface Pick {
  project: string
  chat: string
  turn: number
}

// InspectorView is the turn inspector (SPEC 8.4): what the context
// builder sent for a turn, and what came back. It opens at a turn, from
// *Inspect* in the turn footer, or at the newest turn recorded.
export function InspectorView({ project, chat, turn }: Partial<Pick>) {
  const [turns, setTurns] = React.useState<TurnItem[]>()
  const [pick, setPick] = React.useState<Pick | undefined>(
    project && chat && turn ? { project, chat, turn } : undefined,
  )
  const [view, setView] = React.useState<TurnView>()
  const [error, setError] = React.useState<string>()

  const loadTurns = React.useCallback(async () => {
    const list = (await DevService.Turns()) ?? []
    setTurns(list)
    setPick((p) => p ?? (list[0] && { project: list[0].project, chat: list[0].chat, turn: list[0].turn }))
  }, [])

  React.useEffect(() => {
    loadTurns().catch((err) => setError(toUIError(err).message))
  }, [loadTurns])

  // The turn, again every second while it runs.
  React.useEffect(() => {
    if (!pick) return
    let stop = false
    let timer: ReturnType<typeof setTimeout> | undefined
    const load = async () => {
      try {
        const v = await DevService.Turn(pick.project, pick.chat, pick.turn)
        if (stop) return
        setView(v)
        setError(undefined)
        if (v.running) timer = setTimeout(load, 1000)
        else void loadTurns().catch(() => {})
      } catch (err) {
        if (!stop) {
          setView(undefined)
          setError(toUIError(err).message)
        }
      }
    }
    void load()
    return () => {
      stop = true
      clearTimeout(timer)
    }
  }, [pick, loadTurns])

  return (
    <DevPage
      title="Turn inspector"
      description="What the context builder sent, and what came back."
      actions={turns && turns.length > 0 && <Pickers turns={turns} pick={pick} onPick={setPick} />}
    >
      {turns?.length === 0 ? (
        <NoTurns />
      ) : error ? (
        <Empty className="border">
          <EmptyHeader>
            <EmptyTitle>Can't show this turn</EmptyTitle>
            <EmptyDescription>{error}</EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : view && pick ? (
        <Turn key={`${pick.project}/${pick.chat}/${pick.turn}`} view={view} />
      ) : (
        <Spinner />
      )}
    </DevPage>
  )
}

function NoTurns() {
  return (
    <Empty className="border">
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <SearchIcon />
        </EmptyMedia>
        <EmptyTitle>No turns recorded yet</EmptyTitle>
        <EmptyDescription>
          Turns are recorded from the start of the app while the developer tools are on, in memory only: the last 100.
        </EmptyDescription>
      </EmptyHeader>
    </Empty>
  )
}

function Pickers({ turns, pick, onPick }: { turns: TurnItem[]; pick?: Pick; onPick: (p: Pick) => void }) {
  const chats = chatOptions(turns)
  const key = pick ? chatKey(pick.project, pick.chat) : undefined
  const ofChat = turns.filter((t) => chatKey(t.project, t.chat) === key)
  return (
    <>
      <Select
        value={key}
        onValueChange={(k) => {
          const t = turns.find((t) => chatKey(t.project, t.chat) === k)
          if (t) onPick({ project: t.project, chat: t.chat, turn: t.turn })
        }}
      >
        <SelectTrigger size="sm" className="max-w-64" aria-label="Chat">
          <SelectValue placeholder="Chat" />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            {chats.map((c) => (
              <SelectItem key={c.key} value={c.key}>
                <span dir="auto" className="truncate">
                  {c.label}
                </span>
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
      <Select value={pick ? String(pick.turn) : undefined} onValueChange={(n) => pick && onPick({ ...pick, turn: Number(n) })}>
        <SelectTrigger size="sm" aria-label="Turn">
          <SelectValue placeholder="Turn" />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            {ofChat.map((t) => (
              <SelectItem key={t.turn} value={String(t.turn)}>
                {turnLabel(t)}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </>
  )
}

function turnLabel(t: TurnItem) {
  const at = new Date(t.started).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
  const reqs = `${t.requests} ${t.requests === 1 ? "request" : "requests"}`
  return `${at} · turn ${t.turn} · ${t.running ? "running" : reqs}`
}

function Turn({ view }: { view: TurnView }) {
  const requests = view.requests ?? []
  const tools = view.tools ?? []
  const [chosen, setChosen] = React.useState(0)
  const request = requests[Math.min(chosen, requests.length - 1)]
  return (
    <>
      <Timeline view={view} requests={requests} tools={tools} />
      <div className="grid items-start gap-4 lg:grid-cols-2">
        <div className="flex flex-col gap-4">
          <Requests requests={requests} tools={tools} chosen={chosen} onChoose={setChosen} />
          {tools.length > 0 && <Tools tools={tools} />}
        </div>
        {request && <Blocks view={view} index={requests.indexOf(request)} request={request} />}
      </div>
    </>
  )
}

// Timeline lays the turn's requests, tool calls and the parts that came
// back on one time axis, from the turn's start.
function Timeline({ view, requests, tools }: { view: TurnView; requests: RequestView[]; tools: ToolView[] }) {
  const s = scale(view.took_ms)
  const at = (ms: number) => (ms / s.span) * 100
  const bar = (start: number, took: number) => ({ insetInlineStart: `${at(start)}%`, width: `${at(took)}%`, minWidth: 4 })
  return (
    <Card>
      <CardHeader>
        <CardTitle>Timeline</CardTitle>
        <CardDescription>
          {view.model || "No model yet"} · {duration(view.took_ms)}
          {view.running && " so far, still running"}
        </CardDescription>
      </CardHeader>
      <CardContent className="grid grid-cols-[4.5rem_1fr] items-center gap-x-3 gap-y-2 text-xs">
        <span />
        <div className="relative h-4 text-muted-foreground tabular-nums">
          {s.ticks.map((t, i) => (
            <span
              key={t}
              className={cn("absolute", i > 0 && (i === s.ticks.length - 1 ? "-translate-x-full rtl:translate-x-full" : "-translate-x-1/2 rtl:translate-x-1/2"))}
              style={{ insetInlineStart: `${at(t)}%` }}
            >
              {tick(t, s.step)}
            </span>
          ))}
        </div>
        <Lane label="Model">
          {requests.map((r, i) => (
            <Bar key={i} style={bar(r.start_ms, r.took_ms)} className={r.err ? "bg-tone-red-soft text-tone-red" : "bg-tone-blue-soft text-tone-blue"}>
              req {i + 1}
            </Bar>
          ))}
        </Lane>
        <Lane label="Tools">
          {tools.map((t, i) => (
            <Bar
              key={i}
              style={bar(t.start_ms, t.took_ms)}
              className={t.error ? "bg-tone-red-soft text-tone-red" : "bg-tone-green-soft text-tone-green"}
              title={`${t.name} · ${duration(t.took_ms)}`}
            >
              {t.name}
            </Bar>
          ))}
        </Lane>
        <Lane label="Parts">
          {requests.map((r, i) => (
            <Bar key={i} style={bar(r.start_ms, r.took_ms)} className="bg-tone-neutral-soft text-tone-neutral">
              {parts(r.parts ?? [])}
            </Bar>
          ))}
        </Lane>
        {tools.length > 0 && (
          <p className="col-start-2 text-muted-foreground">
            Tool calls:{" "}
            {tools.map((t, i) => (
              <React.Fragment key={i}>
                {i > 0 && ", "}
                <span className="font-mono">{t.name}</span> {duration(t.took_ms)}
              </React.Fragment>
            ))}
            .
          </p>
        )}
      </CardContent>
    </Card>
  )
}

function Lane({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <>
      <span className="text-muted-foreground">{label}</span>
      <div className="relative h-6 rounded-md bg-muted/50">{children}</div>
    </>
  )
}

function Bar({ className, style, title, children }: { className: string; style: React.CSSProperties; title?: string; children: React.ReactNode }) {
  return (
    <div className={cn("absolute inset-y-0 truncate rounded-md px-1.5 leading-6", className)} style={style} title={title}>
      {children}
    </div>
  )
}

function prompt(r: RequestView) {
  return r.usage.input + r.usage.cache_read + r.usage.cache_write
}

// counted is a token count, or "–" before the provider sent its counts.
function counted(r: RequestView, n: number) {
  return prompt(r) + r.usage.output === 0 ? "–" : num(n)
}

function Requests({
  requests,
  tools,
  chosen,
  onChoose,
}: {
  requests: RequestView[]
  tools: ToolView[]
  chosen: number
  onChoose: (i: number) => void
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Requests</CardTitle>
        <CardDescription>Token counts from the provider. Pick a request to see its context blocks.</CardDescription>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>#</TableHead>
              <TableHead className="text-end">Prompt</TableHead>
              <TableHead className="text-end">Cached</TableHead>
              <TableHead className="text-end">Output</TableHead>
              <TableHead className="text-end">Time</TableHead>
              <TableHead>Tools</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {requests.map((r, i) => {
              const names = tools.filter((t) => t.request === i).map((t) => t.name)
              return (
                <React.Fragment key={i}>
                  <TableRow
                    data-state={i === chosen ? "selected" : undefined}
                    className="cursor-pointer tabular-nums"
                    tabIndex={0}
                    onClick={() => onChoose(i)}
                    onKeyDown={(e) => (e.key === "Enter" || e.key === " ") && (e.preventDefault(), onChoose(i))}
                  >
                    <TableCell className="text-muted-foreground">{i + 1}</TableCell>
                    <TableCell className="text-end">{counted(r, prompt(r))}</TableCell>
                    <TableCell className="text-end">{counted(r, r.usage.cache_read)}</TableCell>
                    <TableCell className="text-end">{counted(r, r.usage.output)}</TableCell>
                    <TableCell className="text-end">{duration(r.took_ms)}</TableCell>
                    <TableCell className="font-mono text-xs">{names.length ? names.join(", ") : "–"}</TableCell>
                  </TableRow>
                  {r.err && (
                    <TableRow>
                      <TableCell colSpan={6} dir="auto" className="whitespace-normal text-tone-red">
                        {r.err}
                      </TableCell>
                    </TableRow>
                  )}
                </React.Fragment>
              )
            })}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}

function Tools({ tools }: { tools: ToolView[] }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Tool calls</CardTitle>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Tool</TableHead>
              <TableHead className="text-end">Time</TableHead>
              <TableHead>Result</TableHead>
              <TableHead>Ref</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {tools.map((t, i) => (
              <TableRow key={i} className="tabular-nums">
                <TableCell className="font-mono text-xs">{t.name}</TableCell>
                <TableCell className="text-end">{duration(t.took_ms)}</TableCell>
                <TableCell className={cn(t.error && "text-tone-red")}>
                  {size(t.bytes)}
                  {t.error && " · error"}
                </TableCell>
                <TableCell className="font-mono text-xs text-muted-foreground">{t.ref || "–"}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}

const cacheTone: Record<string, string> = {
  hit: "bg-tone-green-soft text-tone-green",
  part: "bg-tone-amber-soft text-tone-amber",
  miss: "bg-tone-neutral-soft text-tone-neutral",
}

function Blocks({ view, index, request }: { view: TurnView; index: number; request: RequestView }) {
  const blocks = request.blocks ?? []
  const most = Math.max(1, ...blocks.map((b) => b.tokens))
  const sum = blocks.reduce((n, b) => n + b.tokens, 0)
  const [open, setOpen] = React.useState<number>()
  return (
    <Card>
      <CardHeader>
        <CardTitle>Context blocks, request {index + 1}</CardTitle>
        <CardDescription>
          Fixed order, so the prefix stays cached. Tokens are estimates.
          {request.dropped && " The texts were dropped to save memory; the numbers stay."}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>#</TableHead>
              <TableHead>Block</TableHead>
              <TableHead />
              <TableHead className="text-end">Tokens</TableHead>
              <TableHead className="text-end">Cache</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {blocks.map((b, i) => (
              <TableRow key={i} className="tabular-nums">
                <TableCell className="text-muted-foreground">{i + 1}</TableCell>
                <TableCell>
                  {request.dropped ? (
                    b.name
                  ) : (
                    <Button variant="link" className="h-auto p-0" onClick={() => setOpen(i)}>
                      {b.name}
                    </Button>
                  )}
                  {b.point && <span className="ms-1.5 text-xs text-muted-foreground">· cache point</span>}
                </TableCell>
                <TableCell className="w-24">
                  <div className="h-1.5 w-24 rounded-full bg-muted">
                    <div className="h-full rounded-full bg-primary" style={{ width: `${(b.tokens / most) * 100}%` }} />
                  </div>
                </TableCell>
                <TableCell className="text-end">{num(b.tokens)}</TableCell>
                <TableCell className="text-end">
                  {b.cache && (
                    <Badge variant="secondary" className={cacheTone[b.cache]}>
                      {b.cache}
                    </Badge>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
          <TableFooter>
            <TableRow className="tabular-nums">
              <TableCell />
              <TableCell>Total</TableCell>
              <TableCell />
              <TableCell className="text-end">{num(sum)}</TableCell>
              <TableCell className="text-end text-xs">{share(request.usage.cache_read, prompt(request))}</TableCell>
            </TableRow>
          </TableFooter>
        </Table>
      </CardContent>
      {open !== undefined && (
        <BlockText view={view} request={index} block={open} name={blocks[open]?.name ?? ""} tokens={blocks[open]?.tokens ?? 0} onClose={() => setOpen(undefined)} />
      )}
    </Card>
  )
}

function BlockText({
  view,
  request,
  block,
  name,
  tokens,
  onClose,
}: {
  view: TurnView
  request: number
  block: number
  name: string
  tokens: number
  onClose: () => void
}) {
  const [text, setText] = React.useState<string>()
  const [error, setError] = React.useState<string>()
  React.useEffect(() => {
    DevService.Block(view.project, view.chat, view.turn, request, block).then(setText, (err) => setError(toUIError(err).message))
  }, [view.project, view.chat, view.turn, request, block])
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{name}</DialogTitle>
          <DialogDescription>
            Request {request + 1} · about {num(tokens)} tokens · secrets redacted
          </DialogDescription>
        </DialogHeader>
        {error ? (
          <p className="text-tone-red">{error}</p>
        ) : text === undefined ? (
          <Spinner />
        ) : (
          <div className="max-h-[60vh] overflow-auto rounded-md border">
            <pre dir="auto" className="p-3 font-mono text-xs break-words whitespace-pre-wrap">
              {text || "(empty)"}
            </pre>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
