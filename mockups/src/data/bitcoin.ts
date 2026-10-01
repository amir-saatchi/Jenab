// Fake data for the mockups: the Bitcoin example from SPEC 9.

export type ChatStatus = "idle" | "working" | "waiting"

export type Chat = {
  id: string
  title: string
  sub: string
  status: ChatStatus
  mother?: boolean
}

export const project = {
  name: "Bitcoin",
  initial: "B",
  folder: "~/Jenab/Bitcoin",
}

export const otherProjects = [
  { name: "Home budget", initial: "H" },
  { name: "Reading list", initial: "R" },
]

export const chats: Chat[] = [
  { id: "mother", title: "Mother", sub: "Project home", status: "idle", mother: true },
  { id: "reviewer", title: "Reviewer", sub: "Reviews new pipelines", status: "idle" },
  { id: "eth", title: "ETH tracker", sub: "Tracks ETH daily", status: "working" },
  { id: "fees", title: "Fees question", sub: "Waiting for you", status: "waiting" },
  { id: "import", title: "Import old prices", sub: "Yesterday", status: "idle" },
]

export const pages = [
  { id: "bitcoin", title: "Bitcoin", blocks: 6 },
  { id: "eth", title: "ETH", blocks: 4 },
  { id: "gold", title: "Gold", blocks: 3 },
]

export const pipelines = [
  { id: "daily_btc", schedule: "Every day 08:00", last: "ok" as const, lastText: "today 08:00" },
  { id: "eth_daily", schedule: "Every day 08:05", last: "ok" as const, lastText: "today 08:05" },
  { id: "news_backfill", schedule: "Once", last: "running" as const, lastText: "running 0:24" },
  { id: "gold_daily", schedule: "Every day 09:00", last: "flagged" as const, lastText: "failed 3× in a row" },
]

export const tray = "2 schedules on · next 08:00"
