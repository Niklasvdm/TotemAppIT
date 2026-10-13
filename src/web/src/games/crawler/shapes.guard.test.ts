// @ts-nocheck — this test reads a file with Node's `fs`, and the app tsconfig (which
// covers src/) has no @types/node, so tsc would flag the builtin. vitest/esbuild runs it
// fine; every other test stays type-checked. (Kept minimal to avoid adding a dep/config.)
import { describe, it, expect } from "vitest";
import { readFileSync } from "node:fs";
import { TILE_ART } from "./logic";

// Drift guard: a tile's shape (which edges open) is defined in TWO runtimes that can't
// share a constant — the Go server (`crSpecialBase` + the base path tile types, which lay
// the tile) and this client (`TILE_ART[kind].base`, which rotates the art PNG). They are
// the same shape up to rotation, so what MUST match is the number of open edges per kind.
// When they disagree the art can't rotate to fit and the tile silently falls back to the
// plain cell (a recurring bug). logic.ts is the source of truth; this test fails the build
// the moment the server's edge-count for any kind drifts from it.
//
// It parses the Go source as text (no Go toolchain needed) to read the server shapes:
// crawler.go holds crSpecialBase + the crKind consts, crawler_deck.go the base path tile
// types (ttStraight, ...). Both are read and concatenated.

const goSrc = ["crawler.go", "crawler_deck.go"]
  .map((f) => readFileSync(new URL(`../../../../internal/game/${f}`, import.meta.url), "utf8"))
  .join("\n");

const countTrue = (s: string) => (s.match(/\btrue\b/g) ?? []).length;

// Base path tile types: ttStraight = crawlerTileType{"straight", [4]bool{true,false,...}}
function serverShapeCounts(src: string): Record<string, number> {
  const counts: Record<string, number> = {};
  for (const m of src.matchAll(/crawlerTileType\{"([a-z-]+)",\s*\[4\]bool\{([^}]*)\}/g)) {
    counts[m[1]] = countTrue(m[2]);
  }
  // Const name -> slug, e.g. crKindShop = "shop"
  const kindSlug: Record<string, string> = {};
  for (const m of src.matchAll(/(crKind\w+)\s*=\s*"([a-z0-9-]+)"/g)) {
    kindSlug[m[1]] = m[2];
  }
  // crSpecialBase entries: crKindShop: {true, false, false, false}
  const block = src.match(/var crSpecialBase = map\[string\]\[4\]bool\{([\s\S]*?)\n\}/);
  if (block) {
    for (const m of block[1].matchAll(/(crKind\w+):\s*\{([^}]*)\}/g)) {
      const slug = kindSlug[m[1]];
      if (slug) counts[slug] = countTrue(m[2]);
    }
  }
  // The boss lair is laid as a crossroads (buildDeck uses ttCross.base).
  if (counts["cross"] !== undefined) counts["boss"] = counts["cross"];
  return counts;
}

describe("tile-shape drift guard (logic.ts vs crawler.go)", () => {
  const server = serverShapeCounts(goSrc);

  it("found server shapes to compare against (parse sanity)", () => {
    // If the Go file moves or the shape tables are renamed, this catches it rather than
    // silently passing with an empty map.
    expect(Object.keys(server).length).toBeGreaterThanOrEqual(10);
    expect(server.cross).toBe(4);
    expect(server.boss).toBe(4);
  });

  it("every TILE_ART kind's open-edge count matches the server's shape", () => {
    const mismatches: string[] = [];
    for (const [kind, art] of Object.entries(TILE_ART)) {
      const clientCount = art.base.filter(Boolean).length;
      const serverCount = server[kind];
      if (serverCount === undefined) {
        mismatches.push(`${kind}: no server shape found (crawler.go)`);
      } else if (serverCount !== clientCount) {
        mismatches.push(`${kind}: logic.ts opens ${clientCount}, server opens ${serverCount}`);
      }
    }
    expect(mismatches, mismatches.join("; ")).toEqual([]);
  });
});
