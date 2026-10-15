// Shared view types for the Totem Crawler client: the shapes the server's `state` frame
// sends (mirrors crawler_view.go), plus two small helper type aliases. Pure types, no
// runtime, so every crawler module can import from here without a dependency cycle.
import type { CrPose } from "./logic";

export interface CrDie {
  value: number;
  spent: boolean;
  usedFor: string;
  bonus?: boolean; // an acquired attack die (Spare Die, Wolf Frenzy): drawn in a distinct colour
}

export interface CrHero {
  s: number;
  n: string;
  totem: string;
  health: number;
  maxHP: number;
  courage: number;
  shields: number;
  tile: number;
  square: number;
  unlocked: [boolean, boolean];
  alive: boolean;
  downed?: boolean; // at 0 Health, out of the fight until an ally revives them
  special: string;
  menu?: { action: string; min: number }[]; // only on your own hero
  unlock0?: string;
  unlock1?: string;
  diceType?: string; // low/balanced/high
  ranged?: boolean;
  objects: string[]; // object slugs held
  slots: number; // object capacity (per totem)
  electrified?: boolean;
  electrocuted?: boolean; // status: -1 to each die value
  snared?: boolean; // status: can't leave your tile
  drained?: boolean; // status: can't gain Courage
  rusted?: boolean; // status: one fewer die
  freeReroll?: boolean; // a free reroll (Glove or Lucky Paws) is available (own hero)
  freeSrc?: string; // emoji for the free-reroll source
  luckyReady?: boolean; // Lucky Charm nudge still available (own hero)
  gone?: boolean;
}

export interface CrMonster {
  id: number;
  kind: string;
  armour: number; // base Armour
  effArmour: number; // Armour your die must meet to hit (base + boss plating / jammer, minus Barbed Shot)
  health: number;
  maxHP: number;
  damage: number;
  reward: number; // Courage granted on defeat
  square: number;
  engaged: boolean;
  alive: boolean;
  isBoss: boolean;
  name?: string; // boss display name
  specials?: string[]; // boss signature specials
}

export interface CrTile {
  id: number;
  x: number;
  y: number;
  kind: string;
  rot: number;
  edges: [boolean, boolean, boolean, boolean];
  isBoss: boolean;
  weakPoint?: boolean; // a live Hardened-Core weak-point hides on this tile (highlighted)
  monsters: CrMonster[];
  loot?: { square: number; slug: string }[]; // objects lying on the floor
  hazards?: { square: number; kind: string }[]; // hazards on squares (square -1 = whole tile)
  fogged?: boolean; // sensor fog hides this tile's contents
}

export interface CrLayOption {
  x: number;
  y: number;
  rotation: number;
  edges: [boolean, boolean, boolean, boolean];
}

export interface CrView {
  t: "state";
  ph: "lobby" | "playing" | "won" | "lost";
  step: "spend" | "lay" | "";
  turn: number;
  you: number;
  dice: CrDie[];
  heroes: CrHero[];
  tiles: CrTile[];
  frontiers: { x: number; y: number }[];
  pending: { kind: string; entryDir: number; options: CrLayOption[]; phaseLay: boolean } | null;
  deckLeft: number;
  bands: number;
  bossNear: boolean;
  bonusMoves: number; // directed steps left in the active hero's multi-move (a fast mover)
  grace: number; // collapse countdown once the stack is empty; -1 while tiles remain
  level: number; // 1..6
  biome: string; // Woodland / Ruins / Machine
  shopStock: { slug: string; name: string; cost: number }[];
  bossPool: { slug: string; name: string; minLvl: number; maxLvl: number }[]; // for the F3 dev start picker
  lootRoll?: { roll: number; chance: number; hit: boolean; name?: string; killer?: string } | null;
  statusRoll?: { roll: number; need: number; status: string; hit: boolean; target?: string } | null;
  log: string[];
  debug?: CrDebug; // only present when the run is in debug mode (the F3 overlay)
}

// CrDebug is the extra payload the F3 debug overlay reads (mirrors crawlerDebugView).
export interface CrDebug {
  seed: number;
  level: number;
  biome: string;
  phase: string;
  step: string;
  turn: number;
  grace: number;
  deckPos: number;
  deckLen: number;
  forcedBoss: string;
  defeated: string[] | null;
  upcoming: { idx: number; kind: string; isBoss: boolean; monsters: string[] | null }[] | null;
  odds: string[];
}

// spriteFor(seat) returns the walk pose/frame/flip for a board pawn (see useHeroSprites).
export type SpriteFor = (seat: number) => { pose: CrPose; frame: number; flip: boolean };

// LayState is what CrawlerMap needs to render the tile-lay drop zones.
export type LayState = {
  kind: string;
  rot: number; // the orientation the player has chosen (0..3)
  options: CrLayOption[];
  place: (x: number, y: number) => void;
  rotate: (delta: number) => void; // scroll-to-rotate over a drop cell (+1 / -1)
};
