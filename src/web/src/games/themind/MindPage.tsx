import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router-dom";
import { createGameRoom } from "../../api";
import { CODE_LEN, NICK_MAX, validNick } from "../validate";
import { useRoom } from "../useRoom";
import { RoomBar } from "../RoomBar";
import { Card, lastN } from "../Card";

// Mirrors mindView in internal/game/mind.go. `hand` is this seat's own cards —
// the private view; everyone else is just a count.
interface MindRoster {
  s: number;
  n: string;
  cards: number;
  voted: boolean;
  gone?: boolean;
}
interface MindView {
  t: "state";
  ph: "lobby" | "playing" | "won" | "lost";
  level: number;
  lives: number;
  stars: number;
  pile: number;
  pileSeq: number[];
  levelsToWin: number;
  hand: number[];
  you: number;
  roster: MindRoster[];
}

export default function MindPage() {
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
        <MindRoom code={session.code} name={session.name} onLeave={() => setSession(null)} />
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
      begin((await createGameRoom("the-mind")).code);
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
        <h2>🧠 {t("gameTheMind")}</h2>
        <p className="muted-note">{t("gameTheMindBlurb")}</p>

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

function MindRoom({
  code,
  name,
  onLeave,
}: {
  code: string;
  name: string;
  onLeave: () => void;
}) {
  const { t } = useTranslation();
  const g = useRoom<MindView>(code, name);
  const v = g.view;

  // Flash a breaking heart whenever a life is lost (hooks must run before the
  // early returns below, so this lives up here).
  const prevLives = useRef<number | null>(null);
  const [showLost, setShowLost] = useState(false);
  useEffect(() => {
    const prev = prevLives.current;
    const cur = v?.lives;
    if (cur == null) return;
    prevLives.current = cur;
    if (prev != null && cur < prev) {
      setShowLost(true);
      const h = setTimeout(() => setShowLost(false), 1700);
      return () => clearTimeout(h);
    }
  }, [v?.lives]);

  const bar = <RoomBar code={code} title={t("gameTheMind")} infoSlug="themind" gameSlug="the-mind" onLeave={onLeave} />;

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
  const me = v.roster.find((r) => r.s === v.you);
  const myVoted = me?.voted ?? false;

  return (
    <div className="game-room mind-room">
      {bar}

      {v.ph === "lobby" ? (
        <div className="mind-lobby">
          <p className="muted-note">2–4 players. Play your cards in rising order — no talking.</p>
          <ul className="mind-players">
            {v.roster.map((r) => (
              <li key={r.s} className={r.gone ? "gone" : ""}>
                👤 {r.n}
                {r.s === v.you ? " (you)" : ""}
              </li>
            ))}
          </ul>
          {isHost ? (
            <button
              className="game-btn"
              disabled={v.roster.length < 2 || v.roster.length > 4}
              onClick={() => g.send({ t: "start" })}
            >
              {v.roster.length < 2 ? "Need at least 2 players" : "Start game"}
            </button>
          ) : (
            <p className="muted-note">Waiting for the host to start…</p>
          )}
        </div>
      ) : (
        <>
          <div className="mind-hud panel">
            <span>Level {v.level}/{v.levelsToWin}</span>
            <span className="mind-lives">
              {"❤️".repeat(Math.max(0, v.lives))}
              {showLost && (
                <span className="heart-lost" aria-label="life lost">
                  <span className="hl-flick">❤️</span>
                  <span className="hl-break">💔</span>
                </span>
              )}
            </span>
            <span className="mind-stars">{"⭐".repeat(Math.max(0, v.stars))}</span>
          </div>

          <div className="mind-pile">
            <small>Pile</small>
            <div className="card-fan">
              {(v.pileSeq ?? []).length === 0 ? (
                <span className="muted-note">{v.pile || "—"}</span>
              ) : (
                lastN(v.pileSeq ?? [], 18).map((c, i) => <Card key={i} n={c} className="sm" />)
              )}
            </div>
          </div>

          <div className="mind-others">
            {v.roster
              .filter((r) => r.s !== v.you)
              .map((r) => (
                <span key={r.s} className={`mind-other ${r.gone ? "gone" : ""}`}>
                  {r.n}: {r.cards} 🂠{r.voted ? " ⭐" : ""}
                </span>
              ))}
          </div>

          {v.ph === "playing" ? (
            <>
              <div className="mind-hand">
                {(v.hand ?? []).length === 0 ? (
                  <span className="muted-note">Your hand is empty — waiting on the others.</span>
                ) : (
                  (v.hand ?? []).map((c, i) => <Card key={c} n={c} className={i === 0 ? "sel" : ""} />)
                )}
              </div>
              <div className="mind-controls panel">
                <button
                  className="game-btn"
                  disabled={(v.hand ?? []).length === 0}
                  onClick={() => g.send({ t: "play" })}
                >
                  Play my lowest ({(v.hand ?? [])[0] ?? "—"})
                </button>
                <button
                  className={`game-btn ghost ${myVoted ? "active" : ""}`}
                  disabled={v.stars <= 0}
                  onClick={() => g.send({ t: "star" })}
                >
                  {myVoted ? "Star voted ✓" : "Vote throwing star ⭐"}
                </button>
              </div>
            </>
          ) : (
            <div className="mind-controls panel mind-over">
              <strong>{v.ph === "won" ? "You win! 🎉" : `Out of lives — level ${v.level} 💀`}</strong>
              {isHost && (
                <button className="game-btn" onClick={() => g.send({ t: "restart" })}>
                  New game
                </button>
              )}
            </div>
          )}
        </>
      )}
    </div>
  );
}
