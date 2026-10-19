"""Run the Persian check from cmd/measure/persian.go through ONNX Runtime with
the reference (Rust) tokenizer, to see the scores without the Go tokenizer.

usage: python -I -X utf8 ortpersian.py PERSIAN_GO MODEL_DIR GRAPH POOLING QPREFIX PPREFIX
POOLING is sentence (the graph's sentence_embedding output), mean or cls.
"""
import re, sys, numpy as np, onnxruntime as ort
from tokenizers import Tokenizer
src = open(sys.argv[1], encoding="utf-8").read()
def block(name):
    body = re.search(r"var " + name + r" = \[\]string\{(.*?)\n\}", src, re.S).group(1)
    return re.findall(r'^\s*"(.*)",\s*$', body, re.M)
P, FQ, EQ = block("faPassages"), block("faQueries"), block("enQueries")
assert (len(P), len(FQ), len(EQ)) == (22, 10, 10)
d, graph, pool, qp, pp = sys.argv[2:7]
tok = Tokenizer.from_file(d + "/tokenizer.json")
s = ort.InferenceSession(d + "/" + graph, providers=["CPUExecutionProvider"])
names = [i.name for i in s.get_inputs()]
def emb(texts):
    enc = tok.encode_batch(texts)
    n = max(len(e.ids) for e in enc)
    ids = np.array([e.ids + [tok.token_to_id("<pad>") or 0] * (n - len(e.ids)) for e in enc], dtype=np.int64)
    m = np.array([[1] * len(e.ids) + [0] * (n - len(e.ids)) for e in enc], dtype=np.int64)
    feed = {"input_ids": ids, "attention_mask": m}
    if "token_type_ids" in names: feed["token_type_ids"] = np.zeros_like(ids)
    for f in ("image_features", "video_features", "audio_features"):
        if f in names: feed[f] = np.zeros((1, 512), dtype=np.float32)
    if pool == "sentence":
        v = s.run(["sentence_embedding"], feed)[0]
    else:
        h = s.run([s.get_outputs()[0].name], feed)[0]
        v = h[:, 0] if pool == "cls" else (h * m[..., None]).sum(1) / m.sum(1, keepdims=True)
    return v / np.linalg.norm(v, axis=1, keepdims=True)
dv = emb([pp + p for p in P])
for name, qs in (("fa -> fa", FQ), ("en -> fa", EQ)):
    sc = emb([qp + q for q in qs]) @ dv.T
    ranks = [int((sc[i] > sc[i, i]).sum()) for i in range(len(qs))]
    print(f"persian {name}: recall@1 {sum(r == 0 for r in ranks)}/10, recall@3 {sum(r < 3 for r in ranks)}/10, MRR {np.mean([1 / (r + 1) for r in ranks]):.3f}")
