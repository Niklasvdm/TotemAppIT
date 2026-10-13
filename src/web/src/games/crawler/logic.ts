// Pure, UI-free helpers for the Totem Crawler client, split out of CrawlerPage.tsx so
// they can be unit-tested without mounting React: tile-art rotation matching, the
// walk-sprite direction picker, and the effective-Armour rule. No DOM, no imports.

export type Edges = [boolean, boolean, boolean, boolean];
const T = true,
  F = false;

// Each tile kind maps to an art file plus the open edges [N,E,S,W] as the art is DRAWN;
// the client rotates the art to match the orientation a tile was actually laid in. To
// re-map a kind or fix an alignment, change its base (or file) here, nothing else.
export const TILE_ART: Record<string, { file: string; base: Edges }> = {
  straight: { file: "tile-1", base: [T, F, T, F] }, //   vertical corridor
  bend: { file: "tile-2", base: [F, T, T, F] }, //        E+S corner
  tee: { file: "tile-3", base: [T, T, T, F] }, //         N+E+S junction
  cross: { file: "tile-4", base: [T, T, T, T] }, //       4-way clearing
  "dead-end": { file: "tile-0", base: [F, F, T, F] }, //  one doorway
  healing: { file: "tile-5", base: [T, F, T, T] }, //     healing spring: a T-junction (3 doorways), same shape the engine lays it as
  portal: { file: "tile-6", base: [T, T, T, T] }, //      transporter/start: same open edges as the crossroads
  shop: { file: "tile-7", base: [F, T, F, F] }, //        merchant stall, a dead-end nook
  boss: { file: "tile-8", base: [T, T, T, T] }, //        the boss lair
  vault: { file: "tile-9", base: [F, F, T, F] }, //       loot cache, a dead-end nook (L2+)
  // Level-2+ feature tiles (open rooms, [T,T,T,T]). Art slots are reserved; until the art
  // sheets land these fall back to the styled cell (the backgroundImage 404s silently).
  shrine: { file: "tile-10", base: [F, F, T, T] }, //     a landmark (cosmetic, L2+)
  gate: { file: "tile-11", base: [F, F, T, F] }, //       broken city gate: woodland -> ruins
  wiring: { file: "tile-12", base: [F, T, F, T] }, //     live-wiring hazard room (L2+)
  coolant: { file: "tile-13", base: [T, T, T, T] }, //    coolant-flood hazard room (L2+)
  fog: { file: "tile-14", base: [T, T, T, T] }, //        sensor-fog hazard room (L2+)
  rust: { file: "tile-15", base: [T, T, T, T] }, //       rust hazard room (Ruins L3+)
  trap: { file: "tile-16", base: [T, T, T, T] }, //       trap/mine hazard room (Ruins L3+)
  overheat: { file: "tile-17", base: [T, T, T, T] }, //   overheat/fire hazard room (Machine L5+)
};

export function rotateEdges(base: Edges, r: number): Edges {
  return [0, 1, 2, 3].map((d) => base[(d - r + 4) % 4]) as Edges;
}

// tileArt picks the art for a tile and the rotation (0/90/180/270) to draw it at. Among
// the rotations whose edges line up with the tile's actual open edges it prefers the one
// matching the tile's stored `rot` so a symmetric tile (the crossroads) still rotates
// visibly. Returns null when the kind has no art.
export function tileArt(kind: string, edges: Edges, rot = 0): { file: string; deg: number } | null {
  const art = TILE_ART[kind];
  if (!art) return null;
  const valid: number[] = [];
  for (let r = 0; r < 4; r++) {
    if (rotateEdges(art.base, r).every((v, i) => v === edges[i])) valid.push(r);
  }
  const want = ((rot % 4) + 4) % 4;
  const pick = valid.includes(want) ? want : valid.length ? valid[0] : want;
  return { file: art.file, deg: pick * 90 };
}

// Walk poses in the shared totem atlas: each pose's first column and its cycle length.
// The art faces right, so a leftward move mirrors the `side` pose.
export type CrPose = "front" | "back" | "side";
export const POSE_BASE: Record<CrPose, number> = { front: 0, back: 3, side: 6 };
export const POSE_LEN: Record<CrPose, number> = { front: 3, back: 3, side: 4 };

// poseFromDelta picks a walk pose + facing from a move's cell delta: the dominant axis
// decides (left/right -> side, mirrored for left; up -> back, down -> front). A purely
// vertical move keeps the previous facing. Used by the board's walk animation.
export function poseFromDelta(dx: number, dy: number, prevFlip: boolean): { pose: CrPose; flip: boolean } {
  if (Math.abs(dx) >= Math.abs(dy)) {
    let flip = prevFlip;
    if (dx < 0) flip = true;
    else if (dx > 0) flip = false;
    return { pose: "side", flip };
  }
  return { pose: dy < 0 ? "back" : "front", flip: prevFlip };
}

// effectiveArmour is the die value an attack needs to land: the monster's Armour, minus
// 1 while the Arrow Squid's Barbed Shot is active (floor 1). Mirrors the server so the
// client only offers attacks that will be accepted.
export function effectiveArmour(armour: number, barbed: boolean): number {
  return Math.max(1, armour - (barbed ? 1 : 0));
}

// Tile art is organised per biome (sprites/crawler/tiles/<biome>/tile-N.png). Only the
// woodland set exists so far; ruins and machine fall back to it until their sheets land.
// Add a biome here the moment its tiles/<biome>/ folder is populated.
const HAVE_TILES = new Set(["woodland"]);
export function biomeTileDir(biome: string): string {
  const s = (biome || "woodland").toLowerCase();
  return HAVE_TILES.has(s) ? s : "woodland";
}
