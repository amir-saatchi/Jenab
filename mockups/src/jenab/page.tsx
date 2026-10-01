import * as React from "react"
import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from "recharts"
import {
  ArrowDownIcon,
  ArrowUpIcon,
  CalendarIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
  RefreshCwIcon,
  XIcon,
} from "lucide-react"

import { Button } from "@/components/ui/button"
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { ChartContainer, ChartTooltip, ChartTooltipContent, type ChartConfig } from "@/components/ui/chart"
import { cn } from "@/lib/utils"

// Page layout (SPEC 5.9): a header, then rows on a 12-column grid.

export function PageHeader({
  title,
  description,
  children,
}: {
  title: string
  description: string
  children?: React.ReactNode
}) {
  return (
    <header className="flex flex-col gap-3 px-6 pt-5 pb-4">
      <div className="flex items-start gap-3">
        <div className="grid min-w-0 flex-1 gap-0.5">
          <h1 className="text-xl font-semibold tracking-tight">{title}</h1>
          <p className="text-sm text-muted-foreground" dir="auto">
            {description}
          </p>
        </div>
        <Button variant="ghost" size="sm">
          <XIcon data-icon="inline-start" />
          Close page
        </Button>
      </div>
      {children && <div className="flex flex-wrap items-center gap-2">{children}</div>}
    </header>
  )
}

export function DateFilter({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center gap-2 text-sm">
      <span className="text-muted-foreground">{label}</span>
      <Button variant="outline" size="sm">
        <CalendarIcon data-icon="inline-start" />
        {value}
      </Button>
    </div>
  )
}

export function RefreshButton() {
  return (
    <Button variant="outline" size="sm">
      <RefreshCwIcon data-icon="inline-start" />
      Refresh now
    </Button>
  )
}

export function Row({ children, className }: { children: React.ReactNode; className?: string }) {
  return <div className={cn("grid grid-cols-12 gap-4", className)}>{children}</div>
}

// Below about 900 px the rows stack into one column (SPEC 5.9).
const span: Record<number, string> = {
  4: "col-span-4 max-[900px]:col-span-12",
  5: "col-span-5 max-[900px]:col-span-12",
  7: "col-span-7 max-[900px]:col-span-12",
  8: "col-span-8 max-[900px]:col-span-12",
  12: "col-span-12",
}

export function Block({
  cols,
  title,
  description,
  action,
  children,
  className,
}: {
  cols: number
  title?: string
  description?: string
  action?: React.ReactNode
  children: React.ReactNode
  className?: string
}) {
  return (
    <Card className={cn(span[cols], className)} size="sm">
      {title && (
        <CardHeader>
          <CardTitle>{title}</CardTitle>
          {description && <CardDescription>{description}</CardDescription>}
          {action && <CardAction>{action}</CardAction>}
        </CardHeader>
      )}
      <CardContent className="min-w-0">{children}</CardContent>
    </Card>
  )
}

export function StatBlock({
  cols,
  title,
  value,
  change,
  caption,
}: {
  cols: number
  title: string
  value: string
  change: number
  caption: string
}) {
  const up = change >= 0
  return (
    <Block cols={cols} title={title}>
      <div className="flex flex-col gap-1.5">
        <span className="text-3xl font-semibold tracking-tight tabular-nums">{value}</span>
        <span
          className={cn(
            "flex items-center gap-1 text-sm tabular-nums",
            up ? "text-tone-green" : "text-tone-red"
          )}
        >
          {up ? <ArrowUpIcon className="size-3.5" /> : <ArrowDownIcon className="size-3.5" />}
          {Math.abs(change).toFixed(1)} % vs yesterday
        </span>
        <span className="text-xs text-muted-foreground">{caption}</span>
      </div>
    </Block>
  )
}

// 90 days of fake closes, ending at 64,210 with a drop on 12 Sep.
export const priceSeries = (() => {
  const out: { day: string; close: number }[] = []
  const start = new Date(Date.UTC(2026, 6, 1))
  let v = 61200
  let seed = 7
  const rnd = () => ((seed = (seed * 16807) % 2147483647) / 2147483647 - 0.5)
  for (let i = 0; i < 91; i++) {
    const d = new Date(start.getTime() + i * 86400000)
    const iso = d.toISOString().slice(0, 10)
    v += rnd() * 1400 + 25
    if (iso === "2026-09-12") v -= 4200
    out.push({ day: iso, close: Math.round(v) })
  }
  const shift = 64210 - out[out.length - 1].close
  return out.map((p) => ({ ...p, close: p.close + shift }))
})()

const priceConfig = {
  close: { label: "Close", color: "var(--tone-blue)" },
} satisfies ChartConfig

const monthTick = (iso: string) =>
  iso.endsWith("-01")
    ? new Date(iso + "T00:00:00Z").toLocaleString("en", { month: "short", timeZone: "UTC" })
    : ""

export function PriceChart() {
  return (
    <ChartContainer config={priceConfig} className="aspect-auto h-52 w-full">
      <AreaChart data={priceSeries} margin={{ left: 0, right: 8, top: 8 }}>
        <defs>
          <linearGradient id="fillClose" x1="0" y1="0" x2="0" y2="1">
            <stop offset="5%" stopColor="var(--color-close)" stopOpacity={0.25} />
            <stop offset="95%" stopColor="var(--color-close)" stopOpacity={0.02} />
          </linearGradient>
        </defs>
        <CartesianGrid vertical={false} />
        <XAxis
          dataKey="day"
          tickLine={false}
          axisLine={false}
          interval={0}
          tickFormatter={monthTick}
          tickMargin={8}
        />
        <YAxis
          width={52}
          tickLine={false}
          axisLine={false}
          domain={["dataMin - 1500", "dataMax + 1500"]}
          tickFormatter={(v: number) => `${Math.round(v / 1000)}k`}
        />
        <ChartTooltip content={<ChartTooltipContent indicator="line" />} />
        <Area
          dataKey="close"
          type="monotone"
          stroke="var(--color-close)"
          strokeWidth={2}
          fill="url(#fillClose)"
          isAnimationActive={false}
        />
      </AreaChart>
    </ChartContainer>
  )
}

export function TablePager({ page, pages }: { page: number; pages: number }) {
  return (
    <div className="flex items-center justify-end gap-1 pt-2 text-xs text-muted-foreground">
      <span className="pe-1">
        Page {page} of {pages}
      </span>
      <Button variant="ghost" size="icon-xs" aria-label="Previous page" disabled={page === 1}>
        <ChevronLeftIcon />
      </Button>
      <Button variant="ghost" size="icon-xs" aria-label="Next page">
        <ChevronRightIcon />
      </Button>
    </div>
  )
}
