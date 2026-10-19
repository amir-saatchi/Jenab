"""ONNX Runtime stand-in for hugot's ORT backend.

hugot's ORT backend needs cgo and a C toolchain, which the test machine
doesn't have. This script runs the same ONNX Runtime engine from Python on
the same graph and the same text (from `measure texts`), so its speed is
what hugot's ORT backend would get, give or take the tokenizer.

usage: python -I ortbench.py MODEL_DIR GRAPH_FILE TEXTS_JSON [BATCH]
needs: onnxruntime tokenizers numpy psutil
"""
import json, sys, time, threading
import numpy as np, onnxruntime as ort, psutil
from tokenizers import Tokenizer

d, graph, texts_path = sys.argv[1], sys.argv[2], sys.argv[3]
batch = int(sys.argv[4]) if len(sys.argv) > 4 else 32
proc = psutil.Process()
texts = json.load(open(texts_path, encoding="utf-8"))
print(f"onnxruntime {ort.__version__}, {psutil.cpu_count()} logical CPUs")

t = time.perf_counter()
tok = Tokenizer.from_file(d + "/tokenizer.json")
sess = ort.InferenceSession(d + "/" + graph, providers=["CPUExecutionProvider"])
load = time.perf_counter() - t
names = [i.name for i in sess.get_inputs()]
out_name = "sentence_embedding" if "sentence_embedding" in [o.name for o in sess.get_outputs()] else sess.get_outputs()[0].name
print(f"load {load:.2f}s, output {out_name}, rss {proc.memory_info().rss >> 20} MB, private {proc.memory_info().private >> 20} MB")


def embed(batch_texts):
    enc = tok.encode_batch(batch_texts)
    n = max(len(e.ids) for e in enc)
    ids = np.zeros((len(enc), n), np.int64)
    mask = np.zeros((len(enc), n), np.int64)
    for i, e in enumerate(enc):
        ids[i, : len(e.ids)] = e.ids
        mask[i, : len(e.ids)] = 1
    feeds = {"input_ids": ids, "attention_mask": mask}
    if "token_type_ids" in names:
        feeds["token_type_ids"] = np.zeros_like(ids)
    for f in ("image_features", "video_features", "audio_features"):
        if f in names:
            feeds[f] = np.zeros((0, 512), np.float32)
    return sess.run([out_name], feeds)[0]


t = time.perf_counter()
embed(texts["queries"][:1])
print(f"first query {time.perf_counter() - t:.3f}s")
lat = []
for _ in range(3):
    for q in texts["queries"]:
        t = time.perf_counter()
        embed([q])
        lat.append(time.perf_counter() - t)
lat.sort()
print(f"single query, {len(lat)} runs: p50 {lat[len(lat) // 2] * 1000:.0f}ms, p95 {lat[len(lat) * 95 // 100] * 1000:.0f}ms")

chunks = texts["chunks"]
peak = [0]
stop = threading.Event()


def sample():
    while not stop.is_set():
        peak[0] = max(peak[0], proc.memory_info().rss)
        time.sleep(0.1)


th = threading.Thread(target=sample)
th.start()
cpu0 = sum(proc.cpu_times()[:2])
t = time.perf_counter()
for i in range(0, len(chunks), batch):
    embed(chunks[i : i + batch])
took = time.perf_counter() - t
cpu = sum(proc.cpu_times()[:2]) - cpu0
stop.set()
th.join()
print(f"throughput: {len(chunks)} chunks in batches of {batch}: {took:.1f}s, {len(chunks) / took:.1f} chunks/s; CPU {cpu:.0f}s ({cpu / took:.1f} cores busy)")
print(f"memory while embedding: peak rss {peak[0] >> 20} MB, private now {proc.memory_info().private >> 20} MB")
