import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router-dom";
import { createGameRoom } from "../../api";
import { CODE_LEN, NICK_MAX, validNick } from "../validate";
import { useRoom } from "../useRoom";
import { RoomBar } from "../RoomBar";

// Mirrors wlView in internal/game/wavelength.go. `target` is the hidden spot —
// sent only to the psychic until the reveal (the private view).
interface WlRoster {
  s: number;
  n: string;
  team: number;
  gone?: boolean;
}
interface WlView {
  t: "state";
  ph: "lobby" | "clue" | "guess" | "bet" | "reveal" | "over";
  spectrum: [string, string];
  clue: string;
  guess: number;
  active: number;
  psychic: number;
  counter: string;
  left: number;
  right: number;
  winner: number;
  lastScore: number;
  betOk: boolean;
  target: number | null;
  you: number;
  youTeam: number;
  roster: WlRoster[];
}

const SCALE = 20;
const pct = (v: number) => `${(Math.max(0, Math.min(SCALE, v)) / SCALE) * 100}%`;
const teamName = (t: number) => (t === 1 ? "Right" : "Left");

export default function WavelengthPage() {
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
        <WavelengthRoom code={session.code} name={session.name} onLeave={() => setSession(null)} />
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
      begin((await createGameRoom("wavelength")).code);
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
        <h2>📡 {t("gameWavelength")}</h2>
        <p className="muted-note">{t("gameWavelengthBlurb")}</p>

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

function WavelengthRoom({ code, name, onLeave }: { code: string; name: string; onLeave: () => void }) {
  const { t } = useTranslation();
  const g = useRoom<WlView>(code, name);
  const v = g.view;
  const bar = <RoomBar code={code} title={t("gameWavelength")} infoSlug="wavelength" gameSlug="wavelength" onLeave={onLeave} />;

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

  if (v.ph === "lobby") {
    const setup = (team: number) => g.send({ t: "setup", team });
    const sized = [0, 1].map((tm) => v.roster.filter((p) => p.team === tm).length);
    const canStart = sized[0] >= 2 && sized[1] >= 2;
    return (
      <div className="game-room wl-room">
        {bar}
        <p className="muted-note" style={{ textAlign: "center" }}>
          Two teams, 2+ each. A psychic sees a hidden spot; their team dials to it, the other team bets.
        </p>
        <div className="cn-teams">
          {[0, 1].map((tm) => (
            <div key={tm} className={`cn-team cn-team-${tm === 1 ? "blue" : "red"}`}>
              <h3>{teamName(tm)}</h3>
              <ul>
                {v.roster
                  .filter((p) => p.team === tm)
                  .map((p) => (
                    <li key={p.s} className={p.gone ? "gone" : ""}>
                      👤 {p.n}
                      {p.s === v.you ? " (you)" : ""}
                    </li>
                  ))}
              </ul>
              <button className="game-btn ghost" onClick={() => setup(tm)}>
                Join {teamName(tm)}
              </button>
            </div>
          ))}
        </div>
        {isHost ? (
          <button className="game-btn cn-start" disabled={!canStart} onClick={() => g.send({ t: "start" })}>
            {canStart ? "Start game" : "Need 2+ players on each team"}
          </button>
        ) : (
          <p className="muted-note">Waiting for the host to start…</p>
        )}
      </div>
    );
  }

  const amPsychic = v.you === v.psychic;
  const onActive = v.youTeam === v.active;
  const psychicName = v.roster.find((r) => r.s === v.psychic)?.n ?? "";

  return (
    <div className="game-room wl-room">
      {bar}

      <div className="wl-score panel">
        <span className="cn-score-red">Left {v.left}</span>
        <span className="cn-turn">
          {v.ph === "over"
            ? `${teamName(v.winner)} wins! 🎉`
            : `${teamName(v.active)}'s turn · first to 10`}
        </span>
        <span className="cn-score-blue">Right {v.right}</span>
      </div>

      <div className="wl-concepts">
        <span>{v.spectrum[0]}</span>
        <span>{v.spectrum[1]}</span>
      </div>

      <div className="wl-spectrum">
        {v.target != null &&
          [
            { w: 5, c: "wl-z2" },
            { w: 3, c: "wl-z3" },
            { w: 1, c: "wl-z4" },
          ].map((z, i) => (
            <div
              key={i}
              className={`wl-zone ${z.c}`}
              style={{ left: pct((v.target as number) - z.w / 2), width: `${(z.w / SCALE) * 100}%` }}
            />
          ))}
        <div className="wl-dial" style={{ left: pct(v.guess) }} />
      </div>

      {v.clue && (
        <p className="wl-clue">
          “{v.clue}” <small>— {psychicName}</small>
        </p>
      )}

      {/* phase controls */}
      {v.ph === "clue" &&
        (amPsychic ? (
          <ClueBox send={g.send} />
        ) : (
          <p className="muted-note wl-wait">{psychicName} sees the target and is thinking of a clue…</p>
        ))}

      {v.ph === "guess" &&
        (onActive && !amPsychic ? (
          <div className="mind-controls panel">
            <input
              type="range"
              min={0}
              max={SCALE}
              value={v.guess}
              onChange={(e) => g.send({ t: "dial", value: Number(e.target.value) })}
            />
            <button className="game-btn" onClick={() => g.send({ t: "lock" })}>
              Lock it in
            </button>
          </div>
        ) : (
          <p className="muted-note wl-wait">{teamName(v.active)} is dialling it in…</p>
        ))}

      {v.ph === "bet" &&
        (!onActive ? (
          <div className="mind-controls panel">
            <span className="muted-note">Is the real target left or right of their guess?</span>
            <button className="game-btn ghost" onClick={() => g.send({ t: "bet", side: "left" })}>
              ◀ Left
            </button>
            <button className="game-btn ghost" onClick={() => g.send({ t: "bet", side: "right" })}>
              Right ▶
            </button>
          </div>
        ) : (
          <p className="muted-note wl-wait">{teamName(1 - v.active)} is betting…</p>
        ))}

      {v.ph === "reveal" && (
        <div className="mind-controls panel mind-over">
          <strong>
            {teamName(v.active)} scored {v.lastScore}
            {v.betOk ? ` · ${teamName(1 - v.active)} +1 (bet ${v.counter})` : ` · ${teamName(1 - v.active)} missed the bet`}
          </strong>
          {isHost && (
            <button className="game-btn" onClick={() => g.send({ t: "next" })}>
              Next round
            </button>
          )}
        </div>
      )}

      {v.ph === "over" && isHost && (
        <div className="mind-controls panel mind-over">
          <button className="game-btn" onClick={() => g.send({ t: "restart" })}>
            New game
          </button>
        </div>
      )}
    </div>
  );
}

function ClueBox({ send }: { send: (m: object) => void }) {
  const [clue, setClue] = useState("");
  const give = () => {
    const c = clue.trim();
    if (c) {
      send({ t: "clue", clue: c });
      setClue("");
    }
  };
  return (
    <div className="mind-controls panel">
      <span className="muted-note">You see the target. Give a one-line clue:</span>
      <input
        className="trait-search"
        value={clue}
        maxLength={60}
        placeholder="e.g. a lukewarm coffee"
        onChange={(e) => setClue(e.target.value)}
        onKeyDown={(e) => e.key === "Enter" && give()}
      />
      <button className="game-btn" onClick={give}>
        Give clue
      </button>
    </div>
  );
}
