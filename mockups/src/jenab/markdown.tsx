import ReactMarkdown from "react-markdown"
import type { Element, ElementContent } from "hast"
import rehypeHighlight from "rehype-highlight"
import remarkGfm from "remark-gfm"
import { cn } from "@/lib/utils"

// The direction of the first strong character, like dir="auto" does.
const rtl = /[֐-ࣿיִ-﷿ﹰ-﻿]/
const strong = /[A-Za-zÀ-ɏͰ-ϿЀ-ӿ֐-ࣿיִ-﷿ﹰ-﻿]/
function text(n: ElementContent | Element): string {
  if (n.type === "text") return n.value
  if (n.type === "element") return n.children.map(text).join("")
  return ""
}
function dirOf(node?: Element): "rtl" | "ltr" {
  const c = node ? text(node).match(strong)?.[0] : undefined
  return c && rtl.test(c) ? "rtl" : "ltr"
}

// Rendered Markdown (SPEC 5.8): Typeset styles it, dir="auto" handles
// right-to-left text, wide tables scroll.
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
    <div
      dir="auto"
      className={cn("typeset", preset === "chat" ? "typeset-chat" : "typeset-page", className)}
    >
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={[rehypeHighlight]}
        components={{
          table: ({ node, ...props }) => (
            <div className="typeset-scroll">
              <table dir={dirOf(node)} {...props} />
            </div>
          ),
          p: (props) => <p dir="auto" {...props} />,
          // Lists and items get an explicit direction, so an item that differs
          // from its list can make room for its marker (typeset.css).
          ul: ({ node, ...props }) => <ul dir={dirOf(node)} {...props} />,
          ol: ({ node, ...props }) => <ol dir={dirOf(node)} {...props} />,
          li: ({ node, ...props }) => <li dir={dirOf(node)} {...props} />,
          td: (props) => <td dir="auto" {...props} />,
        }}
      >
        {children}
      </ReactMarkdown>
    </div>
  )
}
