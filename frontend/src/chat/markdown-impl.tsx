import * as React from "react"
import { Browser } from "@wailsio/runtime"
import type { Element, ElementContent } from "hast"
import ReactMarkdown, { type Components } from "react-markdown"
import rehypeHighlight from "rehype-highlight"
import remarkGfm from "remark-gfm"

import { splitBlocks } from "@/lib/blocks"
import { textDir } from "@/lib/dir"

// This module is loaded lazily (SPEC 5.8): react-markdown and the
// highlighter are about 97 KB gzip.

function text(n: ElementContent | Element): string {
  if (n.type === "text") return n.value
  if (n.type === "element") return n.children.map(text).join("")
  return ""
}
function dirOf(node?: Element): "rtl" | "ltr" {
  return node ? textDir(text(node)) : "ltr"
}

// Links open in the browser, not in the app's window; a middle click too.
function open(e: React.MouseEvent<HTMLAnchorElement>) {
  if (e.type === "auxclick" && e.button !== 1) return
  const href = e.currentTarget.href
  e.preventDefault()
  if (/^(https?|mailto):/i.test(href)) void Browser.OpenURL(href)
}

const remarkPlugins = [remarkGfm]
const rehypePlugins = [rehypeHighlight]
const components: Components = {
  a: ({ node: _, ...props }) => <a {...props} onClick={open} onAuxClick={open} />,
  // Only the project's own pictures load; a picture from the web would
  // tell its host the chat was read.
  img: ({ node: _, src, alt }) =>
    typeof src === "string" && src.startsWith("/objects/") ? <img src={src} alt={alt ?? ""} /> : <span>{alt || src}</span>,
  table: ({ node, ...props }) => (
    <div className="typeset-scroll">
      <table dir={dirOf(node)} {...props} />
    </div>
  ),
  p: ({ node: _, ...props }) => <p dir="auto" {...props} />,
  h1: ({ node: _, ...props }) => <h1 dir="auto" {...props} />,
  h2: ({ node: _, ...props }) => <h2 dir="auto" {...props} />,
  h3: ({ node: _, ...props }) => <h3 dir="auto" {...props} />,
  h4: ({ node: _, ...props }) => <h4 dir="auto" {...props} />,
  h5: ({ node: _, ...props }) => <h5 dir="auto" {...props} />,
  h6: ({ node: _, ...props }) => <h6 dir="auto" {...props} />,
  blockquote: ({ node: _, ...props }) => <blockquote dir="auto" {...props} />,
  // Lists and items get an explicit direction, so an item that differs
  // from its list can make room for its marker (typeset.css).
  ul: ({ node, ...props }) => <ul dir={dirOf(node)} {...props} />,
  ol: ({ node, ...props }) => <ol dir={dirOf(node)} {...props} />,
  li: ({ node, ...props }) => <li dir={dirOf(node)} {...props} />,
  td: ({ node: _, ...props }) => <td dir="auto" {...props} />,
  th: ({ node: _, ...props }) => <th dir="auto" {...props} />,
}

const Block = React.memo(function Block({ src }: { src: string }) {
  return (
    <ReactMarkdown remarkPlugins={remarkPlugins} rehypePlugins={rehypePlugins} components={components}>
      {src}
    </ReactMarkdown>
  )
})

// Blocks renders Markdown block by block; a block that didn't change
// isn't parsed again.
export default function Blocks({ src }: { src: string }) {
  return splitBlocks(src).map((b, i) => <Block key={i} src={b} />)
}
