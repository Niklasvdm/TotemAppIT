#!/usr/bin/env python3
"""
Translate animal descriptions, names, and traits to Italian and English using the DeepL API.
Skips animals that already have a translation (safe to re-run).

Fields translated:
  desc_it  — Dutch description → Italian
  desc_en  — Dutch description → English
  en       — Dutch name → English
  traits_en — Dutch traits list → English (batched per animal)

Usage:
    export DEEPL_API_KEY="your-key-here"   # free keys end with :fx

    # 1. Check your quota and see what would be translated (no API calls):
    python3 scripts/translate_descriptions.py --dry-run

    # 2. Test with 2 animals to verify key + output quality before full run:
    python3 scripts/translate_descriptions.py --test

    # 3. Full run:
    python3 scripts/translate_descriptions.py

Free-tier limit: 500,000 chars/month.
"""
import argparse
import html
import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

ROOT = Path(__file__).parent.parent
DATA_FILE = ROOT / "data" / "animals.json"


# ---------------------------------------------------------------------------
# DeepL helpers
# ---------------------------------------------------------------------------

def base_url(key: str) -> str:
    """Free-tier keys end with :fx and use a different hostname."""
    if key.endswith(":fx"):
        return "https://api-free.deepl.com/v2"
    return "https://api.deepl.com/v2"


def check_usage(api_key: str) -> dict:
    """Return DeepL usage dict: {character_count, character_limit}."""
    req = urllib.request.Request(
        f"{base_url(api_key)}/usage",
        headers={"Authorization": f"DeepL-Auth-Key {api_key}"},
    )
    with urllib.request.urlopen(req, timeout=15) as resp:
        return json.loads(resp.read())


def _deepl_post(payload: bytes, api_key: str) -> list:
    """POST to DeepL /translate and return the translations list. Retries on 429."""
    req = urllib.request.Request(
        f"{base_url(api_key)}/translate",
        data=payload,
        headers={
            "Authorization": f"DeepL-Auth-Key {api_key}",
            "Content-Type": "application/json",
        },
    )
    for attempt in range(5):
        try:
            with urllib.request.urlopen(req, timeout=30) as resp:
                return json.loads(resp.read())["translations"]
        except urllib.error.HTTPError as e:
            if e.code == 429:
                wait = 2 ** attempt  # 1s, 2s, 4s, 8s, 16s
                print(f"  Rate limited — waiting {wait}s before retry {attempt + 1}/5 …")
                time.sleep(wait)
            else:
                body = e.read().decode(errors="replace")
                print(f"  HTTP {e.code}: {body}", file=sys.stderr)
                raise
    raise RuntimeError("Still rate limited after 5 retries")


def translate(text: str, target_lang: str, api_key: str) -> str:
    """Translate a single text string."""
    if not text:
        return ""
    text = html.unescape(text)
    payload = json.dumps({
        "text": [text],
        "source_lang": "NL",
        "target_lang": target_lang,
    }).encode()
    return _deepl_post(payload, api_key)[0]["text"]


def translate_many(texts: list, target_lang: str, api_key: str) -> list:
    """Translate a list of strings in a single DeepL request (batched)."""
    if not texts:
        return []
    decoded = [html.unescape(t) for t in texts]
    payload = json.dumps({
        "text": decoded,
        "source_lang": "NL",
        "target_lang": target_lang,
    }).encode()
    return [t["text"] for t in _deepl_post(payload, api_key)]


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def save(animals: list, path: Path) -> None:
    with open(path, "w", encoding="utf-8") as f:
        json.dump(animals, f, ensure_ascii=False, indent=2)


def main() -> None:
    parser = argparse.ArgumentParser(description="Translate totem descriptions via DeepL")
    parser.add_argument(
        "--dry-run", action="store_true",
        help="Show what would be translated and quota info; make no API calls",
    )
    parser.add_argument(
        "--test", action="store_true",
        help="Translate only the first 2 animals that need it, print results, then stop",
    )
    parser.add_argument(
        "--limit", type=int, metavar="N",
        help="Translate at most N animals, save them, then stop (good for spot-checking before a full run)",
    )
    args = parser.parse_args()

    api_key = os.environ.get("DEEPL_API_KEY", "").strip()
    if not api_key:
        print("Error: set the DEEPL_API_KEY environment variable.", file=sys.stderr)
        sys.exit(1)

    # --- Validate key and show quota ---
    print("Checking DeepL API key and quota …")
    try:
        usage = check_usage(api_key)
    except urllib.error.HTTPError as e:
        if e.code == 403:
            print("Error: API key rejected (HTTP 403). Check DEEPL_API_KEY.", file=sys.stderr)
        else:
            print(f"Error reaching DeepL API: HTTP {e.code}", file=sys.stderr)
        sys.exit(1)
    except Exception as e:
        print(f"Error reaching DeepL API: {e}", file=sys.stderr)
        sys.exit(1)

    used = usage["character_count"]
    limit = usage["character_limit"]
    remaining = limit - used
    print(f"  Quota used : {used:,} / {limit:,} chars")
    print(f"  Remaining  : {remaining:,} chars")

    # --- Load data ---
    with open(DATA_FILE, encoding="utf-8") as f:
        animals = json.load(f)

    needs_it      = [a for a in animals if a.get("desc_nl") and not a.get("desc_it")]
    needs_en      = [a for a in animals if a.get("desc_nl") and not a.get("desc_en")]
    needs_en_name = [a for a in animals if a.get("nl") and not a.get("en")]
    needs_en_traits = [a for a in animals if a.get("traits_nl") and not a.get("traits_en")]
    chars_it      = sum(len(html.unescape(a["desc_nl"])) for a in needs_it)
    chars_en      = sum(len(html.unescape(a["desc_nl"])) for a in needs_en)
    chars_en_name = sum(len(a["nl"]) for a in needs_en_name)
    chars_en_traits = sum(sum(len(t) for t in a["traits_nl"]) for a in needs_en_traits)
    chars_total   = chars_it + chars_en + chars_en_name + chars_en_traits

    print(f"\nWork to do:")
    print(f"  IT descriptions : {len(needs_it):3d} animals  ({chars_it:,} chars)")
    print(f"  EN descriptions : {len(needs_en):3d} animals  ({chars_en:,} chars)")
    print(f"  EN names        : {len(needs_en_name):3d} animals  ({chars_en_name:,} chars)")
    print(f"  EN traits       : {len(needs_en_traits):3d} animals  ({chars_en_traits:,} chars)")
    print(f"  Total           :             ({chars_total:,} chars)")

    if chars_total > remaining:
        print(f"\nWarning: need {chars_total:,} chars but only {remaining:,} remaining!", file=sys.stderr)
        if not args.dry_run and not args.test:
            print("Aborting. Use --test to try a small batch anyway.", file=sys.stderr)
            sys.exit(1)
    else:
        print(f"  OK — {remaining - chars_total:,} chars will remain after full run")

    if args.dry_run:
        print("\n--dry-run: no translations performed.")
        return

    # --- Test mode: translate 2 animals and print output ---
    if args.test:
        samples = []
        for a in animals:
            needs = (
                (a.get("desc_nl") and (not a.get("desc_it") or not a.get("desc_en")))
                or (a.get("nl") and not a.get("en"))
                or (a.get("traits_nl") and not a.get("traits_en"))
            )
            if needs:
                samples.append(a)
            if len(samples) == 2:
                break

        if not samples:
            print("\nNothing to translate — all animals already have all fields.")
            return

        print(f"\n-- TEST MODE: translating {len(samples)} animal(s), NOT saving --\n")
        for a in samples:
            print(f"Slug       : {a['slug']}")
            # EN name
            if not a.get("en") and a.get("nl"):
                en_name = translate(a["nl"], "EN-GB", api_key)
                print(f"EN name    : {en_name}")
            else:
                print(f"EN name    : (already exists) {a.get('en', '—')}")
            # EN traits
            if not a.get("traits_en") and a.get("traits_nl"):
                en_traits = translate_many(a["traits_nl"], "EN-GB", api_key)
                print(f"EN traits  : {en_traits}")
            else:
                print(f"EN traits  : (already exists) {a.get('traits_en', '—')}")
            # Descriptions
            if a.get("desc_nl"):
                print(f"Dutch desc : {a['desc_nl'][:80]}…")
                if not a.get("desc_it"):
                    it = translate(a["desc_nl"], "IT", api_key)
                    print(f"IT desc    : {it[:80]}…")
                else:
                    print(f"IT desc    : (already exists)")
                if not a.get("desc_en"):
                    en = translate(a["desc_nl"], "EN-GB", api_key)
                    print(f"EN desc    : {en[:80]}…")
                else:
                    print(f"EN desc    : (already exists)")
            print()
        print("Test OK. Run without --test to translate everything.")
        return

    # --- Full / limited run ---
    total = len(animals)
    counters = dict(
        desc_it_done=0, desc_it_skip=0,
        desc_en_done=0, desc_en_skip=0,
        en_name_done=0, en_name_skip=0,
        en_traits_done=0, en_traits_skip=0,
        no_nl=0,
    )
    translated_count = 0

    for i, animal in enumerate(animals, 1):
        slug = animal.get("slug", "?")
        desc_nl = animal.get("desc_nl", "")
        changed = False

        # ── English name ──────────────────────────────────────────────
        if animal.get("en"):
            counters["en_name_skip"] += 1
        elif animal.get("nl"):
            animal["en"] = translate(animal["nl"], "EN-GB", api_key)
            counters["en_name_done"] += 1
            changed = True
            print(f"[{i}/{total}] EN-name  {slug}  →  {animal['en']}")
            time.sleep(0.5)

        # ── English traits ────────────────────────────────────────────
        if animal.get("traits_en"):
            counters["en_traits_skip"] += 1
        elif animal.get("traits_nl"):
            animal["traits_en"] = translate_many(animal["traits_nl"], "EN-GB", api_key)
            counters["en_traits_done"] += 1
            changed = True
            print(f"[{i}/{total}] EN-traits {slug}")
            time.sleep(0.5)

        # ── Descriptions ──────────────────────────────────────────────
        if not desc_nl:
            counters["no_nl"] += 1
        else:
            if animal.get("desc_it"):
                counters["desc_it_skip"] += 1
            else:
                animal["desc_it"] = translate(desc_nl, "IT", api_key)
                counters["desc_it_done"] += 1
                changed = True
                print(f"[{i}/{total}] IT-desc  {slug}")
                time.sleep(0.5)

            if animal.get("desc_en"):
                counters["desc_en_skip"] += 1
            else:
                animal["desc_en"] = translate(desc_nl, "EN-GB", api_key)
                counters["desc_en_done"] += 1
                changed = True
                print(f"[{i}/{total}] EN-desc  {slug}")
                time.sleep(0.5)

        # Save after every animal so progress survives interruption
        if changed:
            save(animals, DATA_FILE)
            translated_count += 1
            if args.limit and translated_count >= args.limit:
                print(f"\nReached --limit {args.limit}. Stopping.")
                print("Check the results, then run without --limit to finish.")
                return

    print(
        f"\nDone."
        f"\n  IT descriptions : {counters['desc_it_done']} translated, {counters['desc_it_skip']} already existed"
        f"\n  EN descriptions : {counters['desc_en_done']} translated, {counters['desc_en_skip']} already existed"
        f"\n  EN names        : {counters['en_name_done']} translated, {counters['en_name_skip']} already existed"
        f"\n  EN traits       : {counters['en_traits_done']} translated, {counters['en_traits_skip']} already existed"
        f"\n  No desc_nl      : {counters['no_nl']} animals skipped (no Dutch description)"
        f"\n\nNext step: python3 scripts/generate_pages.py"
    )


if __name__ == "__main__":
    main()
