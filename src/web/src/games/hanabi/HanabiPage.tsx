import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router-dom";
import { createGameRoom } from "../../api";
import { CODE_LEN, NICK_MAX, validNick } from "../validate";
import { useRoom } from "../useRoom";
import { RoomBar } from "../RoomBar";

// Five Hanabi suits. Index matches the backend's colour ints.
const COLORS = [
  { label: "White", css: "#e9e9ee", ink: "#222" },
  { label: "Red", css: "#e1495c", ink: "#fff" },
  { label: "Blue", css: "#3f7fd8", ink: "#fff" },
  { label: "Yellow", css: "#e7c14b", ink: "#222" },
  { label: "Green", css: "#4caf72", ink: "#fff" },
];

interface HanCard {
  color: number; // -1 hidden
  value: number; // 0 hidden
  kc: boolean;
  kv: boolean;
  nc: number[];
  nv: number[];
}
interface HanHand {
  s: number;
  n: string;
  gone?: boolean;
  cards: HanCard[];
}
interface HanView {
  t: "state";
  ph: "lobby" | "play" | "over";
  turn: number;
  you: number;
  hints: number;
  fuses: number;
  deck: number;
  stacks: number[];
  discard: { color: number; value: number }[];
  lastRound: boolean;
  score: number;
  win: boolean;
  lost: boolean;
  hands: HanHand[];
}

export default function HanabiPage() {
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
        <HanabiRoom code={session.code} name={session.name} onLeave={() => setSession(null)} />
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
      begin((await createGameRoom("hanabi")).code);
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
        <h2>🎆 {t("gameHanabi")}</h2>
        <p className="muted-note">{t("gameHanabiBlurb")}</p>
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

function CardFace({ c }: { c: HanCard }) {
  const known = c.color >= 0;
  const col = known ? COLORS[c.color] : null;
  return (
    <div
      className="han-card"
      style={col ? { background: col.css, color: col.ink } : undefined}
    >
      <span className="han-card-v">{c.value > 0 ? c.value : "?"}</span>
      {!known && (c.nc.length > 0 || c.nv.length > 0) && (
        <span className="han-card-not">
          {c.nc.length > 0 && <>not {c.nc.map((i) => COLORS[i].label[0]).join("")}</>}
          {c.nv.length > 0 && <> ≠{c.nv.join("")}</>}
        </span>
      )}
    </div>
  );
}

function HanabiRoom({ code, name, onLeave }: { code: string; name: string; onLeave: () => void }) {
  const { t } = useTranslation();
  const g = useRoom<HanView>(code, name);
  const [hintTarget, setHintTarget] = useState<number | null>(null);
  const v = g.view;
  const bar = <RoomBar code={code} title={t("gameHanabi")} infoSlug="hanabi" onLeave={onLeave} />;

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
  const myTurn = v.ph === "play" && v.turn === v.you;
  const nameOf = (s: number) => v.hands.find((h) => h.s === s)?.n ?? "?";

  if (v.ph === "lobby") {
    return (
      <div className="game-room han-room">
        {bar}
        <div className="mind-lobby">
          <p className="muted-note">2–5 players, co-op. You see everyone's cards but your own. Build five stacks 1→5.</p>
          <ul className="mind-players">
            {v.hands.map((h) => <li key={h.s} className={h.gone ? "gone" : ""}>👤 {h.n}{h.s === v.you ? " (you)" : ""}</li>)}
          </ul>
          {isHost ? (
            <button className="game-btn" disabled={v.hands.length < 2} onClick={() => g.send({ t: "start" })}>
              {v.hands.length < 2 ? "Need at least 2 players" : "Start game"}
            </button>
          ) : (
            <p className="muted-note">Waiting for the host to start…</p>
          )}
        </div>
      </div>
    );
  }

  const others = v.hands.filter((h) => h.s !== v.you);
  const me = v.hands.find((h) => h.s === v.you);

  return (
    <div className="game-room han-room">
      {bar}
      <div className="mind-hud panel">
        <span>💡 {v.hints}/8</span>
        <span>💥 {v.fuses}/3</span>
        <span>🂠 {v.deck}{v.lastRound ? " · final round!" : ""}</span>
        <span>★ {v.score}/25</span>
      </div>

      <div className="han-stacks">
        {v.stacks.map((top, col) => (
          <div key={col} className="han-stack" style={{ background: COLORS[col].css, color: COLORS[col].ink }}>
            <small>{COLORS[col].label}</small>
            <strong>{top > 0 ? top : "–"}</strong>
          </div>
        ))}
      </div>

      <p className="muted-note han-turn">
        {v.ph === "over" ? "" : myTurn ? "Your turn — play or discard one of your cards, or give a hint." : `${nameOf(v.turn)}'s turn`}
      </p>

      {/* other players: you see their faces, and can hint them on your turn */}
      {others.map((h) => (
        <div key={h.s} className={`han-player panel${h.s === v.turn ? " active" : ""}`}>
          <div className="han-player-head"><strong>{h.n}{h.gone ? " (away)" : ""}</strong></div>
          <div className="han-cards">
            {h.cards.map((c, i) => (
              <div key={i} className="han-card-wrap">
                <CardFace c={c} />
                {(c.kc || c.kv) && <span className="han-known">{c.kc ? "col" : ""}{c.kv ? "#" : ""}</span>}
              </div>
            ))}
          </div>
          {myTurn && v.hints > 0 && (
            hintTarget === h.s ? (
              <div className="han-hint-pick">
                <small className="muted-note">Hint {h.n}:</small>
                <div className="han-hint-row">
                  {COLORS.map((cl, ci) => (
                    <button
                      key={ci}
                      className="han-chip"
                      style={{ background: cl.css, color: cl.ink }}
                      onClick={() => { g.send({ t: "hint", target: h.s, kind: "color", value: ci }); setHintTarget(null); }}
                    >
                      {cl.label[0]}
                    </button>
                  ))}
                </div>
                <div className="han-hint-row">
                  {[1, 2, 3, 4, 5].map((n) => (
                    <button key={n} className="han-chip num" onClick={() => { g.send({ t: "hint", target: h.s, kind: "value", value: n }); setHintTarget(null); }}>
                      {n}
                    </button>
                  ))}
                </div>
                <button className="game-btn ghost" onClick={() => setHintTarget(null)}>Cancel</button>
              </div>
            ) : (
              <button className="game-btn ghost han-hint-btn" onClick={() => setHintTarget(h.s)}>💡 Hint {h.n}</button>
            )
          )}
        </div>
      ))}

      {/* your own hand: card backs with only what hints revealed */}
      {me && (
        <div className="han-player panel han-self">
          <div className="han-player-head"><strong>Your hand (hidden to you)</strong></div>
          <div className="han-cards">
            {me.cards.map((c, i) => (
              <div key={i} className="han-card-wrap">
                <CardFace c={c} />
                {myTurn && (
                  <div className="han-self-actions">
                    <button className="game-btn tiny" onClick={() => g.send({ t: "play", card: i })}>Play</button>
                    <button className="game-btn ghost tiny" disabled={v.hints >= 8} onClick={() => g.send({ t: "discard", card: i })}>Discard</button>
                  </div>
                )}
              </div>
            ))}
          </div>
        </div>
      )}

      {v.discard.length > 0 && (
        <div className="han-discard panel">
          <small className="muted-note">Discard & misfires</small>
          <div className="han-discard-row">
            {v.discard.map((d, i) => (
              <span key={i} className="han-dchip" style={{ background: COLORS[d.color].css, color: COLORS[d.color].ink }}>{d.value}</span>
            ))}
          </div>
        </div>
      )}

      {v.ph === "over" && (
        <div className="mind-controls panel mind-over">
          <strong>
            {v.win ? "Perfect display — 25! 🎆" : v.lost ? `Three fuses blown — the show's over. Score ${v.score}/25` : `Deck empty — final score ${v.score}/25`}
          </strong>
          {isHost && <button className="game-btn" onClick={() => g.send({ t: "restart" })}>New game</button>}
        </div>
      )}
    </div>
  );
}
