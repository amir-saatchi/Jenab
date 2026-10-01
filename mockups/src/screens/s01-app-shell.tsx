import { Button } from "@/components/ui/button"
import { AppShell } from "@/jenab/shell"
import {
  AgentMessage,
  AgentText,
  ChatHeader,
  Composer,
  MemoryChip,
  StreamingText,
  Thread,
  ToolChip,
  ToolDetail,
  TurnFooter,
  UserMessage,
} from "@/jenab/chat"
import { ArrowRightIcon } from "lucide-react"

// 01 · App shell, full chat: the Mother chat of the Bitcoin project.
export function Screen01() {
  return (
    <AppShell activeChat="mother" rightTab="pages" statuses={{ mother: "working" }}>
      <div className="flex h-full min-h-0 flex-col">
        <ChatHeader mother title="Mother" sub="Project home · directs the other chats" />
        <Thread>
          <UserMessage>
            Track Bitcoin every day: the price and the news that moves it. And what was the
            price a year ago?
          </UserMessage>
          <AgentMessage footer={<TurnFooter time="10:14" />}>
            <AgentText>
              A year ago Bitcoin closed at about **$114,300** (29 Sep 2025). I'll set up the
              daily tracking now.
            </AgentText>
            <ToolChip name="query" detail="1 row · 40 ms" defaultOpen>
              <ToolDetail label="sql">
                SELECT close FROM btc_prices WHERE day = '2025-09-29'
              </ToolDetail>
              <ToolDetail label="result">1 row · close = 114,300.00</ToolDetail>
            </ToolChip>
            <ToolChip agent name="Schema agent" detail="created btc_prices, news, watch_keywords" />
            <ToolChip name="save_pipeline" detail="daily_btc · every day at 08:00" />
            <ToolChip name="save_page" detail="bitcoin · 6 blocks" />
            <AgentText>
              Done. The pipeline runs every day at 08:00 and fills the Bitcoin page.
            </AgentText>
            <Button variant="outline" size="sm" className="w-fit">
              Open page
              <ArrowRightIcon data-icon="inline-end" />
            </Button>
            <MemoryChip>
              Memory updated: <span className="text-foreground">project goals</span>
            </MemoryChip>
          </AgentMessage>
          <UserMessage>Also add ETH.</UserMessage>
          <AgentMessage>
            <StreamingText>
              Adding ETH means the price table needs a coin column, so the Bitcoin page and
              daily_btc keep working. I'm asking the schema agent for the change
            </StreamingText>
            <ToolChip agent name="Schema agent" detail="designing prices(coin, day, close)" state="running" />
          </AgentMessage>
        </Thread>
        <Composer running mother />
      </div>
    </AppShell>
  )
}
