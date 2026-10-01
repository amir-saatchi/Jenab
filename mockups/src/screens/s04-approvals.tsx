import { CircleAlertIcon, DiamondIcon } from "lucide-react"

import {
  ApprovalCard,
  ApprovalRecord,
  ConnectionCard,
  DesktopNotification,
  DetailList,
  QuestionForm,
  WaitingBar,
} from "@/jenab/approvals"
import { Composer } from "@/jenab/chat"

function Label({ children }: { children: React.ReactNode }) {
  return <h2 className="text-xs font-medium text-muted-foreground">{children}</h2>
}

// 04 · Approval card and question form: the parts that make a turn wait,
// shown without the rest of the chat.
export function Screen04() {
  return (
    <div className="grid h-svh grid-cols-2 content-start gap-x-8 gap-y-4 overflow-hidden bg-muted/40 px-8 py-6">
      <div className="flex flex-col gap-4">
        <Label>Approvals in the chat</Label>
        <ApprovalCard
          kind="Approval · New host"
          title="Allow api.coingecko.com?"
          why="Pipeline daily_btc gets the price from this host."
          risk="Jenab blocks hosts until you allow them. Private addresses stay blocked."
          details={
            <DetailList
              rows={[
                ["Used by", "daily_btc, step fetch_price"],
                ["Requests", "1 per run, GET only"],
              ]}
            />
          }
          allow="Allow for this project"
          note
        />
        <ApprovalCard
          kind="Approval · Schema change"
          title="Merge btc_prices into prices(coin, …)?"
          why="You asked to track ETH too. One table for all coins keeps the views simple."
          detailsOpen
          details={
            <DetailList
              rows={[
                ["Plan", "Create prices, copy 92 rows with coin = BTC, drop btc_prices"],
                ["Destructive", "Drops table btc_prices", true],
                ["Updates", "3 views, 1 pipeline, 1 page filter"],
                ["Safety", "A snapshot is taken first · about 1 s"],
              ]}
            />
          }
          allow="Approve"
        />
        <ApprovalRecord>Allowed api.coingecko.com for this project · 10:14</ApprovalRecord>
      </div>
      <div className="flex flex-col gap-4">
        <Label>Connection and question</Label>
        <ConnectionCard />
        <QuestionForm />
      </div>
      <div className="col-span-2 grid grid-cols-[16rem_20rem_1fr] items-start gap-6">
        <div className="flex flex-col gap-2">
          <Label>Chat list, chat not on screen</Label>
          <div className="flex items-center gap-2 rounded-lg border bg-sidebar p-2 text-sm">
            <DiamondIcon className="size-4 fill-current" />
            <div className="grid flex-1 leading-tight">
              <span>Mother</span>
              <span className="text-xs text-tone-amber">Waiting for you</span>
            </div>
            <CircleAlertIcon className="size-4 text-tone-amber" />
          </div>
        </div>
        <div className="flex flex-col gap-2">
          <Label>Desktop notification</Label>
          <DesktopNotification />
        </div>
        <div className="flex flex-col gap-2">
          <Label>Card scrolled out of view: bar above the composer</Label>
          <div className="flex flex-col gap-2 rounded-xl border bg-background p-3">
            <WaitingBar>
              Waiting for your approval: <span className="font-medium">Allow api.coingecko.com?</span>
            </WaitingBar>
            <Composer mother compact placeholder="Message Mother…" className="max-w-none px-0 pb-0" />
          </div>
        </div>
      </div>
    </div>
  )
}
