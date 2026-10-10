#!/usr/bin/env python3
"""Split an enemy sprite sheet into individual Totem Crawler enemy frames.

This is the enemy counterpart of scripts/make_sprites.py (totem atlas) and
make_tiles.py (map tiles). An enemy sheet is one image holding the creature in
several poses on a flat (usually white) background, laid out in a grid. It:

  1. splits the sheet into rows x cols cells (arithmetic, the poses sit on a grid);
  2. strips the flat background to transparency by flooding in from the corners
     (so a white highlight *inside* the creature survives), unless --no-whitebg;
  3. crops each frame to its opaque content and centres it on a square canvas at
     one shared size;
  4. writes each frame to src/web/public/sprites/crawler/enemies/<slug>/<name>.png
     plus a manifest.json. The client draws a chosen frame (e.g. "idle") for that
     enemy; re-running refreshes the art with no code change.

  scripts/make_enemies.py SHEET SLUG --rows 2 --cols 4
  scripts/make_enemies.py drone.png drone --rows 2 --cols 4 \\
    --names idle,alert,scan,hover,tilt,attack,lean,destroyed
"""
import argparse
import json
import os
from collections import Counter

from PIL import Image, ImageDraw

DEFAULT_OUT = "src/web/public/sprites/crawler/enemies"


def _learn_bg(rgb: Image.Image) -> list:
    """Dominant border colours, deduplicated (one = flat bg, several = pattern)."""
    w, h = rgb.size
    px = rgb.load()
    c: Counter = Counter()
    for x in range(0, w, 2):
        for y in (0, 1, h - 2, h - 1):
            c[px[x, y]] += 1
    for y in range(0, h, 2):
        for x in (0, 1, w - 2, w - 1):
            c[px[x, y]] += 1
    shades = []
    for col, _ in c.most_common():
        if all(sum((a - b) ** 2 for a, b in zip(col, s)) > 14 * 14 for s in shades):
            shades.append(col)
        if len(shades) >= 4:
            break
    return shades[:3]


def strip_background(im: Image.Image, thresh: int = 42) -> Image.Image:
    """Make the flat background transparent, flooding from the corners so interior
    same-coloured pixels (a white highlight) survive; a patterned bg is keyed out."""
    im = im.convert("RGBA")
    rgb = im.convert("RGB")
    shades = _learn_bg(rgb)
    if len(shades) <= 1:
        sentinel = (255, 0, 255)
        w, h = rgb.size
        for seed in [(0, 0), (w - 1, 0), (0, h - 1), (w - 1, h - 1)]:
            ImageDraw.floodfill(rgb, seed, sentinel, thresh=thresh)
        src, dst = rgb.load(), im.load()
        for y in range(h):
            for x in range(w):
                if src[x, y] == sentinel:
                    r, g, b, _ = dst[x, y]
                    dst[x, y] = (r, g, b, 0)
    else:
        px = im.load()
        w, h = im.size
        t2 = thresh * thresh
        for y in range(h):
            for x in range(w):
                r, g, b, _ = px[x, y]
                if min((r - s[0]) ** 2 + (g - s[1]) ** 2 + (b - s[2]) ** 2 for s in shades) <= t2:
                    px[x, y] = (r, g, b, 0)
    return im


def autocrop_square(im: Image.Image, size: int, pad_frac: float = 0.06) -> Image.Image:
    """Crop to the opaque content, centre on a square canvas, scale to `size`."""
    bbox = im.getbbox()
    if not bbox:
        return im.resize((size, size), Image.LANCZOS)
    im = im.crop(bbox)
    w, h = im.size
    side = max(w, h)
    pad = int(side * pad_frac)
    canvas = Image.new("RGBA", (side + 2 * pad, side + 2 * pad), (0, 0, 0, 0))
    canvas.paste(im, ((canvas.width - w) // 2, (canvas.height - h) // 2), im)
    return canvas.resize((size, size), Image.LANCZOS)


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("sheet", help="the enemy sprite sheet image")
    ap.add_argument("slug", help="the enemy slug (folder name), e.g. drone")
    ap.add_argument("--rows", type=int, required=True)
    ap.add_argument("--cols", type=int, required=True)
    ap.add_argument("--size", type=int, default=128, help="square px per frame (default 128)")
    ap.add_argument("--names", default="", help="comma pose names in reading order; default frame-<i>")
    ap.add_argument("--no-whitebg", action="store_true", help="keep the background (it already has alpha)")
    ap.add_argument("--out", default=DEFAULT_OUT)
    args = ap.parse_args()

    img = Image.open(args.sheet).convert("RGBA")
    w, h = img.size
    cw, ch = w / args.cols, h / args.rows
    names = [s.strip() for s in args.names.split(",")] if args.names else []
    outdir = os.path.join(args.out, args.slug)
    os.makedirs(outdir, exist_ok=True)

    manifest = []
    i = 0
    for r in range(args.rows):
        for c in range(args.cols):
            cell = img.crop((round(c * cw), round(r * ch), round((c + 1) * cw), round((r + 1) * ch)))
            if not args.no_whitebg:
                cell = strip_background(cell)
            cell = autocrop_square(cell, args.size)
            name = names[i] if i < len(names) else f"frame-{i}"
            cell.save(os.path.join(outdir, f"{name}.png"))
            manifest.append({"i": i, "name": name, "file": f"{name}.png"})
            i += 1

    with open(os.path.join(outdir, "manifest.json"), "w") as f:
        json.dump({"slug": args.slug, "size": args.size, "frames": manifest}, f, indent=2)
    print(f"wrote {i} frames to {outdir}")


if __name__ == "__main__":
    main()
