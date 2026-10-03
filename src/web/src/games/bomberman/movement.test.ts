import { describe, expect, it } from "vitest";
import type { Arena, BombDTO } from "../types";
import { makeBlocked, PLAYER_RADIUS, stepMovement, type Movable } from "./movement";

// These rules are a port of movePlayer in internal/game/sim.go and MUST stay in
// step with it: the client predicts with them and the server decides with them.
// The asserts below pin the shared invariants (clamp flush, no penetration,
// corridor re-centring, per-tick step size).

function arena(rows: string[]): Arena {
  return { w: rows[0].length, h: rows.length, solid: rows.join("") };
}
const OPEN = arena(["#####", "#...#", "#...#", "#...#", "#####"]);
const dots = (a: Arena) => ".".repeat(a.w * a.h);

describe("makeBlocked", () => {
  const blocked = makeBlocked(OPEN, dots(OPEN), [], 0);

  it("blocks border walls, open tiles pass, out of bounds blocks", () => {
    expect(blocked(0, 0)).toBe(true); // border wall
    expect(blocked(1, 1)).toBe(false); // open interior
    expect(blocked(-1, 1)).toBe(true); // out of bounds
    expect(blocked(5, 1)).toBe(true); // out of bounds (w = 5)
  });

  it("blocks crate tiles", () => {
    const crates = dots(OPEN).split("");
    crates[2 * OPEN.w + 2] = "x"; // crate at (2,2)
    const b = makeBlocked(OPEN, crates.join(""), [], 0);
    expect(b(2, 2)).toBe(true);
    expect(b(1, 2)).toBe(false);
  });

  it("a bomb is solid to players who stepped off it, not the one still on it", () => {
    const bombs: BombDTO[] = [{ x: 1, y: 1, f: 30, s: 1 << 1 }]; // only slot 1 still standing
    expect(makeBlocked(OPEN, dots(OPEN), bombs, 0)(1, 1)).toBe(true); // others: solid
    expect(makeBlocked(OPEN, dots(OPEN), bombs, 1)(1, 1)).toBe(false); // owner on it: pass
  });
});

describe("stepMovement", () => {
  const hz = 60;

  it("no input leaves the player exactly put", () => {
    const p: Movable = { x: 1.5, y: 2.5, speed: 4 };
    stepMovement(p, { dx: 0, dy: 0, bomb: false }, hz, makeBlocked(OPEN, dots(OPEN), [], 0));
    expect(p.x).toBe(1.5);
    expect(p.y).toBe(2.5);
  });

  it("moves exactly speed/hz per tick in open space", () => {
    const p: Movable = { x: 1.5, y: 2.5, speed: 4 };
    stepMovement(p, { dx: 1, dy: 0, bomb: false }, hz, makeBlocked(OPEN, dots(OPEN), [], 0));
    expect(p.x).toBeCloseTo(1.5 + 4 / hz, 9);
    expect(p.y).toBeCloseTo(2.5, 9);
  });

  it("clamps flush against a wall and never penetrates it", () => {
    const a = arena(["#####", "#...#", "#..##", "#...#", "#####"]); // wall at (3,2)
    const b = makeBlocked(a, dots(a), [], 0);
    const p: Movable = { x: 1.5, y: 2.5, speed: 4 };
    for (let i = 0; i < 300; i++) stepMovement(p, { dx: 1, dy: 0, bomb: false }, hz, b);
    expect(p.x).toBeLessThanOrEqual(3 - PLAYER_RADIUS); // box right edge never reaches the wall
    expect(p.x).toBeCloseTo(3 - PLAYER_RADIUS - 1e-6, 6); // sits flush against its face
    expect(p.y).toBeCloseTo(2.5, 6);
  });

  it("re-centres onto the corridor line while moving along one axis", () => {
    const p: Movable = { x: 1.5, y: 2.2, speed: 4 }; // off the centre line in y
    const b = makeBlocked(OPEN, dots(OPEN), [], 0);
    for (let i = 0; i < 100; i++) stepMovement(p, { dx: 1, dy: 0, bomb: false }, hz, b);
    expect(p.y).toBeCloseTo(2.5, 6);
  });

  it("keeps the free axis when running diagonally into a wall", () => {
    const a = arena(["#####", "#...#", "#..##", "#...#", "#####"]); // wall at (3,2)
    const b = makeBlocked(a, dots(a), [], 0);
    const p: Movable = { x: 3 - PLAYER_RADIUS - 1e-6, y: 2.5, speed: 4 }; // already flush to the wall
    stepMovement(p, { dx: 1, dy: 1, bomb: false }, hz, b); // x is blocked, down is free
    expect(p.x).toBeCloseTo(3 - PLAYER_RADIUS - 1e-6, 6); // x stays clamped
    expect(p.y).toBeGreaterThan(2.5); // y still advanced
  });
});
