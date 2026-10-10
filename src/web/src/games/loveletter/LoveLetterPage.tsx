import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router-dom";
import { createGameRoom } from "../../api";
import { CODE_LEN, NICK_MAX, validNick } from "../validate";
import { useRoom } from "../useRoom";
import { RoomBar } from "../RoomBar";

const CARDS: Record<number, { name: string; hint: string }> = {
  1: { name: "Guard", hint: "Name another card (not Guard) + a player; if right, they're out." },
  2: { name: "Priest", hint: "Secretly look at one player's hand." },
  3: { name: "Baron", hint: "Compare hands with a player; the lower card is out." },
  4: { name: "Handmaid", hint: "You can't be targeted until your next turn." },
  5: { name: "Prince", hint: "Pick a player (or yourself) to discard and redraw." },
  6: { name: "King", hint: "Trade hands with another player." },
  7: { name: "Countess", hint: "No effect — but must be played if you hold a King or Prince." },
  8: { name: "Princess", hint: "If you ever discard this, you're out. Keep it!" },
};
const needsTarget = (c: number) => c === 1 || c === 2 || c === 3 || c === 5 || c === 6;
const canTargetSelf = (c: number) => c === 5; // Prince

interface LlPlayer {
  s: number;
  n: string;
  tokens: number;
  out: boolean;
  protected: boolean;
  hand: number;
  reveal?: number[];
  discard: number[];
  gone?: boolean;
}
interface LlView {
  t: "state";
  ph: "lobby" | "play" | "roundEnd" | "over";
  active: number;
  you: number;
  yourHand: number[];
  saw?: { target: number; card: number };
  deckLeft: number;
  aside: number[];
  need: number;
  logs: string[];
  winner: number;
  gameWinner: number;
  players: LlPlayer[];
}

export default function LoveLetterPage() {
  const { t } = useTranslation();
  const [params] = useSearchParams();
  const [name, setName] = useState(() => localStorage.getItem("totem-game-name") ?? "");
  const [code, setCode] = useState(() => (params.get("code") ?? "").toUpperCase());
  const [session, setSession] = useState<{ code: string; name: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (session) {
    return (
      <main className="game-page">
        <LoveLetterRoom code={session.code} name={session.name} onLeave={() => setSession(null)} />
      </main>
    );
  }

  const begin = (roomCode: string) => {
    const nick = name.trim();
    localStorage.setItem("totem-game-name", nick);
    setSession({ code: roomCode, name: nick });
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
      begin((await createGameRoom("love-letter")).code);
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
        <h2>💌 {t("gameLoveLetter")}</h2>
        <p className="muted-note">{t("gameLoveLetterBlurb")}</p>
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

function LoveLetterRoom({ code, name, onLeave }: { code: string; name: string; onLeave: () => void }) {
  const { t } = useTranslation();
  const g = useRoom<LlView>(code, name);
  const [pick, setPick] = useState<number | null>(null); // card chosen to play
  const [target, setTarget] = useState<number | null>(null);
  const [guess, setGuess] = useState<number>(2);
  const v = g.view;
  const bar = <RoomBar code={code} title={t("gameLoveLetter")} infoSlug="loveletter" gameSlug="love-letter" onLeave={onLeave} />;

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
  const myTurn = v.ph === "play" && v.active === v.you && !(me?.out);
  const nameOf = (s: number) => v.players.find((p) => p.s === s)?.n ?? "?";

  const reset = () => { setPick(null); setTarget(null); setGuess(2); };
  const playNow = (card: number, tgt: number | null, gs: number) => {
    g.send({ t: "play", card, target: tgt ?? -1, guess: gs });
    reset();
  };
  const choose = (card: number) => {
    if (!needsTarget(card)) { playNow(card, null, 0); return; }
    setPick(card); setTarget(null);
  };
  const validTargets = (card: number) =>
    v.players.filter((p) => !p.out && !p.protected && (p.s !== v.you || canTargetSelf(card)));

  if (v.ph === "lobby") {
    return (
      <div className="game-room ll-room">
        {bar}
        <div className="mind-lobby">
          <p className="muted-note">2–4 players. Hold one secret card, outlast the table, win tokens.</p>
          <ul className="mind-players">
            {v.players.map((p) => (
              <li key={p.s} className={p.gone ? "gone" : ""}>👤 {p.n}{p.s === v.you ? " (you)" : ""}</li>
            ))}
          </ul>
          {isHost ? (
            <button className="game-btn" disabled={v.players.length < 2} onClick={() => g.send({ t: "start" })}>
              {v.players.length < 2 ? "Need at least 2 players" : "Start game"}
            </button>
          ) : (
            <p className="muted-note">Waiting for the host to start…</p>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className="game-room ll-room">
      {bar}
      <div className="mind-hud panel">
        <span>🂠 {v.deckLeft} left</span>
        <span>{v.ph === "play" ? (myTurn ? "Your turn" : `${nameOf(v.active)}'s turn`) : ""}</span>
        <span>First to {v.need} 🏅</span>
      </div>

      {v.aside.length > 0 && (
        <p className="muted-note ll-aside">Set aside (face up): {v.aside.map((c) => CARDS[c].name).join(", ")}</p>
      )}

      <div className="ll-players">
        {v.players.map((p) => (
          <div key={p.s} className={`ll-player${p.s === v.active ? " active" : ""}${p.out ? " out" : ""}`}>
            <div className="ll-player-head">
              <strong>{p.n}{p.s === v.you ? " (you)" : ""}</strong>
              <span>{"🏅".repeat(p.tokens)}</span>
            </div>
            <div className="ll-player-sub">
              {p.out ? "out" : p.protected ? "🛡 protected" : `${p.hand} card`}
              {p.reveal && p.reveal.length > 0 && <> · holds {p.reveal.map((c) => CARDS[c].name).join(", ")}</>}
            </div>
            {p.discard.length > 0 && (
              <div className="ll-discard">{p.discard.map((c, i) => <span key={i} className="ll-dcard">{c} {CARDS[c].name}</span>)}</div>
            )}
          </div>
        ))}
      </div>

      {v.saw && (
        <p className="ll-saw">🔎 You saw <strong>{nameOf(v.saw.target)}</strong> holds <strong>{CARDS[v.saw.card].name}</strong></p>
      )}

      {(v.ph === "play") && (
        <div className="ll-hand panel">
          <small className="muted-note">Your hand</small>
          <div className="ll-hand-cards">
            {v.yourHand.map((c, i) => (
              <button
                key={i}
                className={`ll-card${pick === c ? " sel" : ""}`}
                disabled={!myTurn}
                onClick={() => choose(c)}
                title={CARDS[c].hint}
              >
                <span className="ll-card-v">{c}</span>
                <span className="ll-card-n">{CARDS[c].name}</span>
              </button>
            ))}
          </div>
          {myTurn && pick != null && (
            <div className="ll-resolve">
              <small className="muted-note">{CARDS[pick].hint}</small>
              <div className="ll-targets">
                {validTargets(pick).map((p) => (
                  <button
                    key={p.s}
                    className={`game-btn ghost${target === p.s ? " sel" : ""}`}
                    onClick={() => setTarget(p.s)}
                  >
                    {p.n}{p.s === v.you ? " (you)" : ""}
                  </button>
                ))}
                {validTargets(pick).length === 0 && <span className="muted-note">No valid target — play for no effect.</span>}
              </div>
              {pick === 1 && (
                <select className="trait-search" value={guess} onChange={(e) => setGuess(Number(e.target.value))}>
                  {[2, 3, 4, 5, 6, 7, 8].map((n) => <option key={n} value={n}>{n} · {CARDS[n].name}</option>)}
                </select>
              )}
              <div className="ll-resolve-actions">
                <button
                  className="game-btn"
                  disabled={validTargets(pick).length > 0 && target == null}
                  onClick={() => playNow(pick, target, pick === 1 ? guess : 0)}
                >
                  Play {CARDS[pick].name}
                </button>
                <button className="game-btn ghost" onClick={reset}>Cancel</button>
              </div>
            </div>
          )}
          {myTurn && pick == null && <small className="muted-note">Tap a card to play it.</small>}
          {!myTurn && <small className="muted-note">Waiting for {nameOf(v.active)}…</small>}
        </div>
      )}

      {(v.ph === "roundEnd" || v.ph === "over") && (
        <div className="mind-controls panel mind-over">
          <strong>
            {v.ph === "over"
              ? (v.gameWinner >= 0 ? `${nameOf(v.gameWinner)} wins the game! 🎉` : "Game over")
              : `${nameOf(v.winner)} wins the round 🏅`}
          </strong>
          {isHost && v.ph === "roundEnd" && (
            <button className="game-btn" onClick={() => g.send({ t: "next" })}>Next round</button>
          )}
          {isHost && v.ph === "over" && (
            <button className="game-btn" onClick={() => g.send({ t: "restart" })}>New game</button>
          )}
        </div>
      )}

      {v.logs.length > 0 && (
        <div className="ll-log panel">
          {v.logs.slice(-8).map((l, i) => <div key={i}>{l}</div>)}
        </div>
      )}
    </div>
  );
}
