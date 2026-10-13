#!/usr/bin/env python3
"""Cut a grid tile sheet into the crawler's individual tile images.

The crawler draws each map tile from its own PNG under
src/web/public/sprites/crawler/tiles/<biome>/tile-<N>.png. This script takes one
"tile sheet" (an image with several tiles laid out in a regular grid) and writes
out those PNGs, numbered tile-0, tile-1, ... in reading order (left to right, top
to bottom), plus a tiles.json manifest. Tiles are separate files, not one atlas,
because the client rotates each tile on its own to match how it was laid; a shared
atlas would rotate its neighbours too.

WHAT GETS WRITTEN
  Every cell becomes <out>/tile-<index>.png. The index is the cell's position in
  reading order, shifted by --start. The manifest is merged with any existing
  tiles.json, so writing a few tiles never drops the ones you did not touch. The
  engine tile kinds (straight, bend, tee, ...) map to these files in TILE_ART
  (src/web/src/games/crawler/logic.ts); re-running refreshes the art, no code change.

THE KNOBS THAT PICK TILES
  --rows / --cols  describe the sheet's grid (always required).
  --start N        number the first written tile tile-N instead of tile-0.
  --only K         write ONLY cell K (0-based, reading order) instead of them all.
  --variant S      write tile-<start><S>.png (e.g. tile-16b): an alternate look for the
                   same kind, listed beside the base in TILE_ART (variant rules there).
  So --start says what to CALL the output, --only which cell to TAKE, and --variant
  marks it an alternate of that number rather than a replacement.

RECIPES
  # a whole 3x3 sheet -> tile-0 .. tile-8
  scripts/make_tiles.py sheet.png --rows 3 --cols 3

  # same, into a biome folder, trimming an 8px frame, 256px tiles
  scripts/make_tiles.py sheet.png --rows 3 --cols 3 --inset 8 --size 256 \
    --out src/web/public/sprites/crawler/tiles/woodland

  # one corrected tile over the existing tile-3 (sheet holds just that tile)
  scripts/make_tiles.py one-tile.png --rows 1 --cols 1 --start 3

  # the 4th cell of a 3x3 sheet (reading order: index 3), written as tile-9
  scripts/make_tiles.py sheet.png --rows 3 --cols 3 --only 3 --start 9

  # cell 1 of a sheet as a SECOND trap look: tile-16b (a variant of tile-16)
  scripts/make_tiles.py sheet.png --rows 3 --cols 3 --only 1 --start 16 --variant b \
    --out src/web/public/sprites/crawler/tiles/ruins
"""
import argparse
import json
import os

from PIL import Image

DEFAULT_OUT = "src/web/public/sprites/crawler/tiles"


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("sheet", help="the grid tile sheet image")
    ap.add_argument("--rows", type=int, required=True, help="rows of tiles in the sheet")
    ap.add_argument("--cols", type=int, required=True, help="columns of tiles in the sheet")
    ap.add_argument("--out", default=DEFAULT_OUT, help=f"output dir (default {DEFAULT_OUT})")
    ap.add_argument("--size", type=int, default=256, help="square px per tile (0 keeps the source size)")
    ap.add_argument("--inset", type=int, default=0, help="px trimmed from each side of a cell (drop the frame)")
    ap.add_argument("--start", type=int, default=0, help="number the first written tile tile-<start> (default 0)")
    ap.add_argument("--only", type=int, default=-1, help="write ONLY this cell (0-based, reading order), named by --start")
    ap.add_argument("--variant", default="", help="suffix for a variant look of the same kind, e.g. 'b' -> tile-<start>b.png (list it in TILE_ART)")
    args = ap.parse_args()

    img = Image.open(args.sheet).convert("RGBA")
    w, h = img.size
    cw, ch = w / args.cols, h / args.rows
    os.makedirs(args.out, exist_ok=True)

    # Merge into any existing manifest so a partial re-split keeps untouched tiles. Keys are
    # the tile stem as a string ("16", "16b") so a variant lives beside its base tile.
    by_stem = {}
    existing = os.path.join(args.out, "tiles.json")
    if os.path.exists(existing):
        with open(existing) as f:
            for t in json.load(f).get("tiles", []):
                by_stem[str(t["i"])] = t

    written = []
    i = args.start
    cell_no = -1  # reading-order index within the sheet, for --only
    for r in range(args.rows):
        for c in range(args.cols):
            cell_no += 1
            if args.only >= 0 and cell_no != args.only:
                continue  # skip every cell but the one requested
            left = round(c * cw) + args.inset
            top = round(r * ch) + args.inset
            right = round((c + 1) * cw) - args.inset
            bottom = round((r + 1) * ch) - args.inset
            cell = img.crop((left, top, right, bottom))
            if args.size:
                cell = cell.resize((args.size, args.size), Image.LANCZOS)
            stem = f"{i}{args.variant}"  # e.g. "16" or, for a variant, "16b"
            file = f"tile-{stem}.png"
            cell.save(os.path.join(args.out, file))
            by_stem[stem] = {"i": stem, "row": r, "col": c, "file": file}
            written.append(stem)
            i += 1

    manifest = [by_stem[k] for k in sorted(by_stem, key=str)]
    with open(existing, "w") as f:
        json.dump({"size": args.size, "rows": args.rows, "cols": args.cols, "tiles": manifest}, f, indent=2)
    print(f"wrote tiles {written} ({args.size or 'source'}px) to {args.out}")


if __name__ == "__main__":
    main()
