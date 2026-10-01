import { Badge } from "@/components/ui/badge"
import { ScrollArea } from "@/components/ui/scroll-area"
import { AppShell } from "@/jenab/shell"
import { Block } from "@/jenab/page"
import {
  CodePreview,
  Flow,
  OutputPreview,
  PipelineHeader,
  RefLine,
  RunActions,
  RunsTable,
  StepSettings,
  ViewStepYaml,
} from "@/jenab/pipeline"

// 06-A · The same pipeline page as a read-only flow (the default view).
export function Screen06A() {
  return (
    <AppShell leftCollapsed activeChat="" statuses={{ mother: "working" }} rightTab="pipelines" activeItem="daily_btc">
      <ScrollArea className="h-full">
        <PipelineHeader view="flow" />
        <div className="grid grid-cols-[minmax(0,1fr)_24rem] items-start gap-6 px-6 pb-6">
          <Flow selected={4} />
          <div className="flex flex-col gap-4">
            <Block cols={12} title="Runs">
              <RunsTable compact />
            </Block>
            <OutputPreview title="Step 4 · pick · llm.select" meta="2.3 s · 1,840 tokens">
              <Badge className="w-fit bg-tone-purple-soft text-tone-purple">LLM · fast · costs tokens</Badge>
              <span className="text-xs font-medium text-muted-foreground">Settings</span>
              <StepSettings
                rows={[
                  ["items", "${{ steps.news.output }}"],
                  ["keep", "news that moves the price"],
                  ["max", "5"],
                  ["model", "fast"],
                ]}
              />
              <span className="text-xs font-medium text-muted-foreground">Output preview (secrets redacted)</span>
              <CodePreview>{`kept 5: SEC delays decision on bitcoin ETF options, …
12 dropped: not about Bitcoin (8), duplicates (4)`}</CodePreview>
              <RefLine>
                Full output as ref: <span className="font-mono">cache/runs/r_88/pick.json</span>
              </RefLine>
              <ViewStepYaml />
              <RunActions />
            </OutputPreview>
          </div>
        </div>
      </ScrollArea>
    </AppShell>
  )
}
