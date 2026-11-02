#!/usr/bin/env python3
"""Score retrieval systems against queries.jsonl.

Gold poems are stored as (author, signature line) rather than IDs, because IDs
change between data releases. They are resolved to IDs of the given database on
every run. Scores are a lower bound: only the well-known poems are labelled, so
an unlabelled poem that also fits a query counts as a miss.

Usage:
    evaluate.py check --db poetry.db
    evaluate.py baseline --db poetry.db --api http://localhost:1279
"""

import argparse
import json
import sqlite3
import urllib.parse
import urllib.request
from collections import defaultdict
from pathlib import Path

HERE = Path(__file__).parent
K = 10
# 现有搜索不按相关度排序，取多一些才能看出答案是否排在前 K 之外
FETCH = 100


def load_queries():
    with open(HERE / "queries.jsonl", encoding="utf-8") as f:
        return [json.loads(line) for line in f if line.strip()]


def resolve_gold(db_path, queries):
    """Map each query to one set of poem IDs per gold entry.

    One entry can resolve to several IDs because the corpus keeps the same poem
    under several titles or authors; finding any of them counts as finding it.
    """
    db = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True)
    cache = {}
    resolved, missing = {}, []
    for q in queries:
        entries = []
        for author, line in q["gold"]:
            key = (author, line)
            if key not in cache:
                sql = """SELECT p.id FROM poems_zh_hans p
                         LEFT JOIN authors_zh_hans a ON a.id = p.author_id
                         WHERE p.content LIKE ?"""
                args = [f"%{line}%"]
                if author:
                    sql += " AND a.name = ?"
                    args.append(author)
                cache[key] = {row[0] for row in db.execute(sql, args)}
            if cache[key]:
                entries.append(cache[key])
            else:
                missing.append((q["id"], author, line))
        resolved[q["id"]] = entries
    db.close()
    return resolved, missing


def score(ranked, entries):
    """Return (hit@K, recall@K, reciprocal rank) for a ranked ID list."""
    top = ranked[:K]
    found = sum(1 for ids in entries if ids.intersection(top))
    rr = 0.0
    for rank, poem_id in enumerate(top, 1):
        if any(poem_id in ids for ids in entries):
            rr = 1 / rank
            break
    return (1.0 if found else 0.0, found / len(entries), rr)


def report(name, queries, gold, run):
    """Print per-category metrics and return them. run maps query id to ranked IDs."""
    by_cat = defaultdict(list)
    for q in queries:
        if gold[q["id"]]:
            by_cat[q["category"]].append(score(run[q["id"]], gold[q["id"]]))

    rows = {}
    print(f"\n{name}")
    print(f"{'category':10} {'n':>3} {'hit@10':>7} {'recall@10':>10} {'mrr@10':>7}")
    everything = []
    for cat, scores in by_cat.items():
        everything += scores
        rows[cat] = summarize(scores)
        print(fmt(cat, rows[cat]))
    rows["all"] = summarize(everything)
    print(fmt("all", rows["all"]))
    return rows


def summarize(scores):
    n = len(scores)
    return {
        "n": n,
        "hit": sum(s[0] for s in scores) / n,
        "recall": sum(s[1] for s in scores) / n,
        "mrr": sum(s[2] for s in scores) / n,
    }


def fmt(cat, m):
    return f"{cat:10} {m['n']:>3} {m['hit']:>7.3f} {m['recall']:>10.3f} {m['mrr']:>7.3f}"


def search(api, query):
    params = urllib.parse.urlencode({"q": query, "page_size": FETCH})
    with urllib.request.urlopen(f"{api}/api/v1/poems/search?{params}", timeout=30) as resp:
        return [poem["id"] for poem in json.load(resp)["data"]]


def interleave(lists):
    """Merge ranked lists round-robin, the way a client reads several searches."""
    merged, seen = [], set()
    for rank in range(max(map(len, lists), default=0)):
        for ranked in lists:
            if rank < len(ranked) and ranked[rank] not in seen:
                seen.add(ranked[rank])
                merged.append(ranked[rank])
    return merged


def cmd_check(args):
    queries = load_queries()
    gold, missing = resolve_gold(args.db, queries)
    for item in missing:
        print("MISSING", *item)
    entries = sum(len(q["gold"]) for q in queries)
    print(f"{len(queries)} queries, {entries} gold entries, {len(missing)} unresolved")
    return 1 if missing else 0


def cmd_baseline(args):
    queries = load_queries()
    gold, missing = resolve_gold(args.db, queries)
    if missing:
        raise SystemExit(f"{len(missing)} gold entries unresolved; run check first")

    literal = {q["id"]: search(args.api, q["query"]) for q in queries}
    rewritten = {q["id"]: interleave([search(args.api, kw) for kw in q["keywords"]]) for q in queries}

    results = {
        "literal": report("A. current search, raw query", queries, gold, literal),
        "keywords": report("B. current search, keywords chosen by a client model (simulated)", queries, gold, rewritten),
    }
    if args.out:
        Path(args.out).write_text(json.dumps(results, ensure_ascii=False, indent=2), encoding="utf-8")
    return 0


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = parser.add_subparsers(dest="command", required=True)

    check = sub.add_parser("check", help="verify every gold entry exists in the database")
    check.add_argument("--db", required=True)
    check.set_defaults(func=cmd_check)

    baseline = sub.add_parser("baseline", help="score the current keyword search")
    baseline.add_argument("--db", required=True)
    baseline.add_argument("--api", default="http://localhost:1279")
    baseline.add_argument("--out")
    baseline.set_defaults(func=cmd_baseline)

    args = parser.parse_args()
    return args.func(args)


if __name__ == "__main__":
    raise SystemExit(main())
