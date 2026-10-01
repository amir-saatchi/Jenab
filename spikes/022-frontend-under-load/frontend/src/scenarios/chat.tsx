// Q1: streaming chat. 200 history messages, then a ~2,000-token Markdown reply
// streamed from Go as Wails events; measures frames, long tasks, input latency,
// event ordering/loss and render cost for three batching modes.
import "highlight.js/styles/github.css";
import { memo, useLayoutEffect, useRef, type ComponentType } from "react";
import { flushSync } from "react-dom";
import { create } from "zustand";
import type { Ctx } from "../main";
import { Shell } from "../shell";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { afterPaint, blockingIn, epoch, eventsIn, FrameRecorder, phase, r1, sleep, summary } from "../lib/metrics";
import { call, on } from "../lib/wails";

type Msg = { id: number; role: string; text: string };
type MD = ComponentType<{ src: string }>;

const useChat = create<{ history: Msg[]; stream: string; composer: string }>(() => ({ history: [], stream: "", composer: "" }));

const commits: { at: number; ms: number; len: number }[] = [];
let scrollEl: HTMLDivElement | null = null;

const HistoryItem = memo(function HistoryItem({ m, M }: { m: Msg; M: MD }) {
  return (
    <div className={m.role === "user" ? "ml-auto max-w-[70%] rounded-lg bg-muted px-3 py-2 text-sm" : "max-w-[85%]"}>
      {m.role === "user" ? m.text : <M src={m.text} />}
    </div>
  );
});

function History({ M }: { M: MD }) {
  const history = useChat((s) => s.history);
  return <>{history.map((m) => <HistoryItem key={m.id} m={m} M={M} />)}</>;
}

function Streaming({ M }: { M: MD }) {
  const text = useChat((s) => s.stream);
  const t0 = performance.now();
  useLayoutEffect(() => {
    if (!text) return;
    // stick to bottom, as a chat does while streaming (forces layout)
    if (scrollEl) scrollEl.scrollTop = scrollEl.scrollHeight;
    commits.push({ at: t0, ms: performance.now() - t0, len: text.length });
  });
  if (!text) return null;
  return <div className="max-w-[85%]"><M src={text} /></div>;
}

const keyLat: number[] = [];
let trustedKeys = 0;

function Composer({ taRef }: { taRef: React.RefObject<HTMLTextAreaElement | null> }) {
  const value = useChat((s) => s.composer);
  return (
    <div className="flex gap-2 border-t p-3">
      <Textarea
        ref={taRef}
        value={value}
        rows={2}
        placeholder="Message"
        onChange={(e) => useChat.setState({ composer: e.target.value })}
        onKeyDown={(e) => {
          if (!e.isTrusted) return;
          trustedKeys++;
          const t0 = e.timeStamp;
          afterPaint().then((p) => keyLat.push(p - t0));
        }}
      />
      <Button>Send</Button>
    </div>
  );
}

function Chat({ M, S, taRef }: { M: MD; S: MD; taRef: React.RefObject<HTMLTextAreaElement | null> }) {
  const ref = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => { scrollEl = ref.current; });
  return (
    <div className="flex h-full flex-col">
      <div ref={ref} className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto p-4">
        <History M={M} />
        <Streaming M={S} />
      </div>
      <Composer taRef={taRef} />
    </div>
  );
}

export async function run({ root, params, info }: Ctx) {
  const rate = Number(params.get("rate") ?? 100);
  const mode = params.get("mode") ?? "raf"; // token | sync | raf | tNN (e.g. t50)
  const md = params.get("md") ?? "rm"; // rm | rmb | marked | markedn
  const histN = Number(params.get("hist") ?? 200);
  const realKeys = params.get("keys") !== "0";

  const mdMod = md.startsWith("marked") ? await import("./md-marked") : await import("./md-rm");
  const M = mdMod.Markdown;
  const S = md === "rmb" || md === "marked" ? mdMod.MarkdownBlocks : mdMod.Markdown;

  const taRef: React.RefObject<HTMLTextAreaElement | null> = { current: null };
  root.render(<Shell title={`chat ${rate}/s ${mode} ${md}`}><Chat M={M} S={S} taRef={taRef} /></Shell>);
  await afterPaint();

  // history: load via binding, render, measure
  let t = performance.now();
  const hist: Msg[] = await call("History", histN);
  const histLoadMs = performance.now() - t;
  const histBytes = JSON.stringify(hist).length;
  t = performance.now();
  useChat.setState({ history: hist });
  const histRenderMs = (await afterPaint()) - t;
  const histBlocking = blockingIn(t, performance.now());
  if (scrollEl) scrollEl.scrollTop = scrollEl.scrollHeight;
  const replyText: string = await call("ReplyText");
  const domNodesBefore = document.getElementsByTagName("*").length;
  await sleep(500);

  // idle frames for reference
  const idle = new FrameRecorder();
  idle.start();
  await sleep(1000);
  const idleFrames = idle.stop();

  // subscribe to tokens
  const seqs: number[] = [];
  const lat: number[] = [];
  let buf: string[] = [];
  let scheduled = false;
  let flushes = 0;
  const append = (s: string) => useChat.setState((st) => ({ stream: st.stream + s }));
  const flush = () => {
    scheduled = false;
    if (!buf.length) return;
    const s = buf.join("");
    buf = [];
    flushes++;
    flushSync(() => append(s));
  };
  let endStats: any = null;
  const offTok = on("tok", (d) => {
    lat.push(epoch() - d.ts);
    seqs.push(d.i);
    if (mode === "token") append(d.t);
    else if (mode === "sync") flushSync(() => append(d.t));
    else {
      buf.push(d.t);
      if (!scheduled) {
        scheduled = true;
        // raf: flush once per frame; tNN: flush at most every NN ms
        if (mode === "raf") requestAnimationFrame(flush);
        else setTimeout(flush, Number(mode.slice(1)));
      }
    }
  });
  const offEnd = on("tok-end", (d) => { endStats = d; });

  // input: synthetic typing (timer -> state -> paint) and real key presses from Go
  const synthLate: number[] = [];
  const synthE2P: number[] = [];
  let typing = true;
  const synth = (async () => {
    while (typing) {
      const due = performance.now() + 120;
      await sleep(120);
      if (!typing) break;
      synthLate.push(performance.now() - due);
      useChat.setState((s) => ({ composer: s.composer.length > 200 ? "x" : s.composer + "x" }));
      synthE2P.push((await afterPaint()) - due);
    }
  })();

  commits.length = 0;
  const fr = new FrameRecorder();
  const tStart = performance.now();
  const runId = `${md}-${mode}-${rate}`;
  taRef.current?.focus();
  let streamMs = 0;
  await phase("stream", async () => {
    fr.start();
    await call("StartStream", runId, rate);
    const n = info.replyTokens as number;
    if (realKeys) await call("TypeKeys", Math.max(5, Math.floor((n / rate) * 1000 / 150)), 150);
    const deadline = performance.now() + (n / rate) * 1000 * 3 + 15000;
    while ((!endStats || seqs.length < n) && performance.now() < deadline) await sleep(20);
    if (mode === "raf" || /^t[0-9]+$/.test(mode)) flush();
    await afterPaint();
    streamMs = performance.now() - tStart;
    await sleep(300);
  });
  typing = false;
  await synth;
  const tEnd = performance.now();
  const frames = fr.stop();
  offTok();
  offEnd();

  // ordering / loss
  let outOfOrder = 0, dupes = 0;
  const seen = new Set<number>();
  for (let i = 0; i < seqs.length; i++) {
    if (seen.has(seqs[i])) dupes++;
    seen.add(seqs[i]);
    if (i > 0 && seqs[i] < seqs[i - 1]) outOfOrder++;
  }
  const n = endStats?.n ?? 0;
  const missing = n - seen.size;
  const finalText = useChat.getState().stream;
  const ev = eventsIn(tStart, tEnd);

  return {
    rate, mode, md,
    history: { messages: hist.length, loadMs: r1(histLoadMs), bytes: histBytes, renderMs: r1(histRenderMs), blocking: histBlocking, domNodes: domNodesBefore },
    idleFrames,
    stream: {
      tokensSent: n, tokensReceived: seqs.length, missing, dupes, outOfOrder,
      textIntact: finalText === replyText, replyChars: replyText.length,
      durationMs: r1(streamMs), goEmit: endStats,
      emitToReceiveMs: summary(lat),
    },
    frames,
    blocking: blockingIn(tStart, tEnd),
    render: { commits: commits.length, flushes, commitMs: summary(commits.map((c) => c.ms)) },
    input: {
      synthTimerLateMs: summary(synthLate),
      synthEventToPaintMs: summary(synthE2P),
      trustedKeys, realKeyToPaintMs: summary(keyLat),
      eventTimingOver16: ev.length,
      eventTimingMaxMs: ev.length ? Math.max(...ev.map((e) => e.duration)) : 0,
      eventTimingMaxInputDelayMs: ev.length ? r1(Math.max(...ev.map((e) => e.inputDelay))) : 0,
    },
    domNodesAfter: document.getElementsByTagName("*").length,
  };
}
