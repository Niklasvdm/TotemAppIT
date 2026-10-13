import { useEffect, useState } from "react";
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
import { biomeTileDir } from "./logic";
import type { CrView } from "./types";
import { TOTEMS, DIRS, DIR_ARROW, tileKind, heroStatuses, objInfo, OBJECT_CAP, STATUSES } from "./data";
import { useHeroSprites, ObjIcon } from "./sprites";
import { Die } from "./Die";
import { TilePreview } from "./TilePreview";
import { CharacterSheet } from "./sheet";
import { CrawlerActions } from "./actions";
import { CrawlerShopCallout } from "./shop";
import { CrawlerMap } from "./board";
import { CrawlerDebug } from "./debug";
import { CrawlerLootRoll, CrawlerStatusRoll } from "./banners";

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
  const rotateLay = () => {
    if (layRots.length <= 1) return;
    const i = layRots.indexOf(layRot);
    setLayRot(layRots[(i + 1) % layRots.length]);
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
