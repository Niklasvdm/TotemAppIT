import { describe, it, expect } from "vitest";
import { type Edges, rotateEdges, tileArt, poseFromDelta, effectiveArmour, TILE_ART, pickVariant } from "./logic";

describe("pickVariant", () => {
  it("returns the only file for a single-string kind, whatever the id", () => {
    expect(pickVariant("tile-4", 0)).toBe("tile-4");
    expect(pickVariant("tile-4", 7)).toBe("tile-4");
  });
  it("cycles a variant list by tile id, stably", () => {
    const v = ["tile-2", "tile-19", "tile-20"];
    expect(pickVariant(v, 0)).toBe("tile-2");
    expect(pickVariant(v, 1)).toBe("tile-19");
    expect(pickVariant(v, 2)).toBe("tile-20");
    expect(pickVariant(v, 3)).toBe("tile-2"); // wraps
    expect(pickVariant(v, 5)).toBe("tile-20");
  });
  it("handles negative ids without crashing", () => {
    expect(pickVariant(["a", "b"], -1)).toBe("b");
  });
});

describe("rotateEdges", () => {
  it("rotates N-only one quarter-turn to E", () => {
    expect(rotateEdges([true, false, false, false], 1)).toEqual([false, true, false, false]);
  });
  it("is identity at r=0 and r=4", () => {
    const e: Edges = [true, false, true, false];
    expect(rotateEdges(e, 0)).toEqual(e);
    expect(rotateEdges(e, 4)).toEqual(e);
  });
});

describe("tileArt", () => {
  it("returns null for an unknown kind", () => {
    expect(tileArt("nope", [true, true, true, true])).toBeNull();
  });
  it("picks 0deg when the art base already matches the edges", () => {
    // cross base is [T,T,T,T]; symmetric, so the stored rot is honoured.
    expect(tileArt("cross", [true, true, true, true], 0)).toEqual({ file: "tile-4", deg: 0 });
    expect(tileArt("cross", [true, true, true, true], 2)).toEqual({ file: "tile-4", deg: 180 });
  });
  it("rotates the art to line up with the tile's real open edges", () => {
    // straight art base is [T,F,T,F]; edges rotated a quarter-turn -> [F,T,F,T] -> 90deg.
    expect(tileArt("straight", [false, true, false, true])).toEqual({ file: "tile-1", deg: 90 });
  });
  it("renders the healing spring at the rotation matching its laid edges", () => {
    // healing is a T-junction; a tile laid 180deg from the drawn base renders tile-5 at 180.
    const edges = rotateEdges(TILE_ART.healing.base, 2);
    expect(tileArt("healing", edges)?.deg).toBe(180);
  });
});

describe("poseFromDelta", () => {
  it("moves right -> side facing right (no flip)", () => {
    expect(poseFromDelta(1, 0, false)).toEqual({ pose: "side", flip: false });
  });
  it("moves left -> side facing left (flip)", () => {
    expect(poseFromDelta(-1, 0, false)).toEqual({ pose: "side", flip: true });
  });
  it("moves up -> back, down -> front", () => {
    expect(poseFromDelta(0, -1, true).pose).toBe("back");
    expect(poseFromDelta(0, 1, true).pose).toBe("front");
  });
  it("keeps the previous facing on a purely vertical move", () => {
    expect(poseFromDelta(0, -2, true).flip).toBe(true);
  });
});

describe("effectiveArmour", () => {
  it("is the raw armour without Barbed Shot", () => {
    expect(effectiveArmour(4, false)).toBe(4);
  });
  it("lowers armour by 1 with Barbed Shot, floored at 1", () => {
    expect(effectiveArmour(4, true)).toBe(3);
    expect(effectiveArmour(1, true)).toBe(1);
  });
});
