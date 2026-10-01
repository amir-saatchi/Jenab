import { memo } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeHighlight from "rehype-highlight";
import { splitBlocks } from "../lib/blocks";

const remarkPlugins = [remarkGfm];
const rehypePlugins = [rehypeHighlight];

/** react-markdown + remark-gfm + rehype-highlight, whole text per render. */
export const Markdown = memo(function Markdown({ src }: { src: string }) {
  return <div className="md"><ReactMarkdown remarkPlugins={remarkPlugins} rehypePlugins={rehypePlugins}>{src}</ReactMarkdown></div>;
});

const Block = memo(function Block({ src }: { src: string }) {
  return <ReactMarkdown remarkPlugins={remarkPlugins} rehypePlugins={rehypePlugins}>{src}</ReactMarkdown>;
});

/** Same renderer, but finished blocks are memoised; only the last block re-parses. */
export const MarkdownBlocks = memo(function MarkdownBlocks({ src }: { src: string }) {
  const blocks = splitBlocks(src);
  return <div className="md">{blocks.map((b, i) => <Block key={i} src={b} />)}</div>;
});
