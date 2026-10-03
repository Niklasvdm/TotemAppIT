import type { Arena, RosterEntry, Snapshot } from "../types";
import { POWER_ICONS, SLOT_COLORS } from "../types";
import { loadSprites, pickFrame } from "./sprites";

// How large a totem sprite is drawn, in tiles (the frame is square; the drawn
// character is ~90% of it after the atlas padding). Tune here if sprites feel
// too big or small on the board.
const SPRITE_TILES = 1.2;

export interface Palette {
  floorA: string;
  floorB: string;
  wall: string;
  wallFace: string;
  crate: string;
  crateLine: string;
  ink: string;
  surface: string;
}

// readPalette pulls the arena's colours from the site's CSS custom properties,
// so the board follows the light/dark theme instead of fighting it. Called on
// mount, resize and theme change — never per frame.
export function readPalette(): Palette {
  const s = getComputedStyle(document.documentElement);
  const v = (name: string, fallback: string) => s.getPropertyValue(name).trim() || fallback;
  return {
    floorA: v("--green-soft", "#ddede2"),
    floorB: v("--surface-2", "#f6eedc"),
    wall: v("--green-deep", "#1f4d39"),
    wallFace: v("--green", "#2e6b4f"),
    crate: v("--gold", "#e8b54b"),
    crateLine: v("--gold-soft", "#fbebc4"),
    ink: v("--ink", "#2a2320"),
    surface: v("--surface", "#ffffff"),
  };
}

function roundRect(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  w: number,
  h: number,
  r: number,
) {
  const rad = Math.min(r, w / 2, h / 2);
  ctx.beginPath();
  ctx.moveTo(x + rad, y);
  ctx.arcTo(x + w, y, x + w, y + h, rad);
  ctx.arcTo(x + w, y + h, x, y + h, rad);
  ctx.arcTo(x, y + h, x, y, rad);
  ctx.arcTo(x, y, x + w, y, rad);
  ctx.closePath();
}

export interface DrawArgs {
  ctx: CanvasRenderingContext2D;
  arena: Arena;
  crates: string;
  snap: Snapshot;
  roster: RosterEntry[];
  emoji: Record<string, string>;
  you: number;
  pal: Palette;
  tile: number;
  now: number;
  fuseTicks: number; // full fuse length, so the countdown is tick-rate agnostic
}

export function draw(a: DrawArgs) {
  const { ctx, arena, crates, snap, tile } = a;

  loadSprites(); // one-time; until it's ready, players draw as emoji discs

  ctx.clearRect(0, 0, arena.w * tile, arena.h * tile);
  drawFloor(a);

  for (let y = 0; y < arena.h; y++) {
    for (let x = 0; x < arena.w; x++) {
      const i = y * arena.w + x;
      if (arena.solid[i] === "#") drawWall(a, x, y);
      else if (crates[i] === "x") drawCrate(a, x, y);
    }
  }

  for (const u of snap.u ?? []) drawPower(a, u.x, u.y, POWER_ICONS[u.k]);
  for (const b of snap.b ?? []) drawBomb(a, b.x, b.y, b.f);
  for (const f of snap.f ?? []) drawFlame(a, f.x, f.y);

  // Dead players first, so the living are never hidden behind a ghost.
  const players = [...(snap.p ?? [])].sort((p, q) => Number(p.a) - Number(q.a));
  for (const p of players) drawPlayer(a, p.s, p.x, p.y, p.a);
}

function drawFloor({ ctx, arena, pal, tile }: DrawArgs) {
  for (let y = 0; y < arena.h; y++) {
    for (let x = 0; x < arena.w; x++) {
      ctx.fillStyle = (x + y) % 2 === 0 ? pal.floorA : pal.floorB;
      ctx.fillRect(x * tile, y * tile, tile, tile);
    }
  }
}

function drawWall({ ctx, pal, tile }: DrawArgs, x: number, y: number) {
  const px = x * tile;
  const py = y * tile;
  ctx.fillStyle = pal.wall;
  roundRect(ctx, px + 1, py + 1, tile - 2, tile - 2, tile * 0.18);
  ctx.fill();
  // A lighter top face reads as height without needing real 3D.
  ctx.fillStyle = pal.wallFace;
  roundRect(ctx, px + 1, py + 1, tile - 2, (tile - 2) * 0.62, tile * 0.18);
  ctx.fill();
}

function drawCrate({ ctx, pal, tile }: DrawArgs, x: number, y: number) {
  const px = x * tile + tile * 0.07;
  const py = y * tile + tile * 0.07;
  const s = tile * 0.86;

  ctx.fillStyle = pal.crate;
  roundRect(ctx, px, py, s, s, tile * 0.14);
  ctx.fill();

  ctx.strokeStyle = pal.crateLine;
  ctx.lineWidth = Math.max(1, tile * 0.055);
  ctx.beginPath();
  ctx.moveTo(px, py + s / 2);
  ctx.lineTo(px + s, py + s / 2);
  ctx.moveTo(px + s / 2, py);
  ctx.lineTo(px + s / 2, py + s);
  ctx.stroke();
}

function drawPower({ ctx, tile }: DrawArgs, x: number, y: number, icon: string) {
  const cx = x * tile + tile / 2;
  const cy = y * tile + tile / 2;

  ctx.save();
  ctx.fillStyle = "rgba(255,255,255,.92)";
  ctx.beginPath();
  ctx.arc(cx, cy, tile * 0.33, 0, Math.PI * 2);
  ctx.fill();
  ctx.strokeStyle = "rgba(0,0,0,.18)";
  ctx.lineWidth = Math.max(1, tile * 0.04);
  ctx.stroke();
  ctx.restore();

  glyph(ctx, icon, cx, cy, tile * 0.42);
}

function drawBomb({ ctx, tile, now, fuseTicks }: DrawArgs, x: number, y: number, fuse: number) {
  const cx = x * tile + tile / 2;
  const cy = y * tile + tile / 2;

  // The closer the fuse is to zero, the harder the bomb pulses.
  const urgency = Math.max(0, 1 - fuse / fuseTicks);
  const pulse = 1 + 0.1 * urgency * Math.sin(now / (90 - 60 * urgency));
  const r = tile * 0.3 * pulse;

  ctx.fillStyle = "rgba(0,0,0,.22)";
  ctx.beginPath();
  ctx.ellipse(cx, cy + r * 0.72, r * 0.9, r * 0.35, 0, 0, Math.PI * 2);
  ctx.fill();

  ctx.fillStyle = "#241f19";
  ctx.beginPath();
  ctx.arc(cx, cy, r, 0, Math.PI * 2);
  ctx.fill();

  // Highlight and a fuse spark that brightens as it burns down.
  ctx.fillStyle = "rgba(255,255,255,.35)";
  ctx.beginPath();
  ctx.arc(cx - r * 0.33, cy - r * 0.36, r * 0.24, 0, Math.PI * 2);
  ctx.fill();

  ctx.fillStyle = urgency > 0.65 ? "#ffe6a3" : "#e8b54b";
  ctx.beginPath();
  ctx.arc(cx + r * 0.42, cy - r * 0.86, r * 0.22 * (1 + urgency), 0, Math.PI * 2);
  ctx.fill();
}

function drawFlame({ ctx, tile, now }: DrawArgs, x: number, y: number) {
  const px = x * tile;
  const py = y * tile;
  const cx = px + tile / 2;
  const cy = py + tile / 2;
  const flicker = 0.9 + 0.1 * Math.sin(now / 55 + x * 1.7 + y * 2.3);

  const g = ctx.createRadialGradient(cx, cy, tile * 0.05, cx, cy, tile * 0.6 * flicker);
  g.addColorStop(0, "rgba(255,246,214,.98)");
  g.addColorStop(0.45, "rgba(232,165,59,.95)");
  g.addColorStop(1, "rgba(209,73,91,.12)");

  ctx.fillStyle = g;
  roundRect(ctx, px + 1, py + 1, tile - 2, tile - 2, tile * 0.22);
  ctx.fill();
}

function drawPlayer(a: DrawArgs, slot: number, x: number, y: number, alive: boolean) {
  const { ctx, roster, emoji, you, tile, now } = a;
  const cx = x * tile;
  const cy = y * tile;
  const color = SLOT_COLORS[slot % SLOT_COLORS.length];
  const entry = roster.find((r) => r.s === slot);

  // A sprite only if the atlas is loaded and knows this animal; otherwise null,
  // and we draw the emoji disc exactly as before.
  const blit = entry ? pickFrame(slot, entry.m, x, y, alive, now) : null;

  ctx.save();
  // A ghost stays faint but readable; the sprite's own despair is the defeated
  // frame, so it need not fade as hard as the emoji 💀 did.
  if (!alive) ctx.globalAlpha = blit ? 0.5 : 0.3;

  ctx.fillStyle = "rgba(0,0,0,.2)";
  ctx.beginPath();
  ctx.ellipse(cx, cy + tile * 0.32, tile * 0.27, tile * 0.1, 0, 0, Math.PI * 2);
  ctx.fill();

  if (blit) {
    // No slot-coloured disc behind a sprite — it would show through the art's
    // transparency. A coloured ground ring carries the same "who is who", and
    // the local player's ring is white like their disc outline used to be.
    ctx.strokeStyle = slot === you ? "#fff" : color;
    ctx.lineWidth = Math.max(2, tile * (slot === you ? 0.1 : 0.07));
    ctx.beginPath();
    ctx.ellipse(cx, cy + tile * 0.3, tile * 0.26, tile * 0.095, 0, 0, Math.PI * 2);
    ctx.stroke();

    // A player occupies about one tile; the frame (content ~85% of it after the
    // atlas padding) is drawn roughly tile-sized so the character reads without
    // dwarfing the board. Feet rest at the shadow. Smoothing ON: cartoon art,
    // not pixel art.
    const span = tile * SPRITE_TILES;
    const top = cy + tile * 0.33 - span; // bottom of the frame sits at the feet
    ctx.imageSmoothingEnabled = true;
    if (blit.flip) {
      ctx.save();
      ctx.translate(cx, 0);
      ctx.scale(-1, 1); // mirror around cx; the art faces right by default
      ctx.drawImage(blit.img, blit.sx, blit.sy, blit.sw, blit.sh, -span / 2, top, span, span);
      ctx.restore();
    } else {
      ctx.drawImage(blit.img, blit.sx, blit.sy, blit.sw, blit.sh, cx - span / 2, top, span, span);
    }
  } else {
    const icon = (entry && emoji[entry.m]) || "🐾";

    // Slot-coloured disc: the animal glyph alone isn't enough to tell four
    // players apart at a glance.
    ctx.fillStyle = color;
    ctx.beginPath();
    ctx.arc(cx, cy, tile * 0.33, 0, Math.PI * 2);
    ctx.fill();

    ctx.strokeStyle = slot === you ? "#fff" : "rgba(0,0,0,.25)";
    ctx.lineWidth = Math.max(1.5, tile * (slot === you ? 0.08 : 0.04));
    ctx.stroke();

    glyph(ctx, alive ? icon : "💀", cx, cy, tile * 0.42);
  }

  if (entry) {
    // A sprite is taller than a disc, so its name clears the head.
    const labelY = blit ? cy - tile * 0.98 : cy - tile * 0.38;
    ctx.font = `800 ${Math.max(8, tile * 0.24)}px Nunito, sans-serif`;
    ctx.textAlign = "center";
    ctx.textBaseline = "bottom";
    ctx.lineWidth = Math.max(2, tile * 0.09);
    ctx.strokeStyle = "rgba(0,0,0,.55)";
    ctx.strokeText(entry.n, cx, labelY);
    ctx.fillStyle = "#fff";
    ctx.fillText(entry.n, cx, labelY);
  }

  ctx.restore();
}

function glyph(
  ctx: CanvasRenderingContext2D,
  text: string,
  cx: number,
  cy: number,
  size: number,
) {
  ctx.font = `${size}px "Apple Color Emoji","Segoe UI Emoji","Noto Color Emoji",serif`;
  ctx.textAlign = "center";
  ctx.textBaseline = "middle";
  ctx.fillText(text, cx, cy + size * 0.04);
}
