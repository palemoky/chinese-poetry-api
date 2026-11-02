#!/usr/bin/env python3
"""Score reranking by the MCP client's own model.

An MCP client is itself a language model, so the server can return a short
candidate list and let the client pick. This script builds the top CANDIDATES
of the best GPU-free pipeline (keyword search ranked by fields and anthology,
fused with anthology-boosted int8 vectors), asks `claude -p` to pick and order
them, and scores the result on queries.jsonl and control.jsonl.

Each call runs in an empty directory with no tools and no saved session, so the
model sees only the query and the candidates. Answers are cached per query.

Usage:
    uv run --python 3.13 --with numpy eval/semantic/client_rerank.py \
        --db poetry.db --cache .embed-cache --signals signals.json
"""

import argparse
import json
import re
import subprocess
import sys
import tempfile
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

import numpy as np

sys.dont_write_bytecode = True
import embed_eval as ee
import evaluate as ev
import keyword_eval as ke
import rerank_eval as rr

CANDIDATES = 30
PICKS = 10
# 客户端看到的是完整诗词，这里截断只为控制 token，绝大多数诗短于这个长度
SNIPPET = 200
PROMPT = """用户想找一首诗词，他的描述是：{query}

诗词检索工具返回了下面 {n} 条候选。请挑出最符合用户描述的诗词，按符合程度从高到低排列，最多 {picks} 条。只能从候选里选，没有合适的就少选。

{items}

只输出一个 JSON 数组，内容是候选编号，例如 [3, 1, 12]，不要输出其他文字。"""


def ask(prompt, workdir, model):
    cmd = ["claude", "-p", "--tools", "", "--strict-mcp-config", "--no-session-persistence", "--output-format", "json"]
    if model:
        cmd += ["--model", model]
    out = subprocess.run(cmd, input=prompt, capture_output=True, text=True, cwd=workdir, timeout=600)
    if out.returncode != 0:
        raise RuntimeError(out.stderr[-500:] or out.stdout[-500:])
    return json.loads(out.stdout)


def parse_picks(text, n):
    match = re.search(r"\[[\d,\s]*\]", text)
    picks = json.loads(match.group(0)) if match else []
    seen = []
    for pick in picks:
        if 1 <= pick <= n and pick not in seen:
            seen.append(pick)
    return seen


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--db", required=True)
    parser.add_argument("--cache", required=True)
    parser.add_argument("--signals", required=True)
    parser.add_argument("--model", default="bge-m3", help="embedding model of the cached vectors")
    parser.add_argument("--client-model", help="model passed to claude --model; default is the CLI default")
    parser.add_argument("--limit", type=int, help="only ask about the first N queries, for a smoke test")
    parser.add_argument("--workers", type=int, default=4)
    parser.add_argument("--out")
    args = parser.parse_args()

    famous, control = rr.load_all()
    everything = famous + control
    gold, missing = ev.resolve_gold(args.db, everything)
    if missing:
        raise SystemExit(f"unresolved gold: {missing}")

    cache = Path(args.cache)
    corpus = ke.Corpus(args.db, args.signals)
    position = {poem_id: i for i, poem_id in enumerate(corpus.ids)}

    ids, _ = ee.load_corpus(args.db, gold, 0)
    docs = ee.normalize(np.load(cache / f"{args.model}-0.npy"))
    q_cache = json.loads((cache / "queries.json").read_text())
    q = ee.normalize(np.array([q_cache[item["query"]] for item in everything], dtype=np.float32))
    scores = (ee.quantize_int8(q).astype(np.float32) / 127) @ (ee.quantize_int8(docs).astype(np.float32) / 127).T
    anthology, _ = rr.load_signals(args.signals, ids)

    pipeline = {}
    for i, item in enumerate(everything):
        vector = rr.pool_by_prior(scores[i], ids, anthology, 0.05)
        pipeline[item["id"]] = rr.rrf_two(corpus.search(item["query"], "anthology"), vector)

    answers_path = cache / f"client-rerank-{args.client_model or 'default'}.json"
    answers = json.loads(answers_path.read_text()) if answers_path.exists() else {}
    todo = [item for item in everything if item["id"] not in answers][: args.limit]

    def describe(poem_id):
        i = position[poem_id]
        return f"{corpus.titles[i]}（{corpus.authors[i] or '佚名'}）：{corpus.bodies[i][:SNIPPET]}"

    def run(item):
        candidates = pipeline[item["id"]][:CANDIDATES]
        items = "\n".join(f"{n}. {describe(p)}" for n, p in enumerate(candidates, 1))
        prompt = PROMPT.format(query=item["query"], n=len(candidates), picks=PICKS, items=items)
        with tempfile.TemporaryDirectory() as workdir:
            reply = ask(prompt, workdir, args.client_model)
        return item["id"], {
            "picks": parse_picks(reply.get("result", ""), len(candidates)),
            "raw": reply.get("result", ""),
            "models": list(reply.get("modelUsage", {})),
            "cost": reply.get("total_cost_usd"),
        }

    with ThreadPoolExecutor(args.workers) as pool:
        for n, (qid, answer) in enumerate(pool.map(run, todo), 1):
            answers[qid] = answer
            answers_path.write_text(json.dumps(answers, ensure_ascii=False, indent=1))
            print(f"asked {n}/{len(todo)} {qid} picks={answer['picks']} models={answer['models']}", flush=True)

    if len(answers) < len(everything):
        print(f"{len(answers)}/{len(everything)} answered; rerun to finish")
        return 0

    reranked = {}
    for qid, ranked in pipeline.items():
        candidates = ranked[:CANDIDATES]
        picked = [candidates[n - 1] for n in answers[qid]["picks"]]
        rest = [p for p in candidates if p not in picked]
        reranked[qid] = picked + rest + ranked[CANDIDATES:]
    runs = {"pipeline (no GPU)": pipeline, "client model rerank top30": reranked}

    rows = {}
    rr.evaluate_set("famous", famous, gold, runs, rows)
    rr.evaluate_set("control", control, gold, runs, rows)
    rr.print_rows(rows, ["famous", "control"])

    # 重排只能在候选内调整，候选里没有答案的查询它救不了
    for name, queries in (("famous", famous), ("control", control)):
        reachable = sum(1 for item in queries if set().union(*gold[item["id"]]) & set(pipeline[item["id"]][:CANDIDATES]))
        print(f"{name}: answer within top {CANDIDATES} candidates for {reachable}/{len(queries)}")
    costs = [a["cost"] for a in answers.values() if a.get("cost")]
    print(f"models: {sorted({m for a in answers.values() for m in a['models']})}, total cost ${sum(costs):.2f}")

    if args.out:
        Path(args.out).write_text(json.dumps(rows, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
