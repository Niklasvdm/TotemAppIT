import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router-dom";
import { createGameRoom } from "../../api";
import { CODE_LEN, NICK_MAX, validNick } from "../validate";
import { useRoom } from "../useRoom";
import { RoomBar } from "../RoomBar";
import { Card } from "../Card";

// Mirrors theGameView in internal/game/thegame.go. `hand` is this seat's own
// cards (the private view); others are counts.
interface TgRoster {
  s: number;
  n: string;
  cards: number;
  gone?: boolean;
}
interface TgView {
  t: "state";
  ph: "lobby" | "playing" | "won" | "lost";
  piles: number[]; // [asc, asc, desc, desc] current tops
  history: number[][]; // every card played per pile, for display
  reserved: number[]; // seat that "called" each pile, or -1
  deck: number;
  hand: number[];
  played: number;
  min: number;
  turn: number;
  handSize: number;
  you: number;
  roster: TgRoster[];
}

// Mirror of canPlay in thegame.go — only to light up legal moves; the server
// decides.
function canPlay(card: number, pile: number, piles: number[]): boolean {
  const top = piles[pile];
  if (pile < 2) return card > top || card === top - 10; // ascending
  return card < top || card === top + 10; // descending
}

export default function TheGamePage() {
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
        <TheGameRoom code={session.code} name={session.name} onLeave={() => setSession(null)} />
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
      begin((await createGameRoom("the-game")).code);
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
        <h2>🃏 {t("gameTheGame")}</h2>
        <p className="muted-note">{t("gameTheGameBlurb")}</p>

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

function TheGameRoom({
  code,
  name,
  onLeave,
}: {
  code: string;
  name: string;
  onLeave: () => void;
}) {
  const { t } = useTranslation();
  const g = useRoom<TgView>(code, name);
  const [selected, setSelected] = useState<number | null>(null);
  const v = g.view;

  const bar = <RoomBar code={code} title={t("gameTheGame")} onLeave={onLeave} />;

  if (g.status === "closed") {
    return (
      <div className="game-room">
        {bar}
        <div className="panel game-notice">
          <h3>Disconnected</h3>
          <button className="game-btn" onClick={onLeave}>
            {t("gameBack")}
          </button>
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
  const myTurn = v.you === v.turn;
  const turnName = v.roster.find((r) => r.s === v.turn)?.n ?? "";

  if (v.ph === "lobby") {
    return (
      <div className="game-room tg-room">
        {bar}
        <div className="mind-lobby">
          <p className="muted-note">1–5 players. Empty the deck onto the piles — no naming numbers.</p>
          <ul className="mind-players">
            {v.roster.map((r) => (
              <li key={r.s} className={r.gone ? "gone" : ""}>
                🃏 {r.n}
                {r.s === v.you ? " (you)" : ""}
              </li>
            ))}
          </ul>
          {isHost ? (
            <button
              className="game-btn"
              disabled={v.roster.length < 1 || v.roster.length > 5}
              onClick={() => g.send({ t: "start" })}
            >
              Start game
            </button>
          ) : (
            <p className="muted-note">Waiting for the host to start…</p>
          )}
        </div>
      </div>
    );
  }

  const playCard = (pile: number) => {
    if (selected == null || !myTurn) return;
    if (!canPlay(selected, pile, v.piles)) return;
    g.send({ t: "play", card: selected, pile });
    setSelected(null);
  };

  return (
    <div className="game-room tg-room">
      {bar}

      <div className="mind-hud panel">
        <span>Deck: {v.deck}</span>
        <span>{myTurn ? `Your turn — ${Math.max(0, v.min - v.played)} more to play` : `${turnName}'s turn`}</span>
      </div>

      {v.ph === "playing" ? (
        <>
          <div className="tg-hand">
            {(v.hand ?? []).length === 0 ? (
              <span className="muted-note">Hand empty.</span>
            ) : (
              (v.hand ?? []).map((c) => (
                <Card
                  key={c}
                  n={c}
                  className={selected === c ? "sel" : ""}
                  disabled={!myTurn}
                  onClick={() => setSelected(selected === c ? null : c)}
                />
              ))
            )}
          </div>
          <div className="mind-controls panel">
            <span className="muted-note">
              {myTurn ? "Tap a card, then a pile. " : ""}
            </span>
            <button
              className="game-btn"
              disabled={!myTurn || v.played < v.min}
              onClick={() => g.send({ t: "endturn" })}
            >
              End turn
            </button>
          </div>
        </>
      ) : (
        <div className="mind-controls panel mind-over">
          <strong>{v.ph === "won" ? "You win! 🎉" : "Stuck — nobody could play 💀"}</strong>
          {isHost && (
            <button className="game-btn" onClick={() => g.send({ t: "restart" })}>
              New game
            </button>
          )}
        </div>
      )}

      <div className="tg-piles">
        {v.piles.map((top, i) => {
          const playable = myTurn && selected != null && canPlay(selected, i, v.piles);
          const stack = [...(v.history?.[i] ?? [top])].reverse(); // newest on top; grows down
          const reservedBy = v.reserved?.[i] ?? -1;
          const reserver = v.roster.find((r) => r.s === reservedBy)?.n;
          return (
            <div
              key={i}
              className={`tg-pile ${i < 2 ? "asc" : "desc"} ${reservedBy >= 0 ? "reserved" : ""} ${playable ? "playable" : ""}`}
            >
              <div className="tg-pile-head">
                <small>{i < 2 ? "↑" : "↓"}</small>
                {!myTurn && v.ph === "playing" && (
                  <button
                    className={`tg-reserve ${reservedBy === v.you ? "active" : ""}`}
                    title="Ask to play here (hold on)"
                    onClick={() => g.send({ t: "reserve", pile: i })}
                  >
                    ✋
                  </button>
                )}
              </div>
              <button className="tg-playbtn" disabled={!playable} onClick={() => playCard(i)}>
                <div className="tg-stack">
                  {stack.map((c, k) => (
                    <Card key={k} n={c} className="strip" />
                  ))}
                </div>
              </button>
              {reserver && <small className="tg-reserver">✋ {reserver}</small>}
            </div>
          );
        })}
      </div>
    </div>
  );
}
