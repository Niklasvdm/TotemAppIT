#!/usr/bin/env python3
"""
Scrape Dutch descriptions from the SGV Totemzoeker website and save to data/animals.json.

Usage:
    python3 scripts/scrape_descriptions.py

Reads:  data/animals.json
Writes: data/animals.json  (adds/updates desc_nl for animals that had none)

Skips animals that already have a non-empty desc_nl.
"""

import json
import re
import sys
import urllib.request
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path

ROOT = Path(__file__).parent.parent
DATA_FILE = ROOT / "data" / "animals.json"

HEADERS = {
    "User-Agent": (
        "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 "
        "(KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
    ),
    "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
    "Accept-Language": "nl-BE,nl;q=0.9",
}

BASE_URL = "https://www.scoutsengidsenvlaanderen.be/totemzoeker/"


def fetch_desc(slug: str) -> tuple:
    """Fetch Dutch description for a slug. Returns (slug, desc_nl or None)."""
    url = BASE_URL + slug
    try:
        req = urllib.request.Request(url, headers=HEADERS)
        resp = urllib.request.urlopen(req, timeout=15)
        html = resp.read().decode("utf-8", errors="replace")
        m = re.search(r'<meta property="og:description" content="([^"]+)"', html)
        if m:
            return slug, m.group(1).strip()
        return slug, None
    except Exception as e:
        print(f"  ERROR {slug}: {e}", file=sys.stderr)
        return slug, None


def main():
    with open(DATA_FILE, encoding="utf-8") as f:
        animals = json.load(f)

    to_scrape = [a for a in animals if not a.get("desc_nl")]
    print(f"Animals total: {len(animals)}")
    print(f"Already have desc_nl: {len(animals) - len(to_scrape)}")
    print(f"Need scraping: {len(to_scrape)}")

    if not to_scrape:
        print("Nothing to scrape.")
        return

    slug_to_animal = {a["slug"]: a for a in animals}
    found = 0
    not_found = []

    print("\nScraping SGV website (20 parallel workers)…")
    with ThreadPoolExecutor(max_workers=20) as executor:
        futures = {executor.submit(fetch_desc, a["slug"]): a["slug"] for a in to_scrape}
        for i, future in enumerate(as_completed(futures), 1):
            slug, desc = future.result()
            animal = slug_to_animal[slug]
            if desc:
                animal["desc_nl"] = desc
                found += 1
                print(f"  [{i}/{len(to_scrape)}] OK  {slug}")
            else:
                not_found.append(slug)
                print(f"  [{i}/{len(to_scrape)}] --  {slug} (not found)")

    with open(DATA_FILE, "w", encoding="utf-8") as f:
        json.dump(animals, f, ensure_ascii=False, indent=2)

    print(f"\nDone.")
    print(f"  Scraped: {found}/{len(to_scrape)}")
    print(f"  Not found: {len(not_found)}")
    if not_found:
        print(f"  Missing slugs: {', '.join(not_found)}")


if __name__ == "__main__":
    main()
