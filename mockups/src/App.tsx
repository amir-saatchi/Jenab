import { Screen01 } from "@/screens/s01-app-shell"
import { Screen02 } from "@/screens/s02-page-docked"
import { Screen03 } from "@/screens/s03-turn-anatomy"
import { Screen04 } from "@/screens/s04-approvals"
import { Screen05 } from "@/screens/s05-mother-delegates"
import { Screen06 } from "@/screens/s06-pipelines"
import { Screen06A } from "@/screens/s06a-pipeline-flow"
import { Screen07 } from "@/screens/s07-settings"
import { Screen08 } from "@/screens/s08-project-settings"
import { Screen09 } from "@/screens/s09-first-run"
import { Screen10 } from "@/screens/s10-turn-inspector"
import { NarrowWindow, Screen11 } from "@/screens/s11-narrow-window"
import { Screen12 } from "@/screens/s12-rtl"

// Each screen is a URL: ?screen=01&theme=dark
const screens: Record<string, { title: string; view: () => React.ReactElement }> = {
  "01": { title: "App shell, full chat", view: Screen01 },
  "02": { title: "Page open, chat docked", view: Screen02 },
  "03": { title: "Anatomy of a turn", view: Screen03 },
  "04": { title: "Approval card and question form", view: Screen04 },
  "05": { title: "Mother delegates", view: Screen05 },
  "06": { title: "Pipelines and run log", view: Screen06 },
  "06A": { title: "Pipeline as a flow", view: Screen06A },
  "07": { title: "Settings", view: Screen07 },
  "08": { title: "Project settings", view: Screen08 },
  "09": { title: "First run", view: Screen09 },
  "10": { title: "Turn inspector", view: Screen10 },
  "11": { title: "Narrow window", view: Screen11 },
  "12": { title: "Right-to-left check", view: Screen12 },
}

export default function App() {
  const id = new URLSearchParams(location.search).get("screen")
  if (id === "11w") return <NarrowWindow />
  const screen = id ? screens[id] : undefined
  if (screen) return <screen.view />

  return (
    <div className="mx-auto flex max-w-xl flex-col gap-4 p-10">
      <h1 className="text-lg font-medium">Jenab high-fidelity mockups</h1>
      <ul className="flex flex-col gap-2 text-sm">
        {Object.entries(screens).map(([key, s]) => (
          <li key={key} className="flex gap-3">
            <span className="w-8 text-muted-foreground">{key}</span>
            <span className="flex-1">{s.title}</span>
            <a className="underline" href={`?screen=${key}&theme=light`}>
              light
            </a>
            <a className="underline" href={`?screen=${key}&theme=dark`}>
              dark
            </a>
          </li>
        ))}
      </ul>
    </div>
  )
}
