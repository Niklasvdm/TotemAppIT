#!/usr/bin/env python3
"""Split one animal's sprite sheet into the Bomberman texture atlas.

A sheet is a single image with the animal drawn in several poses. This script:

  1. (optional, --whitebg) strips a solid OR checkerboard background to
     transparency - handy for the flattened JPEGs image tools often hand back,
     where the "transparent" checkerboard got baked into the pixels;
  2. finds the figures by their transparent gaps - across ROWS first (a 2D grid
     is read top-to-bottom) then COLUMNS within each row, left to right. It cuts
     on the real gaps, not arithmetic Nths, so a tail or paw that overhangs a
     cell can't bleed into its neighbour;
  3. normalises every frame to a square cell at one shared scale, feet on the
     floor, so the character does not change size as it turns;
  4. writes it as a row in the shared atlas, src/web/public/sprites/totems.png
     plus totems.json. Re-running an animal replaces its row; a new animal
     extends the atlas downward. No code change is needed to add an animal.

A pose may hold several frames for a walk cycle: the game cycles them while the
player moves and shows the first while idle.

The standard layout needs no --poses: lay out the same number of walk frames for
front, then back, then side, then a single defeated, and the script infers the
count from the figures it finds (4 -> statics, 7 -> 2-frame walk, 10 -> 3-frame
walk; drop the defeated and it reads front/back/side only: 3, 6, 9).

  scripts/make_sprites.py SHEET SLUG --whitebg

For a non-standard sheet (a subset of directions, or uneven counts like
3 front / 3 back / 4 side / 1 defeated) name every figure yourself in reading
order (top-to-bottom, left-to-right), repeating a pose for its walk frames:

  scripts/make_sprites.py SHEET SLUG \\
    --poses front,front,front,back,back,back,side,side,side,side,defeated --whitebg

Canonical poses: front (toward the camera), back (away), side (FACING RIGHT -
the game mirrors it for leftward movement), defeated. An omitted pose falls back
(back -> front); an omitted animal falls back to the emoji disc in-game.
"""

import argparse
import json
import sys
from collections import Counter
from pathlib import Path

from PIL import Image, ImageDraw

FRAME = 128          # px per cell, square
PAD = 6              # transparent margin inside a cell (small, so art fills it)
ALPHA_MIN = 16       # a pixel counts as "content" above this alpha
POSES = {"front", "back", "side", "defeated"}
UPRIGHT = {"front", "back", "side"}  # poses that share a standing height

REPO = Path(__file__).resolve().parent.parent
ATLAS_PNG = REPO / "src/web/public/sprites/totems.png"
ATLAS_JSON = REPO / "src/web/public/sprites/totems.json"


# --- background removal -----------------------------------------------------

def _learn_bg(rgb: Image.Image) -> list[tuple[int, int, int]]:
    """The dominant colours around the border, deduplicated. One colour means a
    flat background; two or more usually means a checkerboard."""
    w, h = rgb.size
    px = rgb.load()
    c: Counter = Counter()
    for x in range(0, w, 2):
        for y in (0, 1, h - 2, h - 1):
            c[px[x, y]] += 1
    for y in range(0, h, 2):
        for x in (0, 1, w - 2, w - 1):
            c[px[x, y]] += 1
    shades: list[tuple[int, int, int]] = []
    for col, _ in c.most_common():
        if all(sum((a - b) ** 2 for a, b in zip(col, s)) > 14 * 14 for s in shades):
            shades.append(col)
        if len(shades) >= 4:
            break
    return shades[:3]


def strip_background(im: Image.Image, thresh: int = 42) -> Image.Image:
    """Make the background transparent. A flat background is flooded in from the
    corners (so same-coloured pixels *inside* the drawing, like white teeth,
    survive). A patterned background - a transparency checkerboard flattened
    into the image - is keyed out globally by its two shades, which is safe
    because those greys are far from the saturated character colours."""
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


# --- geometry: find the figures as connected blobs --------------------------
#
# Each figure is one connected region of opaque pixels, so a figure stays whole
# whatever its spacing, size or walk pose (legs apart still connect through the
# body). This beats cutting on gaps, which cannot tell an even grid from an
# uneven one, or a between-figure gap from a between-legs gap. Figures are then
# ordered the way you read them: grouped into rows by vertical overlap, rows
# top-to-bottom, figures left-to-right.

DOWNSCALE = 4  # label on a 1/4-size mask: faster, and bridges hairline gaps


def _label_blobs(mask: list[bool], sw: int, sh: int) -> dict[int, list[int]]:
    """Union-find connected components (8-connected) over a boolean mask.
    Returns {root: [x0, y0, x1, y1, area]}."""
    parent = list(range(sw * sh))

    def find(a: int) -> int:
        while parent[a] != a:
            parent[a] = parent[parent[a]]
            a = parent[a]
        return a

    for y in range(sh):
        for x in range(sw):
            if not mask[y * sw + x]:
                continue
            i = y * sw + x
            for dx, dy in ((1, 0), (-1, 1), (0, 1), (1, 1)):  # right + row below
                nx, ny = x + dx, y + dy
                if 0 <= nx < sw and 0 <= ny < sh and mask[ny * sw + nx]:
                    ra, rb = find(i), find(ny * sw + nx)
                    if ra != rb:
                        parent[ra] = rb

    boxes: dict[int, list[int]] = {}
    for y in range(sh):
        for x in range(sw):
            if not mask[y * sw + x]:
                continue
            r = find(y * sw + x)
            b = boxes.get(r)
            if b is None:
                boxes[r] = [x, y, x, y, 1]
            else:
                b[0], b[1] = min(b[0], x), min(b[1], y)
                b[2], b[3] = max(b[2], x), max(b[3], y)
                b[4] += 1
    return boxes


def extract_cells(im: Image.Image) -> list[Image.Image]:
    """Return each figure's crop in reading order (rows top-to-bottom, figures
    left-to-right within a row)."""
    w, h = im.size
    sw, sh = max(1, w // DOWNSCALE), max(1, h // DOWNSCALE)
    small = im.getchannel("A").resize((sw, sh), Image.BOX).load()
    mask = [small[x, y] > 8 for y in range(sh) for x in range(sw)]

    boxes = list(_label_blobs(mask, sw, sh).values())
    if not boxes:
        return []
    # Drop specks: anything much smaller than the biggest blob is JPEG noise or
    # a defringe crumb, not a figure.
    biggest = max(b[4] for b in boxes)
    figs = [b for b in boxes if b[4] >= 0.05 * biggest]

    # Scale boxes back to full resolution.
    full = []
    for x0, y0, x1, y1, _ in figs:
        full.append(
            (
                x0 * DOWNSCALE,
                y0 * DOWNSCALE,
                min((x1 + 1) * DOWNSCALE, w),
                min((y1 + 1) * DOWNSCALE, h),
            )
        )

    # Group into rows by vertical overlap, then order.
    full.sort(key=lambda b: b[1])  # by top edge
    rows: list[list[tuple]] = []
    for box in full:
        for row in rows:
            ry0 = min(b[1] for b in row)
            ry1 = max(b[3] for b in row)
            if box[1] <= ry1 and box[3] >= ry0:  # vertical overlap -> same row
                row.append(box)
                break
        else:
            rows.append([box])
    rows.sort(key=lambda r: min(b[1] for b in r))

    cells = []
    for row in rows:
        for x0, y0, x1, y1 in sorted(row, key=lambda b: b[0]):
            cells.append(im.crop((x0, y0, x1, y1)))
    return cells


def auto_poses(count: int) -> list[str]:
    """Infer the pose list from the figure count, assuming the standard layout
    (front/back/side with equal walk frames, then an optional single
    defeated)."""
    if count >= 4 and (count - 1) % 3 == 0:
        k = (count - 1) // 3
        return ["front"] * k + ["back"] * k + ["side"] * k + ["defeated"]
    if count >= 3 and count % 3 == 0:
        k = count // 3
        return ["front"] * k + ["back"] * k + ["side"] * k
    sys.exit(
        f"error: auto-detected {count} figures, which is not 'front/back/side x k "
        f"(+ defeated)'. Use --poses to name them explicitly, or fix the sheet so "
        f"each pose is clearly separated."
    )


# --- normalise + atlas ------------------------------------------------------

def normalise(seq: list[tuple[str, Image.Image]]) -> list[tuple[str, Image.Image]]:
    """Trim each frame to its content, scale them all by one factor (so the
    character keeps a constant size across poses and walk frames), and drop each
    onto a square cell with its feet on the floor."""
    trimmed = []
    for pose, img in seq:
        bbox = img.getbbox()
        trimmed.append((pose, img.crop(bbox) if bbox else img))

    upright_h = [t.height for p, t in trimmed if p in UPRIGHT]
    max_h = max(upright_h) if upright_h else max(t.height for _, t in trimmed)
    max_w = max(t.width for _, t in trimmed)
    scale = min((FRAME - 2 * PAD) / max_h, (FRAME - 2 * PAD) / max_w)

    cells = []
    for pose, t in trimmed:
        nw, nh = max(1, round(t.width * scale)), max(1, round(t.height * scale))
        r = t.resize((nw, nh), Image.LANCZOS)
        cell = Image.new("RGBA", (FRAME, FRAME), (0, 0, 0, 0))
        cell.paste(r, ((FRAME - nw) // 2, FRAME - PAD - nh), r)  # centred, feet down
        cells.append((pose, cell))
    return cells


def load_atlas() -> tuple[Image.Image | None, dict]:
    if ATLAS_JSON.exists():
        meta = json.loads(ATLAS_JSON.read_text())
        meta.setdefault("rows", [])
        meta.setdefault("frames", {})
        img = Image.open(ATLAS_PNG).convert("RGBA") if ATLAS_PNG.exists() else None
        return img, meta
    return None, {"frameW": FRAME, "frameH": FRAME, "rows": [], "frames": {}}


def atlas_width(frames: dict) -> int:
    right = FRAME
    for slug in frames:
        for pose in frames[slug]:
            for rect in frames[slug][pose]:
                right = max(right, rect["x"] + FRAME)
    return right


def main() -> None:
    ap = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    ap.add_argument("sheet", help="source sprite sheet")
    ap.add_argument("slug", help="animal catalog slug, e.g. bever")
    ap.add_argument(
        "--poses",
        help="override auto-detect: comma-separated poses in reading order "
        "(top-to-bottom, left-to-right), repeating a name for walk frames",
    )
    ap.add_argument(
        "--whitebg",
        action="store_true",
        help="strip a solid or checkerboard background to transparency first",
    )
    args = ap.parse_args()

    sheet = Image.open(args.sheet).convert("RGBA")
    if args.whitebg:
        sheet = strip_background(sheet)

    cells = extract_cells(sheet)
    if not cells:
        sys.exit("error: no figures found. Did the background need --whitebg?")

    if args.poses:
        poses = [p.strip() for p in args.poses.split(",") if p.strip()]
        bad = [p for p in poses if p not in POSES]
        if bad:
            sys.exit(f"error: unknown pose(s) {bad}; known: {sorted(POSES)}")
        if len(poses) != len(cells):
            sys.exit(
                f"error: --poses lists {len(poses)} but {len(cells)} figures were "
                f"found. Fix the list or the sheet's spacing."
            )
    else:
        poses = auto_poses(len(cells))
        print(f"  auto-detected {len(cells)} figures -> {','.join(poses)}")

    placed = normalise(list(zip(poses, cells)))  # [(pose, cell), ...] reading order

    old, meta = load_atlas()
    rows = meta["rows"]
    if args.slug not in rows:
        rows.append(args.slug)
    row = rows.index(args.slug)
    y = row * FRAME

    frames: dict[str, list[dict]] = {}
    placements = []
    for col, (pose, cell) in enumerate(placed):
        frames.setdefault(pose, []).append({"x": col * FRAME, "y": y})
        placements.append((col, cell))

    meta["frames"][args.slug] = frames
    width = atlas_width(meta["frames"])
    height = len(rows) * FRAME

    atlas = Image.new("RGBA", (width, height), (0, 0, 0, 0))
    if old is not None:
        atlas.paste(old, (0, 0))
    atlas.paste((0, 0, 0, 0), (0, y, width, y + FRAME))  # clear this row
    for col, cell in placements:
        atlas.paste(cell, (col * FRAME, y), cell)

    ATLAS_PNG.parent.mkdir(parents=True, exist_ok=True)
    atlas.save(ATLAS_PNG)
    ATLAS_JSON.write_text(json.dumps(meta, indent=2) + "\n")

    summary = ", ".join(f"{p}x{len(f)}" for p, f in frames.items())
    print(f"✓ {args.slug}: {summary}  -> row {row}")
    print(f"  {ATLAS_PNG.relative_to(REPO)}  ({atlas.width}x{atlas.height})")
    print(f"  {ATLAS_JSON.relative_to(REPO)}")


if __name__ == "__main__":
    main()
