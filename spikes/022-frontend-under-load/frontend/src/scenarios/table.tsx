// Q2: 10,000 rows x 12 columns loaded from Go in 500-row pages, TanStack Table v9
// with virtual rows. Measures page loads, scroll frames, client-side sort and filter.
import { useLayoutEffect, useRef, useState } from "react";
import { flushSync } from "react-dom";
import {
  columnFilteringFeature,
  columnSizingFeature,
  createColumnHelper,
  createFilteredRowModel,
  createSortedRowModel,
  filterFn_equalsString,
  filterFn_includesString,
  filterFn_inNumberRange,
  globalFilteringFeature,
  rowSortingFeature,
  sortFn_alphanumeric,
  sortFn_basic,
  sortFn_text,
  tableFeatures,
  useTable,
} from "@tanstack/react-table";
import { useVirtualizer } from "@tanstack/react-virtual";
import type { Ctx } from "../main";
import { Shell } from "../shell";
import { afterPaint, blockingIn, FrameRecorder, nextFrame, phase, r1, sleep, summary } from "../lib/metrics";
import { call } from "../lib/wails";

type Row = { id: number; name: string; email: string; city: string; status: string; amount: number; qty: number; ratio: number; created: string; updated: string; score: number; note: string };

const features = tableFeatures({
  rowSortingFeature,
  columnFilteringFeature,
  columnSizingFeature,
  globalFilteringFeature,
  sortedRowModel: createSortedRowModel(),
  filteredRowModel: createFilteredRowModel(),
  sortFns: { alphanumeric: sortFn_alphanumeric, text: sortFn_text, basic: sortFn_basic },
  filterFns: { includesString: filterFn_includesString, inNumberRange: filterFn_inNumberRange, equalsString: filterFn_equalsString },
});

const eur = new Intl.NumberFormat("en-US", { style: "currency", currency: "EUR", maximumFractionDigits: 2 });
const pctF = new Intl.NumberFormat("en-US", { style: "percent", maximumFractionDigits: 1 });
const numF = new Intl.NumberFormat("en-US", { maximumFractionDigits: 2 });
const dateF = new Intl.DateTimeFormat("en-GB", { dateStyle: "medium" });
const dtF = new Intl.DateTimeFormat("en-GB", { dateStyle: "short", timeStyle: "short" });

const h = createColumnHelper<typeof features, Row>();
const columns = h.columns([
  h.accessor("id", { header: "ID", size: 60, sortFn: "basic" }),
  h.accessor("name", { header: "Name", size: 130, sortFn: "text" }),
  h.accessor("email", { header: "Email", size: 200, sortFn: "text" }),
  h.accessor("city", { header: "City", size: 90, sortFn: "text" }),
  h.accessor("status", { header: "Status", size: 70, filterFn: "equalsString" }),
  h.accessor("amount", { header: "Amount", size: 100, sortFn: "basic", filterFn: "inNumberRange", cell: (c) => eur.format(c.getValue()) }),
  h.accessor("qty", { header: "Qty", size: 50, sortFn: "basic" }),
  h.accessor("ratio", { header: "Ratio", size: 60, sortFn: "basic", cell: (c) => pctF.format(c.getValue()) }),
  h.accessor("created", { header: "Created", size: 100, sortFn: "text", cell: (c) => dateF.format(new Date(c.getValue())) }),
  h.accessor("updated", { header: "Updated", size: 120, sortFn: "text", cell: (c) => dtF.format(new Date(c.getValue())) }),
  h.accessor("score", { header: "Score", size: 60, sortFn: "basic", cell: (c) => numF.format(c.getValue()) }),
  h.accessor("note", { header: "Note", size: 200, enableSorting: false }),
]);

const ROW_H = 32;
let api: any = null;
let scroller: HTMLDivElement | null = null;
const renders: number[] = [];

function Grid({ data }: { data: Row[] }) {
  const t0 = performance.now();
  const table = useTable({
    features, columns, data,
    globalFilterFn: "includesString",
    getColumnCanGlobalFilter: (col) => ["name", "email", "city", "note"].includes(col.id),
  });
  const rows = table.getRowModel().rows;
  const ref = useRef<HTMLDivElement>(null);
  const v = useVirtualizer({ count: rows.length, getScrollElement: () => ref.current, estimateSize: () => ROW_H, overscan: 10 });
  api = table;
  useLayoutEffect(() => { scroller = ref.current; renders.push(performance.now() - t0); });
  const width = table.getAllLeafColumns().reduce((a, c) => a + c.getSize(), 0);
  return (
    <div ref={ref} className="h-full overflow-auto text-sm" data-testid="grid">
      <div style={{ width }} className="sticky top-0 z-10 flex border-b bg-background font-medium">
        {table.getHeaderGroups()[0].headers.map((hd) => (
          <div key={hd.id} style={{ width: hd.getSize() }} className="cursor-pointer truncate px-2 py-1.5" onClick={hd.column.getToggleSortingHandler()}>
            <table.FlexRender header={hd} />
            {({ asc: " ▲", desc: " ▼" } as any)[hd.column.getIsSorted() as string] ?? ""}
          </div>
        ))}
      </div>
      <div style={{ height: v.getTotalSize(), width, position: "relative" }}>
        {v.getVirtualItems().map((vi) => {
          const row = rows[vi.index];
          return (
            <div key={row.id} className="absolute left-0 flex border-b hover:bg-muted/50" style={{ height: ROW_H, transform: `translateY(${vi.start}px)`, width }}>
              {row.getAllCells().map((cell) => (
                <div key={cell.id} style={{ width: cell.column.getSize() }} className={"truncate px-2 leading-8 " + (typeof cell.getValue() === "number" ? "text-right tabular-nums" : "")}>
                  <table.FlexRender cell={cell} />
                </div>
              ))}
            </div>
          );
        })}
      </div>
    </div>
  );
}

let setData: (d: Row[]) => void = () => {};
function TableView() {
  const [data, set] = useState<Row[]>([]);
  setData = set;
  return <div className="h-full p-2"><Grid data={data} /></div>;
}

const toRows = (cols: string[], arr: any[][]): Row[] => arr.map((a) => { const o: any = {}; for (let i = 0; i < cols.length; i++) o[cols[i]] = a[i]; return o; });

async function timed(fn: () => void) {
  const t0 = performance.now();
  flushSync(fn);
  const commit = performance.now() - t0;
  const paint = (await afterPaint()) - t0;
  return { commit, paint };
}

export async function run({ root, params }: Ctx) {
  const pageSize = Number(params.get("page") ?? 500);
  const total = 10000;
  root.render(<Shell title="table 10k x 12"><TableView /></Shell>);
  await afterPaint();

  // load pages sequentially, appending each to the table as it arrives
  const pageMs: number[] = [], pageBytes: number[] = [], appendPaint: number[] = [];
  let all: Row[] = [];
  const tLoad = performance.now();
  await phase("load", async () => {
    for (let off = 0; off < total; off += pageSize) {
      const t0 = performance.now();
      const p = await call("GetRows", off, pageSize);
      pageMs.push(performance.now() - t0);
      pageBytes.push(JSON.stringify(p).length);
      all = all.concat(toRows(p.columns, p.rows));
      const t1 = performance.now();
      setData(all);
      appendPaint.push((await afterPaint()) - t1);
    }
  });
  const loadAllMs = performance.now() - tLoad;
  const loadBlocking = blockingIn(tLoad, performance.now());
  await sleep(500);

  // scroll the whole list programmatically, a fixed step per frame
  const step = Number(params.get("step") ?? 200);
  const el = scroller!;
  const fr = new FrameRecorder();
  const tS = performance.now();
  let blankChecks = 0, blank = 0;
  await phase("scroll", async () => {
    fr.start();
    while (el.scrollTop + el.clientHeight < el.scrollHeight - 1) {
      el.scrollTop += step;
      await nextFrame();
      // after the frame: is the viewport covered by rendered rows?
      if (++blankChecks % 10 === 0) {
        const probe = document.elementFromPoint(el.getBoundingClientRect().left + 30, el.getBoundingClientRect().top + el.clientHeight - 10);
        if (!probe || probe === el || probe.parentElement === el) blank++;
      }
    }
    await sleep(200);
  });
  const scrollFrames = fr.stop();
  const scrollBlocking = blockingIn(tS, performance.now());
  el.scrollTop = 0;
  await sleep(300);

  // client-side sort and filter on 10k rows (flushSync: commit time; afterPaint: to next frame)
  const ops: { op: string; commitMs: number; paintMs: number; rows: number }[] = [];
  const doOp = async (op: string, fn: () => void) => {
    const { commit, paint } = await timed(fn);
    ops.push({ op, commitMs: r1(commit), paintMs: r1(paint), rows: api.getRowModel().rows.length });
    await sleep(150);
  };
  await phase("sortfilter", async () => {
    for (let rep = 0; rep < 3; rep++) {
      await doOp("sort amount asc", () => api.setSorting([{ id: "amount", desc: false }]));
      await doOp("sort amount desc", () => api.setSorting([{ id: "amount", desc: true }]));
      await doOp("sort name asc", () => api.setSorting([{ id: "name", desc: false }]));
      await doOp("sort created desc", () => api.setSorting([{ id: "created", desc: true }]));
      await doOp("sort updated asc", () => api.setSorting([{ id: "updated", desc: false }]));
      await doOp("sort clear", () => api.setSorting([]));
      await doOp("global 'a'", () => api.setGlobalFilter("a"));
      await doOp("global 'an'", () => api.setGlobalFilter("an"));
      await doOp("global 'ann'", () => api.setGlobalFilter("ann"));
      await doOp("global clear", () => api.setGlobalFilter(""));
      await doOp("amount 100-500", () => api.setColumnFilters([{ id: "amount", value: [100, 500] }]));
      await doOp("status = active", () => api.setColumnFilters([{ id: "status", value: "active" }]));
      await doOp("filters clear", () => api.setColumnFilters([]));
      await doOp("sort amount + global 'an'", () => { api.setSorting([{ id: "amount", desc: false }]); api.setGlobalFilter("an"); });
      await doOp("reset", () => { api.setSorting([]); api.setGlobalFilter(""); });
    }
  });
  const byOp: Record<string, any> = {};
  for (const o of ops) (byOp[o.op] ??= { commitMs: [], paintMs: [], rows: o.rows }).commitMs.push(o.commitMs), byOp[o.op].paintMs.push(o.paintMs);
  for (const k in byOp) byOp[k] = { rows: byOp[k].rows, commitMs: summary(byOp[k].commitMs), paintMs: summary(byOp[k].paintMs) };

  return {
    rows: all.length, pageSize,
    load: { pages: pageMs.length, callMs: summary(pageMs), bytesPerPage: summary(pageBytes), appendToPaintMs: summary(appendPaint), totalMs: r1(loadAllMs), blocking: loadBlocking },
    scroll: { step, frames: scrollFrames, blocking: scrollBlocking, blankViewportChecks: blankChecks / 10 | 0, blankViewport: blank, virtualRowsRendered: el.querySelectorAll("[class*='translate'], .absolute").length },
    sortFilter: byOp,
    sortFilterWorstPaintMs: Math.max(...ops.map((o) => o.paintMs)),
    renderMs: summary(renders),
  };
}
