#!/usr/bin/env python3
"""List the animals mapped to a given emoji in data/emoji.json.

Useful for auditing a suspicious mapping (e.g. after a community report that an
animal shows the wrong emoji). Emoji live in data/emoji.json, keyed by slug, so
this is a file query, not a DB query.

Usage:
  python3 scripts/list_by_emoji.py            # defaults to the boar/everzwijn 🐗
  python3 scripts/list_by_emoji.py 🐦         # any emoji passed as an argument
  python3 scripts/list_by_emoji.py --csv      # machine-readable (slug,nl,en,emoji)

Exit status is 0 even when nothing matches (an empty list is a valid answer).
"""
import csv
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
EMOJI = ROOT / "data" / "emoji.json"
ANIMALS = ROOT / "data" / "animals.json"

BOAR = "\U0001F417"  # 🐗 wild boar / everzwijn


def main():
    args = [a for a in sys.argv[1:] if a != "--csv"]
    as_csv = "--csv" in sys.argv[1:]
    target = args[0] if args else BOAR

    emoji = json.loads(EMOJI.read_text(encoding="utf-8"))
    by_slug = {a["slug"]: a for a in json.loads(ANIMALS.read_text(encoding="utf-8"))}

    rows = []
    for slug, e in emoji.items():
        if e == target:
            a = by_slug.get(slug, {})
            rows.append((slug, a.get("nl", "?"), a.get("en", "?"), e))
    rows.sort()

    if as_csv:
        w = csv.writer(sys.stdout)
        w.writerow(["slug", "nl", "en", "emoji"])
        w.writerows(rows)
        return

    print(f"Animals mapped to {target} : {len(rows)}")
    for slug, nl, en, _ in rows:
        print(f"  {slug:18} {nl:20} {en}")


if __name__ == "__main__":
    main()
