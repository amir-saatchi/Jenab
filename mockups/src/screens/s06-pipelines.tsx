import { FlagIcon } from "lucide-react"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from "@/components/ui/resizable"
import { ScrollArea } from "@/components/ui/scroll-area"
import { AppShell } from "@/jenab/shell"
import {
  AgentMessage,
  AgentText,
  Composer,
  DockHeader,
  Thread,
  ToolChip,
  TurnFooter,
  UserMessage,
} from "@/jenab/chat"
import { Block, Row } from "@/jenab/page"
import {
  CodePreview,
  OutputPreview,
  PipelineHeader,
  RefLine,
  RunActions,
  RunsTable,
  StepsTable,
} from "@/jenab/pipeline"

function PipelineSteps() {
  return (
    <ScrollArea className="h-full">
      <PipelineHeader view="steps" />
      <div className="flex flex-col gap-4 px-6 pb-6">
        <Row>
          <Block cols={12} title="Runs">
            <RunsTable />
          </Block>
        </Row>
        <Row>
          <Block cols={12} title="Run r_88 · steps">
            <StepsTable selected={3} />
          </Block>
        </Row>
        <OutputPreview title="Step 3 · news · output preview" meta="secrets redacted">
          <CodePreview>{`[
  { "title": "SEC delays decision on bitcoin ETF options", "url": "https://coindesk.com/…", "published": "2026-09-12T07:40Z" },
  { "title": "Miners sell as hash price hits a yearly low", "url": "https://theblock.co/…", "published": "2026-09-12T06:15Z" },
  … 36 more
]`}</CodePreview>
          <RefLine>
            Full output as ref: <span className="font-mono">cache/runs/r_88/news.json</span>
          </RefLine>
          <RunActions />
        </OutputPreview>
      </div>
    </ScrollArea>
  )
}

function DockMother() {
  return (
    <div className="flex h-full min-h-0 flex-col border-s">
      <DockHeader mother title="Mother" />
      <Thread dense fromTop>
        <Alert variant="destructive">
          <FlagIcon />
          <AlertTitle>gold_daily failed 3 times in a row</AlertTitle>
          <AlertDescription className="flex flex-col items-start gap-2">
            Its schedule keeps running.
            <Button variant="outline" size="xs">
              Open run log
            </Button>
          </AlertDescription>
        </Alert>
        <UserMessage>Why does gold_daily fail?</UserMessage>
        <AgentMessage footer={<TurnFooter time="09:04" />}>
          <AgentText>The price API now needs a key. Shall I set up a connection for it?</AgentText>
          <ToolChip name="run_status" detail="r_85 · failed at step price · 401" />
        </AgentMessage>
      </Thread>
      <Composer mother placeholder="Message Mother…" className="px-3 pb-3" />
    </div>
  )
}

// 06 · Pipelines tab and a pipeline page in the Steps view (SPEC 6.10).
export function Screen06() {
  return (
    <AppShell leftCollapsed activeChat="mother" rightTab="pipelines" activeItem="daily_btc">
      <ResizablePanelGroup orientation="horizontal">
        <ResizablePanel defaultSize="66%" minSize="40%">
          <PipelineSteps />
        </ResizablePanel>
        <ResizableHandle withHandle />
        <ResizablePanel defaultSize="34%" minSize={320}>
          <DockMother />
        </ResizablePanel>
      </ResizablePanelGroup>
    </AppShell>
  )
}
