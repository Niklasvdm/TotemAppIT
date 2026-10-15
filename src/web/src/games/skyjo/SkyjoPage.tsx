import { useEffect, useState, type CSSProperties } from "react";
import { usePersistedSession } from "../session";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router-dom";
import { createGameRoom } from "../../api";
import { CODE_LEN, NICK_MAX, validNick } from "../validate";
import { useRoom } from "../useRoom";
import { WrongGameNotice } from "../WrongGameNotice";
import { RoomBar } from "../RoomBar";
import { LocalSeats, NameAddForm, type LocalPlayer } from "../LocalSeats";

const SK_NONE = -100; // matches skNone on the server ("no card in hand")
const SK_COLS = 4;

interface SkCell {
  v: number;
  up: boolean;
  gone: boolean;
}
interface SkPlayer {
  s: number;
  n: string;
  score: number;
  grid: SkCell[];
  flips: number;
  gone?: boolean;
}
interface SkView {
  t: "state";
  ph: "lobby" | "setup" | "play" | "roundEnd" | "over";
  turn: number;
  you: number;
  players: SkPlayer[];
  deckLeft: number;
  discardTop: number;
  hasDiscard: boolean;
  drawn: number;
  fromDeck: boolean;
  mustFlip: boolean;
  closer: number;
  endScore: number;
  gameWinner: number;
  logs: string[];
}

// The colour band of a Skyjo card by value: dark blue negatives, grey 0, green low,
// yellow mid, red high. Keeps the grid readable at a glance (low = good, high = bad).
function cardColor(v: number): { bg: string; ink: string } {
  if (v <= -1) return { bg: "#3f5fd0", ink: "#fff" };
  if (v === 0) return { bg: "#9aa0ab", ink: "#1a1a1a" };
  if (v <= 4) return { bg: "#4caf72", ink: "#08240f" };
  if (v <= 8) return { bg: "#e7c14b", ink: "#2a2205" };
  return { bg: "#e1495c", ink: "#fff" };
}

// chip is an inline style for a small coloured value badge (the held/discard card).
function chip(v: number): CSSProperties {
  const c = cardColor(v);
  return { background: c.bg, color: c.ink, padding: "0 .35rem", borderRadius: "4px" };
}

export default function SkyjoPage() {
  const { t } = useTranslation();
  const [params] = useSearchParams();
  const [name, setName] = useState(() => localStorage.getItem("totem-game-name") ?? "");
  const [code, setCode] = useState(() => (params.get("code") ?? "").toUpperCase());
  const [session, setSession] = usePersistedSession<{ code: string; players: LocalPlayer[] }>("totem-skyjo-session");
  const [focus, setFocus] = useState<string | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (session) {
    return (
      <main className="game-page">
        <LocalSeats
          players={session.players}
          onAddPlayer={(p) => setSession((s) => (s ? { ...s, players: [...s.players, p] } : s))}
          addMenu={(submit, cancel) => <NameAddForm submit={submit} cancel={cancel} maxLen={NICK_MAX} />}
          focusSeatKey={focus}
          renderSeat={(p, seatKey) => (
            <SkyjoRoom code={session.code} name={p.name} seatKey={seatKey} onMyTurn={() => setFocus(seatKey)} onLeave={() => setSession(null)} />
          )}
        />
      </main>
    );
  }

  const begin = (roomCode: string) => {
    const nick = name.trim();
    localStorage.setItem("totem-game-name", nick);
    setSession({ code: roomCode, players: [{ name: nick }] });
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
      begin((await createGameRoom("skyjo")).code);
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
        <h2>🂡 {t("gameSkyjo")}</h2>
        <p className="muted-note">{t("gameSkyjoBlurb")}</p>
        <h3>{t("gameYourName")}</h3>
        <input
          className="trait-search"
          value={name}
          maxLength={NICK_MAX}
          placeholder={t("gameNamePlaceholder")}
          onChange={(e) => setName(e.target.value)}
        />
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

// One player's 3x4 grid. `onCell` is set only for the cells the viewer may act on now.
function Grid({ p, onCell, canClick }: { p: SkPlayer; onCell: (cell: number) => void; canClick: (c: SkCell, i: number) => boolean }) {
  return (
    <div className="sk-grid" style={{ gridTemplateColumns: `repeat(${SK_COLS}, 1fr)` }}>
      {p.grid.map((c, i) => {
        if (c.gone) return <div key={i} className="sk-cell gone" />;
        const click = canClick(c, i);
        const col = c.up ? cardColor(c.v) : null;
        return (
          <button
            key={i}
            className={`sk-cell${c.up ? " up" : " down"}${click ? " actionable" : ""}`}
            style={col ? { background: col.bg, color: col.ink } : undefined}
            disabled={!click}
            onClick={() => click && onCell(i)}
          >
            {c.up ? c.v : "?"}
          </button>
        );
      })}
    </div>
  );
}

function SkyjoRoom({ code, name, onLeave, seatKey = "", onMyTurn }: { code: string; name: string; onLeave: () => void; seatKey?: string; onMyTurn?: () => void }) {
  const { t } = useTranslation();
  const g = useRoom<SkView>(code, name, "", seatKey);
  const v = g.view;
  const myTurnNow = v?.ph === "play" && v?.turn === v?.you;
  useEffect(() => {
    if (myTurnNow) onMyTurn?.();
  }, [myTurnNow, onMyTurn]);
  const bar = <RoomBar code={code} title={t("gameSkyjo")} infoSlug="skyjo" gameSlug="skyjo" onLeave={onLeave} />;

  if (g.game && g.game !== "skyjo") {
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
  const me = v.players.find((p) => p.s === v.you);
  const myTurn = v.ph === "play" && v.turn === v.you;
  const nameOf = (s: number) => v.players.find((p) => p.s === s)?.n ?? "?";
  const holding = myTurn && v.drawn !== SK_NONE;

  if (v.ph === "lobby") {
    return (
      <div className="game-room">
        {bar}
        <div className="mind-lobby">
          <p className="muted-note">2–8 players. Lower your grid, clear matching columns, dodge a high total.</p>
          <ul className="mind-players">
            {v.players.map((p) => (
              <li key={p.s} className={p.gone ? "gone" : ""}>
                🂠 {p.n}{p.s === v.you ? " (you)" : ""}
              </li>
            ))}
          </ul>
          {isHost ? (
            <button className="game-btn" onClick={() => g.send({ t: "start" })}>Start game</button>
          ) : (
            <p className="muted-note">Waiting for the host to start…</p>
          )}
        </div>
      </div>
    );
  }

  // Whether the viewer may click a given cell of THEIR OWN grid right now.
  const canClickMine = (c: SkCell, _i: number): boolean => {
    if (!me) return false;
    if (v.ph === "setup") return !c.up && !c.gone && me.flips < 2;
    if (!myTurn) return false;
    if (v.mustFlip) return !c.up && !c.gone; // must reveal a face-down card
    if (holding) return !c.gone; // swap the held card into any live cell
    return false;
  };
  const onMine = (cell: number) => {
    if (v.ph === "setup" || v.mustFlip) g.send({ t: "flip", cell });
    else if (holding) g.send({ t: "swap", cell });
  };
  const noClick = () => false;

  const status =
    v.ph === "over"
      ? `Game over — ${nameOf(v.gameWinner)} wins!`
      : v.ph === "roundEnd"
        ? "Round scored"
        : v.ph === "setup"
          ? me && me.flips < 2 ? `Flip ${2 - me.flips} more` : "Waiting for others to flip…"
          : myTurn ? "Your turn" : `${nameOf(v.turn)}'s turn`;

  return (
    <div className="game-room sk-room">
      {bar}
      <div className="mind-hud panel">
        <span>🂠 {v.deckLeft} in deck</span>
        <span className="sk-discard">
          🗑 discard:{" "}
          {v.hasDiscard ? (
            <b style={chip(v.discardTop)}>{v.discardTop}</b>
          ) : "—"}
        </span>
        {v.closer >= 0 && <span className="cr-collapse">⚑ {nameOf(v.closer)} closed</span>}
        <span>{status}</span>
      </div>

      {/* your turn controls */}
      {myTurn && (
        <div className="panel sk-controls">
          {!holding && !v.mustFlip && (
            <>
              <button className="game-btn" onClick={() => g.send({ t: "draw" })}>Draw from deck</button>
              <button className="game-btn ghost" disabled={!v.hasDiscard} onClick={() => g.send({ t: "takeDiscard" })}>
                Take discard ({v.hasDiscard ? v.discardTop : "—"})
              </button>
            </>
          )}
          {holding && (
            <>
              <span>Holding <b style={chip(v.drawn)}>{v.drawn}</b>: click a card in your grid to swap it in{v.fromDeck ? ", or" : "."}</span>
              {v.fromDeck && <button className="game-btn ghost" onClick={() => g.send({ t: "discardDraw" })}>Discard it &amp; flip a card</button>}
            </>
          )}
          {v.mustFlip && <span>Click a face-down card in your grid to flip it.</span>}
        </div>
      )}

      {/* the tables */}
      <div className="sk-tables">
        {v.players.map((p) => {
          const mine = p.s === v.you;
          return (
            <div key={p.s} className={`panel sk-player${p.s === v.turn ? " active" : ""}${p.gone ? " gone" : ""}`}>
              <div className="sk-player-head">
                <strong>{p.n}{mine ? " (you)" : ""}</strong>
                <span className="muted-note">score {p.score}{v.ph === "setup" ? ` · flipped ${p.flips}/2` : ""}</span>
              </div>
              <Grid p={p} onCell={mine ? onMine : () => {}} canClick={mine ? canClickMine : noClick} />
            </div>
          );
        })}
      </div>

      {(v.ph === "roundEnd" || v.ph === "over") && (
        <div className="mind-controls panel mind-over">
          <strong>
            {v.ph === "over"
              ? `🏆 ${nameOf(v.gameWinner)} wins with the lowest total!`
              : "Round scored — totals updated."}
          </strong>
          {isHost && v.ph === "roundEnd" && <button className="game-btn" onClick={() => g.send({ t: "next" })}>Next round</button>}
          {isHost && v.ph === "over" && <button className="game-btn" onClick={() => g.send({ t: "restart" })}>New game</button>}
        </div>
      )}

      {v.logs.length > 0 && (
        <div className="cr-log panel">
          {v.logs.slice(-6).map((l, i) => <div key={i}>{l}</div>)}
        </div>
      )}
    </div>
  );
}
