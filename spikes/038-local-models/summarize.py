"""SPIKE-038: summarize speed, RAM, scenario reports and schema logs."""
import csv, glob, json, os, sys
from collections import defaultdict

S = os.path.dirname(os.path.abspath(__file__))
R = os.path.join(S, "results")

print("## Speed (speed.jsonl)")
print("| model | target | prompt tok | read s | read tok/s | out tok | write s | write tok/s | load s | wall s | note |")
print("|---|---|---|---|---|---|---|---|---|---|---|")
for line in open(os.path.join(R, "speed.jsonl"), encoding="utf-8"):
    r = json.loads(line)
    print(f"| {r['model']} | {r['target'] or 'warm-up'} | {r['prompt_tokens']} | {r['prompt_seconds']:.1f} | {r['prompt_per_second']:.1f} | {r['output_tokens']} | {r['output_seconds']:.1f} | {r['output_per_second']:.1f} | {r['load_seconds']:.1f} | {r['wall_seconds']:.1f} | {r.get('done_reason','')}{r.get('error','')} |")

print("\n## RAM (ram.csv): peak per model")
peak = defaultdict(lambda: [0, 0, 0, 10**9])
p = os.path.join(R, "ram.csv")
if os.path.exists(p):
    for row in csv.DictReader(open(p, encoding="utf-8-sig")):
        m = row["model"].strip(";")
        if not m:
            continue
        v = peak[m]
        v[0] = max(v[0], int(row["ps_size_mib"]))
        v[1] = max(v[1], int(row["runner_ws_mib"]))
        v[2] = max(v[2], int(row["runner_private_mib"]))
        v[3] = min(v[3], int(row["available_mib"]))
for m, v in peak.items():
    print(f"- {m}: /api/ps size {v[0]} MiB, Ollama processes' working set {v[1]} MiB, private {v[2]} MiB; least available on the PC {v[3]} MiB")

def reports(d):
    out = []
    for f in sorted(glob.glob(os.path.join(d, "*.json"))):
        out.append(json.load(open(f, encoding="utf-8")))
    return out

print("\n## Scenario runs")
for md in sorted(glob.glob(os.path.join(R, "*", ""))):
    model = os.path.basename(os.path.dirname(md))
    for cond in ("basic", "refresh-on", "refresh-off"):
        for rep in reports(os.path.join(md, cond)):
            for run in rep["runs"]:
                fails = [f"{a['type']}({a.get('test','')})" for a in run["asserts"] if a["status"] == "fail" and a.get("test") != "info"]
                infos = [f"{a['type']}:{a['status']}" for a in run["asserts"] if a.get("test") == "info"]
                state = "broke: " + run["error"][:160] if run.get("error") else ("fail: " + ", ".join(fails) if fails else "pass")
                print(f"- {model} {cond} {run['scenario']} r{run['rep']}: {state}; {run['seconds']:.0f}s, {run['requests']} req ({run['provider_requests']} to provider), prompt {run['prompt_tokens']}, peak {run.get('peak_prompt_tokens')}, out {run['output_tokens']}; info {' '.join(infos)}")

print("\n## Schema calls (schema-*.jsonl)")
for f in sorted(glob.glob(os.path.join(R, "*", "schema-*.jsonl"))):
    calls = [json.loads(l) for l in open(f, encoding="utf-8")]
    if not calls:
        continue
    first = next(c["input"] for c in calls if c["schema"] == "items" and c["messages"] == 1)
    runs, cur = [], None
    for c in calls:
        if c["schema"] == "items" and c["messages"] == 1 and c["input"] == first:
            cur = []
            runs.append(cur)
        cur.append(c)
    print(f"### {os.path.relpath(f, R)}")
    for i, run in enumerate(runs, 1):
        parts = []
        for c in run:
            kind = "merge" if "edits" in c["schema"] else "map"
            tag = "retry" if c["messages"] > 1 else "try"
            parts.append(f"{kind}-{tag}({'json' if c['valid'] else 'NOT json'}{', err' if c.get('error') else ''}, {c['seconds']:.0f}s, sent={c['sent']})")
        print(f"- r{i}: " + "; ".join(parts))
