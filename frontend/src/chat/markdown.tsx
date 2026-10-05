import * as React from "react"

import { cn } from "@/lib/utils"

const Blocks = React.lazy(() => import("@/chat/markdown-impl"))

// Markdown is rendered Markdown (SPEC 5.8): Typeset styles it and
// dir="auto" handles right-to-left text. Until the renderer has loaded,
// the plain text shows.
export function Markdown({
  children,
  preset = "chat",
  className,
}: {
  children: string
  preset?: "chat" | "page"
  className?: string
}) {
  return (
    <div dir="auto" className={cn("typeset", preset === "chat" ? "typeset-chat" : "typeset-page", className)}>
      <React.Suspense fallback={<p className="whitespace-pre-wrap">{children}</p>}>
        <Blocks src={children} />
      </React.Suspense>
    </div>
  )
}
