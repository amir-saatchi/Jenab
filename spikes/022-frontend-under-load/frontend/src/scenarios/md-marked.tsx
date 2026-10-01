import { memo, useMemo } from "react";
import { Marked, type Tokens } from "marked";
import hljs from "highlight.js/lib/core";
import go from "highlight.js/lib/languages/go";
import typescript from "highlight.js/lib/languages/typescript";
import sql from "highlight.js/lib/languages/sql";
import bash from "highlight.js/lib/languages/bash";
import json from "highlight.js/lib/languages/json";
import python from "highlight.js/lib/languages/python";
import yaml from "highlight.js/lib/languages/yaml";
import DOMPurify from "dompurify";
import { splitBlocks } from "../lib/blocks";

hljs.registerLanguage("go", go);
hljs.registerLanguage("typescript", typescript);
hljs.registerLanguage("sql", sql);
hljs.registerLanguage("bash", bash);
hljs.registerLanguage("json", json);
hljs.registerLanguage("python", python);
hljs.registerLanguage("yaml", yaml);

const esc = (s: string) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");

const md = new Marked({
  gfm: true,
  renderer: {
    code({ text, lang }: Tokens.Code) {
      const l = lang && hljs.getLanguage(lang) ? lang : "";
      const html = l ? hljs.highlight(text, { language: l, ignoreIllegals: true }).value : esc(text);
      return `<pre><code class="hljs language-${l}">${html}</code></pre>`;
    },
  },
});

// LLM output is untrusted: sanitise the HTML before it touches the DOM.
const toHtml = (src: string) => DOMPurify.sanitize(md.parse(src, { async: false }) as string);

const Html = memo(function Html({ src }: { src: string }) {
  const html = useMemo(() => toHtml(src), [src]);
  return <div dangerouslySetInnerHTML={{ __html: html }} />;
});

/** marked + highlight.js + DOMPurify, whole text per render. */
export const Markdown = memo(function Markdown({ src }: { src: string }) {
  return <div className="md"><Html src={src} /></div>;
});

/** marked, finished blocks memoised. */
export const MarkdownBlocks = memo(function MarkdownBlocks({ src }: { src: string }) {
  const blocks = splitBlocks(src);
  return <div className="md">{blocks.map((b, i) => <Html key={i} src={b} />)}</div>;
});
