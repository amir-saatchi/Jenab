import { PencilIcon, Trash2Icon } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from "@/components/ui/resizable"
import { ScrollArea } from "@/components/ui/scroll-area"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { AppShell } from "@/jenab/shell"
import {
  AgentMessage,
  AgentText,
  Composer,
  DockHeader,
  Notice,
  Thread,
  ToolChip,
  TurnFooter,
  UserMessage,
} from "@/jenab/chat"
import {
  Block,
  DateFilter,
  PageHeader,
  PriceChart,
  RefreshButton,
  Row,
  StatBlock,
  TablePager,
  priceSeries,
} from "@/jenab/page"

const fmt = (n: number) => "$" + n.toLocaleString("en-US")
const months = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"]
const dayLabel = (iso: string) => `${Number(iso.slice(8))} ${months[Number(iso.slice(5, 7)) - 1]}`

const news = [
  { title: "SEC delays decision on bitcoin ETF options", source: "coindesk.com", status: "new", selected: true },
  { title: "Miners sell as hash price hits a yearly low", source: "theblock.co", status: "new", selected: true },
  { title: "Asian markets open lower on rate fears", source: "reuters.com", status: "read" },
  { title: "Large wallet moves 12,000 BTC to an exchange", source: "whale-alert.io", status: "read" },
  { title: "Options expiry: $4.1 B in BTC contracts", source: "deribit.com", status: "read" },
]

const keywords = ["bitcoin ETF", "halving", "SEC"]

export function PageBitcoin() {
  const last = priceSeries.slice(-6).reverse()
  return (
    <ScrollArea className="h-full">
      <PageHeader title="Bitcoin" description="Daily price and the news most likely to move it">
        <DateFilter label="From" value="30 Jun 2026" />
        <RefreshButton />
      </PageHeader>
      <div className="flex flex-col gap-4 px-6 pb-6">
        <Row>
          <Block cols={8} title="Price, 90 days">
            <PriceChart />
          </Block>
          <StatBlock cols={4} title="Today" value="$64,210" change={2.1} caption="Last update 08:00" />
        </Row>
        <Row>
          <Block cols={5} title="Prices">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Date</TableHead>
                  <TableHead className="text-end">Close</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {last.map((p) => (
                  <TableRow key={p.day}>
                    <TableCell>{dayLabel(p.day)}</TableCell>
                    <TableCell className="text-end tabular-nums">{fmt(p.close)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            <TablePager page={1} pages={15} />
          </Block>
          <Block cols={7} title="News">
            <div className="mb-2 flex items-center gap-2 rounded-lg bg-muted px-2 py-1 text-xs">
              <span className="text-muted-foreground">2 selected</span>
              <Button variant="outline" size="xs">
                Mark read
              </Button>
            </div>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-8">
                    <Checkbox aria-label="Select all" checked="indeterminate" />
                  </TableHead>
                  <TableHead className="w-full">Title</TableHead>
                  <TableHead>Source</TableHead>
                  <TableHead className="w-14" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {news.map((n) => (
                  <TableRow key={n.title} data-state={n.selected ? "selected" : undefined}>
                    <TableCell>
                      <Checkbox aria-label="Select row" checked={!!n.selected} />
                    </TableCell>
                    <TableCell className="max-w-0 truncate" dir="auto">
                      {n.title}
                    </TableCell>
                    <TableCell className="text-muted-foreground">{n.source}</TableCell>
                    <TableCell>
                      <Badge variant={n.status === "new" ? "secondary" : "outline"}>{n.status}</Badge>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </Block>
        </Row>
        <Row>
          <Block cols={12} title="Keywords" description="News is kept when it matches one of these.">
            <div className="grid grid-cols-2 gap-4">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Keyword</TableHead>
                    <TableHead className="w-20" />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {keywords.map((k) => (
                    <TableRow key={k}>
                      <TableCell dir="auto">{k}</TableCell>
                      <TableCell className="text-end">
                        <Button variant="ghost" size="icon-xs" aria-label="Edit">
                          <PencilIcon />
                        </Button>
                        <Button variant="ghost" size="icon-xs" aria-label="Delete">
                          <Trash2Icon />
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
              <form className="flex flex-col gap-3">
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor="kw">Add keyword</FieldLabel>
                    <Input id="kw" placeholder="e.g. mining" dir="auto" />
                  </Field>
                </FieldGroup>
                <Button size="sm" className="w-fit">
                  Add
                </Button>
              </form>
            </div>
          </Block>
        </Row>
      </div>
    </ScrollArea>
  )
}

export function DockChat() {
  return (
    <div className="flex h-full min-h-0 flex-col border-s">
      <DockHeader mother title="Mother" />
      <Thread dense>
        <Notice>You opened Bitcoin · From = 30 Jun 2026</Notice>
        <UserMessage>Why did the price drop on 12 Sep?</UserMessage>
        <AgentMessage footer={<TurnFooter time="10:31" />}>
          <AgentText>
            Three news items that morning mention the SEC delaying its ruling on ETF options.
            The drop started the same hour.
          </AgentText>
          <ToolChip name="query" detail="news · 3 rows" />
        </AgentMessage>
      </Thread>
      <Composer mother placeholder="Ask about this page…" className="px-3 pb-3" />
    </div>
  )
}

// 02 · Page open, chat docked.
export function Screen02() {
  return (
    <AppShell leftCollapsed activeChat="mother" rightTab="pages" activeItem="bitcoin">
      <ResizablePanelGroup orientation="horizontal">
        <ResizablePanel defaultSize="66%" minSize="40%">
          <PageBitcoin />
        </ResizablePanel>
        <ResizableHandle withHandle />
        <ResizablePanel defaultSize="34%" minSize={320}>
          <DockChat />
        </ResizablePanel>
      </ResizablePanelGroup>
    </AppShell>
  )
}
