import * as React from "react"

import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Table, TableBody, TableCell, TableFooter, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { SettingsLayout, SettingsTitle } from "@/jenab/settings"
import { cn } from "@/lib/utils"

const pages = ["General", "Appearance", "Models", "Usage", "Search", "Connections", "MCP servers", "Updates", "Developer"]

// Timeline, in seconds. The turn's own work fits in the first 3.2 s;
// the background run outlives it and its finish notice starts the next turn.
const SPAN = 4
type Bar = { from: number; to: number; label: string; tone: string; dashed?: boolean }
const lanes: { name: string; bars: Bar[] }[] = [
  {
    name: "Model",
    bars: [
      { from: 0, to: 1.2, label: "req 1", tone: "bg-tone-blue-soft text-tone-blue" },
      { from: 1.3, to: 2.1, label: "req 2", tone: "bg-tone-blue-soft text-tone-blue" },
      { from: 2.2, to: 3.0, label: "req 3", tone: "bg-tone-blue-soft text-tone-blue" },
    ],
  },
  {
    name: "Tools",
    bars: [
      { from: 1.2, to: 1.3, label: "query", tone: "bg-tone-green-soft text-tone-green" },
      { from: 2.1, to: 2.2, label: "run_pipeline", tone: "bg-tone-green-soft text-tone-green" },
    ],
  },
  {
    name: "Background",
    bars: [{ from: 2.1, to: 4, label: "run r_88 · daily_btc · 64 s", tone: "bg-tone-amber-soft text-tone-amber", dashed: true }],
  },
  {
    name: "Parts",
    bars: [
      { from: 0, to: 1.25, label: "answer", tone: "bg-muted text-muted-foreground" },
      { from: 2.2, to: 3.0, label: "reply", tone: "bg-muted text-muted-foreground" },
    ],
  },
]

function Timeline() {
  const ticks = [0, 1, 2, 3, 4]
  return (
    <div className="grid grid-cols-[6rem_1fr] gap-y-2 text-xs">
      <span />
      <div className="relative h-4 text-muted-foreground">
        {ticks.map((t) => (
          <span key={t} className={cn("absolute tabular-nums whitespace-nowrap", t === SPAN ? "-translate-x-full" : t > 0 && "-translate-x-1/2")} style={{ insetInlineStart: `${(t / SPAN) * 100}%` }}>
            {t} s
          </span>
        ))}
      </div>
      {lanes.map((lane) => (
        <React.Fragment key={lane.name}>
          <span className="self-center text-muted-foreground">{lane.name}</span>
          <div className="relative h-7 rounded-md bg-muted/40">
            {lane.bars.map((b) => (
              <span
                key={b.label}
                title={b.label}
                className={cn(
                  "absolute inset-y-1 flex items-center overflow-visible rounded px-1.5 font-medium whitespace-nowrap",
                  b.tone,
                  b.dashed && "rounded-e-none border border-e-0 border-dashed border-current/40"
                )}
                style={{ insetInlineStart: `${(b.from / SPAN) * 100}%`, width: `${((b.to - b.from) / SPAN) * 100}%` }}
              >
                {b.to - b.from >= 0.5 && b.label}
              </span>
            ))}
          </div>
        </React.Fragment>
      ))}
      <span />
      <p className="text-muted-foreground">
        Tool calls: <span className="font-mono">query</span> 38 ms, <span className="font-mono">run_pipeline</span> 12 ms.
        The run's finish notice starts the next turn at 10:03.
      </p>
    </div>
  )
}

const requests = [
  { n: 1, prompt: "7,412", cached: "6,120", output: "84", time: "1.2 s", tools: "query" },
  { n: 2, prompt: "7,560", cached: "7,380", output: "42", time: "0.8 s", tools: "run_pipeline" },
  { n: 3, prompt: "7,690", cached: "7,510", output: "61", time: "0.8 s", tools: "—" },
]

const calls = [
  { tool: "query", time: "38 ms", result: "1 row · 0.2 KB", ref: "—" },
  { tool: "run_pipeline", time: "12 ms", result: "run r_88", ref: "runs/r_88" },
]

const blocks: { name: string; tokens: number; cache: "hit" | "part" | "miss" }[] = [
  { name: "System prompt and role", tokens: 1850, cache: "hit" },
  { name: "User memory", tokens: 320, cache: "hit" },
  { name: "Project memory", tokens: 610, cache: "hit" },
  { name: "Project card", tokens: 1420, cache: "hit" },
  { name: "Session notes", tokens: 210, cache: "hit" },
  { name: "History window", tokens: 2860, cache: "part" },
  { name: "This turn", tokens: 142, cache: "miss" },
]

const cacheTone = {
  hit: "bg-tone-green-soft text-tone-green",
  part: "bg-tone-amber-soft text-tone-amber",
  miss: "bg-tone-neutral-soft text-tone-neutral",
}

function Picker({ value }: { value: string }) {
  return (
    <Select defaultValue={value}>
      <SelectTrigger size="sm">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectGroup>
          <SelectItem value={value}>{value}</SelectItem>
        </SelectGroup>
      </SelectContent>
    </Select>
  )
}

// 10 · Turn inspector, Settings → Developer (SPEC 8.4). Read-only; secrets redacted.
export function Screen10() {
  const max = Math.max(...blocks.map((b) => b.tokens))
  return (
    <SettingsLayout items={pages} active="Developer" wide>
      <div className="flex flex-wrap items-end gap-3">
        <SettingsTitle title="Turn inspector" description="What the context builder sent, and what came back." />
        <div className="ms-auto flex items-center gap-2">
          <Picker value="Chat: Mother" />
          <Picker value="Turn: 10:02 · 3 requests" />
          <Badge variant="outline">Read-only · secrets redacted</Badge>
        </div>
      </div>

      <Card size="sm">
        <CardHeader>
          <CardTitle>Timeline</CardTitle>
        </CardHeader>
        <CardContent>
          <Timeline />
        </CardContent>
      </Card>

      <div className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)] items-start gap-4">
        <div className="flex flex-col gap-4">
          <Card size="sm">
            <CardHeader>
              <CardTitle>Requests</CardTitle>
            </CardHeader>
            <CardContent>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-8">#</TableHead>
                    <TableHead className="text-end">Prompt</TableHead>
                    <TableHead className="text-end">Cached</TableHead>
                    <TableHead className="text-end">Output</TableHead>
                    <TableHead className="text-end">Time</TableHead>
                    <TableHead>Tools</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {requests.map((r) => (
                    <TableRow key={r.n} data-state={r.n === 1 ? "selected" : undefined}>
                      <TableCell className="text-muted-foreground">{r.n}</TableCell>
                      <TableCell className="text-end tabular-nums">{r.prompt}</TableCell>
                      <TableCell className="text-end tabular-nums">{r.cached}</TableCell>
                      <TableCell className="text-end tabular-nums">{r.output}</TableCell>
                      <TableCell className="text-end tabular-nums">{r.time}</TableCell>
                      <TableCell className="font-mono text-xs">{r.tools}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
          <Card size="sm">
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
                  {calls.map((c) => (
                    <TableRow key={c.tool}>
                      <TableCell className="font-mono text-xs">{c.tool}</TableCell>
                      <TableCell className="text-end tabular-nums">{c.time}</TableCell>
                      <TableCell>{c.result}</TableCell>
                      <TableCell className="font-mono text-xs text-muted-foreground">{c.ref}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </div>

        <Card size="sm">
          <CardHeader>
            <CardTitle>Context blocks, request 1</CardTitle>
            <CardDescription>Fixed order, so the prefix stays cached.</CardDescription>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-8">#</TableHead>
                  <TableHead>Block</TableHead>
                  <TableHead className="w-28" />
                  <TableHead className="text-end">Tokens</TableHead>
                  <TableHead className="text-end">Cache</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {blocks.map((b, i) => (
                  <TableRow key={b.name}>
                    <TableCell className="text-muted-foreground">{i + 1}</TableCell>
                    <TableCell>{b.name}</TableCell>
                    <TableCell>
                      <div className="h-1.5 rounded-full bg-muted">
                        <div
                          className={cn("h-full rounded-full", b.cache === "miss" ? "bg-muted-foreground/40" : "bg-tone-blue")}
                          style={{ width: `${(b.tokens / max) * 100}%` }}
                        />
                      </div>
                    </TableCell>
                    <TableCell className="text-end tabular-nums">{b.tokens.toLocaleString("en-US")}</TableCell>
                    <TableCell className="text-end">
                      <Badge className={cacheTone[b.cache]}>{b.cache}</Badge>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
              <TableFooter>
                <TableRow>
                  <TableCell />
                  <TableCell colSpan={2}>Total</TableCell>
                  <TableCell className="text-end tabular-nums">7,412</TableCell>
                  <TableCell className="text-end text-xs whitespace-nowrap">6,120 cached (83 %)</TableCell>
                </TableRow>
              </TableFooter>
            </Table>
          </CardContent>
        </Card>
      </div>
    </SettingsLayout>
  )
}
