#!/usr/bin/env python3
"""Score relevance ranking for the keyword search, on the full corpus.

The server matches the whole query as a substring of the title, the body or
the author name, and returns matches in ID order. This script reproduces that
match set in memory, checks it against the cached API results, then ranks the
same matches several ways. Control queries search for a line of each obscure
poem in control.jsonl, to see whether a popularity boost buries them.

Usage:
    uv run --python 3.13 --with numpy eval/semantic/keyword_eval.py \
        --db poetry.db --cache .embed-cache --signals signals.json
"""

import argparse
import json
import sqlite3
import sys
from pathlib import Path

import numpy as np

sys.dont_write_bytecode = True
import embed_eval as ee
import evaluate as ev
import rerank_eval as rr

# 字段权重：标题完全相同最可能是用户要找的那首，其次是标题包含、作者、正文
TITLE_EXACT = 4.0
TITLE_PART = 2.0
AUTHOR_EXACT = 2.0
AUTHOR_PART = 1.0
CONTENT = 1.0
REPEAT = 0.5
ANTHOLOGY = 1.5


class Corpus:
    def __init__(self, db_path, signals_path):
        db = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True)
        rows = db.execute(
            """SELECT p.id, p.title, COALESCE(a.name, ''), p.content FROM poems_zh_hans p
               LEFT JOIN authors_zh_hans a ON a.id = p.author_id ORDER BY p.id"""
        ).fetchall()
        db.close()
        self.ids = [row[0] for row in rows]
        self.titles = [row[1] for row in rows]
        self.authors = [row[2] for row in rows]
        self.bodies = ["".join(json.loads(row[3])) for row in rows]
        # LIKE 不区分 ASCII 大小写
        self.folded = [(t.lower(), a.lower(), b.lower()) for t, a, b in zip(self.titles, self.authors, self.bodies)]
        signals = json.loads(Path(signals_path).read_text())
        self.anthology = [bool(set(signals.get(str(i), {})) - {"rank"}) for i in self.ids]

    def matches(self, term):
        term = term.lower()
        return [i for i, (t, a, b) in enumerate(self.folded) if term in t or term in a or term in b]

    def field_score(self, i, term):
        t, a, b = self.folded[i]
        term = term.lower()
        score = 0.0
        # 词的标题是“词牌·首句”，搜词牌名也算完全匹配
        if t == term or t.split("·")[0] == term:
            score += TITLE_EXACT
        elif term in t:
            score += TITLE_PART
        if a == term:
            score += AUTHOR_EXACT
        elif term in a:
            score += AUTHOR_PART
        if term in b:
            score += CONTENT + REPEAT * min(b.count(term) - 1, 2)
        return score

    def search(self, query, method):
        """Return poem IDs, at most ev.FETCH, the way the API pages them."""
        terms = query.split() if method.endswith("split") else [query]
        found = None
        for term in terms:
            hits = set(self.matches(term))
            found = hits if found is None else found & hits
        found = sorted(found or ())
        if method == "id":
            ranked = found
        else:
            boost = ANTHOLOGY if method.startswith("anthology") else 0.0
            # 同分时保持原来的 ID 顺序。按篇幅排会把只剩几个字的残篇排到最前
            ranked = sorted(
                found,
                key=lambda i: (-(sum(self.field_score(i, term) for term in terms) + boost * self.anthology[i]), self.ids[i]),
            )
        return [self.ids[i] for i in ranked[: ev.FETCH]]


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--db", required=True)
    parser.add_argument("--cache", required=True)
    parser.add_argument("--signals", required=True)
    parser.add_argument("--model", default="bge-m3")
    parser.add_argument("--out")
    args = parser.parse_args()

    famous, control = rr.load_all()
    # 冷门对照：直接搜每首冷门诗的标志句
    lines = [{**item, "id": item["id"] + "-line", "category": "control-line", "query": item["gold"][0][1]} for item in control]
    gold, missing = ev.resolve_gold(args.db, famous + lines)
    if missing:
        raise SystemExit(f"unresolved gold: {missing}")

    corpus = Corpus(args.db, args.signals)
    cache = Path(args.cache)

    # 先确认复现的命中集合与线上 API 一致，否则后面的比较没有意义
    api = json.loads((cache / "search-a.json").read_text())
    mismatched = [q["id"] for q in famous if corpus.search(q["query"], "id") != api[q["id"]]]
    print(f"reproduced API results: {len(famous) - len(mismatched)}/{len(famous)} identical {mismatched}")

    methods = ["id", "fields", "anthology", "fields-split", "anthology-split"]
    runs = {}
    for method in methods:
        runs[f"A {method}"] = {q["id"]: corpus.search(q["query"], method) for q in famous + lines}
        runs[f"B {method}"] = {
            q["id"]: ev.interleave([corpus.search(kw, method) for kw in q["keywords"]]) for q in famous
        }

    # 关键词排序变了，与向量混合的结果也会变；向量部分用上一步的推荐配置
    ids, _ = ee.load_corpus(args.db, gold, 0)
    docs = ee.normalize(np.load(cache / f"{args.model}-0.npy"))
    q_cache = json.loads((cache / "queries.json").read_text())
    q = ee.normalize(np.array([q_cache[item["query"]] for item in famous], dtype=np.float32))
    scores = (ee.quantize_int8(q).astype(np.float32) / 127) @ (ee.quantize_int8(docs).astype(np.float32) / 127).T
    anthology, _ = rr.load_signals(args.signals, ids)
    vector = {item["id"]: rr.pool_by_prior(scores[i], ids, anthology, 0.05) for i, item in enumerate(famous)}
    for method in ("id", "fields", "anthology"):
        runs[f"A {method} + vector"] = {qid: rr.rrf_two(runs[f"A {method}"][qid], vector[qid]) for qid in vector}

    rows = {}
    rr.evaluate_set("famous", famous, gold, runs, rows)
    rr.evaluate_set("literal", [q for q in famous if q["category"] == "literal"], gold, runs, rows)
    rr.evaluate_set("control-line", lines, gold, runs, rows)
    rr.print_rows(rows, ["famous", "literal", "control-line"])

    if args.out:
        Path(args.out).write_text(json.dumps(rows, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
