import { describe, it, expect } from "vitest";
import { attackTargets, useItemMsg, effArmour, canStep, crWalkable, lowestDie } from "./data";
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
  cleared: false, postBoss: 0,
  level: 1, levelTiles: 0, bossName: "", bossSlug: "", bossTwist: [], biome: "Woodland", shopStock: [], bossPool: [], log: [], ...o,
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

describe("crWalkable", () => {
  it("is the centre plus the doorway of each open edge", () => {
    // Open N(1) and E(5) only: walkable = centre(4) + door N(1) + door E(5).
    expect(crWalkable([true, true, false, false])).toEqual([
      false, true, false, false, true, true, false, false, false,
    ]);
  });
});

describe("lowestDie", () => {
  const dice = [
    { value: 5, spent: false, usedFor: "" },
    { value: 2, spent: false, usedFor: "" },
    { value: 6, spent: true, usedFor: "attack" },
  ];
  it("picks the lowest unspent die meeting the minimum", () => {
    expect(lowestDie(dice, 1)).toBe(1); // the 2
    expect(lowestDie(dice, 5)).toBe(0); // the 2 is too low; the 5 qualifies (6 is spent)
    expect(lowestDie(dice, 7)).toBe(-1); // nothing qualifies
  });
});

describe("canStep", () => {
  // Hero on the centre of an all-open cross at (0,0).
  const base = (o: Partial<CrView> = {}) => view({ tiles: [tile({ id: 0 })], heroes: [hero({ tile: 0, square: 4 })], ...o });
  it("allows a step from centre toward any open doorway", () => {
    const v = base();
    expect([0, 1, 2, 3].map((d) => canStep(v, v.heroes[0], d))).toEqual([true, true, true, true]);
  });
  it("blocks a step toward a closed edge", () => {
    const v = view({ tiles: [tile({ id: 0, edges: [true, false, false, false] })], heroes: [hero({ tile: 0, square: 4 })] });
    expect(canStep(v, v.heroes[0], 0)).toBe(true); // N open
    expect(canStep(v, v.heroes[0], 1)).toBe(false); // E closed -> corner, off-path
  });
  it("blocks stepping onto the shop centre", () => {
    // On the W doorway (3) of a shop tile, stepping E toward the centre (4) is refused.
    const v = view({ tiles: [tile({ id: 0, kind: "shop" })], heroes: [hero({ tile: 0, square: 3 })] });
    expect(canStep(v, v.heroes[0], 1)).toBe(false);
  });
  it("steps off a doorway into a placed neighbour", () => {
    const here = tile({ id: 0, x: 0, y: 0, edges: [true, false, false, false] });
    const north = tile({ id: 1, x: 0, y: -1 });
    const v = view({ tiles: [here, north], heroes: [hero({ tile: 0, square: 1 })] }); // on the N doorway
    expect(canStep(v, v.heroes[0], 0)).toBe(true);
  });
  it("lays at a frontier only while tiles can still be laid", () => {
    const here = tile({ id: 0, x: 0, y: 0, edges: [true, false, false, false] });
    const open = view({ tiles: [here], heroes: [hero({ tile: 0, square: 1 })], frontiers: [{ x: 0, y: -1 }], grace: -1, deckLeft: 5 });
    expect(canStep(open, open.heroes[0], 0)).toBe(true);
    const collapsing = view({ tiles: [here], heroes: [hero({ tile: 0, square: 1 })], frontiers: [{ x: 0, y: -1 }], grace: 3, deckLeft: 5 });
    expect(canStep(collapsing, collapsing.heroes[0], 0)).toBe(false); // collapse clock started: no more laying
  });
  it("locks a hero in the boss lair while the boss lives", () => {
    const lair = tile({ id: 0, x: 0, y: 0, kind: "boss", isBoss: true, edges: [true, false, false, false], monsters: [mon({ id: 9, isBoss: true, square: 4 })] });
    const north = tile({ id: 1, x: 0, y: -1 });
    const v = view({ tiles: [lair, north], heroes: [hero({ tile: 0, square: 1 })] });
    expect(canStep(v, v.heroes[0], 0)).toBe(false);
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
