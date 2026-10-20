"""Embed the harness's texts with the reference tokenizer, for the
granite-reftok condition in models.yaml.

The Go backend computes granite exactly when it gets the right token ids
(builtin/results.md). Its tokenizer is wrong for XLM-R, so this script
tokenizes with Python `tokenizers` and runs the same graph in ONNX Runtime,
one text at a time, with CLS pooling. The vectors go into vectors.db under
the harness's cache key, so `go run . -model granite-reftok` reads them.

usage:
  go run . -texts results/tmp/texts.jsonl
  python -I reftok.py MODEL_DIR TEXTS_JSONL VECTORS_DB [CACHE_MODEL]

CACHE_MODEL is the cache's model key, name/model from models.yaml
(granite-reftok/granite-embedding-107m-multilingual).
"""
import hashlib, json, sqlite3, struct, sys, time
import numpy as np, onnxruntime as ort
from tokenizers import Tokenizer

d, texts_path, db_path = sys.argv[1:4]
model = sys.argv[4] if len(sys.argv) > 4 else "granite-reftok/granite-embedding-107m-multilingual"

tok = Tokenizer.from_file(d + "/tokenizer.json")
tok.enable_truncation(512)
sess = ort.InferenceSession(d + "/model.onnx", providers=["CPUExecutionProvider"])
names = [i.name for i in sess.get_inputs()]
out = sess.get_outputs()[0].name  # logits: the token-level hidden state

rows = [json.loads(l) for l in open(texts_path, encoding="utf-8")]
db = sqlite3.connect(db_path)
db.execute("""CREATE TABLE IF NOT EXISTS vectors (
	model TEXT NOT NULL, use TEXT NOT NULL, prefix INTEGER NOT NULL, sha TEXT NOT NULL,
	dims INTEGER NOT NULL, vec BLOB NOT NULL, ns INTEGER NOT NULL,
	PRIMARY KEY (model, use, prefix, sha)) WITHOUT ROWID""")
done = truncated = 0
for r in rows:
    sha = hashlib.sha256(r["text"].encode("utf-8")).hexdigest()
    if db.execute("SELECT 1 FROM vectors WHERE model=? AND use=? AND prefix=0 AND sha=?",
                  (model, r["use"], sha)).fetchone():
        continue
    start = time.perf_counter_ns()
    e = tok.encode(r["text"])
    truncated += len(e.overflowing) > 0
    ids = np.array([e.ids], np.int64)
    feeds = {"input_ids": ids, "attention_mask": np.ones_like(ids)}
    if "token_type_ids" in names:
        feeds["token_type_ids"] = np.zeros_like(ids)
    v = sess.run([out], feeds)[0][0][0].astype(np.float32)
    ns = time.perf_counter_ns() - start
    db.execute("INSERT OR REPLACE INTO vectors VALUES (?, ?, 0, ?, ?, ?, ?)",
               (model, r["use"], sha, len(v), struct.pack("<%df" % len(v), *v), ns))
    done += 1
db.commit()
print(f"{done} texts embedded, {len(rows) - done} already in the cache, {truncated} cut at 512 tokens")
