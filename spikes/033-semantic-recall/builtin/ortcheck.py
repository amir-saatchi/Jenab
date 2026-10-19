"""Compare hugot's vectors (from `measure ... vectors FILE`) with ONNX
Runtime's for the same texts, graph and pooling.

usage: python -I ortcheck.py MODEL_DIR GRAPH_FILE POOLING HUGOT_JSON [TOKENS_JSON]
POOLING is sentence (the graph's sentence_embedding output), mean or cls.
TOKENS_JSON (from `measure ... tokens FILE`) splits the result by whether
hugot's Go tokenizer gave the same ids as the reference tokenizer.
"""
import json, sys
import numpy as np, onnxruntime as ort
from tokenizers import Tokenizer

d, graph, pooling, path = sys.argv[1:5]
data = json.load(open(path, encoding="utf-8"))
goids = None
if len(sys.argv) > 5:
    goids = json.load(open(sys.argv[5], encoding="utf-8"))["ids"]
tok = Tokenizer.from_file(d + "/tokenizer.json")
sess = ort.InferenceSession(d + "/" + graph, providers=["CPUExecutionProvider"])
names = [i.name for i in sess.get_inputs()]
outs = [o.name for o in sess.get_outputs()]
cos, same = [], []
for i, (text, hv) in enumerate(zip(data["texts"], data["vectors"])):
    e = tok.encode(text)
    same.append(goids is None or goids[i] == e.ids)
    ids = np.array([e.ids], np.int64)
    feeds = {"input_ids": ids, "attention_mask": np.ones_like(ids)}
    if "token_type_ids" in names:
        feeds["token_type_ids"] = np.zeros_like(ids)
    for f in ("image_features", "video_features", "audio_features"):
        if f in names:
            feeds[f] = np.zeros((0, 512), np.float32)
    if pooling == "sentence":
        v = sess.run(["sentence_embedding"], feeds)[0][0]
    else:
        h = sess.run([outs[0]], feeds)[0][0]
        v = h[0] if pooling == "cls" else h.mean(axis=0)
    hv = np.array(hv)
    cos.append(float(v @ hv / np.linalg.norm(v) / np.linalg.norm(hv)))
cos = np.array(cos)
print(f"hugot vs onnxruntime over {len(cos)} texts: min {cos.min():.6f}, median {np.median(cos):.6f}")
if goids is not None:
    same = np.array(same)
    for label, sel in (("same tokens", same), ("different tokens", ~same)):
        if sel.any():
            print(f"  {label}, {sel.sum()} texts: min {cos[sel].min():.6f}, median {np.median(cos[sel]):.6f}")
