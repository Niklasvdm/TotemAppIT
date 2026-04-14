#!/usr/bin/env python3
"""
Translate Dutch totem descriptions to Italian using the Anthropic API.

Usage:
    export ANTHROPIC_API_KEY=sk-...
    python3 scripts/translate_descriptions.py

Reads:  data/animals.json
Writes: data/animals.json  (adds/updates the desc_it field for each animal)

The script is idempotent: it skips animals that already have a non-empty desc_it.
Use --force to retranslate everything.
"""

import argparse
import json
import os
import sys
import time
from pathlib import Path

try:
    import anthropic
except ImportError:
    print("anthropic SDK not found. Install with:  pip install anthropic")
    sys.exit(1)

ROOT = Path(__file__).parent.parent
DATA_FILE = ROOT / "data" / "animals.json"

SYSTEM_PROMPT = (
    "Sei un traduttore esperto di italiano. Traduci fedelmente i testi olandesi in italiano. "
    "Rispondi SOLO con la traduzione, senza aggiungere prefissi, note o spiegazioni."
)

def translate(client: anthropic.Anthropic, animal: dict) -> str:
    """Return an Italian translation of the animal's Dutch description."""
    name_nl = animal["nl"]
    name_it = animal["it"]
    desc_nl = animal["desc_nl"]

    message = client.messages.create(
        model="claude-opus-4-5",
        max_tokens=512,
        system=SYSTEM_PROMPT,
        messages=[
            {
                "role": "user",
                "content": (
                    f'Traduci il seguente testo olandese in italiano. '
                    f'È la descrizione ufficiale dell\'animale totem "{name_it}" ({name_nl}) '
                    f'del sito scout belga Scouts en Gidsen Vlaanderen.\n\n'
                    f'Testo olandese:\n{desc_nl}\n\n'
                    f'Rispondi SOLO con la traduzione italiana.'
                ),
            }
        ],
    )
    return message.content[0].text.strip()


def main():
    parser = argparse.ArgumentParser(description="Translate totem descriptions NL → IT")
    parser.add_argument("--force", action="store_true", help="Re-translate even if desc_it already exists")
    parser.add_argument("--start", type=int, default=0, help="Start from index N (0-based)")
    parser.add_argument("--end", type=int, default=None, help="Stop before index N")
    parser.add_argument("--dry-run", action="store_true", help="Print what would be translated without calling the API")
    args = parser.parse_args()

    api_key = os.environ.get("ANTHROPIC_API_KEY")
    if not api_key:
        print("Error: ANTHROPIC_API_KEY environment variable not set.")
        sys.exit(1)

    with open(DATA_FILE, encoding="utf-8") as f:
        animals = json.load(f)

    client = anthropic.Anthropic(api_key=api_key)

    subset = animals[args.start : args.end]
    to_translate = [a for a in subset if args.force or not a.get("desc_it")]

    print(f"Animals in file: {len(animals)}")
    print(f"Slice [{args.start}:{args.end}]: {len(subset)}")
    print(f"Need translation: {len(to_translate)}")

    if args.dry_run:
        for a in to_translate:
            print(f"  Would translate: {a['nl']} ({a['it']})")
        return

    for i, animal in enumerate(to_translate, 1):
        name = f"{animal['it']} ({animal['nl']})"
        print(f"[{i}/{len(to_translate)}] {name} … ", end="", flush=True)
        try:
            desc_it = translate(client, animal)
            animal["desc_it"] = desc_it
            print("OK")
        except Exception as e:
            print(f"ERROR: {e}")
            # Save progress and exit so we don't lose work
            with open(DATA_FILE, "w", encoding="utf-8") as f:
                json.dump(animals, f, ensure_ascii=False, indent=2)
            print(f"Progress saved. Resume with --start {animals.index(animal)}")
            sys.exit(1)

        # Polite rate limiting
        if i % 10 == 0:
            with open(DATA_FILE, "w", encoding="utf-8") as f:
                json.dump(animals, f, ensure_ascii=False, indent=2)
            print(f"  → Saved progress ({i}/{len(to_translate)})")
            time.sleep(0.5)

    # Final save
    with open(DATA_FILE, "w", encoding="utf-8") as f:
        json.dump(animals, f, ensure_ascii=False, indent=2)

    filled = sum(1 for a in animals if a.get("desc_it"))
    print(f"\nDone. {filled}/{len(animals)} animals now have an Italian description.")
    print(f"Next step:  python3 scripts/generate_pages.py")


if __name__ == "__main__":
    main()
