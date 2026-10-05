import * as React from "react"
import { TriangleAlertIcon } from "lucide-react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { SettingsService, type UsageLine, type UsageReport } from "@/lib/api"
import { showError } from "@/lib/errors"
import { cost, tokens, total } from "@/settings/format"
import { SettingsSection } from "@/shell/settings-layout"

const UsageChart = React.lazy(() => import("@/settings/usage-chart"))

const ranges = [7, 30, 90]

type Report = UsageReport & { days: UsageLine[]; chats: UsageLine[]; models: UsageLine[]; skipped: string[] }

// UsageSection is *Settings → Usage* (SPEC 3.9): tokens per day, per chat
// and per model, with the cost from the catalog prices.
export function UsageSection() {
  const [days, setDays] = React.useState(30)
  const [report, setReport] = React.useState<Report | null>(null)
  const [busy, setBusy] = React.useState(true)

  React.useEffect(() => {
    let live = true
    setBusy(true)
    SettingsService.Usage(days)
      .then((r) => live && setReport({ ...r, days: r.days ?? [], chats: r.chats ?? [], models: r.models ?? [], skipped: r.skipped ?? [] }))
      .catch(showError)
      .finally(() => live && setBusy(false))
    return () => {
      live = false
    }
  }, [days])

  const unpriced = !!report && report.total.unpriced > 0
  return (
    <SettingsSection
      title="Usage"
      description="Tokens across all projects. Costs come from the built-in prices; your provider's bill is what counts."
    >
      <div className="flex items-center justify-between gap-3">
        {report ? (
          <span className="text-sm">
            <span className="font-medium tabular-nums">{tokens(total(report.total))}</span> tokens ·{" "}
            <span className="font-medium tabular-nums">{cost(report.total)}</span>
          </span>
        ) : (
          <Skeleton className="h-4 w-32" />
        )}
        <div className="flex items-center gap-2">
          {busy && report && <Spinner />}
          <ToggleGroup
            type="single"
            variant="outline"
            size="sm"
            value={String(days)}
            onValueChange={(v) => v && setDays(Number(v))}
            aria-label="Days"
          >
            {ranges.map((d) => (
              <ToggleGroupItem key={d} value={String(d)}>
                {d} days
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
        </div>
      </div>
      {report ? (
        <>
          <React.Suspense fallback={<Skeleton className="h-48 w-full" />}>
            <UsageChart days={report.days} />
          </React.Suspense>
          {report.skipped.length > 0 && (
            <Alert>
              <TriangleAlertIcon className="text-tone-amber" />
              <AlertDescription>
                <span>
                  Not counted, their chats couldn&apos;t be read: <span dir="auto">{report.skipped.join(", ")}</span>
                </span>
              </AlertDescription>
            </Alert>
          )}
          <UsageTable
            title="By model"
            lines={report.models}
            split
            name={(l) => (
              <>
                <span dir="auto">{l.model}</span>
                <span className="text-muted-foreground"> · {l.provider}</span>
              </>
            )}
          />
          <UsageTable
            title="By chat"
            lines={report.chats}
            name={(l) => (
              <>
                <span dir="auto">{l.chat || "Untitled chat"}</span>
                <span className="text-muted-foreground"> · </span>
                <span className="text-muted-foreground" dir="auto">
                  {l.project}
                </span>
              </>
            )}
          />
          {unpriced && (
            <p className="text-xs text-muted-foreground">
              A + means some tokens are from models without a price; they count as tokens only.
            </p>
          )}
        </>
      ) : (
        <Skeleton className="h-48 w-full" />
      )}
    </SettingsSection>
  )
}

function UsageTable({
  title,
  lines,
  name,
  split,
}: {
  title: string
  lines: UsageLine[]
  name: (l: UsageLine) => React.ReactNode
  split?: boolean
}) {
  return (
    <div className="flex flex-col gap-2">
      <h3 className="text-sm font-medium">{title}</h3>
      {lines.length === 0 ? (
        <p className="text-sm text-muted-foreground">No tokens used in these days.</p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{split ? "Model" : "Chat"}</TableHead>
              {split ? (
                <>
                  <TableHead className="text-end">Input</TableHead>
                  <TableHead className="text-end">Cached</TableHead>
                  <TableHead className="text-end">Output</TableHead>
                </>
              ) : (
                <TableHead className="text-end">Tokens</TableHead>
              )}
              <TableHead className="text-end">Cost</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {lines.map((l, i) => (
              <TableRow key={i}>
                <TableCell className="max-w-72 truncate">{name(l)}</TableCell>
                {split ? (
                  <>
                    <TableCell className="text-end tabular-nums">{tokens(l.input + l.cache_write)}</TableCell>
                    <TableCell className="text-end tabular-nums">{tokens(l.cache_read)}</TableCell>
                    <TableCell className="text-end tabular-nums">{tokens(l.output)}</TableCell>
                  </>
                ) : (
                  <TableCell className="text-end tabular-nums">{tokens(total(l))}</TableCell>
                )}
                <TableCell className="text-end tabular-nums">{cost(l)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  )
}
