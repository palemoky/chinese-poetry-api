#!/usr/bin/env python3
"""Embed poems through Ollama and score vector search against queries.jsonl.

The corpus is every gold poem plus a seeded random sample of distractors, so
several models can be compared cheaply before running the full corpus. Vectors
are cached in --cache, one file per model and corpus size, and an interrupted
run resumes where it stopped.

Usage:
    uv run --python 3.13 --with numpy eval/semantic/embed_eval.py \
        --db poetry.db --cache .embed-cache --model bge-m3 --sample 50000

Pass --search-api with a running server (rate limit off) to also score the
current keyword search on the same corpus and its RRF hybrids with vectors.
"""

import argparse
import json
import random
import sqlite3
import time
import urllib.request
from collections import defaultdict
from pathlib import Path

import numpy as np

import evaluate as ev

SEED = 42
BATCH = 64
RRF_C = 60
# 约 99.9% 的诗词短于这个长度，超长的多是元曲套数，截断只影响开头以外的内容
MAX_CHARS = 1500


def load_corpus(db_path, gold, sample):
    """Return (ids, texts). sample <= 0 means the full corpus."""
    db = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True)
    rows = db.execute(
        """SELECT p.id, p.title, COALESCE(a.name, ''), p.content FROM poems_zh_hans p
           LEFT JOIN authors_zh_hans a ON a.id = p.author_id ORDER BY p.id"""
    ).fetchall()
    db.close()

    if sample > 0:
        wanted = set().union(*(ids for entries in gold.values() for ids in entries))
        others = [row[0] for row in rows if row[0] not in wanted]
        wanted |= set(random.Random(SEED).sample(others, sample))
        rows = [row for row in rows if row[0] in wanted]

    ids = np.array([row[0] for row in rows], dtype=np.int64)
    return ids, [document(*row[1:]) for row in rows]


def document(title, author, content):
    return f"{title}\n{author}\n{''.join(json.loads(content))}"[:MAX_CHARS]


def embed(api, model, texts):
    body = json.dumps({"model": model, "input": texts}).encode()
    req = urllib.request.Request(f"{api}/api/embed", body, {"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=600) as resp:
        return np.array(json.load(resp)["embeddings"], dtype=np.float32)


def embed_corpus(api, model, texts, path):
    """Embed in batches, resuming from the partial file next to path."""
    if path.exists():
        return np.load(path)

    partial = path.with_suffix(".partial.bin")
    meta = path.with_suffix(".partial.json")
    dim = embed(api, model, ["测试"]).shape[1]
    done = json.loads(meta.read_text())["done"] if meta.exists() else 0

    start = time.time()
    with open(partial, "ab") as f:
        f.truncate(done * dim * 4)
        for i in range(done, len(texts), BATCH):
            f.write(embed(api, model, texts[i : i + BATCH]).tobytes())
            f.flush()
            done = min(i + BATCH, len(texts))
            meta.write_text(json.dumps({"done": done}))
            if i % (BATCH * 50) == 0:
                print(f"embedded {done}/{len(texts)} after {time.time() - start:.0f}s", flush=True)

    vectors = np.fromfile(partial, dtype=np.float32).reshape(-1, dim)
    np.save(path, vectors)
    partial.unlink()
    meta.unlink()
    return vectors


def normalize(x):
    return x / np.clip(np.linalg.norm(x, axis=1, keepdims=True), 1e-12, None)


def rank(queries, docs, ids, k=ev.FETCH):
    """Rank docs for each query by dot product; returns a matrix of poem IDs."""
    out = []
    for start in range(0, len(queries), 16):
        scores = queries[start : start + 16] @ docs.T
        top = np.argpartition(-scores, k, axis=1)[:, :k]
        order = np.take_along_axis(scores, top, axis=1).argsort(axis=1)[:, ::-1]
        out.append(ids[np.take_along_axis(top, order, axis=1)])
    return np.concatenate(out)


def quantize_int8(x):
    return np.clip(np.round(x * 127), -127, 127).astype(np.int8)


def evaluate_variant(name, query_ids, q, d, ids, queries, gold, megabytes):
    """Score one storage format. Vectors stay float32 here, so timing says nothing about it."""
    ranked = rank(q, d, ids)
    run = {qid: ranked[i].tolist() for i, qid in enumerate(query_ids)}
    rows = ev.report(f"{name}  ({megabytes:.0f} MB)", queries, gold, run)
    return {"metrics": rows, "megabytes": megabytes}, run


def rrf(*runs):
    """Reciprocal rank fusion of runs, each mapping query id to ranked IDs."""
    fused = {}
    for qid in runs[0]:
        scores = defaultdict(float)
        for run in runs:
            for position, poem_id in enumerate(run[qid][: ev.FETCH], 1):
                scores[poem_id] += 1 / (RRF_C + position)
        fused[qid] = sorted(scores, key=scores.get, reverse=True)
    return fused


def evaluate_hybrids(api, ids, queries, gold, vector_runs):
    """Score keyword search and its hybrids, keeping only poems in the embedded corpus."""
    corpus = set(ids.tolist())

    def keep(ranked):
        return [poem_id for poem_id in ranked if poem_id in corpus]

    literal = {q["id"]: keep(ev.search(api, q["query"])) for q in queries}
    rewritten = {q["id"]: keep(ev.interleave([ev.search(api, kw) for kw in q["keywords"]])) for q in queries}

    results = {
        "A": ev.report("A. raw query", queries, gold, literal),
        "B": ev.report("B. simulated keywords", queries, gold, rewritten),
    }
    for name, run in vector_runs.items():
        results[f"A+{name}"] = ev.report(f"hybrid A + {name}", queries, gold, rrf(literal, run))
        results[f"B+{name}"] = ev.report(f"hybrid B + {name}", queries, gold, rrf(rewritten, run))
    return results


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--db", required=True)
    parser.add_argument("--cache", required=True)
    parser.add_argument("--model", default="bge-m3")
    parser.add_argument("--api", default="http://localhost:11434")
    parser.add_argument("--sample", type=int, default=50000, help="distractor count, 0 for the full corpus")
    parser.add_argument("--search-api", help="running server URL; adds keyword search and hybrids")
    parser.add_argument("--out")
    args = parser.parse_args()

    queries = ev.load_queries()
    gold, missing = ev.resolve_gold(args.db, queries)
    if missing:
        raise SystemExit(f"{len(missing)} gold entries unresolved; run evaluate.py check first")

    ids, texts = load_corpus(args.db, gold, args.sample)
    print(f"corpus: {len(ids)} poems, model: {args.model}", flush=True)

    cache = Path(args.cache)
    cache.mkdir(parents=True, exist_ok=True)
    stem = f"{args.model.replace('/', '_').replace(':', '_')}-{args.sample}"
    docs = normalize(embed_corpus(args.api, args.model, texts, cache / f"{stem}.npy"))

    query_ids = [q["id"] for q in queries]
    q = normalize(embed(args.api, args.model, [item["query"] for item in queries]))

    results = {}
    results["float32"], float_run = evaluate_variant("float32", query_ids, q, docs, ids, queries, gold, docs.nbytes / 1e6)

    # 向量几乎压缩不动，这几种变体决定用户要多下载多少
    d8 = quantize_int8(docs).astype(np.float32)
    results["int8"], int8_run = evaluate_variant("int8", query_ids, quantize_int8(q).astype(np.float32), d8, ids, queries, gold, docs.size / 1e6)
    db = np.where(docs > 0, 1.0, -1.0).astype(np.float32)
    results["binary"], _ = evaluate_variant("binary", query_ids, np.where(q > 0, 1.0, -1.0).astype(np.float32), db, ids, queries, gold, docs.size / 8 / 1e6)

    if args.search_api:
        results["hybrids"] = evaluate_hybrids(args.search_api, ids, queries, gold, {"float32": float_run, "int8": int8_run})

    if args.out:
        Path(args.out).write_text(json.dumps(results, ensure_ascii=False, indent=2), encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
