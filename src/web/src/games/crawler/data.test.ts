import { describe, it, expect } from "vitest";
import { attackTargets, useItemMsg, effArmour } from "./data";
import type { CrHero, CrMonster, CrTile, CrView } from "./types";

// Minimal fixtures: only the fields the helpers read, cast to the full view types.
const mon = (o: Partial<CrMonster>): CrMonster => ({
  id: 0, kind: "scrapling", armour: 3, effArmour: 3, health: 1, maxHP: 1, damage: 1,
  reward: 1, square: 1, engaged: false, alive: true, isBoss: false, ...o,
});
const tile = (o: Partial<CrTile>): CrTile => ({
  id: 0, x: 0, y: 0, kind: "cross", rot: 0, edges: [true, true, true, true], isBoss: false, monsters: [], ...o,
});
const hero = (o: Partial<CrHero>): CrHero => ({
  s: 0, n: "me", totem: "bever", health: 7, maxHP: 7, courage: 0, shields: 0, tile: 0, square: 4,
  unlocked: [false, false], alive: true, special: "guard", objects: [], slots: 3, ...o,
});
const view = (o: Partial<CrView>): CrView => ({
  t: "state", ph: "playing", step: "spend", turn: 0, you: 0, dice: [], heroes: [], tiles: [],
  frontiers: [], pending: null, deckLeft: 0, bands: 3, bossNear: false, bonusMoves: 0, grace: -1,
  level: 1, biome: "Woodland", shopStock: [], bossPool: [], log: [], ...o,
});

describe("effArmour", () => {
  it("prefers the server's effective Armour over the base", () => {
    expect(effArmour(mon({ armour: 5, effArmour: 6 }))).toBe(6);
  });
});

describe("attackTargets", () => {
  it("lists living monsters on the hero's tile, not dead ones", () => {
    const t = tile({ monsters: [mon({ id: 1 }), mon({ id: 2, alive: false })] });
    const v = view({ tiles: [t] });
    const got = attackTargets(v, hero({ tile: 0 }));
    expect(got.map((x) => x.m.id)).toEqual([1]);
    expect(got[0].ranged).toBe(false);
  });

  it("adds adjacent-tile robots (not the boss) for a ranged totem", () => {
    const here = tile({ id: 0, x: 0, y: 0, edges: [true, false, false, false] }); // open N
    const north = tile({ id: 1, x: 0, y: -1, monsters: [mon({ id: 7 }), mon({ id: 8, isBoss: true })] });
    const v = view({ tiles: [here, north] });
    const got = attackTargets(v, hero({ tile: 0, ranged: true }));
    expect(got.map((x) => x.m.id)).toEqual([7]); // ranged neighbour, boss excluded
    expect(got[0].ranged).toBe(true);
  });

  it("does not reach adjacent tiles for a melee totem", () => {
    const here = tile({ id: 0, x: 0, y: 0, edges: [true, false, false, false] });
    const north = tile({ id: 1, x: 0, y: -1, monsters: [mon({ id: 7 })] });
    const v = view({ tiles: [here, north] });
    expect(attackTargets(v, hero({ tile: 0, ranged: false }))).toEqual([]);
  });
});

describe("useItemMsg", () => {
  const v0 = view({ tiles: [tile({ id: 0 })] });
  it("returns null for a passive permanent", () => {
    expect(useItemMsg(v0, hero({ tile: 0 }), "whetstone", null)).toBeNull();
  });
  it("gates the medkit on being hurt", () => {
    expect(useItemMsg(v0, hero({ tile: 0, health: 7, maxHP: 7 }), "medkit", null)).toBeNull();
    expect(useItemMsg(v0, hero({ tile: 0, health: 4, maxHP: 7 }), "medkit", null)).toEqual({ t: "use", obj: "medkit" });
  });
  it("auto-aims the grenade at the lowest-Health eligible robot", () => {
    const t = tile({ id: 0, monsters: [mon({ id: 1, health: 1 }), mon({ id: 2, health: 1 }), mon({ id: 3, health: 2 })] });
    const v = view({ tiles: [t] });
    // Both id 1 and 2 are Health 1 (eligible); id 3 (Health 2) is not a grenade target.
    expect(useItemMsg(v, hero({ tile: 0 }), "grenade", null)).toEqual({ t: "use", obj: "grenade", monster: 1 });
  });
  it("needs a selected die for the Lucky Charm", () => {
    expect(useItemMsg(v0, hero({ tile: 0 }), "lucky-charm", null)).toBeNull();
    expect(useItemMsg(v0, hero({ tile: 0 }), "lucky-charm", 2)).toEqual({ t: "lucky", die: 2 });
  });
});
