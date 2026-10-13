import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router-dom";
import { createGameRoom } from "../../api";
import { CODE_LEN, NICK_MAX, validNick } from "../validate";
import { useRoom } from "../useRoom";
import { WrongGameNotice } from "../WrongGameNotice";
import { RoomBar } from "../RoomBar";
import { LocalSeats, type LocalPlayer } from "../LocalSeats";
import { LossFlash, useLossPulse } from "../LossFlash";
import { useStatsFlag } from "../bomberman/NetHud";
import {
  type Edges, type CrPose, tileArt,
  POSE_BASE, POSE_LEN, poseFromDelta, effectiveArmour, biomeTileDir,
} from "./logic";

// A roster of totems to play as. Only the Beaver has a distinct sheet in V0
// (Brace); the rest share the generic sheet. meta = the slug sent on join. Every
// slug here has sprites in the Bomberman atlas (see SPRITE_ROWS).
const TOTEMS = [
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
const DIRS = ["N", "E", "S", "W"];
const DIR_ARROW = ["↑", "→", "↓", "←"];

// Row order of the totem sprite atlas (public/sprites/totems.{png,json}); the
// front-facing frame of each totem is at column 0 of its row. Reused from the
// Bomberman atlas so heroes are drawn as their real totem, not an emoji.
const SPRITE_ROWS = [
  "bever", "wasbeer", "pijlinktvis", "comorenwever", "ezel", "eekhoorn", "hondshaai",
  "franjeschildpad", "wouw", "spinaap", "goudvink", "chimpansee", "havik", "koala", "boomkikker",
  "wolf",
];
const ATLAS_COLS = 11; // front(3)+back(3)+side(4)+defeated(1)
const ATLAS_ROWS = SPRITE_ROWS.length;

// Walk poses, tile-art rotation and the effective-Armour rule live in ./logic (pure,
// unit-tested). The defeated frame is the last atlas column; the pose columns come from
// POSE_BASE/POSE_LEN. (Atlas layout matches Bomberman's totems.json: front 0-2, back
// 3-5, side 6-9, defeated 10.)
const WALK_FRAME_MS = 140; // ms per walk-cycle frame (matches Bomberman's walkMs)
const WALK_STEP_MS = 560; // how long a single server move plays its walk cycle

// TotemSprite draws one totem frame from the shared atlas via CSS background
// positioning: a walk pose + frame (cycled while moving), mirrored for a leftward
// side pose, or the defeated frame when dead. Falls back to an emoji for unsprited
// totems.
function TotemSprite({ slug, size = 40, pose = "front", frame = 0, flip = false, dead = false }: {
  slug: string; size?: number; pose?: CrPose; frame?: number; flip?: boolean; dead?: boolean;
}) {
  const row = SPRITE_ROWS.indexOf(slug);
  if (row < 0) return <span style={{ fontSize: size * 0.8 }}>{dead ? "💀" : "🧭"}</span>;
  const col = dead ? ATLAS_COLS - 1 : POSE_BASE[pose] + (frame % POSE_LEN[pose]);
  return (
    <span
      className="cr-sprite"
      style={{
        width: size,
        height: size,
        backgroundImage: "url(/sprites/totems.png)",
        backgroundSize: `${ATLAS_COLS * size}px ${ATLAS_ROWS * size}px`,
        backgroundPosition: `${-col * size}px ${-row * size}px`,
        transform: !dead && flip ? "scaleX(-1)" : undefined,
      }}
    />
  );
}

interface CrDie {
  value: number;
  spent: boolean;
  usedFor: string;
}
interface CrHero {
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
interface CrMonster {
  id: number;
  kind: string;
  armour: number;
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

// Enemies with art (scripts/make_enemies.py). Each renders its idle frame, or its
// attack frame + a lunge while engaged (the hero is fighting it this turn). A boss kind
// without its own art falls back to the generic boss sprite; other unknowns to an emoji.
const ENEMY_ART = new Set(["scrapling", "sentry", "hulk", "jammer", "grappler", "boss", "vespex", "pylon"]);

// Human labels for a boss's signature specials (keys match the server's bs* consts).
const BOSS_SPECIALS: Record<string, string> = {
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
function specialLabel(s: string): string {
  return BOSS_SPECIALS[s] ?? s;
}

// Objects/loot (mirrors the design note + crawler_objects.go). Each has an icon under
// sprites/crawler/objects/<slug>.png with an emoji fallback; `target` marks consumables
// whose use needs a monster target (Grenade). Permanents are passive (no Use button).
const OBJECTS: Record<string, { name: string; emoji: string; perm: boolean; effect: string; target?: boolean }> = {
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
function objInfo(slug: string) {
  return OBJECTS[slug] ?? { name: slug, emoji: "📦", perm: false, effect: "" };
}

// ObjIcon shows the object's art, falling back to its emoji if the image is missing.
function ObjIcon({ slug, size = 22 }: { slug: string; size?: number }) {
  const [broken, setBroken] = useState(false);
  const info = objInfo(slug);
  if (broken) return <span style={{ fontSize: size * 0.9 }}>{info.emoji}</span>;
  return (
    <img
      className="cr-obj-icon"
      src={`/sprites/crawler/objects/${slug}.png`}
      alt={info.name}
      width={size}
      height={size}
      onError={() => setBroken(true)}
    />
  );
}
function MonsterSprite({ m }: { m: CrMonster }) {
  const slug = ENEMY_ART.has(m.kind) ? m.kind : m.isBoss ? "boss" : "";
  if (!slug) return <>{m.alive ? "👹" : "💀"}</>;
  // A defeated robot stays on its square in its destroyed frame; a living one that is
  // being fought this turn plays its attack frame, otherwise it idles.
  const pose = !m.alive ? "destroyed" : m.engaged ? "attack" : "idle";
  return (
    <span
      className={`cr-enemy${m.isBoss ? " boss" : ""}${m.engaged && m.alive ? " engaged" : ""}${m.alive ? "" : " dead"}`}
      style={{ backgroundImage: `url(/sprites/crawler/enemies/${slug}/${pose}.png)` }}
      aria-label={m.kind}
    />
  );
}
interface CrTile {
  id: number;
  x: number;
  y: number;
  kind: string;
  rot: number;
  edges: [boolean, boolean, boolean, boolean];
  isBoss: boolean;
  used?: boolean; // a looted Vault
  monsters: CrMonster[];
  loot?: { square: number; slug: string }[]; // objects lying on the floor
  hazards?: { square: number; kind: string }[]; // hazards on squares (square -1 = whole tile)
  fogged?: boolean; // sensor fog hides this tile's contents
}
interface CrLayOption {
  x: number;
  y: number;
  rotation: number;
  edges: [boolean, boolean, boolean, boolean];
}
interface CrView {
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
  lootRoll?: { roll: number; chance: number; hit: boolean; name?: string; killer?: string } | null;
  statusRoll?: { roll: number; need: number; status: string; hit: boolean; target?: string } | null;
  log: string[];
}

// Status metadata: an icon + one-line meaning, keyed by the server's status slug.
const STATUSES: Record<string, { icon: string; label: string; blurb: string }> = {
  electrified: { icon: "⚡", label: "Electrified", blurb: "Loses 1 health each round until cleared (a heal clears it)." },
  electrocuted: { icon: "🔌", label: "Electrocuted", blurb: "-1 to each die value until cleared." },
  snared: { icon: "🕸️", label: "Snared", blurb: "Can't leave your tile until cleared." },
  drained: { icon: "🪫", label: "Drained", blurb: "Can't gain Courage until cleared." },
  rusted: { icon: "🦠", label: "Rusted", blurb: "One fewer die until cleared." },
};

// heroStatuses lists the active status slugs on a hero, in display order.
function heroStatuses(h: CrHero): string[] {
  const out: string[] = [];
  if (h.electrified) out.push("electrified");
  if (h.electrocuted) out.push("electrocuted");
  if (h.snared) out.push("snared");
  if (h.drained) out.push("drained");
  if (h.rusted) out.push("rusted");
  return out;
}

// Hazard metadata for the board tokens.
const HAZARDS: Record<string, { icon: string; label: string }> = {
  wiring: { icon: "⚡", label: "Live wiring: 1 damage to cross, Electrocuted if you end here" },
  fire: { icon: "🔥", label: "Fire: 2 damage and Electrified if you end here" },
  coolant: { icon: "💧", label: "Coolant: Snares you when you step in" },
  fog: { icon: "🌫️", label: "Sensor fog: contents hidden until a hero is on or next to the tile" },
  mine: { icon: "💥", label: "Mine: bursts once for 3 damage when stepped on" },
};

// The minimum die value each action needs. Comes from the hero's sheet (sent by the
// server on your own hero); falls back to the default map. Attack has min 1 (any die
// can attack; the hit is decided by the monster's Armour, not the menu).
function menuMins(me: CrHero): Record<string, number> {
  const m: Record<string, number> = { move: 1, attack: 1, courage: 5, [me.special]: 2 };
  (me.menu ?? []).forEach((e) => { m[e.action] = e.min; });
  return m;
}

// The Courage thresholds that unlock the two specials. The bar fills toward the cap
// (10), with the two unlocks marked along it.
const COURAGE_UNLOCKS: [number, number] = [3, 5];
const COURAGE_CAP = 10;
const OBJECT_CAP = 3; // object slots per hero (matches crObjectCap on the server)

// Human labels and one-line explanations for the tile shapes (kind on the wire).
const TILE_KINDS: Record<string, { label: string; blurb: string }> = {
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
  gate: { label: "City gate", blurb: "The broken gate from the woodland into the ruins (cosmetic)." },
  wiring: { label: "Live wiring", blurb: "Hazard room: the doorway squares are live floor (1 damage to cross, Electrocuted if you end on one)." },
  coolant: { label: "Coolant flood", blurb: "Hazard room: wading into the coolant on a doorway square Snares you." },
  fog: { label: "Sensor fog", blurb: "Hazard room: its robots and loot stay hidden until a hero is on or next to it." },
  rust: { label: "Rust room", blurb: "Ruins hazard room (from Level 3)." },
  trap: { label: "Trap room", blurb: "Hazard room: a mine on a doorway square bursts once for 3 damage." },
  overheat: { label: "Overheat room", blurb: "Machine hazard room: fire deals 2 and can Electrify if you end on it (from Level 5)." },
};
function tileKind(kind: string) {
  return TILE_KINDS[kind] ?? { label: kind || "Tile", blurb: "" };
}

// Tile art, split from a sheet by scripts/make_tiles.py into
// public/sprites/crawler/tiles/tile-<i>.png. Each entry gives the file and the
// open edges (N,E,S,W) as the art is DRAWN; the client rotates the art to match
// the orientation a tile was actually laid in (see tileArt). To re-map a kind to
// a different cell, just change its file + base here. The boss (stairs, tile-7)
// and the gate (tile-6) are deliberately left out for now, so a boss tile uses
// the styled fallback cell (red border + "Boss lair" label).
// What each inherent special does, keyed by its action. Shown in the info hover on
// the character sheet so players know before spending Courage.
const SPECIAL_INFO: Record<string, { name: string; blurb: string }> = {
  brace: { name: "Brace", blurb: "Spend a die (≥2) for 2 Shields to yourself or an ally on your tile." },
  mend: { name: "Mend", blurb: "Spend a die (≥2) to heal 2 to a hero on your tile." },
  guard: { name: "Guard", blurb: "Spend a die (≥2) for +1 Shield (a V0 placeholder for this totem's full special)." },
};
function specialInfo(special: string): { name: string; blurb: string } {
  return SPECIAL_INFO[special] ?? { name: special, blurb: "A special action." };
}

// Blurbs for the two unlockable specials, keyed by name (the server sends the names
// per totem). Specials that need unbuilt systems say so.
const UNLOCK_BLURBS: Record<string, string> = {
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
function unlockInfo(me: CrHero): [{ name: string; blurb: string }, { name: string; blurb: string }] {
  const one = me.unlock0 ?? "First special";
  const two = me.unlock1 ?? "Second special";
  return [
    { name: one, blurb: UNLOCK_BLURBS[one] ?? "A special unlocked with Courage." },
    { name: two, blurb: UNLOCK_BLURBS[two] ?? "A stronger special unlocked with Courage." },
  ];
}

// InfoDot is a small ⓘ badge with a hover box explaining an ability.
function InfoDot({ text }: { text: string }) {
  return (
    <span className="cr-info" tabIndex={0}>
      ⓘ<span className="cr-info-box">{text}</span>
    </span>
  );
}

// useHeroSprites drives the Bomberman-style walk animation for the board pawns in a
// turn-based game. The hero positions arrive as discrete server states, not a 60Hz
// stream, so instead of smoothed velocity it diffs each hero's global cell (tile ×3 +
// square) between states: on a change it commits a facing from the move's dominant
// axis (left/right → side, mirrored for left; up → back, down → front) and plays that
// pose's walk cycle for one step (WALK_STEP_MS), then rests on the standing frame and
// holds the pose. A dead hero, or the whole party on a loss, draws the defeated frame
// (the caller passes `dead` to TotemSprite). Returns spriteFor(seat).
type SpriteFor = (seat: number) => { pose: CrPose; frame: number; flip: boolean };

function useHeroSprites(v?: CrView | null): SpriteFor {
  const anim = useRef(new Map<number, { gx: number; gy: number; pose: CrPose; flip: boolean; until: number }>());
  const [, setTick] = useState(0);

  // Diff positions whenever a new server state arrives; start a walk on any mover.
  useEffect(() => {
    if (!v) return;
    const now = performance.now();
    const tilePos = new Map(v.tiles.map((t) => [t.id, [t.x, t.y] as [number, number]]));
    v.heroes.forEach((h) => {
      const tp = tilePos.get(h.tile);
      if (!tp) return;
      const gx = tp[0] * 3 + (h.square % 3);
      const gy = tp[1] * 3 + Math.floor(h.square / 3);
      const prev = anim.current.get(h.s);
      if (!prev) {
        anim.current.set(h.s, { gx, gy, pose: "front", flip: false, until: 0 });
        return;
      }
      if (prev.gx === gx && prev.gy === gy) return;
      const { pose, flip } = poseFromDelta(gx - prev.gx, gy - prev.gy, prev.flip);
      anim.current.set(h.s, { gx, gy, pose, flip, until: now + WALK_STEP_MS });
    });
  }, [v]);

  // Re-render to advance the walk frames only while a hero is mid-step (idle = quiet).
  useEffect(() => {
    const id = window.setInterval(() => {
      const now = performance.now();
      let walking = false;
      anim.current.forEach((s) => {
        if (now < s.until) walking = true;
      });
      if (walking) setTick((t) => t + 1);
    }, 90);
    return () => window.clearInterval(id);
  }, []);

  return (seat: number): { pose: CrPose; frame: number; flip: boolean } => {
    const s = anim.current.get(seat);
    if (!s) return { pose: "front", frame: 0, flip: false };
    const now = performance.now();
    const frame = now < s.until ? Math.floor(now / WALK_FRAME_MS) % POSE_LEN[s.pose] : 0;
    return { pose: s.pose, frame, flip: s.flip };
  };
}

export default function CrawlerPage() {
  const { t } = useTranslation();
  const [params] = useSearchParams();
  const [name, setName] = useState(() => localStorage.getItem("totem-game-name") ?? "");
  const [totem, setTotem] = useState(() => localStorage.getItem("totem-crawler-totem") ?? "bever");
  const [code, setCode] = useState(() => (params.get("code") ?? "").toUpperCase());
  const [session, setSession] = useState<{ code: string; players: LocalPlayer[] } | null>(null);
  const [focus, setFocus] = useState<string | undefined>(undefined); // seat whose turn it is (auto-switch)
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (session) {
    return (
      <main className="game-page">
        <LocalSeats
          players={session.players}
          onAddPlayer={(p) => setSession((s) => (s ? { ...s, players: [...s.players, p] } : s))}
          addMenu={(submit, cancel) => <CrawlerAddForm submit={submit} cancel={cancel} />}
          focusSeatKey={focus}
          renderSeat={(p, seatKey) => (
            <CrawlerRoom
              code={session.code}
              name={p.name}
              totem={p.meta ?? "bever"}
              seatKey={seatKey}
              onMyTurn={() => setFocus(seatKey)}
              onLeave={() => setSession(null)}
            />
          )}
        />
      </main>
    );
  }

  const begin = (roomCode: string) => {
    const nick = name.trim();
    localStorage.setItem("totem-game-name", nick);
    localStorage.setItem("totem-crawler-totem", totem);
    setSession({ code: roomCode, players: [{ name: nick, meta: totem }] });
  };
  const guard = () => {
    if (!validNick(name)) {
      setError(t("gameNeedName"));
      return false;
    }
    setError(null);
    return true;
  };
  const create = async () => {
    if (!guard()) return;
    setBusy(true);
    try {
      begin((await createGameRoom("crawler")).code);
    } catch {
      setError(t("gameNoRoom"));
    } finally {
      setBusy(false);
    }
  };
  const join = () => {
    if (!guard()) return;
    const c = code.trim().toUpperCase();
    if (c.length !== CODE_LEN) {
      setError(t("gameBadCode", { n: CODE_LEN }));
      return;
    }
    begin(c);
  };

  return (
    <main className="game-page">
      <div className="panel game-setup">
        <Link className="game-crumb" to="/games">
          {t("gameAllGames")}
        </Link>
        <h2>🏰 {t("gameCrawler")}</h2>
        <p className="muted-note">{t("gameCrawlerBlurb")}</p>
        <h3>{t("gameYourName")}</h3>
        <input
          className="trait-search"
          value={name}
          maxLength={NICK_MAX}
          placeholder={t("gameNamePlaceholder")}
          onChange={(e) => setName(e.target.value)}
        />
        <h3>Your totem</h3>
        <select className="trait-search" value={totem} onChange={(e) => setTotem(e.target.value)}>
          {TOTEMS.map((x) => (
            <option key={x.slug} value={x.slug}>
              {x.label}
            </option>
          ))}
        </select>
        {error && <p className="game-error">{error}</p>}
        <div className="game-actions">
          <button className="game-btn" disabled={busy} onClick={create}>
            {busy ? t("loading") : t("gameCreate")}
          </button>
          <div className="game-join">
            <input
              className="trait-search game-codeinput"
              value={code}
              maxLength={CODE_LEN}
              placeholder={t("gameCodePlaceholder")}
              onChange={(e) => setCode(e.target.value.toUpperCase())}
              onKeyDown={(e) => e.key === "Enter" && join()}
            />
            <button className="game-btn ghost" onClick={join}>
              {t("gameJoin")}
            </button>
          </div>
        </div>
      </div>
    </main>
  );
}

// CrawlerAddForm is the Crawler's local-player selection menu (name + totem), reusing the
// same TOTEMS picker as the lobby, for adding another player on the shared device.
function CrawlerAddForm({ submit, cancel }: { submit: (p: LocalPlayer) => void; cancel: () => void }) {
  const [name, setName] = useState("");
  const [totem, setTotem] = useState("wolf");
  const go = () => {
    const n = name.trim();
    if (n) submit({ name: n, meta: totem });
  };
  return (
    <div className="local-add-form cr-add-form">
      <input
        className="trait-search"
        autoFocus
        value={name}
        maxLength={NICK_MAX}
        placeholder="Player name"
        onChange={(e) => setName(e.target.value)}
        onKeyDown={(e) => e.key === "Enter" && go()}
      />
      <select className="trait-search" value={totem} onChange={(e) => setTotem(e.target.value)}>
        {TOTEMS.map((x) => (
          <option key={x.slug} value={x.slug}>{x.label}</option>
        ))}
      </select>
      <button className="game-btn" onClick={go}>Add</button>
      <button className="game-btn ghost" onClick={cancel}>Cancel</button>
    </div>
  );
}

function CrawlerRoom({
  code,
  name,
  totem,
  onLeave,
  seatKey = "",
  onMyTurn,
}: {
  code: string;
  name: string;
  totem: string;
  onLeave: () => void;
  seatKey?: string;
  onMyTurn?: () => void;
}) {
  const { t } = useTranslation();
  const g = useRoom<CrView>(code, name, totem, seatKey);
  const debug = useStatsFlag(); // F3 toggles a debug overlay (like Bomberman)
  const spriteFor = useHeroSprites(g.view); // walk/facing animation for the board pawns
  const [sel, setSel] = useState<number | null>(null); // selected die index
  const [layRot, setLayRot] = useState(0); // chosen orientation (0..3) while laying a tile
  const v = g.view;
  // Reset the orientation to the first legal one whenever a fresh tile is drawn.
  useEffect(() => {
    setLayRot(g.view?.pending?.options[0]?.rotation ?? 0);
  }, [v?.step, v?.pending?.kind, v?.pending?.options.length]);
  // Tell the page when the turn lands on THIS seat so local multiplayer auto-switches to
  // the player whose turn it is.
  const myTurnNow = v?.ph === "playing" && v?.turn === v?.you;
  useEffect(() => {
    if (myTurnNow) onMyTurn?.();
  }, [myTurnNow, onMyTurn]);
  // Flash when your health drops (your totem takes damage).
  const myHp = g.view ? g.view.heroes.find((h) => h.s === g.view!.you)?.health : undefined;
  const hpLost = useLossPulse(myHp);
  const bar = <RoomBar code={code} title={t("gameCrawler")} infoSlug="crawler" gameSlug="crawler" onLeave={onLeave} />;
  if (g.game && g.game !== "crawler") {
    return <div className="game-room">{bar}<WrongGameNotice actual={g.game} code={code} onLeave={onLeave} /></div>;
  }

  if (g.status === "closed") {
    return (
      <div className="game-room">
        {bar}
        <div className="panel game-notice">
          <h3>Disconnected</h3>
          <button className="game-btn" onClick={onLeave}>{t("gameBack")}</button>
        </div>
      </div>
    );
  }
  if (!v) {
    return (
      <div className="game-room">
        {bar}
        <p className="muted-note" style={{ padding: "1rem" }}>
          {g.status === "reconnecting" ? "Reconnecting…" : t("loading")}
        </p>
      </div>
    );
  }

  const isHost = g.you >= 0 && g.you === g.host;
  const me = v.heroes.find((h) => h.s === v.you);
  const myTurn = v.ph === "playing" && v.turn === v.you;
  const nameOf = (s: number) => v.heroes.find((h) => h.s === s)?.n ?? "?";

  // Tile-laying: the options span every legal frontier cell, each with the
  // rotations that fit there. `layRot` is the orientation the player has chosen; it
  // is kept across cells, so rotating then dropping anywhere keeps that rotation
  // wherever it is legal (and falls back to a cell's first legal rotation only when
  // the chosen one does not fit there).
  const layOpts = v.pending?.options ?? [];
  const layRots = Array.from(new Set(layOpts.map((o) => o.rotation))).sort((a, b) => a - b);
  const layPreview = layOpts.find((o) => o.rotation === layRot) ?? layOpts[0] ?? null;
  const layActive = myTurn && v.ph === "playing" && v.step === "lay" && !!layPreview;
  const placeAt = (x: number, y: number) => {
    const at = layOpts.filter((o) => o.x === x && o.y === y);
    if (at.length === 0) return;
    const chosen = at.find((o) => o.rotation === layRot) ?? at[0];
    g.send({ t: "layTile", x: chosen.x, y: chosen.y, rotation: chosen.rotation });
  };
  const rotateLay = () => {
    if (layRots.length <= 1) return;
    const i = layRots.indexOf(layRot);
    setLayRot(layRots[(i + 1) % layRots.length]);
  };
  // Where to keep the board scrolled: a move-triggered lay centres the one cell you
  // stepped onto; a proactive LAY-phase lay (placements across the frontier) and a
  // normal turn centre on your own tile.
  const myTile = me ? v.tiles.find((ti) => ti.id === me.tile) : undefined;
  // Portals: a hero on a portal tile's centre may teleport to any other portal. They
  // are numbered by tile id so the destination buttons read "Portal 1 / 2 / ...".
  const allPortals = v.tiles.filter((ti) => ti.kind === "portal").slice().sort((a, b) => a.id - b.id);
  const portalNo = (id: number) => allPortals.findIndex((p) => p.id === id) + 1;
  const onPortalCenter = !!me && !!myTile && myTile.kind === "portal" && me.square === 4;
  const portalDests = me ? allPortals.filter((ti) => ti.id !== me.tile) : [];
  const focus = layActive && !v.pending?.phaseLay && layOpts[0]
    ? { x: layOpts[0].x, y: layOpts[0].y }
    : myTile ? { x: myTile.x, y: myTile.y } : null;

  if (v.ph === "lobby") {
    return (
      <div className="game-room cr-room">
        {bar}
        <div className="mind-lobby">
          <p className="muted-note">1–6 heroes, co-op. Roll dice, spend them, explore tiles, and beat the boss before the stack runs out.</p>
          <ul className="mind-players">
            {v.heroes.map((h) => (
              <li key={h.s} className={h.gone ? "gone" : ""}>
                🧭 {h.n} · {h.totem}
                {h.s === v.you ? " (you)" : ""}
              </li>
            ))}
          </ul>
          {isHost ? (
            <button className="game-btn" onClick={() => g.send({ t: "start" })}>Start run</button>
          ) : (
            <p className="muted-note">Waiting for the host to start…</p>
          )}
        </div>
      </div>
    );
  }

  const isPhaseLay = !!v.pending?.phaseLay;
  const status =
    v.ph === "won"
      ? "Victory 🎉"
      : v.ph === "lost"
        ? "Defeat 💀"
        : v.step === "lay"
          ? isPhaseLay
            ? "Lay phase"
            : "Lay a tile"
          : myTurn
            ? "Your turn"
            : `${nameOf(v.turn)}'s turn`;

  return (
    <div className="game-room cr-room">
      {bar}
      <LossFlash show={hpLost} icon="💔" label="damage" />
      <div className="mind-hud panel">
        {v.level > 0 && <span className="cr-level">⛰ Level {v.level}/6 · {v.biome}</span>}
        <span>
          {v.grace >= 0
            ? `🂠 stack empty`
            : `🂠 ${v.deckLeft} tiles${v.bossNear ? " · boss near!" : ""}`}
        </span>
        {v.grace >= 0 && <span className="cr-collapse">⚠ collapse in {v.grace} turn{v.grace === 1 ? "" : "s"}</span>}
        <span>{status}</span>
      </div>

      {/* top row: character sheet (3/4) beside the dice + actions (1/4) */}
      <div className="cr-top-row">
        {me && <CharacterSheet me={me} lost={v.ph === "lost"} view={v} myTurn={myTurn} selDie={sel} onSend={(msg) => { g.send(msg); }} />}
        <div className="panel cr-action-col">
          {v.step === "lay" ? (
            layActive && layPreview ? (
              <>
                <strong>{isPhaseLay ? "Lay phase · you drew" : "You drew"}: {tileKind(v.pending!.kind).label}</strong>
                <span className="muted-note">{tileKind(v.pending!.kind).blurb}</span>
                <div
                  className="cr-lay-tile"
                  draggable
                  onDragStart={(e) => e.dataTransfer.setData("text/plain", "tile")}
                  title={isPhaseLay ? "Rotate me, then drop me on any glowing cell" : "Rotate me, then drop me on the glowing cell"}
                >
                  <TilePreview kind={v.pending!.kind} edges={layPreview.edges} rot={layPreview.rotation} size={92} dir={biomeTileDir(v.biome)} />
                </div>
                <div className="cr-lay-ctl">
                  {layRots.length > 1 && <button className="game-btn ghost" onClick={rotateLay}>⟳ Rotate</button>}
                  <span className="muted-note">
                    {isPhaseLay
                      ? "Rotate the tile, then drag it onto any glowing cell (or click one) to lay it so the paths line up. You move on your own turn."
                      : "The spot is fixed (where you stepped): rotate the tile, then click the glowing cell to step through."}
                  </span>
                </div>
              </>
            ) : (
              <p className="muted-note">{nameOf(v.turn)} is laying a tile…</p>
            )
          ) : myTurn && me ? (
            <>
              <div className="cr-dice">
                {v.dice.map((d, i) => (
                  <Die
                    key={i}
                    value={d.value}
                    spent={d.spent}
                    selected={sel === i}
                    usedFor={d.usedFor}
                    disabled={d.spent}
                    onClick={() => setSel(sel === i ? null : i)}
                  />
                ))}
              </div>
              {v.bonusMoves > 0 && (
                <div className="cr-actions">
                  <div className="cr-action-row">
                    <span>Keep moving ({v.bonusMoves} left):</span>
                    {DIRS.map((_, d) => (
                      <button key={d} className="game-btn" onClick={() => g.send({ t: "step", dir: d })}>{DIR_ARROW[d]}</button>
                    ))}
                  </div>
                </div>
              )}
              {onPortalCenter && portalDests.length > 0 && (
                <div className="cr-actions">
                  <div className="cr-action-row">
                    <span>🌀 Portal to:</span>
                    {portalDests.map((ti) => (
                      <button key={ti.id} className="game-btn" onClick={() => { g.send({ t: "portal", tile: ti.id }); setSel(null); }}>→ Portal {portalNo(ti.id)}</button>
                    ))}
                  </div>
                </div>
              )}
              {sel != null && v.dice[sel] && !v.dice[sel].spent && (
                <CrawlerActions
                  die={v.dice[sel]}
                  me={me}
                  view={v}
                  onAct={(msg) => { g.send(msg); setSel(null); }}
                  onReroll={() => { g.send({ t: "reroll", die: sel }); }}
                />
              )}
              <div className="cr-turn-actions">
                <button className="game-btn" onClick={() => { g.send({ t: "endTurn" }); setSel(null); }}>End turn</button>
              </div>
            </>
          ) : (
            <p className="muted-note">Waiting for {nameOf(v.turn)}…</p>
          )}
        </div>
      </div>

      {/* party */}
      <div className="cr-party">
        {v.heroes.map((h) => (
          <div key={h.s} className={`cr-hero${h.s === v.turn ? " active" : ""}${!h.alive ? " down" : ""}`}>
            <strong>{h.n}{h.s === v.you ? " (you)" : ""}</strong>
            <span>❤ {h.health}/{h.maxHP} · ✊ {h.courage} · 🛡 {h.shields}</span>
            {heroStatuses(h).length > 0 && (
              <span className="cr-status-row">
                {heroStatuses(h).map((s) => (
                  <span key={s} className="cr-status" title={`${STATUSES[s].label}: ${STATUSES[s].blurb}`}>{STATUSES[s].icon}</span>
                ))}
              </span>
            )}
            <span className="muted-note">{h.totem}{h.unlocked[0] ? " ·★" : ""}{h.unlocked[1] ? "★" : ""}</span>
            <span className="cr-pack-row" title={`${h.objects.length}/${h.slots || OBJECT_CAP} object slots`}>
              {Array.from({ length: h.slots || OBJECT_CAP }, (_, i) => {
                const o = h.objects[i];
                return o
                  ? <span key={i} className="cr-pack-cell" title={`${objInfo(o).name}: ${objInfo(o).effect}`}><ObjIcon slug={o} size={18} /></span>
                  : <span key={i} className="cr-pack-cell empty" />;
              })}
            </span>
          </div>
        ))}
      </div>

      {(v.ph === "won" || v.ph === "lost") && (
        <div className="mind-controls panel mind-over">
          <strong>{v.ph === "won" ? "The boss falls. You win! 🎉" : "The dungeon claims you. 💀"}</strong>
          {isHost && <button className="game-btn" onClick={() => g.send({ t: "restart" })}>New run</button>}
        </div>
      )}

      {/* the board, with the shop as an overlay callout when you stand on a shop tile */}
      <div className="cr-board-wrap">
        <CrawlerMap
          v={v}
          focus={focus}
          debug={debug}
          spriteFor={spriteFor}
          onPickup={(sq) => g.send({ t: "pickup", square: sq })}
          lay={layActive && layPreview ? { kind: v.pending!.kind, rot: layRot, options: layOpts, place: placeAt } : null}
        />
        {me && myTurn && myTile?.kind === "shop" && me.square === 4 && (
          <CrawlerShopCallout me={me} shop={v.shopStock} onSend={(msg) => g.send(msg)} />
        )}
        {v.lootRoll && <CrawlerLootRoll key={`lr-${v.lootRoll.roll}-${v.lootRoll.chance}-${v.lootRoll.killer ?? ""}`} roll={v.lootRoll} />}
        {v.statusRoll && <CrawlerStatusRoll key={`sr-${v.statusRoll.roll}-${v.statusRoll.status}-${v.statusRoll.target ?? ""}`} roll={v.statusRoll} />}
      </div>

      {debug && <CrawlerDebug v={v} />}

      {/* the game log */}
      {v.log.length > 0 && (
        <div className="cr-log panel">
          {v.log.slice(-8).map((l, i) => <div key={i}>{l}</div>)}
        </div>
      )}
    </div>
  );
}

// The player's own character sheet, shown prominently at the top: totem sprite,
// health bar, resources, the action menu (die thresholds) and the two specials.
function CharacterSheet({ me, lost, view, myTurn, selDie, onSend }: {
  me: CrHero;
  lost: boolean;
  view: CrView;
  myTurn: boolean;
  selDie: number | null;
  onSend: (msg: object) => void;
}) {
  const mins = menuMins(me);
  const spec = specialInfo(me.special);
  const unlocks = unlockInfo(me);
  const myTile = view.tiles.find((t) => t.id === me.tile);
  const grenadeTargets = (myTile?.monsters ?? []).filter((m) => m.alive && m.health <= 1 && !m.isBoss);
  const hasPortal = view.tiles.some((t) => t.kind === "portal");
  // Whether a held object can be used right now (from its slot), and what happens.
  const usable = (slug: string): boolean => {
    if (!myTurn || !me.alive) return false;
    switch (slug) {
      case "lucky-charm": return selDie != null;
      case "grenade": return grenadeTargets.length > 0;
      case "medkit": return me.health < me.maxHP || !!me.electrified;
      case "battery": return me.courage < COURAGE_CAP;
      case "recall-whistle": return hasPortal;
      case "torch": return (myTile?.hazards?.length ?? 0) > 0; // clears this tile's hazards
      case "courage-horn": case "beacon": case "sap-seed": return true;
      default: return false; // other permanents are passive
    }
  };
  const useSlot = (slug: string) => {
    if (slug === "lucky-charm" && selDie != null) onSend({ t: "lucky", die: selDie });
    else if (slug === "grenade" && grenadeTargets.length > 0) onSend({ t: "use", obj: "grenade", monster: grenadeTargets[0].id });
    else onSend({ t: "use", obj: slug });
  };
  const hpPct = Math.max(0, Math.min(100, Math.round((me.health / me.maxHP) * 100)));
  const couPct = Math.max(0, Math.min(100, Math.round((me.courage / COURAGE_CAP) * 100)));
  // A special's label turns blue once Courage reaches its unlock cost (and it is
  // not yet unlocked), so "you can unlock this now" is unmissable.
  // A special auto-activates at its Courage threshold and switches off below it.
  const specClass = (i: 0 | 1) => (me.unlocked[i] ? "ready" : "");
  return (
    <div className="panel cr-sheet">
      <div className="cr-sheet-head">
        <TotemSprite slug={me.totem} size={56} dead={!me.alive || lost} />
        <div className="cr-sheet-id">
          <strong>{me.n}</strong>
          <span className="muted-note">{me.totem}{me.alive ? "" : " · down"}</span>
        </div>
        <div className="cr-sheet-stats">
          <div className="cr-bar-row">
            <span className="cr-bar-label">❤ {me.health}/{me.maxHP}</span>
            <div className="cr-bar cr-hpbar"><span style={{ width: `${hpPct}%` }} /></div>
          </div>
          <div className="cr-bar-row">
            <span className="cr-bar-label">✊ {me.courage}/{COURAGE_CAP}</span>
            <div className="cr-bar cr-courbar">
              <span style={{ width: `${couPct}%` }} />
              <i className="cr-bar-tick" style={{ left: `${(COURAGE_UNLOCKS[0] / COURAGE_CAP) * 100}%` }} title={`${unlocks[0].name} at ${COURAGE_UNLOCKS[0]}`} />
              <i className="cr-bar-tick" style={{ left: `${(COURAGE_UNLOCKS[1] / COURAGE_CAP) * 100}%` }} title={`${unlocks[1].name} at ${COURAGE_UNLOCKS[1]}`} />
            </div>
          </div>
          <span className="muted-note">🛡 {me.shields} shields</span>
        </div>
      </div>
      <div className="cr-menu">
        <span className="cr-menu-item">Move <b>≥{mins.move}</b></span>
        {me.special && (
          <span className="cr-menu-item">{spec.name} <b>≥{mins[me.special]}</b> <InfoDot text={spec.blurb} /></span>
        )}
        <span className="cr-menu-item">Attack <InfoDot text="Spend any die that meets the monster's Armour as an attack. A die below the Armour cannot land, so it is not offered; spend low dice on Move or your Special." /></span>
        <span className="cr-menu-item">Courage <b>≥{mins.courage}</b></span>
      </div>
      <div className="cr-specials">
        <span className={specClass(0)} key={`u0-${me.unlocked[0]}`}>
          ★ {unlocks[0].name} {me.unlocked[0] ? "⚡active" : `(${COURAGE_UNLOCKS[0]}✊)`} <InfoDot text={unlocks[0].blurb} />
        </span>
        <span className={specClass(1)} key={`u1-${me.unlocked[1]}`}>
          ★ {unlocks[1].name} {me.unlocked[1] ? "⚡active" : `(${COURAGE_UNLOCKS[1]}✊)`} <InfoDot text={unlocks[1].blurb} />
        </span>
      </div>
      <div className="cr-slots">
        <span className="cr-slots-label">🎒 Objects {me.objects.length}/{me.slots || OBJECT_CAP}</span>
        {Array.from({ length: me.slots || OBJECT_CAP }, (_, i) => {
          const slug = me.objects[i];
          if (!slug) {
            return (
              <span key={i} className="cr-slot empty">
                <InfoDot text="An empty object slot. Fill it with loot dropped by robots, from a Vault, or bought at the kangaroo shop." />
              </span>
            );
          }
          const info = objInfo(slug);
          const canUse = usable(slug);
          const tip = `${info.name}: ${info.effect}${info.perm ? " (permanent)" : " (one-use)"}`;
          return (
            <span key={i} className={`cr-slot filled${canUse ? " usable" : ""}`}>
              {canUse ? (
                <button className="cr-slot-btn" title={`Use: ${info.name}`} onClick={() => useSlot(slug)}><ObjIcon slug={slug} size={26} /></button>
              ) : (
                <ObjIcon slug={slug} size={26} />
              )}
              {myTurn && me.alive && (
                <button className="cr-slot-drop" title={`Drop ${info.name} on your square`} onClick={() => onSend({ t: "drop", obj: slug })}>⤓</button>
              )}
              <InfoDot text={tip} />
            </span>
          );
        })}
      </div>
    </div>
  );
}

// CrawlerShop is the kangaroo's stall, shown only while you stand on a shop tile: buy
// objects with Courage into a free slot. (Using/held objects live on the character
// sheet's slots now.)
// CrawlerShopCallout overlays the board (solid panel, its own scroll) when you stand on
// a shop tile's centre. Each item shows its icon, name, cost and a readable description
// of what it does; Buy spends Courage into a free slot.
function CrawlerShopCallout({ me, shop, onSend }: {
  me: CrHero;
  shop: { slug: string; name: string; cost: number }[];
  onSend: (msg: object) => void;
}) {
  const packFull = me.objects.length >= (me.slots || OBJECT_CAP);
  return (
    <div className="cr-shop-callout">
      <div className="cr-shop-callout-head">
        🛒 Kangaroo shop <span className="muted-note">({me.courage}✊ · {me.objects.length}/{me.slots || OBJECT_CAP} slots)</span>
      </div>
      {packFull && <div className="cr-shop-note">Pack full: use or drop something first.</div>}
      {shop.length === 0 && <div className="cr-shop-note">Sold out this level.</div>}
      <ul className="cr-shop-list">
        {shop.map((s) => {
          const info = objInfo(s.slug);
          const afford = me.courage >= s.cost && !packFull;
          return (
            <li key={s.slug} className="cr-shop-item">
              <ObjIcon slug={s.slug} size={26} />
              <div className="cr-shop-item-text">
                <strong>{info.name}</strong>
                <span className="muted-note">{info.effect}</span>
              </div>
              <button className="game-btn" disabled={!afford} onClick={() => onSend({ t: "buy", obj: s.slug })}>
                {s.cost}✊
              </button>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

function CrawlerActions({
  die,
  me,
  view,
  onAct,
  onReroll,
}: {
  die: CrDie;
  me: CrHero;
  view: CrView;
  onAct: (msg: object) => void;
  onReroll: () => void;
}) {
  const mins = menuMins(me);
  const can = (action: string) => die.value >= (mins[action] ?? 99);
  const myTile = view.tiles.find((x) => x.id === me.tile);
  const allies = view.heroes.filter((h) => h.alive && h.tile === me.tile);
  const others = allies.filter((a) => a.s !== me.s);
  const specLabel = specialInfo(me.special).name;
  const di = dieIdx(view, die);
  // Arrow Squid's Barbed Shot (first special, active by Courage) lowers a target's
  // Armour by 1; a die lands only if it meets the effective Armour (mirrors the server).
  const barbed = me.totem === "pijlinktvis" && me.unlocked[0];
  const effArmour = (m: CrMonster) => effectiveArmour(m.armour, barbed);
  // Attack targets: living monsters on your tile, plus (for a ranged totem) ones on a
  // tile reachable through an open edge of yours. Only those the selected die can
  // actually hit are offered: attacking with a die below the Armour is not allowed.
  const targets: { tileId: number; m: CrMonster; ranged: boolean }[] = [];
  (myTile?.monsters ?? []).filter((m) => m.alive).forEach((m) => targets.push({ tileId: myTile!.id, m, ranged: false }));
  if (me.ranged && myTile) {
    CR_DELTA.forEach(([dx, dy], d) => {
      if (!myTile.edges[d]) return;
      const nt = view.tiles.find((t) => t.x === myTile.x + dx && t.y === myTile.y + dy);
      // The boss can't be sniped from an adjacent tile (the server rejects it), so don't
      // offer it as a ranged target at all; only ordinary robots next door are hittable.
      nt?.monsters.filter((m) => m.alive && !m.isBoss).forEach((m) => targets.push({ tileId: nt.id, m, ranged: true }));
    });
  }
  const hittable = targets.filter(({ m }) => die.value >= effArmour(m));

  return (
    <div className="cr-actions">
      <div className="cr-action-row">
        {me.freeReroll ? (
          <button className="game-btn ghost" title="A free reroll is available this turn" onClick={onReroll}>↻ reroll ({me.freeSrc || "🎲"} free)</button>
        ) : (
          <button className="game-btn ghost" disabled={me.courage < 1} onClick={onReroll}>↻ reroll (1✊)</button>
        )}
      </div>
      {can("move") && (
        <div className="cr-action-row">
          <span>Move:</span>
          {DIRS.map((_, d) => (
            <button key={d} className="game-btn" onClick={() => onAct({ t: "spend", die: di, action: "move", dir: d })}>{DIR_ARROW[d]}</button>
          ))}
        </div>
      )}
      {hittable.length > 0 && (
        <div className="cr-action-row">
          <span>Attack:</span>
          {hittable.map(({ tileId, m, ranged }) => (
            <button
              key={`${tileId}-${m.id}`}
              className="game-btn"
              title={`This ${die.value} meets Armour ${effArmour(m)}${barbed && effArmour(m) < m.armour ? " (Barbed Shot: -1)" : ""}: a hit.${ranged ? " Ranged, adjacent tile." : ""} Health ${m.health}/${m.maxHP}.`}
              onClick={() => onAct({ t: "spend", die: di, action: "attack", monster: m.id, tile: tileId })}
            >
              {ranged ? "🎯" : m.isBoss ? "👑" : "👹"} A{effArmour(m)} · ❤{m.health}/{m.maxHP} ✓
            </button>
          ))}
        </div>
      )}
      {can(me.special) && (me.special === "brace" || me.special === "mend") && (
        <div className="cr-action-row">
          <span>{specLabel}:</span>
          {allies.map((a) => (
            <button key={a.s} className="game-btn" onClick={() => onAct({ t: "spend", die: di, action: me.special, ally: a.s })}>
              {a.s === me.s ? "self" : a.n}
            </button>
          ))}
        </div>
      )}
      {can(me.special) && me.special === "carry" && others.map((a) => (
        <div className="cr-action-row" key={a.s}>
          <span>Carry {a.n}:</span>
          {DIRS.map((_, d) => (
            <button key={d} className="game-btn" onClick={() => onAct({ t: "spend", die: di, action: "carry", ally: a.s, dir: d })}>{DIR_ARROW[d]}</button>
          ))}
        </div>
      ))}
      {can(me.special) && me.special === "guard" && (
        <div className="cr-action-row">
          <span>{specLabel}:</span>
          <button className="game-btn" onClick={() => onAct({ t: "spend", die: di, action: "guard", ally: me.s })}>self (+1🛡)</button>
        </div>
      )}
      {can("courage") && me.courage < COURAGE_CAP && (
        <div className="cr-action-row">
          <button className="game-btn" onClick={() => onAct({ t: "spend", die: di, action: "courage" })}>✊ Courage +1</button>
        </div>
      )}
      {can("courage") && me.courage >= COURAGE_CAP && <div className="cr-action-row muted-note">✊ Courage is maxed ({COURAGE_CAP}).</div>}
    </div>
  );
}

const CR_DELTA: [number, number][] = [[0, -1], [1, 0], [0, 1], [-1, 0]];

// CrawlerDebug is the F3 overlay (parallels Bomberman's NetHud): the state that used
// to clutter the board as labels/hover boxes — tile kinds and coords, and every
// monster's full stat line — lives here instead.
function CrawlerDebug({ v }: { v: CrView }) {
  return (
    <div className="nethud cr-debug">
      <div className="nethud-title">crawler debug · phase {v.ph}/{v.step} · turn {v.turn} · deck {v.deckLeft}{v.grace >= 0 ? ` · collapse ${v.grace}` : ""}</div>
      {v.tiles.map((t) => (
        <div key={t.id} className="nethud-row">
          <span>#{t.id} {tileKind(t.kind).label} ({t.x},{t.y}) rot{t.rot}</span>
          <span>{t.monsters.filter((m) => m.alive).map((m) => `${m.kind} A${m.armour}/H${m.health}/D${m.damage}`).join(", ") || "—"}</span>
        </div>
      ))}
      <div className="nethud-hint">F3 to hide</div>
    </div>
  );
}

// dieIdx recovers the selected die's index (the view's dice array is stable).
function dieIdx(view: CrView, die: CrDie): number {
  return view.dice.findIndex((d) => d === die);
}

// TileGlyph draws a small square with doorway notches on open edges.
function TileGlyph({ edges }: { edges: [boolean, boolean, boolean, boolean] }) {
  return (
    <span className="cr-glyph">
      {edges[0] && <i className="cr-door n" />}
      {edges[1] && <i className="cr-door e" />}
      {edges[2] && <i className="cr-door s" />}
      {edges[3] && <i className="cr-door w" />}
    </span>
  );
}

// TilePreview shows a tile's art (rotated to its edges) at an arbitrary size, or
// the line glyph if the kind has no art. Used for the lay panel and the map ghost.
function TilePreview({ kind, edges, rot = 0, size = 56, dir = "woodland" }: { kind: string; edges: Edges; rot?: number; size?: number; dir?: string }) {
  const art = tileArt(kind, edges, rot);
  if (!art) return <TileGlyph edges={edges} />;
  return (
    <span
      className="cr-tile-prev"
      style={{
        width: size,
        height: size,
        backgroundImage: `url(/sprites/crawler/tiles/${dir}/${art.file}.png)`,
        transform: `rotate(${art.deg}deg)`,
      }}
    />
  );
}

// Pip layout (row,col on a 3x3) for each die value, drawn as dots (no image fetch).
const DIE_PIPS: Record<number, [number, number][]> = {
  1: [[1, 1]],
  2: [[0, 0], [2, 2]],
  3: [[0, 0], [1, 1], [2, 2]],
  4: [[0, 0], [0, 2], [2, 0], [2, 2]],
  5: [[0, 0], [0, 2], [1, 1], [2, 0], [2, 2]],
  6: [[0, 0], [1, 0], [2, 0], [0, 2], [1, 2], [2, 2]],
};

// Die renders a value as pips. The face is keyed by value so it remounts and plays
// the tumble animation whenever the value changes (a roll or a reroll).
function Die({ value, spent, selected, usedFor, disabled, onClick }: {
  value: number; spent: boolean; selected: boolean; usedFor: string; disabled: boolean; onClick: () => void;
}) {
  const on = new Set((DIE_PIPS[value] ?? []).map(([r, c]) => r * 3 + c));
  return (
    <button
      className={`cr-die${spent ? " spent" : ""}${selected ? " sel" : ""}`}
      disabled={disabled}
      onClick={onClick}
      title={spent ? usedFor : "select this die"}
    >
      <span className="cr-die-face" key={value}>
        {Array.from({ length: 9 }, (_, i) => (
          <i key={i} className={on.has(i) ? "pip on" : "pip"} />
        ))}
      </span>
      {spent && <small>{usedFor[0]}</small>}
    </button>
  );
}

// useAutoHide returns true, then false after `ms`, so a transient banner removes itself.
// Give the component a key tied to the event so a fresh event remounts it and the timer
// restarts (otherwise the one-shot timer never fires again).
function useAutoHide(ms: number): boolean {
  const [shown, setShown] = useState(true);
  useEffect(() => {
    const t = setTimeout(() => setShown(false), ms);
    return () => clearTimeout(t);
  }, [ms]);
  return shown;
}

// CrawlerLootRoll is the visible drop die shown after a kill: the d100 result against
// the robot's drop chance, and whether an item fell. It overlays the board for the rest
// of the turn (the server clears it when the next turn begins). Keyed on the roll so a
// fresh kill re-triggers the tumble.
function CrawlerLootRoll({ roll }: { roll: NonNullable<CrView["lootRoll"]> }) {
  if (!useAutoHide(2000)) return null;
  return (
    <div className={`cr-lootroll${roll.hit ? " hit" : " miss"}`} key={`${roll.roll}-${roll.chance}`} role="status">
      <span className="cr-lootroll-die">🎲 {roll.roll}</span>
      <span className="cr-lootroll-vs">vs ≤{roll.chance}</span>
      <span className="cr-lootroll-result">
        {roll.hit ? `${roll.name || "Loot"} drops!` : "No drop"}
      </span>
    </div>
  );
}

// CrawlerStatusRoll is the visible status die: when a robot's sting rolls for a status,
// it shows the d6 result against the needed face and whether the status landed. Keyed on
// the roll so a fresh sting re-triggers the tumble. Cleared by the server next turn.
function CrawlerStatusRoll({ roll }: { roll: NonNullable<CrView["statusRoll"]> }) {
  const meta = STATUSES[roll.status];
  if (!useAutoHide(2000)) return null;
  return (
    <div className={`cr-statusroll${roll.hit ? " hit" : " miss"}`} key={`${roll.roll}-${roll.status}`} role="status">
      <span className="cr-lootroll-die">🎲 {roll.roll}</span>
      <span className="cr-lootroll-vs">vs ≥{roll.need}</span>
      <span className="cr-lootroll-result">
        {roll.hit ? `${roll.target ?? "A hero"} ${meta?.icon ?? ""} ${meta?.label ?? roll.status}` : "no status"}
      </span>
    </div>
  );
}

type LayState = {
  kind: string;
  rot: number; // the orientation the player has chosen (0..3)
  options: CrLayOption[];
  place: (x: number, y: number) => void;
};

// CrawlerMap lays out placed tiles (and frontier markers) on a scrollable grid.
// `focus` is the cell to keep centred (your tile, or the tile being laid); `lay`
// turns every legal frontier cell into a glowing drop-zone, with a ghost of the
// tile on the currently-selected cell.
function CrawlerMap({ v, focus, lay, debug, spriteFor, onPickup }: { v: CrView; focus: { x: number; y: number } | null; lay: LayState | null; debug?: boolean; spriteFor: SpriteFor; onPickup?: (square: number) => void }) {
  const scrollRef = useRef<HTMLDivElement>(null);
  // Keep the focus cell centred in the scroll box as the map grows / you move.
  useEffect(() => {
    const c = scrollRef.current;
    if (!c) return;
    const el = c.querySelector('[data-focus="1"]') as HTMLElement | null;
    if (!el) return;
    const cr = c.getBoundingClientRect();
    const er = el.getBoundingClientRect();
    c.scrollLeft += er.left + er.width / 2 - (cr.left + cr.width / 2);
    c.scrollTop += er.top + er.height / 2 - (cr.top + cr.height / 2);
  }, [focus?.x, focus?.y, v.tiles.length]);

  const coords = [...v.tiles.map((t) => [t.x, t.y]), ...v.frontiers.map((f) => [f.x, f.y])];
  if (lay) lay.options.forEach((o) => coords.push([o.x, o.y]));
  if (coords.length === 0) return null;
  const xs = coords.map((c) => c[0]);
  const ys = coords.map((c) => c[1]);
  const minX = Math.min(...xs), maxX = Math.max(...xs);
  const minY = Math.min(...ys), maxY = Math.max(...ys);
  const cols = maxX - minX + 1;
  const tileAt = (x: number, y: number) => v.tiles.find((t) => t.x === x && t.y === y);
  const frontierAt = (x: number, y: number) => v.frontiers.some((f) => f.x === x && f.y === y);
  const layAt = (x: number, y: number) => (lay ? lay.options.filter((o) => o.x === x && o.y === y) : []);
  const isFocus = (x: number, y: number) => !!focus && focus.x === x && focus.y === y;

  const cells = [];
  for (let y = minY; y <= maxY; y++) {
    for (let x = minX; x <= maxX; x++) {
      const key = `${x},${y}`;
      const tile = tileAt(x, y);
      const atCell = layAt(x, y);
      if (lay && atCell.length > 0) {
        // Show the chosen rotation where it fits; otherwise this cell's first legal one.
        const hasRot = atCell.some((o) => o.rotation === lay.rot);
        const ghost = atCell.find((o) => o.rotation === lay.rot) ?? atCell[0];
        const art = tileArt(lay.kind, ghost.edges, ghost.rotation);
        cells.push(
          <div
            key={key}
            className={`cr-cell cr-lay-drop${hasRot ? " sel" : " alt"}${isFocus(x, y) ? " focus" : ""}`}
            data-focus={isFocus(x, y) ? "1" : undefined}
            onDragOver={(e) => e.preventDefault()}
            onDrop={(e) => { e.preventDefault(); lay.place(x, y); }}
            onClick={() => lay.place(x, y)}
            title={hasRot ? "Drop or click to place here (your rotation)" : "Drop or click to place here (this cell's rotation)"}
          >
            {art ? (
              <div className="cr-tile-art" style={{ backgroundImage: `url(/sprites/crawler/tiles/${biomeTileDir(v.biome)}/${art.file}.png)`, transform: `rotate(${art.deg}deg)` }} />
            ) : (
              <TileGlyph edges={ghost.edges} />
            )}
          </div>,
        );
      } else if (tile) {
        cells.push(<CrawlerTileCell key={key} tile={tile} v={v} focus={isFocus(x, y)} debug={debug} spriteFor={spriteFor} onPickup={onPickup} />);
      } else if (frontierAt(x, y)) {
        cells.push(<div key={key} className="cr-cell cr-frontier">?</div>);
      } else {
        cells.push(<div key={key} className="cr-cell cr-empty" />);
      }
    }
  }
  return (
    <div className="cr-map-scroll" ref={scrollRef}>
      <div className="cr-map" style={{ gridTemplateColumns: `repeat(${cols}, var(--cr-cell))` }}>
        {cells}
      </div>
    </div>
  );
}

function CrawlerTileCell({ tile, v, focus, debug, spriteFor, onPickup }: { tile: CrTile; v: CrView; focus?: boolean; debug?: boolean; spriteFor: SpriteFor; onPickup?: (square: number) => void }) {
  const heroesHere = v.heroes.filter((h) => h.tile === tile.id); // includes the fallen (death stance)
  const lost = v.ph === "lost"; // a party wipe or collapse: everyone drops to the defeated frame
  // You can grab ground loot only on your own turn, in the spend step, while your hero
  // stands on THIS tile (no sniping loot from the tile next door).
  const you = v.heroes.find((h) => h.s === v.you);
  // Loot can be grabbed off-turn: any time your hero is alive and standing on this tile
  // (no need to wait for your turn), so a boss's drops stay reachable after the turn moves on.
  const canGrab = !!onPickup && v.ph === "playing" && !!you?.alive && you?.tile === tile.id;
  const edgeClass = ["n", "e", "s", "w"].filter((_, d) => tile.edges[d]).map((s) => `open-${s}`).join(" ");
  const art = tileArt(tile.kind, tile.edges, tile.rot);
  return (
    <div
      className={`cr-cell cr-tile ${edgeClass}${tile.isBoss ? " boss" : ""}${tile.used ? " used" : ""}`}
      data-focus={focus ? "1" : undefined}
    >
      {art && (
        <div
          className="cr-tile-art"
          style={{ backgroundImage: `url(/sprites/crawler/tiles/${biomeTileDir(v.biome)}/${art.file}.png)`, transform: `rotate(${art.deg}deg)` }}
        />
      )}
      {debug && <span className="cr-tile-kind">{tileKind(tile.kind).label} ({tile.x},{tile.y})</span>}
      {tile.fogged && <div className="cr-fog" title="Sensor fog: move a hero onto or next to this tile to reveal it">🌫️</div>}
      <div className="cr-squares">
        {Array.from({ length: 9 }, (_, sq) => {
          // Each square holds at most one monster; a defeated one stays put in its
          // destroyed frame (no stat badge), a living one shows its stats + Courage reward.
          const monster = tile.monsters.find((m) => m.square === sq);
          const here = heroesHere.filter((h) => h.square === sq);
          const loot = (tile.loot ?? []).filter((l) => l.square === sq);
          const hazard = (tile.hazards ?? []).find((hz) => hz.square === sq);
          return (
            <div key={sq} className="cr-sq">
              {hazard && HAZARDS[hazard.kind] && (
                <span className="cr-hazard" title={HAZARDS[hazard.kind].label}>{HAZARDS[hazard.kind].icon}</span>
              )}
              {loot.map((l, i) => {
                const info = objInfo(l.slug);
                const title = `${info.name} on the floor${canGrab ? " (click to pick up)" : " (stand on this tile to pick it up)"}`;
                return canGrab ? (
                  <button
                    key={`loot-${i}`}
                    type="button"
                    className="cr-loot grab"
                    title={title}
                    onClick={() => onPickup!(sq)}
                  >
                    <ObjIcon slug={l.slug} size={18} />
                  </button>
                ) : (
                  <span key={`loot-${i}`} className="cr-loot" title={title}>
                    <ObjIcon slug={l.slug} size={18} />
                  </span>
                );
              })}
              {monster && (
                <span className="cr-mon">
                  {monster.alive && monster.name && (
                    <span className="cr-boss-name" title={(monster.specials ?? []).map(specialLabel).join(" · ")}>
                      {monster.name}{(monster.specials?.length ?? 0) > 0 ? " ⚡" : ""}
                    </span>
                  )}
                  <MonsterSprite m={monster} />
                  {monster.alive && (
                    <span className="cr-mon-badge" title={`Armour ${monster.armour}, Health ${monster.health}/${monster.maxHP}, Damage ${monster.damage}, grants ${monster.reward} Courage on defeat${(monster.specials?.length ?? 0) > 0 ? ". Specials: " + (monster.specials ?? []).map(specialLabel).join(", ") : ""}`}>
                      🛡{monster.armour} ❤{monster.health} 💥{monster.damage} ✊{monster.reward}
                    </span>
                  )}
                </span>
              )}
              {here.map((h) => {
                const a = spriteFor(h.s);
                const dead = !h.alive || lost;
                return (
                  <span key={h.s} className={`cr-pawn${h.s === v.turn ? " active" : ""}${h.s === v.you ? " you" : ""}${dead ? " dead" : ""}`} title={h.n + (h.alive ? "" : " (down)")}>
                    <TotemSprite slug={h.totem} size={40} pose={a.pose} frame={a.frame} flip={a.flip} dead={dead} />
                  </span>
                );
              })}
            </div>
          );
        })}
      </div>
    </div>
  );
}
