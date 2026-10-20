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
  cleared: boolean; // boss down: the post-boss victory lap (End turn / End level)
  postBoss: number; // post-boss turns the current player has left (0 = none / not cleared)
  level: number; // 1..6
  levelTiles: number; // total tiles in this level's stack (for the start-of-level title card)
  bossName: string; // this level's boss display name (for the title card)
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

// CrCombat drives the compact-view tap-to-attack: tapping a monster selects it (`sel` is its
// "tileId-monsterId" key), and the ⚔ button it shows calls onAttack; canHit gates the sword.
// CrCombat drives the on-board attack ⚔: clicking a reachable enemy SELECTS it (`sel` is its
// "tileId-monsterId" key) and the ⚔ appears over that one; clicking the ⚔ attacks it (canHit
// gates it). It lives on the board (not the overlay), so it works whether or not the overlay
// is up. `hl` highlights a sprite from elsewhere (hovering the sidebar's Attack bar).
export type CrCombat = {
  reachable: Set<string>; // "tileId-monsterId" keys the active hero may attack (own tile / ranged-reachable)
  sel: string | null; // the selected enemy's key (⚔ shown on it)
  onSelect: (key: string) => void;
  onAttack: (tileId: number, m: CrMonster) => void;
  canHit: (m: CrMonster) => boolean;
};

// CrAllyAct drives tapping an ally pawn: `act(h)` returns what a tap does (revive a downed ally,
// Brace/Mend them, or begin a Carry), or null when nothing applies; `onTap` performs it. While
// carrying, `carrying` is that ally's seat (highlighted, its sprite shown by the dice).
export type CrAllyAct = {
  act: (h: CrHero) => "revive" | "brace" | "mend" | "carry" | null;
  onTap: (h: CrHero) => void;
  carrying: number | null;
};

// CrSnatch drives the Raccoon's Snatch: ground loot on a tile orthogonally adjacent to the hero
// glows and, on a click, is pulled in with pilfer in that direction. `hx,hy` is the hero's tile.
export type CrSnatch = {
  hx: number;
  hy: number;
  onSnatch: (dir: number) => void;
};

// LayState is what CrawlerMap needs to render the tile-lay drop zones.
export type LayState = {
  kind: string;
  rot: number; // the orientation the player has chosen (0..3)
  options: CrLayOption[];
  place: (x: number, y: number) => void;
  rotate: (delta: number) => void; // scroll-to-rotate over a drop cell (+1 / -1)
};
