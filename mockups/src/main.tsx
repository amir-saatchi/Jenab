import { StrictMode } from "react"
import { createRoot } from "react-dom/client"

import "./index.css"
import App from "./App.tsx"
import { TooltipProvider } from "@/components/ui/tooltip"

// The theme comes from the URL so screenshots are repeatable.
const theme = new URLSearchParams(location.search).get("theme")
document.documentElement.classList.toggle("dark", theme === "dark")

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <TooltipProvider>
      <App />
    </TooltipProvider>
  </StrictMode>
)
