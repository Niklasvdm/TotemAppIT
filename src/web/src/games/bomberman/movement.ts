// A port of movePlayer and its helpers from internal/game/sim.go.
//
// These rules run on BOTH sides: the server decides, the client predicts ahead
// of it. The two must agree step for step, because any disagreement shows up as
// the player's sprite being yanked backwards a few times a second. The order of
// operations matters as much as the constants do — JavaScript numbers are IEEE
// 754 doubles, the same as Go's float64, so performing the same operations in
// the same order gives bit-identical results.
//
// Any change here needs the same change in sim.go, and vice versa.

import type { Arena, BombDTO, Input } from "../types";

// Must match playerRadius and eps in internal/game/state.go and sim.go.
export const PLAYER_RADIUS = 0.34;
const EPS = 1e-6;

export interface Movable {
  x: number;
  y: number;
  speed: number; // tiles per second
}

/** Blocked reports whether a tile stops this player. */
export type Blocked = (tx: number, ty: number) => boolean;

// makeBlocked resolves the arena, the crate layer and the live bombs into a
// single predicate for one player. A bomb only blocks players who are no longer
// standing on it, which the server sends as a slot bitmask — without it, every
// bomb you dropped would appear solid the instant it landed and prediction
// would disagree with the server for as long as you stood there.
export function makeBlocked(
  arena: Arena,
  crates: string,
  bombs: BombDTO[],
  slot: number,
): Blocked {
  const solidBombs = new Set<number>();
  for (const b of bombs) {
    if ((b.s & (1 << slot)) === 0) solidBombs.add(b.y * arena.w + b.x);
  }
  return (tx, ty) => {
    if (tx < 0 || ty < 0 || tx >= arena.w || ty >= arena.h) return true;
    const i = ty * arena.w + tx;
    return arena.solid[i] === "#" || crates[i] === "x" || solidBombs.has(i);
  };
}

function collides(x: number, y: number, blocked: Blocked): boolean {
  const x0 = Math.floor(x - PLAYER_RADIUS);
  const y0 = Math.floor(y - PLAYER_RADIUS);
  const x1 = Math.floor(x + PLAYER_RADIUS);
  const y1 = Math.floor(y + PLAYER_RADIUS);
  for (let ty = y0; ty <= y1; ty++) {
    for (let tx = x0; tx <= x1; tx++) {
      if (blocked(tx, ty)) return true;
    }
  }
  return false;
}

/** slide moves one axis by step, clamping flush to whatever it hits. */
function slide(p: Movable, dir: number, step: number, horizontal: boolean, blocked: Blocked) {
  let nx = p.x;
  let ny = p.y;
  if (horizontal) nx += dir * step;
  else ny += dir * step;

  if (!collides(nx, ny, blocked)) {
    p.x = nx;
    p.y = ny;
    return;
  }
  if (horizontal) {
    p.x =
      dir > 0
        ? Math.floor(nx + PLAYER_RADIUS) - PLAYER_RADIUS - EPS
        : Math.floor(nx - PLAYER_RADIUS) + 1 + PLAYER_RADIUS + EPS;
  } else {
    p.y =
      dir > 0
        ? Math.floor(ny + PLAYER_RADIUS) - PLAYER_RADIUS - EPS
        : Math.floor(ny - PLAYER_RADIUS) + 1 + PLAYER_RADIUS + EPS;
  }
}

/** recentre eases the off-axis coordinate toward the corridor centre line. */
function recentre(p: Movable, step: number, horizontal: boolean, blocked: Blocked) {
  const cur = horizontal ? p.x : p.y;
  const target = Math.floor(cur) + 0.5;
  const delta = target - cur;
  if (Math.abs(delta) < EPS) return;

  let move = Math.min(Math.abs(delta), step);
  if (delta < 0) move = -move;

  const nx = horizontal ? p.x + move : p.x;
  const ny = horizontal ? p.y : p.y + move;
  if (!collides(nx, ny, blocked)) {
    p.x = nx;
    p.y = ny;
  }
}

// stepMovement advances one tick. Mirrors movePlayer in sim.go: axes resolve
// independently so a diagonal into a wall keeps the free component, and pure
// single-axis movement pulls toward the corridor centre so the collision box
// does not snag on the pillar lattice.
export function stepMovement(p: Movable, input: Input, hz: number, blocked: Blocked) {
  if (input.dx === 0 && input.dy === 0) return;
  const step = p.speed / hz;

  if (input.dx !== 0) slide(p, input.dx, step, true, blocked);
  if (input.dy !== 0) slide(p, input.dy, step, false, blocked);
  if (input.dx !== 0 && input.dy === 0) recentre(p, step, false, blocked);
  if (input.dy !== 0 && input.dx === 0) recentre(p, step, true, blocked);
}
