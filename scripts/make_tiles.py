#!/usr/bin/env python3
"""Split a grid tile sheet into individual Totem Crawler tile images.

The crawler draws map tiles from art under src/web/public/sprites/crawler/tiles/.
A "tile sheet" is one image holding several tiles in a regular grid (e.g. a 3x3
sheet of forest tiles). This is the tile counterpart of scripts/make_sprites.py
(which builds the Bomberman totem atlas); tiles are kept as INDIVIDUAL files
rather than one atlas because the client rotates each tile independently to match
the orientation it was laid in, and rotating a shared atlas viewport would rotate
its neighbours too.

What it does:
  1. splits the sheet into rows x cols equal cells (the tiles sit in a clean grid,
     each already framed, so an arithmetic split is exact);
  2. optionally trims a frame inset and rescales every cell to one square size;
  3. writes each cell to <out>/tile-<i>.png in reading order (top-to-bottom,
     left-to-right), plus a tiles.json manifest (index, row, col, file).

The client maps engine tile kinds (straight/bend/tee/cross/dead-end/boss) to these
files in TILE_ART (CrawlerPage.tsx); re-running with a new sheet refreshes the art
with no code change as long as the grid shape is the same.

--start N numbers the FIRST cell tile-N instead of tile-0, so a small sheet can
overwrite specific tiles without clobbering the rest: splitting a single corrected
T-junction over the existing tile-3 is `--rows 1 --cols 1 --start 3`. The manifest is
MERGED with any existing tiles.json (only the regenerated indices change), so a
partial re-split never drops the tiles it did not touch.

  scripts/make_tiles.py SHEET --rows 3 --cols 3
  scripts/make_tiles.py SHEET --rows 3 --cols 3 --size 256 --inset 8
  scripts/make_tiles.py ONE-TILE.png --rows 1 --cols 1 --start 3   # overwrite tile-3
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
    ap.add_argument("--start", type=int, default=0, help="index of the first cell (tile-<start>); overwrite a specific tile without renumbering the rest")
    args = ap.parse_args()

    img = Image.open(args.sheet).convert("RGBA")
    w, h = img.size
    cw, ch = w / args.cols, h / args.rows
    os.makedirs(args.out, exist_ok=True)

    # Merge into any existing manifest so a partial re-split keeps untouched tiles.
    by_index = {}
    existing = os.path.join(args.out, "tiles.json")
    if os.path.exists(existing):
        with open(existing) as f:
            for t in json.load(f).get("tiles", []):
                by_index[t["i"]] = t

    written = []
    i = args.start
    for r in range(args.rows):
        for c in range(args.cols):
            left = round(c * cw) + args.inset
            top = round(r * ch) + args.inset
            right = round((c + 1) * cw) - args.inset
            bottom = round((r + 1) * ch) - args.inset
            cell = img.crop((left, top, right, bottom))
            if args.size:
                cell = cell.resize((args.size, args.size), Image.LANCZOS)
            file = f"tile-{i}.png"
            cell.save(os.path.join(args.out, file))
            by_index[i] = {"i": i, "row": r, "col": c, "file": file}
            written.append(i)
            i += 1

    manifest = [by_index[k] for k in sorted(by_index)]
    with open(existing, "w") as f:
        json.dump({"size": args.size, "rows": args.rows, "cols": args.cols, "tiles": manifest}, f, indent=2)
    print(f"wrote tiles {written} ({args.size or 'source'}px) to {args.out}")


if __name__ == "__main__":
    main()
