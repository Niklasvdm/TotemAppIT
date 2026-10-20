import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router-dom";
import { createGameRoom } from "../../api";
import { CODE_LEN, NICK_MAX, validNick } from "../validate";
import { useRoom } from "../useRoom";
import { usePersistedSession } from "../session";
import { WrongGameNotice } from "../WrongGameNotice";
import { RoomBar } from "../RoomBar";
import { LocalSeats, type LocalPlayer } from "../LocalSeats";
import { LossFlash, useLossPulse } from "../LossFlash";
import { useStatsFlag } from "../bomberman/NetHud";
import type { CrView, CrMonster } from "./types";
import { TOTEMS, objInfo, OBJECTS, menuMins, COURAGE_CAP, attackTargets, effArmour, useItemMsg, lowestDie } from "./data";
import { useHeroSprites, ObjIcon } from "./sprites";
import { CharacterSheet } from "./sheet";
import { CrawlerActionPanel } from "./actionpanel";
import { CrawlerHeroPanel } from "./heropanel";
import { HeroCard } from "./herocard";
import { CrawlerShopCallout } from "./shop";
import { CrawlerMap } from "./board";
import { CrawlerDebug } from "./debug";
import { CrawlerOverlay } from "./overlay";
import { CrawlerLootRoll, CrawlerStatusRoll } from "./banners";

export default function CrawlerPage() {
  const { t } = useTranslation();
  const [params] = useSearchParams();
  const [name, setName] = useState(() => localStorage.getItem("totem-game-name") ?? "");
  // The animal is now picked in the lobby, not here; this is just the initial meta sent on
  // join (the last pick, or a default), which the lobby picker can then change.
  const [totem] = useState(() => localStorage.getItem("totem-crawler-totem") ?? "bever");
  const [code, setCode] = useState(() => (params.get("code") ?? "").toUpperCase());
  // Persisted so a mobile tab reload resumes the game (see usePersistedSession).
  const [session, setSession] = usePersistedSession<{ code: string; players: LocalPlayer[] }>("totem-crawler-session");
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
        <p className="muted-note">You'll pick your animal in the lobby.</p>
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
  const [hlMon, setHlMon] = useState<string | null>(null); // "tileId-monsterId" of the attack button under the cursor, glowed on the board
  const [selMon, setSelMon] = useState<string | null>(null); // "tileId-monsterId" tapped on the board in compact view (shows the ⚔ attack button)
  const [lucky, setLucky] = useState(false); // Lucky Charm armed: the next die clicked gets +1
  const [carryAlly, setCarryAlly] = useState<number | null>(null); // Carry armed: the arrows move this ally
  const armLucky = () => setLucky((x) => !x);
  const pickLucky = (i: number) => { g.send({ t: "lucky", die: i }); setLucky(false); };
  const rerollDie = (i: number) => g.send({ t: "reroll", die: i });
  const [layRot, setLayRot] = useState(0); // chosen orientation (0..3) while laying a tile
  const [startLevel, setStartLevel] = useState(1); // lobby: which level to begin at
  const [devBoss, setDevBoss] = useState(""); // F3 dev: forced boss slug ("" = random)
  const [devLoot, setDevLoot] = useState<string[]>([]); // F3 dev: starting loot slugs
  const [shopDismissedTile, setShopDismissedTile] = useState<number | null>(null); // shop closed by hand; click the stall (its tile id) to re-open
  // Compact (mobile) view: a touch overlay of dice + move d-pad + quick specials floats over
  // the board so you can act without scrolling up to the sheet. The second toggle draws the
  // move d-pad. Both persist so a reload keeps the player's choice. Default: compact off
  // (desktop unchanged), move-buttons on once compact is enabled.
  const [compact, setCompact] = useState(() => localStorage.getItem("totem-crawler-compact") !== "0"); // sidebar layout is the default now
  const [moveBtns, setMoveBtns] = useState(() => localStorage.getItem("totem-crawler-movebtns") !== "0");
  // The character overlay (dice/d-pad/specials on the pawn) and the enemy overlay (tap-to-
  // attack) are INDEPENDENT, each its own toggle, so the character controls never cover the
  // combat button. Tapping an enemy shows only its ⚔ (the character overlay steps aside);
  // tapping your avatar brings the character overlay back.
  const [charOv, setCharOv] = useState(() => localStorage.getItem("totem-crawler-charov") !== "0");
  const [enemyOv, setEnemyOv] = useState(() => localStorage.getItem("totem-crawler-enemyov") !== "0");
  const toggleCompact = () => setCompact((c) => { localStorage.setItem("totem-crawler-compact", c ? "0" : "1"); return !c; });
  const toggleMove = () => setMoveBtns((m) => { localStorage.setItem("totem-crawler-movebtns", m ? "0" : "1"); return !m; });
  const toggleChar = () => setCharOv((c) => { localStorage.setItem("totem-crawler-charov", c ? "0" : "1"); return !c; });
  const toggleEnemies = () => setEnemyOv((e) => { localStorage.setItem("totem-crawler-enemyov", e ? "0" : "1"); if (e) setSelMon(null); return !e; });
  // The left party bar (compact view): collapsible with « / ».
  const [railOpen, setRailOpen] = useState(() => localStorage.getItem("totem-crawler-rail") !== "0");
  const toggleRail = () => setRailOpen((o) => { localStorage.setItem("totem-crawler-rail", o ? "0" : "1"); return !o; });
  const avatarToggleChar = () => { // tap avatar: toggle the character overlay (enabling compact first)
    setSelMon(null);
    if (!compact) {
      setCompact(true); localStorage.setItem("totem-crawler-compact", "1");
      setCharOv(true); localStorage.setItem("totem-crawler-charov", "1");
      return;
    }
    toggleChar();
  };
  const boardRef = useRef<HTMLDivElement>(null); // the board wrap, so the overlay can anchor on the pawn
  const v = g.view;
  // Reset the orientation to the first legal one whenever a fresh tile is drawn.
  useEffect(() => {
    setLayRot(g.view?.pending?.options[0]?.rotation ?? 0);
  }, [v?.step, v?.pending?.kind, v?.pending?.options.length]);
  // Tell the page when the turn lands on THIS seat so local multiplayer auto-switches to
  // the player whose turn it is. We also count this seat's turns so the keyboard hint can
  // show only during a player's first few turns on Level 1.
  const myTurnNow = v?.ph === "playing" && v?.turn === v?.you;
  const [myTurns, setMyTurns] = useState(0);
  useEffect(() => {
    if (myTurnNow) { onMyTurn?.(); setMyTurns((n) => n + 1); }
    else { setLucky(false); setCarryAlly(null); } // drop half-armed Lucky Charm / Carry when the turn passes
  }, [myTurnNow, onMyTurn]);
  // Keyboard controls, a fast twin of the mouse (which still works: select a die for the
  // action menu, use items from your sheet). Each key auto-spends your LOWEST usable die, so
  // you skip the die-select step. Arrows/WASD move (a fast mover's extra steps go through
  // `step`); `c` gains Courage; `f` attacks; number keys use that slot's object. Dice are
  // shown sorted low to high, so the keys eat them lowest-first. Ignored while typing.
  useEffect(() => {
    if (!v) return;
    const DIR: Record<string, number> = { ArrowUp: 0, ArrowRight: 1, ArrowDown: 2, ArrowLeft: 3, w: 0, d: 1, s: 2, a: 3 };
    // The lowest unspent die whose value meets `min` (and an optional predicate), or -1.
    const lowestDie = (min: number, ok?: (val: number) => boolean) => {
      let lo = -1, loVal = 99;
      v.dice.forEach((die, i) => {
        if (!die.spent && die.value >= min && die.value < loVal && (!ok || ok(die.value))) { loVal = die.value; lo = i; }
      });
      return lo;
    };
    const onKey = (e: KeyboardEvent) => {
      const tag = (document.activeElement?.tagName ?? "").toLowerCase();
      if (tag === "input" || tag === "select" || tag === "textarea") return;
      if (v.ph !== "playing" || v.turn !== v.you) return;
      const me = v.heroes.find((h) => h.s === v.you);
      if (!me || me.downed) return;
      const key = e.key.length === 1 ? e.key.toLowerCase() : e.key;
      // LAY phase: Enter confirms the previewed placement, r rotates the tile.
      if (v.step === "lay" && v.pending) {
        const opts = v.pending.options;
        const rots = Array.from(new Set(opts.map((o) => o.rotation))).sort((a, b) => a - b);
        if (key === "Enter") {
          e.preventDefault();
          const chosen = opts.find((o) => o.rotation === layRot) ?? opts[0];
          if (chosen) g.send({ t: "layTile", x: chosen.x, y: chosen.y, rotation: chosen.rotation });
        } else if (key === "r" && rots.length > 1) {
          e.preventDefault();
          const i = rots.indexOf(layRot);
          setLayRot(rots[(i + 1) % rots.length]);
        }
        return;
      }
      if (v.step !== "spend") return;
      // End turn (e), clear the selected die (Escape), revive a downed ally on your tile (v).
      if (key === "e") { e.preventDefault(); g.send({ t: "endTurn" }); setSel(null); return; }
      if (key === "Escape") { setSel(null); return; }
      if (key === "v") {
        const downed = v.heroes.find((h) => h.downed && h.tile === me.tile && h.s !== me.s);
        if (downed && v.dice.some((d) => !d.spent)) { e.preventDefault(); g.send({ t: "revive", ally: downed.s }); }
        return;
      }
      const mins = menuMins(me);
      // Move: arrows / WASD.
      const dir = DIR[key];
      if (dir !== undefined) {
        e.preventDefault();
        if (v.bonusMoves > 0) { g.send({ t: "step", dir }); return; }
        const lo = lowestDie(mins.move);
        if (lo >= 0) { g.send({ t: "spend", die: lo, action: "move", dir }); setSel(null); }
        return;
      }
      // Courage: c (lowest die meeting the Courage minimum, if not already maxed).
      if (key === "c") {
        e.preventDefault();
        if (me.courage >= COURAGE_CAP) return;
        const lo = lowestDie(mins.courage);
        if (lo >= 0) { g.send({ t: "spend", die: lo, action: "courage" }); setSel(null); }
        return;
      }
      // Fight: f. The lowest die that can beat some enemy's Armour attacks the lowest-Health
      // enemy that die can hit (finish wounded robots first).
      if (key === "f") {
        e.preventDefault();
        const targets = attackTargets(v, me);
        const lo = lowestDie(1, (val) => targets.some((t) => val >= effArmour(t.m)));
        if (lo < 0) return;
        const val = v.dice[lo].value;
        const hit = targets.filter((t) => val >= effArmour(t.m));
        if (!hit.length) return;
        const tgt = hit.reduce((best, t) => (t.m.health < best.m.health ? t : best));
        g.send({ t: "spend", die: lo, action: "attack", monster: tgt.m.id, tile: tgt.tileId });
        setSel(null);
        return;
      }
      // Items: 1-9 use that pack slot's object (auto-targeted, same as the sheet buttons).
      if (key >= "1" && key <= "9") {
        const slug = me.objects[Number(key) - 1];
        if (!slug) return;
        const msg = useItemMsg(v, me, slug, sel);
        if (msg) { e.preventDefault(); g.send(msg); setSel(null); }
        return;
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [v, g, sel, layRot]);
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
          <p className="muted-note">{g.closeReason ?? "Connection lost."}</p>
          <div className="cr-action-row">
            <button className="game-btn" onClick={() => g.reconnect()}>Reconnect</button>
            <button className="game-btn" onClick={onLeave}>{t("gameBack")}</button>
          </div>
        </div>
      </div>
    );
  }
  if (!v) {
    return (
      <div className="game-room">
        {bar}
        <p className="muted-note" style={{ padding: "1rem" }}>
          {g.status === "reconnecting" ? `Reconnecting…${g.closeReason ? ` (${g.closeReason})` : ""}` : t("loading")}
        </p>
      </div>
    );
  }

  const isHost = g.you >= 0 && g.you === g.host;
  const me = v.heroes.find((h) => h.s === v.you);
  const myTurn = v.ph === "playing" && v.turn === v.you;
  const nameOf = (s: number) => v.heroes.find((h) => h.s === s)?.n ?? "?";

  // Tap-to-attack (compact view): tapping a monster on the board selects it and shows a ⚔
  // button; the ⚔ spends your lowest die that can beat its (effective) Armour. Mirrors the
  // keyboard 'f' logic but aimed at the one monster you picked. canHit gates the sword.
  const canHitMon = (m: CrMonster) => lowestDie(v.dice, 1, (val) => val >= effArmour(m)) >= 0;
  const attackMon = (tileId: number, m: CrMonster) => {
    const lo = lowestDie(v.dice, 1, (val) => val >= effArmour(m));
    if (lo < 0) return;
    g.send({ t: "spend", die: lo, action: "attack", monster: m.id, tile: tileId });
    // keep the monster selected so the ⚔ stays up for repeat hits; it auto-clears when it dies
    // (selMonValid) — so you fight the same enemy until it drops.
    setSel(null);
  };
  // Only keep a selection that still points at a living monster (it dies, you deselect it).
  const selMonValid = !!selMon && v.tiles.some((ti) => ti.monsters.some((m) => m.alive && `${ti.id}-${m.id}` === selMon));
  const touchCombat = compact && enemyOv && myTurn && v.step === "spend" && !!me && !me.downed;
  // Enemies the active hero can actually reach this turn (their tile + ranged-adjacent),
  // keyed "tileId-monsterId" — the board only lets you tap these, not a monster across the map.
  const reachableMon = me ? new Set(attackTargets(v, me).map(({ tileId, m }) => `${tileId}-${m.id}`)) : new Set<string>();
  const combat = touchCombat
    ? {
        sel: selMonValid ? selMon : null,
        onSelect: (key: string) => setSelMon((k) => (k === key ? null : key)),
        onAttack: attackMon,
        canHit: canHitMon,
        reachable: reachableMon,
      }
    : null;

  // Tap an ally pawn on your tile: revive a downed ally, Brace/Mend a standing one, or begin a
  // Carry (then the arrows move them). Mirrors the enemy tap-to-attack.
  const myTileNow = me ? v.tiles.find((ti) => ti.id === me.tile) : undefined;
  const allyAct = (h: typeof v.heroes[number]): "revive" | "brace" | "mend" | "carry" | null => {
    if (!me || !myTurn || v.step !== "spend" || me.downed) return null;
    if (h.s === me.s || h.tile !== me.tile) return null;
    if (h.downed) return "revive";
    if (!h.alive) return null;
    const sp = me.special;
    if (sp === "brace" || sp === "guard") return "brace";
    if (sp === "mend") return "mend";
    if (sp === "carry") return "carry";
    return null;
  };
  const tapAlly = (h: typeof v.heroes[number]) => {
    if (!me) return;
    const act = allyAct(h);
    if (!act) return;
    if (act === "revive") { if (v.dice.some((d) => !d.spent)) g.send({ t: "revive", ally: h.s }); return; }
    if (act === "carry") { setCarryAlly((c) => (c === h.s ? null : h.s)); return; }
    const sp = me.special;
    const lo = lowestDie(v.dice, menuMins(me)[sp] ?? 99);
    if (lo >= 0) g.send({ t: "spend", die: lo, action: sp, ally: h.s });
  };
  // keep carry armed only while the carried ally is still a valid target on your tile
  const carryValid = carryAlly != null && !!me && v.heroes.some((h) => h.s === carryAlly && h.alive && !h.downed && h.tile === me.tile);
  const ally = compact && myTurn && v.step === "spend" && !!me && !me.downed
    ? { act: allyAct, onTap: tapAlly, carrying: carryValid ? carryAlly : null }
    : null;

  // Snatch (Raccoon): ground loot on an orthogonally-adjacent tile glows; a click pulls it in.
  const snatch = compact && myTurn && v.step === "spend" && !!me && !me.downed && me.special === "pilfer" && !!myTileNow && v.dice.some((d) => !d.spent)
    ? {
        hx: myTileNow.x,
        hy: myTileNow.y,
        onSnatch: (dir: number) => { const lo = lowestDie(v.dice, menuMins(me)["pilfer"] ?? 2); if (lo >= 0) g.send({ t: "spend", die: lo, action: "pilfer", dir }); },
      }
    : null;

  // Tile-laying: the options span every legal frontier cell, each with the rotations that
  // fit there. `layRot` is the orientation the player has chosen; it is kept across cells.
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
  const rotateLay = () => rotateLayBy(1);
  // rotateLayBy steps the chosen orientation forward (+1) or back (-1) through the legal
  // rotations. Used by the Rotate button, the scroll wheel, and the r key.
  const rotateLayBy = (delta: number) => {
    if (layRots.length <= 1) return;
    const i = Math.max(0, layRots.indexOf(layRot));
    setLayRot(layRots[(i + delta + layRots.length) % layRots.length]);
  };
  // Where to keep the board scrolled: a move-triggered lay centres the one cell you
  // stepped onto; a proactive LAY-phase lay and a normal turn centre on your own tile.
  const myTile = me ? v.tiles.find((ti) => ti.id === me.tile) : undefined;
  // Portals: a hero on a portal tile's centre may teleport to any other portal.
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
          <div className="cr-lobby-row">
            <label className="cr-field-label" htmlFor="cr-totem">Your animal</label>
            <select
              id="cr-totem"
              className="trait-search"
              value={v.heroes.find((h) => h.s === v.you)?.totem ?? "bever"}
              onChange={(e) => {
                g.send({ t: "totem", totem: e.target.value });
                localStorage.setItem("totem-crawler-totem", e.target.value);
              }}
            >
              {TOTEMS.map((x) => (
                <option key={x.slug} value={x.slug}>{x.label}</option>
              ))}
            </select>
          </div>
          {isHost ? (
            <>
              <div className="cr-lobby-row">
                <label className="cr-field-label" htmlFor="cr-start-level">Start at level</label>
                <select
                  id="cr-start-level"
                  className="trait-search cr-level-select"
                  value={startLevel}
                  onChange={(e) => setStartLevel(Number(e.target.value))}
                >
                  {[1, 2, 3, 4, 5, 6].map((n) => <option key={n} value={n}>Level {n}</option>)}
                </select>
              </div>
              {debug && (
                <div className="panel cr-dev">
                  <strong>🛠 Dev (F3)</strong>
                  <div className="cr-lobby-row">
                    <label className="cr-field-label" htmlFor="cr-dev-boss">Boss</label>
                    <select id="cr-dev-boss" className="trait-search" value={devBoss} onChange={(e) => setDevBoss(e.target.value)}>
                      <option value="">(random for the level)</option>
                      {v.bossPool.map((b) => (
                        <option key={b.slug} value={b.slug}>{b.name} (L{b.minLvl}{b.maxLvl !== b.minLvl ? `–${b.maxLvl}` : ""})</option>
                      ))}
                    </select>
                  </div>
                  <div className="cr-dev-loot">
                    <span className="muted-note">Starting loot (given to every hero):</span>
                    <div className="cr-dev-loot-grid">
                      {Object.keys(OBJECTS).map((slug) => (
                        <label key={slug} className="cr-dev-loot-item" title={objInfo(slug).effect}>
                          <input
                            type="checkbox"
                            checked={devLoot.includes(slug)}
                            onChange={(e) => setDevLoot((xs) => e.target.checked ? [...xs, slug] : xs.filter((s) => s !== slug))}
                          />
                          <ObjIcon slug={slug} size={16} /> {objInfo(slug).name}
                        </label>
                      ))}
                    </div>
                  </div>
                </div>
              )}
              <button
                className="game-btn"
                onClick={() => g.send({ t: "start", level: startLevel, boss: debug ? devBoss : "", loot: debug ? devLoot : [], debug })}
              >
                Start run{startLevel !== 1 ? ` (Level ${startLevel})` : ""}
              </button>
            </>
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

  // The per-turn action column, shared by the classic top row and the compact rail's expanded
  // block (built once so the two cannot drift).
  const actionPanel = (
    <CrawlerActionPanel
      v={v} me={me} myTurn={myTurn} sel={sel} setSel={setSel} send={(msg) => g.send(msg)} setHlMon={setHlMon}
      layActive={layActive} layPreview={layPreview} isPhaseLay={isPhaseLay} layRots={layRots}
      rotateLay={rotateLay} rotateLayBy={rotateLayBy}
      onPortalCenter={onPortalCenter} portalDests={portalDests} portalNo={portalNo} nameOf={nameOf}
    />
  );

  return (
    <div className={`game-room cr-room${compact ? " compact" : ""}`}>
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

      {/* classic top row: character sheet (3/4) beside the dice + actions (1/4). Hidden in
          compact view, where the left rail is its replacement. */}
      {!compact && (
        <div className="cr-top-row">
          {me && <CharacterSheet me={me} lost={v.ph === "lost"} view={v} myTurn={myTurn} selDie={sel} onSend={(msg) => { g.send(msg); }} />}
          <div className="panel cr-action-col">{actionPanel}</div>
        </div>
      )}

      {/* classic party list (the compact view folds this into the left rail) */}
      {!compact && (
        <div className="cr-party">
          {v.heroes.map((h) => (
            <HeroCard key={h.s} h={h} you={v.you} turn={v.turn} />
          ))}
        </div>
      )}

      {(v.ph === "won" || v.ph === "lost") && (
        <div className="mind-controls panel mind-over">
          <strong>{v.ph === "won" ? "The boss falls. You win! 🎉" : "The dungeon claims you. 💀"}</strong>
          {isHost && <button className="game-btn" onClick={() => g.send({ t: "restart" })}>New run</button>}
        </div>
      )}

      {/* keyboard hint: only while learning (a player's first 3 turns, Level 1), above the
          board. The mouse still works (select a die for the action menu, use items from your
          sheet); these keys are the quick auto-lowest-die twin. */}
      {myTurn && v.level === 1 && myTurns <= 3 && (
        <div className="cr-kbd-hint panel" role="note">
          <span className="cr-kbd-hint-title">⌨ Shortcuts</span>
          <span><kbd>←</kbd><kbd>↑</kbd><kbd>↓</kbd><kbd>→</kbd> or <kbd>W</kbd><kbd>A</kbd><kbd>S</kbd><kbd>D</kbd> move</span>
          <span><kbd>C</kbd> Courage</span>
          <span><kbd>F</kbd> Fight</span>
          <span><kbd>1</kbd>–<kbd>9</kbd> use item</span>
          <span><kbd>E</kbd> end turn</span>
          <span><kbd>V</kbd> revive a downed ally</span>
          <span className="cr-kbd-hint-sub">laying: <kbd>scroll</kbd>/<kbd>R</kbd> rotate · <kbd>Enter</kbd> place</span>
          <span className="muted-note">each spends your lowest usable die — or click a die / your items as usual</span>
        </div>
      )}

      {/* compact view splits the play area: the left party rail, then the board on the right */}
      <div className={`cr-play-area${compact ? " compact" : ""}`}>
        {compact && (railOpen ? (
          <aside className="cr-rail panel">
            <div className="cr-rail-head">
              <strong>Party</strong>
              <div className="cr-rail-toggles" role="group" aria-label="view controls">
                <button className={`cr-rail-tog on`} onClick={toggleCompact} title="Leave compact view" aria-label="Leave compact view">📱</button>
                <button className={`cr-rail-tog${charOv ? " on" : ""}`} onClick={toggleChar} title="Character overlay on your hero" aria-pressed={charOv}>🧍</button>
                <button className={`cr-rail-tog${enemyOv ? " on" : ""}`} onClick={toggleEnemies} title="Enemy overlay: tap a monster to show its ⚔" aria-pressed={enemyOv}>⚔</button>
                {charOv && <button className={`cr-rail-tog${moveBtns ? " on" : ""}`} onClick={toggleMove} title="Move d-pad around your hero" aria-pressed={moveBtns}>✛</button>}
                <button className="cr-rail-tog" onClick={toggleRail} title="Collapse the party bar" aria-label="Collapse the party bar">«</button>
              </div>
            </div>
            {/* your block: the redesigned panel with controls on your turn, a card otherwise */}
            {me && (myTurn ? (
              <CrawlerHeroPanel
                v={v} me={me} lost={v.ph === "lost"} onSend={(msg) => { g.send(msg); setSel(null); }}
                layNode={actionPanel}
                onPortalCenter={onPortalCenter} portalDests={portalDests} portalNo={portalNo}
                lucky={lucky} onArmLucky={armLucky} onPickLucky={pickLucky} onReroll={rerollDie}
                carry={carryValid ? carryAlly : null}
              />
            ) : (
              <HeroCard h={me} you={v.you} turn={v.turn} />
            ))}
            {/* teammates, so you can see what they hold and whose turn it is */}
            <div className="cr-rail-mates">
              {v.heroes.filter((h) => h.s !== v.you).map((h) => (
                <HeroCard key={h.s} h={h} you={v.you} turn={v.turn} />
              ))}
            </div>
          </aside>
        ) : (
          <button className="cr-rail-expand" onClick={toggleRail} title="Show the party bar" aria-label="Show the party bar">»</button>
        ))}
      {/* the board, with the shop as an overlay callout when you stand on a shop tile */}
      <div className={`cr-board-wrap${compact ? " compact" : ""}`} ref={boardRef}>
        {/* classic view: a single emoji to enter compact (the rest of the toggles live in the
            compact rail header). In compact, tapping your avatar re-toggles the hero overlay. */}
        {!compact && (
          <div className="cr-view-toggles" role="group" aria-label="view controls">
            <button className="cr-view-toggle emoji" onClick={toggleCompact} title="Compact (mobile) view" aria-label="Compact view">📱</button>
          </div>
        )}
        {/* character overlay: steps aside while an enemy is selected so its ⚔ is never covered */}
        {compact && charOv && !selMonValid && myTurn && me && !me.downed && v.step === "spend" && !layActive && (
          <CrawlerOverlay v={v} me={me} showMove={moveBtns} boardRef={boardRef} onSend={(msg) => { g.send(msg); setSel(null); }} lucky={lucky} onArmLucky={armLucky} onPickLucky={pickLucky} onReroll={rerollDie} carry={carryValid ? carryAlly : null} onPortalCenter={onPortalCenter} portalDests={portalDests} portalNo={portalNo} onPortal={(id) => g.send({ t: "portal", tile: id })} />
        )}
        <CrawlerMap
          v={v}
          focus={focus}
          debug={debug}
          spriteFor={spriteFor}
          highlight={hlMon}
          combat={combat}
          ally={ally}
          snatch={snatch}
          onAvatar={avatarToggleChar}
          onPickup={(sq) => g.send({ t: "pickup", square: sq })}
          onReopenShop={(id) => setShopDismissedTile((d) => (d === id ? null : d))}
          lay={layActive && layPreview ? { kind: v.pending!.kind, rot: layRot, options: layOpts, place: placeAt, rotate: rotateLayBy } : null}
        />
        {/* The shop is an overlay over the board, so it must step aside while you lay a tile
            (otherwise, on a phone, it covers the lay cells and you can neither place nor buy).
            It comes back on its own once the tile is down; or click the stall to re-open sooner. */}
        {me && myTurn && !layActive && myTile?.kind === "shop" && shopDismissedTile !== myTile.id && (
          <CrawlerShopCallout me={me} shop={v.shopStock} onSend={(msg) => g.send(msg)} onClose={() => setShopDismissedTile(myTile.id)} />
        )}
        {v.lootRoll && <CrawlerLootRoll key={`lr-${v.lootRoll.roll}-${v.lootRoll.chance}-${v.lootRoll.killer ?? ""}`} roll={v.lootRoll} />}
        {v.statusRoll && <CrawlerStatusRoll key={`sr-${v.statusRoll.roll}-${v.statusRoll.status}-${v.statusRoll.target ?? ""}`} roll={v.statusRoll} />}
        </div>
      </div>

      {debug && <CrawlerDebug v={v} conn={{ status: g.status, retries: g.retries, seats: g.roster.length, you: g.you, host: g.host }} />}

      {/* the game log */}
      {v.log.length > 0 && (
        <div className="cr-log panel">
          {v.log.slice(-8).map((l, i) => <div key={i}>{l}</div>)}
        </div>
      )}
    </div>
  );
}
