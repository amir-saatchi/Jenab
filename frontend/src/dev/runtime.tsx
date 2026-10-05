import * as React from "react"
import { TriangleAlertIcon } from "lucide-react"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Spinner } from "@/components/ui/spinner"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { DevPage } from "@/dev/dev-page"
import { duration, num } from "@/dev/format"
import { DevService, type ProjectRuntime, type ProviderStatus, type Runtime } from "@/lib/api"
import { toUIError } from "@/lib/errors"
import { cn } from "@/lib/utils"

// RuntimeView is the runtime panel (SPEC 8.4): the chats in a turn, the
// background work, each database writer, the LLM-call slots, and anything
// that hasn't moved for longer than its limit. It reads again every second.
export function RuntimeView() {
  const [rt, setRt] = React.useState<Runtime>()
  const [error, setError] = React.useState<string>()
  React.useEffect(() => {
    let stop = false
    let timer: ReturnType<typeof setTimeout> | undefined
    const load = async () => {
      try {
        const r = await DevService.Runtime()
        if (stop) return
        setRt(r)
        setError(undefined)
      } catch (err) {
        if (!stop) setError(toUIError(err).message)
      }
      if (!stop) timer = setTimeout(load, document.visibilityState === "visible" ? 1000 : 5000)
    }
    void load()
    return () => {
      stop = true
      clearTimeout(timer)
    }
  }, [])

  return (
    <DevPage title="Runtime" description="What runs now. Reads again every second.">
      {error && (
        <Alert variant="destructive">
          <TriangleAlertIcon />
          <AlertTitle>Can't read the runtime</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      {!rt ? (
        !error && <Spinner />
      ) : (
        <>
          {rt.stuck > 0 && (
            <Alert>
              <TriangleAlertIcon className="text-tone-amber" />
              <AlertTitle>
                {rt.stuck} {rt.stuck === 1 ? "thing hasn't" : "things haven't"} moved for longer than {rt.stuck === 1 ? "its" : "their"} limit
              </AlertTitle>
              <AlertDescription>They are marked Stuck below.</AlertDescription>
            </Alert>
          )}
          <Calls rt={rt} />
          {(rt.projects ?? []).length === 0 ? (
            <p className="text-sm text-muted-foreground">No project is open.</p>
          ) : (
            (rt.projects ?? []).map((p) => <Project key={p.project} p={p} />)
          )}
        </>
      )}
    </DevPage>
  )
}

function Stuck() {
  return (
    <Badge variant="secondary" className="bg-tone-amber-soft text-tone-amber">
      Stuck
    </Badge>
  )
}

function Calls({ rt }: { rt: Runtime }) {
  const providers = rt.providers ?? []
  return (
    <Card>
      <CardHeader>
        <CardTitle>LLM calls</CardTitle>
        <CardDescription>
          {rt.calls.in_use} of {rt.calls.size} slots in use, {rt.calls.waiting} waiting. Chat calls take a slot but never wait
          for one, so they can go past the limit.
        </CardDescription>
      </CardHeader>
      {providers.length > 0 && (
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Provider</TableHead>
                <TableHead className="text-end">Running</TableHead>
                <TableHead className="text-end">Waiting</TableHead>
                <TableHead className="text-end">Limit</TableHead>
                <TableHead>State</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {providers.map((p) => (
                <TableRow key={p.name} className="tabular-nums">
                  <TableCell>{p.name}</TableCell>
                  <TableCell className="text-end">{p.running}</TableCell>
                  <TableCell className="text-end">{p.waiting}</TableCell>
                  <TableCell className="text-end">{p.current === p.limit ? p.limit : `${p.current} of ${p.limit}`}</TableCell>
                  <TableCell dir="auto" className="whitespace-normal">
                    {providerState(p)}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      )}
    </Card>
  )
}

function providerState(p: ProviderStatus) {
  const paused = p.paused_ms > 0 ? `Paused for ${duration(p.paused_ms)}` : ""
  return [paused, p.last_problem].filter(Boolean).join(" · ") || "OK"
}

function Project({ p }: { p: ProjectRuntime }) {
  const work = p.work ?? []
  const writers = p.writers ?? []
  const now = Date.now()
  return (
    <Card>
      <CardHeader>
        <CardTitle dir="auto">{p.name || p.project}</CardTitle>
        <CardDescription>
          {work.length === 0 ? "Nothing running." : `${work.length} running.`} Held open by {p.leases}{" "}
          {p.leases === 1 ? "lease" : "leases"}.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {work.length > 0 && (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Work</TableHead>
                <TableHead>State</TableHead>
                <TableHead className="text-end">Running for</TableHead>
                <TableHead className="text-end">Last moved</TableHead>
                <TableHead className="text-end">Limit</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {work.map((w) => (
                <TableRow key={w.id} className="tabular-nums">
                  <TableCell className="whitespace-normal">
                    <span dir="auto">{w.title || w.id}</span>
                    <span className="ms-1.5 text-xs text-muted-foreground">{w.kind}</span>
                    {w.err && (
                      <p dir="auto" className="text-xs text-tone-red">
                        {w.err}
                      </p>
                    )}
                  </TableCell>
                  <TableCell>
                    {w.state}
                    {w.progress && <span className="text-muted-foreground"> · {w.progress}</span>}
                  </TableCell>
                  <TableCell className="text-end">{duration(now - Date.parse(w.started))}</TableCell>
                  <TableCell className="text-end">{duration(w.idle_ms)} ago</TableCell>
                  <TableCell className="text-end">{w.limit_ms > 0 ? duration(w.limit_ms) : "–"}</TableCell>
                  <TableCell className="text-end">{w.stuck && <Stuck />}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Writer</TableHead>
              <TableHead className="text-end">Queue</TableHead>
              <TableHead>Now</TableHead>
              <TableHead className="text-end">Done</TableHead>
              <TableHead>Last error</TableHead>
              <TableHead />
            </TableRow>
          </TableHeader>
          <TableBody>
            {writers.map((w) => (
              <TableRow key={w.file} className="tabular-nums">
                <TableCell className="font-mono text-xs">{w.file}</TableCell>
                <TableCell className="text-end">
                  {w.interactive} · {w.background}
                </TableCell>
                <TableCell className={cn(!w.busy && "text-muted-foreground")}>
                  {w.busy ? `Writing for ${duration(w.running_ms)}` : "Idle"}
                </TableCell>
                <TableCell className="text-end">{num(w.done)}</TableCell>
                <TableCell dir="auto" className="max-w-64 truncate text-tone-red" title={w.last_error}>
                  {w.last_error || ""}
                </TableCell>
                <TableCell className="text-end">{w.stuck && <Stuck />}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        <p className="text-xs text-muted-foreground">Queue: interactive · background requests waiting.</p>
      </CardContent>
    </Card>
  )
}
