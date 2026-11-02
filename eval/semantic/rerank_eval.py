#!/usr/bin/env python3
"""Score ranking improvements on top of vector search, on the full corpus.

Needs the vectors from embed_eval.py and popularity signals from signals.json.
queries.jsonl is biased toward famous poems, so every popularity method must be
read together with control.jsonl: paraphrases of random obscure poems, written
before any retrieval result was seen.

Usage (--reranker needs torch and transformers, see README):
    uv run --python 3.13 --with numpy eval/semantic/rerank_eval.py \
        --db poetry.db --cache .embed-cache --signals signals.json \
        --search-api http://localhost:1279
"""

import argparse
import json
import math
import re
import sys
from collections import defaultdict
from pathlib import Path

import numpy as np

sys.dont_write_bytecode = True
import embed_eval as ee
import evaluate as ev

POOL = 1000
PRIOR_WEIGHTS = (0.02, 0.05, 0.1, 0.2, 0.4)
RERANK_DEPTHS = (100, 200, 500, 1000)
# 交叉编码器输出 logit，选本加分的单位也是 logit
RERANK_PRIOR_WEIGHTS = (1, 2, 3, 4, 6)


def load_all():
    queries = ev.load_queries()
    with open(ev.HERE / "control.jsonl", encoding="utf-8") as f:
        return queries, [json.loads(line) for line in f if line.strip()]


def load_signals(path, ids):
    """Return (anthology, popularity), both arrays aligned with ids and in 0..1."""
    raw = json.loads(Path(path).read_text())
    anthology = np.zeros(len(ids), dtype=np.float32)
    popularity = np.zeros(len(ids), dtype=np.float32)
    for i, poem_id in enumerate(ids.tolist()):
        entry = raw.get(str(poem_id))
        if entry:
            anthology[i] = 1.0 if set(entry) - {"rank"} else 0.0
            # 结果数跨 6 个数量级，取对数；百度覆盖最全，其他引擎噪声更大
            popularity[i] = min(math.log10(1 + entry.get("rank", [0])[0]) / 6, 1.0)
    return anthology, popularity


def pool_by_prior(scores, ids, prior, weight):
    """Re-sort a query's POOL candidates by cosine + weight * prior."""
    top = np.argpartition(-scores, POOL)[:POOL]
    adjusted = scores[top] + weight * prior[top]
    return ids[top[np.argsort(-adjusted)]].tolist()


def rrf_two(first, second):
    scores = defaultdict(float)
    for ranked in (first, second):
        for position, poem_id in enumerate(ranked[: POOL], 1):
            scores[poem_id] += 1 / (ee.RRF_C + position)
    return sorted(scores, key=scores.get, reverse=True)


def couplets(text):
    """Split a poem body into sentences ending in full-stop punctuation."""
    return [part + "。" for part in re.split(r"[。！？；]", text) if len(part) >= 4]


def line_index(args, cache, ids, texts, prior, label):
    """Embed each sentence of the poems where prior > 0; returns (vectors, poem IDs)."""
    selected = np.flatnonzero(prior > 0)
    lines, owners = [], []
    for i in selected:
        for line in couplets(texts[i].split("\n", 2)[-1]):
            lines.append(line)
            owners.append(int(ids[i]))
    print(f"line index {label}: {len(selected)} poems, {len(lines)} lines", flush=True)
    vectors = ee.embed_corpus(args.api, args.model, lines, cache / f"{args.model}-lines-{label}.npy")
    return ee.normalize(vectors), np.array(owners, dtype=np.int64)


def line_run(q8, vectors, owners, query_ids):
    """Rank poems by their best line, one list per query."""
    scores = q8 @ (ee.quantize_int8(vectors).astype(np.float32) / 127).T
    run = {}
    for i, qid in enumerate(query_ids):
        ranked, seen = [], set()
        for j in np.argsort(-scores[i])[:300]:
            poem_id = int(owners[j])
            if poem_id not in seen:
                seen.add(poem_id)
                ranked.append(poem_id)
        run[qid] = ranked
    return run


def evaluate_set(name, queries, gold, runs, rows):
    for label, run in runs.items():
        if any(q["id"] not in run for q in queries):
            continue
        scored =[ev.score(run[q["id"]], gold[q["id"]]) for q in queries if gold[q["id"]]]
        hits = sum(s[0] for s in scored)
        mrr = sum(s[2] for s in scored) / len(scored)
        rows.setdefault(label, {})[name] = {"hits": int(hits), "n": len(scored), "mrr": round(mrr, 3)}


def print_rows(rows, names):
    header = "".join(f"{n + ' hit':>14}{'mrr':>7}" for n in names)
    print(f"{'method':34}{header}")
    for label, row in rows.items():
        cells = "".join(f"{str(row[n]['hits']) + '/' + str(row[n]['n']):>14}{row[n]['mrr']:>7.3f}" for n in names if n in row)
        print(f"{label:34}{cells}")


def rerank_scores(path, model_dir, pool_runs, query_text, doc_text):
    """Cross-encoder scores for every (query, candidate), cached per query in path."""
    cache = json.loads(path.read_text()) if path.exists() else {}
    todo = [qid for qid in pool_runs if qid not in cache]
    if todo:
        import torch
        from transformers import AutoModelForSequenceClassification, AutoTokenizer

        device = "mps" if torch.backends.mps.is_available() else "cpu"
        tokenizer = AutoTokenizer.from_pretrained(model_dir)
        model = AutoModelForSequenceClassification.from_pretrained(model_dir).to(device).eval()
        for n, qid in enumerate(todo, 1):
            out = {}
            pool = pool_runs[qid]
            for start in range(0, len(pool), 32):
                chunk = pool[start : start + 32]
                pairs = [[query_text[qid], doc_text(p)] for p in chunk]
                batch = tokenizer(pairs, padding=True, truncation=True, max_length=512, return_tensors="pt").to(device)
                with torch.no_grad():
                    logits = model(**batch).logits.view(-1).float().cpu().tolist()
                out.update({str(p): s for p, s in zip(chunk, logits)})
            cache[qid] = out
            path.write_text(json.dumps(cache))
            print(f"reranked {n}/{len(todo)} {qid}", flush=True)
    return cache


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--db", required=True)
    parser.add_argument("--cache", required=True)
    parser.add_argument("--signals", required=True)
    parser.add_argument("--model", default="bge-m3")
    parser.add_argument("--api", default="http://localhost:11434")
    parser.add_argument("--search-api", required=True)
    parser.add_argument("--reranker", help="local cross-encoder directory")
    parser.add_argument("--out")
    args = parser.parse_args()

    famous, control = load_all()
    everything = famous + control
    gold, missing = ev.resolve_gold(args.db, everything)
    if missing:
        raise SystemExit(f"unresolved gold: {missing}")

    cache = Path(args.cache)
    ids, texts = ee.load_corpus(args.db, gold, 0)
    docs = ee.normalize(np.load(cache / f"{args.model}-0.npy"))
    position = {int(p): i for i, p in enumerate(ids)}

    q_path = cache / "queries.json"
    q_cache = json.loads(q_path.read_text()) if q_path.exists() else {}
    if any(q["query"] not in q_cache for q in everything):
        vectors = ee.embed(args.api, args.model, [q["query"] for q in everything])
        q_cache = {q["query"]: v.tolist() for q, v in zip(everything, vectors)}
        q_path.write_text(json.dumps(q_cache, ensure_ascii=False))
    q = ee.normalize(np.array([q_cache[item["query"]] for item in everything], dtype=np.float32))

    # int8 是推荐的存储格式，分数换算回余弦量级，这样先验权重的含义不随格式变化
    d8 = ee.quantize_int8(docs).astype(np.float32) / 127
    q8 = ee.quantize_int8(q).astype(np.float32) / 127
    scores = q8 @ d8.T

    a_path = cache / "search-a.json"
    if a_path.exists():
        literal = json.loads(a_path.read_text())
    else:
        literal = {item["id"]: ev.search(args.search_api, item["query"]) for item in everything}
        a_path.write_text(json.dumps(literal))

    anthology, popularity = load_signals(args.signals, ids)
    priors = {"anthology": anthology, "searchcount": popularity, "both": np.maximum(anthology, popularity)}

    runs = {}
    base = {}
    for i, item in enumerate(everything):
        base[item["id"]] = pool_by_prior(scores[i], ids, popularity, 0.0)
    runs["vector int8"] = base
    runs["hybrid A+int8"] = {qid: rrf_two(literal[qid], ranked) for qid, ranked in base.items()}

    for name, prior in priors.items():
        for weight in PRIOR_WEIGHTS:
            run = {}
            for i, item in enumerate(everything):
                run[item["id"]] = rrf_two(literal[item["id"]], pool_by_prior(scores[i], ids, prior, weight))
            runs[f"A+int8 prior={name} w={weight}"] = run

    query_ids = [item["id"] for item in everything]
    best = runs["A+int8 prior=anthology w=0.05"]
    vectors, owners = line_index(args, cache, ids, texts, anthology, "anthology")
    lines = line_run(q8, vectors, owners, query_ids)
    runs["A+int8 + lines"] = {qid: rrf_two(runs["hybrid A+int8"][qid], lines[qid]) for qid in query_ids}
    runs["anthology 0.05 + lines"] = {qid: rrf_two(best[qid], lines[qid]) for qid in query_ids}

    if args.reranker:
        fused = runs["hybrid A+int8"]
        pool_runs = {qid: ranked[: max(RERANK_DEPTHS)] for qid, ranked in fused.items()}
        query_text = {item["id"]: item["query"] for item in everything}
        ce = rerank_scores(cache / "rerank-scores.json", args.reranker, pool_runs, query_text, lambda p: texts[position[p]])
        for depth in RERANK_DEPTHS:
            run = {}
            for qid, ranked in fused.items():
                head = sorted(ranked[:depth], key=lambda p: ce[qid][str(p)], reverse=True)
                run[qid] = head + ranked[depth:]
            runs[f"rerank top{depth}"] = run
            # 字面命中仍要排在前面，否则重排会丢掉精确诗句
            runs[f"rerank top{depth} + A"] = {qid: rrf_two(literal[qid], run[qid]) for qid in run}
            for weight in RERANK_PRIOR_WEIGHTS:
                boosted = {}
                for qid, ranked in fused.items():
                    head = ranked[:depth]
                    key = {p: ce[qid][str(p)] + weight * anthology[position[p]] for p in head}
                    boosted[qid] = sorted(head, key=key.get, reverse=True) + ranked[depth:]
                runs[f"rerank top{depth} anthology +{weight}"] = boosted

    rows = {}
    evaluate_set("famous", famous, gold, runs, rows)
    evaluate_set("control", control, gold, runs, rows)
    print_rows(rows, ["famous", "control"])

    categories = {}
    for label, run in runs.items():
        per = defaultdict(int)
        for item in everything:
            per[item["category"]] += int(ev.score(run[item["id"]], gold[item["id"]])[0])
        categories[label] = dict(per)

    if args.out:
        Path(args.out).write_text(json.dumps({"overall": rows, "categories": categories}, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
