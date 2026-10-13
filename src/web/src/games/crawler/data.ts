// Lookup tables and pure helpers for the Totem Crawler client: the totem roster, the
// sprite-atlas layout, object/status/hazard/tile metadata, and the small derived-value
// functions the components share. No JSX and no React, so this is the common data layer.
import type { CrDie, CrHero, CrMonster, CrView } from "./types";

// A roster of totems to play as. meta = the slug sent on join. Every slug here has
// sprites in the Bomberman atlas (see SPRITE_ROWS).
export const TOTEMS = [
  { slug: "bever", label: "Beaver 🦫 (Guardian · 4 Low dice, HP7)" },
  { slug: "pijlinktvis", label: "Arrow squid 🦑 (Striker ranged · 3 High dice, HP4)" },
  { slug: "wolf", label: "Wolf 🐺 (Striker melee · 4 High dice, HP5)" },
  { slug: "boomkikker", label: "Treefrog 🐸 (Healer · 5 Low dice, HP6)" },
  { slug: "wasbeer", label: "Raccoon 🦝 (Trickster · 5 Balanced dice, HP5)" },
  { slug: "comorenwever", label: "Comoros weaver 🕷 (Carrier · 5 Low dice, HP5)" },
  { slug: "eekhoorn", label: "Squirrel (generic)" },
  { slug: "ezel", label: "Donkey (generic)" },
  { slug: "hondshaai", label: "Houndshark (generic)" },
  { slug: "spinaap", label: "Spider monkey (generic)" },
  { slug: "goudvink", label: "Bullfinch (generic)" },
  { slug: "wouw", label: "Kite (generic)" },
];
export const DIRS = ["N", "E", "S", "W"];
export const DIR_ARROW = ["↑", "→", "↓", "←"];

// Row order of the totem sprite atlas (public/sprites/totems.{png,json}); the
// front-facing frame of each totem is at column 0 of its row. Reused from the Bomberman
// atlas so heroes are drawn as their real totem, not an emoji.
export const SPRITE_ROWS = [
  "bever", "wasbeer", "pijlinktvis", "comorenwever", "ezel", "eekhoorn", "hondshaai",
  "franjeschildpad", "wouw", "spinaap", "goudvink", "chimpansee", "havik", "koala", "boomkikker",
  "wolf",
];
export const ATLAS_COLS = 11; // front(3)+back(3)+side(4)+defeated(1)
export const ATLAS_ROWS = SPRITE_ROWS.length;
export const WALK_FRAME_MS = 140; // ms per walk-cycle frame (matches Bomberman's walkMs)
export const WALK_STEP_MS = 560; // how long a single server move plays its walk cycle

// Enemies with art (scripts/make_enemies.py). A boss kind without its own art falls back
// to the generic boss sprite; other unknowns to an emoji.
export const ENEMY_ART = new Set(["scrapling", "sentry", "hulk", "jammer", "grappler", "boss", "vespex", "pylon", "hauler", "vultrix", "teslacrane", "widowframe"]);

// Human labels for a boss's signature specials (keys match the server's bs* consts).
export const BOSS_SPECIALS: Record<string, string> = {
  adaptive: "Adaptive Plating (+1 Armour each round until a 6 lands, then resets)",
  summon1: "Summoner I (spawns 1 Scrapling each round)",
  summon2: "Summoner II (spawns 2 Scraplings each round)",
  summon3: "Summoner III (spawns a Sentry and a Scrapling each round)",
  jammer: "Jammer Field (+1 Armour to every other robot)",
  shockwave: "Shockwave (every hero on its tile takes 1 each round)",
  hardcore: "Hardened Core (immune until its weak-point is destroyed)",
  overload: "Overload (Electrifies a random hero each round)",
  split: "Split (breaks off a second core at half health)",
  enrage: "Enrage (+1 Damage each round it survives)",
};
export function specialLabel(s: string): string {
  return BOSS_SPECIALS[s] ?? s;
}

// Objects/loot (mirrors the design note + crawler_objects.go). Each has an icon under
// sprites/crawler/objects/<slug>.png with an emoji fallback; `target` marks consumables
// whose use needs a monster target (Grenade). Permanents are passive (no Use button).
export const OBJECTS: Record<string, { name: string; emoji: string; perm: boolean; effect: string; target?: boolean }> = {
  "spare-die": { name: "Spare Die", emoji: "🎲", perm: true, effect: "adds a High attack die to your pool" },
  whetstone: { name: "Whetstone", emoji: "🔪", perm: true, effect: "your 6s count as two hits" },
  "lucky-charm": { name: "Lucky Charm", emoji: "🍀", perm: true, effect: "once a turn, nudge one die +1" },
  "gamblers-glove": { name: "Gambler's Glove", emoji: "🧤", perm: true, effect: "one free reroll each turn" },
  scope: { name: "Scope", emoji: "🎯", perm: true, effect: "your attacks reach an adjacent tile" },
  plating: { name: "Plating", emoji: "🛡️", perm: true, effect: "start each turn with Shields topped to 2" },
  harness: { name: "Harness", emoji: "🎒", perm: true, effect: "carry an ally when you move" },
  "grounding-boots": { name: "Grounding Boots", emoji: "🥾", perm: true, effect: "immune to shock statuses" },
  medkit: { name: "Medkit", emoji: "🩹", perm: false, effect: "heal 3" },
  grenade: { name: "Grenade", emoji: "💣", perm: false, effect: "auto-kill a Health-1 robot on your tile", target: true },
  "recall-whistle": { name: "Recall Whistle", emoji: "↩️", perm: false, effect: "teleport to a transporter" },
  battery: { name: "Battery", emoji: "🔋", perm: false, effect: "+2 Courage" },
  torch: { name: "Torch", emoji: "🔦", perm: false, effect: "clears the hazards on your tile" },
  "courage-horn": { name: "Courage Horn", emoji: "📯", perm: false, effect: "+1 Courage to every ally on your tile" },
  beacon: { name: "Beacon", emoji: "📍", perm: false, effect: "place a transporter tile next to you" },
  "sap-seed": { name: "Sap Seed", emoji: "🌱", perm: false, effect: "place a healing tile next to you" },
};
export function objInfo(slug: string) {
  return OBJECTS[slug] ?? { name: slug, emoji: "📦", perm: false, effect: "" };
}

// Status metadata: an icon + one-line meaning, keyed by the server's status slug.
export const STATUSES: Record<string, { icon: string; label: string; blurb: string }> = {
  electrified: { icon: "⚡", label: "Electrified", blurb: "Loses 1 health each round until cleared (a heal clears it)." },
  electrocuted: { icon: "🔌", label: "Electrocuted", blurb: "-1 to each die value until cleared." },
  snared: { icon: "🕸️", label: "Snared", blurb: "You may spend at most 2 dice on Move this turn." },
  drained: { icon: "🪫", label: "Drained", blurb: "Can't gain Courage until cleared." },
  rusted: { icon: "🦠", label: "Rusted", blurb: "One fewer die until cleared." },
};

// heroStatuses lists the active status slugs on a hero, in display order.
export function heroStatuses(h: CrHero): string[] {
  const out: string[] = [];
  if (h.electrified) out.push("electrified");
  if (h.electrocuted) out.push("electrocuted");
  if (h.snared) out.push("snared");
  if (h.drained) out.push("drained");
  if (h.rusted) out.push("rusted");
  return out;
}

// Hazard metadata for the board tokens.
export const HAZARDS: Record<string, { icon: string; label: string }> = {
  wiring: { icon: "⚡", label: "Live wiring: 1 damage to cross, Electrocuted if you end here" },
  fire: { icon: "🔥", label: "Fire: 2 damage and Electrified if you end here" },
  coolant: { icon: "💧", label: "Coolant: Snares you when you step in" },
  fog: { icon: "🌫️", label: "Sensor fog: contents hidden until a hero is on or next to the tile" },
  mine: { icon: "💥", label: "Mine: bursts once for 3 damage when stepped on" },
};

// The minimum die value each action needs. Comes from the hero's sheet (sent by the
// server on your own hero); falls back to the default map. Attack has min 1 (any die can
// attack; the hit is decided by the monster's Armour, not the menu).
export function menuMins(me: CrHero): Record<string, number> {
  const m: Record<string, number> = { move: 1, attack: 1, courage: 5, [me.special]: 2 };
  (me.menu ?? []).forEach((e) => { m[e.action] = e.min; });
  return m;
}

// The Courage thresholds that unlock the two specials; the cap; the default object slots.
export const COURAGE_UNLOCKS: [number, number] = [3, 5];
export const COURAGE_CAP = 10;
export const OBJECT_CAP = 3; // object slots per hero (matches crObjectCap on the server)

// Human labels and one-line explanations for the tile shapes (kind on the wire).
export const TILE_KINDS: Record<string, { label: string; blurb: string }> = {
  straight: { label: "Straight", blurb: "A corridor: two doorways on opposite sides." },
  bend: { label: "Bend", blurb: "A corner: two doorways on adjacent sides." },
  tee: { label: "T-junction", blurb: "Three doorways." },
  cross: { label: "Crossroads", blurb: "Open on all four sides." },
  "dead-end": { label: "Dead end", blurb: "A single doorway: caps a branch." },
  boss: { label: "Boss lair", blurb: "The boss waits here. One doorway in." },
  portal: { label: "Portal", blurb: "Stand on the centre to teleport to another portal." },
  healing: { label: "Healing spring", blurb: "Step onto the centre to heal to full. No die needed, and it keeps working on each fresh entry." },
  shop: { label: "Shop", blurb: "The kangaroo's stall: buy objects with Courage. Standing here also clears your statuses." },
  vault: { label: "Vault", blurb: "A loot cache: step onto the centre for one free consumable (once)." },
  shrine: { label: "Shrine", blurb: "A landmark (cosmetic, from Level 2)." },
  gate: { label: "City gate", blurb: "The broken gate from the woodland into the ruins: the Level 2 boss lair and the Level 3 start." },
  descent: { label: "Descent", blurb: "The stairwell down into the machine rooms: the Level 4 boss lair." },
  wiring: { label: "Live wiring", blurb: "Hazard room: the doorway squares are live floor (1 damage to cross, Electrocuted if you end on one)." },
  coolant: { label: "Coolant flood", blurb: "Hazard room: wading into the coolant on a doorway square Snares you." },
  fog: { label: "Sensor fog", blurb: "Hazard room: its robots and loot stay hidden until a hero is on or next to it." },
  rust: { label: "Rust room", blurb: "Ruins hazard room (from Level 3)." },
  trap: { label: "Trap room", blurb: "Hazard room: a mine on a doorway square bursts once for 3 damage." },
  overheat: { label: "Overheat room", blurb: "Machine hazard room: fire deals 2 and can Electrify if you end on it (from Level 5)." },
};
export function tileKind(kind: string) {
  return TILE_KINDS[kind] ?? { label: kind || "Tile", blurb: "" };
}

// What each inherent special does, keyed by its action. Shown in the info hover on the
// character sheet so players know before spending Courage.
export const SPECIAL_INFO: Record<string, { name: string; blurb: string }> = {
  brace: { name: "Brace", blurb: "Spend a die (≥2) for 2 Shields to yourself or an ally on your tile." },
  mend: { name: "Mend", blurb: "Spend a die (≥2) to heal 2 to a hero on your tile." },
  guard: { name: "Guard", blurb: "Spend a die (≥2) for +1 Shield (a placeholder for this totem's full special)." },
};
export function specialInfo(special: string): { name: string; blurb: string } {
  return SPECIAL_INFO[special] ?? { name: special, blurb: "A special action." };
}

// Blurbs for the two unlockable specials, keyed by name (the server sends the names per
// totem).
export const UNLOCK_BLURBS: Record<string, string> = {
  "Thick Hide": "Passive: start every turn with +2 Shields on yourself.",
  "Bulwark": "Passive: when an ally on your tile would take Damage, you may take it instead, reduced by your Shields first.",
  "Barbed Shot": "Active at 3 Courage: your attacks ignore 1 Armour, so a lower die can land.",
  "Deep Volley": "Active at 5 Courage: once a turn, an attacking 6 counts as two hits.",
  "Spores": "Your Mend also clears the target's statuses (works even at full health).",
  "Bloom": "Once a round, your Mend also heals every other hero on your tile 1.",
  "Lucky Paws": "Once a turn, reroll any one die for free.",
  "Big Score": "Hold one more object (4), and your kills drop one extra consumable.",
  "Ferry": "Your Carry can cross a tile edge, moving the ally into the next tile.",
  "Flock": "Your Carry moves the ally two squares (a free extra step).",
  "Pack Tactics": "While a living ally shares your tile, your attacks land an extra hit.",
  "Frenzy": "Each kill on your turn grants a free attack die, so kills can chain.",
  "Resolve": "Passive: start your turn with at least 1 Shield.",
  "Second Wind": "Passive: heal 1 at the start of your turn.",
};
export function unlockInfo(me: CrHero): [{ name: string; blurb: string }, { name: string; blurb: string }] {
  const one = me.unlock0 ?? "First special";
  const two = me.unlock1 ?? "Second special";
  return [
    { name: one, blurb: UNLOCK_BLURBS[one] ?? "A special unlocked with Courage." },
    { name: two, blurb: UNLOCK_BLURBS[two] ?? "A stronger special unlocked with Courage." },
  ];
}

// Pip layout (row,col on a 3x3) for each die value, drawn as dots (no image fetch).
export const DIE_PIPS: Record<number, [number, number][]> = {
  1: [[1, 1]],
  2: [[0, 0], [2, 2]],
  3: [[0, 0], [1, 1], [2, 2]],
  4: [[0, 0], [0, 2], [2, 0], [2, 2]],
  5: [[0, 0], [0, 2], [1, 1], [2, 0], [2, 2]],
  6: [[0, 0], [1, 0], [2, 0], [0, 2], [1, 2], [2, 2]],
};

// Cell deltas for N,E,S,W (used for ranged/adjacent-tile lookups).
export const CR_DELTA: [number, number][] = [[0, -1], [1, 0], [0, 1], [-1, 0]];

// dieIdx recovers the selected die's index (the view's dice array is stable).
export function dieIdx(view: CrView, die: CrDie): number {
  return view.dice.findIndex((d) => d === die);
}

// effArmour is the die value an attack needs against a monster. The server sends each
// monster's EFFECTIVE Armour for the viewing hero (base + boss plating / jammer aura, minus
// the Arrow Squid's Barbed Shot), so prefer it over the raw Armour.
export function effArmour(m: CrMonster): number {
  return m.effArmour ?? m.armour;
}

// attackTargets lists every living monster the hero could attack this turn, ignoring die
// value: the robots on their own tile, plus (for a ranged totem through an open edge) the
// ordinary robots one tile away (the boss can't be sniped). Shared by the per-die action
// menu and the keyboard 'f' shortcut so the two never disagree on what is hittable.
export function attackTargets(view: CrView, me: CrHero): { tileId: number; m: CrMonster; ranged: boolean }[] {
  const myTile = view.tiles.find((x) => x.id === me.tile);
  const out: { tileId: number; m: CrMonster; ranged: boolean }[] = [];
  if (!myTile) return out;
  myTile.monsters.filter((m) => m.alive).forEach((m) => out.push({ tileId: myTile.id, m, ranged: false }));
  if (me.ranged) {
    CR_DELTA.forEach(([dx, dy], d) => {
      if (!myTile.edges[d]) return;
      const nt = view.tiles.find((t) => t.x === myTile.x + dx && t.y === myTile.y + dy);
      nt?.monsters.filter((m) => m.alive && !m.isBoss).forEach((m) => out.push({ tileId: nt.id, m, ranged: true }));
    });
  }
  return out;
}

// useItemMsg returns the message to use the object `slug` from a hero's pack, or null when
// it cannot be used right now (passive permanent, or its precondition is unmet). Shared by
// the character sheet's Use buttons and the keyboard number-key shortcuts, so both resolve a
// use the same way. A targeted consumable (Grenade) auto-aims at the lowest-Health eligible
// robot; Lucky Charm needs a selected die. Callers still gate on it being the hero's turn.
export function useItemMsg(view: CrView, me: CrHero, slug: string, selDie: number | null): object | null {
  if (!me.alive) return null;
  const myTile = view.tiles.find((t) => t.id === me.tile);
  const grenadeTargets = (myTile?.monsters ?? [])
    .filter((m) => m.alive && m.health <= 1 && !m.isBoss)
    .sort((a, b) => a.health - b.health);
  const hasPortal = view.tiles.some((t) => t.kind === "portal");
  switch (slug) {
    case "lucky-charm": return selDie != null ? { t: "lucky", die: selDie } : null;
    case "grenade": return grenadeTargets.length ? { t: "use", obj: "grenade", monster: grenadeTargets[0].id } : null;
    case "medkit": return me.health < me.maxHP || me.electrified ? { t: "use", obj: slug } : null;
    case "battery": return me.courage < COURAGE_CAP ? { t: "use", obj: slug } : null;
    case "recall-whistle": return hasPortal ? { t: "use", obj: slug } : null;
    case "torch": return (myTile?.hazards?.length ?? 0) > 0 ? { t: "use", obj: slug } : null;
    case "courage-horn":
    case "beacon":
    case "sap-seed": return { t: "use", obj: slug };
    default: return null; // passive permanents have no use action
  }
}
