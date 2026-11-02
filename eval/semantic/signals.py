#!/usr/bin/env python3
"""Build popularity signals for the eval from the poetry-data submodule.

Poems are matched to the database by shared lines, because anthologies keep
different text variants than the full collections. The output is keyed by poem
ID, so rebuild it for each database: IDs change between data releases.

Usage:
    signals.py --data poetry-data --db poetry.db --out signals.json
"""

import argparse
import glob
import json
import os
import re
import sqlite3
from collections import Counter, defaultdict

NON_HAN = re.compile(r"[^㐀-鿿\U00020000-\U0002ffff]")
BREAK = re.compile(r"[，。！？；、,.!?;：:\s]+")
ANTHOLOGIES = {
    "ts300": "蒙学/tangshisanbaishou.json",
    "qjs": "蒙学/qianjiashi.json",
    "sc300": "宋词/宋词三百首.json",
}
# 百度覆盖最全，但这里保留全部引擎，方便以后换算法
ENGINES = ("baidu", "so360", "bing", "bing_en", "google")
# 出现在太多诗里的句子是套语，不能用来判断是同一首诗
MAX_LINE_OWNERS = 30


def lines_of(paragraphs):
    found = []
    for paragraph in paragraphs:
        if isinstance(paragraph, str):
            found += [NON_HAN.sub("", part) for part in BREAK.split(paragraph)]
    return {line for line in found if len(line) >= 4}


def build_index(db_path):
    """Map each line to poem IDs. Sources mix scripts, and both tables share IDs."""
    db = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True)
    index = defaultdict(set)
    for table in ("poems_zh_hant", "poems_zh_hans"):
        for poem_id, content in db.execute(f"SELECT id, content FROM {table}"):
            for line in lines_of(json.loads(content)):
                index[line].add(poem_id)
    db.close()
    return index


def match(index, paragraphs):
    """Return IDs of the poem with these lines, including its duplicate versions."""
    lines = lines_of(paragraphs)
    votes = Counter()
    for line in lines:
        owners = index.get(line, ())
        if len(owners) <= MAX_LINE_OWNERS:
            votes.update(owners)
    if not votes:
        return set()
    best = max(votes.values())
    if best < max(2, 0.4 * len(lines)):
        return set()
    return {poem_id for poem_id, count in votes.items() if count >= max(2, 0.7 * best)}


def walk(node):
    """Yield every poem in a nested anthology file."""
    if isinstance(node, dict):
        if "paragraphs" in node:
            yield node
        node = list(node.values())
    if isinstance(node, list):
        for child in node:
            if isinstance(child, (list, dict)):
                yield from walk(child)


def add_anthologies(index, root, signals):
    for name, relative in ANTHOLOGIES.items():
        with open(os.path.join(root, relative), encoding="utf-8") as f:
            poems = list(walk(json.load(f)))
        matched = 0
        for poem in poems:
            ids = match(index, poem["paragraphs"])
            matched += bool(ids)
            for poem_id in ids:
                signals[poem_id][name] = 1
        print(f"{name}: {matched}/{len(poems)} matched", flush=True)


def add_search_counts(index, root, signals):
    """rank/ files align with source files by position; verify before trusting a row."""
    records = matched = 0
    for path in glob.glob(os.path.join(root, "rank/poet/*.json")) + glob.glob(os.path.join(root, "rank/ci/*.json")):
        kind, dynasty, part = re.match(r"(poet|ci)\.(tang|song)\.rank\.(\w+)\.json", os.path.basename(path)).groups()
        source = os.path.join(root, "全唐诗" if kind == "poet" else "宋词", f"{kind}.{dynasty}.{part}.json")
        if not os.path.exists(source):
            continue
        with open(path, encoding="utf-8") as f:
            ranks = json.load(f)
        with open(source, encoding="utf-8") as f:
            poems = json.load(f)
        for rank, poem in zip(ranks, poems):
            records += 1
            same_title = (rank.get("title") or rank.get("rhythmic")) == (poem.get("title") or poem.get("rhythmic"))
            if rank.get("author") != poem.get("author") or not same_title:
                continue
            ids = match(index, poem.get("paragraphs", []))
            matched += bool(ids)
            counts = [rank.get(engine) or 0 for engine in ENGINES]
            for poem_id in ids:
                old = signals[poem_id].get("rank", [0] * len(ENGINES))
                signals[poem_id]["rank"] = [max(a, b) for a, b in zip(old, counts)]
    print(f"search counts: {matched}/{records} matched", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--data", default="poetry-data")
    parser.add_argument("--db", required=True)
    parser.add_argument("--out", required=True)
    args = parser.parse_args()

    index = build_index(args.db)
    signals = defaultdict(dict)
    add_anthologies(index, args.data, signals)
    add_search_counts(index, args.data, signals)

    with open(args.out, "w", encoding="utf-8") as f:
        json.dump(signals, f)
    anthology = sum(1 for entry in signals.values() if set(entry) - {"rank"})
    print(f"{len(signals)} poems with a signal, {anthology} in an anthology")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
