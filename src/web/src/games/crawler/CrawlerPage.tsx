import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router-dom";
import { createGameRoom } from "../../api";
import { CODE_LEN, NICK_MAX, validNick } from "../validate";
import { useRoom } from "../useRoom";
import { RoomBar } from "../RoomBar";
import { LossFlash, useLossPulse } from "../LossFlash";
import { useStatsFlag } from "../bomberman/NetHud";

// A roster of totems to play as. Only the Beaver has a distinct sheet in V0
// (Brace); the rest share the generic sheet. meta = the slug sent on join. Every
// slug here has sprites in the Bomberman atlas (see SPRITE_ROWS).
const TOTEMS = [
  { slug: "bever", label: "Beaver 🦫 (Guardian · 4 Low dice, HP7)" },
  { slug: "pijlinktvis", label: "Arrow squid 🦑 (Striker · 4 High dice, HP4)" },
  { slug: "boomkikker", label: "Treefrog 🐸 (Healer · 4 Low dice, HP6)" },
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
];
const ATLAS_COLS = 11; // front(3)+back(3)+side(4)+defeated(1)
const ATLAS_ROWS = SPRITE_ROWS.length;

// Walk poses in the atlas: each pose's first column and its walk-cycle length. The
// art faces right, so a leftward move mirrors the `side` pose. The defeated frame is
// the last column. (Same layout as Bomberman's totems.json: front 0-2, back 3-5,
// side 6-9, defeated 10.)
type CrPose = "front" | "back" | "side";
const POSE_BASE: Record<CrPose, number> = { front: 0, back: 3, side: 6 };
const POSE_LEN: Record<CrPose, number> = { front: 3, back: 3, side: 4 };
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
}

// Enemies with art (scripts/make_enemies.py). Each renders its idle frame, or its
// attack frame + a lunge while engaged (the hero is fighting it this turn). Unknown
// kinds and the boss fall back to an emoji.
const ENEMY_ART = new Set(["drone", "brute", "tank", "boss"]);
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
  monsters: CrMonster[];
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
  log: string[];
}

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

// Human labels and one-line explanations for the tile shapes (kind on the wire).
const TILE_KINDS: Record<string, { label: string; blurb: string }> = {
  straight: { label: "Straight", blurb: "A corridor: two doorways on opposite sides." },
  bend: { label: "Bend", blurb: "A corner: two doorways on adjacent sides." },
  tee: { label: "T-junction", blurb: "Three doorways." },
  cross: { label: "Crossroads", blurb: "Open on all four sides." },
  "dead-end": { label: "Dead end", blurb: "A single doorway: caps a branch." },
  boss: { label: "Boss lair", blurb: "The boss waits here. One doorway in." },
  portal: { label: "Portal", blurb: "Stand on the centre to teleport to another portal." },
  healing: { label: "Healing spring", blurb: "Step onto the centre to heal. No die needed, and it keeps working on each fresh entry." },
  shop: { label: "Shop", blurb: "A merchant's stall. Trading arrives with the loot system." },
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
type Edges = [boolean, boolean, boolean, boolean];
const T = true, F = false;
const TILE_ART: Record<string, { file: string; base: Edges }> = {
  straight: { file: "tile-1", base: [T, F, T, F] }, //   horizontal corridor
  bend: { file: "tile-2", base: [F, T, T, F] }, //        N+W corner
  tee: { file: "tile-3", base: [T, T, T, F] }, //         E+S+W junction
  cross: { file: "tile-4", base: [T, T, T, T] }, //       4-way clearing
  "dead-end": { file: "tile-0", base: [F, F, T, F] }, //  one doorway (from the top)
  healing: { file: "tile-5", base: [T, T, T, F] }, //     healing spring: same open edges as the T-junction (tee)
  portal: { file: "tile-6", base: [T, T, T, T] }, //      transporter/start: same open edges as the crossroads
  shop: { file: "tile-7", base: [F, T, F, F] }, //        merchant stall, a dead-end nook
  boss: { file: "tile-8", base: [T, F, F, F] }, //        the boss lair
};

function rotateEdges(base: Edges, r: number): Edges {
  return [0, 1, 2, 3].map((d) => base[(d - r + 4) % 4]) as Edges;
}

// tileArt picks the art for a tile and the rotation (0/90/180/270) to draw it at.
// It uses the rotations whose edges line up with the tile's actual open edges, and
// among those prefers the one matching the tile's stored `rot` so a symmetric tile
// (the crossroads) still rotates visibly. Returns null when the kind has no art.
function tileArt(kind: string, edges: Edges, rot = 0): { file: string; deg: number } | null {
  const art = TILE_ART[kind];
  if (!art) return null;
  const valid: number[] = [];
  for (let r = 0; r < 4; r++) {
    if (rotateEdges(art.base, r).every((v, i) => v === edges[i])) valid.push(r);
  }
  const want = ((rot % 4) + 4) % 4;
  const pick = valid.includes(want) ? want : valid.length ? valid[0] : want;
  return { file: art.file, deg: pick * 90 };
}

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
  "Spores": "Mend also clears a hazard on the target. Spec'd; needs the hazard system.",
  "Bloom": "Once a round, Mend heals everyone on your tile 1. Spec'd; not active in V0 yet.",
  "Lucky Paws": "Once a turn, reroll any one die for free. Spec'd; not active in V0 yet.",
  "Big Score": "Defeated robots drop +1 loot; hold 4 loot. Spec'd; needs the loot system.",
  "Ferry": "Carry an ally as far as you move, across tiles. Spec'd; needs the carry system.",
  "Flock": "Carry two allies, or the carried ally gets a free step. Spec'd; needs the carry system.",
  "Resolve": "A self-buff unlocked with Courage.",
  "Second Wind": "A stronger self-buff unlocked with Courage.",
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
      const dx = gx - prev.gx, dy = gy - prev.gy;
      let pose = prev.pose, flip = prev.flip;
      if (Math.abs(dx) >= Math.abs(dy)) {
        pose = "side";
        if (dx < 0) flip = true;
        else if (dx > 0) flip = false;
      } else {
        pose = dy < 0 ? "back" : "front"; // up = away (back), down = toward (front)
      }
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
  const [session, setSession] = useState<{ code: string; name: string; totem: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (session) {
    return (
      <main className="game-page">
        <CrawlerRoom {...session} onLeave={() => setSession(null)} />
      </main>
    );
  }

  const begin = (roomCode: string) => {
    const nick = name.trim();
    localStorage.setItem("totem-game-name", nick);
    localStorage.setItem("totem-crawler-totem", totem);
    setSession({ code: roomCode, name: nick, totem });
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

function CrawlerRoom({
  code,
  name,
  totem,
  onLeave,
}: {
  code: string;
  name: string;
  totem: string;
  onLeave: () => void;
}) {
  const { t } = useTranslation();
  const g = useRoom<CrView>(code, name, totem);
  const debug = useStatsFlag(); // F3 toggles a debug overlay (like Bomberman)
  const spriteFor = useHeroSprites(g.view); // walk/facing animation for the board pawns
  const [sel, setSel] = useState<number | null>(null); // selected die index
  const [layRot, setLayRot] = useState(0); // chosen orientation (0..3) while laying a tile
  const v = g.view;
  // Reset the orientation to the first legal one whenever a fresh tile is drawn.
  useEffect(() => {
    setLayRot(g.view?.pending?.options[0]?.rotation ?? 0);
  }, [v?.step, v?.pending?.kind, v?.pending?.options.length]);
  // Flash when your health drops (your totem takes damage).
  const myHp = g.view ? g.view.heroes.find((h) => h.s === g.view!.you)?.health : undefined;
  const hpLost = useLossPulse(myHp);
  const bar = <RoomBar code={code} title={t("gameCrawler")} infoSlug="crawler" gameSlug="crawler" onLeave={onLeave} />;

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
        {me && <CharacterSheet me={me} lost={v.ph === "lost"} />}
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
                  <TilePreview kind={v.pending!.kind} edges={layPreview.edges} rot={layPreview.rotation} size={92} />
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
            <span className="muted-note">{h.totem}{h.unlocked[0] ? " ·★" : ""}{h.unlocked[1] ? "★" : ""}</span>
          </div>
        ))}
      </div>

      {(v.ph === "won" || v.ph === "lost") && (
        <div className="mind-controls panel mind-over">
          <strong>{v.ph === "won" ? "The boss falls. You win! 🎉" : "The dungeon claims you. 💀"}</strong>
          {isHost && <button className="game-btn" onClick={() => g.send({ t: "restart" })}>New run</button>}
        </div>
      )}

      {/* the board */}
      <CrawlerMap
        v={v}
        focus={focus}
        debug={debug}
        spriteFor={spriteFor}
        lay={layActive && layPreview ? { kind: v.pending!.kind, rot: layRot, options: layOpts, place: placeAt } : null}
      />

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
function CharacterSheet({ me, lost }: { me: CrHero; lost: boolean }) {
  const mins = menuMins(me);
  const spec = specialInfo(me.special);
  const unlocks = unlockInfo(me);
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
    </div>
  );
}

// Actions available for the selected die, with their targets inline.
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
  const effArmour = (m: CrMonster) => Math.max(1, m.armour - (barbed ? 1 : 0));
  // Attack targets: living monsters on your tile, plus (for a ranged totem) ones on a
  // tile reachable through an open edge of yours. Only those the selected die can
  // actually hit are offered: attacking with a die below the Armour is not allowed.
  const targets: { tileId: number; m: CrMonster; ranged: boolean }[] = [];
  (myTile?.monsters ?? []).filter((m) => m.alive).forEach((m) => targets.push({ tileId: myTile!.id, m, ranged: false }));
  if (me.ranged && myTile) {
    CR_DELTA.forEach(([dx, dy], d) => {
      if (!myTile.edges[d]) return;
      const nt = view.tiles.find((t) => t.x === myTile.x + dx && t.y === myTile.y + dy);
      nt?.monsters.filter((m) => m.alive).forEach((m) => targets.push({ tileId: nt.id, m, ranged: true }));
    });
  }
  const hittable = targets.filter(({ m }) => die.value >= effArmour(m));

  return (
    <div className="cr-actions">
      <div className="cr-action-row">
        <button className="game-btn ghost" disabled={me.courage < 1} onClick={onReroll}>↻ reroll (1✊)</button>
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
function TilePreview({ kind, edges, rot = 0, size = 56 }: { kind: string; edges: Edges; rot?: number; size?: number }) {
  const art = tileArt(kind, edges, rot);
  if (!art) return <TileGlyph edges={edges} />;
  return (
    <span
      className="cr-tile-prev"
      style={{
        width: size,
        height: size,
        backgroundImage: `url(/sprites/crawler/tiles/${art.file}.png)`,
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
function CrawlerMap({ v, focus, lay, debug, spriteFor }: { v: CrView; focus: { x: number; y: number } | null; lay: LayState | null; debug?: boolean; spriteFor: SpriteFor }) {
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
              <div className="cr-tile-art" style={{ backgroundImage: `url(/sprites/crawler/tiles/${art.file}.png)`, transform: `rotate(${art.deg}deg)` }} />
            ) : (
              <TileGlyph edges={ghost.edges} />
            )}
          </div>,
        );
      } else if (tile) {
        cells.push(<CrawlerTileCell key={key} tile={tile} v={v} focus={isFocus(x, y)} debug={debug} spriteFor={spriteFor} />);
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

function CrawlerTileCell({ tile, v, focus, debug, spriteFor }: { tile: CrTile; v: CrView; focus?: boolean; debug?: boolean; spriteFor: SpriteFor }) {
  const heroesHere = v.heroes.filter((h) => h.tile === tile.id); // includes the fallen (death stance)
  const lost = v.ph === "lost"; // a party wipe or collapse: everyone drops to the defeated frame
  const edgeClass = ["n", "e", "s", "w"].filter((_, d) => tile.edges[d]).map((s) => `open-${s}`).join(" ");
  const art = tileArt(tile.kind, tile.edges, tile.rot);
  return (
    <div
      className={`cr-cell cr-tile ${edgeClass}${tile.isBoss ? " boss" : ""}`}
      data-focus={focus ? "1" : undefined}
    >
      {art && (
        <div
          className="cr-tile-art"
          style={{ backgroundImage: `url(/sprites/crawler/tiles/${art.file}.png)`, transform: `rotate(${art.deg}deg)` }}
        />
      )}
      {debug && <span className="cr-tile-kind">{tileKind(tile.kind).label} ({tile.x},{tile.y})</span>}
      <div className="cr-squares">
        {Array.from({ length: 9 }, (_, sq) => {
          // Each square holds at most one monster; a defeated one stays put in its
          // destroyed frame (no stat badge), a living one shows its stats + Courage reward.
          const monster = tile.monsters.find((m) => m.square === sq);
          const here = heroesHere.filter((h) => h.square === sq);
          return (
            <div key={sq} className="cr-sq">
              {monster && (
                <span className="cr-mon">
                  <MonsterSprite m={monster} />
                  {monster.alive && (
                    <span className="cr-mon-badge" title={`Armour ${monster.armour}, Health ${monster.health}/${monster.maxHP}, Damage ${monster.damage}, grants ${monster.reward} Courage on defeat`}>
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
