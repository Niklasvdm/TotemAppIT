#!/usr/bin/env python3
"""
Translate animal descriptions to Italian and English using the DeepL API.
Skips animals that already have a translation (safe to re-run).

Usage:
    export DEEPL_API_KEY="your-key-here"   # free keys end with :fx

    # 1. Check your quota and see what would be translated (no API calls):
    python3 scripts/translate_descriptions.py --dry-run

    # 2. Test with 2 animals to verify key + output quality before full run:
    python3 scripts/translate_descriptions.py --test

    # 3. Full run:
    python3 scripts/translate_descriptions.py

Free-tier limit: 500,000 chars/month.
Our full dataset is ~238k chars (311 IT + 440 EN), well within the limit.
"""
import argparse
import html
import json
import os
import sys
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


def translate(text: str, target_lang: str, api_key: str) -> str:
    if not text:
        return ""
    # Decode HTML entities in source (e.g. &#039; -> ') before sending
    text = html.unescape(text)
    payload = json.dumps({
        "text": [text],
        "source_lang": "NL",
        "target_lang": target_lang,
    }).encode()
    req = urllib.request.Request(
        f"{base_url(api_key)}/translate",
        data=payload,
        headers={
            "Authorization": f"DeepL-Auth-Key {api_key}",
            "Content-Type": "application/json",
        },
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return json.loads(resp.read())["translations"][0]["text"]
    except urllib.error.HTTPError as e:
        body = e.read().decode(errors="replace")
        print(f"  HTTP {e.code}: {body}", file=sys.stderr)
        raise


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

    needs_it = [a for a in animals if a.get("desc_nl") and not a.get("desc_it")]
    needs_en = [a for a in animals if a.get("desc_nl") and not a.get("desc_en")]
    chars_it  = sum(len(html.unescape(a["desc_nl"])) for a in needs_it)
    chars_en  = sum(len(html.unescape(a["desc_nl"])) for a in needs_en)
    chars_total = chars_it + chars_en

    print(f"\nWork to do:")
    print(f"  Italian : {len(needs_it):3d} animals  ({chars_it:,} chars)")
    print(f"  English : {len(needs_en):3d} animals  ({chars_en:,} chars)")
    print(f"  Total   :             ({chars_total:,} chars)")

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
            if a.get("desc_nl") and (not a.get("desc_it") or not a.get("desc_en")):
                samples.append(a)
            if len(samples) == 2:
                break

        if not samples:
            print("\nNothing to translate — all animals already have both languages.")
            return

        print(f"\n-- TEST MODE: translating {len(samples)} animal(s), NOT saving --\n")
        for a in samples:
            print(f"Slug   : {a['slug']}")
            print(f"Dutch  : {a['desc_nl']}")
            if not a.get("desc_it"):
                it = translate(a["desc_nl"], "IT", api_key)
                print(f"Italian: {it}")
            else:
                print(f"Italian: (already exists) {a['desc_it']}")
            if not a.get("desc_en"):
                en = translate(a["desc_nl"], "EN-GB", api_key)
                print(f"English: {en}")
            else:
                print(f"English: (already exists) {a['desc_en']}")
            print()
        print("Test OK. Run without --test to translate everything.")
        return

    # --- Full run ---
    total = len(animals)
    it_done = en_done = it_skip = en_skip = no_nl = 0

    for i, animal in enumerate(animals, 1):
        slug = animal.get("slug", "?")
        desc_nl = animal.get("desc_nl", "")

        if not desc_nl:
            no_nl += 1
            continue

        changed = False

        if animal.get("desc_it"):
            it_skip += 1
        else:
            animal["desc_it"] = translate(desc_nl, "IT", api_key)
            it_done += 1
            changed = True
            print(f"[{i}/{total}] IT  {slug}")

        if animal.get("desc_en"):
            en_skip += 1
        else:
            animal["desc_en"] = translate(desc_nl, "EN-GB", api_key)
            en_done += 1
            changed = True
            print(f"[{i}/{total}] EN  {slug}")

        # Save after every animal so progress survives interruption
        if changed:
            save(animals, DATA_FILE)

    print(
        f"\nDone."
        f"\n  Italian : {it_done} translated, {it_skip} already existed"
        f"\n  English : {en_done} translated, {en_skip} already existed"
        f"\n  No desc : {no_nl} animals skipped (no desc_nl)"
        f"\n\nNext step: python3 scripts/generate_pages.py"
    )


if __name__ == "__main__":
    main()
