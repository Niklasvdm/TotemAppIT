import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router-dom";
import { createGameRoom } from "../../api";
import { CODE_LEN, NICK_MAX, validNick } from "../validate";
import { useRoom } from "../useRoom";
import { RoomBar } from "../RoomBar";

// A roster of totems to play as. Only the Beaver has a distinct sheet in V0
// (Brace); the rest share the generic sheet. meta = the slug sent on join. Every
// slug here has sprites in the Bomberman atlas (see SPRITE_ROWS).
const TOTEMS = [
  { slug: "bever", label: "Beaver 🦫 (Guardian)" },
  { slug: "wasbeer", label: "Raccoon" },
  { slug: "eekhoorn", label: "Squirrel" },
  { slug: "ezel", label: "Donkey" },
  { slug: "hondshaai", label: "Houndshark" },
  { slug: "spinaap", label: "Spider monkey" },
  { slug: "goudvink", label: "Bullfinch" },
  { slug: "wouw", label: "Kite" },
  { slug: "comorenwever", label: "Comoros weaver" },
  { slug: "pijlinktvis", label: "Arrow squid" },
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

// TotemSprite draws one totem's front frame from the shared atlas via CSS
// background positioning. Falls back to a compass emoji for unsprited totems.
function TotemSprite({ slug, size = 40 }: { slug: string; size?: number }) {
  const row = SPRITE_ROWS.indexOf(slug);
  if (row < 0) return <span style={{ fontSize: size * 0.8 }}>🧭</span>;
  return (
    <span
      className="cr-sprite"
      style={{
        width: size,
        height: size,
        backgroundImage: "url(/sprites/totems.png)",
        backgroundSize: `${ATLAS_COLS * size}px ${ATLAS_ROWS * size}px`,
        backgroundPosition: `0px ${-row * size}px`,
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
  gone?: boolean;
}
interface CrMonster {
  id: number;
  armour: number;
  health: number;
  maxHP: number;
  damage: number;
  square: number;
  engaged: boolean;
  alive: boolean;
  isBoss: boolean;
}
interface CrTile {
  id: number;
  x: number;
  y: number;
  kind: string;
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
  pending: { kind: string; entryDir: number; options: CrLayOption[] } | null;
  deckLeft: number;
  bands: number;
  bossNear: boolean;
  log: string[];
}

// The minimum die value each action needs, derived from the hero's sheet.
function menuMins(special: string): Record<string, number> {
  return { move: 1, attack: 4, courage: 5, [special]: special === "brace" ? 2 : 3 };
}

// The Courage thresholds that unlock the two specials; the bar fills toward the
// second (5), which is the meaningful ceiling (both specials unlockable).
const COURAGE_UNLOCKS: [number, number] = [3, 5];
const COURAGE_BAR_MAX = 5;

// Human labels and one-line explanations for the tile shapes (kind on the wire).
const TILE_KINDS: Record<string, { label: string; blurb: string }> = {
  straight: { label: "Straight", blurb: "A corridor: two doorways on opposite sides." },
  bend: { label: "Bend", blurb: "A corner: two doorways on adjacent sides." },
  tee: { label: "T-junction", blurb: "Three doorways." },
  cross: { label: "Crossroads", blurb: "Open on all four sides." },
  "dead-end": { label: "Dead end", blurb: "A single doorway: caps a branch." },
  boss: { label: "Boss lair", blurb: "The boss waits here. One doorway in." },
};
function tileKind(kind: string) {
  return TILE_KINDS[kind] ?? { label: kind || "Tile", blurb: "" };
}

// What each special does, keyed by the sheet (Beaver vs generic). Shown in the
// info hover on the character sheet so players know before spending Courage.
function specialInfo(special: string): { name: string; blurb: string } {
  return special === "brace"
    ? { name: "Brace", blurb: "Spend a die (≥2) to give 2 Shields to yourself or an ally on your tile." }
    : { name: "Guard", blurb: "Spend a die (≥3) to give yourself +1 Shield." };
}
function unlockInfo(special: string): [{ name: string; blurb: string }, { name: string; blurb: string }] {
  return special === "brace"
    ? [
        { name: "Thick Hide", blurb: "Passive: start every turn with +2 Shields on yourself." },
        { name: "Bulwark", blurb: "Passive: when an ally on your tile would take Damage, you may take it instead, reduced by your Shields first." },
      ]
    : [
        { name: "Resolve", blurb: "A self-buff unlocked with Courage." },
        { name: "Second Wind", blurb: "A stronger self-buff unlocked with Courage." },
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
  const [sel, setSel] = useState<number | null>(null); // selected die index
  const v = g.view;
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

  return (
    <div className="game-room cr-room">
      {bar}
      <div className="mind-hud panel">
        <span>🂠 {v.deckLeft} tiles{v.bossNear ? " · boss near!" : ""}</span>
        <span>{v.ph === "playing" ? (myTurn ? "Your turn" : `${nameOf(v.turn)}'s turn`) : v.ph === "won" ? "Victory 🎉" : "Defeat 💀"}</span>
      </div>

      {/* everything about your character, up top: sheet (sprite, HP + Courage
          bars, menu, specials), then the party, then the dice + turn controls. */}
      {me && <CharacterSheet me={me} />}

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

      {/* lay a tile */}
      {v.ph === "playing" && v.step === "lay" && v.pending && (
        <div className="panel cr-lay">
          <strong>
            {myTurn ? "Lay the new tile. Pick an orientation:" : `${nameOf(v.turn)} is laying a tile…`}
          </strong>
          <span className="cr-lay-kind">
            <b>{tileKind(v.pending.kind).label}</b>
            <span className="muted-note"> {tileKind(v.pending.kind).blurb}</span>
          </span>
          {myTurn && (
            <div className="cr-lay-options">
              {v.pending.options.map((o, i) => (
                <button key={i} className="cr-lay-opt" onClick={() => g.send({ t: "layTile", x: o.x, y: o.y, rotation: o.rotation })}>
                  <TileGlyph edges={o.edges} />
                  <small>rot {o.rotation * 90}°</small>
                </button>
              ))}
            </div>
          )}
        </div>
      )}

      {/* turn controls */}
      {v.ph === "playing" && v.step === "spend" && myTurn && me && (
        <div className="panel cr-controls">
          <div className="cr-dice">
            {v.dice.map((d, i) => (
              <button
                key={i}
                className={`cr-die${d.spent ? " spent" : ""}${sel === i ? " sel" : ""}`}
                disabled={d.spent}
                onClick={() => setSel(sel === i ? null : i)}
                title={d.spent ? d.usedFor : "select"}
              >
                {d.value}
                {d.spent && <small>{d.usedFor[0]}</small>}
              </button>
            ))}
          </div>

          {sel != null && !v.dice[sel].spent && (
            <CrawlerActions
              die={v.dice[sel]}
              me={me}
              view={v}
              onAct={(msg) => { g.send(msg); setSel(null); }}
              onReroll={() => { g.send({ t: "reroll", die: sel }); }}
            />
          )}

          <div className="cr-turn-actions">
            {!me.unlocked[0] && <button className="game-btn ghost" disabled={me.courage < 3} onClick={() => g.send({ t: "unlock", which: 0 })}>Unlock 1st special (3✊)</button>}
            {me.unlocked[0] && !me.unlocked[1] && <button className="game-btn ghost" disabled={me.courage < 5} onClick={() => g.send({ t: "unlock", which: 1 })}>Unlock 2nd special (5✊)</button>}
            <button className="game-btn" onClick={() => { g.send({ t: "endTurn" }); setSel(null); }}>End turn</button>
          </div>
        </div>
      )}

      {v.ph === "playing" && !myTurn && v.step === "spend" && (
        <p className="muted-note wl-wait">Waiting for {nameOf(v.turn)}…</p>
      )}

      {(v.ph === "won" || v.ph === "lost") && (
        <div className="mind-controls panel mind-over">
          <strong>{v.ph === "won" ? "The boss falls. You win! 🎉" : "The dungeon claims you. 💀"}</strong>
          {isHost && <button className="game-btn" onClick={() => g.send({ t: "restart" })}>New run</button>}
        </div>
      )}

      {/* the board */}
      <CrawlerMap v={v} />

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
function CharacterSheet({ me }: { me: CrHero }) {
  const mins = menuMins(me.special);
  const spec = specialInfo(me.special);
  const unlocks = unlockInfo(me.special);
  const hpPct = Math.max(0, Math.min(100, Math.round((me.health / me.maxHP) * 100)));
  const couPct = Math.max(0, Math.min(100, Math.round((me.courage / COURAGE_BAR_MAX) * 100)));
  // A special's label turns blue once Courage reaches its unlock cost (and it is
  // not yet unlocked), so "you can unlock this now" is unmissable.
  const specClass = (i: 0 | 1) =>
    me.unlocked[i] ? "on" : me.courage >= COURAGE_UNLOCKS[i] && (i === 0 || me.unlocked[0]) ? "ready" : "";
  return (
    <div className="panel cr-sheet">
      <div className="cr-sheet-head">
        <TotemSprite slug={me.totem} size={56} />
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
            <span className="cr-bar-label">✊ {me.courage}</span>
            <div className="cr-bar cr-courbar"><span style={{ width: `${couPct}%` }} /></div>
          </div>
          <span className="muted-note">🛡 {me.shields} shields</span>
        </div>
      </div>
      <div className="cr-menu">
        <span className="cr-menu-item">Move <b>≥{mins.move}</b></span>
        <span className="cr-menu-item">{spec.name} <b>≥{mins[me.special]}</b> <InfoDot text={spec.blurb} /></span>
        <span className="cr-menu-item">Attack <b>≥{mins.attack}</b></span>
        <span className="cr-menu-item">Courage <b>≥{mins.courage}</b></span>
      </div>
      <div className="cr-specials">
        <span className={specClass(0)}>
          ★ {unlocks[0].name} {me.unlocked[0] ? "✓" : "(3✊)"} <InfoDot text={unlocks[0].blurb} />
        </span>
        <span className={specClass(1)}>
          ★ {unlocks[1].name} {me.unlocked[1] ? "✓" : "(5✊)"} <InfoDot text={unlocks[1].blurb} />
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
  const mins = menuMins(me.special);
  const can = (action: string) => die.value >= (mins[action] ?? 99);
  const myTile = view.tiles.find((x) => x.id === me.tile);
  const monsters = (myTile?.monsters ?? []).filter((m) => m.alive);
  const allies = view.heroes.filter((h) => h.alive && h.tile === me.tile);

  return (
    <div className="cr-actions">
      <div className="cr-action-row">
        <button className="game-btn ghost" disabled={me.courage < 1} onClick={onReroll}>↻ reroll (1✊)</button>
      </div>
      {can("move") && (
        <div className="cr-action-row">
          <span>Move:</span>
          {DIRS.map((_, d) => (
            <button key={d} className="game-btn" onClick={() => onAct({ t: "spend", die: dieIdx(view, die), action: "move", dir: d })}>
              {DIR_ARROW[d]}
            </button>
          ))}
        </div>
      )}
      {can("attack") && monsters.length > 0 && (
        <div className="cr-action-row">
          <span>Attack:</span>
          {monsters.map((m) => (
            <button key={m.id} className="game-btn" onClick={() => onAct({ t: "spend", die: dieIdx(view, die), action: "attack", monster: m.id })}>
              {m.isBoss ? "💀" : "👹"} A{m.armour} · ❤{m.health}/{m.maxHP}
            </button>
          ))}
        </div>
      )}
      {can(me.special) && (
        <div className="cr-action-row">
          <span>{me.special === "brace" ? "Brace:" : "Guard:"}</span>
          {me.special === "brace" ? (
            allies.map((a) => (
              <button key={a.s} className="game-btn" onClick={() => onAct({ t: "spend", die: dieIdx(view, die), action: "brace", ally: a.s })}>
                {a.s === me.s ? "self" : a.n}
              </button>
            ))
          ) : (
            <button className="game-btn" onClick={() => onAct({ t: "spend", die: dieIdx(view, die), action: "guard", ally: me.s })}>self (+1🛡)</button>
          )}
        </div>
      )}
      {can("courage") && (
        <div className="cr-action-row">
          <button className="game-btn" onClick={() => onAct({ t: "spend", die: dieIdx(view, die), action: "courage" })}>✊ Courage +1</button>
        </div>
      )}
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

// CrawlerMap lays out placed tiles (and frontier markers) on a grid.
function CrawlerMap({ v }: { v: CrView }) {
  const coords = [...v.tiles.map((t) => [t.x, t.y]), ...v.frontiers.map((f) => [f.x, f.y])];
  if (coords.length === 0) return null;
  const xs = coords.map((c) => c[0]);
  const ys = coords.map((c) => c[1]);
  const minX = Math.min(...xs), maxX = Math.max(...xs);
  const minY = Math.min(...ys), maxY = Math.max(...ys);
  const cols = maxX - minX + 1;
  const tileAt = (x: number, y: number) => v.tiles.find((t) => t.x === x && t.y === y);
  const frontierAt = (x: number, y: number) => v.frontiers.some((f) => f.x === x && f.y === y);

  const cells = [];
  for (let y = minY; y <= maxY; y++) {
    for (let x = minX; x <= maxX; x++) {
      const tile = tileAt(x, y);
      if (tile) {
        cells.push(<CrawlerTileCell key={`${x},${y}`} tile={tile} v={v} />);
      } else if (frontierAt(x, y)) {
        cells.push(<div key={`${x},${y}`} className="cr-cell cr-frontier">?</div>);
      } else {
        cells.push(<div key={`${x},${y}`} className="cr-cell cr-empty" />);
      }
    }
  }
  return (
    <div className="cr-map" style={{ gridTemplateColumns: `repeat(${cols}, 1fr)` }}>
      {cells}
    </div>
  );
}

function CrawlerTileCell({ tile, v }: { tile: CrTile; v: CrView }) {
  const heroesHere = v.heroes.filter((h) => h.tile === tile.id && h.alive);
  const edgeClass = ["n", "e", "s", "w"].filter((_, d) => tile.edges[d]).map((s) => `open-${s}`).join(" ");
  return (
    <div
      className={`cr-cell cr-tile ${edgeClass}${tile.isBoss ? " boss" : ""}`}
      title={`${tileKind(tile.kind).label}: ${tileKind(tile.kind).blurb}`}
    >
      <span className="cr-tile-kind">{tileKind(tile.kind).label}</span>
      <div className="cr-squares">
        {Array.from({ length: 9 }, (_, sq) => {
          const monster = tile.monsters.find((m) => m.alive && m.square === sq);
          const here = heroesHere.filter((h) => h.square === sq);
          return (
            <div key={sq} className="cr-sq">
              {monster && (
                <span className="cr-mon">
                  {monster.isBoss ? "💀" : "👹"}
                  <span className="cr-mon-stats">
                    {monster.isBoss ? "BOSS · " : ""}Armour {monster.armour} · ❤ {monster.health}/{monster.maxHP} · Damage {monster.damage}
                    {monster.engaged ? " · engaged" : ""}
                  </span>
                </span>
              )}
              {here.map((h) => (
                <span key={h.s} className={`cr-pawn${h.s === v.turn ? " active" : ""}${h.s === v.you ? " you" : ""}`} title={h.n}>
                  <TotemSprite slug={h.totem} size={18} />
                </span>
              ))}
            </div>
          );
        })}
      </div>
    </div>
  );
}
