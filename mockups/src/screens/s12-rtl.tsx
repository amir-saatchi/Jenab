import { Badge } from "@/components/ui/badge"
import { Input } from "@/components/ui/input"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { AppShell } from "@/jenab/shell"
import {
  AgentMessage,
  AgentText,
  ChatHeader,
  Composer,
  Thread,
  ToolChip,
  TurnFooter,
  UserMessage,
} from "@/jenab/chat"
import { Block, PageHeader, Row } from "@/jenab/page"
import type { Chat } from "@/data/bitcoin"

// Synthetic Persian text for the right-to-left check (SPEC 5.8). The UI stays English.
const list: Chat[] = [
  { id: "mother", title: "Mother", sub: "Project home", status: "idle", mother: true },
  { id: "fa", title: "خبرهای بازار", sub: "دیروز · خلاصه‌ی خبرها", status: "idle" },
  { id: "reviewer", title: "Reviewer", sub: "Reviews new pipelines", status: "idle" },
  { id: "eth", title: "ETH tracker", sub: "Tracks ETH daily", status: "idle" },
]

const answer = `قیمت بیت‌کوین در هفته‌ی گذشته **۲٫۱ درصد** بالا رفت. بیشترین افت در ۱۲ سپتامبر بود، هم‌زمان با خبر SEC درباره‌ی ETF.

| روز | قیمت (USD) | تغییر |
|---|---:|---:|
| ۲۹ سپتامبر | 64,210 | +2.1 % |
| ۲۸ سپتامبر | 62,880 | −0.4 % |
| ۲۷ سپتامبر | 63,140 | +0.9 % |

- داده از جدول \`btc_prices\` آمده است.
- پایپ‌لاین \`daily_btc\` هر روز ساعت ۰۸:۰۰ اجرا می‌شود.
- The mixed line: قیمت امروز $64,210 است (up 2.1 %).

\`\`\`sql
SELECT day, close FROM btc_prices ORDER BY day DESC LIMIT 7;
\`\`\``

const rows = [
  { title: "کمیسیون بورس تصمیم درباره‌ی ETF را به تعویق انداخت", source: "coindesk.com", tag: "مهم" },
  { title: "Miners sell as hash price hits a yearly low", source: "theblock.co", tag: "new" },
  { title: "بازارهای آسیا با نگرانی از نرخ بهره پایین‌تر باز شدند", source: "reuters.com", tag: "خوانده شد" },
]

function PersianPage() {
  return (
    <ScrollArea className="h-full">
      <PageHeader title="بیت‌کوین" description="قیمت روزانه و خبرهایی که بیشترین اثر را دارند" />
      <div className="flex flex-col gap-4 px-6 pb-6">
        <Row>
          <Block cols={12} title="News" description="Titles in both directions; each cell picks its own.">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-full">Title</TableHead>
                  <TableHead>Source</TableHead>
                  <TableHead />
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((r) => (
                  <TableRow key={r.title}>
                    <TableCell className="max-w-0 truncate" dir="auto">
                      {r.title}
                    </TableCell>
                    <TableCell className="text-muted-foreground">{r.source}</TableCell>
                    <TableCell>
                      <Badge variant="outline" dir="auto">
                        {r.tag}
                      </Badge>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </Block>
        </Row>
        <Row>
          <Block cols={12} title="Form input">
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="fa-kw">Add keyword</FieldLabel>
                <Input id="fa-kw" dir="auto" defaultValue="هاوینگ ۲۰۲۸" />
              </Field>
            </FieldGroup>
          </Block>
        </Row>
      </div>
    </ScrollArea>
  )
}

// 12 · Right-to-left check: Persian in chat, Markdown, tables, inputs and the chat list.
export function Screen12() {
  return (
    <AppShell activeChat="fa" chatList={list} right={<aside className="w-[26rem] shrink-0 border-s"><PersianPage /></aside>}>
      <div className="flex h-full min-h-0 flex-col">
        <ChatHeader title="خبرهای بازار" sub="دیروز · خلاصه‌ی خبرها" />
        <Thread fromTop>
          <UserMessage>قیمت بیت‌کوین در هفته‌ی گذشته چطور بود؟</UserMessage>
          <AgentMessage footer={<TurnFooter time="10:12" />}>
            <ToolChip name="query" detail="btc_prices · 7 rows · 12 ms" />
            <AgentText>{answer}</AgentText>
          </AgentMessage>
          <UserMessage>Add «هاوینگ» as a keyword, please.</UserMessage>
        </Thread>
        <Composer value="یک کلمه‌ی کلیدی دیگر هم اضافه کن: ماینرها" placeholder="Message…" />
      </div>
    </AppShell>
  )
}
