#!/usr/bin/env python3
"""Split an object/loot icon sheet into individual Totem Crawler object icons.

The crawler shows an emoji per object by default; drop in real icon art by splitting a
grid sheet of icons into named PNGs under src/web/public/sprites/crawler/objects/. This
is the object counterpart of make_enemies.py / make_tiles.py.

What it does:
  1. splits the sheet into rows x cols equal cells (the icons sit on a clean grid);
  2. strips a flat (near-white) background to transparency, like make_enemies.py;
  3. trims each icon to its content and centres it on a square canvas;
  4. writes each to <out>/<slug>.png in reading order, from --names.

  scripts/make_objects.py SHEET --rows 4 --cols 4 --names spare-die,whetstone,...

The slug order must match the sheet's reading order (left to right, top to bottom). The
client (SkyjoPage/CrawlerPage OBJECTS table) maps each object slug to this file, falling
back to the emoji when the file is absent.
"""
import argparse
import os

from PIL import Image

DEFAULT_OUT = "src/web/public/sprites/crawler/objects"


def strip_bg(img: Image.Image) -> Image.Image:
    """Make near-white pixels transparent (the sheets are on a flat white ground)."""
    img = img.convert("RGBA")
    px = img.load()
    w, h = img.size
    for y in range(h):
        for x in range(w):
            r, g, b, a = px[x, y]
            if r > 238 and g > 238 and b > 238:
                px[x, y] = (r, g, b, 0)
    return img


def trim_center(cell: Image.Image, size: int) -> Image.Image:
    """Crop to the opaque content and centre it on a transparent square."""
    bbox = cell.getbbox()
    if bbox:
        cell = cell.crop(bbox)
    scale = min(size / cell.width, size / cell.height, 1.0) * 0.92
    cell = cell.resize((max(1, int(cell.width * scale)), max(1, int(cell.height * scale))), Image.LANCZOS)
    canvas = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    canvas.paste(cell, ((size - cell.width) // 2, (size - cell.height) // 2), cell)
    return canvas


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("sheet", help="the grid icon sheet image")
    ap.add_argument("--rows", type=int, required=True)
    ap.add_argument("--cols", type=int, required=True)
    ap.add_argument("--names", required=True, help="comma-separated object slugs in reading order")
    ap.add_argument("--out", default=DEFAULT_OUT)
    ap.add_argument("--size", type=int, default=128, help="output square px per icon")
    args = ap.parse_args()

    names = [n.strip() for n in args.names.split(",") if n.strip()]
    img = strip_bg(Image.open(args.sheet))
    w, h = img.size
    cw, ch = w / args.cols, h / args.rows
    os.makedirs(args.out, exist_ok=True)

    i = 0
    written = []
    for r in range(args.rows):
        for c in range(args.cols):
            if i >= len(names):
                break
            cell = img.crop((round(c * cw), round(r * ch), round((c + 1) * cw), round((r + 1) * ch)))
            icon = trim_center(cell, args.size)
            icon.save(os.path.join(args.out, names[i] + ".png"))
            written.append(names[i])
            i += 1

    print(f"wrote {len(written)} object icons to {args.out}: {', '.join(written)}")


if __name__ == "__main__":
    main()
