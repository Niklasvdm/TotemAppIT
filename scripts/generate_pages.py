#!/usr/bin/env python3
"""
Generate individual animal pages and rebuild index.html.

Usage:
    python3 scripts/generate_pages.py

Reads:  data/animals.json
Writes: animals/<slug>.html  (one page per animal)
        index.html           (updated ANIMALS array with desc_it embedded)

Run this after translate_descriptions.py has populated desc_it in animals.json.
"""

import json
import re
from pathlib import Path
from html import escape

ROOT = Path(__file__).parent.parent
DATA_FILE = ROOT / "data" / "animals.json"
ANIMALS_DIR = ROOT / "animals"
INDEX_FILE = ROOT / "index.html"

SHARED_CSS = """
  :root {
    --ink:#1a1208; --green:#2c5f2e; --green-mid:#3d7a40; --green-light:#e8f4e8;
    --green-pale:#f4faf4; --red:#b91c1c;
    --amber:#c17f24;
    --bg:#faf8f4; --surface:#fff; --surface2:#f6f3ee;
    --border:#ddd5c8; --muted:#7a6e60;
    --tag-bg:#ecf4ec; --tag-border:#b8d4b9; --r:10px;
  }
  *{margin:0;padding:0;box-sizing:border-box;}
  body{background:var(--bg);color:var(--ink);font-family:'DM Sans',sans-serif;min-height:100vh;}
  header{background:var(--green);padding:1.2rem 2rem;display:flex;align-items:center;justify-content:space-between;gap:1rem;flex-wrap:wrap;}
  .h-title{font-family:'Playfair Display',serif;font-size:1.5rem;font-weight:700;color:#fff;}
  .h-sub{font-size:.78rem;color:rgba(255,255,255,.6);margin-top:.1rem;}
  .h-back{font-size:.8rem;color:rgba(255,255,255,.85);text-decoration:none;background:rgba(255,255,255,.12);border:1px solid rgba(255,255,255,.2);padding:.3rem .75rem;border-radius:20px;transition:background .15s;}
  .h-back:hover{background:rgba(255,255,255,.22);}
"""

ANIMAL_PAGE_CSS = """
  .page{max-width:680px;margin:2.5rem auto;padding:0 1.2rem 4rem;}
  .animal-name{font-family:'Playfair Display',serif;font-size:2.2rem;font-weight:700;color:var(--green);line-height:1.1;margin-bottom:.25rem;}
  .name-nl{font-size:.9rem;color:var(--muted);margin-bottom:.1rem;}
  .name-alt{font-size:.82rem;color:var(--muted);font-style:italic;margin-bottom:1.5rem;}
  .section{margin-bottom:1.6rem;}
  .slabel{font-size:.65rem;font-weight:500;letter-spacing:.12em;text-transform:uppercase;color:var(--muted);margin-bottom:.55rem;display:block;}
  .traits{display:flex;flex-wrap:wrap;gap:.3rem;}
  .trait{font-size:.8rem;padding:.28rem .65rem;border-radius:20px;background:var(--tag-bg);color:var(--green);border:1px solid var(--tag-border);}
  .desc{font-size:.97rem;line-height:1.82;color:var(--ink);}
  .desc-other{font-size:.82rem;color:var(--muted);background:var(--surface2);border-left:3px solid var(--border);padding:.65rem .9rem;border-radius:0 6px 6px 0;line-height:1.65;font-style:italic;margin-top:.8rem;}
  .desc-other .orig-label{font-size:.63rem;font-weight:500;letter-spacing:.1em;text-transform:uppercase;color:var(--muted);display:block;margin-bottom:.3rem;font-style:normal;}
  .source{font-size:.72rem;color:var(--muted);border-top:1px solid var(--border);padding-top:.9rem;}
  .source a{color:var(--green);}
"""

FONTS = 'https://fonts.googleapis.com/css2?family=Playfair+Display:wght@400;600;700&family=DM+Sans:ital,wght@0,300;0,400;0,500;1,300&display=swap'


def animal_page_html(animal: dict) -> str:
    name_it  = escape(animal["it"])
    name_nl  = escape(animal["nl"])
    name_alt = escape(animal.get("alt", ""))
    desc_it  = escape(animal.get("desc_it", "")).replace("\n", "<br>")
    desc_en  = escape(animal.get("desc_en", "")).replace("\n", "<br>")
    desc_nl  = escape(animal.get("desc_nl", "")).replace("\n", "<br>")
    slug_url = escape(animal["slug"])
    sgv_url  = f"https://www.scoutsengidsenvlaanderen.be/totemzoeker/{slug_url}"

    traits_html = "".join(
        f'<span class="trait">{escape(t)}</span>' for t in animal.get("traits_it", [])
    )

    # Primary description block (Italian preferred, Dutch fallback)
    if desc_it:
        description_html = f'<p class="desc">{desc_it}</p>'
    else:
        description_html = (
            f'<div class="desc-other">'
            f'<span class="orig-label">Testo originale (Nederlands — traduzione non ancora disponibile)</span>'
            f'{desc_nl}'
            f'</div>'
        )

    # Secondary blocks: English then Dutch originals
    english_block = ""
    if desc_en:
        english_block = (
            f'<div class="desc-other">'
            f'<span class="orig-label">🇬🇧 English</span>'
            f'{desc_en}'
            f'</div>'
        )

    dutch_block = ""
    if desc_it and desc_nl:
        dutch_block = (
            f'<div class="desc-other">'
            f'<span class="orig-label">🇧🇪 Nederlands (origineel)</span>'
            f'{desc_nl}'
            f'</div>'
        )

    alt_line = f'<div class="name-alt">{name_alt}</div>' if name_alt else ""

    return f"""<!DOCTYPE html>
<html lang="it">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{name_it} – Totem Scouts</title>
<link href="{FONTS}" rel="stylesheet">
<style>
{SHARED_CSS}
{ANIMAL_PAGE_CSS}
</style>
</head>
<body>

<header>
  <div>
    <div class="h-title">🐾 Cercatore di Totem</div>
    <div class="h-sub">Scouts en Gidsen Vlaanderen · Totemzoeker</div>
  </div>
  <a class="h-back" href="../index.html">← Tutti i totem</a>
</header>

<div class="page">
  <div class="animal-name">{name_it}</div>
  <div class="name-nl">🇧🇪 {name_nl}</div>
  {alt_line}

  <div class="section">
    <span class="slabel">Caratteristiche</span>
    <div class="traits">{traits_html}</div>
  </div>

  <div class="section">
    <span class="slabel">Descrizione</span>
    {description_html}
    {english_block}
    {dutch_block}
  </div>

  <div class="source">
    Fonte: <a href="{sgv_url}" target="_blank" rel="noopener">Scouts en Gidsen Vlaanderen – Totemzoeker</a>
  </div>
</div>

</body>
</html>
"""


def update_index_animals(animals: list) -> None:
    """Replace the ANIMALS constant in index.html with the current data.

    Descriptions (desc_nl, desc_it, desc_en) are intentionally stripped out —
    they live in the individual animals/*.html pages, not in index.html.
    index.html only needs slug, names, and traits for search/filtering.
    """
    with open(INDEX_FILE, encoding="utf-8") as f:
        source = f.read()

    STRIP_FIELDS = {"desc_nl", "desc_it", "desc_en"}
    slim = [{k: v for k, v in a.items() if k not in STRIP_FIELDS} for a in animals]
    new_array = json.dumps(slim, ensure_ascii=False, separators=(",", ":"))
    updated = re.sub(
        r'const ANIMALS = \[.*?\];',
        f'const ANIMALS = {new_array};',
        source,
        flags=re.DOTALL,
    )

    if updated == source:
        print("  index.html ANIMALS array unchanged (or pattern not matched).")
    else:
        with open(INDEX_FILE, "w", encoding="utf-8") as f:
            f.write(updated)
        print("  index.html ANIMALS array updated.")


def update_index_descriptions(animals: list) -> None:
    """Replace the DESCRIPTIONS constant in index.html with current description data.

    Embedded inline (like ANIMALS) so it works with file:// and needs no network request.
    """
    with open(INDEX_FILE, encoding="utf-8") as f:
        source = f.read()

    descs = {}
    for a in animals:
        entry = {}
        if a.get("desc_it"): entry["it"] = a["desc_it"]
        if a.get("desc_en"): entry["en"] = a["desc_en"]
        if a.get("desc_nl"): entry["nl"] = a["desc_nl"]
        if entry:
            descs[a["slug"]] = entry

    new_obj = json.dumps(descs, ensure_ascii=False, separators=(",", ":"))
    updated = re.sub(
        r'const DESCRIPTIONS = \{.*?\};',
        f'const DESCRIPTIONS = {new_obj};',
        source,
        flags=re.DOTALL,
    )

    if updated == source:
        print("  index.html DESCRIPTIONS unchanged (or pattern not matched).")
    else:
        with open(INDEX_FILE, "w", encoding="utf-8") as f:
            f.write(updated)
        size_kb = len(updated.encode()) // 1024
        print(f"  index.html DESCRIPTIONS updated (~{size_kb} KB total)")


def main():
    with open(DATA_FILE, encoding="utf-8") as f:
        animals = json.load(f)

    ANIMALS_DIR.mkdir(exist_ok=True)

    translated = sum(1 for a in animals if a.get("desc_it"))
    print(f"Loaded {len(animals)} animals ({translated} with Italian description)")

    for animal in animals:
        slug = animal["slug"]
        out_path = ANIMALS_DIR / f"{slug}.html"
        html = animal_page_html(animal)
        out_path.write_text(html, encoding="utf-8")

    print(f"Generated {len(animals)} pages in animals/")

    print("Updating index.html ANIMALS array …")
    update_index_animals(animals)

    print("Updating index.html DESCRIPTIONS …")
    update_index_descriptions(animals)

    print(f"\nDone.")
    print(f"  animals/   → {len(animals)} individual pages")
    print(f"  index.html → ANIMALS array + DESCRIPTIONS updated")


if __name__ == "__main__":
    main()
