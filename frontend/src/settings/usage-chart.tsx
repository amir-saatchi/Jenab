import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from "recharts"

import {
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/components/ui/chart"
import type { UsageLine } from "@/lib/api"
import { tokens } from "@/settings/format"

const config = {
  input: { label: "Input", color: "var(--tone-blue)" },
  output: { label: "Output", color: "var(--tone-purple)" },
} satisfies ChartConfig

const dayLabel = (day: string) =>
  new Date(day + "T00:00:00").toLocaleDateString(undefined, { month: "short", day: "numeric" })

// UsageChart shows tokens per day, input (cache reads included) and
// output stacked. It is loaded with the Usage page, so recharts stays out
// of the first bundle.
export default function UsageChart({ days }: { days: UsageLine[] }) {
  const data = days.map((d) => ({ day: d.day, input: d.input + d.cache_read + d.cache_write, output: d.output }))
  return (
    <ChartContainer config={config} className="aspect-auto h-48 w-full" dir="ltr">
      <BarChart data={data} margin={{ left: 0, right: 8, top: 8 }}>
        <CartesianGrid vertical={false} />
        <XAxis dataKey="day" tickLine={false} axisLine={false} tickMargin={8} minTickGap={24} tickFormatter={dayLabel} />
        <YAxis width={44} tickLine={false} axisLine={false} tickFormatter={(v: number) => tokens(v)} />
        <ChartTooltip content={<ChartTooltipContent labelFormatter={(_, p) => dayLabel(String(p?.[0]?.payload?.day ?? ""))} />} />
        <ChartLegend content={<ChartLegendContent />} />
        <Bar dataKey="input" stackId="t" fill="var(--color-input)" isAnimationActive={false} />
        <Bar dataKey="output" stackId="t" fill="var(--color-output)" radius={[3, 3, 0, 0]} isAnimationActive={false} />
      </BarChart>
    </ChartContainer>
  )
}
